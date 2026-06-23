# Upstream Sync Notes

## 2026-06-23 upstream v0.1.138 targeted bugfix sync

Upstream reviewed: `Wei-Shaw/sub2api` release `v0.1.138` (`69366878`) and `upstream/main` through `85a3b122`.

Scope applied to this fork:

- Reviewed each candidate before porting; only Claude/OpenAI/Gemini bugfixes were synced.
- Kept local request audit/request intercept behavior.
- Updated `backend/cmd/server/VERSION` to `0.1.138`.
- Build verification for this project must run through GitHub workflows; do not run local builds for delivery validation.

Synced bugfixes:

- `6cfb7898` / `5cb8cdd3` - adapt Claude Code mimicry/detection to the new CLI billing block without `cch`.
- `e3e31bd4` - recognize Claude Code IDE clients via any `cc_entrypoint`.
- `40e1cc14` / `efffd5d7` - filter unsupported `anthropic-beta` tokens on the Vertex Anthropic path.
- `6c2db4f4` / `8c4a43cf` - clean Gemini unsupported tool schema fields, including `$defs`, `definitions`, and nullable type arrays.
- `b0d5592a` / `69366878` - recognize OpenAI image `response.incomplete`, record soft-failure upstream diagnostics, and keep lint-safe builder writes.
- `bab8a9a9` - record the actual OpenAI upstream endpoint for chat-only API-key accounts.

Reviewed but intentionally skipped:

- `89cfe24a` - GLM reasoning-effort normalization. It is implemented in an OpenAI-compatible raw chat path, but the release item is GLM-specific and outside the requested Claude/OpenAI/Gemini upstream scope.
- `0fa604ba`, `510adf70`, `d3dfa28f`, `51d72290`, `31640363`, `2dc1387b`, `952be871`, `ecedc7c8` and other unrelated feature/UI/deploy/auth/promo changes from the release.

## 2026-06-18 upstream bugfix sync

Upstream reviewed: `Wei-Shaw/sub2api` through `upstream/main` `4a5665da`.

Scope applied to this fork:

- Cherry-picked bugfix and security/dependency-fix commits only.
- Kept local request audit and request intercept functionality.
- Skipped upstream feature commits unless a small non-feature migration maintenance commit was required to keep selected bugfix migrations ordered correctly.

Permanent exclusion:

- Do not merge the upstream admin compliance acknowledgement gate. This includes, but is not limited to, backend compliance handlers/services/middleware, frontend compliance dialog/store/API/i18n/routes, `docs/legal/admin-compliance.*.md`, and Docker build-context changes that exist only to support that gate.

Reason:

- The feature blocks admin console usage until an acknowledgement is submitted and records acknowledgement evidence such as admin user ID, IP address, User-Agent, version, and timestamp. This fork intentionally excludes that behavior.

Skipped upstream commits related to this exclusion:

- `0acf00c4` - Add admin compliance acknowledgement gate
- `ad135854` - fix(docker): ship docs/legal in build context for admin-compliance gate
