# Phase 5A — paid task idempotency implementation

## Result and scope

**Overall Phase 5A: PARTIAL.** The selected shared task-submission pipeline is protected. Its 27 documented public POST operation slots are conditional on a task/plugin-backed implementation; this is not universal idempotency coverage for every gateway branch. Unclaimed built-in Responses background mode, ordinary built-in image/audio/Responses fallbacks and legacy Midjourney submission remain outside the selected scope. A header on an unsupported fallback retains the old behavior and can be ignored; clients must check the documented task-backed capability. A future fail-closed unsupported-header contract needs its own compatibility decision.

Baseline: clean `main` at `1fee3b56d`, observed before Phase 5. No public operation was added or removed; public HTTP coverage remains 107 operations. No commit, staging or push. Paid and external provider requests: **0**. All provider-shaped traffic used local fixtures or transport interception to local fixtures.

The selected slots are:

- `/v1/tasks/{key}`
- `/v1/videos`, `/v1/videos/generations`, `/v1/video/generations`, `/v1/videos/{video_id}/remix`
- `/v1/responses`, `/v1/images/generations`, `/v1/images/edits`, `/v1/audio/speech` when claimed by a task plugin
- `/ali/api/v1/services/aigc/multimodal-generation/generation`
- `/ali/api/v1/services/aigc/image-generation/generation`
- `/ali/api/v1/services/aigc/text2image/image-synthesis`
- `/ali/api/v1/services/aigc/image2image/image-synthesis`
- `/ali/api/v1/services/aigc/video-generation/video-synthesis`
- `/ali/api/v1/services/aigc/image2video/video-synthesis`
- `/dflop-image/v1/images/generations`, `/dflop-image/v1/images/edits`
- `/dflop/v1/videos/generations`, `/dflop/v1/music/generations`, `/dflop/v1/audio/voices`, `/dflop/v1/videos/avatars`
- `/doubao/api/v3/contents/generations/tasks`, `/doubao/api/v3/images/generations`
- `/jimeng/`
- `/kling/v1/videos/text2video`, `/kling/v1/videos/image2video`
- `/suno/submit/{action}`

Native routes remain conditional on enabled plugin/routing generation and retain the project's experimental/beta stability decision. Adding an optional header does not make a plugin or provider universally available.

## Retry and acceptance audit

Client resubmission, internal channel retry and background polling remain distinct:

| Boundary | Behavior |
| --- | --- |
| No key supplied | Existing task retry, billing and presentation behavior is retained |
| First keyed request | Atomically claims its logical identity before submit attempts or billing |
| Known failure before any funding attempt or outbound | The owned unfunded preflight claim is removed, allowing validation/transient failures to try again |
| Funding has been attempted, including a nil returned BillingSession | The claim is retained for reconciliation; partial debits/rollback failure and asynchronous refund completion cannot be disproved by a nil session |
| First possibly paid outbound | A committed `indeterminate` marker must exist **before** `adaptor.DoRequest`; marker failure prevents provider submission |
| Any keyed non-durable failure after funding attempt or outbound marking | First caller receives 409 `idempotency_indeterminate` with `NoRetry`, explicitly warning against a new-key/blind resubmission; this includes transport, parse, cancellation, adjustment and persistence failures |
| Durable task | Task insertion and exact task-row binding commit in one primary-database transaction |
| Duplicate durable request | Retrieves that canonical task row and returns through the existing presenter; no submit, new BillingSession, reservation, settlement, consume log or refund |
| Duplicate active/unknown request | 409; no upstream call or billing operation |
| Background polling/observation | Continues the existing durable task; it is not a resubmission |

The existing accepted SSE `NoRetry` rule is preserved. Keyed submissions conservatively stop internal resubmission once an outbound call may have created paid work, even if no provider task ID was recovered. A funding attempt returning no BillingSession also stops internal retry before a second pre-consume call; an existing successful BillingSession can retain its established pre-outbound reuse behavior. Unkeyed Request Policy routing/affinity/retry behavior was not bypassed.

## Identity, fingerprint and privacy

`Idempotency-Key` accepts exactly one header value: 1–200 printable ASCII characters without surrounding whitespace. The 200-character bound preserves the existing DFLOP header limit. The value is not a secret or a bearer credential. Multiple values, empty values, controls, non-ASCII and surrounding whitespace are rejected before any claim/upstream work.

Identity is `(token_id, SHA-256(HTTP method + concrete request path), SHA-256(client key))`. Concrete paths include remix IDs and native route parameters, so independent operations do not share a claim. User ownership is checked in addition to token identity. Different tokens belonging to one user and different users with their own tokens have independent namespaces. No raw API credential, Authorization header, request body, upload, raw client key or arbitrary provider response is stored in the idempotency row.

Fingerprint includes method, concrete path, ordered repeated query values, the exposed `Accept` header, supported media type and its parameters, and semantic body. JSON default `charset=utf-8` is normalized case-insensitively; multipart excludes only its transport boundary. Other media-type parameters remain bound, preventing replay across different JSON profiles:

- JSON ignores object-member order and insignificant whitespace; decoded duplicate member names, including escaped duplicates, are rejected. Numeric identity uses compact exact coefficient/exponent normalization: `1` and `1.0` match, while adjacent integers above 2^53 remain distinct. It never rounds through float64 or expands enormous exponent values.
- An iterative string/escape-aware nesting check runs **before** recursive JSON validation. JSON limits are 16 MiB, 32 container levels, 65,536 nodes, 128-byte numeric lexemes and explicit exponents from −4096 to +4096. Unsupported representations fail before claiming/billing.
- Multipart ignores the transport boundary, sorts independent field names and preserves repeated values/files within each field. It binds form values, file names/media types/sizes and streamed SHA-256 file-byte digests. `BodyStorage.NewReader` gives an independent cursor; the upload is not stored in the idempotency table. Limits are 128 MiB body, 128 parts, 1 MiB per ordinary field, 256-byte field names and 1024-byte filename/media-type metadata. Invalid UTF-8 metadata/field values is rejected. File-name-only hashing is not used.
- Unsupported media types and unsafe/unbounded representations return 400 `idempotency_request_unsupported`; no weak fallback fingerprint is created.

Safe decision logging records `new`, `replay`, `conflict`, `claimed` (in progress) and `indeterminate`, without raw key/body/credential fields. No new metrics framework was added.

## Storage, concurrency and lifecycle

`TaskIdempotency` stores primary row ID, user/token IDs, three 64-character hashes, a random 256-bit ownership nonce, state, exact task-row reference and timestamps. A portable unique index on token/operation/key protects concurrent claims. Claim ownership is determined by inserting with conflict handling and reading back the nonce, **not** by `RowsAffected`; this remains safe under MySQL `clientFoundRows=true`.

States are `claimed`, `indeterminate` and `durable`. The durable task itself supplies current completed/failed state. Exact `ResourceRecordID` lookup prevents task-ID alias/duplication from substituting another resource. Replay additionally verifies user, token, canonical ownership and retained result. Contradictory provenance, deleted/unavailable task rows and discarded immediate results fail closed with 409 `idempotency_replay_unavailable`; they do not start paid work again.

Replay is read-only with respect to task state. In particular, Responses captures `background` in private data before initial insertion and does not rewrite a stale `private_data` snapshot on replay, protecting concurrent poll state.

Terminal retention is at least configured `TASK_TIMEOUT_MINUTES` (minimum existing 24h task lifetime) plus a 48h retry window: 72h at defaults. A terminal binding can expire only after that deadline **and** at least 48h after terminal finish. Active/missing/unknown resources never expire automatically. Master-only hourly cleanup uses the existing cleanup pattern and deletes only eligible terminal bindings.

**Abandoned `claimed` records are deliberately retained indefinitely.** Billing pre-consume runs before outbound marking; a process crash in that interval can leave held funding even though the upstream was not reached. Nonce CAS alone cannot prove a refund occurred, so expiring/reacquiring that claim could create a second reservation. Only failures before any funding attempt can release a live preflight claim. An explicit in-memory funding-attempt flag is set before `PreConsumeBilling`, because partial pre-consume failure can return a nil session. A created session also prevents release: the existing refund runs asynchronously, so its invocation is not proof that funding was returned. The existing refund lifecycle remains responsible for refunds; no new refund layer was added. Abandoned claimed/indeterminate records require operator reconciliation; there is no new public cancellation or reconciliation endpoint. An expired original actor cannot pass the outbound marker. This trades availability for billing safety and leaves Phase 3D final ownership/settlement history blocked.

## DFLOP compatibility

Existing pre-Phase-5 user/plugin/key and token/plugin/key journal identities are checked before a new scoped operation proceeds. An own-token or legacy token-zero journal fails closed for manual reconciliation; a proven foreign positive token does not deny the current token's independent namespace. The separately configured log database is honored.

For a new scoped claim, DFLOP billing/provider identity derives from the record ID and ownership nonce. The existing adaptor injects that scoped `RequestId` into plugin `requestHeaders["Idempotency-Key"]` before `buildSubmitRequest`; the raw client header/fingerprint is unchanged. Actual plugin descriptor tests verify distinct provider identities across operations/tokens and identical identity for one claim. Legacy requests outside this new claim path retain their old journal behavior. Canonical HTTP request ID remains separate and unchanged.

## Errors and authentication

Host-owned errors are preserved through native and Responses presentation rather than being reclassified by a provider renderer:

| HTTP | Code |
| --- | --- |
| 400 | `invalid_idempotency_key`, `idempotency_request_unsupported` |
| 409 | `idempotency_key_conflict`, `idempotency_in_progress`, `idempotency_indeterminate`, `idempotency_legacy_indeterminate`, `idempotency_replay_unavailable` |
| 503 | `idempotency_storage_unavailable` |

Messages are static and contain no cross-principal existence details or provider/database internals. Native/general task envelopes and OpenAI protocol error envelopes retain their established shapes.

Authentication was not bypassed or changed. Invalid, expired, revoked and exhausted/zero-allowance API keys still fail normal `TokenAuth` with 401 before any idempotency record/provider submit. This includes exhausted-key replay; no general quota bypass was introduced. Existing IP/group/model restrictions still apply. A zero wallet fails the existing pre-consume gate; once funding was attempted its claim is conservatively retained rather than allowing another reservation. Security guidance reviewed: [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) and [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html); validation concerns are server-side identity, rejected credentials before mutations, principal isolation and non-secret errors. This is not a blanket ASVS compliance claim, and no session/login mechanism changed.

## Verification

All regressions use explicit local fixtures. One new backend test file consolidates fingerprint/model/database/legacy-descriptor cases; existing controller and native protocol fixtures were extended rather than scattering new test files.

Latest commands and observed results after the abandoned-claim safety correction; the targeted and core race checks additionally include the final funding-attempt and exposed-header fingerprint changes:

```sh
# Disposable task-owned databases; DSNs are supplied in environment, not committed.
PHASE5_MYSQL_DSN="<local MySQL 5.7.44 DSN including clientFoundRows=true>" \
PHASE5_POSTGRES_DSN="<local PostgreSQL 15.19 DSN>" \
go test ./model -run TestTaskIdempotency -v -count=1

go test ./controller ./model \
  -run 'Test(TaskIdempotency|ImmediateTaskSettlementDatabase|DFLOPAudioSpeechProduction|BackgroundResponseIdempotency|RespondPluginProtocolSubmissionError)' \
  -count=1

# SQLite only: no PHASE5_MYSQL_DSN/PHASE5_POSTGRES_DSN supplied.
go test -race ./model ./controller -parallel=1 \
  -run 'Test(TaskIdempotency|BackgroundResponseIdempotency|RespondPluginProtocolSubmissionError)' \
  -count=1
```

- Real database matrix: **PASS**, 1.763s. SQLite **3.50.4**; MySQL **5.7.44**, explicitly `clientFoundRows=true`; PostgreSQL **15.19**. Logs: `/tmp/phase5-idempotency-db.log`.
- Final targeted controller/model suite: **PASS**, controller 1.341s and model 0.641s. Logs: `/tmp/phase5-idempotency-targeted.log`.
- Final SQLite core idempotency race suite: **PASS**, model 2.677s and controller 2.652s. Logs: `/tmp/phase5-idempotency-core-race.log`.
- Each database verifies empty fresh startup twice, representative pre-feature task-schema upgrade twice with preserved task data, eight simultaneous atomic claims, operation/token/user isolation, sequential replay/conflict, expired terminal reuse, active/indeterminate/abandoned claimed retention, marker DB-write failure, task+binding transaction rollback, and independently configured log-store handles with separate prefixes (SQLite also separate physical file).
- Preflight regression distinguishes retryable unfunded validation failure from funded failure and failed funding with no session: the latter two retain the claim, return a safe first-response warning, and permit neither a second reservation nor internal pre-consume retry.
- Controller concurrency pins one submit, one reservation, one settlement and one durable row. Real BillingSession/loopback provider fixtures additionally verify one provider POST, one wallet deduction/consume log and no new billing session across eight immediate success/failure/refund/zero/fallback-usage cases.
- Real native DFLOP TTS production path verifies upstream scoped header, durable replay without another submit/reservation, polling settlement/refund and accepted lost-response indeterminate behavior. Shared Video path and Responses background duplicate/conflict/error presentation are verified. Background replay performs no durable task write and preserves poll state.
- JSON/multipart fingerprint cases verify exact numeric normalization, key reordering, duplicate-member rejection, exponent/depth bounds, quoted braces/escaped quotes, boundary independence, file-byte differences, `Accept` differences and non-default JSON profile conflicts. Exact task ownership/discarded-result behavior uses the existing canonical task guard.
- Real `TokenAuth` followed by the shared executor verifies invalid/expired/revoked/exhausted credentials return 401 with idempotency-record count 0 and provider-submit count 0.
- Optional broader race run exposing the existing real-billing fixture's global-cache cleanup race: **FAIL**, `TestImmediateTaskSettlementDatabase`; fixture cleanup restores cache flags while the existing asynchronous `IncreaseUserQuota`/`cacheIncrUserQuota` worker reads them (`controller/relay_task_plugin_test.go:527`, `model/user.go:1353`, `model/user_cache.go:144`). Log: `/tmp/phase5-idempotency-race.log`. This failure is not presented as a passing full race suite; core idempotency race verification is recorded separately.

The full root regression and release validation are recorded in the final Phase 5 report after the final source freeze. Unresolved general streaming/logger/cache race limitations remain explicit. This change adds no billing formula, independent refund layer, generated SDK patch, webhook/status backend or final-usage ownership endpoint.
