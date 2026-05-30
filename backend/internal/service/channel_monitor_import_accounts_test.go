package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type importAccountsEncryptor struct{}

func (importAccountsEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}
func (importAccountsEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

type importAccountsMonitorRepo struct {
	ChannelMonitorRepository
	items   []*ChannelMonitor
	created []*ChannelMonitor
	nextID  int64
}

func (r *importAccountsMonitorRepo) List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	_ = ctx
	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(r.items) {
		return []*ChannelMonitor{}, int64(len(r.items)), nil
	}
	end := start + pageSize
	if end > len(r.items) {
		end = len(r.items)
	}
	out := make([]*ChannelMonitor, 0, end-start)
	for _, item := range r.items[start:end] {
		copy := *item
		out = append(out, &copy)
	}
	return out, int64(len(r.items)), nil
}

func (r *importAccountsMonitorRepo) Create(ctx context.Context, m *ChannelMonitor) error {
	_ = ctx
	if r.nextID == 0 {
		r.nextID = 100
	}
	copy := *m
	copy.ID = r.nextID
	copy.CreatedAt = time.Now().UTC()
	copy.UpdatedAt = copy.CreatedAt
	r.nextID++
	r.created = append(r.created, &copy)
	m.ID = copy.ID
	m.CreatedAt = copy.CreatedAt
	m.UpdatedAt = copy.UpdatedAt
	return nil
}

type importAccountsAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *importAccountsAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	_ = ctx
	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(r.accounts) {
		return []Account{}, &pagination.PaginationResult{Total: int64(len(r.accounts)), Page: page, PageSize: pageSize}, nil
	}
	end := start + pageSize
	if end > len(r.accounts) {
		end = len(r.accounts)
	}
	out := append([]Account(nil), r.accounts[start:end]...)
	return out, &pagination.PaginationResult{Total: int64(len(r.accounts)), Page: page, PageSize: pageSize}, nil
}

func TestChannelMonitorServiceCreateFromAccounts(t *testing.T) {
	monitorRepo := &importAccountsMonitorRepo{items: []*ChannelMonitor{
		{ID: 1, Provider: MonitorProviderOpenAI, Endpoint: "https://api.openai.com", APIKey: "enc:sk-existing"},
	}}
	accountRepo := &importAccountsAccountRepo{accounts: []Account{
		{ID: 10, Name: "duplicate", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-existing", "base_url": "https://api.openai.com/v1"}},
		{ID: 11, Name: "gpt upstream", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-openai", "base_url": "https://api.openai.com/v1/responses"}},
		{ID: 12, Name: "claude upstream", Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-ant", "base_url": "https://api.anthropic.com/v1"}},
		{ID: 13, Name: "gemini upstream", Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "gem-key", "base_url": "https://generativelanguage.googleapis.com/v1beta"}},
		{ID: 14, Name: "gemini oauth", Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "ya29"}},
		{ID: 15, Name: "unsupported", Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "ag-key"}},
		{ID: 16, Name: "openai oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "oa-token"}},
		{ID: 17, Name: "gpt chat", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-chat", "base_url": "https://api.openai.com/v1"}, Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}},
	}}
	svc := NewChannelMonitorService(monitorRepo, importAccountsEncryptor{}, accountRepo)

	result, err := svc.CreateFromAccounts(context.Background(), 99)
	require.NoError(t, err)
	require.Equal(t, &ChannelMonitorImportAccountsResult{TotalAccounts: 8, Created: 4, SkippedDuplicate: 1, SkippedUnsupported: 3}, result)
	require.Len(t, monitorRepo.created, 4)

	require.Equal(t, "gpt upstream", monitorRepo.created[0].Name)
	require.Equal(t, MonitorProviderOpenAI, monitorRepo.created[0].Provider)
	require.Equal(t, MonitorAPIModeResponses, monitorRepo.created[0].APIMode)
	require.Equal(t, "https://api.openai.com", monitorRepo.created[0].Endpoint)
	require.Equal(t, "enc:sk-openai", monitorRepo.created[0].APIKey)
	require.Equal(t, "gpt-5.4-mini", monitorRepo.created[0].PrimaryModel)
	require.Equal(t, monitorImportDefaultIntervalSeconds, monitorRepo.created[0].IntervalSeconds)
	require.Equal(t, int64(99), monitorRepo.created[0].CreatedBy)

	require.Equal(t, "claude upstream", monitorRepo.created[1].Name)
	require.Equal(t, MonitorProviderAnthropic, monitorRepo.created[1].Provider)
	require.Equal(t, "https://api.anthropic.com", monitorRepo.created[1].Endpoint)
	require.Equal(t, "claude-haiku-4-5", monitorRepo.created[1].PrimaryModel)

	require.Equal(t, "gemini upstream", monitorRepo.created[2].Name)
	require.Equal(t, MonitorProviderGemini, monitorRepo.created[2].Provider)
	require.Equal(t, "https://generativelanguage.googleapis.com", monitorRepo.created[2].Endpoint)
	require.Equal(t, "gemini-3-flash", monitorRepo.created[2].PrimaryModel)

	require.Equal(t, "gpt chat", monitorRepo.created[3].Name)
	require.Equal(t, MonitorProviderOpenAI, monitorRepo.created[3].Provider)
	require.Equal(t, MonitorAPIModeChatCompletions, monitorRepo.created[3].APIMode)
	require.Equal(t, "https://api.openai.com", monitorRepo.created[3].Endpoint)
	require.Equal(t, "gpt-5.4-mini", monitorRepo.created[3].PrimaryModel)
}

func TestMonitorEndpointOrigin(t *testing.T) {
	endpoint, ok := monitorEndpointOrigin("https://api.example.com/v1/models?key=redacted#frag")
	require.True(t, ok)
	require.Equal(t, "https://api.example.com", endpoint)
}
