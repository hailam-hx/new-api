# Public status/health gap report — Phase 4H

Status: **AUDIT_COMPLETE / DESIGN_ONLY**. Baseline: `main` at `efb670890`. No public status endpoint/domain or SLA is introduced.

## Current surfaces

The repository has real console status and monitoring endpoints, but no established HOTX developer service-status contract. An anonymous endpoint is not automatically a public developer contract.

| Surface | Evidence | Actual semantics |
| --- | --- | --- |
| `GET /api/status` | `router/api-router.go:29`; `controller/misc.go:44–129` | Anonymous console bootstrap/configuration response. Includes version/start time, branding, login settings, feature flags and optional console sections. It does not measure relay availability, task backlog, incident state or provider health. |
| `GET /api/status/test` | `router/api-router.go:32`; `controller/misc.go:25–40` | Admin-authenticated DB connectivity check plus HTTP statistics. Returns **503** on DB ping failure and **200** with `http_stats` on success. A healthy DB is not evidence that all paid inference/task providers work. |
| `GET /api/uptime/status` | `router/api-router.go:30`; `controller/uptime_kuma.go:19–23,26–36,57–129,131–156` | Anonymous console integration with configured Uptime Kuma status-page and heartbeat feeds. Returns **200 `success:true`** with empty data when no groups exist. Upstream fetch failures become empty monitor lists; the outer response still reports success. Monitor fields are `name`, numeric `uptime`, numeric `status`, optional `group`, grouped by `categoryName`. This is deployment-dependent aggregation, with no public freshness, degraded/unknown, incident, component or availability guarantee. |
| Administrative runtime/channel/system APIs | `router/api-router.go:270`; `router/channel-router.go:70–71`; `router/api-router.go` system-info/performance groups | Protected console/internal operational inspection; not a safe replacement for a public status feed. |
| `/health`, `/live`, `/ready` | Static route inspection in `router/*.go`, `main.go` | No dedicated registration found in these sources. A deployment may have external probes; this audit neither asserts their existence nor publishes them. |
| Catalog, balance, usage and task reads | `router/relay-router.go:24–27`; `router/task-router.go:24–25` | Authenticated data/capability/ownership APIs. Catalog listing a model does not guarantee immediate upstream availability, and task status describes one operation rather than platform health. |

There is no dedicated public incident/status page or versioned status DTO established by the inspected developer route contract. Configured console Uptime Kuma panels are not a HOTX-wide product commitment. Do not put the above `/api` routes into the public OpenAPI merely to fill the status gap, and do not expose internal/Kubernetes probes as a public service-status API.

## Future proposal — DESIGN_ONLY

A product decision may choose a public status page or a narrowly scoped API; this report reserves no path and invents no domain. Define which service components are actually monitored (gateway request handling, task execution, and selected public model/provider groups), responsible operators and authoritative incident sources before publishing a health claim.

A minimal future representation could include schema version, generated/observed timestamps, overall and component states (`operational`, `degraded`, `outage`, `unknown`), public incident IDs, affected components and announced maintenance. These enum names are proposals. Missing or stale telemetry must become `unknown`, not silently healthy. Define cache TTL, stale threshold, historical incident retention and update cadence; candidate values require monitoring/product approval rather than copying the existing Uptime Kuma request timeout into an availability SLA.

Use coarse component identities and publish only reviewed summaries. Exclude channel IDs, internal URLs, credentials, database topology, per-user workloads and sensitive incident details. Anonymous reads can be rate-limited/cached; authenticated operational probes stay separate. Status must never bypass authorization, expose signed artifact links, or trigger a paid provider probe from a public request.

Document that status is an informational snapshot and may lag an incident, only after the actual monitoring design exists. Do not promise availability percentages, response times, every model being operational, `Retry-After`, or incident retention without implementation and an explicit policy.

Validation: source inspection of static routes, controller responses and console monitoring integration only; no deployment probe, external monitoring fetch, paid upstream call or database change. Phase 3D remains **BLOCKED_BY_OWNERSHIP_MODEL**.
