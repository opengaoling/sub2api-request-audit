-- Update model_mapping whitelist for all OpenAI OAuth accounts
--
-- Background:
-- OpenAI OAuth (ChatGPT Codex / Responses API) supports current generation models
-- including GPT-6.2, GPT-6.1, GPT-6, GPT-5.6, GPT-5.5, GPT-5.4, GPT-5.3, GPT-5.2,
-- o1/o3/o4 reasoning models, GPT-4.1, GPT-4o, and image models.
-- Existing OpenAI OAuth accounts need their model whitelist automatically updated
-- with the new supported models while preserving any custom mappings.

UPDATE accounts
SET credentials = jsonb_set(
    COALESCE(credentials, '{}'::jsonb),
    '{model_mapping}',
    (
        '{
            "gpt-6.2": "gpt-6.2-sol",
            "gpt-6.2-sol": "gpt-6.2-sol",
            "gpt-6.2-astra": "gpt-6.2-astra",
            "gpt-6.1": "gpt-6.1-sol",
            "gpt-6.1-sol": "gpt-6.1-sol",
            "gpt-6.1-terra": "gpt-6.1-terra",
            "gpt-6": "gpt-6-astra",
            "gpt-6-astra": "gpt-6-astra",
            "gpt-6-sol": "gpt-6-sol",
            "gpt-6-luna": "gpt-6-luna",
            "gpt-5.6": "gpt-5.6-sol",
            "gpt-5.6-sol": "gpt-5.6-sol",
            "gpt-5.6-terra": "gpt-5.6-terra",
            "gpt-5.6-luna": "gpt-5.6-luna",
            "gpt-5.5": "gpt-5.5",
            "gpt-5.5-pro": "gpt-5.5-pro",
            "gpt-5.4": "gpt-5.4",
            "gpt-5.4-pro": "gpt-5.4-pro",
            "gpt-5.4-mini": "gpt-5.4-mini",
            "gpt-5.4-nano": "gpt-5.4-nano",
            "gpt-5.3-codex-spark": "gpt-5.3-codex-spark",
            "codex-auto-review": "codex-auto-review",
            "gpt-5.2": "gpt-5.2",
            "gpt-5.2-pro": "gpt-5.2-pro",
            "o1": "o1",
            "o1-pro": "o1-pro",
            "o1-mini": "o1-mini",
            "o3": "o3",
            "o3-pro": "o3-pro",
            "o3-mini": "o3-mini",
            "o4-mini": "o4-mini",
            "gpt-4.1": "gpt-4.1",
            "gpt-4.1-mini": "gpt-4.1-mini",
            "gpt-4.1-nano": "gpt-4.1-nano",
            "gpt-4o": "gpt-4o",
            "gpt-4o-mini": "gpt-4o-mini",
            "gpt-4-turbo": "gpt-4-turbo",
            "gpt-4": "gpt-4",
            "gpt-3.5-turbo": "gpt-3.5-turbo",
            "gpt-image-1": "gpt-image-1",
            "gpt-image-1.5": "gpt-image-1.5",
            "gpt-image-2": "gpt-image-2",
            "gpt-image-2.5-flare": "gpt-image-2.5-flare",
            "gpt-image-2.5-sunburst": "gpt-image-2.5-sunburst",
            "gpt-image-2.6-flare": "gpt-image-2.6-flare",
            "gpt-image-2.6-sunburst": "gpt-image-2.6-sunburst"
        }'::jsonb ||
        CASE 
            WHEN jsonb_typeof(credentials->'model_mapping') = 'object' 
            THEN credentials->'model_mapping' 
            ELSE '{}'::jsonb 
        END
    ),
    true
)
WHERE platform = 'openai'
  AND type = 'oauth'
  AND deleted_at IS NULL;
