package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamWalletRepoStub struct {
	wallet                  *UpstreamWallet
	assignmentInput         UpstreamWalletAssignmentInput
	bindableProtocolVersion bool
	activeAccountIDs        []int64
}

func (s *upstreamWalletRepoStub) ListWallets(context.Context, int64, bool) ([]UpstreamWallet, error) {
	return nil, nil
}
func (s *upstreamWalletRepoStub) GetWallet(context.Context, int64) (*UpstreamWallet, error) {
	copy := *s.wallet
	copy.EncryptedCredential = append([]byte(nil), s.wallet.EncryptedCredential...)
	return &copy, nil
}
func (s *upstreamWalletRepoStub) CreateWallet(_ context.Context, wallet *UpstreamWallet, _, _, _ string) error {
	wallet.ID = 8
	s.wallet = wallet
	return nil
}
func (s *upstreamWalletRepoStub) UpdateWallet(_ context.Context, wallet *UpstreamWallet, _, _, _ string, _ bool) error {
	s.wallet = wallet
	return nil
}
func (s *upstreamWalletRepoStub) SoftDeleteWallet(context.Context, int64, time.Time) error {
	return nil
}
func (s *upstreamWalletRepoStub) AssignWalletAccounts(_ context.Context, _ int64, input UpstreamWalletAssignmentInput) error {
	s.assignmentInput = input
	return nil
}
func (s *upstreamWalletRepoStub) ListActiveWalletAccountIDs(context.Context, int64, time.Time) ([]int64, error) {
	return append([]int64(nil), s.activeAccountIDs...), nil
}
func (s *upstreamWalletRepoStub) IsBindableProtocolVersion(context.Context, int64) (bool, error) {
	return s.bindableProtocolVersion, nil
}

type upstreamWalletEncryptorStub struct{}

func (upstreamWalletEncryptorStub) Encrypt(value string) (string, error) {
	return "encrypted:" + value, nil
}
func (upstreamWalletEncryptorStub) Decrypt(value string) (string, error) { return value, nil }

func TestUpstreamWalletServiceCreateEncryptsCredentialAndSanitizesURL(t *testing.T) {
	repo := &upstreamWalletRepoStub{}
	svc := NewUpstreamWalletService(repo, upstreamWalletEncryptorStub{})
	credential := " finance-secret "
	wallet, err := svc.Create(context.Background(), 3, UpstreamWalletInput{
		Name: " Main wallet ", AdapterType: "NEWAPI", BaseURL: "https://example.com/base/?token=leak#fragment",
		Credential: &credential, Currency: "usd", BalanceKind: "wallet_cash", PricingGroup: "vip",
	})
	require.NoError(t, err)
	require.Equal(t, int64(8), wallet.ID)
	require.Equal(t, "https://example.com/base", wallet.BaseURL)
	require.Equal(t, []byte("encrypted:finance-secret"), wallet.EncryptedCredential)
	require.True(t, wallet.CredentialConfigured)
	require.Equal(t, "USD", wallet.Currency)
}

func TestUpstreamWalletServiceUpdateWithoutCredentialPreservesCiphertext(t *testing.T) {
	enabled := false
	repo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{
		ID: 4, UpstreamID: 2, Name: "old", Enabled: true, EncryptedCredential: []byte("cipher"), CredentialConfigured: true,
	}}
	svc := NewUpstreamWalletService(repo, upstreamWalletEncryptorStub{})
	wallet, err := svc.Update(context.Background(), 4, UpstreamWalletInput{
		Name: "new", AdapterType: UpstreamAdapterManual, BaseURL: "https://example.com",
		Currency: "USD", BalanceKind: "token_quota", Enabled: &enabled,
	})
	require.NoError(t, err)
	require.Equal(t, []byte("cipher"), wallet.EncryptedCredential)
	require.False(t, wallet.Enabled)
}

func TestUpstreamWalletServiceManualWalletAllowsEmptyBaseURL(t *testing.T) {
	repo := &upstreamWalletRepoStub{}
	svc := NewUpstreamWalletService(repo, upstreamWalletEncryptorStub{})
	wallet, err := svc.Create(context.Background(), 3, UpstreamWalletInput{
		Name: "manual ledger", AdapterType: UpstreamAdapterManual, BaseURL: "",
		Currency: "USD", BalanceKind: "wallet_cash",
	})
	require.NoError(t, err)
	require.Equal(t, "", wallet.BaseURL)
}

func TestUpstreamWalletServiceBindsOnlyPublishedProtocolVersion(t *testing.T) {
	versionID := int64(91)
	credential := "vendor-secret"
	repo := &upstreamWalletRepoStub{bindableProtocolVersion: true}
	svc := NewUpstreamWalletService(repo, upstreamWalletEncryptorStub{})
	wallet, err := svc.Create(context.Background(), 3, UpstreamWalletInput{
		Name: "generic vendor", AdapterType: UpstreamAdapterProtocol, BaseURL: "https://vendor.example",
		Credential: &credential, ProtocolVersionID: &versionID, Currency: "USD", BalanceKind: "wallet_cash",
	})
	require.NoError(t, err)
	require.Equal(t, &versionID, wallet.ProtocolVersionID)
	require.Equal(t, UpstreamAdapterProtocol, wallet.AdapterType)

	repo.bindableProtocolVersion = false
	otherVersionID := int64(92)
	_, err = svc.Create(context.Background(), 3, UpstreamWalletInput{
		Name: "unpublished vendor", AdapterType: UpstreamAdapterProtocol, BaseURL: "https://vendor.example",
		Credential: &credential, ProtocolVersionID: &otherVersionID, Currency: "USD", BalanceKind: "wallet_cash",
	})
	require.EqualError(t, err, "protocol_version_id must reference a valid published protocol version")
}

func TestUpstreamWalletServiceRejectsEmptyProvidedCredential(t *testing.T) {
	repo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 4, UpstreamID: 2}}
	svc := NewUpstreamWalletService(repo, upstreamWalletEncryptorStub{})
	empty := "  "
	_, err := svc.Update(context.Background(), 4, UpstreamWalletInput{
		Name: "wallet", AdapterType: UpstreamAdapterManual, BaseURL: "https://example.com",
		Currency: "USD", BalanceKind: "wallet_cash", Credential: &empty,
	})
	require.EqualError(t, err, "credential must not be empty when provided")
}

func TestUpstreamWalletServiceAssignmentDeduplicatesAccounts(t *testing.T) {
	repo := &upstreamWalletRepoStub{}
	svc := NewUpstreamWalletService(repo, upstreamWalletEncryptorStub{})
	effectiveAt := time.Now().UTC()
	err := svc.AssignAccounts(context.Background(), 7, UpstreamWalletAssignmentInput{
		AccountIDs: []int64{3, 3, 5}, EffectiveAt: effectiveAt, Reason: "move to settlement wallet",
	})
	require.NoError(t, err)
	require.Equal(t, []int64{3, 5}, repo.assignmentInput.AccountIDs)
}
