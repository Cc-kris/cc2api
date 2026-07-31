package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type protocolDetectionRepoStub struct{ audit FinanceProtocolDetectionAudit }

func (s *protocolDetectionRepoStub) ListProtocols(context.Context, FinanceProtocolListFilter) ([]UpstreamFinanceProtocol, int64, error) {
	return nil, 0, nil
}
func (s *protocolDetectionRepoStub) GetProtocol(context.Context, int64) (*UpstreamFinanceProtocol, error) {
	return nil, ErrUpstreamFinanceProtocolNotFound
}
func (s *protocolDetectionRepoStub) GetVersion(context.Context, int64) (*UpstreamFinanceProtocolVersion, error) {
	return nil, ErrUpstreamFinanceProtocolNotFound
}
func (s *protocolDetectionRepoStub) ListVersions(context.Context, int64) ([]UpstreamFinanceProtocolVersion, error) {
	return nil, nil
}
func (s *protocolDetectionRepoStub) CreateProtocol(context.Context, FinanceProtocolCreateInput, FinanceProtocolValidationResult) (*UpstreamFinanceProtocol, error) {
	return nil, nil
}
func (s *protocolDetectionRepoStub) CreateDraftVersion(context.Context, int64, FinanceProtocolDraftInput, FinanceProtocolValidationResult) (*UpstreamFinanceProtocolVersion, error) {
	return nil, nil
}
func (s *protocolDetectionRepoStub) PublishVersion(context.Context, int64, int64, *int64) error {
	return nil
}
func (s *protocolDetectionRepoStub) DisableProtocol(context.Context, int64, *int64) error { return nil }
func (s *protocolDetectionRepoStub) DeleteDraft(context.Context, int64) error             { return nil }
func (s *protocolDetectionRepoStub) CreateDetectionAudit(_ context.Context, audit FinanceProtocolDetectionAudit) error {
	s.audit = audit
	return nil
}

func TestFinanceProtocolDetectorReturnsConflictInsteadOfSilentSelection(t *testing.T) {
	executor := NewUpstreamFinanceHTTPExecutorWithClient(financeProtocolDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"kind":"compatible"}`)), Header: make(http.Header)}, nil
	}))
	detector := NewUpstreamFinanceProtocolDetector(executor)
	protocols := []UpstreamFinanceProtocol{publishedDetectionProtocol(1, "one"), publishedDetectionProtocol(2, "two")}
	result := detector.Detect(context.Background(), "https://example.com", "", "", "", protocols)
	require.Equal(t, "conflict", result.Status)
	require.Nil(t, result.Selected)
	require.Len(t, result.Candidates, 2)
}

func TestFinanceProtocolDetectorDoesNotPerformModelConsumption(t *testing.T) {
	requested := make([]string, 0)
	executor := NewUpstreamFinanceHTTPExecutorWithClient(financeProtocolDoerFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Path)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"kind":"compatible"}`)), Header: make(http.Header)}, nil
	}))
	result := NewUpstreamFinanceProtocolDetector(executor).Detect(context.Background(), "https://example.com", "", "", "", []UpstreamFinanceProtocol{publishedDetectionProtocol(1, "one")})
	require.Equal(t, "matched", result.Status)
	require.Equal(t, []string{"/api/status"}, requested)
}

func TestUpstreamFinanceProtocolServiceDetectAccountPersistsCredentialFreeAudit(t *testing.T) {
	repo := &protocolDetectionRepoStub{}
	svc := NewUpstreamFinanceProtocolService(repo, NewUpstreamFinanceHTTPExecutor())
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://upstream.example/v1", "api_key": "should-not-persist"}}
	result, err := svc.DetectAccount(context.Background(), account, nil)
	require.NoError(t, err)
	require.Equal(t, "not_found", result.Status)
	require.Equal(t, int64(7), repo.audit.AccountID)
	require.Equal(t, "not_found", repo.audit.Status)
	require.Len(t, repo.audit.BaseURLHash, 64)
	require.NotContains(t, repo.audit.BaseURLHash, "upstream.example")
}

func publishedDetectionProtocol(id int64, code string) UpstreamFinanceProtocol {
	version := &UpstreamFinanceProtocolVersion{ID: id * 10, ProtocolID: id, Config: FinanceProtocolConfig{
		Authentication: FinanceProtocolAuthentication{Type: "none"},
		Recognition:    []FinanceProtocolRecognitionRule{{Method: http.MethodGet, Path: "/api/status", Match: FinanceProtocolRecognitionMatch{Path: "$.kind", Exists: boolPointer(true)}}},
	}}
	return UpstreamFinanceProtocol{ID: id, Code: code, Status: FinanceProtocolStatusPublished, CurrentVersion: version}
}
