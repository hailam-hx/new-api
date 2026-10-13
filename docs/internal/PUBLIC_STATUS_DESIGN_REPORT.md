# Customer-facing service status — Phase 5G

**Status: DESIGN_ONLY / GAP_IDENTIFIED.** Existing dashboard uptime information
is present. A versioned customer-facing production status API/SLA contract is not
implemented by this report. No route or domain is created.

## Actual surface audit

| Surface | Source and access | Meaning/limit |
| --- | --- | --- |
| `/live`, `/health`, `/ready`, `/readiness`, `/metrics` | No static registration found in the Go router/main code. [`router/web-router.go`](../../router/web-router.go) serves SPA HTML with HTTP 200 for otherwise unmatched non-API paths. | An HTTP 200 from these unregistered paths can be the SPA, not liveness/readiness. Plugin/deployment-specific routing or an external proxy may differ; no universal contract is inferred. |
| `GET /api/status` | Anonymous dashboard bootstrap in [`router/api-router.go`](../../router/api-router.go), [`controller/misc.go`](../../controller/misc.go). | Global UI configuration, enabled features, branding, version/start time and public auth configuration. No DB/Redis readiness check or incident aggregate. `success:true` is not an availability/SLA assertion. |
| `GET /api/status/test` | AdminAuth; `TestStatus` in [`controller/misc.go`](../../controller/misc.go). | DB ping, HTTP statistics; DB failure is 503, success 200. Operational diagnostic, not public customer status. |
| `GET /api/performance/stats` and related routes | RootAuth in [`router/api-router.go`](../../router/api-router.go), [`controller/performance.go`](../../controller/performance.go). | Internal process/runtime resource statistics and administrative operations. Must remain protected. |
| `GET /api/perf-metrics[/summary]` | Pricing module visibility/public-or-user authentication policy; [`controller/perf_metrics.go`](../../controller/perf_metrics.go), [`pkg/perf_metrics`](../../pkg/perf_metrics). | Recent model/group performance samples. Not independent liveness/readiness, incident feed or uptime guarantee. Error responses can contain underlying query error text; do not use them as a new public status projection. |
| `GET /api/uptime/status` | Anonymous route; [`controller/uptime_kuma.go`](../../controller/uptime_kuma.go), operator groups in [`setting/console_setting/validation.go`](../../setting/console_setting/validation.go). | Configured Uptime Kuma proxy returns category, monitor name/group, status integer and 24-hour uptime. Empty configuration returns empty data; remote failures can also yield empty monitor lists with `success:true`. This is not proof of operational health. |
| Dashboard uptime panel | [`web/src/features/dashboard/components/overview/uptime-panel.tsx`](../../web/src/features/dashboard/components/overview/uptime-panel.tsx), dashboard configuration. | Existing UI for configured external monitoring, including empty-state display; not a separate public incident status site. |
| Provider `/health` and `/metrics` requests | [`controller/channel_inference.go`](../../controller/channel_inference.go). | Operator inference-channel inspection of upstream endpoints. They are not gateway-owned public health routes. |

Uptime Kuma fetching is bounded by a 30-second request context and 10-second HTTP
client timeout; up to 20 configured groups are allowed. Its monitor labels are
operator-controlled and may reveal internal names if configured carelessly.
The proxy currently uses a generic HTTP client; this report does not claim that
it supplies a hardened reusable public-monitor fetch pipeline.

## Internal health versus customer status

Internal liveness asks whether a process is running; readiness asks whether a
node should receive traffic. Database/cache/poll-worker probes belong in protected
deployment monitoring. They need bounded checks, explicit dependency policy and
response-body verification rather than SPA status-code checks. This phase does
not invent those endpoints or expose administrative diagnostics.

Customer status asks whether an advertised service function is available, what
incident affects customers and how fresh the assessment is. A running process
or successful DB ping does not establish model/provider availability. Conversely,
an unavailable optional provider does not imply all gateway APIs are down.

## Proposed presentation and contract

Prefer an operator-selected public status page/monitoring service first, using
the existing configurable Uptime Kuma facility where its public projection is
appropriate. A page supports incident explanations, maintenance and history
without expanding inference API obligations. Configure an existing approved
origin/URL; do not create or hard-code a new status domain.

A machine-readable API can be considered later when customer automation requires
it. Its versioned projection would include an aggregate status, assessment time,
public component status and optionally active incident ID/title/update time. A
starting component vocabulary could be **API, Models, Media, Dashboard**, marked
**DESIGN**, not a current promise. States need explicit operational/degraded/
outage/maintenance/unknown semantics and stale-data behavior. No measurements or
missing observations should become `operational` by default. Separate service
availability from individual model capabilities and operator deployment features.

Serve a cached bounded projection with rate limits, conservative cache lifetime,
last-updated/freshness metadata and validated inputs. Generate it from sanitized
monitor/incident evidence rather than forwarding raw diagnostics. The operator
must define component ownership, aggregation rules, reporting window, incident
workflow, freshness budget and maintenance behavior before implementation.
External monitor failures should yield an explicit unknown/stale assessment, not
an empty success interpreted as green. Any future external fetching requires
mandatory SSRF, redirect, response-size and timeout controls.

Never expose DB/Redis hostnames, addresses or topology; node names/internal IPs;
channel/provider credentials; private model/channel inventories; stack traces or
raw dependency errors. Public monitor labels require review. Existing dashboard
bootstrap configuration remains its own contract and must not become an
unfiltered status response. Do not claim an uptime SLA from a 24-hour monitor
value or a single locally successful request.

## Decision and verification boundaries

Keep existing surfaces intact and retain **DESIGN_ONLY** for a new public status
contract. No OpenAPI `/v1/status` operation, fake `/health`, synthetic readiness
probe or webhook incident channel is added. Audit used source inspection and
route searches; no external Uptime Kuma/provider calls were made. A future status
implementation needs local tests for stale/unknown data, partial outages,
privacy projection, authentication boundaries, cached failures and configured
external source safety. Production operators must verify their own monitor
targets, body expectations, TLS, labels and incident ownership.
