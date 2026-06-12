package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type contextAwareAccountRepoStub struct {
	AccountRepository
}

func (r contextAwareAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, context.Canceled
}

func TestAccountTestService_TestAccountConnectionReportsCanceledContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &AccountTestService{accountRepo: contextAwareAccountRepoStub{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil).WithContext(ctx)

	err := svc.TestAccountConnection(c, 1, "", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Account test canceled: context canceled")
	require.Contains(t, rec.Body.String(), "Account test canceled: context canceled")
	require.NotContains(t, rec.Body.String(), "Account not found")
}
