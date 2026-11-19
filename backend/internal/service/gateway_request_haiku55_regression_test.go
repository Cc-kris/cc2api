package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHaiku55RequestRegressionMatrix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		match string
	}{
		{"default", `{"model":"claude-haiku-5-5","messages":[]}`, ""},
		{"adaptive medium", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`, ""},
		{"adaptive max", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"max"}}`, ""},
		{"disabled high", `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, ""},
		{"disabled xhigh", `{"thinking":{"type":"disabled"},"output_config":{"effort":"xhigh"}}`, "disabled"},
		{"disabled max", `{"thinking":{"type":"disabled"},"output_config":{"effort":"max"}}`, "disabled"},
		{"between tools", `{"thinking":{"type":"between_tools"},"output_config":{"effort":"high"}}`, "between_tools"},
		{"manual thinking", `{"thinking":{"type":"enabled"},"output_config":{"effort":"high"}}`, "adaptive"},
		{"minimal effort", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"minimal"}}`, "effort"},
		{"temperature", `{"temperature":0.2}`, "temperature"},
		{"top_p", `{"top_p":0.3}`, "top_p"},
		{"both", `{"temperature":1,"top_p":0.99}`, "together"},
		{"default temperature", `{"temperature":1}`, ""},
		{"default top_p", `{"top_p":0.99}`, ""},
		{"forced tool choice", `{"tool_choice":{"type":"tool","name":"lookup"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHaiku55Request([]byte(tc.body), "claude-haiku-5-5")
			if tc.match == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.match)
			}
		})
	}
	require.NoError(t, ValidateHaiku55Request([]byte(`{"thinking":{"type":"disabled"},"output_config":{"effort":"max"},"temperature":0.2}`), "claude-sonnet-4-5"))
}
