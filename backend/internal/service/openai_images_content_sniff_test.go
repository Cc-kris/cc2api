package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsAllowedCodexImageContent 锁定 V01-R12：Codex 生图下载只信字节嗅探结果，
// 且只接受 png/jpeg/webp/gif。响应头声称 image/png 也不影响判定（下载函数不读响应头）。
func TestIsAllowedCodexImageContent(t *testing.T) {
	webp := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 8)...)
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "png", data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), want: true},
		{name: "jpeg", data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, want: true},
		{name: "webp", data: webp, want: true},
		{name: "gif89a", data: []byte("GIF89a\x01\x00\x01\x00"), want: true},
		{name: "gif87a", data: []byte("GIF87a\x01\x00\x01\x00"), want: true},
		{name: "bmp not allowed", data: []byte("BM\x00\x00\x00\x00\x00\x00\x00\x00"), want: false},
		{name: "ico not allowed", data: []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}, want: false},
		{name: "svg text", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), want: false},
		{name: "svg with xml prolog", data: []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), want: false},
		{name: "html", data: []byte("<html><body>login required</body></html>"), want: false},
		{name: "plain text", data: []byte("not an image"), want: false},
		{name: "json", data: []byte(`{"error":"unauthorized"}`), want: false},
		{name: "empty", data: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isAllowedCodexImageContent(tt.data))
		})
	}
}

type codexImageDownloadRoundTripperStub struct {
	contentType string
	body        []byte
	calls       int
}

func (s *codexImageDownloadRoundTripperStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{s.contentType}},
		Body:       io.NopCloser(bytes.NewReader(s.body)),
		Request:    req,
	}, nil
}

// TestDownloadCodexImageResponseURL_TrustsOnlySniffedAllowedFormats 走真实的下载函数：
// 响应头声称 image/png，但字节是 HTML/SVG/纯文本时拒绝；png/jpeg/webp/gif 正常返回。
// 用公网 IP 字面量避免 DNS 解析，Transport 换成桩，不发真实网络请求。
func TestDownloadCodexImageResponseURL_TrustsOnlySniffedAllowedFormats(t *testing.T) {
	original := defaultCodexImageDownloadHTTPClient
	t.Cleanup(func() { defaultCodexImageDownloadHTTPClient = original })

	webp := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 8)...)
	const imageURL = "https://93.184.216.34/generated.png"
	svc := &OpenAIGatewayService{}

	for name, body := range map[string][]byte{
		"html": []byte("<!doctype html><html><body>login</body></html>"),
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"text": []byte("plain text pretending to be png"),
	} {
		t.Run("reject_"+name, func(t *testing.T) {
			stub := &codexImageDownloadRoundTripperStub{contentType: "image/png", body: body}
			defaultCodexImageDownloadHTTPClient = &http.Client{Transport: stub}
			got, err := svc.downloadCodexImageResponseURL(context.Background(), imageURL)
			require.Error(t, err)
			require.Contains(t, err.Error(), "disallowed content")
			require.Nil(t, got)
			require.Equal(t, 1, stub.calls)
		})
	}

	for name, body := range map[string][]byte{
		"png":  []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
		"jpeg": {0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00},
		"webp": webp,
		"gif":  []byte("GIF89a\x01\x00\x01\x00"),
	} {
		t.Run("accept_"+name, func(t *testing.T) {
			// 响应头故意写成非图片，证明判定只看字节。
			stub := &codexImageDownloadRoundTripperStub{contentType: "application/octet-stream", body: body}
			defaultCodexImageDownloadHTTPClient = &http.Client{Transport: stub}
			got, err := svc.downloadCodexImageResponseURL(context.Background(), imageURL)
			require.NoError(t, err)
			require.Equal(t, body, got)
		})
	}

	// SSRF 守卫仍在下载之前生效（AC40 回归）：私网地址不会走到 Transport。
	stub := &codexImageDownloadRoundTripperStub{contentType: "image/png", body: []byte("\x89PNG\r\n\x1a\n")}
	defaultCodexImageDownloadHTTPClient = &http.Client{Transport: stub}
	for _, rawURL := range []string{"https://127.0.0.1/a.png", "https://10.0.0.5/a.png", "https://169.254.169.254/latest", "http://93.184.216.34/a.png"} {
		_, err := svc.downloadCodexImageResponseURL(context.Background(), rawURL)
		require.Error(t, err, "url=%s", rawURL)
	}
	require.Zero(t, stub.calls)
}
