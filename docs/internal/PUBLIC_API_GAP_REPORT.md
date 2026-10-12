# Public developer API gap report

This report records the repository's implemented surface and proposes additions. The original documentation audit did not change backend, authentication, database or billing behavior. Phase 2 adds public read contracts; their implementation and verification status is recorded separately below. Proposed paths and schemas remain **design proposals** until their implementation is verified. Priorities are product recommendations: P0 blocks a safe integration, P1 materially improves developer operations, P2 is optional workflow support.

## Phase 2 status

| Priority | Capability | Status |
| --- | --- | --- |
| P0 | `GET /v1/catalog` | **COMPLETED**. Typed schema and key-scoped local pricing/routing projection; authorization, zero balance and three-database regression matrix passed. |
| P0 | `GET /v1/key/balance` | **COMPLETED**. Decimal display-unit conversion and exact authenticated-key allowance; quota/expiry, cache/revocation and three-database matrix passed. |
| P0 | Request-correlated final usage | **BLOCKED_BY_OWNERSHIP_MODEL** for complete canonical async/refund request-to-task/event attribution. No endpoint implemented; no automatic schema migration. Existing synchronous logs already persist user/key ownership. |
| P1 | Full key usage/history | Remaining: versioned period/history contract, distinct from current-key balance. |
| P1 | Public task listing | Remaining. Public generic task detail and artifacts already exist. |
| P2 | Capability-aware cancellation and advanced monitoring | Remaining; do not advertise unimplemented capabilities. |

`/api/pricing`, `/api/usage/token/`, `/v1/models` and existing inference APIs retain their existing roles and behavior. New recommended public read contracts use `/v1`; legacy compatibility does not establish a public v1 contract.

## Current surface and classification

| Capability | Implemented source and access | Classification / limitation |
| --- | --- | --- |
| Accessible model list | `router/relay-router.go`: `GET /v1/models`, `/v1/models/:model`, `/v1beta/models`, `/v1beta/openai/models`, `TokenAuth` | Public relay API. Names available to a key are not a complete price and capability catalog. |
| Pricing / dashboard catalog | `router/api-router.go`: `/api/pricing` uses `HeaderNavModuleAuth("pricing")`; `/api/models` uses `UserAuth`; `controller/pricing.go` and model metadata handlers | Existing site/dashboard capabilities. Do not reinterpret site navigation access or session auth as a canonical API-key catalog contract. |
| API-key quota totals | `GET /api/usage/token/`, `TokenAuthReadOnly`, `controller/token.go:GetTokenUsage` | Legacy compatibility API; excluded from recommended public v1 documentation. Response is `{code:true,message:"ok",data:{object:"token_usage",name,total_granted,total_used,total_available,unlimited_quota,model_limits,model_limits_enabled,expires_at}}`. Totals are internal quota units. |
| API-key consumption history | `GET /api/log/token`, `TokenAuthReadOnly`, `controller/log.go:GetLogByKey`, `model/log.go:GetLogByTokenId` | CONSOLE by the requested documentation boundary (§35), despite its read-only API-key middleware. It uses `{success,message,data}` internally; do not promote it in the public reference. No dedicated versioned request-ID lookup or cursor contract exists. |
| Compatibility billing | `router/dashboard.go`: `/dashboard/billing/subscription`, `/dashboard/billing/usage`, plus `/v1` aliases; `TokenAuth`; `controller/billing.go` | Implemented compatibility surface. Scope changes with `DisplayTokenStatEnabled`; amounts follow site display mode (USD, CNY or TOKENS), and usage is multiplied by 100. `*_usd` naming is not proof of invariant USD values. Controller lookup failures may be HTTP 200 with an error object. No removal/deprecation schedule is established. |
| User consumption / task lists | `/api/log/self`, `/api/log/self/stat`, `/api/task/self`, `UserAuth`; administrator equivalents use `AdminAuth` | Dashboard/session APIs. They provide existing internal query and projection building blocks but are not API-key task-list contracts. |
| Generic task submit/retrieve | `router/task-router.go`: `POST /v1/tasks/:key` (plugin key), `GET /v1/tasks/:key` (task ID); `TokenAuth`; `controller/task.go:GetTask` | Already implemented public task surface. Retrieve returns public task ID, platform, status, progress, fail reason and timestamps, owner-scoped. Do not propose another generic retrieve endpoint as missing. |
| Generic task artifacts | `GET /v1/tasks/:key/artifacts`; `GET` / `HEAD .../artifacts/:artifact_key/content` | Already implemented. Artifact listing uses `TokenAuth`; content supports token or scoped artifact-access authentication. Projection is explicit, not embedded in task lists. |
| Host task protocols | `pkg/jsplugin/routing.go:hostProtocols`, `router/task-plugin-protocol-router.go` | Responses create/retrieve; video create, generations alias, retrieve and GET/HEAD content; images generate/edit; audio speech create/retrieve. Runtime claims and bindings determine execution. |
| Provider-native tasks | Plugin manifests and routing, `docs/plugin-api/v1.md` | Installed and enabled plugin routes, provider-specific envelopes. They cannot establish a uniform cancellation or list API. |

## Registered but unusable legacy operation

`POST /v1/edits` is registered but currently routes an `ImageRequest` into `TextHelper`, which requires `GeneralOpenAIRequest` and returns HTTP 400. It must not be advertised as a working image-edit alias. The reference identifies the limitation; use the separately implemented `/v1/images/edits` contract. Evidence: `relay/constant/relay_mode.go`, `relay/helper/valid_request.go`, `controller/relay.go`, `relay/compatible_handler.go`. Repair is a backend follow-up outside this documentation task.

## Proposed additions

### P0 — API-key catalog (Phase 2 implemented)

- **Name / proposed path:** Key-scoped catalog, `GET /v1/catalog`.
- **Problem / audience:** SDK and application developers need one key-authorized discovery response for usable models, supported endpoint families and customer prices. Existing model listing plus site pricing does not provide that combined contract.
- **Internal sources:** `controller/model.go`, `controller/pricing.go`, effective model metadata, `pkg/jsplugin/routing.go`, enabled plugin routing. Reuse current public pricing projections, not administrator configuration snapshots.
- **Authentication / response proposal:** API-key read access; model entries containing client-facing name, authorized protocol operations/modes, effective customer pricing unit and configuration/availability state. Add an explicit currency and price basis when a monetary value is returned. Do not expose upstream model mappings, credentials, channel identities or provider procurement prices.
- **Version / security:** New versioned read contract, independently specified before implementation. Distinguish deployment metadata from key access; constrain response size and avoid leaking unavailable models through global catalog caching.
- **Effort / status:** Medium: key-scoped authorization and pricing/protocol projections need contract design and regression coverage. Priority P0; Phase 2 public read implementation and focused three-database verification completed.

Implemented DTO envelope (examples are illustrative, not a configured price list):

```json
{"object":"catalog","schema_version":"1.0","currency":"points","models":[]}
```

Each entry contains `id`, proven endpoint path/method pairs, customer-facing `pricing` and `billing`. Provider and category are omitted when unknown; capabilities and live availability are not fabricated. Prices are decimal strings; missing is distinct from configured zero. Unsupported lossless projections return `variable:true`. Different eligible route/group prices also become variable. The sources are four batched local reads (enabled ability/channel joins, credential-free channel projections, model metadata, vendors), current key/group permissions, existing ratio/fixed/expression sources, and actual plugin bindings. No provider request occurs.

Display currency follows configured Console conversion: points, USD, CNY or a named custom symbol. Raw-quota TOKENS display is rejected rather than exposing quota as money. `currency_symbol`/`unit_symbol` are supplied only for arbitrary custom currency. Token prices are per million; media components retain second/count/character/credit/token units from actual billing schemas. Generic Go routing metadata does not establish audio capabilities; unverified categories are omitted.

### P0 — Explicit key balance (Phase 2 implemented)

- **Name / path:** `GET /v1/key/balance`. Full `GET /v1/key/usage` history is a separate P1 proposal below.
- **Problem / audience:** Developer budgeting currently combines quota-unit `/api/usage/token/` totals with display-mode-dependent compatibility billing. The gap is an explicit modern unit/scope contract, not absence of balance queries.
- **Internal sources:** `controller/token.go:GetTokenUsage`, `controller/billing.go`, existing user/token quota getters and customer-facing formatting. A usage-period query requires current consumption-query support; existing cumulative fields must not be presented as arbitrary-period totals.
- **Authentication / response proposal:** API-key read access. Balance distinguishes key allowance from account funding, limited/unlimited state, expiry and the customer-facing `points` unit. Normalize no expiry to null and define unlimited numeric fields explicitly. Allow zero-balance inspection without quota reservation, inference charges or upstream calls. Do not represent subscription entitlements as an interchangeable wallet balance.
- **Version / security:** Additive `/v1` contract; retain current compatibility behavior. No-store/private caching; key ownership only; redact account and subscription details not needed by the key holder. Do not expose secrets or internal pricing factors.
- **Effort / status:** Medium: explicit scopes, units and subscription interactions require design. Priority P0; Phase 2 public read implementation and focused three-database verification completed.

### P1 — Full key usage/history

- **Name / proposed path:** `GET /v1/key/usage`.
- **Problem / audience:** Developers need bounded history and supported period totals beyond current-key balance; existing cumulative allowance fields are not arbitrary-period usage.
- **Internal sources:** `model/log.go`, `controller/log.go:GetLogByKey`, existing token quota totals and customer-facing points conversion. Keep `/api/log/token` in its existing Console classification.
- **Authentication / response proposal:** API-key read access scoped to the authenticated user and exact token ID, with a defined retention, pagination and consumption/refund aggregation contract. No query parameter may select a different key.
- **Version / security:** Additive `/v1` contract. Exclude internal ratios, costs, channels, upstream IDs and privileged metadata. Treat missing/expired logs as unavailable evidence rather than zero usage.
- **Effort / status:** Medium to high; P1 remaining, not implemented in Phase 2.

### P0 — Request-correlated final usage

**Status: BLOCKED_BY_OWNERSHIP_MODEL.** This blocker concerns complete canonical async/refund request-to-task/event attribution, not absence of log user or token ownership. No public endpoint is implemented in Phase 2, and no schema migration is authorized automatically.

- **Name / proposed path:** `GET /v1/usage/requests/:request_id`.
- **Problem / audience:** Developers cannot safely decide final cost or whether timed-out work completed from the client HTTP response alone. `/api/log/token` supplies history, but no request-ID lookup/filter contract. `/api/log/self` has request filters under session auth.
- **Internal sources:** `controller/log.go:GetUserLogs`, `model/log.go` request-ID fields and consumption/refund projections, `service/log_info_generate.go`, `service/task_billing.go`; `middleware/request-id.go` emits `X-Oneapi-Request-Id`.
- **Authentication / response proposal:** API-key read access, filtered to that key's records. Public request ID, model, timestamp, available usage categories, billable unit, final consumption/refund records, currency/scale where monetary, and clearly defined pending/final state when supported. Do not synthesize finality from missing logs; absence, retention and eventual availability need explicit behavior.
- **Version / security:** New `/v1` read contract. Uniform not-found for another key; no prompts, outputs, usable tokens, upstream IDs, channel details or administrator log metadata. Scope is stricter than account-wide `UserAuth` history.
- **Effort:** Medium to high: finality, multiple log events and async reconciliation need a coherent public projection. Priority P0 because retry decisions can duplicate billable work.

#### Persisted ownership and missing correlation facts

`model.Log` already stores indexed `UserId`, `TokenId` and `RequestId`, plus model, quota and token counts (`model/log.go`). `RecordConsumeLog` and `RecordErrorLog` copy the request ID from the authenticated request context. A synchronous log lookup can therefore securely enforce `request_id = ? AND user_id = ? AND token_id = ?`, using positive IDs from authentication rather than client-supplied owner identifiers. This exact key boundary must also deny a second key belonging to the same user. Token names, user-only scope, upstream IDs and request-ID randomness are not ownership proofs.

The following missing or inconsistent facts prevent a complete final-usage contract using the current canonical request ID alone:

- `RecordTaskBillingLogParams` has no request-ID field, and `RecordTaskBillingLog` leaves `Log.RequestId` unset; `createLog` generates a new unrelated ID. Async final consumes, adjustments and refunds therefore do not reliably share their initiating HTTP request ID (`model/log.go`, `service/task_billing.go`).
- New task submissions persist `Task.UserId`, `PrivateData.TokenId` and `PrivateData.Execution.RequestID/RequestPath` (`controller/relay.go`, `model/task.go`, `service/task_plugin_audit.go`), but historical snapshots may omit them. Task logs carry `other.task_id`, without a guaranteed canonical task-record/request association. Historical public task IDs are not globally unique (`model/task.go:GetUniqueByOnlyTaskId`); joining only by that string can misattribute events.
- `PrepareDFLOPTaskBillingIdentity` replaces the context/relay request ID with a hash of user ID, plugin and idempotency key after `middleware.RequestId` has emitted the response header. It does not update that header, and the hash excludes token ID (`service/billing_reservation.go`). The returned header can differ from the persisted identity, and two keys under one user can share the same operation hash. Neither fact permits widening a usage query to account scope.
- Legacy Midjourney stores user/token billing markers but no dedicated initiating request ID (`model/midjourney.go`, `service/midjourney.go`). Its refund writer also creates an unrelated log request ID. Historical correlation must not be inferred from timestamps, model names or quota amounts.
- Log recording may be disabled or fail, records may be deleted or expire, and retry errors may precede a successful consume. Absence or a single row does not establish final outcome, zero charge or completed refund. Image relay also uses synthetic token counts in some fixed-price paths (`relay/image_handler.go`); those must not become fake public token usage.

#### Recommended corrections and future storage proposal

First assess corrections that use existing columns: add an explicit request ID to `RecordTaskBillingLogParams`, populate `Log.RequestId` from immutable task execution for every linked final consume/refund/adjustment, and keep the returned correlation ID consistent with persisted facts while preserving separate billing idempotency semantics. Persist exact submitting token ownership at task creation, validate unique task-record association before joining events, and fail closed for historical records whose association cannot be proven. These writer corrections do not themselves require new log columns; do not backfill guessed provenance or alter billing formulas to support the read API.

A future full-lifecycle design should evaluate dedicated canonical request ID and submitting token ID columns on task records, an explicit task-record/event association, and a log composite index such as `(user_id, token_id, request_id, created_at)`. Existing individual indexes support secure exact synchronous predicates; an absent composite index alone is not an ownership blocker. Define collision handling, event finality and refund aggregation before adopting uniqueness constraints or exposing a stable endpoint. No migration is performed by this report.

Migration and query risks include ambiguous historical task IDs, absent historical execution/key snapshots, separate main/log databases without cross-store atomicity, cross-dialect JSON extraction, index size/locking during deployment and incompatible backfill assumptions. ClickHouse logs use `MergeTree`, `id DEFAULT 0`, ordering by `(created_at, request_id)` and optional TTL (`model/main.go`); their IDs are not unique lifecycle keys. Existing billing reservation helpers reject ClickHouse (`model/billing_reservation_log.go`), so they cannot be treated as a universal public read abstraction. Avoid unbounded scans of an account's private task JSON as a substitute for indexed canonical association.

Required verification for a future implementation includes disposable real SQLite, MySQL and PostgreSQL instances, plus the separately configured ClickHouse log path; schema changes also require fresh/released-version upgrade and repeated-startup checks. No database migration or request-usage endpoint was implemented or tested for this proposed lifecycle contract; the separate catalog/balance matrix does not certify future request-usage isolation. Effort is medium to high for writer correlation, explicit projection and regression coverage; historical remediation and a durable indexed lifecycle migration are additional work and cannot guarantee reconstruction of missing facts.

Security guidance reviewed: [OWASP ASVS 5.0.0](https://owasp.org/projects/asvs), [Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html), [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) and [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html). Apply server-side per-object authorization, deny-by-default, credential revocation enforcement, indistinguishable foreign/missing results and explicit secret-free output projection. This audit makes no ASVS compliance claim.

### P1 — Public task listing

- **Name / proposed path:** `GET /v1/tasks`.
- **Problem / audience:** Durable-job applications can retrieve known IDs but cannot enumerate their jobs with an API key. Dashboard `/api/task/self` already provides user listing.
- **Internal sources:** `controller/task.go:GetUserTask`, `model.TaskGetAllUserTask`, `dto.TaskDto`; existing public `GetTask` projection.
- **Authentication / response proposal:** API-key read access; bounded pagination, optional status/time filters and public task summaries. Decide and document key-only versus account-wide scope; default proposed scope is key-only. Keep artifact discovery on its existing dedicated endpoint.
- **Version / security:** Additive `/v1` route. Avoid copying dashboard DTO root/admin fields; exclude private snapshots, upstream IDs and provider URLs. Prevent unbounded queries and cross-key enumeration.
- **Effort:** Medium: token association and pagination contract. Priority P1.

### P2 — Capability-aware task cancellation

- **Name / proposed path:** `POST /v1/tasks/:task_id/cancel`.
- **Problem / audience:** Applications need a deliberate way to stop queued/running work. Client disconnection is not durable-task cancellation. No generic public cancel route or host plugin cancellation contract is implemented.
- **Internal sources:** Task state/polling in `service/task.go`, existing owner-scoped task lookup, `pkg/jsplugin/routing.go`, task billing/refund paths. Existing hooks are insufficient for a universal cancellation operation; this needs a new capability and provider-specific implementation.
- **Authentication / response proposal:** Mutating API-key access; owner/key-scoped capability and state checks. Return supported/unsupported cancellation outcome and latest public status; repeated requests and completion races need defined behavior. A cancellation acknowledgement must not promise immediate refund or stopped upstream work.
- **Version / security:** Additive public contract plus explicitly versioned host/plugin capability. Enforce authorization, atomic terminal-state transitions and exactly-once financial adjustment; do not leak upstream keys/IDs.
- **Effort:** High, provider dependent. Priority P2; do not advertise until implemented and verified.

### P2 — Advanced monitoring

Developer monitoring beyond individual task retrieval and request-correlated usage remains a separate P2 proposal. Define the evidence, ownership scope, retention and delivery contract before adding polling summaries or notifications; no monitoring endpoint is implemented in Phase 2.

## OPENAPI_GAPS

`docs/openapi/relay.json` and `docs/openapi/api.json` are useful partial references, not a generated authoritative inventory. Both report `info.version: 1.0.0`, which is not evidence of the running gateway release or a universal public API stability promise.

Observed relay omissions include generic task submit/retrieve/artifact/content, `GET /v1/responses/:response_id`, `POST /v1/videos/generations`, `HEAD /v1/videos/:task_id/content`, `GET /v1/audio/speech/:task_id`, Responses WebSocket and additional registered relay operations. Relay OpenAPI also contains `/v1/video/generations` and its retrieval shape; these singular paths do not match the current plural host registry and need individual reconciliation before reuse.

The management OpenAPI includes `/api/usage/token/` and `/api/log/token`. The latter remains CONSOLE under the explicit documentation scope, although it accepts read-only API keys. Importing management OpenAPI wholesale would misclassify console, administration and setup operations. The four compatibility billing paths are missing from these checked-in documents.

Future OpenAPI work should derive route identity from the full inventory, distinguish placeholders from implemented handlers, and explicitly encode body formats, success/error envelopes, access family, pagination and limits. Cover actual empty-body 429 responses, model limiter errors without `Retry-After`, protocol errors and task state responses. Do not invent headers, idempotency guarantees or retirement dates to fill schema gaps.

## Verification and boundaries

The original read-only documentation audit covered relay/task/dashboard/API route registrations, task protocol registry, quota and log controllers, rate-limit/error emitters, request correlation, task settlement documentation and checked-in OpenAPI path sets. Phase 2's request-usage ownership/storage audit remained read-only and inspected no live user database. No upstream billable request was made. The report does not certify runtime behavior on every deployment or verify future proposals. Catalog and balance implementation verification remains pending above; the request-usage blocker introduced no database, auth or billing changes.

Repository release-note evidence: `README.md` links to [QuantumNous/new-api Releases](https://github.com/QuantumNous/new-api/releases). There is no inferred API sunset policy here. Client guidance should pin deployment versions, inspect actual endpoint capabilities and consult these release notes when upgrading.

## Phase 2 implementation verification

The catalog/balance regression suite ran against real SQLite (bundled runtime 3.50.4), MySQL 5.7.44 and PostgreSQL 15.19 in disposable fixtures. No schema migration or dependency change was introduced. The complete router/middleware run recorded 353 passing test/subtest events, including 79 developer API events, with no failures or skips. It covered key/group/IP/account restrictions, expired/disabled/zero/unlimited keys, model allowlists, current quota cache and revocation, fixed/token/cache/media/variable pricing, missing-versus-zero prices, malformed configuration, existing model discovery and inference rejection, unchanged legacy pricing/usage reads, and credential-free SQL logging.

The full built-in catalog fixture enabled 238 names and projected 234 models. Query count stayed six (one credential lookup intentionally silent, one user lookup, four catalog reads); model count did not add queries. Observed fixture latency was 5.48 ms SQLite, 8.91 ms MySQL and 11.25 ms PostgreSQL. This is a local sanity result, not a production latency guarantee or a live operator catalog audit.

Exact matrix command (DSNs refer only to disposable local fixtures):

```sh
CGO_ENABLED=0 \
PHASE2_MYSQL_DSN='root:phase2-test-only@tcp(127.0.0.1:62947)/phase2?charset=utf8mb4&parseTime=true' \
PHASE2_POSTGRES_DSN='postgres://postgres:phase2-test-only@127.0.0.1:62948/phase2?sslmode=disable' \
go test -p 1 -ldflags='-s -w' ./router ./middleware -count=1 -json
```

Security controls reviewed against ASVS 5.0.0: 6.3.4; 8.2.1–8.2.4; 8.3.1–8.3.2; 14.2.1, 14.2.2, 14.2.5; 16.2.5, 16.3.1–16.3.2 and 16.5.1. Regression coverage verifies server-side access restrictions, key ownership, current revocation, no-store responses, exact non-metered route exceptions and no credential-bearing SQL trace. Existing request-ID/stats/error/limiter conventions are reused; no new successful API-key audit event system or blanket ASVS compliance claim is introduced.

Release configuration still needs an operator-provided `DOCS_SITE_URL`. The environment example explains its frontend canonical/sitemap purpose without guessing a production origin.

Final repository verification: all 87 root-module packages completed in a serial package loop using `CGO_ENABLED=0 go test -p 1 -gcflags=all=-dwarf=false -ldflags='-s -w' -json <package>` for each result of `go list ./...`. This recorded 5,367 passing test/subtest events and zero failures; 66 environment-gated tests were skipped in that broad run. The separately supplied three-database developer matrix had zero skips. Serial per-package execution and omitted debug symbols were required after monolithic Go test linking exhausted disk space; assertions were not disabled. SQLite runtime `select sqlite_version()` returned `3.50.4`. The two disposable database containers were stopped and removed after verification.

Frontend: `bun run test --maxWorkers=2` passed all 2,343 tests in 184 files; `bun run typecheck`, `bun run build` (158 prerendered documentation pages), `bun run docs:check`, DTO/OpenAPI field and reference validation, and changed-files oxlint passed. The browser verifier passed 21 developer pages, 14 domains, 7 locales, 320–1440px widths, light/dark modes, search/expansion/clipboard and no page errors. Full-repository lint still reports the 182 pre-existing errors; changed-file errors are zero. The optional legacy shell-example validator stopped at a pre-existing `jq` requirement because that utility is unavailable; the new read examples were covered by reference/browser checks and direct Gin GET tests.

No paid/provider upstream API calls, dependency/lockfile changes, schema migrations, commits or pushes were performed. The tree remains on baseline `c78d5f465` for review. Six new Go files implement the two controllers, two DTOs, catalog projection and shared public currency projection; existing auth/token/router/helper, OpenAPI, inventory/gap report, environment example, reference/semantics/test/browser/generated metadata and seven locale files were updated.

### Working tree for review

```text
 M .env.example
 M docs/internal/API_INVENTORY.md
 M docs/internal/PUBLIC_API_GAP_REPORT.md
 M docs/openapi/relay.json
 M middleware/auth.go
 M model/token.go
 M relay/helper/price.go
 M router/relay-router.go
 M router/relay_router_test.go
 M web/scripts/verify-documentation-browser.mjs
 M web/src/features/documentation/__tests__/api-reference.test.tsx
 M web/src/features/documentation/__tests__/seo.test.ts
 M web/src/features/documentation/api-reference-data.ts
 M web/src/features/documentation/developer-semantics-data.ts
 M web/src/features/documentation/generated/reference.json
 M web/src/i18n/locales/en.json
 M web/src/i18n/locales/fr.json
 M web/src/i18n/locales/ja.json
 M web/src/i18n/locales/ru.json
 M web/src/i18n/locales/vi.json
 M web/src/i18n/locales/zh-TW.json
 M web/src/i18n/locales/zh.json
?? controller/developer_balance.go
?? controller/developer_catalog.go
?? dto/developer_balance.go
?? dto/developer_catalog.go
?? model/developer_catalog.go
?? service/public_currency.go
```

Six untracked implementation files contain 942 source lines. `git diff --stat` lists the tracked edits separately; no files were staged.
