package service

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// A replayed conversation may contain many items with a status rejected by the
// upstream schema. One rejection must clear the whole item type: clearing one
// index per round trip exhausts the local one-retry compatibility budget.
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyClearsStatusForWholeType(t *testing.T) {
	input := make([]string, 0, 12)
	for i := 0; i < 10; i++ {
		input = append(input, `{"type":"tool_search_output","status":"completed","call_id":"call_`+strconv.Itoa(i)+`","tools":[]}`)
	}
	input = append(input, `{"type":"message","role":"user","status":"completed","content":"hi"}`)
	body := []byte(`{"input":[` + strings.Join(input, ",") + `]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[7].status'.","param":"input[7].status"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEmpty(t, reason)
	for i := 0; i < 10; i++ {
		require.False(t, gjson.GetBytes(retryBody, "input."+strconv.Itoa(i)+".status").Exists(), "index %d", i)
		require.Equal(t, "call_"+strconv.Itoa(i), gjson.GetBytes(retryBody, "input."+strconv.Itoa(i)+".call_id").String())
	}
	require.Equal(t, "completed", gjson.GetBytes(retryBody, "input.10.status").String(), "a different item type keeps status")
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyClearsUntypedStatusAtIndexOnly(t *testing.T) {
	body := []byte(`{"input":[{"status":"keep_a"},{"status":"remove"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].status'.","param":"input[1].status"}}`)
	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep_a", gjson.GetBytes(retryBody, "input.0.status").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.status").Exists())
}
