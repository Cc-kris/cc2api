package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func seedOpsAIAnalysisConfig(t *testing.T, svc *OpsService, baseURL string, interfaceType string) {
	t.Helper()
	req := validOpsAIConfigUpdate("sk-test-secret")
	req.BaseURL = baseURL
	req.InterfaceType = interfaceType
	req.Model = "gpt-5.5"
	req.TimeoutSeconds = 5
	if _, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), req); err != nil {
		t.Fatalf("seed AI config: %v", err)
	}
}

func TestOpsAIAnalysisConnectionTestResponsesSuccess(t *testing.T) {
	var gotPath, gotAuth string
	svcClient := newOpsAITestHTTPClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "gpt-5.5" || body["input"] == "" {
			t.Fatalf("unexpected body: %+v", body)
		}
		return opsAITestResponse(http.StatusOK, `{"id":"resp_1"}`), nil
	})

	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo, cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}}, secretEncryptor: opsAIConfigEncryptorStub{}}
	svc.SetAIAnalysisHTTPClient(svcClient)
	seedOpsAIAnalysisConfig(t, svc, "https://93.184.216.34/v1", "responses")

	got, err := svc.TestOpsAIAnalysisConnection(context.Background())
	if err != nil {
		t.Fatalf("TestOpsAIAnalysisConnection() error = %v", err)
	}
	if !got.Success || got.Status != OpsAIAnalysisConnectionStatusSuccess {
		t.Fatalf("unexpected result: %+v", got)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotAuth != "Bearer sk-test-secret" {
		t.Fatalf("auth header = %q", gotAuth)
	}
}

func TestOpsAIAnalysisConnectionTestClassifiesAuthNetworkTimeoutAndConfig(t *testing.T) {
	t.Run("auth_failed", func(t *testing.T) {
		repo := newRuntimeSettingRepoStub()
		svc := &OpsService{settingRepo: repo, cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}}, secretEncryptor: opsAIConfigEncryptorStub{}}
		svc.SetAIAnalysisHTTPClient(newOpsAITestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return opsAITestResponse(http.StatusUnauthorized, `{}`), nil
		}))
		seedOpsAIAnalysisConfig(t, svc, "https://93.184.216.34", "openai_compatible")

		got, err := svc.TestOpsAIAnalysisConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Success || got.Status != OpsAIAnalysisConnectionStatusAuthFailed || got.HTTPStatus != http.StatusUnauthorized {
			t.Fatalf("unexpected result: %+v", got)
		}
	})

	t.Run("network_failed", func(t *testing.T) {
		repo := newRuntimeSettingRepoStub()
		svc := &OpsService{settingRepo: repo, cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}}, secretEncryptor: opsAIConfigEncryptorStub{}}
		svc.SetAIAnalysisHTTPClient(newOpsAITestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}))
		seedOpsAIAnalysisConfig(t, svc, "https://93.184.216.34", "responses")

		got, err := svc.TestOpsAIAnalysisConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Success || got.Status != OpsAIAnalysisConnectionStatusNetworkFail {
			t.Fatalf("unexpected result: %+v", got)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		repo := newRuntimeSettingRepoStub()
		svc := &OpsService{settingRepo: repo, cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}}, secretEncryptor: opsAIConfigEncryptorStub{}}
		svc.SetAIAnalysisHTTPClient(newOpsAITestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}))
		seedOpsAIAnalysisConfig(t, svc, "https://93.184.216.34", "responses")

		got, err := svc.TestOpsAIAnalysisConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Success || got.Status != OpsAIAnalysisConnectionStatusTimeout {
			t.Fatalf("unexpected result: %+v", got)
		}
	})

	t.Run("config_missing", func(t *testing.T) {
		repo := newRuntimeSettingRepoStub()
		svc := &OpsService{settingRepo: repo, cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}}, secretEncryptor: opsAIConfigEncryptorStub{}}

		got, err := svc.TestOpsAIAnalysisConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Success || got.Status != OpsAIAnalysisConnectionStatusConfigError || !strings.Contains(got.Message, "请先配置") {
			t.Fatalf("unexpected result: %+v", got)
		}
	})
}

func TestBuildOpsAIAnalysisProbeRequestByInterfaceType(t *testing.T) {
	for _, tt := range []struct {
		name          string
		interfaceType string
		wantPathPart  string
		wantHeader    string
	}{
		{"openai", "openai_compatible", "/v1/chat/completions", "Authorization"},
		{"anthropic", "anthropic_compatible", "/v1/messages", "x-api-key"},
		{"gemini", "gemini_compatible", "/v1beta/models/gpt-5.5:generateContent", "x-goog-api-key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := buildOpsAIAnalysisProbeRequest(context.Background(), &OpsAIAnalysisConfig{BaseURL: "https://93.184.216.34", Model: "gpt-5.5", InterfaceType: tt.interfaceType}, "sk-test")
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if !strings.Contains(req.URL.String(), tt.wantPathPart) {
				t.Fatalf("url = %s, want contains %s", req.URL.String(), tt.wantPathPart)
			}
			if req.Header.Get(tt.wantHeader) == "" {
				t.Fatalf("missing header %s", tt.wantHeader)
			}
		})
	}
}

func TestOpsAIAnalysisConnectionBlocksSSRFHostsBeforeRequest(t *testing.T) {
	for _, baseURL := range []string{
		"http://127.0.0.1:8080",
		"http://localhost:8080",
		"http://169.254.169.254",
		"http://10.0.0.8",
	} {
		t.Run(baseURL, func(t *testing.T) {
			repo := newRuntimeSettingRepoStub()
			svc := &OpsService{settingRepo: repo, cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}}, secretEncryptor: opsAIConfigEncryptorStub{}}
			called := false
			svc.SetAIAnalysisHTTPClient(newOpsAITestHTTPClient(func(r *http.Request) (*http.Response, error) {
				called = true
				return opsAITestResponse(http.StatusOK, `{}`), nil
			}))
			seedOpsAIAnalysisConfig(t, svc, baseURL, "responses")

			got, err := svc.TestOpsAIAnalysisConnection(context.Background())
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if called {
				t.Fatalf("SSRF-blocked URL still issued HTTP request")
			}
			if got.Success || got.Status != OpsAIAnalysisConnectionStatusConfigError || !strings.Contains(got.Message, "不允许访问") {
				t.Fatalf("unexpected result: %+v", got)
			}
			if strings.Contains(got.Message, "127.0.0.1") || strings.Contains(got.Message, "169.254") || strings.Contains(got.Message, "10.0.0.8") {
				t.Fatalf("message leaked internal address detail: %q", got.Message)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newOpsAITestHTTPClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: roundTripFunc(fn)}
}

func opsAITestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), Header: http.Header{}}
}
