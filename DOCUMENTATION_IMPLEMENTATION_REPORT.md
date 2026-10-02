# DOCUMENTATION IMPLEMENTATION REPORT

## Design and implementation plan

Integrate a public `/docs` feature in the existing React 19 / TanStack Router / Rsbuild application. Use the existing PublicLayout, Dialog, CopyButton, Tabs, Input, Markdown and pricing display components. Articles are English source strings resolved by the existing i18next architecture. UI and editorial translations cover all seven supported locales; article paragraphs, provider metadata and search follow the selected interface language. Keep API and billing execution unchanged.

1. Audit source, resolve the nine navigation groups, and retain source references for each article.
2. Extract registered routes with Go AST, join only matching operations to OpenAPI, and generate error constants. Never publish unimplemented OpenAPI paths as supported.
3. Build the responsive documentation shell, search, heading anchors, TOC, SDK tabs and navigation.
4. Reuse `/api/pricing` and its access policy for catalogs, compatibility, model details and prices. Missing metadata stays unknown. Token-scoped `/v1/models` is authoritative for actual key access.
5. Add original getting started, protocol, integration, guide, console, administrator and reference articles grounded in source.
6. Generate crawlable article HTML, sitemap and metadata from the same article registry after the production build. Use a configured deployment origin for absolute canonicals.
7. Verify generator contracts, UI interactions, typecheck, lint, tests, build and browser navigation. Record limits honestly.

## Source mapping before implementation

| Documentation section | Actual source | Status | Implementation |
| --- | --- | --- | --- |
| Frontend / router | `web/package.json`, `web/rsbuild.config.ts`, `web/src/routes/` | FOUND | Integrated feature, file routes |
| Existing documentation | `docs/openapi/{api,relay}.json`, `docs/plugin-api/v1.md`, README; header `/docs` link with external override | FOUND; no internal docs route | New public shell; preserve override |
| API reference | `router/*.go`, `pkg/jsplugin/routing.go`, OpenAPI | FOUND; OpenAPI has stale routes | AST-generated route index + matched schemas |
| Model catalog / provider / endpoint types | `controller/pricing.go`, `model/pricing.go`, `model/model.go`, `model/ability.go` | FOUND | Existing pricing query, no model seed list |
| Context / modalities / capabilities | Pricing DTO frontend reserves fields; backend pricing does not expose all of them | PARTIAL | Display supplied fields only; unknown otherwise |
| Callable model access | `router/relay-router.go`, `controller/model.go`, `/v1/models` | FOUND | Examples select a model from the API key's list |
| Pricing | `model/pricing.go`, `setting/billing_setting/`, `setting/ratio_setting/` | FOUND | Reuse ModelPriceCell and expression breakdown |
| Billing | `relay/request_billing.go`, `service/billing*.go`, `service/{text_quota,image_billing,tiered_settle,task_billing}.go`, `pkg/billingexpr/expr.md` | FOUND | Explain reservation/settlement/refunds; no billing changes |
| Channels / model mapping / routing | `router/channel-router.go`, `middleware/distributor.go`, `model/{channel,cache,ability}.go` | FOUND | Administrator guides |
| Authentication / tokens | `middleware/auth.go`, `docs/authentication.md`, `router/api-router.go`, `web/src/features/{auth,keys,security,profile}/` | FOUND | Separate relay API keys, management access tokens and browser sessions |
| Console | `web/src/routes/_authenticated/`, feature section registries | FOUND | Exact UI destinations: keys, dashboard, usage-logs, wallet, subscriptions, profile, security |
| Administration | channels, models metadata/vendors/deployments, users, subscriptions, usage-logs/audit, system-settings | FOUND | Document actual routes and permission restrictions |
| Finance / orders | wallet and subscription features, user top-up APIs | FOUND within existing features | Document wallet/order workflow; no invented Finance route |
| Video / async tasks | `router/{video,task,task-plugin-protocol}-router.go`, host protocol registry | FOUND; conditional provider support | Protocol-specific caveats |
| Errors / limits | `relaykit/types/error.go`, `middleware/{model-rate-limit,rate-limit}.go` | FOUND | Generated constant index; configurable limits, no invented quotas |
| Webhooks | Payment provider webhooks in `router/api-router.go` | FOUND payment-only | Explain inbound payment callbacks; no generic AI task webhook promise |
| Changelog | `VERSION`, Git history; no dedicated release registry | PARTIAL | Link release history; no invented release notes |
| Status | Optional Uptime Kuma integration | CONDITIONAL | No fake status page |
| Client integrations | Existing protocol surfaces and model endpoint metadata | CONDITIONAL | Configuration guides with explicit compatibility limits; no certification claims |
| SEO | SPA served from embedded `web/dist` by `router/web-router.go` | FOUND | Static generated article files served by existing static middleware |

## Governance and security

Preserve New API / QuantumNous branding, all existing copyright headers, user changes and external Docs configuration. No database/schema/dependency changes. No upstream keys, live API keys or provider configuration exported. Authentication guidance checked against OWASP Authentication and Session Management cheat sheets and ASVS 5.0.0; documentation work does not certify the existing authentication implementation.

## 1. Architecture discovered

React 19 + TypeScript, TanStack Router/Query, Rsbuild 2, Base UI/Tailwind 4 và i18next trong `web/`; Bun là package manager. Backend Go/Gin cung cấp management API, relay và host task protocols. GORM models/abilities và pricing settings là nguồn dữ liệu catalog. Authentication của bản customize dùng JWT ngắn hạn trong memory, refresh cookie và server session; relay API key và personal management token là hai loại credential khác nhau.

Không có Finance, Orders hay routing-rule-builder độc lập để tạo menu mới. Wallet/subscription có workflow thanh toán và đơn hàng; routing nằm trong cấu hình channel, priority/weight, distributor và cache. Payment webhooks có thật; generic AI-task webhook chưa có bằng chứng hỗ trợ.

## 2. Existing documentation state

Repository có OpenAPI, tài liệu plugin/billing/authentication và README, nhưng chưa có frontend docs nội bộ. OpenAPI chứa một số đường dẫn cũ hoặc chưa implement. Generator lấy route đăng ký từ code làm bộ lọc, rồi ghép schema OpenAPI tương ứng; không công bố Files/fine-tuning hoặc vendor route cũ chỉ vì chúng xuất hiện trong spec.

Nguồn UX được tham khảo: [DFLOP Docs](https://model.dflop.top/docs). Nội dung hướng dẫn được viết mới từ repository; không sao chép source, nội dung hay branding của site đó. Tên DFLOP trong provider directory đến từ ba plugin thực sự có trong repository.

## 3. Final IA

Chín nhóm: Getting started; API & SDK; Models & Pricing; APIs; Integrations; Guides; Console; Administration; Reference. Có 68 bài hướng dẫn/reference và directory quản trị riêng với 52 channel type + 13 shipped task plugin. Provider pages nằm trong directory, tránh làm sidebar dài thêm 65 mục.

Console liên kết đúng route hiện tại: Dashboard, API Keys, Model Square, Usage Logs, Wallet, Subscriptions, Profile và Security. Administration liên kết Channels, Task Plugins, Models, Users, Audit và System Settings. Các trang Finance/Orders giải thích workflow ở Wallet/subscription, không tạo route console giả.

## 4. Routes added

- `/docs`: homepage với luồng Developer và Administrator.
- `/docs/$slug`: bài viết; unknown slug trả Not Found; `/docs/overview` redirect về canonical `/docs`.
- `/docs/models/$modelId`: model detail từ catalog thực tế.
- Provider article slugs: `/docs/provider-<channel-type>` và `/docs/provider-plugin-<plugin-key>`, qua cùng article route.

TanStack route tree được generate lại. Không tạo route language độc lập; đổi ngôn ngữ giữ cùng canonical.

## 5. Components added

Documentation shell dùng PublicLayout, shared Dialog, CopyButton, Tabs, Breadcrumb, Markdown, StaticDataTable, LoadingState, ErrorState và EmptyState. Pricing dùng nguyên ModelPriceCell và DynamicPricingBreakdown hiện có.

DocsNavigation/DocsSearch là composition theo domain Documentation. Console CommandMenu/SearchProvider chỉ tìm và điều hướng các section console, nên không đáp ứng search heading/endpoint/error/article. DocsCode dùng CodeBlockFrame + CopyButton và Shiki lazy import: CodeBlock hiện tại tập trung JS/TS/Markdown editor, chưa cung cấp highlighting Bash/Python/JSON cho SDK docs. ModelDetailsContent cũ có display/mock fields không phù hợp catalog theo source-of-truth; ModelDocument dùng dữ liệu pricing thực và các formatter dùng chung. Đây là các capability gap cụ thể; không thêm primitive thay thế cho CopyButton/Dialog/pricing.

Desktop có sidebar/content/TOC; tablet/mobile dùng drawer. Code và tables scroll ngang trong vùng nội dung. Theme đi qua ThemeProvider của project. Search hỗ trợ bài, heading, endpoint, model/vendor từ catalog được phép xem, error code, `BILLING_*` prefix và shipped provider/plugin names.

## 6. Dynamic data sources

`usePricingData` dùng `/api/pricing`, query cache `['pricing']`, vendor metadata, group ratios và usable groups hiện có. Access gate đọc `/api/status` + auth store và chính sách pricing module; khi yêu cầu login, docs không tải hoặc tìm kiếm catalog trái chính sách. Browser regression xác minh trường hợp này không gọi pricing endpoint.

Model names/prices/capabilities không có seed list riêng trong docs. Quickstart liên kết trực tiếp tới `/keys` và gọi `/v1/models` với key của người chạy, chọn model có endpoint phù hợp hoặc dùng `NEW_API_MODEL` override đã kiểm tra. Catalog status là **Listed in catalog**, không coi đây là health/callability của một key. Category/context/modalities/capabilities thiếu được ghi **Not provided**; không suy đoán từ tên model.

Price UI dùng đúng logic có sẵn: mặc định là multiplier nhóm thấp nhất được enabled; chọn group để xem multiplier của group đó. Không đổi formula, quota, reservation, settlement/refund, DB hoặc provider behavior.

## 7. Generated pages

- 391 registered operations từ Go AST: group, middleware, permission table và host protocol handlers.
- 178 operations có schema OpenAPI khớp; 213 operations chưa có schema khớp và được ghi rõ. OpenAPI chỉ ghép với method/path hiện đăng ký; 21 stale/unimplemented operations bị loại.
- 44 relay error constants/auth mappings/billing diagnostic markers. AUTH có status từ mapper; status relay không có mapping duy nhất ghi Depends on call site. `BILLING_*` là diagnostic marker, không giả định là `error.code` HTTP.
- 52 provider pages từ registry channel picker; 13 plugin provider pages chỉ lấy key/name/version/source từ shipped manifest, không execute plugin JS, không export credential/model list/pricing.
- 133 article HTML pages được prerender sau build, gồm nội dung semantic và API/error reference.

`bun run docs:generate` cập nhật artifact; `bun run docs:check` phát hiện drift. Build frontend dùng artifact đã check-in để stage Docker chỉ copy `web/` không phải cài Go hoặc truy cập source root. Khi đổi router/plugin identity, cần regenerate + check trước build/review.

## 8. Manual pages

68 bài source có sections và source references: overview/quickstart/base URL/authentication/FAQ; OpenAI/Anthropic/Gemini/cURL; catalog/matrices/pricing/billing; Chat/Responses/Embeddings/Images/Video/Audio/Tasks; tám integration; streaming/tools/structured output/multimodal/image/video/retries/migrations; Console; Administration; headers/errors/limits/payment webhooks/changelog.

SDK ví dụ minimal + streaming cho cURL/Python/TypeScript. Model selection không hard-code tên demo. Không đưa response upstream giả vào docs: schema là checked-in contract, những operation thiếu schema ghi rõ gap. Integration guides dẫn tài liệu chính chủ và nêu giới hạn theo client/protocol/model; không tuyên bố certification hay hỗ trợ mọi tính năng client.

## 9. i18n changes

Dùng namespace và bảy locale hiện có, thêm 370 keys mỗi locale qua script `add-missing-keys.mjs`, rồi chạy `bun run i18n:sync`; script tạm đã xóa. Các entry người dùng sửa từ trước được giữ nguyên. `static-keys.ts` đăng ký dynamic group/article/section keys từ cùng article registry.

Shell UI và toàn bộ nội dung bài viết được đồng bộ cho en, zh, zh-TW, fr, ja, ru, vi. Đã áp dụng 1.316 cập nhật qua add-missing-keys.mjs và i18n:sync. Provider metadata dùng template dịch có tham số; giữ nguyên tên sản phẩm, mã API, URL, code và version. Tìm kiếm dùng nội dung đã dịch; metadata đổi cùng interface language. Không tạo translation system hoặc language routes riêng. Sync report: missingCount = 0, extrasCount = 0 ở tất cả locale. Các brand/literal và khóa provider ghép cũ còn nằm trong dictionary có thể xuất hiện trong báo cáo untranslated; nội dung đang hiển thị được kiểm tra riêng bằng registry tests.

## 10. SEO changes

SSG tạo nội dung đọc được trước JavaScript, title/description/canonical/Open Graph riêng, h1/h2 anchors, breadcrumb, internal links và previous/next. Metadata SSG được đánh dấu và dọn trước createRoot; DocumentationMetadata quản lý/restores template defaults để không giữ canonical của bài trước sau client navigation. Regression test và browser check xác minh chỉ một title/description/canonical.

Đặt `DOCS_SITE_URL` bằng public origin thật khi build để có absolute canonical, `docs-sitemap.xml` và robots Sitemap entry. Lần kiểm tra dùng `http://localhost:5173`, chỉ cho preview cục bộ. Khi không có biến này, generator không invent hostname/sitemap absolute; canonical trong SSG là relative, runtime dùng current origin.

Catalog/model data không snapshot vào HTML public; vẫn tải theo access policy. Host Gin static middleware hiện có phục vụ các article HTML trong embedded `web/dist`; không sửa production API/router.

## 11. Tests executed

Các command dưới đây chạy thực tế từ `web/` trừ Go router. Các log test/build được giữ ở `/tmp/docs-*.log` trong session này.

| Command | Observed result |
| --- | --- |
| `bun run docs:generate` | 391 operations, 44 errors/markers; 21 excluded OpenAPI operations |
| `bun run docs:check` | PASS, artifact deterministic/current |
| `bun run typecheck` | PASS |
| `PATH=/Users/jake/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH bun run test --maxWorkers=2` | PASS: 176 files / 2,168 tests; before the final additional shipped-provider search/nav cases |
| Same runtime, `bun run test src/features/documentation` | PASS: 4 files / 16 tests after final provider navigation/search changes |
| `DOCS_JQ_DIRECTORY=/tmp/new-api-docs-qa bun scripts/verify-documentation-examples.mjs` | PASS: six cURL requests against a local HTTP fixture; six Python examples parsed; six TypeScript examples transpiled |
| `DOCS_QA_URL=http://localhost:5174 PLAYWRIGHT_MODULE_PATH=…/dependencies/node/node_modules/playwright/index.mjs DOCS_CHROME_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' node scripts/verify-documentation-browser.mjs` using bundled Node | PASS with Chromium: navigation, metadata, SDK tabs, streaming/copy, endpoint anchor, dynamic fixture catalog/model detail, mobile drawer/overflow, dark mode, restricted catalog, Vietnamese UI switch; no page errors |
| `DOCS_SITE_URL=http://localhost:5173 PATH=…/dependencies/node/bin:$PATH bun run build` | PASS; 133 prerendered pages |
| `bunx oxlint -c .oxlintrc.json` with all touched TS/TSX/scripts paths | PASS, 0 errors; one non-blocking prefer-query-selector warning |
| `bunx oxfmt --check` with all touched source paths, excluding deterministic generated JSON | PASS |
| `bun run lint` | FAIL: 182 errors / 67 warnings in existing/out-of-scope files; no errors in changed docs/main/Hero/static-key scopes |
| `go test ./router` from repository root | PASS (cached) |
| `git diff --check` | PASS |

Runtime: Bun 1.4.2, bundled Node 24.19.0 for the full suite/browser, Go 1.27.1 darwin/arm64. An initial full test attempt selected Node 20.13.1 and failed before collecting tests (`ERR_REQUIRE_ESM` in jsdom dependency); switching runtime resolved it without dependency/config changes.

## 12. Test results

Focused regressions cover current vs stale/unimplemented endpoints, admin/token/host middleware boundaries, unknown capabilities, model discovery, search/anchors, mobile keyboard navigation, active navigation, empty search, SSG metadata/sitemap/provider links, and metadata cleanup on route changes.

Manual visual inspection: 1440px desktop model detail and 390px mobile dark Quickstart are readable; no page-level horizontal overflow. Screenshots: `/tmp/new-api-docs-desktop.png`, `/tmp/new-api-docs-mobile-dark.png` (fixture-only model/provider, no account data). Independent review found two important issues (Go dependency in Docker frontend build, stale/duplicate metadata); both fixed and re-reviewed with no remaining important findings.

## 13. Known gaps

1. Không có backend/upstream/test account chạy trên localhost khi audit. cURL fixture chứng minh discovery/headers/path/JSON/stream flags, không chứng minh thành công với upstream thật. Python/TypeScript chưa chạy với SDK installed + provider thật. Chưa chốt acceptance “request succeeds” trên deployment thật.
2. Full repository lint còn lỗi ngoài phạm vi, ví dụ `confirm-dialog.tsx`, `lib/utils.ts`, provider imports và `sync-i18n.mjs`. Không sửa unrelated code để làm số lint đẹp. Chưa thể nói toàn repository lint sạch.
3. Backend pricing payload chưa cung cấp đủ category/context/modalities/capabilities và không là key-scoped callable list. Docs ghi unknown và dùng `/v1/models` trong examples; không thêm DB field hoặc metadata heuristic.
4. Checked-in OpenAPI chưa bao phủ mọi registered operation và có schema cũ. Reference ghi source/gap, không tự suy ra request/response DTO hoặc fabricate response. Error index không tuyên bố là mọi provider-specific error.
5. Native task-plugin routes phụ thuộc installed/enabled configuration; shipped provider pages không chứng minh availability và không expose private deployment inventory.
6. Sitemap absolute cần public `DOCS_SITE_URL` khi deploy; dynamic model detail không public-snapshot/SSG private catalog.
7. Integration setup đã đối chiếu docs chính chủ, nhưng không có tài khoản/ứng dụng thực tế để certify mọi client feature. Không có Docker daemon run được ghi nhận; build dependency issue được sửa bằng frontend-only-compatible scripts và review.

Không tuyên bố toàn bộ acceptance production đã đóng trong khi các mục 1/2 còn chưa xác minh/giải quyết.

## 14. Follow-up recommendations

- Trên staging, đưa key vào env của máy chạy (không gửi secret vào chat), cài SDK của ba protocol, chạy minimal/stream và đối chiếu Usage Logs/settlement. Dùng model từ key-scoped registry.
- Đặt public `DOCS_SITE_URL`, build lại và kiểm tra HTML/sitemap/robots qua deployment thực; preview build riêng hiện đang ở port 5174 (port 5173 có dev server của workspace).
- Thêm `docs:check` vào CI có checkout đầy đủ/Go để kiểm soát drift khi đổi backend; giữ frontend Docker stage độc lập.
- Bổ sung OpenAPI từ DTO/source và metadata catalog bằng một thay đổi backend được review riêng. Nếu thay đổi DB, thực hiện matrix SQLite/MySQL/PostgreSQL theo policy.
- Sửa baseline lint trong task riêng.

## File inventory

### Created

- `DOCUMENTATION_IMPLEMENTATION_REPORT.md`
- `web/scripts/docs-routes/main.go`
- `web/scripts/generate-documentation.mjs`
- `web/scripts/prerender-documentation.mjs`
- `web/scripts/verify-documentation-browser.mjs`
- `web/scripts/verify-documentation-examples.mjs`
- `web/src/features/documentation/__tests__/metadata.test.tsx`
- `web/src/features/documentation/__tests__/navigation.test.tsx`
- `web/src/features/documentation/__tests__/reference.test.ts`
- `web/src/features/documentation/__tests__/seo.test.ts`
- `web/src/features/documentation/catalog.tsx`
- `web/src/features/documentation/code-examples.tsx`
- `web/src/features/documentation/content-data.json`
- `web/src/features/documentation/content.ts`
- `web/src/features/documentation/generated/reference.json`
- `web/src/features/documentation/index.tsx`
- `web/src/features/documentation/lib.ts`
- `web/src/features/documentation/metadata.tsx`
- `web/src/features/documentation/navigation.tsx`
- `web/src/features/documentation/reference.tsx`
- `web/src/routes/docs/$slug.tsx`
- `web/src/routes/docs/index.tsx`
- `web/src/routes/docs/models/$modelId.tsx`

### Modified

- `web/package.json`: build SSG và scripts generate/check/prerender; không thêm dependencies.
- `web/src/main.tsx`: mount prerender root, cleanup SSG metadata và giữ docs title khi branding refresh.
- `web/src/routeTree.gen.ts`: generated routes/types cho docs.
- `web/src/features/home/components/sections/hero.tsx`: default docs CTA dùng `/docs`, vẫn giữ configured external override và reference URL cũ trong comment.
- `web/src/i18n/static-keys.ts`: dynamic documentation keys.
- `web/src/i18n/locales/en.json`, `vi.json`, `zh.json`, `zh-TW.json`, `fr.json`, `ja.json`, `ru.json`: additive keys; giữ sửa đổi có trước.

### Deleted

Không có file bị xóa bởi phần việc Documentation. Các thay đổi channels/model-actions và deletion `channel-test-hide-failed.tsx` đã có trước task, được giữ nguyên và không thuộc inventory này.

## Localization follow-up

Documentation có bản dịch đầy đủ cho cả bảy locale, bao gồm hướng dẫn xử lý lỗi API. Đã kiểm tra 133 trang qua article registry: không thiếu paragraph translation, giữ nguyên code/link/interpolation tokens. Browser QA xác minh prose và page title khi chuyển cả bảy ngôn ngữ trên cùng route. Production build prerender thành công 133 trang; HTML ban đầu vẫn dùng English source trước khi ứng dụng đọc interface language.

Validation follow-up: full frontend suite 177 files / 2.192 tests PASS, bao gồm 38 tests Documentation; typecheck, lint các file sửa (0 errors), format check và browser QA PASS. Production build PASS, prerender 133 trang.


## Beginner documentation revision — 2026-10-02

The latest user brief supersedes the earlier nine-group main navigation. The public documentation now has exactly 18 beginner pages in six groups: Getting Started, Models & Pricing, API, Integrations, Account, Support. Technical and administration URLs remain accessible, preserving existing New API / QuantumNous content, but are excluded from the main sidebar, search and previous/next flow.

The homepage has one concise hero, three starting steps and five popular guides. A single shared PublicHeader provides Docs, Models, Pricing, search and Console; this removes the overlapping bars reported at 1091px. Existing PublicLayout, CopyButton, Dialog, Tabs, Collapsible, Markdown, StaticDataTable and pricing components are reused. The shared header gained compatible composition slots. DynamicPricingBreakdown exposes billing expressions and internal multipliers, so customer prices instead compose its existing published summary, entry, group and currency APIs. No pricing arithmetic was duplicated; unavailable or conditional prices remain explicit.

Models display five columns and five filters. Categories derive from output modalities and actual endpoint metadata; vision input does not imply image generation. Model examples lead to the supported beginner task guide. Base URLs use the same published API addresses/status source as API Keys, normalize a supplied /v1 suffix, and retain deployment path prefixes. Canonical URLs use the documentation origin.

Quickstart and Text / Chat have short cURL, Python and JavaScript examples using a copied model ID. Media examples follow registered image, video, speech and transcription routes. Video keeps the public task ID, polls actual states, downloads only on completion and avoids creating another task on timeout. Speech checks response content type before saving audio. Shell examples preserve quoted model IDs; media examples isolate shell settings and exit in a subshell, including zsh.

Integrations remain conditional on client/provider compatibility. Claude Code uses the gateway origin, while OpenAI clients use /v1; Cursor customization depends on its version. Account links match /keys, /wallet, /usage-logs/common and /usage-logs/task. Insufficient quota is documented as actual HTTP403, not invented402. Failed requests normally return reserved balance; completed work, policy fees and continuing timed-out tasks can still incur cost.

All18 primary articles, UI text, search and headings resolve through the seven supported locales. Locale files were updated through the sanctioned temporary add-missing-keys.mjs script and i18n:sync. Every locale has zero missing keys; unchanged protocol/brand labels and translated composed legacy metadata are exempt from identical-English reports.

Validation observed: full frontend suite180 files/2236 tests passed; after final task/example adjustments the documentation suite7 files/66 tests passed. Typecheck, scoped oxlint, docs generation/check and production build passed. Build prerenders138 canonical pages, including retained legacy URLs. Twelve cURL examples ran against local protocol fixtures in Bash/zsh, including video creation, status polling and download; Python syntax and JS/TS compilation were verified. Media regression tests execute JavaScript against response fixtures, including failed tasks, HTTP errors, JSON speech responses and multipart transcription. Browser QA verifies desktop/mobile, search, copy, catalogs/access policy, dark mode, Vietnamese1091px header and article/title switching across all seven locales. No paid upstream request or live third-party client certification was performed. Set DOCS_SITE_URL at deployment to generate absolute sitemap/canonical metadata; absent this setting the build intentionally emits relative links.
