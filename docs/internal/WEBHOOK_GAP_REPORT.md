# Webhook/event gap report — Phase 4G

Status: **AUDIT_COMPLETE / DESIGN_ONLY**. Baseline: `main` at `efb670890`. No public webhook infrastructure or event operation is introduced. Phase 3D remains **BLOCKED_BY_OWNERSHIP_MODEL**.

## Existing surfaces and boundaries

There is no ready public developer subscription/delivery contract for `task.completed`, `task.failed`, final billing, key balance, or model availability. Existing webhook-related code is real, but serves different boundaries:

| Surface | Evidence | Actual behavior and boundary |
| --- | --- | --- |
| Inbound payment callbacks | `router/api-router.go:65–70,90–91,202–205` | Stripe, Creem, Waffo, Waffo Pancake and Epay notify/return paths accept payment-provider callbacks under `/api`. They are console payment integrations, not customer event subscriptions. Provider verification and payment deduplication do not define task-event delivery. |
| Outbound user notifications | `service/user_notify.go:50–87`; `service/quota.go:495–515,547–564`; `service/channel.go:44,53`; `service/user_notify.go:24–47` | User settings select email/webhook/Bark/Gotify. Quota alerts, channel state/test notifications and opt-in upstream model updates target account/admin notification preferences. They are not key-scoped developer events or an authoritative per-request billing feed. |
| Notification payload/signature | `service/webhook.go:18–30,42–49,71–76,103–106` | JSON fields are `type`, `title`, `content`, optional `values`, Unix `timestamp`. Optional separate `WebhookSecret` generates hex HMAC-SHA256 over serialized payload in `X-Webhook-Signature`. Worker mode additionally sends that secret as `Authorization: Bearer`; direct mode does not. No stable event ID or signing-key ID is present. |
| Delivery and rate limit | `service/webhook.go:77–119`; `service/user_notify.go:56–64`; `service/notify-limit.go` | One send attempt accepts any 2xx and otherwise returns an error. Hourly notification limiting is not retry/backoff, durable outbox, acknowledgment tracking, replay retention or redelivery. |
| Plugin task runtime/polling | `docs/plugin-api/v1.md` (Polling contract, host protocols); `router/task-router.go:18,24–36` | Host polls provider state; public clients can retrieve owner-scoped tasks/artifacts. JavaScript hooks process submit/query/result; they are not developer webhook receivers or subscriptions. SSE response events are transport output, not durable task webhooks. |
| Midjourney notify hook | `router/relay-router.go:227`; `service/midjourney.go:317–329` | Notify route is commented out; certain request processing removes `notifyHook`. Neither establishes a current public completion-event guarantee. |

Direct notification sending applies SSRF URL validation and protected HTTP client (`service/webhook.go:90–110`). Worker mode delegates outbound access to the configured worker; any future public webhook product must verify equivalent destination/redirect/DNS controls there rather than infer them from the direct branch. Existing optional signatures, account preferences, timestamps and alert types do not establish required authentication, event replay protection, rotation or retention for a new developer contract.

## Missing public contract

- No key/endpoint-scoped subscription registration, validation or ownership lifecycle in the public route families.
- No authoritative task event envelope with stable event ID, public task ID, schema version, generation timestamp and redacted body.
- No transactional event publication tied to durable terminal state, nor a public guarantee about billing-settlement ordering. Task completion and final verified billing are distinct; an estimated reservation must not be reported as a final charge.
- No durable delivery outbox, retry schedule/window, redelivery API, delivery inspection, event deduplication or ordering policy.
- No published signature verification algorithm/header version, required signature, clock tolerance, rotation overlap or event retention contract for developer events.

## Future proposal — DESIGN_ONLY

First use cases are asynchronous **`task.completed`** and **`task.failed`**. Optional later balance-threshold and model-availability events need an explicit scope and noise policy. A final-billing event must wait for authoritative ownership/final-settlement evidence; this proposal does not unblock Phase 3D.

A candidate event envelope has `id`, `type`, `schema_version`, `created_at`, and minimal `data` containing an owner-visible public task ID and normalized terminal status. Clients fetch authorized task detail/artifacts when needed. Exclude upstream IDs/credentials, raw provider payloads, prompts, private task state, API keys and secret-bearing URLs. Completion notifications must not silently leak results across keys or tenants.

Delivery should be at least once with stable event IDs across attempts, no global ordering promise, and receiver deduplication. A durable outbox should only publish after the matching task transition commits. Repeated notifications of one terminal transition must preserve the logical event ID. Receiver 2xx acknowledges; timeouts/429/5xx may retry with bounded exponential backoff and jitter; permanent 4xx, disabled/revoked endpoints and expired windows need explicit policies. A candidate retry window is 24 hours with event/delivery retention of 7 days; those numbers require product/storage approval and are **not current guarantees**.

Use a separately generated per-endpoint signing secret; **never reuse the main API key**, send the secret as a bearer header, or expose it in delivery logs. A future signature could cover timestamp + event ID + exact raw body with HMAC-SHA256 and a version/key identifier. Receivers should verify in constant time, enforce a documented clock tolerance (candidate five minutes), and deduplicate event IDs for at least the delivery/replay window. Authenticated manual redelivery preserves event ID but signs a fresh attempt timestamp. A timestamp alone is insufficient to prevent replay within tolerance.

Secret rotation needs one active signing key plus an explicitly bounded verification overlap, key identifiers, immediate revocation and encrypted storage. Endpoint management needs server-side ownership checks, constrained HTTPS destinations, private/link-local/metadata address blocking, DNS rebinding/redirect protections, response/body/time limits and egress policy for both direct and worker modes. Authentication changes require the project's OWASP review/test gate before implementation; this report makes no compliance claim.

Retention should minimize payloads, distinguish task/artifact retention from event and delivery history, redact logs, and cover deletion/revocation. Document final event types, TTLs, retries, ordering and signature headers only after implementation and security validation. No speculative webhook operation belongs in the stable public OpenAPI.

Validation: inspected router, notification, task/plugin and payment integration boundaries; no delivery request, paid upstream operation, authentication implementation or DB change was performed.
