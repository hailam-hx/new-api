# DFLOP Pricing Sync

The root admin can configure DFLOP price synchronization under **System settings → Models → DFLOP Pricing Sync**. Select an enabled, single-key DFLOP channel as the **Pricing Source**. The feature is disabled after migration, and legacy settings without `source_channel_id` cannot auto apply. Enter an explicit positive **CNY to USD** rate before previewing; the server does not fetch a foreign exchange rate automatically.

## Source and conversion

The production cost source is authenticated `GET https://api.dflop.top/v1/catalog` with the selected channel credential. Its prices are the effective points charged to that key, including account or platform discounts. The server fetches `https://api.dflop.top/api/v1/models/public` only for global discovery and source-integrity diagnostics. It fetches `https://api.dflop.top/api/v1/config/currency` for `points_per_cny`. For each supported billing component:

```text
cost CNY = DFLOP points / points_per_cny
cost USD = cost CNY × configured CNY-to-USD rate
selling USD = cost USD × configured markup multiplier
```

Decimal arithmetic keeps 24 fractional digits during division and rounds only when serializing prices to 12 decimal places. A zero source price remains zero; a missing price does not become zero.

No Claude, GPT, HTML badge, or family promotional multiplier is applied to authenticated catalog prices. Public list-price promotion inference remains only in legacy diagnostic previews when no source channel is selected; those previews have no apply candidates. The authenticated `schema_version` must be supported (`1.0`), `currency` must be `points`, and `billing.features` declares the pricing shape. Unknown features or new price fields block the affected model and automatic apply. Composite features retain separate prices in each preview item's `prices` components; they are not flattened into one unit.

The channel API key is read from the existing Channel record only at fetch time. Settings and history store the channel ID/name and response metadata, never the credential. The source base URL must resolve to `https://api.dflop.top`; arbitrary URLs and multi-key channels are rejected. A key's `/v1/catalog` is allowlist scoped: an absent model is shown as `NOT_AVAILABLE_TO_SOURCE_KEY` relative to the last-good snapshot and is never interpreted as a global deletion.

Each run stores a compressed effective catalog, normalized source hash, raw catalog digest, `schema_version`, ETag, URL, final URL, HTTP status, Content-Type, timestamp, model counts, and integrity state in existing run fields. `If-None-Match` is sent when a last-good ETag exists. A 304 reuses that catalog while recalculating with current currency and selling settings; an HTTP error never becomes a silent stale success. More than 25% effective catalog shrink blocks apply. Public catalog shrink or a public count below the authenticated count marks `PUBLIC_CATALOG_ANOMALY`; it cannot delete or reprice local models. A deliberate root-admin manual apply is allowed only after explicit confirmation when the authenticated source is otherwise healthy. Automatic apply remains blocked. Current-source errors, unknown schema, currency conflicts, and missing/disabled source channels fail closed.

## Supported mappings

- Plain text token prices become a `tiered_expr` using actual USD per million input, output, cache read, and cache creation tokens when DFLOP supplies those fields.
- Plain image per-image prices become `tier("image", fixed(price)) * image_count`. The image relay reserves the requested image count and settles from valid returned image payloads.
- Exact task plugin bindings support Alibaba Wan per-second resolution tiers, including Wan3 input plus output seconds; Alibaba Qwen Image 3 output and input image counts; and Doubao Seedream 5.0 Pro per-image size tiers with the first input reference free. Plain per-image Seedream models use a plugin override based on successful output count. Missing or incompatible usage profiles remain unsupported. Other video, audio, tiered image, and special text shapes remain in preview for manual review.
- Non-callable models are skipped. Removed source models do not delete local pricing or channels.

## Preview, apply, and rollback

**Sync Now / Preview** stores a source snapshot and a per-model diff without changing live prices. A preview expires after ten minutes. Applying requires its pricing version, selected channel, current configuration, and a fresh authenticated source hash to match; a 304 may reuse the preview snapshot. Model prices are rechecked under the existing pricing option transaction. A run records before and after pricing, item details, and managed model provenance through its source hash and run ID. An existing manual price requires explicit adoption. Later manual edits cause a drift block until an admin deliberately adopts again. A changed pricing shape is blocked even when numeric prices remain the same. Task prices are proposed as `billing_setting.plugin_billing_expr` overrides for the exact plugin and model; model-level prices and other provider overrides are preserved.

Automatic fetch uses the existing leased system task scheduler, so only one instance owns a run. Automatic apply is a separate opt-in setting. It only selects managed, unchanged-by-admin models within the configured increase and decrease limits. New model auto apply requires another opt-in. Rollback restores only models from the selected run and refuses to overwrite later edits.

Apply and rollback refresh the existing billing and exposed pricing caches. The **Sync history** table shows prior runs and offers rollback for applied runs. If a preview or apply fails, check the run details and the server log for the run ID; fetch, malformed source, invalid rate, version conflict, and unsupported mapping leave live prices unchanged.

## Catalog coverage snapshot

At `2026-09-30T01:01:25Z`, a direct unfiltered authenticated request with the configured DFLOP channel returned schema `1.0`, `points`, 212 models (212 callable), and ETag `"cat1-5c76e30cef0e4fc5"`. Its raw SHA-256 was `5c76e30cef0e4fc55b77fb52f810cf525005f32a2eca86fbb61f7e360fbb35c7`. The public URL at `2026-09-30T01:01:24Z` returned 106 models (105 callable), raw SHA-256 `d85614a323138e8d74f86fcb048471f3fe519232dca64885256170e95124f61d`, without an ETag. Both responses were HTTP 200 JSON at their requested URLs, with no provider filter or redirect. The public response contained models from all four observed provider groups, yet 107 authenticated IDs were absent there and one public ID was absent from the key catalog. Thus the Phase 4 count of 106 reflects the public endpoint's observed scope, not a confirmed global removal. The upstream reason for that public scope change remains unverified; production marks it `PUBLIC_CATALOG_ANOMALY`, blocks automatic apply, and requires explicit confirmation for manual apply.

The source-key catalog currently has 99 direct mappings plus five exact task-plugin mappings: **104/212** `SUPPORTED_AUTO` before local scope, ownership, and drift checks; 108 remain unsupported. Operational coverage is reported as verified automatic pricing divided by the selected key's callable models; public-registry coverage is a separate diagnostic. The effective snapshot lists `voice-clone-pro` at 1800 points per successful call and `voice-tts-pro` at 0.132066 points per character for this key only. These amounts can change with the key or catalog. Runtime settlement for those models remains blocked pending exact authoritative usage facts or task binding. DFLOP documents synchronous speech as JSON with `audio_url` and `characters`, while New API's native `/v1/audio/speech` relay serves OpenAI-compatible binary audio and does not pass that character count to settlement. No paid TTS request or response-shape change was made.

## Phase 6 source and settlement audit

At `2026-09-30T01:39:41Z`, the selected channel's authenticated catalog still returned 212 callable models, schema `1.0`, ETag `"cat1-5c76e30cef0e4fc5"`, raw SHA-256 `5c76e30cef0e4fc55b77fb52f810cf525005f32a2eca86fbb61f7e360fbb35c7`. Currency remained 60 points per CNY. The independent public registry returned 106 models, 105 callable, raw SHA-256 `a5de02fad65699f118430ab96813baba39a67de7dd3c63b46fc6b5b52413a9cf`. The effective catalog denominator and verified automatic coverage remain **104/212**.

The centralized integrity policy treats public anomaly as a warning requiring an explicit root-admin acknowledgement for manual apply, while automatic apply stops. Effective collapse, unsupported schema, and unknown integrity conditions block both. An unknown billing feature blocks its affected preview row and automatic apply; other rows remain manually eligible. Apply still checks the source channel, credential fingerprint, current authenticated source hash, config hash, preview age, selected rows, and pricing version. The public response never contributes a price, expression, or model removal.

Evidence for automatic pricing comprises `PRICE_VERIFIED`, `SEMANTICS_VERIFIED`, `RUNTIME_QUANTITY_VERIFIED`, and `SETTLEMENT_VERIFIED`. The preview's authenticated source kind and component prices establish price evidence; billing features establish the shape; exact route/plugin binding and completion facts establish quantity evidence; the existing BillingSession path establishes settlement evidence. `SUPPORTED_AUTO` is the aggregate verdict only when all relevant evidence exists. Submission facts reserve, completion facts determine final quantity, and missing completion usage retains the conservative reservation. No task plugin supplies a monetary amount.

The repository has no exact task-plugin binding for `voice-tts-pro`, `voice-clone-pro`, `suno-v3.5`/`v4`/`v4.5`/`v5`/`v5.5`, or `clip-compose`. The only `/v1/audio/speech` route is the synchronous binary relay; there is no async speech retrieve/list route. The existing `sunoapi` plugin serves `suno_music`/`suno_lyrics` on `/suno/submit` and counts returned clips, which is not the DFLOP per-generation contract. `tvod-midjourney-v7` and `tvod-midjourney-v8.1` lack an exact DFLOP model-to-runtime binding and a captured successful output-image count. All remain blocked. The eight DFLOP Seedance 2.0/2.5 IDs differ from the Doubao plugin's versioned IDs; no exact mapping or captured final composite usage was found. No paid media call was made, so none of these models was promoted merely from documentation.

Current authenticated feature inventory (a model with multiple features appears in multiple rows):

| Billing feature | Models | Blocked | Mapper / runtime / settlement status |
| --- | ---: | ---: | --- |
| `token` | 109 | 30 | Plain token mapper and relay settlement work; composites remain blocked |
| `per_image` | 34 | 12 | Plain image and exact plugin mappings work; other output bindings are unverified |
| `video_second` | 69 | 66 | Three exact Wan mappings work; other bindings or final durations are missing |
| `video_tiers` | 63 | 60 | Same three Wan mappings; remaining tier bindings unverified |
| `video_input_seconds` | 3 | 1 | Two Wan input-video mappings work; one other binding is missing |
| `image_size_bands` | 2 | 1 | Seedream mapping works; Qwen threshold does not match |
| `input_images` | 3 | 1 | Two exact image mappings work; Qwen threshold remains blocked |
| `server_tool_call` | 9 | 9 | Parser and settlement plumbing exist; captured DFLOP final usage is absent (`LIVE_CANARY_REQUIRED`) |
| `token_long_context` | 8 | 8 | Runtime context length exists; complete composite pricing does not |
| `fast_mode` | 8 | 8 | Exact fast-mode pricing and image combination not mapped |
| `video_token` | 8 | 8 | No exact Seedance binding or verified final video-token basis |
| `video_token_formula_seedance_2_0` | 6 | 6 | Composite runtime facts unverified |
| `video_token_formula_seedance_2_5` | 2 | 2 | Composite runtime facts unverified |
| `video_two_stage` | 4 | 4 | Delivered duration and second-stage facts unverified |
| `music` | 5 | 5 | No exact DFLOP generation binding |
| `fixed_output_count` | 2 | 2 | No exact output-image result binding |
| `tts_char` | 1 | 1 | No async task binding with final `characters` |
| `voice_clone` | 1 | 1 | No exact clone task binding |
| `video_task` | 1 | 1 | No exact fixed-task binding |
| `avatar` | 1 | 1 | No exact avatar task binding |

### Current unsupported models by reason

These IDs belong to the authenticated 212-model snapshot above. No price is taken from the public catalog.

- **`IMAGE_TIER_THRESHOLD_MISMATCH` (1):** `qwen-image-3.0-pro`.
- **`LIVE_CANARY_REQUIRED` (9):** `grok-3-mini`, `grok-3-mini-fast`, `grok-4.20-0309-non-reasoning`, `grok-4.20-0309-reasoning`, `grok-4.20-multi-agent-0309`, `grok-4.3`, `grok-4.5`, `grok-4.6`, `grok-4.7`.
- **`MISSING_CACHE_WRITE_MAPPING` (12):** `claude-fable-5`, `claude-fable-5-1`, `claude-haiku-4-5-20251001`, `claude-opus-4-5-20251101`, `claude-opus-4-6`, `claude-opus-4-7`, `claude-opus-4-8`, `claude-opus-5`, `claude-opus-5-5`, `claude-sonnet-4-5-20250929`, `claude-sonnet-4-6`, `claude-sonnet-5`.
- **`MISSING_FAST_MODE_MAPPING` (8):** `gpt-5.5`, `gpt-5.6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-6`, `gpt-6-astra`, `gpt-6-luna`, `gpt-6-sol`.
- **`MISSING_IMAGE_TOKEN_MAPPING` (1):** `codex-auto-review`.
- **`NO_ASYNC_TTS_BINDING` (1):** `voice-tts-pro`.
- **`NO_EXACT_AVATAR_PLUGIN_BINDING` (1):** `dh-avatar-create`.
- **`NO_EXACT_FIXED_TASK_BINDING` (1):** `clip-compose`.
- **`NO_EXACT_MUSIC_PLUGIN_BINDING` (5):** `suno-v3.5`, `suno-v4`, `suno-v4.5`, `suno-v5`, `suno-v5.5`.
- **`NO_EXACT_VIDEO_PLUGIN_BINDING` (58):** `dh-avatar`, `dh-lipsync`, `dh-lipsync-max`, `dh-lipsync-pro`, `dh-motion`, `grok-imagine-video`, `grok-imagine-video-1.5-preview`, `happyhorse-1.0-i2v`, `happyhorse-1.0-r2v`, `happyhorse-1.0-t2v`, `happyhorse-1.0-video-edit`, `happyhorse-1.1-i2v`, `happyhorse-1.1-r2v`, `happyhorse-1.1-t2v`, `minimax-h3`, `minimax-h3-max`, `tvod-hailuo-02`, `tvod-hailuo-2.3`, `tvod-hailuo-2.3-fast`, `tvod-hailuo-h3`, `tvod-hunyuan-video-1.0`, `tvod-hunyuan-video-1.1`, `tvod-jimeng-1.0-lite-i2v`, `tvod-jimeng-1.0-pro`, `tvod-jimeng-1.0-pro-fast`, `tvod-jimeng-1.5-pro`, `tvod-jimeng-3.0`, `tvod-jimeng-3.0-pro`, `tvod-jimeng-4.0`, `tvod-kling-video-1.6`, `tvod-kling-video-2.0`, `tvod-kling-video-2.1`, `tvod-kling-video-2.5-turbo`, `tvod-kling-video-2.6`, `tvod-kling-video-3.0`, `tvod-kling-video-3.0-omni`, `tvod-kling-video-master`, `tvod-kling-video-o1`, `tvod-pixverse-c1`, `tvod-pixverse-v5.6`, `tvod-pixverse-v6`, `tvod-sora-2`, `tvod-subtitle-soft`, `tvod-veo-3.1`, `tvod-veo-3.1-fast`, `tvod-veo-3.1-lite`, `tvod-vidu-video-2.0`, `tvod-vidu-video-q1`, `tvod-vidu-video-q1-classic`, `tvod-vidu-video-q2`, `tvod-vidu-video-q2-pro`, `tvod-vidu-video-q2-pro-fast`, `tvod-vidu-video-q2-turbo`, `tvod-vidu-video-q3`, `tvod-vidu-video-q3-ad`, `tvod-vidu-video-q3-drama`, `tvod-vidu-video-q3-pro`, `tvod-vidu-video-q3-turbo`.
- **`NO_EXACT_VOICE_CLONE_BINDING` (1):** `voice-clone-pro`.
- **`UNVERIFIED_OUTPUT_COUNT` (2):** `tvod-midjourney-v7`, `tvod-midjourney-v8.1`.
- **`UNVERIFIED_VIDEO_TOKEN_USAGE` (8):** `doubao-seedance-2.0`, `doubao-seedance-2.0-fast`, `doubao-seedance-2.0-fast-lite`, `doubao-seedance-2.0-lite`, `doubao-seedance-2.0-mini`, `doubao-seedance-2.0-mini-lite`, `doubao-seedance-2.5`, `doubao-seedance-2.5-lite`.

The formerly broad `UNSUPPORTED_PRICE_SHAPE` group is now split into cache-write, fast-mode, and image-mapping blockers. The former `NO_PLUGIN_USAGE_PROFILE` group is split by exact missing task binding. Grok server-tool models use `LIVE_CANARY_REQUIRED` because the repository has synthetic parser tests but no captured successful DFLOP response; no paid canary was authorized. These classifications improve diagnostics and do not change prices or unlock models.

Historical public-only Phase 1–4 observations follow. They are dated diagnostics, not the current source-key denominator. Refresh the authenticated preview for operational coverage.

- Fetched at: `2026-09-29T14:58:17Z`
- Source hash: `c3501a52df5f5a9df4ed152532e45413a47052df3cb05efcb751791e7628f34d`
- Total: 220; callable: 212; non-callable: 8
- Before this phase: 99 supported, 113 unsupported, 8 non-callable (text 79/30, image 20/5, video 0/71, audio 0/7 supported/unsupported).
- After this phase with built-in task plugins: 104 supported, 108 unsupported, 8 non-callable (text 79/30, image 22/3, video 3/68, audio 0/7 supported/unsupported).
- Phase 3 safety classification, fetched at `2026-09-29T16:11Z`: 104 supported, 108 unsupported, 8 non-callable. Source hash `b29a405f2dfb6158fb2a85285055c68c583b96eaa2c1a9e39a4b68558ca0f4f6`; reason counts are listed below. This hash includes promotion metadata and therefore differs from the earlier snapshot even when catalog values are unchanged.

The preview also checks local configuration, ownership, drift, scope, and the current plugin registry. Those checks can block a model counted as supported in this clean-registry snapshot.

### Unsupported by reason code

- **`PROMOTION_STATE_CHANGED` (12):** Claude catalog badge is 0.8, conflicting with the model-list documentation's 0.6. The effective price is not inferred.
- **`MULTIMODAL_PROMOTION_MAPPING_REQUIRED` (8):** GPT token promotion is verified at 0.3, but the same models also carry image, fast-mode, or long-context billing components without a complete runtime mapping.
- **`AMBIGUOUS_DISCOUNT` (5):** `codex-auto-review` and four Seedance fast/mini variants have no verified price-source semantics for all components.
- **`IMAGE_TIER_THRESHOLD_MISMATCH` (1):** The DFLOP pixel threshold does not match the exact plugin usage tier; an output pixel count or matching upstream tier is needed.
  Models: `qwen-image-3.0-pro`.
- **`MISSING_SERVER_TOOL_USAGE` (9):** The relay does not provide a verified count of billable server-side tool calls for this source price.
  Models: `grok-3-mini`, `grok-3-mini-fast`, `grok-4.20-0309-non-reasoning`, `grok-4.20-0309-reasoning`, `grok-4.20-multi-agent-0309`, `grok-4.3`, `grok-4.5`, `grok-4.6`, `grok-4.7`.
- **`NO_PLUGIN_USAGE_PROFILE` (66):** No exact New API task-plugin model binding with the required usage profile was found; similar vendor names are not treated as bindings.
  Models: `clip-compose`, `dh-avatar`, `dh-avatar-create`, `dh-lipsync`, `dh-lipsync-max`, `dh-lipsync-pro`, `dh-motion`, `grok-imagine-video`, `grok-imagine-video-1.5-preview`, `happyhorse-1.0-i2v`, `happyhorse-1.0-r2v`, `happyhorse-1.0-t2v`, `happyhorse-1.0-video-edit`, `happyhorse-1.1-i2v`, `happyhorse-1.1-r2v`, `happyhorse-1.1-t2v`, `minimax-h3`, `minimax-h3-max`, `suno-v3.5`, `suno-v4`, `suno-v4.5`, `suno-v5`, `suno-v5.5`, `tvod-hailuo-02`, `tvod-hailuo-2.3`, `tvod-hailuo-2.3-fast`, `tvod-hailuo-h3`, `tvod-hunyuan-video-1.0`, `tvod-hunyuan-video-1.1`, `tvod-jimeng-1.0-lite-i2v`, `tvod-jimeng-1.0-pro`, `tvod-jimeng-1.0-pro-fast`, `tvod-jimeng-1.5-pro`, `tvod-jimeng-3.0`, `tvod-jimeng-3.0-pro`, `tvod-jimeng-4.0`, `tvod-kling-video-1.6`, `tvod-kling-video-2.0`, `tvod-kling-video-2.1`, `tvod-kling-video-2.5-turbo`, `tvod-kling-video-2.6`, `tvod-kling-video-3.0`, `tvod-kling-video-3.0-omni`, `tvod-kling-video-master`, `tvod-kling-video-o1`, `tvod-pixverse-c1`, `tvod-pixverse-v5.6`, `tvod-pixverse-v6`, `tvod-sora-2`, `tvod-subtitle-soft`, `tvod-veo-3.1`, `tvod-veo-3.1-fast`, `tvod-veo-3.1-lite`, `tvod-vidu-video-2.0`, `tvod-vidu-video-q1`, `tvod-vidu-video-q1-classic`, `tvod-vidu-video-q2`, `tvod-vidu-video-q2-pro`, `tvod-vidu-video-q2-pro-fast`, `tvod-vidu-video-q2-turbo`, `tvod-vidu-video-q3`, `tvod-vidu-video-q3-ad`, `tvod-vidu-video-q3-drama`, `tvod-vidu-video-q3-pro`, `tvod-vidu-video-q3-turbo`, `voice-clone-pro`.
- **`UNKNOWN_CHARACTER_COUNT_SEMANTICS` (1):** The source does not define a verified billable character-count unit for the available runtime path.
  Models: `voice-tts-pro`.
- **`UNVERIFIED_OUTPUT_COUNT` (2):** A successful output-image count or fixed per-call output contract is not verified for an exact runtime provider.
  Models: `tvod-midjourney-v7`, `tvod-midjourney-v8.1`.
- **`UNVERIFIED_VIDEO_TOKEN_USAGE` (4):** The source has a video-token formula without verified matching runtime token facts and tier semantics.
  Models: `doubao-seedance-2.0`, `doubao-seedance-2.0-lite`, `doubao-seedance-2.5`, `doubao-seedance-2.5-lite`.

The machine-readable investigation report used for this snapshot was generated locally from the catalog and plugin metadata; it is intentionally not committed. Unsupported entries remain `manual_review_required` until their response facts and price semantics can be verified.

## Phase 4 runtime usage audit

The catalog was fetched again at `2026-09-29T23:41:37Z`. Its source hash is `53d515e928ac9a3ab4e2a571865c745b489e5186a5d7c2764f5c283205833628`. This response contains 106 models: 105 callable and one non-callable. With CNY-to-USD `0.15`, markup `1`, and the current exact plugin registry, 44 of the 105 callable models classify as `SUPPORTED_AUTO`; 61 remain unsupported. The earlier 104/212 figure belongs to the dated Phase 3 snapshot above and is not a comparable current-catalog denominator. The source currency response still reports 60 points per CNY.

| Pricing semantics | Runtime quantity and settlement | Current support |
| --- | --- | --- |
| Plain token and image prices | Final token usage or validated returned image count | Supported where the existing exact mapping applies |
| Grok server tools and long context | `usage.num_server_side_tools_used` is preserved as a present-or-missing final fact; text billing expressions can read it as `st`, with reservation using zero and missing final usage retaining the reservation. `len` selects the whole-turn context tier. | Three current Grok models remain blocked: a documented field and parser path do not establish that the deployed DFLOP endpoint reports it for every successful response. No automatic expression is generated. |
| GPT-6 long context and multimodal | `len` can select a whole-turn token tier, but image and fast-mode quantities still need exact mappings. | Seven current GPT multimodal models remain blocked. |
| TTS characters | `/v1/audio/speech` uses a native binary response handler. It currently derives audio duration/tokens and has no authoritative `characters` field to settle or reconcile. | `voice-tts-pro` remains blocked. The catalog's `price_per_tts_char` is `0.132066` points per character; the documented `132.07` points per thousand is rounded. |
| Voice clone, Suno, and digital human task outcomes | Exact model-to-plugin binding and final success quantity are required. Submission estimates may reserve, while a verified final task result must settle success or refund failure. | Blocked; the historical public snapshot showed voice-clone at 1800 points per call. The selected key's authenticated catalog determines the actual amount. |
| Seedance video tokens and Lite upscale | The current Doubao plugin exposes versioned model IDs different from the DFLOP canonical IDs. Final `usage.completion_tokens`, frozen input-video condition, resolution, and final delivered seconds would be needed for the two legs. Requested duration is only a reservation estimate. | Four undiscounted Seedance variants remain blocked pending exact binding and final facts. |

For the 28 current `NO_PLUGIN_USAGE_PROFILE` entries, the exact-binding triage is: `EXACT_BINDING_RUNTIME_FACTS_READY` 0, `EXACT_BINDING_NEEDS_EXTRACTOR` 0, and `BINDING_UNKNOWN` 28. The separate `RUNTIME_FACT_MISSING` group contains the three Grok server-tool models and `voice-tts-pro`; `PRICING_SEMANTICS_UNKNOWN` contains the Claude promotion conflict and the five ambiguous discounts. These groups describe the present evidence, not an assertion that no future plugin can support the model. In particular, similarly named versioned Doubao plugin models do not establish a DFLOP binding.

The historical public Claude Sonnet 4.6 page and catalog showed an 0.8 badge, while the earlier English family reference said 0.6. Neither multiplier enters the authenticated apply path. GPT promotional documentation is likewise diagnostic only. The runtime server-tool count is logged in the existing consume-log `other` JSON as `server_tool_calls` with source `upstream_final_usage` when present; no schema migration was added.

## Phase 7 manual canary planning and evidence policy

The internal `cmd/dflop-canary` CLI defaults to `plan`. Local SQLite usage is `go run ./cmd/dflop-canary plan --channel-id 1 --cny-to-usd 0.15`; replace the channel ID and reporting rate with the operator's current values. For MySQL or PostgreSQL it uses the existing `SQL_DSN` environment setting. The command resolves the existing channel credential through `LoadSourceChannel` and makes only authenticated catalog and currency GET requests. It does not accept an API key flag. It never starts the application scheduler or a migration; the SQLite connection is explicitly read-only, and other databases should use a read-only account. The generated JSON ledger is written with mode `0600` under the OS temporary directory by default.

`execute` parses `--execute`, `--case`, `--max-cost-usd`, and `--confirm-paid-canary`, but **no paid transport is installed in Phase 7**. All current cases remain blocked for missing bounds, route design, semantics, or external fixtures. A future executor must re-fetch the authenticated catalog immediately before a single paid call, recompute the point ceiling and USD estimate, reject any changed price or ceiling, and use a single-invocation authorization. Environment variables, background jobs, tests, health checks, pricing sync, and startup must never trigger a paid canary. A submitted async task ID must be persisted without credentials and resumed by polling; timeout must never cause automatic resubmission.

The provider budget is effective source-key points divided by the catalog's points per CNY and multiplied by the current CNY-to-USD reporting rate. New API selling price and markup are excluded. Unit prices are not a maximum request cost. Cases with unbounded token, character, output, duration, or tool quantities are marked `CANARY_COST_UNBOUNDED`; no execution is allowed. A fixed one-operation catalog price can provide an amount, but the operation route and final settlement semantics still need evidence. The first paid batch must be explicitly approved after every chosen case has a strict upper bound; the present ledger does not provide a defensible total ceiling for TTS, Grok, Suno, or Seedance.

Evidence classes are `SYNTHETIC`, `DOCUMENTATION_EXAMPLE`, `CAPTURED_REAL_RESPONSE`, and `UNKNOWN_ORIGIN`. The offline `verify --input <fixture.json> --case <id> [--plan <ledger.json>]` mode checks fixture-shaped terminal facts and returns a hash of billing-relevant fields. With a plan, it can compute decimal expected points for TTS characters, one-operation fixed prices, and counted image outputs; if the fixture reports points, it compares them exactly and marks unexplained differences. Synthetic fixtures test only the harness. This verifier does not establish that DFLOP returned the shape, that the New API production route extracted the same quantity, or that settlement matched provider charge. It does not yet evaluate composite BillingExpr or reconcile every possible provider cost field. `capture --input <response.json> --case <id> --invocation-id <id> --channel-id <id>` resolves the channel credential in memory and writes a redacted `UNKNOWN_ORIGIN` envelope to the temporary capture directory. The capture helper removes credential and identity fields, local paths, and private media URL content while retaining usage fields and `x-gateway-trace`; operators must inspect the result before classifying it as captured real evidence. No raw capture or generated media is committed automatically. A curated test fixture may be added only after origin, permission, redaction, route identity, final usage, and provider billing semantics have been reviewed.

Future promotion requires separate `PRICE_VERIFIED`, `SEMANTICS_VERIFIED`, `RUNTIME_QUANTITY_VERIFIED`, and `SETTLEMENT_VERIFIED` evidence. One representative response extends to another model only when executing route/plugin, protocol, response usage schema, billing feature vector, and settlement semantics are identical. Source-key amounts can differ. Phase 7 makes no pricing support changes; operational coverage remains **104/212** from the last full pricing preview. The canary ledger's `direct_supported_auto` count excludes five task-plugin mappings and must not be used as operational coverage.
