package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

func TestSetOpsRequestBodyDiagnostics_IncompleteBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req, err := http.NewRequest(http.MethodPost, "/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.ContentLength = 128
	c.Request = req

	setOpsRequestBodyDiagnostics(c, &httputil.RequestBodyReadError{
		Kind:          httputil.RequestBodyReadIncompleteBody,
		BytesRead:     42,
		ContentLength: 128,
		Err:           io.ErrUnexpectedEOF,
	}, 25*time.Millisecond)

	v, ok := c.Get(opsRequestBodyDiagnosticsKey)
	if !ok {
		t.Fatal("expected diagnostics context")
	}
	diag, ok := v.(requestBodyDiagnostics)
	if !ok {
		t.Fatalf("unexpected diagnostics type %T", v)
	}
	if diag.Kind != "incomplete_body" || diag.BytesRead != 42 || diag.ContentLength != 128 || diag.ReadDurationMs != 25 {
		t.Fatalf("unexpected diagnostics: %+v", diag)
	}
}

func TestRequestBodyDiagnosticsErrorBody_WrapsResponse(t *testing.T) {
	body := `{"error":{"type":"invalid_request_error","message":"Request body is incomplete"}}`
	wrapped := requestBodyDiagnosticsErrorBody(body, requestBodyDiagnostics{Kind: "incomplete_body", BytesRead: 10})

	var payload map[string]any
	if err := json.Unmarshal([]byte(wrapped), &payload); err != nil {
		t.Fatalf("wrapped body should be json: %v", err)
	}
	if _, ok := payload["response"]; !ok {
		t.Fatalf("missing response: %s", wrapped)
	}
	diagnostics, ok := payload["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostics: %s", wrapped)
	}
	if diagnostics["kind"] != "incomplete_body" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestRequestBodyReadClientMessage_UsesClassifiedMessage(t *testing.T) {
	msg := requestBodyReadClientMessage(&httputil.RequestBodyReadError{
		Kind: httputil.RequestBodyReadClientDisconnected,
		Err:  errors.New("client disconnected"),
	})
	if msg != "Request body upload was interrupted before completion" {
		t.Fatalf("unexpected message: %s", msg)
	}
}
