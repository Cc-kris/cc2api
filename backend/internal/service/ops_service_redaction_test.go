package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func syntheticGeminiCredential() string {
	// Constructed in pieces so test diagnostics never resemble a real credential.
	return "AQ." + "Ab" + strings.Repeat("Synthetic_W6-", 4) + "Z"
}

func TestIsSensitiveKey_TokenBudgetKeysNotRedacted(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		"max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"max_tokens_to_sample",
		"budget_tokens",
		"prompt_tokens",
		"completion_tokens",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"token_count",
	} {
		if isSensitiveKey(key) {
			t.Fatalf("expected key %q to NOT be treated as sensitive", key)
		}
	}

	for _, key := range []string{
		"authorization",
		"Authorization",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"token",
		"client_secret",
		"private_key",
		"signature",
	} {
		if !isSensitiveKey(key) {
			t.Fatalf("expected key %q to be treated as sensitive", key)
		}
	}
}

func TestSanitizeAndTrimJSONPayload_PreservesTokenBudgetFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"model":"claude-3","max_tokens":123,"thinking":{"type":"enabled","budget_tokens":456},"access_token":"abc","messages":[{"role":"user","content":"hi"}]}`)
	out, _, _ := sanitizeAndTrimJSONPayload(raw, 10*1024)
	if out == "" {
		t.Fatalf("expected non-empty sanitized output")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal sanitized output: %v", err)
	}

	if got, ok := decoded["max_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_tokens=123, got %#v", decoded["max_tokens"])
	}

	thinking, ok := decoded["thinking"].(map[string]any)
	if !ok || thinking == nil {
		t.Fatalf("expected thinking object to be preserved, got %#v", decoded["thinking"])
	}
	if got, ok := thinking["budget_tokens"].(float64); !ok || got != 456 {
		t.Fatalf("expected thinking.budget_tokens=456, got %#v", thinking["budget_tokens"])
	}

	if got := decoded["access_token"]; got != "[REDACTED]" {
		t.Fatalf("expected access_token to be redacted, got %#v", got)
	}
}

func TestOpsServiceRecordError_RedactsGeminiCredentialInAllPersistedErrorFields(t *testing.T) {
	t.Parallel()

	credential := syntheticGeminiCredential()
	status := 400
	message := fmt.Sprintf("provider rejected credential [%s] for model gemini-test", credential)
	body := fmt.Sprintf(`{"error":{"code":400,"message":%q,"status":"INVALID_ARGUMENT"}}`, message)
	var captured *OpsInsertErrorLogInput
	repo := &opsRepoMock{
		InsertErrorLogFn: func(_ context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			captured = input
			return 1, nil
		},
	}
	svc := &OpsService{opsRepo: repo}
	err := svc.RecordError(context.Background(), &OpsInsertErrorLogInput{
		ErrorMessage:         message,
		ErrorBody:            body,
		UpstreamStatusCode:   &status,
		UpstreamErrorMessage: &message,
		UpstreamErrorDetail:  &body,
		UpstreamErrors: []*OpsUpstreamErrorEvent{{
			UpstreamStatusCode: status,
			Kind:               "http_error",
			Message:            message,
			Detail:             body,
		}},
	})
	if err != nil {
		t.Fatalf("record synthetic error: %v", err)
	}
	if captured == nil {
		t.Fatal("expected persisted input")
	}
	persisted := []string{
		captured.ErrorMessage,
		captured.ErrorBody,
		*captured.UpstreamErrorMessage,
		*captured.UpstreamErrorDetail,
		*captured.UpstreamErrorsJSON,
	}
	for i, field := range persisted {
		if strings.Contains(field, credential) {
			t.Fatalf("persisted field %d contains synthetic credential", i)
		}
		if !strings.Contains(field, "provider rejected") {
			t.Fatalf("persisted field %d lost diagnostic message", i)
		}
	}
	if captured.UpstreamStatusCode == nil || *captured.UpstreamStatusCode != status {
		t.Fatalf("expected upstream status %d to be preserved", status)
	}
	for i, raw := range []string{captured.ErrorBody, *captured.UpstreamErrorDetail, *captured.UpstreamErrorsJSON} {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatalf("persisted JSON field %d is invalid: %v", i, err)
		}
	}
}

func TestOpsServiceGetErrorLogByID_RedactsLegacyStoredCredentialsWithoutTruncation(t *testing.T) {
	t.Parallel()

	credential := syntheticGeminiCredential()
	message := fmt.Sprintf("legacy provider error [%s] keep request_id=req-w6", credential)
	body := fmt.Sprintf(`{"error":{"code":400,"message":%q,"status":"INVALID_ARGUMENT"}}`, message)
	events := fmt.Sprintf(`[{"kind":"http_error","status":400,"message":%q,"detail":%q}]`, message, body)
	stored := &OpsErrorLogDetail{
		OpsErrorLog:          OpsErrorLog{ID: 25, Message: message, StatusCode: 400},
		ErrorBody:            body,
		UpstreamErrorMessage: message,
		UpstreamErrorDetail:  body,
		UpstreamErrors:       events,
	}
	repo := &opsRepoMock{GetErrorLogByIDFn: func(context.Context, int64) (*OpsErrorLogDetail, error) {
		return stored, nil
	}}
	svc := &OpsService{opsRepo: repo}
	detail, err := svc.GetErrorLogByID(context.Background(), stored.ID)
	if err != nil {
		t.Fatalf("read synthetic legacy detail: %v", err)
	}
	serialized, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("serialize legacy detail: %v", err)
	}
	if strings.Contains(string(serialized), credential) {
		t.Fatal("legacy detail contains synthetic credential")
	}
	for _, want := range []string{"legacy provider error", "request_id=req-w6", "INVALID_ARGUMENT"} {
		if !strings.Contains(string(serialized), want) {
			t.Fatalf("legacy detail lost diagnostic %q", want)
		}
	}
	if detail.StatusCode != 400 || detail.ID != 25 {
		t.Fatal("legacy detail lost status or identity")
	}
	for i, raw := range []string{detail.ErrorBody, detail.UpstreamErrorDetail, detail.UpstreamErrors} {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatalf("legacy JSON field %d is invalid: %v", i, err)
		}
	}
}

func TestSanitizeUpstreamErrorMessage_RedactsEmbeddedAndQueryCredentials(t *testing.T) {
	t.Parallel()

	credential := syntheticGeminiCredential()
	for _, input := range []string{
		"provider credential [" + credential + "] rejected",
		"request https://example.invalid/v1beta?key=" + credential + "&trace=w6 failed",
		`{"error":{"message":"credential ` + credential + ` rejected","status":"INVALID_ARGUMENT"}}`,
	} {
		got := sanitizeUpstreamErrorMessage(input)
		if strings.Contains(got, credential) {
			t.Fatal("sanitizer returned synthetic credential")
		}
		if !strings.Contains(got, "rejected") && !strings.Contains(got, "failed") {
			t.Fatal("sanitizer lost diagnostic text")
		}
	}
}

func TestOpsServiceGetErrorLogByID_RedactsLegacyDiagnosticTextField(t *testing.T) {
	t.Parallel()

	credential := syntheticGeminiCredential()
	stored := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{ID: 26, StatusCode: 400, ClassificationReason: "provider echoed " + credential},
	}
	repo := &opsRepoMock{GetErrorLogByIDFn: func(context.Context, int64) (*OpsErrorLogDetail, error) {
		return stored, nil
	}}
	svc := &OpsService{opsRepo: repo}
	detail, err := svc.GetErrorLogByID(context.Background(), stored.ID)
	if err != nil {
		t.Fatalf("read synthetic legacy detail: %v", err)
	}
	if strings.Contains(detail.ClassificationReason, credential) {
		t.Fatal("legacy classification reason contains synthetic credential")
	}
	if !strings.Contains(detail.ClassificationReason, "provider echoed AQ.***") {
		t.Fatalf("legacy classification reason lost context: %q", detail.ClassificationReason)
	}
	if detail.StatusCode != 400 {
		t.Fatal("legacy status changed")
	}
}

func TestRedactUpstreamCredentialJSONPreservesSensitiveKeyRedaction(t *testing.T) {
	t.Parallel()

	decoded := map[string]any{
		"api_key":  json.Number("123456"),
		"code":     json.Number("400"),
		"message":  "provider credential [" + syntheticGeminiCredential() + "]",
		"metadata": map[string]any{"request_id": "req-w6"},
	}
	sensitiveRedacted := redactSensitiveJSON(decoded)
	encoded, err := json.Marshal(redactGeminiCredentialJSON(sensitiveRedacted, 0))
	if err != nil {
		t.Fatalf("marshal structured redaction: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal structured redaction: %v", err)
	}
	if got["api_key"] != "[REDACTED]" {
		t.Fatalf("sensitive non-string key lost redaction: %#v", got["api_key"])
	}
	if got["code"] != float64(400) {
		t.Fatalf("numeric status changed: %#v", got["code"])
	}
	if strings.Contains(string(encoded), syntheticGeminiCredential()) {
		t.Fatal("structured redaction returned synthetic credential")
	}
	if !strings.Contains(string(encoded), "request_id") {
		t.Fatal("structured redaction removed unrelated metadata")
	}
}

func TestShrinkToEssentials_IncludesThinking(t *testing.T) {
	t.Parallel()

	root := map[string]any{
		"model":      "claude-3",
		"max_tokens": 100,
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 200,
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "first"},
			map[string]any{"role": "user", "content": "last"},
		},
	}

	out := shrinkToEssentials(root)
	if _, ok := out["thinking"]; !ok {
		t.Fatalf("expected thinking to be included in essentials: %#v", out)
	}
}
