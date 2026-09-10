UPDATE accounts AS accounts_to_clean
SET extra = accounts_to_clean.extra - 'openai_fingerprint_id',
    updated_at = NOW()
WHERE accounts_to_clean.platform = 'openai'
  AND accounts_to_clean.type = 'oauth'
  AND accounts_to_clean.deleted_at IS NULL
  AND NULLIF(accounts_to_clean.extra->>'openai_fingerprint_id', '') IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM client_request_fingerprints
      WHERE client_request_fingerprints.platform = 'openai'
        AND client_request_fingerprints.fingerprint_hash = accounts_to_clean.extra->>'openai_fingerprint_id'
        AND client_request_fingerprints.user_agent ILIKE '%codex%'
  );
