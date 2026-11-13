//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type v043FailoverWriteRepo struct {
	AccountRepository
	account                      *Account
	creates, updates, bulkWrites int
}

func (r *v043FailoverWriteRepo) Create(_ context.Context, a *Account) error {
	r.creates++
	a.ID = 1
	r.account = a
	return nil
}
func (r *v043FailoverWriteRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *v043FailoverWriteRepo) Update(_ context.Context, a *Account) error {
	r.updates++
	r.account = a
	return nil
}
func (r *v043FailoverWriteRepo) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	r.bulkWrites++
	return int64(len(ids)), nil
}

func (r *v043FailoverWriteRepo) BulkUpdateWithGroupsValidated(_ context.Context, ids []int64, _ AccountBulkUpdate, _ []int64, _ []AccountBulkUpdateTarget, _ bool) ([]int64, error) {
	r.bulkWrites++
	return append([]int64(nil), ids...), nil
}

func TestV043PoolRetryStatusCodesWriteValidation(t *testing.T) {
	for _, payload := range []string{`[502.5,"abc",700]`, `[429.5]`, `["429"]`, `["abc"]`, `[99]`, `[600]`, `[null]`, `[true]`, `null`, `"429"`, `{}`} {
		for _, operation := range []string{"create", "update", "bulk"} {
			t.Run(operation+"/"+payload, func(t *testing.T) {
				var value any
				require.NoError(t, json.Unmarshal([]byte(payload), &value))
				repo := &v043FailoverWriteRepo{account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "old"}}}
				svc := &adminServiceImpl{accountRepo: repo}
				creds := map[string]any{"pool_mode_retry_status_codes": value}
				var err error
				switch operation {
				case "create":
					_, err = svc.CreateAccount(context.Background(), &CreateAccountInput{Credentials: creds, SkipDefaultGroupBind: true})
				case "update":
					_, err = svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Credentials: creds})
				case "bulk":
					_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Credentials: creds})
				}
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
				require.Zero(t, repo.creates+repo.updates+repo.bulkWrites)
				require.Equal(t, map[string]any{"api_key": "old"}, repo.account.Credentials)
			})
		}
	}
}

func TestV043PoolRetryStatusCodesWritePreservesMissingAndEmpty(t *testing.T) {
	for _, payload := range []string{`{}`, `{"pool_mode_retry_status_codes":[]}`, `{"pool_mode_retry_status_codes":[400,429,502,529]}`} {
		t.Run(payload, func(t *testing.T) {
			var creds map[string]any
			require.NoError(t, json.Unmarshal([]byte(payload), &creds))
			repo := &v043FailoverWriteRepo{}
			svc := &adminServiceImpl{accountRepo: repo}
			a, err := svc.CreateAccount(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: creds, SkipDefaultGroupBind: true})
			require.NoError(t, err)
			require.Equal(t, 1, repo.creates)
			require.Equal(t, creds, a.Credentials)
			a, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: creds})
			require.NoError(t, err)
			require.Equal(t, 1, repo.updates)
			require.Equal(t, creds, a.Credentials)
			_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Credentials: creds})
			require.NoError(t, err)
			// An entirely omitted bulk update has no columns to persist.
			if len(creds) > 0 {
				require.Equal(t, 1, repo.bulkWrites)
			}
		})
	}
}
