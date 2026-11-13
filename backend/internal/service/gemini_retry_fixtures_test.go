//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

type geminiSequenceUpstreamResponse struct {
	status int
	body   string
}
type geminiSequenceUpstreamStub struct {
	responses []geminiSequenceUpstreamResponse
	calls     int
}

func (s *geminiSequenceUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	r := s.responses[min(s.calls, len(s.responses)-1)]
	s.calls++
	return &http.Response{StatusCode: r.status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(r.body))}, nil
}
func (s *geminiSequenceUpstreamStub) DoWithTLS(r *http.Request, p string, id int64, c int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(r, p, id, c)
}
func newGeminiSingleCheckService(responses ...geminiSequenceUpstreamResponse) (*GeminiMessagesCompatService, *geminiSequenceUpstreamStub, *errorPolicyRepoStub) {
	repo := &errorPolicyRepoStub{}
	upstream := &geminiSequenceUpstreamStub{responses: responses}
	return &GeminiMessagesCompatService{accountRepo: repo, httpUpstream: upstream, rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil), cfg: &config.Config{}}, upstream, repo
}

var geminiSingleCheckForwarders = []struct {
	name    string
	forward func(*testing.T, *GeminiMessagesCompatService, *Account) (error, *gin.Context)
}{
	{"native", func(t *testing.T, s *GeminiMessagesCompatService, a *Account) (error, *gin.Context) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", nil)
		_, err := s.ForwardNative(context.Background(), c, a, "gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
		return err, c
	}},
	{"claude_compat", func(t *testing.T, s *GeminiMessagesCompatService, a *Account) (error, *gin.Context) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		b := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(b))
		_, err := s.Forward(context.Background(), c, a, b)
		return err, c
	}},
	{"chat_completions_compat", func(t *testing.T, s *GeminiMessagesCompatService, a *Account) (error, *gin.Context) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		b := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
		_, err := s.ForwardAsChatCompletions(context.Background(), c, a, b)
		return err, c
	}},
}
