package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// V03-R12 在途预占的 handler 接入（移植上游 43f88c99a / 52c123e6f 的 gateway_inflight_reservation.go）。
//
// 每个 CheckBillingEligibility 调用点之后调用 reserveInflightBalance：预占句柄挂到 c.Request 的
// context 上，之后经 submit*UsageRecordTask 提交的计费任务接管一个引用，直到计费落库、余额缓存
// 同步扣减后才释放；没有提交计费任务的失败路径在 handler 返回（done）时立即释放。
// 续写换号、普通换号都在同一个 handler 循环里，复用同一个预占，不会每次尝试重复预占。

// inflightReservationEstimator 估算单请求在途预占金额（USD）；false 表示无法定价。
type inflightReservationEstimator interface {
	EstimateInflightReservation(ctx context.Context, apiKey *service.APIKey, req service.InflightEstimateRequest) (float64, bool)
}

// requestMaxOutputTokens 从请求体中提取输出 token 上限（兼容 Anthropic / OpenAI Chat / Responses / Gemini）。
func requestMaxOutputTokens(body []byte) int {
	for _, path := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens", "generationConfig.maxOutputTokens", "generation_config.max_output_tokens"} {
		if v := gjson.GetBytes(body, path); v.Exists() && v.Type == gjson.Number && v.Int() > 0 {
			return int(v.Int())
		}
	}
	return 0
}

// requestHasImageInput 粗判请求是否带图片输入（OpenAI image_url / input_image、Anthropic image 块、
// Gemini inlineData 图片）。命中时估算额外按「输入全部是图片 token」计一次。
func requestHasImageInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := string(body)
	for _, marker := range []string{`"image_url"`, `"input_image"`, `"type":"image"`, `"type": "image"`, `"inlineData"`, `"inline_data"`} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// tokenInflightEstimate 文本类请求的估算输入。
func tokenInflightEstimate(model string, body []byte, serviceTier string) service.InflightEstimateRequest {
	return service.InflightEstimateRequest{
		Model:         model,
		BodyBytes:     len(body),
		MaxTokens:     requestMaxOutputTokens(body),
		Kind:          service.InflightEstimateToken,
		ServiceTier:   serviceTier,
		HasImageInput: requestHasImageInput(body),
	}
}

// geminiCandidateCount Gemini 请求的 candidateCount（缺省 1）；图片模型按它估算张数上界。
func geminiCandidateCount(body []byte) int {
	for _, path := range []string{"generationConfig.candidateCount", "generation_config.candidate_count"} {
		if v := gjson.GetBytes(body, path); v.Exists() && v.Int() > 0 {
			return int(v.Int())
		}
	}
	return 1
}

// withGeminiImageEstimate 请求模型或映射模型是 Gemini 图片生成模型时，同时按张估算。
func withGeminiImageEstimate(req service.InflightEstimateRequest, body []byte, models ...string) service.InflightEstimateRequest {
	for _, m := range models {
		if service.IsImageGenerationModel(m) {
			req.MayGenerateImage = true
			req.Units = geminiCandidateCount(body)
			return req
		}
	}
	return req
}

func inflightNoop() {}

// reserveInflightBalance 在 CheckBillingEligibility 之后为余额模式请求登记在途预占。
//
// 成功时把预占句柄挂到 c.Request 的 context 上。返回的 done 必须 defer 调用（停止续期并归还
// handler 引用；没有提交计费任务时即刻释放）。开关关闭、订阅模式、Redis 故障时返回 no-op
// （fail-open）；无法定价时默认 fail-open，配置 fail_closed_on_unpriced=true 时返回
// ErrInsufficientBalance。context 上已有预占（同一请求再次准入，如兜底分组重试）时不重复预占。
func reserveInflightBalance(
	c *gin.Context,
	billing *service.BillingCacheService,
	estimator inflightReservationEstimator,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	req service.InflightEstimateRequest,
) (func(), error) {
	if c == nil || c.Request == nil {
		return inflightNoop, nil
	}
	ctx, done, err := reserveInflightBalanceCtx(c.Request.Context(), billing, estimator, apiKey, subscription, req)
	if err != nil {
		return inflightNoop, err
	}
	c.Request = c.Request.WithContext(ctx)
	return done, nil
}

// reserveInflightBalanceCtx 同 reserveInflightBalance，但返回携带预占句柄的新 context
// （供 WebSocket 等自管 context 的路径）。
func reserveInflightBalanceCtx(
	ctx context.Context,
	billing *service.BillingCacheService,
	estimator inflightReservationEstimator,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	req service.InflightEstimateRequest,
) (context.Context, func(), error) {
	if billing == nil || estimator == nil || apiKey == nil || apiKey.User == nil || !billing.InflightReservationEnabled() {
		return ctx, inflightNoop, nil
	}
	if service.InflightReservationFromContext(ctx) != nil {
		return ctx, inflightNoop, nil
	}
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() && subscription != nil {
		return ctx, inflightNoop, nil
	}
	estimate, priced := estimator.EstimateInflightReservation(ctx, apiKey, req)
	if !priced && billing.InflightReservationFailClosedOnUnpriced() {
		return ctx, inflightNoop, service.ErrInsufficientBalance
	}
	if estimate <= 0 {
		return ctx, inflightNoop, nil
	}
	res, err := billing.ReserveInflight(ctx, apiKey.User, apiKey.Group, subscription, estimate)
	if err != nil {
		return ctx, inflightNoop, err
	}
	if res == nil {
		return ctx, inflightNoop, nil
	}
	return service.WithInflightReservation(ctx, res), res.HandlerDone, nil
}

// reserveInflightFixedCharge 确定金额费用（x_search 单次价）的预占：余额必须覆盖金额。
func reserveInflightFixedCharge(
	c *gin.Context,
	billing *service.BillingCacheService,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	amount float64,
) (func(), error) {
	if c == nil || c.Request == nil || billing == nil || apiKey == nil || apiKey.User == nil {
		return inflightNoop, nil
	}
	if service.InflightReservationFromContext(c.Request.Context()) != nil {
		return inflightNoop, nil
	}
	res, err := billing.ReserveFixedCharge(c.Request.Context(), apiKey.User, apiKey.Group, subscription, amount)
	if err != nil {
		return inflightNoop, err
	}
	if res == nil {
		return inflightNoop, nil
	}
	c.Request = c.Request.WithContext(service.WithInflightReservation(c.Request.Context(), res))
	return res.HandlerDone, nil
}

// wrapUsageRecordTaskInflight 计费任务接管请求的在途预占引用（上游 wrapUsageRecordTaskContext）。
// 任务执行结束（含 panic）时归还；返回的 abandon 在任务未被执行（被丢弃）时必须调用。
// 余额缓存在计费路径中同步扣减（syncBalanceCacheAfterDeduction），所以归还时准入已能看到扣费。
func wrapUsageRecordTaskInflight(parent context.Context, task service.UsageRecordTask) (service.UsageRecordTask, func()) {
	if task == nil {
		return nil, inflightNoop
	}
	done := inflightNoop
	if parent != nil {
		done = service.InflightReservationFromContext(parent).Acquire()
	}
	return func(ctx context.Context) {
		defer done()
		task(ctx)
	}, done
}

// grokMediaInflightEstimate 媒体生成请求的估算输入；状态/内容查询返回空模型（不预占：
// 查询会为已生成的媒体计费，不能因余额预占而拦截用户取回已付费结果）。
func grokMediaInflightEstimate(endpoint service.GrokMediaEndpoint, model string, info service.GrokMediaRequestInfo, body []byte) service.InflightEstimateRequest {
	if !endpoint.IsGenerationRequest() {
		return service.InflightEstimateRequest{}
	}
	switch endpoint {
	case service.GrokMediaEndpointImagesGenerations, service.GrokMediaEndpointImagesEdits:
		return service.InflightEstimateRequest{Model: model, BodyBytes: len(body), Kind: service.InflightEstimateImage, Units: info.N}
	default:
		return service.InflightEstimateRequest{
			Model:                model,
			BodyBytes:            len(body),
			Kind:                 service.InflightEstimateVideo,
			Units:                1,
			VideoResolution:      info.Resolution,
			VideoDurationSeconds: info.DurationSeconds,
		}
	}
}
