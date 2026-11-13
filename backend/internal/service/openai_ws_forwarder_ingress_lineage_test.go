package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_InvalidEncryptedContentLineageStripsNextTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	upstreamConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"error","error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be verified"}}`),
			[]byte(`{"type":"response.failed","response":{"id":"resp_enc_lineage_1","model":"gpt-5.1","error":{"code":"invalid_encrypted_content","message":"The encrypted content could not be verified"}}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_enc_lineage_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	dialer := &openAIWSQueueDialer{
		conns: []openAIWSClientConn{upstreamConn},
	}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	account := &Account{
		ID:          119,
		Name:        "openai-ingress-enc-lineage",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{
			CompressionMode: coderws.CompressionContextTakeover,
		})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() {
			_ = conn.CloseNow()
		}()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "unit-test-agent/1.0")
		ginCtx.Request = req

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErrCh <- errors.New("unsupported websocket client message type")
			return
		}

		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeMessage := func(payload string) {
		writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
	}
	readMessage := func() []byte {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		msgType, message, readErr := clientConn.Read(readCtx)
		require.NoError(t, readErr)
		require.Equal(t, coderws.MessageText, msgType)
		return message
	}

	// turn1：携带失效密文，上游以 invalid_encrypted_content 拒绝（error + response.failed 透传给客户端）。
	writeMessage(`{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"type":"reasoning","id":"rs_1","encrypted_content":"stale-cipher","summary":[]},{"type":"input_text","text":"hi"}]}`)
	firstEvent := readMessage()
	require.Equal(t, "error", gjson.GetBytes(firstEvent, "type").String())
	require.Equal(t, "invalid_encrypted_content", gjson.GetBytes(firstEvent, "error.code").String())
	secondEvent := readMessage()
	require.Equal(t, "response.failed", gjson.GetBytes(secondEvent, "type").String())

	// turn2：客户端历史仍带同一失效密文，进场应被 lineage 预剥离后再发上游。
	writeMessage(`{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"type":"reasoning","id":"rs_1","encrypted_content":"stale-cipher","summary":[]},{"type":"input_text","text":"hi"},{"type":"input_text","text":"again"}]}`)
	thirdEvent := readMessage()
	require.Equal(t, "response.completed", gjson.GetBytes(thirdEvent, "type").String())
	require.Equal(t, "resp_enc_lineage_2", gjson.GetBytes(thirdEvent, "response.id").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
	}

	upstreamConn.mu.Lock()
	writes := append([]map[string]any(nil), upstreamConn.writes...)
	upstreamConn.mu.Unlock()
	require.Len(t, writes, 2, "两轮各应发送一次上游请求")

	firstUpstream := requestToJSONString(writes[0])
	require.Equal(t, "stale-cipher", gjson.Get(firstUpstream, "input.0.encrypted_content").String(), "首轮请求原样携带密文")

	secondUpstream := requestToJSONString(writes[1])
	secondInput := gjson.Get(secondUpstream, "input").Array()
	require.Len(t, secondInput, 3, "剥离仅移除 encrypted_content 字段，reasoning 骨架保留")
	for _, item := range secondInput {
		require.False(t, item.Get("encrypted_content").Exists(), "第二轮请求不得再携带已失效密文: %s", item.Raw)
	}
	require.Equal(t, "rs_1", gjson.Get(secondUpstream, "input.0.id").String())
	require.Equal(t, "again", gjson.Get(secondUpstream, "input.2.text").String())
}
