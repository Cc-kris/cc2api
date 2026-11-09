package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// apiKeyOwnershipRepoStub 只实现 GetByID；其余方法被调用会因嵌入的 nil 接口而 panic。
type apiKeyOwnershipRepoStub struct {
	service.APIKeyRepository
	keys map[int64]*service.APIKey
}

func (s *apiKeyOwnershipRepoStub) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	key, ok := s.keys[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func newAPIKeyOwnershipTestRouter(t *testing.T, currentUserID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &apiKeyOwnershipRepoStub{keys: map[int64]*service.APIKey{
		100: {ID: 100, UserID: 1, Key: "sk-owned-by-a", Name: "a-key", Status: service.StatusActive},
		200: {ID: 200, UserID: 2, Key: "sk-owned-by-b", Name: "b-key", Status: service.StatusActive},
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)
	h := NewAPIKeyHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: currentUserID})
		c.Next()
	})
	r.GET("/api/v1/keys/:id", h.GetByID)
	return r
}

func getAPIKeyByID(r *gin.Engine, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// V01-AC37：别人的 Key 和不存在的 Key 返回同样的 404 和同样的响应体。
func TestAPIKeyGetByIDForeignAndMissingAreIndistinguishable(t *testing.T) {
	r := newAPIKeyOwnershipTestRouter(t, 1)

	foreign := getAPIKeyByID(r, "200")
	missing := getAPIKeyByID(r, "999")

	require.Equal(t, http.StatusNotFound, foreign.Code)
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.JSONEq(t, missing.Body.String(), foreign.Body.String())
	require.NotContains(t, foreign.Body.String(), "sk-owned-by-b")
	require.NotContains(t, foreign.Body.String(), "b-key")
}

func TestAPIKeyGetByIDOwnKeyStillReturned(t *testing.T) {
	r := newAPIKeyOwnershipTestRouter(t, 1)

	w := getAPIKeyByID(r, "100")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "a-key")
}
