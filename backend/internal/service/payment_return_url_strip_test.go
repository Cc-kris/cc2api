package service

// 本文件不带 unit 标签：同包 unit 标签下有基线编译失败的测试文件，放这里保证 V01-R03 用例能跑。

import (
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalizeReturnURLStripsSmuggledTradeStatus(t *testing.T) {
	t.Parallel()

	got, err := CanonicalizeReturnURL(
		"https://example.com/payment/result?trade_status=TRADE_SUCCESS",
		"example.com", "",
	)
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
	}
	if strings.Contains(got, "trade_status") {
		t.Fatalf("CanonicalizeReturnURL kept smuggled trade_status: %q", got)
	}
}

// 服务端签名参数在剥离客户端查询串之后仍然加上（V01-AC12）。
func TestCanonicalizeThenBuildReturnURLKeepsOnlyServerParams(t *testing.T) {
	t.Parallel()

	canonical, err := CanonicalizeReturnURL(
		"https://example.com/payment/result?trade_status=TRADE_SUCCESS&from=checkout",
		"example.com", "",
	)
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
	}
	got, err := buildPaymentReturnURL(canonical, 42, "sub2_42", "resume-token")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	query := parsed.Query()
	if query.Has("trade_status") || query.Has("from") {
		t.Fatalf("client query leaked into return_url: %q", got)
	}
	if query.Get("order_id") != "42" || query.Get("out_trade_no") != "sub2_42" ||
		query.Get("resume_token") != "resume-token" || query.Get("status") != "success" {
		t.Fatalf("server params missing from return_url: %q", got)
	}
}
