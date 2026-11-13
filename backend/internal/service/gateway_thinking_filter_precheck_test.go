package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

// The precheck may only skip the decoding path when that path would return the
// body unchanged; every case is checked against filterThinkingBlocksDecoded.
func TestThinkingBlocksNeedFilteringMatchesDecodingPath(t *testing.T) {
	dummy := antigravity.DummyThoughtSignature
	bodies := []string{
		`{"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hi"}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"},{"type":"text","text":"a"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"` + dummy + `"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":7}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"user","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"thinking":{"type":"disabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"d"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":""}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"thinking":"untyped"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":7,"thinking":null}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"text","text":"\"thinking\": quoted"}]}]}`,
		// Duplicate keys: encoding/json keeps the last value.
		`{"thinking":{"type":"enabled"},"thinking":{"budget_tokens":1},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"thinking":{"type":"disabled","type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"user","role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"sig","signature":""}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"sig"}],"content":"plain"}]}`,
		`{"thinking":{"type":"enabled"},"messages":[],"messages":[{"role":"assistant","content":[{"type":"thinking","signature":""}]}]}`,
		// Escaped keys and values.
		`{"thinking":{"\u0074ype":"enabled"},"messages":[{"role":"assistant","content":[{"\u0074ype":"thinking","thinking":"t","signature":"sig"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","\u0073ignature":""}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"\u0061ssistant","content":[{"type":"\u0074hinking","thinking":"t","signature":"\u0073ig"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"\u0000"}]}]}`,
		// Shapes the decoder rejects or ignores.
		`{"thinking":{"type":"enabled"},"messages":"none"}`,
		`{"thinking":{"type":"enabled"},"messages":[null,1,{"role":"assistant","content":null},{"role":"assistant","content":[null,"x"]}]}`,
		`[{"thinking":{}}]`,
		`{"thinking":{"type":"enabled"},"messages":[`,
		`{"thinking":{"type":"enabled"}} {}`,
	}
	for _, body := range bodies {
		decoded := filterThinkingBlocksDecoded([]byte(body))
		changed := !bytes.Equal(decoded, []byte(body))
		needs, decided := thinkingBlocksNeedFiltering([]byte(body))
		if decided && json.Valid([]byte(body)) {
			require.Equal(t, changed, needs, "body=%s", body)
		}
		// 本地 filterThinkingBlocksInternal 的第二个参数未被使用，两个取值都要一致。
		for _, alwaysThinking := range []bool{false, true} {
			require.Equal(t, string(decoded), string(filterThinkingBlocksInternal([]byte(body), alwaysThinking)), "alwaysThinking=%v body=%s", alwaysThinking, body)
		}
	}
}

// 畸形请求不得因为预检而被放行或改写：预检读取失败时必须与原解码路径输出完全一致。
func TestFilterThinkingBlocksMalformedBodiesMatchDecodingPath(t *testing.T) {
	bodies := []string{
		``,
		`null`,
		`"thinking"`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""}]}]`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking",,"signature":""}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"` + "\xff" + `"}]}]}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","signature":""}]}]}}`,
		`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"sig"}]}]`,
	}
	for _, body := range bodies {
		in := []byte(body)
		want := filterThinkingBlocksDecoded(append([]byte(nil), in...))
		got := filterThinkingBlocksInternal(in, false)
		require.Equal(t, string(want), string(got), "body=%q", body)
		if !json.Valid(in) {
			require.Equal(t, body, string(got), "无效 JSON 必须原样返回，不得被预检改写：%q", body)
		}
	}
}

// 需要过滤的请求必须产出与原路径逐字节相同的结果。
func TestFilterThinkingBlocksFilteringOutputUnchangedByPrecheck(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""},{"type":"text","text":"a"}]}]}`)
	got := filterThinkingBlocksInternal(body, false)
	require.NotEqual(t, string(body), string(got))
	require.Equal(t, string(filterThinkingBlocksDecoded(body)), string(got))
	require.NotContains(t, string(got), `"type":"thinking"`)
}

func signedThinkingHistoryBody() []byte {
	var b strings.Builder
	_, _ = b.WriteString(`{"model":"claude-sonnet-4-5","thinking":{"type":"adaptive"},"messages":[`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		_, _ = b.WriteString(`{"role":"user","content":[{"type":"text","text":"` + strings.Repeat("q", 2048) + `"}]},`)
		_, _ = b.WriteString(`{"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig-valid"},{"type":"text","text":"answer"}]}`)
	}
	_, _ = b.WriteString(`]}`)
	return []byte(b.String())
}

func TestFilterThinkingBlocksSignedHistorySkipsDecode(t *testing.T) {
	body := signedThinkingHistoryBody()
	allocs := testing.AllocsPerRun(5, func() {
		got := filterThinkingBlocksInternal(body, false)
		if &got[0] != &body[0] {
			t.Fatal("valid signed history must be returned unchanged")
		}
	})
	require.Less(t, allocs, float64(16), "allocs: %v", allocs)
}

func FuzzThinkingBlocksNeedFiltering(f *testing.F) {
	f.Add([]byte(`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig"}]}]}`), false)
	f.Add([]byte(`{"thinking":{"type":"adaptive"},"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"d"},{"thinking":1}]}]}`), true)
	f.Add([]byte(`{"messages":[{"role":"user","role":"assistant","content":[{"type":"thinking","signature":"","signature":"s"}]}]}`), true)
	f.Fuzz(func(t *testing.T, body []byte, alwaysThinking bool) {
		original := append([]byte(nil), body...)
		decoded := filterThinkingBlocksDecoded(original)
		needs, decided := thinkingBlocksNeedFiltering(body)
		if !bytes.Equal(filterThinkingBlocksInternal(body, alwaysThinking), decoded) {
			t.Fatalf("filter output differs from decoding path for %q", body)
		}
		if decided && json.Valid(original) && needs == bytes.Equal(decoded, original) {
			t.Fatalf("precheck needs=%v disagrees with decoding path for %q", needs, body)
		}
	})
}

// Decoded is the previous cost of a request whose signed history needs no
// filtering; Precheck is the current path for the same body.
func BenchmarkFilterThinkingBlocksSignedHistory(b *testing.B) {
	body := signedThinkingHistoryBody()
	b.Run("Decoded", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for i := 0; i < b.N; i++ {
			_ = filterThinkingBlocksDecoded(body)
		}
	})
	b.Run("Precheck", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for i := 0; i < b.N; i++ {
			_ = filterThinkingBlocksInternal(body, false)
		}
	})
}
