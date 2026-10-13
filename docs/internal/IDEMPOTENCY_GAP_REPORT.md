# Idempotency gap report — Phase 4F

Status: **AUDIT_COMPLETE / DESIGN_ONLY**. Baseline: `main` at `efb670890`. This report adds no backend behavior or public operation. Phase 3D remains **BLOCKED_BY_OWNERSHIP_MODEL**; request identifiers and reservation journals do not establish a public request-correlated final-usage contract.

## Actual contract

There is **no gateway-wide same-result idempotency contract**. However, saying that `Idempotency-Key` is wholly unsupported would also be incorrect: DFLOP task paths and claimed asynchronous speech validate/use this header. These narrower mechanisms must not be advertised as replaying the original HTTP response or task.

| Mutation family | Implementation evidence | Actual retry/deduplication behavior |
| --- | --- | --- |
| Chat/completions, Claude messages, Responses/compact, alpha search | `router/relay-router.go:101–121`; Responses protocol dispatch: `router/task-plugin-protocol-router.go:29–36` | Ordinary relay has request-local channel retry, no gateway client-key lookup or saved-response replay. A second client call can create new work and charges. Plugin Responses follows task submission rules below. |
| Image generation/edit, legacy `/v1/edits` | `router/task-plugin-protocol-router.go:37–44`; `router/relay-router.go:127` | Relay fallback has no general deduplication; task image can continue after timeout/disconnect. DFLOP image has the narrow journal barrier below. |
| Video create `/v1/videos`, `/v1/videos/generations`, `/v1/video/generations`; remix | `router/task-plugin-protocol-router.go:56–61`; registry bindings in `pkg/jsplugin/routing.go:92–97`; `router/video-router.go:15–31` | New submissions can create new paid jobs. Remix is a mutation, not a retrieval. DFLOP media has narrow reservation protection. |
| Speech, transcription, translation | `router/task-plugin-protocol-router.go:45–55`; `router/relay-router.go:137–144`; `middleware/task_plugin.go:616–630` | Synchronous relay has no general deduplication. Claimed async speech requires a key (400 if absent/invalid), but that requirement alone gives no same-result replay guarantee. |
| Generic task submission `/v1/tasks/{plugin_key}` | `router/task-router.go:15–18`; `controller/relay.go:469–745`; `relay/relay_task.go:200–219` | Fresh task IDs, billing/persistence lifecycle and request-local retries; no universal client operation record. GET list/detail/artifact operations are reads. |
| Installed native plugin submit/dynamic routes, including video, image, music, voice cloning, avatar and other paid actions | `router/plugin-router.go:54–76,83`; manifests `plugins/tasks/*/plugin.js`; DFLOP examples `plugins/tasks/dflop-media/plugin.js:108–115`, `plugins/tasks/dflop-image/plugin.js:29–31` | Routes depend on installed/enabled plugin and deployment. They share task submission behavior; no independent general native replay contract. Provider-specific headers are not gateway guarantees. |
| Embeddings, rerank, moderation, Gemini `/v1/models/*`, `/v1beta/models/*`, legacy engine embeddings | `router/relay-router.go:132–161,202–211` | POST may perform billable computation despite being semantically read-like. No universal client-key deduplication. |
| Realtime and Responses WebSocket creation events | `router/relay-router.go:82–95`; `controller/responses_websocket.go` | Socket/event IDs are protocol identifiers, not a durable gateway retry ledger. Reconnect/re-create cannot be claimed exactly once. |
| Midjourney native paid submissions and actions | `router/relay-router.go:216–234` | Compatibility surface outside the canonical `/v1` namespace; no generic deduplication. `/task/list-by-condition` is a POST read and should not be confused with paid submit. |
| Files, fine-tunes, image variations, model delete | `router/relay-router.go:165–176`; `controller/relay.go` (`RelayNotImplemented`) | NOT_IMPLEMENTED placeholders; no paid idempotency promise. Exclude from supported public operations. |

Source references identify the audited baseline revision; line numbers may shift with unrelated Phase 4 edits. The same source search covered all registered static mutation families and the common submit/dynamic plugin dispatcher, rather than assuming only chat, images and videos can incur charges. Console `/api` changes, inbound payment callbacks, runtime verification and canary tooling are outside this public mutation contract.

## Request IDs and internal retries

`middleware/request-id.go:10–17` always generates a fresh request ID and returns `X-Oneapi-Request-Id` (`common/constants.go:188`). It does not accept a caller-supplied request ID as a deduplication key. Task ownership stores the submitting token and canonical request ID (`model/task.go:55–58,300`); this supports ownership/correlation, not replay.

`controller/relay.go:148–218` retries channel attempts within a request. `controller/relay.go:839–875` separately evaluates task retries: `NoRetry` stops; 429/307 and eligible 5xx can retry, 400/408 stop, and exclusions, attempt budget and pinned-channel constraints apply. Successful upstream SSE acceptance disables retry for subsequent reader/parse failure (`relay/channel/task/jsplugin/adaptor.go:460–485`; `docs/plugin-api/v1.md`). These protections do not prove absence of duplicate upstream work after every transport/acceptance ambiguity.

Plugin image timeout is **504 `task_timeout`** while the persisted task can still finish (`controller/plugin_protocol_image.go:182`). Client timeout, connection loss, a 5xx, or an error after upstream acceptance is not proof of no charge. If a public task ID is known, query its owner-scoped status before resubmitting; key-scoped list may help locate a task but cannot prove that an unpersisted upstream submission never occurred. Do not claim balance or aggregate usage identifies that operation's final charge.

## Narrow DFLOP barrier

For `dflop-media`, `dflop-image`, and `dflop-tts`, origin verification restricts the journal contract to HTTPS `api.dflop.top` with no credentials/query/fragment (`service/billing_reservation.go:133–152`). Key validation is 1–200 printable ASCII characters and no surrounding whitespace; speech's earlier middleware additionally prohibits spaces. Invalid/missing task key returns **400 `invalid_idempotency_key`** (`relay/relay_task.go:212–213`); speech middleware returns an OpenAI-style 400 message (`middleware/task_plugin.go:617–625`).

The billing identity hashes **user ID + token ID + plugin + client key**, retains the separate canonical HTTP request ID, and forces pre-consume (`service/billing_reservation.go:154–170`). The identity omits endpoint and request hash. Paid task pre-consume runs only when billing is absent and the model is not free (`relay/relay_task.go:340–345`). `service/billing.go:63–83` claims a durable billing journal; `model/billing_reservation_log.go:33–61` serializes duplicate claims through the user row and rejects an existing journal. Legacy ownership ambiguity and token-owner mismatch fail closed.

A replay raises **`BILLING_RESERVATION_REPLAY_REQUIRES_MANUAL_RESOLUTION`**, via `model_price_error` with the default API error status **500**, carried to task error (`service/error.go:220–229`) by `relay/relay_task.go:343–344` (`relaykit/types/error.go:257`). It does **not** return the saved task/result; there is no payload mismatch **409** contract. The same key across two operations of one plugin collides even when endpoint/body differs. A new key is a new identity and can create another job.

The host forwards the **hashed identity**, not the raw client key (`relay/channel/task/jsplugin/adaptor.go:1323–1328`; `relay/channel/api_request.go:624–633`). DFLOP drivers include it in upstream POST headers (`plugins/tasks/dflop-media/plugin.js:315–319`, `dflop-image/plugin.js:171–174`, `dflop-tts/plugin.js:50–55`). This establishes header forwarding, not verified upstream same-result behavior, hash-conflict semantics or TTL. No paid provider call was made in this audit.

After outbound DFLOP failure the request is non-retryable (`controller/relay.go:583–585`) and uncertain acceptance can hold a **PENDING** reservation (`service/billing_reservation.go:174–197`), rather than convert an estimate to final usage. Normal old-log cleanup excludes reservation records (`model/log.go:753`); there is no declared public replay TTL. DFLOP cache billing also has request-local reservation checks (`service/billing.go:41–57`), but generated HTTP IDs do not deduplicate separate client requests. Non-DFLOP claimed speech can receive a user-scoped header hash (`middleware/task_plugin.go:628–630`); that is not a token/endpoint/payload-bound durable replay store.

## Future proposal — DESIGN_ONLY

Prioritize paid asynchronous video/image/native task submission. A future product decision should define:

- Scope by immutable API-token owner + normalized method/endpoint + client key. Use bounded opaque keys, separate from API-key credentials; never log raw credentials or keys. Revocation/rotation must have explicit ownership rules.
- Persist a canonical request fingerprint covering semantic JSON, query inputs and multipart file digests. Same key/different fingerprint returns **409**; identical requests return the same logical task/result without a second charge. These are proposed semantics, not current responses.
- Atomically claim before outbound submission; handle simultaneous calls, restart, separate log store, provider acceptance uncertainty, and settlement/refund exactly once. A journal rejection alone is insufficient. Store an explicit pending/accepted/terminal state, not a reconstructed consume-log assumption.
- Decide a bounded replay TTL and retention policy before launch. A candidate is at least 24 hours for completed responses and longer than the maximum async completion window for pending work; unresolved acceptance must retain a tombstone until reconciled. Publish exact selected periods only after product/storage approval. Expired-key reuse and task/result retention must be explicit and distinct.
- Bind an upstream idempotency identity where the provider supports it, document and test its TTL/conflict rules, and keep ambiguous acceptance non-resubmittable until reconciled. Providers lacking that contract require a clear limitation; do not promise exactly-once delivery across a network boundary.
- For streaming, prefer replay of a durable task/status reference; do not imply an already emitted SSE stream can resume or replay identically without a designed event retention protocol.

This audit introduces no new backend behavior, DB/schema change or replay TTL. The canonical public spec documents existing mandatory DFLOP headers and conditional host-protocol use; it does not introduce a new idempotency guarantee. Validation was source/route inspection only; no database matrix or paid upstream test was run or claimed.
