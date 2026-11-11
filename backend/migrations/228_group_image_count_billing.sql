-- V03-T15: 分组「按张计费」开关（上游 fd998129e 迁移 248）。
-- 开启后，专用图片端点（/v1/images/*）按响应实际返回的图片张数计费；解析不到张数但请求成功时按 1 张计。
-- 只对 openai 平台且允许图片生成的分组生效，默认关闭；关闭时保持现有口径。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS image_count_billing BOOLEAN NOT NULL DEFAULT false;
