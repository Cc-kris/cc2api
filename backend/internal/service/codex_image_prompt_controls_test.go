package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyCodexImagePromptControls_ExtractsSupportedOutputConstraints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		body           string
		wantChanged    bool
		wantSize       string
		wantQuality    string
		wantBackground string
	}{
		{
			name:           "exact dimensions quality transparent png",
			body:           `{"model":"gpt-image-2","prompt":"生成 2000*2000 的高质量透明背景 PNG 图片","size":"auto","quality":"auto","background":"auto"}`,
			wantChanged:    true,
			wantSize:       "2000x2000",
			wantQuality:    "high",
			wantBackground: "transparent",
		},
		{
			name:        "aspect ratio",
			body:        `{"model":"gpt-image-2","prompt":"生成 16:9 横版海报","size":"auto"}`,
			wantChanged: true,
			wantSize:    "2048x1152",
		},
		{
			name:        "portrait orientation",
			body:        `{"model":"gpt-image-2","prompt":"制作一张竖版宣传图","size":"auto"}`,
			wantChanged: true,
			wantSize:    "1152x2048",
		},
		{
			name:        "ordinary prompt",
			body:        `{"model":"gpt-image-2","prompt":"画一幅 10:30 日落时的 landscape scene","size":"auto"}`,
			wantChanged: false,
			wantSize:    "auto",
		},
		{
			name:           "opaque background",
			body:           `{"model":"gpt-image-2","prompt":"生成一张不透明背景的 PNG 图片","background":"auto"}`,
			wantChanged:    true,
			wantBackground: "opaque",
		},
		{
			name:        "explicit request fields win",
			body:        `{"model":"gpt-image-2","prompt":"生成 2000x2000 的高质量图片","size":"1024x1024","quality":"low"}`,
			wantChanged: false,
			wantSize:    "1024x1024",
			wantQuality: "low",
		},
		{
			name:        "explicit png format wins over prompt",
			body:        `{"model":"gpt-image-2","prompt":"生成 JPG 图片","output_format":"png"}`,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			parsed, err := parseCodexImagePromptControlsTestRequest(body)
			require.NoError(t, err)

			rewritten, applied, err := ApplyCodexImagePromptControls(body, parsed)
			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, len(applied) > 0)
			require.Equal(t, tt.wantSize, parsed.Size)
			require.Equal(t, tt.wantQuality, parsed.Quality)
			require.Equal(t, tt.wantBackground, parsed.Background)
			require.Equal(t, tt.wantSize, gjson.GetBytes(rewritten, "size").String())
			if tt.wantQuality != "" {
				require.Equal(t, tt.wantQuality, gjson.GetBytes(rewritten, "quality").String())
			}
			if tt.wantBackground != "" {
				require.Equal(t, tt.wantBackground, gjson.GetBytes(rewritten, "background").String())
			}
		})
	}
}

func TestApplyCodexImagePromptControls_RejectsUnsupportedOrConflictingConstraints(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		body    string
		wantErr string
	}{
		{name: "jpeg", prompt: "输出 2000x2000 JPG", wantErr: "only saves generated artifacts as PNG"},
		{name: "webp", prompt: "输出 WebP 格式", wantErr: "only saves generated artifacts as PNG"},
		{name: "multiple outputs", prompt: "请生成 3 张不同版本", wantErr: "one image per imagegen call"},
		{name: "dpi", prompt: "生成 300 DPI 印刷图", wantErr: "DPI metadata"},
		{name: "color space", prompt: "输出 CMYK 文件", wantErr: "color-space conversion"},
		{name: "file size", prompt: "图片文件小于 1MB", wantErr: "target file size"},
		{name: "compression", prompt: "JPG 压缩质量 80%", wantErr: "only saves generated artifacts as PNG"},
		{name: "invalid dimensions", prompt: "生成 2001x2000 图片", wantErr: "multiples of 16"},
		{name: "conflicting dimensions", prompt: "生成 2000x2000 和 2048x1152 两个尺寸", wantErr: "conflicting explicit image dimensions"},
		{name: "conflicting ratio", prompt: "生成 2000x2000、16:9 的图片", wantErr: "conflict with aspect ratio"},
		{name: "explicit jpeg", prompt: "生成图片", body: `{"model":"gpt-image-2","prompt":"生成图片","size":"auto","output_format":"jpeg"}`, wantErr: "only saves generated artifacts as PNG"},
		{name: "explicit multiple outputs", prompt: "生成图片", body: `{"model":"gpt-image-2","prompt":"生成图片","size":"auto","n":2}`, wantErr: "one image per imagegen call"},
		{name: "explicit compression", prompt: "生成图片", body: `{"model":"gpt-image-2","prompt":"生成图片","size":"auto","output_compression":80}`, wantErr: "does not support requested output compression"},
		{name: "invalid explicit size", prompt: "生成图片", body: `{"model":"gpt-image-2","prompt":"生成图片","size":"2001x2000"}`, wantErr: "multiples of 16"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyText := tt.body
			if bodyText == "" {
				bodyText = `{"model":"gpt-image-2","prompt":` + strconv.Quote(tt.prompt) + `,"size":"auto"}`
			}
			body := []byte(bodyText)
			parsed, err := parseCodexImagePromptControlsTestRequest(body)
			require.NoError(t, err)
			_, _, err = ApplyCodexImagePromptControls(body, parsed)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestNormalizeCodexImageOutputBase64_CorrectsExactPixelDimensionsAndPNGEncoding(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1254, 1254))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: 30, G: 90, B: 180, A: 255})
		}
	}
	var sourcePNG bytes.Buffer
	require.NoError(t, png.Encode(&sourcePNG, source))

	normalized, actualSize, err := normalizeCodexImageOutputBase64(
		base64.StdEncoding.EncodeToString(sourcePNG.Bytes()),
		"2000x2000",
	)
	require.NoError(t, err)
	require.Equal(t, "2000x2000", actualSize)

	normalizedBytes, err := base64.StdEncoding.DecodeString(normalized)
	require.NoError(t, err)
	config, format, err := image.DecodeConfig(bytes.NewReader(normalizedBytes))
	require.NoError(t, err)
	require.Equal(t, "png", format)
	require.Equal(t, 2000, config.Width)
	require.Equal(t, 2000, config.Height)
}

func TestNormalizeCodexImageOutputBase64_RejectsUnsupportedEncoding(t *testing.T) {
	webpHeader := []byte("RIFF\x10\x00\x00\x00WEBPVP8 ")
	_, _, err := normalizeCodexImageOutputBase64(base64.StdEncoding.EncodeToString(webpHeader), "2000x2000")
	require.ErrorContains(t, err, "unsupported generated image encoding")
}

func parseCodexImagePromptControlsTestRequest(body []byte) (*OpenAIImagesRequest, error) {
	req := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, ContentType: "application/json", N: 1, Body: body}
	if err := parseOpenAIImagesJSONRequest(body, req); err != nil {
		return nil, err
	}
	applyOpenAIImagesDefaults(req)
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.RequiredCapability = classifyOpenAIImagesCapability(req)
	return req, nil
}
