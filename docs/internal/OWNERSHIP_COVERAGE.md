# Canonical ownership coverage — Phase 3A

Audit baseline: `319bd4d47` (2026-10-12), clean working tree before audit. This report was written before schema/implementation changes. Production database counts were not queried; source-path coverage is not a claim about historical row percentages.

## Before implementation

| Lifecycle | Principal and correlation sources | Coverage / gap |
| --- | --- | --- |
| HTTP / sync relay | `middleware/request-id.go`; `relay/common/relay_info.go:GenRelayInfo`; `model/log.go:RecordConsumeLog/RecordErrorLog` | Authenticated user/key IDs present. HTTP request ID in Gin/context/header; fallback in RelayInfo does not propagate. Optional/missing consume logs do not prove zero usage. |
| Generic/native/plugin/Responses/video/image/speech tasks | `controller/relay.go` central insertion; `model.InitTask` | User column present. Private token ID and execution request ID pinned before insertion, but ownership cannot be indexed/filtered portably without parsing JSON. |
| DFLOP tasks | `service/billing_reservation.go:PrepareDFLOPTaskBillingIdentity` | Existing user/plugin/idempotency hash excludes token and overwrites Gin/RelayInfo request ID; header/context retain another ID. This is not one canonical request identity. |
| Ordinary final adjustments/refunds | `service/task_billing.go`; `model.RecordTaskBillingLog` | User/private token and public task string present. Original request ID missing; logging creates unrelated request IDs. Public task strings may duplicate. |
| DFLOP journal settlement/refund | `model/billing_reservation_log.go`; `service/task_billing.go` | Journal has user/key/operation ID; lookup lacks key scope. Released holds are not consumed charges. Nonfatal/optional consumption logs are not authoritative ledger completeness. |
| Midjourney | `relay/mjproxy_handler.go`; `service/midjourney.go`; `model.Midjourney` | User present. TokenId is mutable accounting marker, not immutable owner. Request correlation absent. Separate ownership fields required for new writes. |
| Internal runtime verification tasks | `service/dflop_runtime_execution.go` | Funding user without submitting API-key principal. Must remain unowned and excluded publicly. |

Task IDs have nonunique indexes; `GetUniqueByOnlyTaskId` already recognizes ambiguous historical identifiers. Current generic GET task is user-scoped and uses inference auth. Raw task payload/presenter and fail-reason fields are not a safe new public DTO. Console and compatibility protocol behavior must remain separate from new key-scoped reads.

## Historical classes

- Fully correlatable: positive private task token that belongs to stored user plus valid non-DFLOP execution request ID; deterministic materialization only.
- Partially correlatable: positive matching token with missing request ID; charged Midjourney token with no request provenance. Key ownership can be materialized, request identity cannot be guessed.
- Unknown/conflicting: absent/invalid token, mismatched token user, malformed private JSON, conflicting canonical fields, internal verification tasks, and DFLOP billing hashes without immutable original HTTP identity. Do not infer key from user, timestamp, name or raw credential.
- Duplicate task IDs remain nonunique. Public queries require exact key/user scope and exactly one retained eligible row; ambiguous results use the same 404 as foreign/missing tasks.

## Minimal design / migration decision

Materialize hidden `tasks.token_id` and `tasks.request_id`; JSON cannot supply portable indexed security predicates across SQLite/MySQL 5.7/PostgreSQL 15. Add nonunique `(token_id,id)` and `(token_id,task_id)` indexes for list/detail only. Add separate hidden `midjourneys.owner_token_id` and `request_id`; do not repurpose its existing billing marker. No unique historical task-ID constraint and no new ledger table.

Use startup AutoMigrate plus a bounded, idempotent Go/GORM backfill, following current repository migration conventions. Read private snapshots as text and decode individually; verify token/user identity including deleted token rows. Update only empty destinations, preserve explicit canonical values, and leave ambiguous historical provenance unknown. No database-specific JSON operators.

Preserve immutable HTTP canonical request ID separately from billing operation identity. Existing private execution request ID can retain historical journal identity; add explicit canonical snapshot provenance. New DFLOP idempotency identity includes submitting token ID; journal reads verify the same key. Preserve pricing, rounding, funds/reservation/refund formulas and journal state transitions.

New task billing logs receive explicit canonical request ID and exact task-record identity when known, rather than regenerating or joining by ambiguous public task strings. Midjourney pins immutable owner/request before persistence. Internal tokenless workflows remain excluded.

## Gate and later phases

3A foundation gate PASSED before endpoint implementation: model fresh/upgrade/repeat matrix, full model package, focused request/refund/unit/journal regression and real refund/journal matrix passed on all three engines. Main/log separation was covered by the fixed-billing matrix. Required tests cover new-write provenance, same-user keys, exact journal ownership, refunds, deterministic/unknown backfill, fresh/upgrade/repeated migration and nonunique historical IDs.

After 3A, key usage may expose bounded retained billing-event aggregates, explicitly not an authoritative wallet or final request ledger. Consume logging is optional and failures/retention can omit records. Token columns can be synthetic for images/audio; historical task facts can lack frozen unit provenance. New plugin task writes freeze numeric billing units alongside their private billing snapshot. Unknown components must not become zero or fake measured tokens.

Complete request-correlated final usage remains **BLOCKED_BY_OWNERSHIP_MODEL** until durable event identity/completeness, finality, retention and unit provenance can be proven. Ordinary adjustment/refund writes are nontransactional; existing CAS does not make the entire funds/token/task/log chain atomic. Ownership changes must not claim to repair accounting idempotency.

Cancellation is design-only: Task Plugin API v1 has no canonical durable cancellation hook. Closing an HTTP/SSE reader does not cancel a durable provider task. A future cancel operation needs explicit capability, exact key ownership and verified provider/billing reconciliation.

Security review uses OWASP ASVS 5.0.0 and the Authentication, Session Management and Authorization Cheat Sheets: server-side object authorization, current expiry/revocation/IP/group checks, uniform foreign/missing responses, secret-free projection and no credential-bearing SQL logs. No blanket compliance claim is made before regression verification.

## Foundation verification gate (before public endpoints)

- Task schema/backfill RED observed before production edits; schema GREEN ran fresh and pre-change upgrade twice on SQLite 3.50.4, MySQL 5.7.44, PostgreSQL 15.19. Upgrade schema matches baseline and available local root release v1.0.0-rc.41; this is not a claim about an uninspected production DB.
- Full model package passed; fixture covers 201 valid rows across a 200-row batch boundary, malformed/unknown/mismatched/deleted-token provenance, internal verification exclusions, historical duplicate IDs and preserved data/billing/indexes. Production coverage percentages remain unmeasured.
- Focused service RED reproduced canonical-ID overwrite, same-user key digest collision, missing refund/task-record correlation, missing unit snapshots and duplicate charging when introducing a token-scoped identity beside a legacy journal. GREEN passed after the narrow provenance/replay fixes.
- Exact persisted owner/request/refund/Midjourney invariants passed in router fixture on all three real engines. A foreign legacy journal denied refunds without changing either key allowance.
- Fixed-price billing matrix passed across the three engines, including separately configured main/log MySQL and PostgreSQL stores, token-owner and duplicate-journal guards. No paid or provider upstream calls.

The gate establishes canonical persisted provenance, not complete authoritative usage or globally private legacy media. Later public task reads must additionally enforce exact bearer-key predicates, reject unknown/conflicting/duplicate provenance, and avoid arbitrary provider payloads. The anonymous Midjourney media proxy was found to dereference private stored tasks by a globally guessed ID. Phase 3C closes this bypass with exact bearer ownership or an expiring route-bound signed asset capability issued from authorized Console/task projections; this is required before claiming key isolation.

## HOTX PUBLIC DEVELOPER API — PHASE 3 REPORT

1. **Git baseline:** clean `main`, `319bd4d47`. No staging, commit, push, stash, reset or discard.
2. **Ownership audit:** HTTP → relay → reservation → log → task → poll → settlement/refund audited before schema work. Key-only reads never widen to account scope.
3. **Existing fields:** user/token/request IDs on logs; private task token/execution IDs; nonunique public task IDs; mutable Midjourney accounting token. Canonical HTTP identity now remains separate from billing operation identity.
4. **Coverage:** each migration fixture contains 212 task rows (213 on SQLite, including malformed JSON): 207 have usable canonical key provenance after migration, of which 3 also have deterministic HTTP correlation; 5 rows remain unknown/conflicting (6 on SQLite). These are fixture counts, not production percentages. Two of four historical Midjourney fixtures get deterministic owner materialization; none gets a guessed request ID.
5. **Migration decision:** hidden task token/request columns, two nonunique owner indexes; separate Midjourney owner/request columns, reusing existing user/public-ID indexes. Existing JSON alone cannot provide portable indexed security predicates. Startup AutoMigrate and bounded idempotent backfill follow the existing convention; no new migration framework, unique historical constraint, ledger table or log-index migration.
6. **Canonical model:** authenticated immutable user/key principal; immutable original HTTP request; exact task-record identity; private billing-operation identity retained for reservations. Public DTOs omit internal principal IDs.
7. **Backfill:** deterministic matching private token/user only, including deleted key identity; empty destinations only. Conflicts, missing/invalid provenance, internal verification and legacy billing digests fail closed. Repeated migration preserves existing data, billing fields and indexes.
8. **New writes:** generic/native/protocol tasks and Midjourney pin caller key and original request before persistence/async separation. New task billing logs carry original request and exact task-record relation when available; original HTTP format remains unchanged.
9. **Key usage:** `GET /v1/key/usage`, authenticated current key, `start`/`end` Unix seconds, half-open interval, default last 30 days, maximum 31 days, optional exact `model`. Zero balance/zero recorded events remain readable.
10. **Usage DTO:** decimal `recorded_consumption`, `recorded_refunds`, `recorded_net`, configured public unit, billing/error event counts and `{subject,unit,quantity}` components. No invented request/success count. Frozen numeric plugin units, measured token/cache evidence and confirmed task billing provenance only. Missing/synthetic/estimated values remain unknown/partial. Latest proven task snapshot is counted once; cached aggregate/TTL quantities do not double count. Refunds in a window can refer to consumption outside it, so recorded net may be negative.
11. **Task list:** `GET /v1/tasks`, default 20/max 100, opaque cursor; one bounded owner query, no per-task lookups. Unknown/conflicting/discarded/internal rows and duplicate IDs within the exact principal are excluded.
12. **Task detail:** existing `GET /v1/tasks/{key}` uses a task ID, exact authenticated principal and typed metadata DTO. Existing public status vocabulary reused; fixed failure summary. Foreign/missing/ambiguous/discarded returns indistinguishable 404. No raw task data/result URL/provider error/private snapshot. This generic list/detail covers retained `model.Task` records; legacy Midjourney records keep their dedicated native/Console routes, now with key ownership protection.
13. **Request usage:** **BLOCKED_BY_OWNERSHIP_MODEL**. No endpoint/OpenAPI operation. Durable event completeness/identity, historical collision lineage and authoritative finality remain unproven; optional/failed/pruned logging is not a final ledger.
14. **Cancellation:** **DESIGN_ONLY**, P2. Explicit versioned provider/plugin capability, exact key ownership, repeat/race semantics and financial reconciliation are required. Task Plugin API v1 has no canonical cancel hook; disconnect is not durable cancellation.
15. **Cross-key security:** A/B same user and C another user tested in all three databases. Expiry/revocation/IP and exhausted-quota exceptions verified. Bearer native/protocol/artifact/origin queries use canonical key ownership. Signed assets are explicit delegated byte access, not task-record/list authorization.
16. **Async settlement/refund:** gross/refund/net follows retained actual log types, not the first consume row. DFLOP identities include caller key; legacy replay/foreign/duplicate journals deny before financial mutation. Existing numeric billing formulas and funding/refund state transitions remain intact. Ordinary nontransactional durability gaps remain.
17. **Sanitization:** explicit DTO whitelist and regression denial of principal/channel/upstream IDs, credentials, provider bodies, costs, ratios and expressions. Reserved quantity subject names rejected. Midjourney unsigned proxy bypass closed: exact bearer owner or signed 15-minute route-bound preview; expiry/tamper/route replay/global duplicate ID denied. Authorized historical Console previews preserved. Capability queries redacted before request logging; provider failure bodies sanitized. Image proxy sniffs a bounded 512-byte prefix using the already declared MIME library, permits verified raster formats only, sets nosniff and streams the complete image; HTML/SVG/JSON and forged image headers return generic 502.
18. **OpenAPI:** only implemented usage/list/detail contracts added/updated. Five public Go DTOs checked against exact required/optional OpenAPI fields. Blocked request usage/cancellation absent.
19. **Documentation:** 108 public entries, 14 domains, 6 guides, 21 developer pages; seven locales. Existing beginner navigation preserved. Ownership, async continuation, retained records, unknown components and interval refund semantics documented. `DOCS_SITE_URL` remains operator configuration, no invented origin/SLA.
20. **Performance:** usage uses two SQL queries: portable money/event aggregate plus streamed maximum 10,000 component rows; cap does not truncate monetary/event totals. Existing token/user/time indexes audited: SQLite used token index, PostgreSQL used time index, MySQL chose a 13-row fixture scan with eligible existing indexes. No evidence justified another index. Main task list/detail each issue one bounded task SQL statement. Local list fixture HTTP latency: SQLite 0.28–0.37 ms, MySQL 1.10–6.78 ms, PostgreSQL 0.99–1.57 ms; not production performance guarantees.
21. **DB matrix:** SQLite 3.50.4, MySQL 5.7.44, PostgreSQL 15.19. Fresh/pre-change release schema upgrade/repeated migration passed, including verified legacy MySQL COMPACT row-format preservation; separate main/log stores verified. Upgrade fixture represents locally available release v1.0.0-rc.41 and baseline, not an uninspected production DB. Real ClickHouse was not exercised.
22. **Tests:** final full 87 root-module packages passed: 5,445 Go test/subtest events, 0 failures, 76 environment-gated skips. Focused three-database public matrix: 74 pass, 0 fail/skip. The final post-recovery six-package focused run passed 225 test/subtest events with zero failures/skips, covering source-final ownership, usage, private-byte projection, fresh/upgrade/repeated COMPACT migration and separate main/log billing stores. Frontend full suite: 2,347 tests/184 files passed; final documentation subset 142 passed. Typecheck, build, 158-page SSG, docs generator/check, DTO/schema and responsive/localized/browser/clipboard checks passed. Changed-file lint has zero errors; full lint remains 182 existing errors/66 warnings. `git diff --check` and modified Go formatting passed. Intermediate runs hit disk exhaustion, an untouched SQLite concurrency failure and a Docker daemon outage during a matrix repeat; these are not hidden. The final 87-package unit run passed with its database environment gates unset; the source-final real three-database suites were then reverified separately after Docker recovery, including COMPACT migration cases. Only task-owned disposable containers were stopped/removed; other containers were preserved. No unrelated auth flow was changed.
23. **Limitations:** retained records are not complete authoritative history; historical requests/refunds remain partially correlatable; generic upstream token provenance, unsupported facts and missing frozen units can be unknown. ClickHouse has no real runtime matrix here and zero/nonunique IDs prevent reliable component ordering. Legacy Midjourney records are not merged into the generic task list. Existing generic artifact capabilities retain their prior lifetime contract; only new Midjourney previews expire after 15 minutes. Legacy DFLOP journal replay can require manual resolution. Operator data volumes were not benchmarked.
24. **Phase 4:** durable event identity/completeness/finality and request usage gate; measured legacy media provenance; capability-specific cancellation and billing reconciliation; production index/retention profiling; operator docs origin.
25. **Working tree:** `git diff --stat`: 59 tracked files changed, 4,492 insertions and 1,282 deletions. Eight new files remain untracked/unstaged (seven Go sources/tests plus this report); 67 changed/new files total. Generated reference metadata accounts for most line churn. No dependency/lockfile changes. The changed-file list is below.

### Verification commands

Root Go verification ran each result of `go list ./...` sequentially with:

```sh
CGO_ENABLED=0 go test -p 1 -gcflags=all=-dwarf=false -ldflags='-s -w' -count=1 -json <package>
```

`PHASE2_*_DSN`, `PHASE3_*_DSN`, `PHASE3_*_LOG_DSN` and `TEST_FIXED_*_DSN` pointed only to disposable local MySQL/PostgreSQL main/log fixtures. Both owned containers were stopped/removed after tests. Logs: `/tmp/hotx-phase3-go-serial.jsonl`, `/tmp/hotx-phase3-go-summary.json`, `/tmp/hotx-phase3-public-matrix-final.jsonl`, `/tmp/hotx-phase3-usage-final-matrix.log`, `/tmp/hotx-phase3-final-focused-matrix.jsonl`. The infrastructure-failed repeat is retained in `/tmp/hotx-phase3-go-infrastructure-failure.jsonl`.

Frontend verification used installed Node with Bun: `bun run test --maxWorkers=2`, `bun run typecheck`, `bun run build`, `bun run docs:check`; scoped oxlint and the existing documentation browser verifier. Local fixtures/mocks only: paid/provider upstream requests **0**.

Security references: [OWASP ASVS 5.0.0](https://owasp.org/www-project-application-security-verification-standard/), [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html), [Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html), [Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html). Applicable controls include 6.3.4; 8.2.1–8.2.4; 8.3.1–8.3.2; 14.2.1/14.2.2/14.2.5; 16.2.5/16.3.1–16.3.2/16.5.1. Focused server-side controls were verified; this is not blanket ASVS certification.

### Status summary

| Requirement | Status |
| --- | --- |
| Phase 3A ownership foundation | PASS |
| New sync/async canonical token ownership | PASS |
| Request↔task correlation | PASS for new relevant writes; PARTIAL historically |
| Refund correlation | PASS for new known tasks; PARTIAL historically |
| GET key usage / task list / task detail | PASS |
| GET request-correlated usage | BLOCKED_BY_OWNERSHIP_MODEL |
| Task cancellation | DESIGN_ONLY |
| Same-user cross-key / cross-user isolation | PASS |
| Historical ambiguous rows fail closed | PASS |
| No raw caller API key newly persisted | PASS |
| No private task fields in public DTO | PASS |
| Recorded gross/refund/net semantics | PASS |
| SQLite / MySQL 5.7 / PostgreSQL 15 | PASS |
| Existing API regression suite | PASS |
| Changed-file lint / typecheck / build | PASS |
| Paid / provider upstream requests | 0 / 0 |
| No commit / no push / no stage | YES |
| Scope respected | YES |

### Changed files

```text
 M controller/midjourney.go
 M controller/plugin_native_e2e_test.go
 M controller/plugin_protocol.go
 M controller/plugin_protocol_audio_speech.go
 M controller/plugin_protocol_sdk_test.go
 M controller/plugin_protocol_test.go
 M controller/relay.go
 M controller/task.go
 M controller/task_generic_test.go
 M controller/token_test.go
 M docs/internal/API_INVENTORY.md
 M docs/internal/PUBLIC_API_GAP_REPORT.md
 M docs/openapi/relay.json
 M e2e/doc_parse_test.go
 M middleware/auth.go
 M middleware/distributor.go
 M middleware/task_artifact_access.go
 M middleware/task_artifact_access_test.go
 M middleware/task_plugin.go
 M middleware/task_plugin_origin_task_test.go
 M middleware/task_plugin_test.go
 M model/billing_reservation_log.go
 M model/log.go
 M model/main.go
 M model/midjourney.go
 M model/task.go
 M relay/common/relay_info.go
 M relay/mjproxy_handler.go
 M relay/relay_task.go
 M relay/relay_task_test.go
 M router/plugin_router_test.go
 M router/relay-router.go
 M router/relay_router_test.go
 M router/task-router.go
 M router/task_router_test.go
 M router/video_router_test.go
 M service/billing.go
 M service/billing_reservation.go
 M service/midjourney.go
 M service/passive_runtime_evidence.go
 M service/task_artifact_access.go
 M service/task_billing.go
 M service/task_billing_test.go
 M service/task_plugin_audit.go
 M service/task_polling.go
 M service/text_quota_test.go
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
?? controller/developer_tasks.go
?? controller/developer_usage.go
?? docs/internal/OWNERSHIP_COVERAGE.md
?? dto/developer_tasks.go
?? dto/developer_usage.go
?? model/developer_usage.go
?? model/task_ownership.go
?? model/task_ownership_test.go
```
