package service

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/tidwall/gjson"
)

func TestProbeBillingHeaderOutput(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":""},{"type":"text","text":"Keep this prompt."}],"messages":[{"role":"user","content":"这是一个用于验证中文指纹计算的测试消息，请保持一致。"}],"unknown":{"keep":true}}`)
	result := syncBillingHeaderVersion(body, "claude-cli/2.1.283")
	fmt.Printf("PROBE_B64: %s\n", base64.StdEncoding.EncodeToString([]byte(gjson.GetBytes(result, "system.0.text").String())))
}
