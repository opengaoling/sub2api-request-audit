ALTER TABLE groups ADD COLUMN IF NOT EXISTS image_responses_model VARCHAR(255) NOT NULL DEFAULT '';

COMMENT ON COLUMN groups.image_responses_model IS 'gpt-image-* 路由到 /responses 端点时使用的主调度模型，空表示使用全局默认值';
