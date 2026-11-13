//go:build unit

package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type v043NoAccountHTTPRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r *v043NoAccountHTTPRepo) ListByGroup(context.Context, int64) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *v043NoAccountHTTPRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return nil, nil
}
func (r *v043NoAccountHTTPRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return nil, nil
}
func TestV043NoAccountRealHTTPResponsesMessagesChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformGrok} {
		for _, route := range []string{"responses", "messages", "chat/completions"} {
			t.Run(platform+"/"+route, func(t *testing.T) {
				groupID := int64(5)
				repo := &v043NoAccountHTTPRepo{accounts: []service.Account{{ID: 1, Platform: platform, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"known": "known"}}}}}
				cfg := &config.Config{}
				cfg.RunMode = config.RunModeSimple
				gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				cache := &concurrencyCacheMock{acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil }}
				h := &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{}, concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)}
				key := &service.APIKey{ID: 2, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: platform, Status: service.StatusActive, AllowMessagesDispatch: true}, User: &service.User{ID: 3, Status: service.StatusActive}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Set(string(middleware.ContextKeyAPIKey), key)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3, Concurrency: 1})
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+route, strings.NewReader(`{"model":"foo-model","max_tokens":20,"input":"hi","messages":[{"role":"user","content":"hi"}]}`))
				switch route {
				case "responses":
					h.Responses(c)
				case "messages":
					h.Messages(c)
				default:
					h.ChatCompletions(c)
				}
				require.Equal(t, 404, rec.Code, "%s", rec.Body.String())
				require.Contains(t, rec.Body.String(), "model_not_found")
			})
		}
	}
}
