package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type FinanceProtocolDetectionCandidate struct {
	ProtocolID   int64  `json:"protocol_id"`
	ProtocolCode string `json:"protocol_code"`
	VersionID    int64  `json:"version_id"`
	Score        int    `json:"score"`
	MatchedRules int    `json:"matched_rules"`
}

type FinanceProtocolDetectionResult struct {
	Status     string                              `json:"status"`
	Selected   *FinanceProtocolDetectionCandidate  `json:"selected,omitempty"`
	Candidates []FinanceProtocolDetectionCandidate `json:"candidates"`
	Errors     []FinanceProtocolDetectionError     `json:"errors,omitempty"`
	Reason     string                              `json:"reason,omitempty"`
}

type FinanceProtocolDetectionError struct {
	ProtocolID   int64  `json:"protocol_id"`
	ProtocolCode string `json:"protocol_code"`
	ErrorCode    string `json:"error_code"`
	ErrorSummary string `json:"error_summary"`
}

type UpstreamFinanceProtocolDetector struct{ executor *UpstreamFinanceHTTPExecutor }

func NewUpstreamFinanceProtocolDetector(executor *UpstreamFinanceHTTPExecutor) *UpstreamFinanceProtocolDetector {
	return &UpstreamFinanceProtocolDetector{executor: executor}
}

func (d *UpstreamFinanceProtocolDetector) Detect(ctx context.Context, baseURL, credential, platform, accountType string, protocols []UpstreamFinanceProtocol) FinanceProtocolDetectionResult {
	return d.detect(ctx, baseURL, func(FinanceProtocolAuthentication) string { return credential }, platform, accountType, protocols)
}

func (d *UpstreamFinanceProtocolDetector) detect(ctx context.Context, baseURL string, credential func(FinanceProtocolAuthentication) string, platform, accountType string, protocols []UpstreamFinanceProtocol) FinanceProtocolDetectionResult {
	candidates := make([]FinanceProtocolDetectionCandidate, 0)
	detectionErrors := make([]FinanceProtocolDetectionError, 0)
	for _, protocol := range protocols {
		if protocol.Status != FinanceProtocolStatusPublished || protocol.CurrentVersion == nil {
			continue
		}
		config := protocol.CurrentVersion.Config
		score, matched := 0, 0
		for _, rule := range config.Recognition {
			if rule.Platform != "" && !strings.EqualFold(rule.Platform, platform) {
				continue
			}
			if rule.AccountType != "" && !strings.EqualFold(rule.AccountType, accountType) {
				continue
			}
			mapping := map[string]string{}
			if rule.Match.Path != "" {
				mapping["match"] = rule.Match.Path
			}
			testConfig := FinanceProtocolConfig{Capabilities: []string{"recognition"}, CostMode: FinanceCostModeManual, UnitSemantics: FinanceUnitNone, Authentication: config.Authentication, Operations: map[string]FinanceProtocolOperation{"recognition": {Method: rule.Method, Path: rule.Path, Mapping: mapping}}}
			result, err := d.executor.executeOperation(ctx, testConfig, "recognition", testConfig.Operations["recognition"], baseURL, credential(config.Authentication))
			if err != nil {
				detectionErrors = append(detectionErrors, FinanceProtocolDetectionError{ProtocolID: protocol.ID, ProtocolCode: protocol.Code, ErrorCode: classifyFinanceProtocolError(err), ErrorSummary: sanitizeFinanceProtocolError(err)})
				continue
			}
			value, exists := result.Facts["match"]
			if rule.Match.Status != 0 && result.StatusCode == rule.Match.Status {
				score += 20
			}
			if rule.Match.Exists != nil && (*rule.Match.Exists == exists) {
				score += 40
				matched++
			}
			if rule.Match.Equals != nil && fmt.Sprint(value) == fmt.Sprint(rule.Match.Equals) {
				score += 40
				matched++
			}
		}
		if score > 0 {
			candidates = append(candidates, FinanceProtocolDetectionCandidate{ProtocolID: protocol.ID, ProtocolCode: protocol.Code, VersionID: protocol.CurrentVersion.ID, Score: score, MatchedRules: matched})
		}
	}
	if len(candidates) == 0 {
		return FinanceProtocolDetectionResult{Status: "not_found", Candidates: candidates, Errors: detectionErrors, Reason: "manual_contract_required"}
	}
	best := candidates[0]
	conflict := false
	for _, candidate := range candidates[1:] {
		if candidate.Score > best.Score {
			best, conflict = candidate, false
		} else if candidate.Score == best.Score {
			conflict = true
		}
	}
	if best.Score < 40 {
		return FinanceProtocolDetectionResult{Status: "not_found", Candidates: candidates, Errors: detectionErrors, Reason: "score_below_threshold"}
	}
	if conflict {
		return FinanceProtocolDetectionResult{Status: "conflict", Candidates: candidates, Errors: detectionErrors, Reason: "multiple_protocols_share_highest_score"}
	}
	return FinanceProtocolDetectionResult{Status: "matched", Selected: &best, Candidates: candidates, Errors: detectionErrors}
}

func (s *UpstreamFinanceProtocolService) DetectAccount(ctx context.Context, account *Account, operatorID *int64) (FinanceProtocolDetectionResult, error) {
	if account == nil || account.ID <= 0 {
		return FinanceProtocolDetectionResult{}, financeValidationError("account is required")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(account.GetExtraString("custom_base_url"))
	}
	if baseURL == "" {
		return FinanceProtocolDetectionResult{}, financeValidationError("account base_url is required for protocol detection")
	}
	protocols, _, err := s.repo.ListProtocols(ctx, FinanceProtocolListFilter{Status: FinanceProtocolStatusPublished, Page: 1, PageSize: 100})
	if err != nil {
		return FinanceProtocolDetectionResult{}, err
	}
	result := s.detector.detect(ctx, baseURL, func(auth FinanceProtocolAuthentication) string {
		return accountFinanceProtocolCredential(account, auth.CredentialSource)
	}, account.Platform, account.Type, protocols)
	digest := sha256.Sum256([]byte(strings.TrimRight(strings.ToLower(baseURL), "/")))
	audit := FinanceProtocolDetectionAudit{
		AccountID: account.ID, Status: result.Status, Reason: result.Reason, Platform: account.Platform, AccountType: account.Type,
		BaseURLHash: hex.EncodeToString(digest[:]), Candidates: result.Candidates, OperatorID: operatorID,
	}
	if result.Selected != nil {
		protocolID, versionID := result.Selected.ProtocolID, result.Selected.VersionID
		audit.ProtocolID, audit.ProtocolVersionID = &protocolID, &versionID
	}
	if err := s.repo.CreateDetectionAudit(ctx, audit); err != nil {
		return FinanceProtocolDetectionResult{}, err
	}
	return result, nil
}

func accountFinanceProtocolCredential(account *Account, source string) string {
	if account == nil {
		return ""
	}
	keys := map[string][]string{
		"account_api_key":      {"api_key"},
		"account_access_token": {"access_token"},
		"account_token":        {"token"},
		"account_setup_token":  {"setup_token"},
		"":                     {"api_key", "access_token", "token", "setup_token"},
	}[strings.ToLower(strings.TrimSpace(source))]
	for _, key := range keys {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return value
		}
	}
	return ""
}
