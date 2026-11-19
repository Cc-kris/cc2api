package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesStreamSequenceRewriteSyntheticAdvancesPastEmittedEvent(t *testing.T) {
	for _, origin := range []string{"0", "40"} {
		t.Run(origin, func(t *testing.T) {
			sequence := newResponsesStreamSequence()
			_, _, _ = sequence.Rewrite(nil, []byte(`{"type":"response.created","sequence_number":`+origin+`}`), nil)
			data := sequence.RewriteSynthetic([]byte(`{"type":"response.failed","sequence_number":0}`))
			require.Equal(t, gjson.Get(origin, "@this").Int()+1, gjson.GetBytes(data, "sequence_number").Int())
		})
	}
}

func TestResponsesStreamSequenceRewriteSyntheticWithoutPredecessorStartsAtZero(t *testing.T) {
	sequence := newResponsesStreamSequence()
	data := sequence.RewriteSynthetic([]byte(`{"type":"response.failed","sequence_number":0}`))
	require.EqualValues(t, 0, gjson.GetBytes(data, "sequence_number").Int())
}
