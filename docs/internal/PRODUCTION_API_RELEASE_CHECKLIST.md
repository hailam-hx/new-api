# Production API release gate — Phase 5H

This checklist is a release procedure, not a claim that every item has passed.
Record the candidate commit/build, exact commands and observed results in the
handoff. Historical Phase 4 counts do not satisfy a fresh gate. User-directed
Phase 5 work remains unstaged/uncommitted/unpushed until separately authorized.

## Required deployment configuration

- [ ] **DOCS_SITE_URL: REQUIRED_FOR_PRODUCTION_DOCS.** Operator supplies the
  externally served HTTPS docs origin/path at frontend build time; no credentials,
  query or fragment. Rebuild/prerender after setting it; backend runtime env alone
  cannot change generated canonical URLs. Verify canonical URLs and
  `docs-sitemap.xml` on the actual intended origin. Source:
  [`.env.example`](../../.env.example), documentation prerender scripts.
- [ ] Public HTTPS/WSS termination, trusted proxy policy and deployment timeout/
  body/concurrency limits reviewed. No provider key or secret is present in
  frontend assets, examples, request logs or credentials-bearing WS headers.
- [ ] Native plugin operations remain deployment/model-dependent and conservatively
  **beta/experimental** unless explicitly promoted. `/v1` alone does not imply
  stability or universal support. Audit installed/enabled plugin and channel
  bindings, supported request modes and opt-in WS channels; no paid canary calls.

## Public contract and tooling

- [ ] `cd web && bun run docs:check` validates current generated references.
- [ ] `cd web && bun run docs:contract -- --base-ref <committed-public-contract-ref>`
  passes OpenAPI schema, callable-route coverage, refs, unique operation IDs and
  examples against the committed contract. Retain all 107 existing operations
  unless an intentional reviewed/versioned change is recorded.
- [ ] Contract comparison demonstrates BREAKING fails, ADDITIVE passes and
  POTENTIALLY_BREAKING is surfaced. The baseline is committed, not an arbitrary
  working-tree snapshot; no magic environment bypass or silent baseline reset.
- [ ] `node web/scripts/verify-public-asyncapi.mjs --install` passes complete
  AsyncAPI 3.0.0 schema/refs/operations and payload examples. Negative fixtures
  reject missing refs, invalid actions/examples and remote refs. No webhook
  channels are published. Parser's informational 3.1.0 recommendation is recorded.
- [ ] `node web/scripts/verify-public-sdk.mjs --install` generates Python and
  TypeScript clients in temporary tools/workspaces, verifies REST compile/runtime
  smoke and local SSE/WebSocket recipes. No generated files are manually patched
  or committed. Generated REST success is not native generated WS/SSE support.
- [ ] SDK readiness report records separate Python/TypeScript REST, SSE and
  WebSocket statuses, exact generator/template limitations and manual recipe
  verification. Critical recipe checks use built-in JSON tools; jq remains optional.
- [ ] CI runs OpenAPI/route/breaking-change and SDK/AsyncAPI gates without paid or
  provider network requests. Tool downloads are distinct from provider calls.

## Backend correctness and security

- [ ] Fresh targeted idempotency tests cover sequential/concurrent same-key replay,
  payload conflict, cross-token/same-user isolation, revoked/expired keys, validation,
  zero balance, pre-durable failure, post-acceptance ambiguity, durable/completed/
  failed duplicates and expiry. Supported JSON/multipart/plugin/video/background
  paths are explicitly listed; unsupported endpoints make no idempotency promise.
- [ ] Upstream logical submissions, reservations, durable task rows, settlement and
  refunds counted. No duplicate paid work, second charge or independent duplicate
  refund; accepted-work errors never trigger blind resubmission. Internal Request
  Policy retry remains distinct from client idempotency and background polling.
- [ ] SSE regression covers actual LF/CRLF framing, multiline data, comments,
  event/id fields, limits, malformed payloads, terminal/EOF, slow/idle/cancel paths
  and exact settlement/disconnect behavior for each audited protocol.
- [ ] Responses WS local handshake/auth/model/channel/messages/errors/size/close/
  timeout/cleanup/billing suites pass, including race-enabled tests. Inactive
  first/next-request sessions are bounded; no stale channel callback or worker
  remains after close. Source report:
  [WEBSOCKET_CONTRACT_REPORT.md](WEBSOCKET_CONTRACT_REPORT.md).
- [ ] Applicable authentication controls are verified server-side against current
  OWASP guidance; record ASVS version/IDs where used, tested expiry/replay/bypass
  cases and unresolved gaps. No blanket compliance claim from a successful build.
- [ ] Full `go test ./...` passes (or every blocker/failure is reported). If relaykit
  or its public API changed, `cd relaykit && GOWORK=off go build ./...` passes.
- [ ] Database verification is **conditional on persistence behavior changes**:
  real SQLite, MySQL 5.7 and PostgreSQL 15; exact versions/commands/results;
  fresh and representative released-version upgrade migration; startup twice;
  uniqueness/concurrency, existing data/index/constraint preservation and separate
  log DB when affected. Mocks/build/one dialect do not substitute. Mark NOT_APPLICABLE
  when no DB behavior changed; otherwise any missing matrix blocks release.

## Frontend, docs and artifacts

- [ ] `cd web && bun run test`, `bun run typecheck`, `bun run build` pass with
  actual counts and prerender page total recorded. Full lint historical baseline
  is reported; changed-file oxlint has zero new errors. Check formatting/diff.
- [ ] Seven locale sources remain consistent; new UI copy is translated; dynamic
  number/currency locales use existing format helpers and valid Intl mappings.
- [ ] Developer navigation/search covers Authentication, Idempotency, Streaming,
  WebSocket, Errors/Retry, Versioning, Changelog and Recipes. Beginner sidebar
  remains appropriate. OpenAPI/AsyncAPI links/downloads resolve after build.
- [ ] Browser review verifies representative developer pages, mobile navigation,
  code samples, spec downloads, external/docs links and useful search results.
  Prerendered HTML, canonical URLs and sitemap agree with the build configuration.
- [ ] Unreleased changelog lists only implemented capabilities/tests/CI. No invented
  release date, universal provider support, idempotency claim on unsupported routes,
  generated streaming support or new status/webhook endpoint is advertised.

## Scope and unresolved product gates

- [ ] Webhook remains **DESIGN_ONLY** until ownership, atomic outbox, independent
  signing secrets/replay, bounded retry/dead letters and mandatory SSRF/DNS/TLS
  strategy are implemented and verified. See
  [WEBHOOK_DESIGN_REPORT.md](WEBHOOK_DESIGN_REPORT.md).
- [ ] New public status remains **DESIGN_ONLY**; existing bootstrap/admin diagnostics
  and configured uptime display are accurately distinguished. No internal DB/Redis/
  node/channel secrets are exposed. See
  [PUBLIC_STATUS_DESIGN_REPORT.md](PUBLIC_STATUS_DESIGN_REPORT.md).
- [ ] Phase 3D stays **BLOCKED_BY_OWNERSHIP_MODEL**; no
  `GET /v1/usage/requests/{request_id}` without canonical final settlement/refund
  history. Phase 3E cancellation stays **DESIGN_ONLY**; no universal public cancel
  API without actual plugin/provider capability.
- [ ] Record operator blockers separately from local test results. An unset
  DOCS_SITE_URL remains a production docs blocker even when relative local docs
  build correctly. Preserve protected new-api and QuantumNous attribution.
- [ ] Final `git status`, `git diff --stat` and `git diff --check` reviewed; no
  staging/commit/push occurred during this task; paid/provider upstream requests 0.
