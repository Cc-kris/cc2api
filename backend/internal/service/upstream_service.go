package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
)

type UpstreamRepository interface {
	ListUpstreams(ctx context.Context) ([]*Upstream, error)
	GetUpstream(ctx context.Context, id int64) (*Upstream, error)
	CreateUpstream(ctx context.Context, input *UpstreamInput) (*Upstream, error)
	UpdateUpstream(ctx context.Context, id int64, input *UpstreamInput) (*Upstream, error)
	DeleteUpstream(ctx context.Context, id int64) error
	SyncFromAccounts(ctx context.Context) (int, error)
	GetUpstreamStats(ctx context.Context, start, end time.Time, granularity string) (*UpstreamStatsResponse, error)
	GetFinanceStats(ctx context.Context, start, end time.Time, granularity string) (*FinanceStatsResponse, error)
	ListBalanceAlertCandidates(ctx context.Context) ([]*Upstream, error)
	MarkBalanceAlertSent(ctx context.Context, id int64, currentBalance float64) error
	ResetBalanceAlert(ctx context.Context, id int64) error
}

type UpstreamService struct {
	repo        UpstreamRepository
	settingRepo SettingRepository
	email       *EmailService
}

func NewUpstreamService(repo UpstreamRepository, settingRepo SettingRepository, email *EmailService) *UpstreamService {
	return &UpstreamService{repo: repo, settingRepo: settingRepo, email: email}
}

func (s *UpstreamService) List(ctx context.Context) ([]*Upstream, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("upstream repository not initialized")
	}
	items, err := s.repo.ListUpstreams(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.sendBalanceAlerts(ctx, items)
	return items, nil
}

func (s *UpstreamService) Get(ctx context.Context, id int64) (*Upstream, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("upstream repository not initialized")
	}
	return s.repo.GetUpstream(ctx, id)
}

func (s *UpstreamService) Create(ctx context.Context, input *UpstreamInput) (*Upstream, error) {
	normalized, err := normalizeUpstreamInput(input)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.CreateUpstream(ctx, normalized)
	if err != nil {
		return nil, err
	}
	_ = s.sendBalanceAlerts(ctx, []*Upstream{item})
	return item, nil
}

func (s *UpstreamService) Update(ctx context.Context, id int64, input *UpstreamInput) (*Upstream, error) {
	normalized, err := normalizeUpstreamInput(input)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.UpdateUpstream(ctx, id, normalized)
	if err != nil {
		return nil, err
	}
	if item != nil && item.AlertBalance != nil && item.CurrentBalance > *item.AlertBalance {
		_ = s.repo.ResetBalanceAlert(context.Background(), item.ID)
	}
	_ = s.sendBalanceAlerts(ctx, []*Upstream{item})
	return item, nil
}

func (s *UpstreamService) Delete(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return errors.New("upstream repository not initialized")
	}
	return s.repo.DeleteUpstream(ctx, id)
}

func (s *UpstreamService) SyncFromAccounts(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("upstream repository not initialized")
	}
	created, err := s.repo.SyncFromAccounts(ctx)
	if err != nil {
		return 0, err
	}
	items, _ := s.repo.ListBalanceAlertCandidates(ctx)
	_ = s.sendBalanceAlerts(ctx, items)
	return created, nil
}

func (s *UpstreamService) GetStats(ctx context.Context, start, end time.Time, granularity string) (*UpstreamStatsResponse, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("upstream repository not initialized")
	}
	return s.repo.GetUpstreamStats(ctx, start, end, normalizeStatsGranularity(granularity))
}

func (s *UpstreamService) GetFinanceStats(ctx context.Context, start, end time.Time, granularity string) (*FinanceStatsResponse, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("upstream repository not initialized")
	}
	return s.repo.GetFinanceStats(ctx, start, end, normalizeStatsGranularity(granularity))
}

func NormalizeUpstreamBaseURLForRepo(raw string) string {
	return normalizeUpstreamBaseURL(raw)
}

func RoundMoneyForRepo(v float64) float64 {
	return roundMoney(v)
}

func normalizeUpstreamInput(input *UpstreamInput) (*UpstreamInput, error) {
	if input == nil {
		return nil, errors.New("invalid upstream input")
	}
	baseURL := normalizeUpstreamBaseURL(input.BaseURL)
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("base_url must be a valid http(s) URL")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = parsed.Host
	}
	if len(name) > 120 {
		return nil, errors.New("name must not exceed 120 characters")
	}
	rate := input.RateMultiplier
	if rate == 0 {
		rate = 1
	}
	if rate < 0 || rate > 1000000 {
		return nil, errors.New("rate_multiplier must be between 0 and 1000000")
	}
	if input.InitialBalance < 0 {
		return nil, errors.New("initial_balance must be >= 0")
	}
	if input.AlertBalance != nil && *input.AlertBalance < 0 {
		return nil, errors.New("alert_balance must be >= 0")
	}
	modelRates := make([]UpstreamModelRate, 0, len(input.ModelRates))
	seenModels := make(map[string]struct{}, len(input.ModelRates))
	for _, row := range input.ModelRates {
		model := strings.TrimSpace(row.Model)
		if model == "" {
			continue
		}
		if len(model) > 120 {
			return nil, errors.New("model must not exceed 120 characters")
		}
		key := strings.ToLower(model)
		if _, ok := seenModels[key]; ok {
			return nil, errors.New("duplicate model rate")
		}
		seenModels[key] = struct{}{}
		modelRate := row.RateMultiplier
		if modelRate == 0 {
			modelRate = 1
		}
		if modelRate < 0 || modelRate > 1000000 {
			return nil, errors.New("model rate_multiplier must be between 0 and 1000000")
		}
		modelRates = append(modelRates, UpstreamModelRate{ID: row.ID, Model: model, RateMultiplier: modelRate})
	}
	return &UpstreamInput{
		BaseURL:             baseURL,
		Name:                name,
		RateMultiplier:      rate,
		ModelRates:          modelRates,
		InitialBalance:      input.InitialBalance,
		BalanceAlertEnabled: input.BalanceAlertEnabled,
		AlertBalance:        input.AlertBalance,
		Notes:               strings.TrimSpace(input.Notes),
	}, nil
}

func normalizeUpstreamBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimRight(value, "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeStatsGranularity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hour", "day", "month":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "day"
	}
}

func roundMoney(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func (s *UpstreamService) sendBalanceAlerts(ctx context.Context, items []*Upstream) error {
	if s == nil || s.repo == nil || s.email == nil || len(items) == 0 {
		return nil
	}
	recipients := s.adminNotifyEmails(ctx)
	if len(recipients) == 0 {
		return nil
	}
	for _, item := range items {
		if item == nil || !item.BalanceAlertEnabled || item.AlertBalance == nil {
			continue
		}
		if item.CurrentBalance > *item.AlertBalance || item.AlertEmailSentAt != nil {
			continue
		}
		subject := fmt.Sprintf("[上游余额告警] %s 余额 %.4f", sanitizeEmailHeader(item.Name), item.CurrentBalance)
		body := buildUpstreamBalanceAlertEmail(item)
		anySent := false
		for _, to := range recipients {
			if err := s.email.SendEmail(ctx, to, subject, body); err == nil {
				anySent = true
			}
		}
		if anySent {
			_ = s.repo.MarkBalanceAlertSent(context.Background(), item.ID, item.CurrentBalance)
		}
	}
	return nil
}

func (s *UpstreamService) adminNotifyEmails(ctx context.Context) []string {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQuotaNotifyEmails)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	entries := ParseNotifyEmails(raw)
	emails := filterVerifiedEmails(entries)
	sort.Strings(emails)
	return emails
}

func buildUpstreamBalanceAlertEmail(item *Upstream) string {
	threshold := 0.0
	if item != nil && item.AlertBalance != nil {
		threshold = *item.AlertBalance
	}
	return fmt.Sprintf(`<h2>上游余额不足告警</h2><p>上游：%s</p><p>Base URL：%s</p><p>当前余额：%.4f</p><p>告警余额：%.4f</p><p>已消耗：%.4f</p>`,
		html.EscapeString(item.Name), html.EscapeString(item.BaseURL), item.CurrentBalance, threshold, item.ConsumedBalance)
}
