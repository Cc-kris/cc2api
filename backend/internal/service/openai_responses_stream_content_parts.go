package service

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
)

// responsesStreamContentParts repairs vendor Responses streams which omit the
// content-part lifecycle. The OpenAI SDK cannot apply output_text.delta to an
// item whose content_part.added was never emitted. Existing upstream events are
// left byte-for-byte untouched; only missing output_text part events are added.
type responsesStreamContentParts struct {
	itemIDs map[int]string
	added   map[string]struct{}
	done    map[string]struct{}
	pending *responsesPendingContentPart
}

type responsesPendingContentPart struct {
	outputIndex  int
	contentIndex int
	itemID       string
	text         string
}

func newResponsesStreamContentParts() *responsesStreamContentParts {
	return &responsesStreamContentParts{
		itemIDs: make(map[int]string),
		added:   make(map[string]struct{}),
		done:    make(map[string]struct{}),
	}
}

// Before returns synthetic data payloads which must precede data. Item-done
// content is authoritative, so a wholly absent part is opened and closed before
// the item itself closes. A text delta receives only its missing opening event.
func (r *responsesStreamContentParts) Before(data []byte) [][]byte {
	if r == nil || !gjson.ValidBytes(data) {
		return nil
	}
	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	outputIndex := int(gjson.GetBytes(data, "output_index").Int())
	if eventType == "response.output_text.done" {
		// An upstream content_part.done may immediately follow. Defer the
		// synthetic closure until the next event proves it is absent.
		r.pending = &responsesPendingContentPart{
			outputIndex:  outputIndex,
			contentIndex: int(gjson.GetBytes(data, "content_index").Int()),
			itemID:       r.itemID(data, outputIndex, strings.TrimSpace(gjson.GetBytes(data, "item_id").String())),
			text:         gjson.GetBytes(data, "text").String(),
		}
		return nil
	}
	events := r.flushPending(eventType, outputIndex, int(gjson.GetBytes(data, "content_index").Int()))
	contentIndex := int(gjson.GetBytes(data, "content_index").Int())
	itemID := strings.TrimSpace(gjson.GetBytes(data, "item_id").String())

	switch eventType {
	case "response.output_item.added":
		if gjson.GetBytes(data, "item.type").String() == "message" {
			if id := strings.TrimSpace(gjson.GetBytes(data, "item.id").String()); id != "" {
				r.itemIDs[outputIndex] = id
			}
		}
	case "response.content_part.added":
		r.added[responsesContentPartKey(outputIndex, contentIndex)] = struct{}{}
	case "response.content_part.done":
		key := responsesContentPartKey(outputIndex, contentIndex)
		r.added[key] = struct{}{}
		if _, exists := r.done[key]; exists {
			return events
		}
		r.done[key] = struct{}{}
	case "response.output_text.delta":
		return append(events, r.addPart(outputIndex, contentIndex, r.itemID(data, outputIndex, itemID))...)
	case "response.output_item.done":
		if gjson.GetBytes(data, "item.type").String() != "message" {
			return events
		}
		itemID = strings.TrimSpace(gjson.GetBytes(data, "item.id").String())
		if itemID != "" {
			r.itemIDs[outputIndex] = itemID
		}
		for index, part := range gjson.GetBytes(data, "item.content").Array() {
			if part.Get("type").String() != "output_text" {
				continue
			}
			events = append(events, r.addPart(outputIndex, index, itemID)...)
			events = append(events, r.donePart(outputIndex, index, itemID, part.Get("text").String())...)
		}
		return events
	}
	return events
}

func (r *responsesStreamContentParts) flushPending(eventType string, outputIndex, contentIndex int) [][]byte {
	if r.pending == nil {
		return nil
	}
	pending := *r.pending
	r.pending = nil
	if eventType == "response.content_part.done" && pending.outputIndex == outputIndex && pending.contentIndex == contentIndex {
		// The real event is the sole closure; do not synthesize a duplicate.
		return nil
	}
	return r.donePart(pending.outputIndex, pending.contentIndex, pending.itemID, pending.text)
}

// After flushes a pending closure at an upstream [DONE] sentinel, which carries
// no output/content indexes of its own.
func (r *responsesStreamContentParts) After(data []byte) [][]byte {
	if r != nil && string(data) == "[DONE]" && r.pending != nil {
		return r.flushPending("", 0, 0)
	}
	return nil
}

func (r *responsesStreamContentParts) itemID(data []byte, outputIndex int, itemID string) string {
	if itemID == "" {
		itemID = r.itemIDs[outputIndex]
	}
	if itemID != "" {
		r.itemIDs[outputIndex] = itemID
	}
	return itemID
}

func responsesContentPartKey(outputIndex, contentIndex int) string {
	return strconv.Itoa(outputIndex) + ":" + strconv.Itoa(contentIndex)
}

func (r *responsesStreamContentParts) addPart(outputIndex, contentIndex int, itemID string) [][]byte {
	key := responsesContentPartKey(outputIndex, contentIndex)
	if _, exists := r.added[key]; exists {
		return nil
	}
	r.added[key] = struct{}{}
	return responsesContentPartEvents("response.content_part.added", outputIndex, contentIndex, itemID, "")
}

func (r *responsesStreamContentParts) donePart(outputIndex, contentIndex int, itemID, text string) [][]byte {
	key := responsesContentPartKey(outputIndex, contentIndex)
	if _, exists := r.done[key]; exists {
		return nil
	}
	r.done[key] = struct{}{}
	return responsesContentPartEvents("response.content_part.done", outputIndex, contentIndex, itemID, text)
}

func responsesContentPartEvents(eventType string, outputIndex, contentIndex int, itemID, text string) [][]byte {
	data, err := json.Marshal(newResponsesStreamContentPartsEvent(eventType, outputIndex, contentIndex, itemID, text))
	if err != nil {
		return nil
	}
	return [][]byte{data}
}

func newResponsesStreamContentPartsEvent(eventType string, outputIndex, contentIndex int, itemID, text string) apicompat.ResponsesStreamEvent {
	return apicompat.ResponsesStreamEvent{
		Type:         eventType,
		OutputIndex:  outputIndex,
		ContentIndex: contentIndex,
		ItemID:       itemID,
		Part:         &apicompat.ResponsesContentPart{Type: "output_text", Text: text},
	}
}

func writeResponsesDataPayloads(writer io.Writer, payloads [][]byte) bool {
	for _, data := range payloads {
		if _, err := io.WriteString(writer, "data: "); err != nil {
			return false
		}
		if _, err := writer.Write(data); err != nil {
			return false
		}
		if _, err := io.WriteString(writer, "\n\n"); err != nil {
			return false
		}
	}
	return true
}

func rewriteSynthesizedResponsesData(streamCont *StreamContinuation, payloads [][]byte, startsOutput bool) [][]byte {
	if streamCont == nil || len(payloads) == 0 {
		return payloads
	}
	kept := payloads[:0]
	for _, payload := range payloads {
		data, keep := streamCont.RewriteResponsesData(string(payload), startsOutput)
		if keep {
			kept = append(kept, []byte(data))
		}
	}
	return kept
}
