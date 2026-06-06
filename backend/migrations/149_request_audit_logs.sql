CREATE TABLE IF NOT EXISTS request_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(128),
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    account_id BIGINT,
    group_id BIGINT,
    platform VARCHAR(32) NOT NULL,
    endpoint VARCHAR(128),
    model VARCHAR(128),
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    status_code INTEGER,
    duration_ms INTEGER,
    request_body TEXT,
    response_body TEXT,
    request_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    request_body_bytes INTEGER NOT NULL DEFAULT 0,
    response_body_bytes INTEGER NOT NULL DEFAULT 0,
    is_mocked BOOLEAN NOT NULL DEFAULT FALSE,
    mock_rule_id BIGINT,
    error_message VARCHAR(1024),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS requestauditlog_created_at ON request_audit_logs (created_at);
CREATE INDEX IF NOT EXISTS requestauditlog_user_id ON request_audit_logs (user_id);
CREATE INDEX IF NOT EXISTS requestauditlog_api_key_id ON request_audit_logs (api_key_id);
CREATE INDEX IF NOT EXISTS requestauditlog_account_id ON request_audit_logs (account_id);
CREATE INDEX IF NOT EXISTS requestauditlog_group_id ON request_audit_logs (group_id);
CREATE INDEX IF NOT EXISTS requestauditlog_platform ON request_audit_logs (platform);
CREATE INDEX IF NOT EXISTS requestauditlog_model ON request_audit_logs (model);
CREATE INDEX IF NOT EXISTS requestauditlog_request_id ON request_audit_logs (request_id);
CREATE INDEX IF NOT EXISTS requestauditlog_is_mocked ON request_audit_logs (is_mocked);
