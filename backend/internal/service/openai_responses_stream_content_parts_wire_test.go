package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesStreamContentPartsSynthesizedEventsSerializeWithZeroIndexes(t *testing.T) {
	for _, eventType := range []string{"response.content_part.added", "response.content_part.done"} {
		data, err := json.Marshal(newResponsesStreamContentPartsEvent(eventType, 0, 0, "msg_1", "text"))
		require.NoError(t, err)
		require.Contains(t, string(data), `"output_index":0`)
		require.Contains(t, string(data), `"content_index":0`)
		require.Equal(t, eventType, gjson.GetBytes(data, "type").String())
	}
}

func TestResponsesStreamSequencePreservesRestoredNonzeroOrigin(t *testing.T) {
	sequence := newResponsesStreamSequence()
	before := [][]byte{[]byte(`{"type":"response.content_part.added","sequence_number":0}`)}
	original := []byte(`{"type":"response.output_text.delta","sequence_number":40}`)
	before, original, after := sequence.Rewrite(before, original, nil)
	require.EqualValues(t, 40, gjson.GetBytes(before[0], "sequence_number").Int())
	require.EqualValues(t, 41, gjson.GetBytes(original, "sequence_number").Int())
	require.Empty(t, after)
	_, original, _ = sequence.Rewrite(nil, []byte(`{"type":"response.output_text.done","sequence_number":41}`), nil)
	require.EqualValues(t, 42, gjson.GetBytes(original, "sequence_number").Int())
}

func TestWriteResponsesDataPayloadsUsesSSEFormFrames(t *testing.T) {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	ok := writeResponsesDataPayloads(writer, [][]byte{[]byte(`{"type":"response.content_part.added"}`)})
	require.True(t, ok)
	require.NoError(t, writer.Flush())
	require.Equal(t, "data: {\"type\":\"response.content_part.added\"}\n\n", buf.String())
}
