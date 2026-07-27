CREATE TABLE IF NOT EXISTS client_request_fingerprints (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(32) NOT NULL,
    fingerprint_hash VARCHAR(64) NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    user_agent TEXT NOT NULL DEFAULT '',
    capture_count BIGINT NOT NULL DEFAULT 1,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT client_request_fingerprints_platform_hash_key UNIQUE (platform, fingerprint_hash)
);

CREATE INDEX IF NOT EXISTS client_request_fingerprints_platform_last_seen_idx
    ON client_request_fingerprints (platform, last_seen_at DESC);
