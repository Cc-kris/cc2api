// V03-T15：分组「按张计费」只在 openai 平台且允许生图时生效，与后端
// service.imageCountBillingAllowed 一致。前端在开关不适用时同步置为 false，提交的请求也是 false。
export const imageCountBillingAllowed = (
  platform: string,
  allowImageGeneration: boolean,
): boolean => allowImageGeneration && platform === "openai";

export const normalizeImageCountBilling = (form: {
  platform: string;
  allow_image_generation: boolean;
  image_count_billing: boolean;
}): boolean =>
  imageCountBillingAllowed(form.platform, form.allow_image_generation) &&
  form.image_count_billing;
