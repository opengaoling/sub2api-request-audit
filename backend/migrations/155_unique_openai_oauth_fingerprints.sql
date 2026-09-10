WITH duplicate_fingerprints AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY extra->>'openai_fingerprint_id'
            ORDER BY id
        ) AS duplicate_rank
    FROM accounts
    WHERE platform = 'openai'
      AND type = 'oauth'
      AND deleted_at IS NULL
      AND NULLIF(extra->>'openai_fingerprint_id', '') IS NOT NULL
)
UPDATE accounts AS accounts_to_clean
SET extra = accounts_to_clean.extra - 'openai_fingerprint_id',
    updated_at = NOW()
FROM duplicate_fingerprints
WHERE accounts_to_clean.id = duplicate_fingerprints.id
  AND duplicate_fingerprints.duplicate_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS accounts_openai_oauth_fingerprint_unique_idx
    ON accounts ((extra->>'openai_fingerprint_id'))
    WHERE platform = 'openai'
      AND type = 'oauth'
      AND deleted_at IS NULL
      AND NULLIF(extra->>'openai_fingerprint_id', '') IS NOT NULL;
