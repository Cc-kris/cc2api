package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesStreamContentPartsSynthOnlyMissingLifecycle(t *testing.T) {
	normalizer := newResponsesStreamContentParts()
	itemAdded := []byte(`{"type":"response.output_item.added","output_index":1,"item":{"id":"msg_1","type":"message","content":[]}}`)
	delta := []byte(`{"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"hello"}`)

	require.Empty(t, normalizer.Before(itemAdded))
	before := normalizer.Before(delta)
	require.Len(t, before, 1)
	require.Equal(t, "response.content_part.added", gjson.GetBytes(before[0], "type").String())
	require.Equal(t, "msg_1", gjson.GetBytes(before[0], "item_id").String(), "recover the id from the preceding item")
	require.EqualValues(t, 1, gjson.GetBytes(before[0], "output_index").Int())
	require.Equal(t, "", gjson.GetBytes(before[0], "part.text").String())
	require.True(t, gjson.GetBytes(before[0], "part.annotations").IsArray())
	require.True(t, gjson.GetBytes(before[0], "part.logprobs").IsArray())
	require.Empty(t, normalizer.Before(delta), "multiple deltas share one opening event")

	textDone := []byte(`{"type":"response.output_text.done","output_index":1,"content_index":0,"item_id":"msg_1","text":"hello"}`)
	require.Empty(t, normalizer.Before(textDone), "wait for an optional upstream part.done")
	require.Empty(t, normalizer.After(textDone))
	next := normalizer.Before([]byte(`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"hello"}]}}`))
	require.Len(t, next, 1)
	require.Equal(t, "response.content_part.done", gjson.GetBytes(next[0], "type").String())
	require.Equal(t, "hello", gjson.GetBytes(next[0], "part.text").String())
	require.Empty(t, normalizer.Before(textDone))
}

func TestResponsesStreamContentPartsDefersDoneUntilFollowingEvent(t *testing.T) {
	normalizer := newResponsesStreamContentParts()
	textDone := []byte(`{"type":"response.output_text.done","output_index":1,"content_index":0,"item_id":"msg_1","text":"hello"}`)
	require.Empty(t, normalizer.Before(textDone))
	require.Empty(t, normalizer.After(textDone))
	partDone := []byte(`{"type":"response.content_part.done","output_index":1,"content_index":0,"item_id":"msg_1","part":{"type":"output_text","text":"hello"}}`)
	require.Empty(t, normalizer.Before(partDone), "the matching real part.done is the only closure")
	require.Empty(t, normalizer.After(partDone))
	itemDone := []byte(`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"hello"}]}}`)
	require.Empty(t, normalizer.Before(itemDone), "item.done must not duplicate an existing part.done")
}

func TestResponsesStreamContentPartsFlushesPendingDoneAtDoneSentinel(t *testing.T) {
	normalizer := newResponsesStreamContentParts()
	require.Empty(t, normalizer.Before([]byte(`{"type":"response.output_text.done","output_index":1,"content_index":0,"item_id":"msg_1","text":"hello"}`)))
	events := normalizer.After([]byte("[DONE]"))
	require.Len(t, events, 1)
	require.Equal(t, "response.content_part.done", gjson.GetBytes(events[0], "type").String())
	require.Empty(t, normalizer.After([]byte("[DONE]")))
}

func TestResponsesStreamContentPartsRebuildsPartFromCompletedItem(t *testing.T) {
	normalizer := newResponsesStreamContentParts()
	done := []byte(`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_w4","type":"message","content":[{"type":"output_text","text":"W4 streamed answer","annotations":[],"logprobs":[]}]}}`)

	events := normalizer.Before(done)
	require.Len(t, events, 2, "the AC13 item-only fixture needs added and done")
	require.Equal(t, "response.content_part.added", gjson.GetBytes(events[0], "type").String())
	require.Equal(t, "msg_w4", gjson.GetBytes(events[0], "item_id").String())
	require.Equal(t, "response.content_part.done", gjson.GetBytes(events[1], "type").String())
	require.Equal(t, "W4 streamed answer", gjson.GetBytes(events[1], "part.text").String())
	require.True(t, gjson.GetBytes(events[1], "part.annotations").IsArray())
	require.True(t, gjson.GetBytes(events[1], "part.logprobs").IsArray())
	require.Empty(t, normalizer.Before(done), "must not duplicate a repaired lifecycle")
}

func TestResponsesStreamContentPartsPreservesUpstreamLifecycle(t *testing.T) {
	normalizer := newResponsesStreamContentParts()
	added := []byte(`{"type":"response.content_part.added","output_index":0,"content_index":0,"item_id":"msg_1","part":{"type":"output_text","text":""}}`)
	done := []byte(`{"type":"response.content_part.done","output_index":0,"content_index":0,"item_id":"msg_1","part":{"type":"output_text","text":"complete"}}`)

	require.Empty(t, normalizer.Before(added))
	require.Empty(t, normalizer.Before([]byte(`{"type":"response.output_text.delta","output_index":0,"content_index":0,"item_id":"msg_1","delta":"complete"}`)))
	require.Empty(t, normalizer.Before(done))
	require.Empty(t, normalizer.After([]byte(`{"type":"response.output_text.done","output_index":0,"content_index":0,"item_id":"msg_1","text":"complete"}`)))
	require.Empty(t, normalizer.Before([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"complete"}]}}`)))
}
