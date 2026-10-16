-- 191: 自愈与双向兼容上游数据库结构
-- 针对切换上游更高版本镜像后再切回本项目导致的字段不匹配问题进行自动修复与持续同步

DO $$
BEGIN
    -- 1. groups.models_list_config 与 groups.model_allowlist 相互兼容
    -- 如果上游 235/236 迁移将 models_list_config 改名为了 model_allowlist，则自动补回 models_list_config 并回填数据
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'groups' AND column_name = 'model_allowlist'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'groups' AND column_name = 'models_list_config'
    ) THEN
        ALTER TABLE groups ADD COLUMN models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;
        UPDATE groups SET models_list_config = model_allowlist WHERE model_allowlist IS NOT NULL;
    END IF;

    -- 反向对齐：如果存在 models_list_config 但尚无 model_allowlist，为未来切上游预置字段并同步
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'groups' AND column_name = 'models_list_config'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'groups' AND column_name = 'model_allowlist'
    ) THEN
        ALTER TABLE groups ADD COLUMN model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;
        UPDATE groups SET model_allowlist = models_list_config WHERE models_list_config IS NOT NULL;
    END IF;

    -- 2. usage_logs 兼容列补齐
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'usage_logs') THEN
        ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_response_model VARCHAR(200);
        ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_model_mismatch BOOLEAN;
        ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS native_compaction_v2 BOOLEAN NOT NULL DEFAULT FALSE;
        ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);
    END IF;

    -- 3. groups 表其它常见高版本列补齐（防止跨版本读取缺失字段）
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'groups') THEN
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS i18n JSONB NOT NULL DEFAULT '{}'::jsonb;
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS peak_rate_enabled BOOLEAN NOT NULL DEFAULT FALSE;
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS peak_start VARCHAR(5) NOT NULL DEFAULT '';
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS peak_end VARCHAR(5) NOT NULL DEFAULT '';
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS peak_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS duplicate_operation_id VARCHAR(64);
        ALTER TABLE groups ADD COLUMN IF NOT EXISTS allow_batch_image_generation BOOLEAN NOT NULL DEFAULT FALSE;
    END IF;
END $$;

-- 4. 创建触发器实现 groups.models_list_config 与 groups.model_allowlist 的双向实时同步
CREATE OR REPLACE FUNCTION sync_group_model_allowlist_compat() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' OR TG_OP = 'UPDATE' THEN
        IF NEW.models_list_config IS NOT NULL AND (NEW.model_allowlist IS NULL OR NEW.model_allowlist = '{}'::jsonb) AND NEW.models_list_config <> '{}'::jsonb THEN
            NEW.model_allowlist := NEW.models_list_config;
        ELSIF NEW.model_allowlist IS NOT NULL AND (NEW.models_list_config IS NULL OR NEW.models_list_config = '{}'::jsonb) AND NEW.model_allowlist <> '{}'::jsonb THEN
            NEW.models_list_config := NEW.model_allowlist;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'groups') THEN
        DROP TRIGGER IF EXISTS trg_sync_group_model_allowlist_compat ON groups;
        CREATE TRIGGER trg_sync_group_model_allowlist_compat
        BEFORE INSERT OR UPDATE ON groups
        FOR EACH ROW EXECUTE FUNCTION sync_group_model_allowlist_compat();
    END IF;
END $$;
