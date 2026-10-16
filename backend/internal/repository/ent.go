// Package repository 提供应用程序的基础设施层组件。
// 包括数据库连接初始化、ORM 客户端管理、Redis 连接、数据库迁移等核心功能。
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // PostgreSQL 驱动，通过副作用导入注册驱动
)

// InitEnt 初始化 Ent ORM 客户端并返回客户端实例和底层的 *sql.DB。
//
// 该函数执行以下操作：
//  1. 初始化全局时区设置，确保时间处理一致性
//  2. 建立 PostgreSQL 数据库连接
//  3. 自动执行数据库迁移，确保 schema 与代码同步
//  4. 创建并返回 Ent 客户端实例
//
// 重要提示：调用者必须负责关闭返回的 ent.Client（关闭时会自动关闭底层的 driver/db）。
//
// 参数：
//   - cfg: 应用程序配置，包含数据库连接信息和时区设置
//
// 返回：
//   - *ent.Client: Ent ORM 客户端，用于执行数据库操作
//   - *sql.DB: 底层的 SQL 数据库连接，可用于直接执行原生 SQL
//   - error: 初始化过程中的错误
func InitEnt(cfg *config.Config) (*ent.Client, *sql.DB, error) {
	// 优先初始化时区设置，确保所有时间操作使用统一的时区。
	// 这对于跨时区部署和日志时间戳的一致性至关重要。
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, nil, err
	}

	// 构建包含时区信息的数据库连接字符串 (DSN)。
	// 时区信息会传递给 PostgreSQL，确保数据库层面的时间处理正确。
	dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

	// 使用 Ent 的 SQL 驱动打开 PostgreSQL 连接。
	// dialect.Postgres 指定使用 PostgreSQL 方言进行 SQL 生成。
	drv, err := entsql.Open(dialect.Postgres, dsn)
	if err != nil {
		return nil, nil, err
	}
	applyDBPoolSettings(drv.DB(), cfg)

	// 确保数据库 schema 已准备就绪。
	// SQL 迁移文件是 schema 的权威来源（source of truth）。
	// 这种方式比 Ent 的自动迁移更可控，支持复杂的迁移场景。
	migrationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := applyMigrationsFS(migrationCtx, drv.DB(), migrations.FS); err != nil {
		_ = drv.Close() // 迁移失败时关闭驱动，避免资源泄露
		return nil, nil, err
	}

	// 自动自愈与双向兼容上游数据库结构（即使切换过上游更高版本镜像，也能平滑启动）
	if err := ensureUpstreamCompatibility(migrationCtx, drv.DB()); err != nil {
		_ = drv.Close()
		return nil, nil, fmt.Errorf("ensure upstream compatibility: %w", err)
	}

	// 创建 Ent 客户端，绑定到已配置的数据库驱动。
	client := ent.NewClient(ent.Driver(drv))

	// 启动阶段：从配置或数据库中确保系统密钥可用。
	if err := ensureBootstrapSecrets(migrationCtx, client, cfg); err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	// 在密钥补齐后执行完整配置校验，避免空 jwt.secret 导致服务运行时失败。
	if err := cfg.Validate(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("validate config after secret bootstrap: %w", err)
	}

	// SIMPLE 模式：启动时补齐各平台默认分组。
	// - anthropic/openai/gemini: 确保存在 <platform>-default
	// - antigravity: 仅要求存在 >=2 个未软删除分组（用于 claude/gemini 混合调度场景）
	if cfg.RunMode == config.RunModeSimple {
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer seedCancel()
		if err := ensureSimpleModeDefaultGroups(seedCtx, client); err != nil {
			_ = client.Close()
			return nil, nil, err
		}
		if err := ensureSimpleModeAdminConcurrency(seedCtx, client); err != nil {
			_ = client.Close()
			return nil, nil, err
		}
	}

	return client, drv.DB(), nil
}

func ensureUpstreamCompatibility(ctx context.Context, db *sql.DB) error {
	const compatSQL = `
DO $$
BEGIN
    -- 1. groups.models_list_config 与 groups.model_allowlist 相互兼容
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

    -- 3. groups 表其它常见高版本列补齐
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
`
	_, err := db.ExecContext(ctx, compatSQL)
	return err
}
