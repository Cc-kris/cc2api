-- V03-T14: usage_logs 单独记录图片输入 token 数与费用（上游 62d57c02d 迁移 179）。
-- image_input_tokens 是 input_tokens 中图片部分的数量（input_tokens 仍为输入总数，财务成本侧
-- 照旧按 input_tokens 计）；image_input_cost 从 input_cost 中拆出，total_cost 口径不变。
-- 带默认值的 ADD COLUMN 在 PG11+ 只改元数据。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_input_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
