package service

// 移植自上游 de27905e8 service/upstream_request_id.go。
//
// 与上游的差异：上游账户未配置头名时不记录；本地按技术方案 3.11、第 5 节与 AC31，
// 未配置时默认读 x-request-id，配置了则只读配置的头。

import (
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/net/http/httpguts"
)

// AccountExtraUpstreamRequestIDHeader 是账户 extra 中的键，值为直接上游声明请求 ID 的响应头名。
const AccountExtraUpstreamRequestIDHeader = "upstream_request_id_header"

// DefaultUpstreamRequestIDHeader 是账户未配置头名时读取的响应头。
const DefaultUpstreamRequestIDHeader = "x-request-id"

const (
	maxUpstreamRequestIDHeaderNameLen = 64
	// maxUsageUpstreamRequestIDLen 与 usage_logs.upstream_request_id VARCHAR(128) 对齐。
	maxUsageUpstreamRequestIDLen = 128
)

// UpstreamRequestIDHeaderName 返回应读取的上游请求 ID 响应头名：账户配置优先，否则为默认 x-request-id。
func UpstreamRequestIDHeaderName(account *Account) string {
	if account != nil {
		if name := strings.TrimSpace(account.GetExtraString(AccountExtraUpstreamRequestIDHeader)); name != "" {
			return name
		}
	}
	return DefaultUpstreamRequestIDHeader
}

// UpstreamRequestIDFromHeaders 从直接上游的响应头解析请求 ID，只读 UpstreamRequestIDHeaderName 指定的那一个头。
func UpstreamRequestIDFromHeaders(account *Account, h http.Header) string {
	if len(h) == 0 {
		return ""
	}
	return strings.TrimSpace(h.Get(UpstreamRequestIDHeaderName(account)))
}

// usageUpstreamRequestIDPtr 生成落库值。WS 轮次没有 HTTP 响应头，保持 nil；
// 含控制字符时丢弃；超长截断到列宽而不是让整条用量行失败。
func usageUpstreamRequestIDPtr(account *Account, h http.Header, wsMode bool) *string {
	if wsMode {
		return nil
	}
	id := UpstreamRequestIDFromHeaders(account, h)
	if id == "" {
		return nil
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return nil
		}
	}
	if len(id) > maxUsageUpstreamRequestIDLen {
		id = id[:maxUsageUpstreamRequestIDLen]
		for len(id) > 0 && !utf8ValidTail(id) {
			id = id[:len(id)-1]
		}
	}
	return &id
}

// utf8ValidTail 判断按字节截断后的字符串结尾是否仍是完整的 UTF-8 字符。
func utf8ValidTail(s string) bool {
	return strings.ToValidUTF8(s, "�") == s
}

// ValidateUpstreamRequestIDHeaderExtra 校验并规范化 extra 中的上游请求 ID 头名：
// 必须是合法的 HTTP 头字段名且不超过 64 字节；空白值视为未配置并从 extra 中删除。
func ValidateUpstreamRequestIDHeaderExtra(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, ok := extra[AccountExtraUpstreamRequestIDHeader]
	if !ok || raw == nil {
		return nil
	}
	name, ok := raw.(string)
	if !ok {
		return infraerrors.BadRequest("INVALID_UPSTREAM_REQUEST_ID_HEADER",
			"upstream_request_id_header must be a string")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		delete(extra, AccountExtraUpstreamRequestIDHeader)
		return nil
	}
	if len(name) > maxUpstreamRequestIDHeaderNameLen || !httpguts.ValidHeaderFieldName(name) {
		return infraerrors.BadRequest("INVALID_UPSTREAM_REQUEST_ID_HEADER",
			"upstream_request_id_header must be a valid HTTP header name of at most 64 bytes")
	}
	extra[AccountExtraUpstreamRequestIDHeader] = name
	return nil
}
