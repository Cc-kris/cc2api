package service

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func originalStripNamespaces(body []byte, keep bool) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"namespace"`)) {
		return body, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, nil
	}
	var rebuilt bytes.Buffer
	changed := false
	copyFrom := 0
	var stripErr error
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() || !item.Get("namespace").Exists() || (keep && isOpenAIResponsesToolCallItemType(item.Get("type").String())) {
			return true
		}
		itemBody, err := sjson.DeleteBytes([]byte(item.Raw), "namespace")
		if err != nil {
			stripErr = err
			return false
		}
		if !changed {
			rebuilt.Grow(len(input.Raw))
			changed = true
		}
		itemStart := item.Index - input.Index
		_, _ = rebuilt.WriteString(input.Raw[copyFrom:itemStart])
		_, _ = rebuilt.Write(itemBody)
		copyFrom = itemStart + len(item.Raw)
		return true
	})
	if stripErr != nil {
		return body, fmt.Errorf("delete OpenAI input namespace: %w", stripErr)
	}
	if !changed {
		return body, nil
	}
	_, _ = rebuilt.WriteString(input.Raw[copyFrom:])
	stripped, err := sjson.SetRawBytes(body, "input", rebuilt.Bytes())
	if err != nil {
		return body, fmt.Errorf("replace OpenAI input after namespace deletion: %w", err)
	}
	return stripped, nil
}

func TestStripOpenAIResponsesInputNamespaces_QueryGuardMatchesOriginal(t *testing.T) {
	bs := string([]byte{92})
	cases := []string{
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":null}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":false}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":""}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":0}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":1}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":{}}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"function_call","namespace":"ns"}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","content":{"namespace":"nested"}}]}`,
		`{"tools":[{"type":"namespace"}],"input":[[{"namespace":"nested"}]]}`,
		`{"tools":[{"type":"namespace"}],"in` + bs + `u0070ut":[{"type":"message","namespace":"escaped input"}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","` + bs + `u006eamespace":"escaped namespace"}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message"}],"input":[{"namespace":"second"}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"namespace":null,"namespace":"second"}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"namespace":"first","namespace":null}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","content":[{"text":"namespace } ]"}]}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message"}]`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message"]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"type":"message","namespace":"bad"}] trailing`,
	}
	for i, body := range cases {
		for _, keep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d_keep_%t", i, keep), func(t *testing.T) {
				want, werr := originalStripNamespaces([]byte(body), keep)
				got, gerr := stripOpenAIResponsesInputNamespaces([]byte(body), keep)
				require.Equal(t, werr, gerr)
				require.Equal(t, string(want), string(got))
			})
		}
	}
}
