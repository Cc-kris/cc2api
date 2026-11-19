package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// responsesStreamSequence allocates positions for events synthesized into an
// upstream stream. Rewriting later upstream events when an insertion has shifted
// the sequence space keeps the client-visible sequence strictly increasing.
type responsesStreamSequence struct {
	last        int64
	initialized bool
	inserted    bool
}

func newResponsesStreamSequence() *responsesStreamSequence {
	return &responsesStreamSequence{last: -1}
}

// Rewrite numbers synthetic events in their output order. Original events keep
// their sequence where possible; after an insertion, later events are renumbered
// rather than sent with a lower sequence than a synthetic predecessor.
func (s *responsesStreamSequence) Rewrite(before [][]byte, data []byte, after [][]byte) ([][]byte, []byte, [][]byte) {
	if s == nil || !gjson.ValidBytes(data) {
		return before, data, after
	}
	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	if !strings.HasPrefix(eventType, "response.") {
		return before, data, after
	}
	sequence := gjson.GetBytes(data, "sequence_number")
	if sequence.Type != gjson.Number || sequence.Int() < 0 {
		return before, data, after
	}
	upstreamSequence := sequence.Int()
	if !s.initialized {
		// A preceding protocol restorer may have supplied a non-zero sequence
		// space. Observe that position before inserting missing lifecycle events.
		s.last = upstreamSequence - 1
		s.initialized = true
	}
	if len(before)+len(after) > 0 {
		s.inserted = true
	}
	for index, event := range before {
		before[index] = s.stamp(event)
	}
	if s.inserted || upstreamSequence > s.last {
		s.last++
		if s.last != upstreamSequence {
			if rewritten, err := sjson.SetBytes(data, "sequence_number", s.last); err == nil {
				data = rewritten
			}
		}
	} else {
		// Without an insertion, preserve unrelated upstream numbering behavior.
		s.last = upstreamSequence
	}
	for index, event := range after {
		after[index] = s.stamp(event)
	}
	return before, data, after
}

// RewriteSynthetic allocates the next client-visible sequence for an event
// synthesized in place of a suppressed upstream event. Unlike Rewrite, it must
// advance even when the source event's fallback sequence duplicates the last
// emitted number (for example, a bare error without sequence_number).
func (s *responsesStreamSequence) RewriteSynthetic(data []byte) []byte {
	if s == nil {
		return data
	}
	s.inserted = true
	_, rewritten, _ := s.Rewrite(nil, data, nil)
	return rewritten
}

func (s *responsesStreamSequence) stamp(data []byte) []byte {
	s.last++
	if rewritten, err := sjson.SetBytes(data, "sequence_number", s.last); err == nil {
		return rewritten
	}
	return data
}
