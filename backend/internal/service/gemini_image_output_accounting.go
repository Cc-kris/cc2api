package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// V03-T15 / R09：Gemini 原生生图按上游实际回吐的图片张数计费（移植上游 b6eb6c1ef，叠加 917dbdc70
// 的 fileData 计数）。

// geminiImageOutputCounterKey 是请求级图片计数器挂在 gin.Context 上的键。
const geminiImageOutputCounterKey = "gemini_image_output_counter"

// geminiImageOutputCounter 记录一次转发里 Gemini 上游真正回吐的图片数量。
//
// 取「单个 payload 内的最大值」而不是累加：Gemini 兼容上游的 SSE 分片可能是累积式的
// （同一份内容在后续 chunk 里重复整段回来），逐 chunk 累加会把同一张图算很多次。计费上宁可
// 少算不可多算，所以用 max 兜底：
//   - 非流式：整份响应体只观测一次，max 即真实张数；
//   - 累积式流：最后一个 chunk 含全部图片，max 仍是真实张数；
//   - 增量式流且多图分散在不同 chunk：会低估到 1，与改动前的模型名启发式同值，不构成回退。
//
// delivered 只在 payload 成功写给客户端后更新，供客户端中途断开时「按已送达图片计费」使用
// （上游 2b1f12938 的断开路径用 observedGeminiImageOutputs；CCAI 断开后会继续读完上游，
// 观测值可能包含没送达的图片，所以单独记）。
type geminiImageOutputCounter struct {
	count     int
	delivered int
}

// beginGeminiImageOutputObservation 在每次 Forward 开头重置计数器。
// failover 会拿同一个 gin.Context 重跑转发，不重置就会把上一个账号的图数带进来。
func beginGeminiImageOutputObservation(c *gin.Context) *geminiImageOutputCounter {
	if c == nil {
		return nil
	}
	counter := &geminiImageOutputCounter{}
	c.Set(geminiImageOutputCounterKey, counter)
	return counter
}

func geminiImageOutputCounterFromContext(c *gin.Context) *geminiImageOutputCounter {
	if c == nil {
		return nil
	}
	value, ok := c.Get(geminiImageOutputCounterKey)
	if !ok {
		return nil
	}
	counter, _ := value.(*geminiImageOutputCounter)
	return counter
}

// observeGeminiImageOutputs 观测一段上游响应（整份或单个 chunk）里的图片，返回该段的图片数。
func observeGeminiImageOutputs(c *gin.Context, payload []byte) int {
	count := countGeminiImageOutputs(payload)
	counter := geminiImageOutputCounterFromContext(c)
	if counter != nil && count > counter.count {
		counter.count = count
	}
	return count
}

// markGeminiImageOutputsDelivered 记录已成功写给客户端的 payload 中的图片数（取 max，口径同上）。
func markGeminiImageOutputsDelivered(c *gin.Context, count int) {
	counter := geminiImageOutputCounterFromContext(c)
	if counter != nil && count > counter.delivered {
		counter.delivered = count
	}
}

func observedGeminiImageOutputs(c *gin.Context) int {
	counter := geminiImageOutputCounterFromContext(c)
	if counter == nil {
		return 0
	}
	return counter.count
}

// deliveredGeminiImageOutputs 返回已送达客户端的图片数。
func deliveredGeminiImageOutputs(c *gin.Context) int {
	counter := geminiImageOutputCounterFromContext(c)
	if counter == nil {
		return 0
	}
	return counter.delivered
}

// resolveGeminiImageCount 决定本次请求按几张图计费。
//
// 优先用上游真正返回的图片数：走 GeminiMessagesCompatService 的账号多是 API Key + 自定义
// 模型映射，客户端请求名和上游模型名都可能是自取的别名，isImageGenerationModel 的白名单判不出，
// 于是 ImageCount=0，按次计费分支不触发，生图请求记 $0。
//
// 只有响应里数不出图时才退回既有的模型名启发式，保证老行为不回退；额外也认映射后的上游模型名。
func resolveGeminiImageCount(c *gin.Context, originalModel, mappedModel string) int {
	if observed := observedGeminiImageOutputs(c); observed > 0 {
		return observed
	}
	if isImageGenerationModel(originalModel) || isImageGenerationModel(mappedModel) {
		return 1
	}
	return 0
}

// resolveGeminiStreamImageCount 在 resolveGeminiImageCount 之上处理客户端中途断开：收到结束分片
// 前断开时只计已送达客户端的图片（上游 2b1f12938 + b6eb6c1ef）。Claude 兼容流不向客户端转发
// 图片，delivered 恒为 0。
func resolveGeminiStreamImageCount(c *gin.Context, originalModel, mappedModel string, clientDisconnect, finished bool) int {
	if clientDisconnect && !finished {
		return deliveredGeminiImageOutputs(c)
	}
	return resolveGeminiImageCount(c, originalModel, mappedModel)
}

// countGeminiImageOutputs 统计一段 Gemini 响应 JSON 里的图片 part。
// 同时识别 inlineData、fileData 及 SDK/中转使用的 snake_case 字段。
func countGeminiImageOutputs(payload []byte) int {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return 0
	}
	count := 0
	gjson.GetBytes(payload, "candidates").ForEach(func(_, candidate gjson.Result) bool {
		candidate.Get("content.parts").ForEach(func(_, part gjson.Result) bool {
			if geminiPartIsImage(part) {
				count++
			}
			return true
		})
		return true
	})
	return count
}

func geminiPartIsImage(part gjson.Result) bool {
	for _, key := range []string{"inlineData", "inline_data", "fileData", "file_data"} {
		image := part.Get(key)
		mimeType := image.Get("mimeType")
		if !mimeType.Exists() {
			mimeType = image.Get("mime_type")
		}
		if !isGeminiInlineImageMIMEType(strings.ToLower(strings.TrimSpace(mimeType.String()))) {
			continue
		}
		data := image.Get("data")
		if key == "fileData" || key == "file_data" {
			data = image.Get("fileUri")
			if !data.Exists() {
				data = image.Get("file_uri")
			}
		}
		// 只认带有实际数据或 URI 的图片 part；同一个 part 最多计费一次。
		if data.Type == gjson.String && strings.TrimSpace(data.String()) != "" {
			return true
		}
	}
	return false
}

// isGeminiInlineImageMIMEType 报告 MIME 是否为 Gemini 生图会回吐的图片类型。
func isGeminiInlineImageMIMEType(mimeType string) bool {
	switch mimeType {
	case "image/gif", "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}
