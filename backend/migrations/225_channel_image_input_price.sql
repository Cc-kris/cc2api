-- V03-T14: 渠道自定义定价（token 模式）新增图片输入 token 单价（上游 06e03f467 迁移 178）。
-- 图文混合请求中图片部分的输入 token 按 image_input_price 计费，文本部分按 input_price。
-- 未配置（NULL）时计费回退到文本输入价，总额与升级前一致。
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS image_input_price NUMERIC(20,12);
