package service

// V03-T15：分组「按张计费」开关（移植上游 fd998129e、aa1a1b11c、3ce09c8af）。

// imageCountBillingAllowed 报告分组能否开启按张计费：只支持允许图片生成的 openai 分组。
// 上游还支持 composite 平台，CCAI 没有该平台。
func imageCountBillingAllowed(platform string, allowImageGeneration bool) bool {
	return allowImageGeneration && platform == PlatformOpenAI
}

// imageCountBillingEnabled 表示该分组强制按实际出图张数计费，即使渠道是 token 计费。
func imageCountBillingEnabled(apiKey *APIKey) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.ImageCountBilling
}

// imageCountBillingApplies 判断分组按张计费开关是否作用于本次请求。
// 开关只影响专用图片端点；混合的 Responses/Chat/WS 请求即使带图片工具，
// 也必须保留 token 计费，避免整条请求被按图片单价结算而丢失文本 token 费用。
func imageCountBillingApplies(apiKey *APIKey, imageEndpoint bool) bool {
	return imageEndpoint && imageCountBillingEnabled(apiKey)
}

// resolveBillableImageCount 决定一次图片请求按几张计费。
//
// 以响应体中实际返回的图片为准。解析不到张数时：
//   - 分组开启按张计费：按 1 张兜底，避免已成功出图的请求完全漏计；
//   - 未开启：保持原口径（流式 0 张、非流式回退请求 n），不能因为开关之外的原因多扣。
func resolveBillableImageCount(reported, requested int, fallbackToOne bool) int {
	if reported > 0 {
		return reported
	}
	if fallbackToOne {
		return 1
	}
	if requested > 0 {
		return requested
	}
	return 0
}
