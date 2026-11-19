package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPassthroughCreatedAtBufferedAndAggregated(t *testing.T) {
	for _, field := range []string{"", `,"created_at":null`, `,"created_at":1700000000`} {
		for _, aggregate := range []bool{false, true} {
			t.Run(fmt.Sprintf("field=%q/aggregate=%t", field, aggregate), func(t *testing.T) {
				body := `{"id":"resp_timestamp","object":"response","status":"completed","output":[]` + field + `}`
				contentType := "application/json"
				if aggregate {
					body = "data: " + `{"type":"response.completed","sequence_number":0,"response":` + body + "}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
				_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, "gpt-5.5", "gpt-5.5")
				require.NoError(t, err)
				createdAt := gjson.Get(recorder.Body.String(), "created_at")
				require.Equal(t, gjson.Number, createdAt.Type, recorder.Body.String())
				require.Positive(t, createdAt.Int())
				if strings.Contains(field, "1700000000") {
					require.EqualValues(t, 1700000000, createdAt.Int())
				}
				require.Equal(t, "resp_timestamp", gjson.Get(recorder.Body.String(), "id").String())
			})
		}
	}
}

func TestPassthroughCreatedAtStreamingLifecycle(t *testing.T) {
	for _, field := range []string{"", `,"created_at":null`, `,"created_at":1700000000`} {
		for _, bare := range []bool{false, true} {
			t.Run(fmt.Sprintf("field=%q/bare=%t", field, bare), func(t *testing.T) {
				response := `{"id":"resp_timestamp","object":"response","status":"in_progress","output":[]` + field + `}`
				body := "data: " + `{"type":"response.created","sequence_number":0,"response":` + response + "}\n\n" +
					"data: " + `{"type":"response.in_progress","sequence_number":1,"response":` + response + "}\n\n"
				if bare {
					body += "data: " + `{"type":"error","error":{"code":"content_policy","message":"blocked"}}` + "\n\n"
				} else {
					body += "data: " + `{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_timestamp","output_index":0,"content_index":0,"delta":"ok"}` + "\n\n"
					body += "data: " + `{"type":"response.completed","sequence_number":3,"response":` + strings.Replace(response, "in_progress", "completed", 1) + "}\n\n"
				}
				body += "data: [DONE]\n\n"
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
				account := &Account{ID: 8119, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
				_, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
				if bare {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				var timestamp int64
				count := 0
				for _, payload := range sseDataPayloads(recorder.Body.String()) {
					if !gjson.Get(payload, "response").IsObject() {
						continue
					}
					createdAt := gjson.Get(payload, "response.created_at")
					require.Equal(t, gjson.Number, createdAt.Type, payload)
					require.Positive(t, createdAt.Int(), payload)
					if timestamp == 0 {
						timestamp = createdAt.Int()
					}
					require.Equal(t, timestamp, createdAt.Int(), payload)
					if strings.Contains(field, "1700000000") {
						require.EqualValues(t, 1700000000, createdAt.Int())
					}
					count++
				}
				require.Equal(t, 3, count, recorder.Body.String())
			})
		}
	}
}
