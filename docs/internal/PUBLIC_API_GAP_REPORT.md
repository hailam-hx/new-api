# Public developer API gap report

This report records the repository's implemented surface and proposes additions. Proposed paths and schemas below are **not implemented contracts**. No backend, authentication, database or billing behavior is changed by the documentation upgrade. Priorities are product recommendations: P0 blocks a safe integration, P1 materially improves developer operations, P2 is optional workflow support.

## Current surface and classification

| Capability | Implemented source and access | Classification / limitation |
| --- | --- | --- |
| Accessible model list | `router/relay-router.go`: `GET /v1/models`, `/v1/models/:model`, `/v1beta/models`, `/v1beta/openai/models`, `TokenAuth` | Public relay API. Names available to a key are not a complete price and capability catalog. |
| Pricing / dashboard catalog | `router/api-router.go`: `/api/pricing` uses `HeaderNavModuleAuth("pricing")`; `/api/models` uses `UserAuth`; `controller/pricing.go` and model metadata handlers | Existing site/dashboard capabilities. Do not reinterpret site navigation access or session auth as a canonical API-key catalog contract. |
| API-key quota totals | `GET /api/usage/token/`, `TokenAuthReadOnly`, `controller/token.go:GetTokenUsage` | Implemented developer-facing read API despite `/api` prefix. Response is `{code:true,message:"ok",data:{object:"token_usage",name,total_granted,total_used,total_available,unlimited_quota,model_limits,model_limits_enabled,expires_at}}`. Totals are internal quota units. |
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

### P1 — API-key catalog

- **Name / proposed path:** Key-scoped catalog, `GET /v1/catalog`.
- **Problem / audience:** SDK and application developers need one key-authorized discovery response for usable models, supported endpoint families and customer prices. Existing model listing plus site pricing does not provide that combined contract.
- **Internal sources:** `controller/model.go`, `controller/pricing.go`, effective model metadata, `pkg/jsplugin/routing.go`, enabled plugin routing. Reuse current public pricing projections, not administrator configuration snapshots.
- **Authentication / response proposal:** API-key read access; model entries containing client-facing name, authorized protocol operations/modes, effective customer pricing unit and configuration/availability state. Add an explicit currency and price basis when a monetary value is returned. Do not expose upstream model mappings, credentials, channel identities or provider procurement prices.
- **Version / security:** New versioned read contract, independently specified before implementation. Distinguish deployment metadata from key access; constrain response size and avoid leaking unavailable models through global catalog caching.
- **Effort:** Medium: key-scoped authorization and pricing/protocol projections need contract design and regression coverage. Priority P1.

Proposed versioned catalog envelope (design only; no route implements this):

```json
{
  "object": "catalog",
  "schema_version": 1,
  "models": [
    {
      "id": "MODEL_ID",
      "provider": null,
      "type": [],
      "endpoints": [],
      "capabilities": {},
      "pricing": [],
      "billing": { "unit": null },
      "availability": { "callable": false }
    }
  ]
}
```

The empty and null values above are schema placeholders, not price or availability data. Each price needs an explicit unit, display currency/points basis and key-scoped effective value. Unknown capabilities must remain unknown rather than being inferred from a name or advertised endpoint family.

### P1 — Explicit key balance and usage summary

- **Name / proposed paths:** `GET /v1/key/balance`, `GET /v1/key/usage`.
- **Problem / audience:** Developer budgeting currently combines quota-unit `/api/usage/token/` totals with display-mode-dependent compatibility billing. The gap is an explicit modern unit/scope contract, not absence of balance queries.
- **Internal sources:** `controller/token.go:GetTokenUsage`, `controller/billing.go`, existing user/token quota getters and customer-facing formatting. A usage-period query requires current consumption-query support; existing cumulative fields must not be presented as arbitrary-period totals.
- **Authentication / response proposal:** API-key read access. Balance distinguishes key allowance from account funding, limited/unlimited state, expiry and unit; usage reports cumulative totals and only supported period filters. Monetary values require currency and scale. Do not represent subscription entitlements as an interchangeable wallet balance.
- **Version / security:** Additive `/v1` contract; retain current compatibility behavior. No-store/private caching; key ownership only; redact account and subscription details not needed by the key holder. Do not expose secrets or internal pricing factors.
- **Effort:** Medium: explicit scopes, units and subscription interactions require design. Priority P1.

### P0 — Request-correlated final usage

- **Name / proposed path:** `GET /v1/usage/requests/:request_id`.
- **Problem / audience:** Developers cannot safely decide final cost or whether timed-out work completed from the client HTTP response alone. `/api/log/token` supplies history, but no request-ID lookup/filter contract. `/api/log/self` has request filters under session auth.
- **Internal sources:** `controller/log.go:GetUserLogs`, `model/log.go` request-ID fields and consumption/refund projections, `service/log_info_generate.go`, `service/task_billing.go`; `middleware/request-id.go` emits `X-Oneapi-Request-Id`.
- **Authentication / response proposal:** API-key read access, filtered to that key's records. Public request ID, model, timestamp, available usage categories, billable unit, final consumption/refund records, currency/scale where monetary, and clearly defined pending/final state when supported. Do not synthesize finality from missing logs; absence, retention and eventual availability need explicit behavior.
- **Version / security:** New `/v1` read contract. Uniform not-found for another key; no prompts, outputs, usable tokens, upstream IDs, channel details or administrator log metadata. Scope is stricter than account-wide `UserAuth` history.
- **Effort:** Medium to high: finality, multiple log events and async reconciliation need a coherent public projection. Priority P0 because retry decisions can duplicate billable work.

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

## OPENAPI_GAPS

`docs/openapi/relay.json` and `docs/openapi/api.json` are useful partial references, not a generated authoritative inventory. Both report `info.version: 1.0.0`, which is not evidence of the running gateway release or a universal public API stability promise.

Observed relay omissions include generic task submit/retrieve/artifact/content, `GET /v1/responses/:response_id`, `POST /v1/videos/generations`, `HEAD /v1/videos/:task_id/content`, `GET /v1/audio/speech/:task_id`, Responses WebSocket and additional registered relay operations. Relay OpenAPI also contains `/v1/video/generations` and its retrieval shape; these singular paths do not match the current plural host registry and need individual reconciliation before reuse.

The management OpenAPI includes `/api/usage/token/` and `/api/log/token`. The latter remains CONSOLE under the explicit documentation scope, although it accepts read-only API keys. Importing management OpenAPI wholesale would misclassify console, administration and setup operations. The four compatibility billing paths are missing from these checked-in documents.

Future OpenAPI work should derive route identity from the full inventory, distinguish placeholders from implemented handlers, and explicitly encode body formats, success/error envelopes, access family, pagination and limits. Cover actual empty-body 429 responses, model limiter errors without `Retry-After`, protocol errors and task state responses. Do not invent headers, idempotency guarantees or retirement dates to fill schema gaps.

## Verification and boundaries

Read-only source inspection covered relay/task/dashboard/API route registrations, task protocol registry, quota and log controllers, rate-limit/error emitters, request correlation, task settlement documentation and checked-in OpenAPI path sets. No upstream billable request was made. The documentation report does not certify runtime behavior on every deployment or verify future proposals. No new database, auth or billing implementation was attempted.

Repository release-note evidence: `README.md` links to [QuantumNous/new-api Releases](https://github.com/QuantumNous/new-api/releases). There is no inferred API sunset policy here. Client guidance should pin deployment versions, inspect actual endpoint capabilities and consult these release notes when upgrading.
