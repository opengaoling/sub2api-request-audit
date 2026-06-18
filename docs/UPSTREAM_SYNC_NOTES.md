# Upstream Sync Notes

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
