package service

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// responsesStreamOutputItems remembers each item actually reported done so a
// terminal response can preserve item identity, ordering and vendor fields.
type responsesStreamOutputItems struct {
	items map[int]json.RawMessage
}

func newResponsesStreamOutputItems() *responsesStreamOutputItems {
	return &responsesStreamOutputItems{items: make(map[int]json.RawMessage)}
}

func (r *responsesStreamOutputItems) Observe(data []byte) {
	if r == nil || len(data) == 0 || !gjson.ValidBytes(data) {
		return
	}
	if strings.TrimSpace(gjson.GetBytes(data, "type").String()) != "response.output_item.done" {
		return
	}
	item := gjson.GetBytes(data, "item")
	if !item.Exists() || !item.IsObject() {
		return
	}
	index := int(gjson.GetBytes(data, "output_index").Int())
	r.items[index] = json.RawMessage(append([]byte(nil), item.Raw...))
}

func (r *responsesStreamOutputItems) HasItems() bool {
	return r != nil && len(r.items) > 0
}

func (r *responsesStreamOutputItems) Count() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

func (r *responsesStreamOutputItems) BuildOutput() ([]byte, bool) {
	if !r.HasItems() {
		return nil, false
	}
	indexes := make([]int, 0, len(r.items))
	for index := range r.items {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	ordered := make([]json.RawMessage, 0, len(indexes))
	for _, index := range indexes {
		ordered = append(ordered, r.items[index])
	}
	encoded, err := json.Marshal(ordered)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func normalizeResponsesStreamingTerminalOutput(data []byte, acc *apicompat.BufferedResponseAccumulator, doneItems *responsesStreamOutputItems, imageOutputs []json.RawMessage) ([]byte, bool) {
	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled":
	default:
		return data, false
	}

	output := gjson.GetBytes(data, "response.output")
	hasAccumulatedOutput := (acc != nil && acc.HasContent()) || len(imageOutputs) > 0 || doneItems.HasItems()
	if output.Exists() && output.IsArray() {
		terminalCount := len(output.Array())
		if terminalCount > 0 && terminalCount >= doneItems.Count() {
			return data, false
		}
		if terminalCount == 0 && !hasAccumulatedOutput {
			return data, false
		}
	}

	outputJSON := []byte("[]")
	if reconstructed, ok := doneItems.BuildOutput(); ok {
		outputJSON = reconstructed
	} else if reconstructed, ok := buildResponsesOutputJSON(acc, imageOutputs); ok {
		outputJSON = reconstructed
	}
	updated, err := sjson.SetRawBytes(data, "response.output", outputJSON)
	if err != nil {
		return data, false
	}
	return updated, true
}
