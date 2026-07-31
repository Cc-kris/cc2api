package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamWalletHandlerRepo struct {
	wallet    *service.UpstreamWallet
	createErr error
}

func (r *upstreamWalletHandlerRepo) ListWallets(context.Context, int64, bool) ([]service.UpstreamWallet, error) {
	return nil, nil
}
func (r *upstreamWalletHandlerRepo) GetWallet(context.Context, int64) (*service.UpstreamWallet, error) {
	if r.wallet == nil {
		return nil, service.ErrUpstreamWalletNotFound
	}
	return r.wallet, nil
}
func (r *upstreamWalletHandlerRepo) CreateWallet(_ context.Context, wallet *service.UpstreamWallet, _, _, _ string) error {
	if r.createErr != nil {
		return r.createErr
	}
	wallet.ID = 12
	wallet.CreatedAt = time.Now().UTC()
	wallet.UpdatedAt = wallet.CreatedAt
	r.wallet = wallet
	return nil
}
func (r *upstreamWalletHandlerRepo) UpdateWallet(context.Context, *service.UpstreamWallet, string, string, string, bool) error {
	return nil
}
func (r *upstreamWalletHandlerRepo) SoftDeleteWallet(context.Context, int64, time.Time) error {
	return nil
}
func (r *upstreamWalletHandlerRepo) AssignWalletAccounts(context.Context, int64, service.UpstreamWalletAssignmentInput) error {
	return nil
}
func (r *upstreamWalletHandlerRepo) ListActiveWalletAccountIDs(context.Context, int64, time.Time) ([]int64, error) {
	return nil, nil
}
func (r *upstreamWalletHandlerRepo) IsBindableProtocolVersion(context.Context, int64) (bool, error) {
	return false, nil
}

type upstreamWalletHandlerEncryptor struct{}

func (upstreamWalletHandlerEncryptor) Encrypt(value string) (string, error) {
	return "cipher:" + value, nil
}
func (upstreamWalletHandlerEncryptor) Decrypt(value string) (string, error) { return value, nil }

type upstreamWalletHandlerFundRepo struct{ events []service.UpstreamFundEvent }

func (r *upstreamWalletHandlerFundRepo) CreateFundEvent(context.Context, *service.UpstreamFundEvent) (bool, error) {
	return true, nil
}
func (r *upstreamWalletHandlerFundRepo) GetFundEvent(_ context.Context, walletID, eventID int64) (*service.UpstreamFundEvent, error) {
	for index := range r.events {
		if r.events[index].WalletID == walletID && r.events[index].ID == eventID {
			return &r.events[index], nil
		}
	}
	return nil, service.ErrUpstreamFundEventNotFound
}
func (r *upstreamWalletHandlerFundRepo) ListFundEvents(_ context.Context, walletID int64, _, _ int) ([]service.UpstreamFundEvent, int64, error) {
	items := make([]service.UpstreamFundEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.WalletID == walletID {
			items = append(items, event)
		}
	}
	return items, int64(len(items)), nil
}

func TestUpstreamWalletHandlerCreateDoesNotExposeCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &upstreamWalletHandlerRepo{}
	handler := NewUpstreamWalletHandler(service.NewUpstreamWalletService(repo, upstreamWalletHandlerEncryptor{}), nil, nil)
	router := gin.New()
	router.POST("/upstreams/:upstream_id/wallets", handler.Create)
	body := []byte(`{"name":"main","adapter_type":"newapi","base_url":"https://example.com?secret=query","credential":"plain-secret","currency":"USD","balance_kind":"wallet_cash"}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upstreams/3/wallets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusCreated, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "plain-secret")
	require.NotContains(t, recorder.Body.String(), "cipher:")
	var envelope struct {
		Data service.UpstreamWallet `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(12), envelope.Data.ID)
	require.True(t, envelope.Data.CredentialConfigured)
	require.Equal(t, "https://example.com", envelope.Data.BaseURL)
}

func TestUpstreamWalletHandlerRejectsEmptyCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &upstreamWalletHandlerRepo{}
	handler := NewUpstreamWalletHandler(service.NewUpstreamWalletService(repo, upstreamWalletHandlerEncryptor{}), nil, nil)
	router := gin.New()
	router.POST("/upstreams/:upstream_id/wallets", handler.Create)
	body := []byte(`{"name":"main","adapter_type":"newapi","base_url":"https://example.com","credential":"","currency":"USD","balance_kind":"wallet_cash"}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upstreams/3/wallets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "credential must not be empty")
}

func TestUpstreamWalletHandlerDoesNotExposeRepositoryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &upstreamWalletHandlerRepo{createErr: errors.New("pq: duplicate key value violates unique constraint upstream_wallets_secret")}
	handler := NewUpstreamWalletHandler(service.NewUpstreamWalletService(repo, upstreamWalletHandlerEncryptor{}), nil, nil)
	router := gin.New()
	router.POST("/upstreams/:upstream_id/wallets", handler.Create)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upstreams/3/wallets", bytes.NewReader([]byte(`{"name":"main","adapter_type":"newapi","base_url":"https://example.com","credential":"secret","currency":"USD","balance_kind":"wallet_cash"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "duplicate key")
	require.NotContains(t, recorder.Body.String(), "upstream_wallets_secret")
}

func TestUpstreamWalletHandlerListsFundEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	walletRepo := &upstreamWalletHandlerRepo{wallet: &service.UpstreamWallet{ID: 7, UpstreamID: 3}}
	fundRepo := &upstreamWalletHandlerFundRepo{events: []service.UpstreamFundEvent{{ID: 11, WalletID: 7, EventType: "topup", BonusStatus: "confirmed"}}}
	handler := NewUpstreamWalletHandler(service.NewUpstreamWalletService(walletRepo, upstreamWalletHandlerEncryptor{}), nil, service.NewUpstreamFundService(service.NewUpstreamWalletService(walletRepo, upstreamWalletHandlerEncryptor{}), fundRepo))
	router := gin.New()
	router.GET("/upstream-wallets/:id/fund-events", handler.ListFundEvents)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/upstream-wallets/7/fund-events?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"bonus_status":"confirmed"`)
	require.Contains(t, recorder.Body.String(), `"total":1`)
}
