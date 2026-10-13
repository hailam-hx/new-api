# Responses WebSocket contract — Phase 5C

Baseline: `main` at `1fee3b56d`. Audit uses this fork's public router,
`controller/responses_websocket.go`, `relay/responses_websocket.go`, channel
selection, authentication and existing local accounting fixtures. No public
operation or provider protocol was added. Paid/provider upstream calls: **0**.

## Public contract

| Concern | Implemented behavior |
| --- | --- |
| Handshake | `GET /v1/responses`, ordinary WebSocket upgrade after TokenAuth and public system-performance checks. A successful upgrade does not guarantee the first model has a compatible channel. |
| Credential | Explicit API key in `Authorization: Bearer …`; browser clients can offer `openai-insecure-api-key.<API_KEY>` in `Sec-WebSocket-Protocol`. This header is secret. Cookies and query `key` do not authenticate this route. Existing TokenAuth legacy credential behavior is retained. |
| Subprotocol | Optional `responses` or `realtime` negotiated by the shared upgrader; neither required. The credential-bearing protocol is never selected/echoed as the negotiated protocol. |
| Origin | Shared upgrader currently permits cross-origin explicit API-key callers. There is no ambient session-cookie authentication. No new origin allowlist policy is introduced. |
| Authentication lifetime | Each create re-runs TokenAuth, IP restrictions and ordinary model request rate limiting with captured client credentials and a fresh request context; token/user quota and permissions are refreshed. Revoked/expired credentials return a local error and close the connection. |
| Create | Flat `{type:"response.create",model,input,…}` and legacy wrapped `{type:"response.create",response:{model,input,…}}`. `model` required by request processing. Only one active create; another receives `error.status=409`. |
| Stream identity | Optional `stream_id`: 1–256 ASCII letters, digits, `_`, `-`, `.`. Top-level identity takes precedence over wrapped identity. Local errors preserve it. |
| Normalization | Envelope fields removed from the upstream HTTP-shaped request. Upstream create is flat; `event_id`, `background`, `stream`, `stream_options` removed. `generate` preserved without local type restriction. `max_output_tokens` accepted from 0 through 1,073,741,823 with explicit zero preserved. Existing request preparation validates mapped/overridden requests. |
| Cancel | `response.cancel` only while a create is active. Pending control waits for upstream acceptance; at most one pending control. Inactive/unknown controls receive local status 400. Upstream cancellation remains capability-dependent; it does not create a universal task cancellation/refund API. |
| Server messages | Forward upstream frames, with ordinary Responses events and usage accumulation. No exhaustive provider event enum is promised. Local gateway errors are `{type:"error",status,error,event_id?,stream_id?}`. Correlated control errors may leave the generation active. |
| Terminal events | Recognized: `response.completed`, `response.done`, `response.incomplete`, `response.failed`, `response.cancelled`, `response.canceled`. Repeated previous terminal/error IDs do not complete a subsequent request. |
| Client limit | `MAX_REQUEST_BODY_MB` MiB; nonpositive fallback 128 MiB. Oversize rejected before request admission, close 1009. |
| Upstream limit | `STREAM_SCANNER_MAX_BUFFER_MB` MiB; fallback `DefaultMaxScannerBufferSize`, actually 128 MiB. Historical 64 MiB comment was inaccurate. |
| Write bound | 30 seconds for upstream/client data writes. Channel-policy close controls use a one-second write deadline. |
| Idle bound | `STREAMING_TIMEOUT` seconds; nonpositive fallback 300 seconds. Active request uses its existing upstream idle timer. Phase 5 adds the same bound while waiting for the first/next create. Data-message activity refreshes the inactive timer; protocol ping/pong does not substitute for application activity. |
| Close | Normal client close echoed as 1000; oversized client message 1009; channel disable/delete policy 1008. Other shutdowns, timeout, upstream disconnect and auth closure may appear as abnormal 1006, rather than a guaranteed application close code. |
| Retry | Existing channel selection retries only before a create is sent. No automatic resubmission after create write. There is no WebSocket idempotency, resume or replay guarantee. An ambiguous paid create must not be blindly resubmitted. |

Availability is **conditional**: enabled channel, authorized model, explicit
`ResponsesWebSocketEnabled`, eligible group/path and a compatible native
Responses upstream. OpenAI-compatible, NewAPI, Sub2API and native AdvancedCustom
Responses routes can qualify. Converter routes, incompatible providers and
unverified DFLOP profiles are rejected. Reuse locks the model/channel/upstream
credential and rechecks that they remain enabled and permitted.

## Hardening and billing

Previously a caller could retain an authenticated connection indefinitely without
a first create, or between creates. A session timer now closes inactive sessions
using the existing operator timeout. It is joined by the existing worker
WaitGroup and exits on context cancellation. Active calls retain their existing
timeout, stream-health classification and accounting; the new idle watcher does
not settle or refund independently.

Socket close cancels the request context, closes both peers, unregisters channel
policy callbacks and waits for request/reader/timer workers. Request body storage
is cleaned by the ordinary middleware. A request worker exclusively owns billing:
initial upstream rejection refunds its reservation; accepted response terminal,
timeout and disconnect finalize observed usage exactly once using existing
settlement rules. Multiple successful creates have separate reservations and
consume logs. Invalid/unknown client messages do not submit paid work or charge.

The existing fixture cleanup now also asserts that closing a channel finds no
stale registration after the handler finishes. No production DB schema, query,
transaction or migration behavior changed. SQLite is used by the existing local
accounting fixture; the three-database persistence verification requirement is
not triggered by this WebSocket-only change.

## Local regression evidence

Commands actually run:

```sh
go test ./relay -run '^TestResponsesWSIdleSessionClosesAndReleasesHandler$' -count=1
go test ./controller ./relay -run 'Responses.*(WebSocket|WS|Interrupted)' -count=1
go test -race ./controller ./relay -run 'Responses.*(WebSocket|WS|Interrupted)' -count=1
go test -race ./relay -run '^TestResponsesWSIdleSessionClosesAndReleasesHandler$' -count=3
go test ./controller ./relay ./pkg/wsmanager -count=1
node web/scripts/verify-public-asyncapi.mjs --tool-root /tmp/hotx-phase5-asyncapi
node web/scripts/verify-public-asyncapi.mjs --install
git diff --check
```

Idle regression **RED**: both no-message and after-message sessions retained their
handler beyond the configured bound. After the timer fix, **GREEN**, including
three repeated race-enabled runs. Targeted controller/relay suites pass with the
race detector. Complete controller, relay and wsmanager package suites also pass.
A fixture configuration race found during validation was fixed by
setting its timeout before opening the socket rather than changing the global
timeout after handshake.

| Required cases | Evidence |
| --- | --- |
| Handshake, invalid/expired key | `TestResponsesWebSocketHandshakeRequiresExplicitAPIKey`: Bearer/browser protocol success, missing/invalid/expired/cookie/query rejection; normal close and handler release. |
| Revocation, IP/model permissions | Existing request-runner revocation tests and token model access tests; per-create freshness and body cleanup verified. |
| Unsupported channels | Existing explicit-opt-in, native-route, converter/provider rejection and pre-routing tests; no upstream submission on rejection. |
| Valid/multiple messages | Existing reused-connection tests verify model mapping, request identity, channel/key reuse and one settlement per create. |
| Invalid JSON/unknown message | New `TestResponsesWebSocketInvalidMessagesDoNotSubmitOrCharge`: local 400, connection remains usable, later valid create charges once. |
| Size | Existing read-limit test asserts 1009 before admission. |
| Server errors/control attribution | Existing initial rejection/refund, control error, ambiguous error, unrelated/previous-response error tests. |
| Client/server/abrupt close | Existing disconnect and stream-outcome fixtures verify both peer completion and exact balance/consume-log outcomes; new handshake test verifies normal 1000. |
| Active/inactive idle timeout | Existing interrupted-stream health/accounting test plus new inactive-session closure regression. |
| Cleanup/repeated close | Existing busy-writer shutdown test, fixture handler/upstream completion and no stale wsmanager callback; idle test repeated three times under race detector. |

These are real local HTTP/WebSocket peers and existing local billing storage,
not provider tests or proof that any production model supports WebSocket.

## AsyncAPI and SDK decision

**ADOPTED**: [public.yaml](../asyncapi/public.yaml), AsyncAPI **3.0.0**, standard
WebSocket binding **0.1.0**. It documents only the existing Responses connection.
OpenAPI remains canonical for HTTP. Model/channel availability stays conditional;
no webhook channel is invented. Upstream event bodies intentionally remain open,
since this gateway forwards provider frames rather than owning every event.

The validator installs pinned `@asyncapi/parser@3.4.0` in a temporary directory
and removes it afterward; no application dependency or generated SDK is added.
It rejects nonlocal references before invoking a resolver, validates the complete
AsyncAPI document, then validates five payload examples using bundled AJV.
Parser 3.4.0 alone accepted an invalid v3 payload example; explicit example
validation was added after observing that failure. Four temporary mutated
specifications were rejected: unresolved ref, invalid operation action, invalid
payload example and remote ref. Canonical spec passes; the only diagnostic is an
informational recommendation for AsyncAPI 3.1.0. Keeping validated 3.0.0 avoids an
unnecessary version change.

Python/TypeScript generated REST smoke does not establish WebSocket support.
WebSocket readiness is **MANUAL_RECIPE**, subject to the separate Phase 5 SDK
fixture verification. Examples must use explicit API-key authentication and the
actual create/error/terminal envelopes above.

## Security references and limits

Reviewed current primary [Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html),
[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html),
[WebSocket Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html),
and the [ASVS project](https://owasp.org/projects/asvs), whose current stable
release is **5.0.0**. Applicable controls considered: explicit authentication,
per-message authorization, expiry/revocation, bounded input and idle resources,
secure transport and exclusion of usable credentials from diagnostics. This is
focused verification, not a claim of complete ASVS conformance.

Operator TLS termination is required for production. Browser subprotocol key
exposure, unrestricted Origin and deployment-level connection/rate controls
remain existing policy considerations. No new exhaustive provider message
schema, universal cancellation, credential rotation during an active generation,
or WebSocket duplicate-submit protection is promised.
