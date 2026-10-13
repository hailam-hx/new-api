# Task event delivery — Phase 5F

**Status: DESIGN_ONLY / GAP_IDENTIFIED.** This report proposes a future task-event
delivery contract. It adds no registration endpoint, background sender, public
event channel, OpenAPI webhook, or AsyncAPI webhook channel.

## Existing behavior and gap

| Evidence | Current behavior |
| --- | --- |
| [`service/webhook.go`](../../service/webhook.go) | Existing outbound user notification webhook: JSON type/title/content/values/timestamp, optional HMAC-SHA256 over raw payload in `X-Webhook-Signature`, one HTTP attempt and non-2xx error. No stable event/delivery identifier or durable retry queue. |
| [`service/user_notify.go`](../../service/user_notify.go), [`service/notify-limit.go`](../../service/notify-limit.go) | Select email/webhook/Bark/Gotify and apply existing per-user/type notification limits. These are notification controls, not durable task delivery guarantees. |
| [`controller/user.go`](../../controller/user.go), `UpdateUserSetting` | Authenticated user notification URL/secret settings. URL syntax validation is not a receiver ownership proof. |
| [`service/quota.go`](../../service/quota.go), [`relaykit/dto/notify.go`](../../relaykit/dto/notify.go) | Existing quota warning and channel notification use cases. This is not a versioned `balance.low` subscription contract. |
| [`router/api-router.go`](../../router/api-router.go) | Stripe/Creem/Waffo payment callbacks are inbound provider integrations, not customer task-event subscriptions. |
| [`service/task_polling.go`](../../service/task_polling.go), `finalizeTerminalTask` | Persisted terminal task transitions and existing settlement/refund path. No customer task-event outbox is emitted here. |

The existing sender is unsuitable as an implicit v1 task-delivery implementation:
signing is optional, signature has no independent delivery timestamp/replay
window, retry state is not durable, and event identity/ownership/versioning are
missing. The worker path additionally sends the secret as `Authorization: Bearer`
and relies on worker-side egress enforcement instead of local URL validation.
These observations do not change that existing notification contract.

Initial proposal includes only **`task.completed` and `task.failed`**. Progress,
balance and model-availability subscriptions are deferred to avoid noisy events
and unnecessary public surface. A terminal status event must not claim that
canonical settlement/refund history is available: Phase 3D remains
**BLOCKED_BY_OWNERSHIP_MODEL**.

## Registration and ownership — proposed

Keep endpoint registrations distinct from notification settings. A minimal future
registration contains an opaque endpoint ID, authenticated owner user/token
identity, HTTPS URL, allowed event types, verification/enabled state, signing key
versions and creation/update timestamps. Store secret material encrypted under
operator-managed keys; return a newly generated secret only at creation/rotation.
Store no raw API key. Namespace registration management and deliveries by the
authenticated principal, never by an unverified body owner field.

For v1, prefer **token-owned task subscriptions**: only tasks attributed to that
token and user may be projected to its receiver. Existing
[`controller/developer_tasks.go`](../../controller/developer_tasks.go) and
[`model/task.go`](../../model/task.go) show user/token-scoped task projections and
private task token provenance. Legacy tasks with missing attribution require an
explicit conservative exclusion; do not infer cross-token ownership. A future
user-wide subscription would need separate authorization and product approval.

Verify receiver control with a short-lived, single-use random challenge sent
through the same protected egress path, bounded response and no redirects.
Challenge verification must neither disclose task content nor prove ownership of
unrelated endpoints. URL changes revoke prior verification. Endpoint create,
update, secret rotation, disable and replay require server-side ownership checks;
cookie-authenticated management additionally requires CSRF protection and
appropriate re-authentication for sensitive changes.

## Event and delivery identity — proposed

Persist an immutable event envelope:

```json
{
  "id": "opaque-event-id",
  "type": "task.completed",
  "version": "1",
  "created_at": "UTC RFC3339 timestamp",
  "data": {"task_id": "public-task-id", "status": "completed"}
}
```

The example is a design shape, not an available production event. Generate a
distinct event ID once at the persisted logical terminal transition. **Task ID
is not event ID**: a task can have multiple event types/versions. Persist the
occurrence time independently from delivery-attempt time. Event version names the
envelope/projection schema and is independent of plugin/API path versions.

Each endpoint/event pair receives a stable delivery ID; retries retain event ID,
delivery ID and exact body bytes. Individual attempts have distinct internal IDs.
Use a portable unique event-transition identity and endpoint/event uniqueness.
A future transaction/outbox must atomically bind winning task transition to event
creation, covering both background polling and immediate completion. The current
CAS winner followed by finalization is an integration point, not an existing
atomic outbox. Do not emit from an uncommitted observer or couple billing success
to outbound delivery success. Recovery must reconcile committed transitions
without duplicating events. New storage requires the full three-database matrix.

Delivery is at least once; no exactly-once network claim. Receivers should verify
the signature and atomically deduplicate event IDs before applying effects. An
acknowledgement is a bounded 2xx HTTP response, independent of response body.
Ordering across tasks/endpoints is not guaranteed. Duplicate delivery must never
resubmit a paid provider operation or perform billing/refund.

## Signature, replay and rotation — proposed

Use existing cryptographic family **HMAC-SHA256**, with a dedicated random
high-entropy per-endpoint secret. Do not use an API key, channel key or payment
provider secret. Proposed signature metadata contains algorithm/version, signing
key ID, delivery-attempt Unix timestamp and signature. Sign exact bytes:

```text
HMAC-SHA256(secret, ASCII(timestamp) + "." + raw_request_body)
```

Timestamp is signed; body includes immutable event ID/version. Receivers compare
signatures in constant time, use raw bytes before JSON parsing, reject unsupported
versions/key IDs and timestamps outside the documented skew/replay window, then
deduplicate event ID. A recommended starting window is five minutes, explicitly
**DESIGN** pending operator clock policy. Every retry/replay gets a fresh delivery
timestamp/signature without changing the event. Never transmit the signing secret
as a Bearer credential, URL argument or log field.

Rotation uses a new key ID and separately generated secret, a bounded overlap
for receiver rollout, and explicit retirement. New attempts use the current key;
consumers may accept the retired key only during the configured overlap.
Emergency compromise revokes immediately and pauses delivery until receiver
configuration is ready. Store audit metadata about rotation, not secret values.

## Bounded delivery lifecycle — proposed

Use a durable worker with atomic leased claims and lease expiry. Bound total
attempts, maximum event age, connect/TLS/header/body/total request time, response
bytes and per-endpoint/global concurrency. Retry transport failures, 408, 429 and
transient 5xx with jittered exponential backoff; cap any accepted Retry-After by
the delivery budget. Treat redirects as failed delivery. Do not blindly retry
other permanent 4xx; repeated authentication failures should pause the endpoint
and require operator/owner correction. Exact schedule, retry budget, event
retention and dead-letter retention remain product decisions, not promises.

After exhaustion/permanent failure, retain a bounded dead-letter record with safe
reason, last status, attempt counts and event/delivery IDs. Owners may inspect and
explicitly replay retained events after correcting/verifying the endpoint; replay
is audited, rate limited and preserves event identity for deduplication. Never
regenerate history from arbitrary provider payloads or imply indefinite replay.

Disable prevents new dispatch/lease acquisition immediately, cancels outstanding
attempt contexts where possible and defines disposition of queued deliveries.
An already transmitted request cannot be retracted. Deleted/disabled endpoints
must not silently continue retrying. Failed delivery does not change task outcome.

## SSRF and privacy — required before implementation

HTTPS-only URLs, strict parsing, no userinfo/fragments, bounded URL/hostname,
approved ports and valid TLS certificate verification. Reject localhost,
loopback, private/link-local/unspecified/multicast/reserved destinations, IPv4
mapped IPv6 bypasses, metadata endpoints and prohibited network ranges. Perform
the same validation during registration/challenge and every attempt/replay.
Resolve all usable A/AAAA results, reject prohibited addresses and dial only
validated IPs while preserving original hostname for TLS SNI/certificate checks.
Do not allow a second DNS resolution to bypass validation; pin the checked
address for that attempt and re-resolve safely next attempt. Do not follow
redirects; URL change requires fresh verification. Apply egress network policy as
defense in depth. Worker/proxy routing must provide equivalent enforcement or be
disabled for this feature.

[`service/protected_fetch_client.go`](../../service/protected_fetch_client.go)
already offers dial-time address checks and checked-IP dialing when enabled and
domain IP filtering is active. [`service/http_client.go`](../../service/http_client.go)
allows global protection to be disabled and follows validated redirects; the
new sender needs mandatory policy independent of those permissive settings,
bounded timeouts and no insecure TLS inheritance. Existing remote proxy/worker
transport needs separate verification before reuse. Guidance follows the
[OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html).

Project only public task ID, model if appropriate, safe terminal status/time and
generic failure code. Reuse the safe developer-task projection rather than raw
task JSON. Exclude prompts, uploaded bytes, raw provider errors/results, provider
URLs containing secrets, internal task IDs/channel IDs/node names, balances,
quota settlement claims and credentials. Owners can retrieve authorized detail
through existing APIs. Delivery logs record endpoint/event/delivery IDs, safe
destination host, attempt time, latency, status and bounded failure category;
exclude raw bodies, signatures, secrets and unbounded receiver responses. Apply
per-owner endpoint caps, event/attempt rate limits, global egress budget and
retention/cleanup policy; notification limits alone are insufficient.

## Implementation gate

Before any future public contract: approve ownership, envelope/version, replay
and retention policies; verify atomic outbox across immediate/background paths;
test lost acknowledgements, duplicate terminal transitions, worker crashes,
expiry, lease recovery, key rotation, replay, cross-token isolation, disable,
DNS rebinding, redirects and IPv4/IPv6/private-network rejection using local
fixtures. Run SQLite/MySQL/PostgreSQL fresh/upgrade/idempotency verification for
new persistence. Only implemented delivery should appear in OpenAPI/AsyncAPI;
choose one canonical event schema source to avoid competing copies.
