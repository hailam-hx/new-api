# HOTX Public OpenAPI coverage — Phase 4

## Preflight and implementation plan

Clean baseline: `main`, `efb670890 feat(api): add canonical usage and key-scoped task ownership`. `git status`, `git log -1 --oneline`, `git diff --stat`, and `git diff` observed a clean working tree before changes. Phase 3 has been committed. No Phase 4 commit or push is authorized.

Preserve existing backend contracts, DTOs and beginner navigation. Use an explicit public allowlist, not router serialization. Keep legacy `docs/openapi/relay.json` and management `api.json` as historical schema inputs; publish the separately reviewed `docs/openapi/public.json`. The old relay export contains 43 operations, a duplicate `createImage`, 242 invalid Apifox combination security schemes, stale placeholder operations, and unused administrator DTOs. The documentation declares 108 entries; 32 exact method/path pairs appear in the old export. Wildcard Gemini paths need normalization; plugin routes are conditional contracts, not universally enabled routes. `POST /v1/edits` is a documented limitation and must be excluded from the callable contract.

Implementation sequence:
1. Audit routes, DTOs, security and transport; record one row per public decision below.
2. Add public spec and exact Phase 2–3 DTO/coverage regression checks, preserving existing unique operation IDs. Resolve the duplicate image-edit ID in the new contract only, with compatibility impact recorded.
3. Validate structure, references, examples and security. Add a small compatibility-diff check and test breaking/additive mutations.
4. Generate temporary Python/TypeScript clients and run compile/local transport smoke tests. Do not publish or retain generated vendor clients.
5. Extend existing developer-page renderer with lifecycle, changelog and five public recipes; translate seven locales through the sanctioned script. Keep beginner sidebar unchanged.
6. Audit idempotency, webhook/events and service status as existing support versus design gaps. No new backend infrastructure or migrations.
7. Integrate contract checks into existing CI without mandatory heavy generator downloads, and run Go/frontend/docs/typecheck/build/prerender/browser checks.

Phase 3D remains `BLOCKED_BY_OWNERSHIP_MODEL`; cancellation remains design-only. SDK readiness is not an official SDK release, and `/v1` is not a universal stability promise.

## Coverage matrix



## Source and before/after audit

- Canonical public spec: `docs/openapi/public.json` (OpenAPI 3.0.3). The reviewed compatibility snapshot is `public-contract-baseline.json`, not another endpoint or SDK package.
- Historical inputs: `relay.json` has 43 operations / 40 paths; `api.json` has 161 management operations / 136 paths, no operationIds and 242 invalid Apifox combination schemes. Both are retained unchanged. They are not the public contract. No runtime DTO-to-OpenAPI generator is present; historical exports are manual schema inputs. AST route extraction is inventory tooling, not the publication decision.
- Before: 32/108 exact advertised method/path pairs appeared in relay.json. Presence does not mean schema correctness. Eleven old export pairs lack exact documentation matches: ten Files/Fine-tunes placeholders and one concrete Gemini action represented by a real wildcard. Kling/Jimeng now refer to conditional shipped manifests, not unconditional static routes.
- After: 107/107 callable documentation decisions covered: 78 static operations plus 29 conditional native operations. The 108th documentation entry, POST /v1/edits, explains broken dispatch and is excluded from the callable spec. Enabled deployment routes can be fewer.
- Excluded: Files/Fine-tunes 501 placeholders; Console/Admin DTO baggage; Apifox combination schemes. No speculative final-usage, cancellation, webhook or status operations. Historical protected branding and metadata unchanged.
- Corrected shapes: nullable chat tool content; speech binary/async JSON/SSE; STT text; video JSON/multipart; conditional JSON image edits; Gemini catch-all semantics; decimal/null/omission read DTOs; mandatory native DFLOP headers and conditional host headers; HEAD body absence; scoped media capabilities. Provider response subsets remain deliberately open.
- Operation IDs: existing unique matched IDs retained, including geminiRelayV1Beta mapped to the real wildcard. Duplicate createImage remains on generation; editing uses editImage in the new spec. This is a potentially breaking naming correction for consumers of the ambiguous historical export. No published HOTX SDK or in-repository consumer was found; external use cannot be disproven. Legacy export unchanged.
- Generator: canonical public schemas take precedence; static endpoints gain operationIds and native search uses a separate publicOperations index. Static count remains 405, conditional routes are not invented as static registrations. Historical unmatched export entries total 16 after separating native decisions.

## Matrix interpretation

MATCH indicates exact public read DTOs or explicit byte/upgrade transport. PARTIAL indicates a deliberately open model/provider/plugin payload with documented core fields; it is not an exhaustive schema or universal availability promise. Every callable pair is covered. Errors use description-only responses where no universal envelope is truthful. All operations default conservatively to experimental pending explicit promotion. No stable/SLA/deprecation commitment is invented.

| Method | Path | OperationId | Tag | Summary | Authentication | Request schema | Response schema | Errors | Implementation source | Documentation page | Status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/catalog` | `getDeveloperCatalog` | models | Read the API-key scoped catalog | Bearer API key | Parameters only | application/json: DeveloperCatalog, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError | 400, 401, 403, 429, 500 | controller/developer_catalog.go; dto/developer_catalog.go | /docs/api-reference-models | MATCH |
| GET | `/v1/models` | `listModels` | models | List accessible models | Bearer API key | Parameters only | application/json: ListModelsResponse | 400, 401, 403, 429, 500 | controller/model.go; relaykit/dto/pricing.go | /docs/api-reference-models | PARTIAL |
| POST | `/v1/chat/completions` | `createChatCompletion` | chat | Create a chat completion | Bearer API key | application/json: CreateChatCompletionRequest | application/json: CreateChatCompletionResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/openai_request.go; relay/helper/valid_request.go; relaykit/dto/openai_response.go | /docs/api-reference-chat | PARTIAL |
| POST | `/v1/responses` | `createResponse` | responses | Create a response | Bearer API key | application/json: CreateResponseRequest | application/json: CreateResponseResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/openai_request.go; relay/helper/valid_request.go; router/task-plugin-protocol-router.go | /docs/api-reference-responses | PARTIAL |
| GET | `/v1/responses` | `getV1Responses` | realtime | Connect to Responses WebSocket | Bearer API key | Parameters only | Headers only / upgrade | 400, 401, 403, 429, 500 | controller/responses_websocket.go; relay/responses_websocket.go; middleware/auth.go | /docs/api-reference-realtime | MATCH |
| GET | `/v1/responses/{response_id}` | `getV1ResponsesResponseId` | responses | Retrieve a plugin response | Bearer API key | Parameters only | application/json: GetV1ResponsesResponseIdResponse | 400, 401, 403, 429, 500, 404 | controller/plugin_protocol.go | /docs/api-reference-responses | PARTIAL |
| POST | `/v1/images/generations` | `createImage` | images | Generate images | Bearer API key | application/json: CreateImageRequest | application/json: CreateImageResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/openai_image.go; relay/helper/valid_request.go; pkg/jsplugin/routing.go | /docs/api-reference-images | PARTIAL |
| POST | `/v1/images/edits` | `editImage` | images | Edit images | Bearer API key | multipart/form-data: EditImageRequest, application/json: ImageEditJSONRequest | application/json: EditImageResponse | 400, 401, 403, 429, 500 | relaykit/dto/openai_image.go; relay/helper/valid_request.go; pkg/jsplugin/routing.go | /docs/api-reference-images | PARTIAL |
| POST | `/v1/videos` | `createVideo` | video | Create a video | Bearer API key | multipart/form-data: CreateVideoRequest, application/json: CreateVideoJSONRequest | application/json: CreateVideoResponse | 400, 401, 403, 429, 500 | pkg/jsplugin/routing.go; relay/channel/openai/adaptor.go; relaykit/dto/openai_video.go | /docs/api-reference-video | PARTIAL |
| GET | `/v1/videos/{task_id}` | `getVideo` | video | Retrieve video status | Bearer API key | Parameters only | application/json: GetVideoResponse | 400, 401, 403, 429, 500, 404 | relay/relay_task.go; relaykit/dto/openai_video.go | /docs/api-reference-video | PARTIAL |
| GET | `/v1/videos/{task_id}/content` | `getVideoContent` | video | Retrieve video content | Bearer API key | Parameters only | video/mp4: inline | 400, 401, 403, 429, 500, 404 | controller/video_proxy.go | /docs/api-reference-video | MATCH |
| HEAD | `/v1/videos/{task_id}/content` | `headV1VideosTaskIdContent` | video | Inspect video content headers | Bearer API key | Parameters only | Headers only / upgrade | 400, 401, 403, 429, 500, 404 | controller/video_proxy.go | /docs/api-reference-video | MATCH |
| POST | `/v1/audio/speech` | `createSpeech` | audio | Generate speech | Bearer API key | application/json: CreateSpeechRequest | audio/mpeg: inline, application/json: AsyncSpeechSubmission, audio/*: inline, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/audio.go; router/task-plugin-protocol-router.go; controller/plugin_protocol_audio_speech.go | /docs/api-reference-audio | MATCH |
| GET | `/v1/audio/speech/{task_id}` | `getV1AudioSpeechTaskId` | audio | Retrieve a speech task | Bearer API key | Parameters only | application/json: GetV1AudioSpeechTaskIdResponse | 400, 401, 403, 429, 500, 404 | controller/plugin_protocol_audio_speech.go; dto/task_plugin.go | /docs/api-reference-audio | PARTIAL |
| POST | `/v1/audio/transcriptions` | `createTranscription` | audio | Transcribe audio | Bearer API key | multipart/form-data: CreateTranscriptionRequest | application/json: CreateTranscriptionResponse, text/plain: inline | 400, 401, 403, 429, 500 | relay/audio_handler.go; relaykit/dto/audio.go; relay/channel/openai/audio.go | /docs/api-reference-audio | PARTIAL |
| GET | `/v1/models/{model}` | `getV1ModelsModel` | models | Retrieve model metadata | Bearer API key | Parameters only | application/json: GetV1ModelsModelResponse | 400, 401, 403, 429, 500, 404 | controller/model.go | /docs/api-reference-models | PARTIAL |
| GET | `/v1beta/models` | `listModelsGemini` | models | List accessible models | Bearer API key | Parameters only | application/json: ListModelsGeminiResponse | 400, 401, 403, 429, 500 | controller/model.go | /docs/api-reference-models | PARTIAL |
| GET | `/v1beta/openai/models` | `getV1betaOpenaiModels` | models | List accessible models | Bearer API key | Parameters only | application/json: GetV1betaOpenaiModelsResponse | 400, 401, 403, 429, 500 | controller/model.go | /docs/api-reference-models | PARTIAL |
| POST | `/v1/completions` | `createCompletion` | chat | Create a text completion | Bearer API key | application/json: CreateCompletionRequest | application/json: CreateCompletionResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/openai_request.go; relaykit/dto/openai_response.go | /docs/api-reference-chat | PARTIAL |
| POST | `/v1/responses/compact` | `compactResponse` | responses | Compact a response conversation | Bearer API key | application/json: CompactResponseRequest | application/json: CompactResponseResponse | 400, 401, 403, 429, 500 | relaykit/dto/openai_responses_compaction_request.go; relaykit/dto/openai_compaction.go | /docs/api-reference-responses | PARTIAL |
| POST | `/v1/messages` | `createMessage` | anthropic | Create an Anthropic message | Bearer API key | application/json: CreateMessageRequest | application/json: CreateMessageResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/claude.go; relay/helper/valid_request.go | /docs/api-reference-anthropic | PARTIAL |
| POST | `/v1/models/{path}` | `postV1ModelsPath` | gemini | Call a Gemini native action | Bearer API key | application/json: PostV1ModelsPathRequest | application/json: PostV1ModelsPathResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/gemini.go; relay/helper/valid_request.go; relay/channel/gemini/adaptor.go | /docs/api-reference-gemini | PARTIAL |
| POST | `/v1beta/models/{path}` | `geminiRelayV1Beta` | gemini | Call a Gemini native action | Bearer API key | application/json: PostV1betaModelsPathRequest | application/json: PostV1betaModelsPathResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/gemini.go; relay/helper/valid_request.go; relay/channel/gemini/adaptor.go | /docs/api-reference-gemini | PARTIAL |
| POST | `/v1/engines/{model}/embeddings` | `createEngineEmbedding` | gemini | Create an engine embedding | Bearer API key | application/json: CreateEngineEmbeddingRequest | application/json: CreateEngineEmbeddingResponse | 400, 401, 403, 429, 500 | relay/gemini_handler.go; relaykit/dto/gemini.go | /docs/api-reference-gemini | PARTIAL |
| POST | `/v1/embeddings` | `createEmbedding` | utilities | Create embeddings | Bearer API key | application/json: CreateEmbeddingRequest | application/json: CreateEmbeddingResponse | 400, 401, 403, 429, 500 | relaykit/dto/embedding.go; relay/helper/valid_request.go | /docs/api-reference-utilities | PARTIAL |
| POST | `/v1/rerank` | `createRerank` | utilities | Rank documents by relevance | Bearer API key | application/json: CreateRerankRequest | application/json: CreateRerankResponse | 400, 401, 403, 429, 500 | relaykit/dto/rerank.go | /docs/api-reference-utilities | PARTIAL |
| POST | `/v1/moderations` | `createModeration` | utilities | Moderate content | Bearer API key | application/json: CreateModerationRequest | application/json: CreateModerationResponse | 400, 401, 403, 429, 500 | relaykit/dto/openai_request.go; relay/helper/valid_request.go; relay/channel/openai/relay-openai.go | /docs/api-reference-utilities | PARTIAL |
| POST | `/v1/alpha/search` | `postV1AlphaSearch` | utilities | Run a standalone web search | Bearer API key | application/json: PostV1AlphaSearchRequest | application/json: PostV1AlphaSearchResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | relaykit/dto/alpha_search_request.go; relay/alpha_search_handler.go | /docs/api-reference-utilities | PARTIAL |
| GET | `/v1/realtime` | `createRealtimeSession` | realtime | Connect to Realtime WebSocket | Bearer API key | Parameters only | Headers only / upgrade | 400, 401, 403, 429, 500 | relaykit/dto/realtime.go; relay/channel/openai/relay_realtime.go; middleware/auth.go | /docs/api-reference-realtime | MATCH |
| POST | `/v1/tasks/{key}` | `postV1TasksKey` | tasks | Submit a plugin task | Bearer API key | application/json: PostV1TasksKeyRequest | application/json: PostV1TasksKeyResponse | 400, 401, 403, 429, 500 | router/task-router.go; middleware/task_plugin.go; controller/relay.go | /docs/api-reference-tasks | PARTIAL |
| GET | `/v1/tasks/{key}` | `getDeveloperTask` | tasks | Retrieve a task created by the current API key | Bearer API key | Parameters only | application/json: DeveloperTask, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError | 400, 401, 403, 429, 500, 404 | controller/developer_tasks.go; dto/developer_tasks.go | /docs/api-reference-tasks | MATCH |
| GET | `/v1/tasks` | `getDeveloperTasks` | tasks | List tasks created by the current API key | Bearer API key | Parameters only | application/json: DeveloperTaskList, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError | 400, 401, 403, 429, 500 | controller/developer_tasks.go; dto/developer_tasks.go | /docs/api-reference-tasks | MATCH |
| GET | `/v1/tasks/{key}/artifacts` | `getV1TasksKeyArtifacts` | tasks | List task artifacts | Bearer API key | Parameters only | application/json: GetV1TasksKeyArtifactsResponse | 400, 401, 403, 429, 500, 404 | controller/task.go | /docs/api-reference-tasks | PARTIAL |
| GET | `/v1/tasks/{key}/artifacts/{artifact_key}/content` | `getV1TasksKeyArtifactsArtifactKeyContent` | tasks | Retrieve artifact content | Bearer API key OR scoped artifact capability | Parameters only | application/octet-stream: inline | 400, 401, 403, 429, 500, 404 | controller/task.go; middleware/auth.go | /docs/api-reference-tasks | MATCH |
| HEAD | `/v1/tasks/{key}/artifacts/{artifact_key}/content` | `headV1TasksKeyArtifactsArtifactKeyContent` | tasks | Inspect artifact content headers | Bearer API key OR scoped artifact capability | Parameters only | Headers only / upgrade | 400, 401, 403, 429, 500, 404 | controller/task.go; middleware/auth.go | /docs/api-reference-tasks | MATCH |
| GET | `/dashboard/billing/subscription` | `getDashboardBillingSubscription` | billing | Read legacy billing limits | Bearer API key | Parameters only | application/json: GetDashboardBillingSubscriptionResponse | 400, 401, 403, 429, 500 | controller/billing.go | /docs/api-reference-billing | PARTIAL |
| GET | `/v1/dashboard/billing/subscription` | `getV1DashboardBillingSubscription` | billing | Read legacy billing limits | Bearer API key | Parameters only | application/json: GetV1DashboardBillingSubscriptionResponse | 400, 401, 403, 429, 500 | controller/billing.go | /docs/api-reference-billing | PARTIAL |
| GET | `/dashboard/billing/usage` | `getDashboardBillingUsage` | billing | Read legacy billing usage | Bearer API key | Parameters only | application/json: GetDashboardBillingUsageResponse | 400, 401, 403, 429, 500 | controller/billing.go | /docs/api-reference-billing | PARTIAL |
| GET | `/v1/dashboard/billing/usage` | `getV1DashboardBillingUsage` | billing | Read legacy billing usage | Bearer API key | Parameters only | application/json: GetV1DashboardBillingUsageResponse | 400, 401, 403, 429, 500 | controller/billing.go | /docs/api-reference-billing | PARTIAL |
| GET | `/v1/key/usage` | `getDeveloperKeyUsage` | billing | Read recorded usage for the current API key | Bearer API key | Parameters only | application/json: DeveloperKeyUsage, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError | 400, 401, 403, 429, 500 | controller/developer_usage.go; dto/developer_usage.go | /docs/api-reference-billing | MATCH |
| GET | `/v1/key/balance` | `getDeveloperKeyBalance` | billing | Read the current API key balance | Bearer API key | Parameters only | application/json: DeveloperKeyBalance, application/json: DeveloperAPIError, application/json: DeveloperAPIError, application/json: DeveloperAPIError | 400, 401, 403, 429, 500 | controller/developer_balance.go; dto/developer_balance.go | /docs/api-reference-billing | MATCH |
| GET | `/{mode}/mj/image/{id}` | `getModeMjImageId` | midjourney | Retrieve a Midjourney image | Bearer API key OR scoped image + expires capability | Parameters only | image/*: inline | 400, 401, 403, 429, 500, 404 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | MATCH |
| POST | `/{mode}/mj/insight-face/swap` | `postModeMjInsightFaceSwap` | midjourney | Swap faces in images | Bearer API key | application/json: PostModeMjInsightFaceSwapRequest | application/json: PostModeMjInsightFaceSwapResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/action` | `postModeMjSubmitAction` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitActionRequest | application/json: PostModeMjSubmitActionResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/blend` | `postModeMjSubmitBlend` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitBlendRequest | application/json: PostModeMjSubmitBlendResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/change` | `postModeMjSubmitChange` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitChangeRequest | application/json: PostModeMjSubmitChangeResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/describe` | `postModeMjSubmitDescribe` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitDescribeRequest | application/json: PostModeMjSubmitDescribeResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/edits` | `postModeMjSubmitEdits` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitEditsRequest | application/json: PostModeMjSubmitEditsResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/imagine` | `postModeMjSubmitImagine` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitImagineRequest | application/json: PostModeMjSubmitImagineResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/modal` | `postModeMjSubmitModal` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitModalRequest | application/json: PostModeMjSubmitModalResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/shorten` | `postModeMjSubmitShorten` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitShortenRequest | application/json: PostModeMjSubmitShortenResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/simple-change` | `postModeMjSubmitSimpleChange` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitSimpleChangeRequest | application/json: PostModeMjSubmitSimpleChangeResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/upload-discord-images` | `postModeMjSubmitUploadDiscordImages` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitUploadDiscordImagesRequest | application/json: PostModeMjSubmitUploadDiscordImagesResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/submit/video` | `postModeMjSubmitVideo` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostModeMjSubmitVideoRequest | application/json: PostModeMjSubmitVideoResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| GET | `/{mode}/mj/task/{id}/fetch` | `getModeMjTaskIdFetch` | midjourney | Retrieve a Midjourney task | Bearer API key | Parameters only | application/json: GetModeMjTaskIdFetchResponse | 400, 401, 403, 429, 500, 404 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| GET | `/{mode}/mj/task/{id}/image-seed` | `getModeMjTaskIdImageSeed` | midjourney | Retrieve a Midjourney image seed | Bearer API key | Parameters only | application/json: GetModeMjTaskIdImageSeedResponse | 400, 401, 403, 429, 500, 404 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/{mode}/mj/task/list-by-condition` | `postModeMjTaskListByCondition` | midjourney | Retrieve Midjourney tasks by ID | Bearer API key | application/json: PostModeMjTaskListByConditionRequest | application/json: PostModeMjTaskListByConditionResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| GET | `/mj/image/{id}` | `getMjImageId` | midjourney | Retrieve a Midjourney image | Bearer API key OR scoped image + expires capability | Parameters only | image/*: inline | 400, 401, 403, 429, 500, 404 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | MATCH |
| POST | `/mj/insight-face/swap` | `postMjInsightFaceSwap` | midjourney | Swap faces in images | Bearer API key | application/json: PostMjInsightFaceSwapRequest | application/json: PostMjInsightFaceSwapResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/action` | `postMjSubmitAction` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitActionRequest | application/json: PostMjSubmitActionResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/blend` | `postMjSubmitBlend` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitBlendRequest | application/json: PostMjSubmitBlendResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/change` | `postMjSubmitChange` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitChangeRequest | application/json: PostMjSubmitChangeResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/describe` | `postMjSubmitDescribe` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitDescribeRequest | application/json: PostMjSubmitDescribeResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/edits` | `postMjSubmitEdits` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitEditsRequest | application/json: PostMjSubmitEditsResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/imagine` | `postMjSubmitImagine` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitImagineRequest | application/json: PostMjSubmitImagineResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/modal` | `postMjSubmitModal` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitModalRequest | application/json: PostMjSubmitModalResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/shorten` | `postMjSubmitShorten` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitShortenRequest | application/json: PostMjSubmitShortenResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/simple-change` | `postMjSubmitSimpleChange` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitSimpleChangeRequest | application/json: PostMjSubmitSimpleChangeResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/upload-discord-images` | `postMjSubmitUploadDiscordImages` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitUploadDiscordImagesRequest | application/json: PostMjSubmitUploadDiscordImagesResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/submit/video` | `postMjSubmitVideo` | midjourney | Submit a Midjourney operation | Bearer API key | application/json: PostMjSubmitVideoRequest | application/json: PostMjSubmitVideoResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| GET | `/mj/task/{id}/fetch` | `getMjTaskIdFetch` | midjourney | Retrieve a Midjourney task | Bearer API key | Parameters only | application/json: GetMjTaskIdFetchResponse | 400, 401, 403, 429, 500, 404 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| GET | `/mj/task/{id}/image-seed` | `getMjTaskIdImageSeed` | midjourney | Retrieve a Midjourney image seed | Bearer API key | Parameters only | application/json: GetMjTaskIdImageSeedResponse | 400, 401, 403, 429, 500, 404 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/mj/task/list-by-condition` | `postMjTaskListByCondition` | midjourney | Retrieve Midjourney tasks by ID | Bearer API key | application/json: PostMjTaskListByConditionRequest | application/json: PostMjTaskListByConditionResponse | 400, 401, 403, 429, 500 | dto/midjourney.go; relay/mjproxy_handler.go | /docs/api-reference-midjourney | PARTIAL |
| POST | `/v1/videos/generations` | `postV1VideosGenerations` | video | Create a video | Bearer API key | multipart/form-data: PostV1VideosGenerationsRequest, application/json: PostV1VideosGenerationsJSONRequest | application/json: PostV1VideosGenerationsResponse | 400, 401, 403, 429, 500 | pkg/jsplugin/routing.go; relay/channel/openai/adaptor.go; relaykit/dto/openai_video.go | /docs/api-reference-video | PARTIAL |
| POST | `/v1/video/generations` | `createVideoGeneration` | video | Create a video | Bearer API key | multipart/form-data: CreateVideoGenerationRequest, application/json: CreateVideoGenerationJSONRequest | application/json: CreateVideoGenerationResponse | 400, 401, 403, 429, 500 | pkg/jsplugin/routing.go; relay/channel/openai/adaptor.go; relaykit/dto/openai_video.go | /docs/api-reference-video | PARTIAL |
| GET | `/v1/video/generations/{task_id}` | `getVideoGeneration` | video | Retrieve video status | Bearer API key | Parameters only | application/json: GetVideoGenerationResponse | 400, 401, 403, 429, 500, 404 | relay/relay_task.go; relaykit/dto/openai_video.go | /docs/api-reference-video | PARTIAL |
| POST | `/v1/videos/{video_id}/remix` | `postV1VideosVideoIdRemix` | video | Remix a video | Bearer API key | multipart/form-data: PostV1VideosVideoIdRemixRequest | application/json: PostV1VideosVideoIdRemixResponse | 400, 401, 403, 429, 500 | router/video-router.go; relay/channel/openai/adaptor.go | /docs/api-reference-video | PARTIAL |
| POST | `/v1/audio/translations` | `createTranslation` | audio | Translate audio | Bearer API key | multipart/form-data: CreateTranslationRequest | application/json: CreateTranslationResponse, text/plain: inline | 400, 401, 403, 429, 500 | relay/audio_handler.go; relaykit/dto/audio.go | /docs/api-reference-audio | PARTIAL |
| POST | `/ali/api/v1/services/aigc/multimodal-generation/generation` | `postAliApiV1ServicesAigcMultimodalGenerationGeneration` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostAliApiV1ServicesAigcMultimodalGenerationGenerationRequest | application/json: PostAliApiV1ServicesAigcMultimodalGenerationGenerationResponse | 400, 401, 403, 429, 500 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/ali/api/v1/services/aigc/image-generation/generation` | `postAliApiV1ServicesAigcImageGenerationGeneration` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostAliApiV1ServicesAigcImageGenerationGenerationRequest | application/json: PostAliApiV1ServicesAigcImageGenerationGenerationResponse | 400, 401, 403, 429, 500 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/ali/api/v1/services/aigc/text2image/image-synthesis` | `postAliApiV1ServicesAigcText2imageImageSynthesis` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostAliApiV1ServicesAigcText2imageImageSynthesisRequest | application/json: PostAliApiV1ServicesAigcText2imageImageSynthesisResponse | 400, 401, 403, 429, 500 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/ali/api/v1/services/aigc/image2image/image-synthesis` | `postAliApiV1ServicesAigcImage2imageImageSynthesis` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostAliApiV1ServicesAigcImage2imageImageSynthesisRequest | application/json: PostAliApiV1ServicesAigcImage2imageImageSynthesisResponse | 400, 401, 403, 429, 500 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/ali/api/v1/services/aigc/video-generation/video-synthesis` | `postAliApiV1ServicesAigcVideoGenerationVideoSynthesis` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostAliApiV1ServicesAigcVideoGenerationVideoSynthesisRequest | application/json: PostAliApiV1ServicesAigcVideoGenerationVideoSynthesisResponse | 400, 401, 403, 429, 500 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/ali/api/v1/services/aigc/image2video/video-synthesis` | `postAliApiV1ServicesAigcImage2videoVideoSynthesis` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostAliApiV1ServicesAigcImage2videoVideoSynthesisRequest | application/json: PostAliApiV1ServicesAigcImage2videoVideoSynthesisResponse | 400, 401, 403, 429, 500 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/ali/api/v1/tasks/{task_id}` | `getAliApiV1TasksTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetAliApiV1TasksTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/alibaba/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/dflop-image/v1/images/generations` | `postDflopImageV1ImagesGenerations` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDflopImageV1ImagesGenerationsRequest | application/json: PostDflopImageV1ImagesGenerationsResponse | 400, 401, 403, 429, 500 | plugins/tasks/dflop-image/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/dflop-image/v1/images/edits` | `postDflopImageV1ImagesEdits` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDflopImageV1ImagesEditsRequest | application/json: PostDflopImageV1ImagesEditsResponse | 400, 401, 403, 429, 500 | plugins/tasks/dflop-image/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/dflop-image/v1/images/generations/{task_id}` | `getDflopImageV1ImagesGenerationsTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetDflopImageV1ImagesGenerationsTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/dflop-image/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/dflop/v1/videos/generations` | `postDflopV1VideosGenerations` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDflopV1VideosGenerationsRequest | application/json: PostDflopV1VideosGenerationsResponse | 400, 401, 403, 429, 500 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/dflop/v1/videos/generations/{task_id}` | `getDflopV1VideosGenerationsTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetDflopV1VideosGenerationsTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/dflop/v1/music/generations` | `postDflopV1MusicGenerations` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDflopV1MusicGenerationsRequest | application/json: PostDflopV1MusicGenerationsResponse | 400, 401, 403, 429, 500 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/dflop/v1/music/generations/{task_id}` | `getDflopV1MusicGenerationsTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetDflopV1MusicGenerationsTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/dflop/v1/audio/voices` | `postDflopV1AudioVoices` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDflopV1AudioVoicesRequest | application/json: PostDflopV1AudioVoicesResponse | 400, 401, 403, 429, 500 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/dflop/v1/audio/voices/{task_id}` | `getDflopV1AudioVoicesTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetDflopV1AudioVoicesTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/dflop/v1/videos/avatars` | `postDflopV1VideosAvatars` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDflopV1VideosAvatarsRequest | application/json: PostDflopV1VideosAvatarsResponse | 400, 401, 403, 429, 500 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/dflop/v1/videos/avatars/{task_id}` | `getDflopV1VideosAvatarsTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetDflopV1VideosAvatarsTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/dflop-media/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/doubao/api/v3/contents/generations/tasks` | `postDoubaoApiV3ContentsGenerationsTasks` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDoubaoApiV3ContentsGenerationsTasksRequest | application/json: PostDoubaoApiV3ContentsGenerationsTasksResponse | 400, 401, 403, 429, 500 | plugins/tasks/doubao/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/doubao/api/v3/contents/generations/tasks/{task_id}` | `getDoubaoApiV3ContentsGenerationsTasksTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetDoubaoApiV3ContentsGenerationsTasksTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/doubao/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/doubao/api/v3/images/generations` | `postDoubaoApiV3ImagesGenerations` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostDoubaoApiV3ImagesGenerationsRequest | application/json: PostDoubaoApiV3ImagesGenerationsResponse, text/event-stream: inline | 400, 401, 403, 429, 500 | plugins/tasks/doubao/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/jimeng/` | `createJimengVideo` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: CreateJimengVideoRequest | application/json: CreateJimengVideoResponse | 400, 401, 403, 429, 500 | plugins/tasks/jimeng/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/kling/v1/videos/text2video` | `createKlingText2Video` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: CreateKlingText2VideoRequest | application/json: CreateKlingText2VideoResponse | 400, 401, 403, 429, 500 | plugins/tasks/kling/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/kling/v1/videos/image2video` | `createKlingImage2Video` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: CreateKlingImage2VideoRequest | application/json: CreateKlingImage2VideoResponse | 400, 401, 403, 429, 500 | plugins/tasks/kling/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/kling/v1/videos/text2video/{task_id}` | `getKlingText2Video` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetKlingText2VideoResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/kling/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/kling/v1/videos/image2video/{task_id}` | `getKlingImage2Video` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetKlingImage2VideoResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/kling/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/suno/submit/{action}` | `postSunoSubmitAction` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostSunoSubmitActionRequest | application/json: PostSunoSubmitActionResponse | 400, 401, 403, 429, 500 | plugins/tasks/sunoapi/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/suno/fetch` | `postSunoFetch` | plugin-routes | Call a native plugin operation | Bearer API key | application/json: PostSunoFetchRequest | application/json: PostSunoFetchResponse | 400, 401, 403, 429, 500 | plugins/tasks/sunoapi/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| GET | `/suno/fetch/{task_id}` | `getSunoFetchTaskId` | plugin-routes | Read a native plugin task | Bearer API key | Parameters only | application/json: GetSunoFetchTaskIdResponse | 400, 401, 403, 429, 500, 404 | plugins/tasks/sunoapi/plugin.js | /docs/api-reference-plugin-routes | PARTIAL |
| POST | `/v1/edits` | — | images | Broken dispatch, unusable image edit | Bearer | Not published | Not published | 400 | router/relay-router.go; controller/relay.go | API Images limitation | EXCLUDED |
| Various | `/v1/files*`, `/v1/fine-tunes*` | Historical IDs only | historical | Explicit 501 placeholders | Bearer | Not published | Not published | 501 | RelayNotImplemented | Not advertised callable | NOT_IMPLEMENTED / EXCLUDED |
| Various | `/api/*`, admin/bootstrap/worker routes | — | management | Console/Admin | session/PAT/admin-specific | Not published | Not published | Varies | Management routers | Console docs | EXCLUDED |

## Validation and compatibility gate

`bun run docs:contract` checks the explicit allowlist against current generated routes and shipped native declarations, local references, operationIds/equivalent paths, security references, required path parameters, responses, required properties, array items and request/response examples. Duplicate JSON keys are rejected. Full validation additionally uses temporary `openapi-spec-validator==0.9.0`, without a product dependency. SDK compiler/mock results are in SDK_READINESS_REPORT.md.

The snapshot resolves schema refs. Removed paths/operations/parameters, required request additions, response field removal, type/format/nullability changes and removed input content types are BREAKING; operationId, authentication, enum and constraint changes are POTENTIALLY_BREAKING; optional additions are ADDITIVE. Non-additive changes fail until deliberately reviewed. This is a compatibility guard, not proof of semantic/provider backward compatibility; reviewers still assess behavior and configuration. No baseline check can establish undeclared provider behavior.

CI checks generated docs, allowlisted contract coverage/diff and formal OpenAPI validation. SDK generation is an opt-in workflow so normal CI avoids generator downloads. Frontend tooling uses Node 24; Bun remains installer/script runner. No dependency or lockfile changes. GitHub-hosted jobs were not dispatched by this task; local equivalent commands were executed.

## HOTX DEVELOPER EXPERIENCE — PHASE 4 REPORT

1. **Git baseline:** clean `main` at `efb670890`; preflight passed. Phase 3 and Phase 4 are not mixed. No staging/commit/push performed in Phase 4.
2. **Public OpenAPI source:** separate canonical `docs/openapi/public.json`, explicit advertised/public allowlist plus real route/manifest/DTO evidence. Existing management and historical relay exports preserved.
3. **Coverage before:** 32/108 exact documented pairs appeared in the 43-operation old relay export; invalid security and stale schemas prevent treating that as readiness.
4. **Coverage after:** 107/107 callable public pairs; 78 static and 29 conditional native. One additional docs entry records the unusable legacy edits route and is excluded. See full matrix above for MATCH/PARTIAL distinctions.
5. **Stale fixed:** old placeholders, duplicate image ID, security schemes, nullable tool-only chat content, speech SSE/JSON/binary, video body formats, typed read nullability/decimal/omission, required native DFLOP headers, signed content auth and conditional deployment semantics.
6. **Missing added:** all 75 callable documentation pairs missing exact legacy coverage are represented, including models detail, Gemini wildcard/alias, WebSocket Responses, host task/artifact reads/submission, video aliases/remix, Midjourney/legacy billing and admitted native manifests. Existing Phase 2–3 operations retained with exact DTOs. No new backend route.
7. **Internal excluded:** no /api, Admin, worker/bootstrap, disabled placeholders or speculative Phase 3D operations in public.json; unused User/Channel/Token/Log schemas are not copied into it.
8. **Validation:** complete temporary openapi-spec-validator validation PASS; coverage/ref/security/required/examples/duplicate-key checks PASS. Zero invalid refs, duplicate IDs, extra operations or missing callable decisions.
9. **Contract tests:** public allowlist/native-route checks; all twelve Phase 2–3 DTO structs; decimal/null fields and task status enum; binary/SSE/WebSocket/HEAD; malformed examples, duplicate IDs/keys/refs; breaking removals/required additions/type/auth/enum change and additive fields. Nine focused tests pass. Review findings were reproduced and fixed.
10. **SDK readiness:** TypeScript/Python full operation generation plus minimal compile/local transport smoke PASS; no SDK publication or generated vendor code in repository. Go NOT_TESTED (optional).
11. **Python smoke:** 107 endpoint modules generated, complete byte compilation and typed sample pass; six JSON mock calls plus one binary call pass. Binary media uses documented generator configuration without changing actual spec Content-Type. Auxiliary strict whole-package mypy has twelve generator-template context-manager errors; see SDK report.
12. **TypeScript smoke:** 107 operations generated; strict TypeScript 5.9.3 sample and seven mock calls pass. No invented resource-method naming or SDK package promise.
13. **Versioning:** new developer API Versioning page. /v1 namespace, unknown fields, breaking changes and separate catalog schema major documented. Catalog remains schema_version 1.0.
14. **Stability:** conservative experimental initial default, explicit promotion required; stable/beta/experimental/deprecated are distinct. No SLA or blanket /v1 stability.
15. **Changelog:** new API Changelog starts Unreleased at the current baseline; no invented historical release dates or copied commit history.
16. **Deprecation:** announcement/reason/replacement/migration/shutdown-date format, no invented sunset date/notice period; legacy/deprecated/removed distinguished. No deprecated flag set without an announcement.
17. **Recipes:** five public workflows: catalog-to-chat, decimal/null-aware allowance check and one submission, retained usage window, saved video task with bounded polling/content, key-scoped opaque pagination. Existing DocsCode/CopyButton used; same builders produce SSG and React output. Bash syntax and Python compile pass; Python balance flow executed against local fixtures. Complete cURL execution remains unverified because jq is absent, a documented recipe prerequisite.
18. **Idempotency:** GAP_IDENTIFIED / DESIGN_ONLY for general same-result replay. Existing DFLOP/speech header validation and fail-closed billing barriers documented; no universal TTL, request-hash conflict or replay guarantee introduced. Paid async retries remain acceptance-sensitive.
19. **Webhook/events:** GAP_IDENTIFIED / DESIGN_ONLY. Payment callbacks and account notifications do not establish a task-event subscription/delivery contract. Proposed separate signing secrets/event IDs/retries/rotation/replay/retention documented, no infrastructure implemented.
20. **Status/health:** GAP_IDENTIFIED / DESIGN_ONLY. Console site settings/Uptime Kuma/admin checks are not a public service-status contract. No internal probe exposed and no hostname invented.
21. **CI:** normal CI adds docs freshness, public coverage/diff, formal OpenAPI; Node 24 frontend runtime and Go parser setup. Optional public-sdk workflow runs temporary generation/mock checks. These jobs were not dispatched; equivalent local commands tested.
22. **Documentation:** three developer pages, existing Errors & Retry enriched, operationId/native search, seven locales, unchanged beginner sidebar and shared render components. DOCS_SITE_URL build guidance clarified; operator origin remains unset/release checklist item.
23. **Files created:** canonical spec and reviewed baseline; requested OpenAPI/SDK/idempotency/webhook/status reports; small contract validator/diff script + declaration; SDK smoke script; focused public contract test; recipe builder/component; optional SDK workflow. No plugin-directory docs or generated SDK client files.
24. **Files modified:** existing CI/.env.example/package scripts, documentation generator/prerender/browser harness, developer semantics/render/search, reference/SEO tests, generated reference and seven locale files. No Go/backend/DB/lockfile dependencies modified.
25. **Tests:** full Go 5,445 test/subtest PASS, 0 FAIL, 76 environment-gated SKIP across 87 packages. Full frontend final 2,362 PASS / 185 files / 0 FAIL. Typecheck, build, 161-page prerender, docs check, formal OpenAPI, full operation generation, mock SDK transports and focused contracts pass. Full lint remains 182 baseline errors/66 warnings; all changed files have zero errors/warnings. No DB matrix required because Phase 4 changes no DB behavior. Browser final result is recorded in the verification section below.
26. **Limits:** provider/plugin payloads remain open core subsets, deployments can omit native operations; full generated Python mypy template issue; streaming/WebSocket runtime not SDK-smoke tested; jq cURL execution gap; DOCS_SITE_URL operator setting; CI not remotely dispatched; no final-usage/cancellation unblock. Intermediate Bun-runtime full suite failed (module errors: 54 suites, 40 tests, 29 uncaught errors); Node 24 full suite then passed. Initial browser attempts failed due missing local server, wrong working directory, inactive tab-transition selector and actual missing developer search scope; harness selection was narrowed to non-inert content, with final outcome below. No unrelated product behavior was changed to make these harness failures disappear.
27. **Phase 5 recommendations:** decide durable accepted-operation/final-ledger event ownership before unblocking final usage; product-review endpoint/payload-bound idempotency before paid async retry guarantees; define task event delivery/signing/retention and public status ownership; model-specific schema refinement and SDK streaming protocol wrappers; fix generator-template strict typing if publishing an official SDK; configure production DOCS_SITE_URL before release.
28. **Diff:** final git status and diff statistics recorded below. Git remains on the initial Phase 3 commit; no Phase 4 commit/push/stage.

Paid upstream requests: **0**. Network upstream/provider requests: **0**. Tool package downloads are separate from upstream API traffic. No new public backend surface, migration, SDK registry upload, release, or billing formula change.

## Final verification evidence

- Full root Go command: `CGO_ENABLED=0 go test -p 1 -gcflags=all=-dwarf=false -ldflags='-s -w' -count=1 -json ./...`, exit 0. `/tmp/hotx-phase4-go.jsonl`: 5,445 PASS, 0 FAIL, 76 gated SKIP test/subtest events. All 87 packages completed successfully (46 package PASS, 41 no-test package SKIP). No root/relaykit source or dependency was changed.
- Final frontend command: Node 24 PATH with `bun run test --maxWorkers=2`, exit 0. `/tmp/hotx-phase4-frontend-final.log`: 185 files, 2,362 tests PASS, no FAIL. The earlier Bun runtime failure is retained in `/tmp/hotx-phase4-frontend.log`.
- `bun run typecheck`: exit 0, `/tmp/hotx-phase4-typecheck-final.log`.
- `bun run build` under Node 24: exit 0; `/tmp/hotx-phase4-build-final.log`: **161 prerender pages**. DOCS_SITE_URL unset warning remains explicit; no hostname was hard-coded.
- Existing browser verifier against the production preview: exit 0; `/tmp/hotx-phase4-browser-final.log`: **24 developer pages, 14 domains**, search operationId and public error guidance, clipboard, five Phase 4 pages in **seven locales × light/dark × 320/375/390/430/1440px**, plus existing 768px checks; no page errors or horizontal page overflow. The real missing developer search scope was reproduced in a component test and fixed by searching public developer articles while retaining beginner-only navigation and excluding /api.
- `bun run docs:check`, `bun run docs:contract`: exit 0; 107/107 allowed callable contracts, 0 operations without route/manifest, 0 internal leaks, 0 duplicate IDs, 0 unresolved refs, no compatibility changes against the initial reviewed snapshot.
- Full formal validator: `python -m openapi_spec_validator docs/openapi/public.json`: OK; also executed inside the SDK smoke.
- SDK final: `node web/scripts/verify-public-sdk.mjs --tool-root /tmp/hotx-sdk-smoke.hdo1lH`, exit 0; `/tmp/hotx-phase4-sdk-final.log`: both targets 107/107, sample typechecking/compilation and seven mock calls per client pass. Generated workspaces are removed by the script. No package publication.
- Changed-files oxlint: exit 0, no output/errors/warnings. Full oxlint remains 182 pre-existing errors / 66 warnings; unrelated lint was not cleaned.
- `git diff --check`: exit 0. No staged changes. No commit/push.

Browser screenshot fixtures remain in OS temporary storage; no screenshots or generated client vendor trees were added to the repository. The task-owned production-preview process was stopped after QA.

## Final files and diff
```text
 M .env.example
 M .github/workflows/ci.yml
 M web/package.json
 M web/scripts/generate-documentation.mjs
 M web/scripts/prerender-documentation.mjs
 M web/scripts/verify-documentation-browser.mjs
 M web/src/features/documentation/__tests__/navigation.test.tsx
 M web/src/features/documentation/__tests__/reference.test.ts
 M web/src/features/documentation/__tests__/seo.test.ts
 M web/src/features/documentation/developer-semantics-data.ts
 M web/src/features/documentation/generated/reference.json
 M web/src/features/documentation/index.tsx
 M web/src/features/documentation/lib.ts
 M web/src/features/documentation/navigation.tsx
 M web/src/i18n/locales/en.json
 M web/src/i18n/locales/fr.json
 M web/src/i18n/locales/ja.json
 M web/src/i18n/locales/ru.json
 M web/src/i18n/locales/vi.json
 M web/src/i18n/locales/zh-TW.json
 M web/src/i18n/locales/zh.json
?? .github/workflows/public-sdk.yml
?? docs/internal/IDEMPOTENCY_GAP_REPORT.md
?? docs/internal/OPENAPI_COVERAGE_REPORT.md
?? docs/internal/SDK_READINESS_REPORT.md
?? docs/internal/STATUS_API_GAP_REPORT.md
?? docs/internal/WEBHOOK_GAP_REPORT.md
?? docs/openapi/public-contract-baseline.json
?? docs/openapi/public.json
?? web/scripts/public-openapi-contract.d.mts
?? web/scripts/public-openapi-contract.mjs
?? web/scripts/verify-public-sdk.mjs
?? web/src/features/documentation/__tests__/public-openapi.test.ts
?? web/src/features/documentation/developer-recipe-code.ts
?? web/src/features/documentation/developer-recipes.tsx
```

Tracked-file diff (untracked new files are listed above, not included by git diff --stat):

```text
 .env.example                                       |     5 +
 .github/workflows/ci.yml                           |    21 +
 web/package.json                                   |     4 +-
 web/scripts/generate-documentation.mjs             |    30 +-
 web/scripts/prerender-documentation.mjs            |    14 +
 web/scripts/verify-documentation-browser.mjs       |    88 +-
 .../documentation/__tests__/navigation.test.tsx    |    24 +
 .../documentation/__tests__/reference.test.ts      |   119 +
 .../features/documentation/__tests__/seo.test.ts   |    26 +
 .../documentation/developer-semantics-data.ts      |   108 +
 .../documentation/generated/reference.json         | 13284 ++++++++++++-------
 web/src/features/documentation/index.tsx           |     7 +
 web/src/features/documentation/lib.ts              |    29 +-
 web/src/features/documentation/navigation.tsx      |    12 +-
 web/src/i18n/locales/en.json                       |    32 +
 web/src/i18n/locales/fr.json                       |    32 +
 web/src/i18n/locales/ja.json                       |    32 +
 web/src/i18n/locales/ru.json                       |    32 +
 web/src/i18n/locales/vi.json                       |    32 +
 web/src/i18n/locales/zh-TW.json                    |    32 +
 web/src/i18n/locales/zh.json                       |    32 +
 21 files changed, 9057 insertions(+), 4938 deletions(-)
```


---

# HOTX PRODUCTION API HARDENING — PHASE 5 REPORT

Candidate: unstaged working tree on `main`, based on committed Phase 4
`1fee3b56d8f012ef35b38324ca33ebeb284a1781`. This section supersedes Phase 4
readiness counts above without replacing its historical audit. Verification uses
local fixtures, loopback servers and disposable databases. No real provider
credentials, paid requests, provider network requests, staging, commit or push.

## 1. Git baseline

Preflight `git status`, `git log -1 --oneline`, `git diff --stat`, and `git diff`
showed a clean Phase 4 baseline before Phase 5 backend work. No stash/reset/discard
was used. HEAD remains the Phase 4 commit; all changes listed below are uncommitted.

## 2. Retry/idempotency audit

Client resubmission, gateway channel retry and polling are distinct. Existing
Request Policy, affinity and unkeyed retry behavior remain. A claimed keyed task
cannot be resubmitted after possible paid outbound or uncertain funding. Polling
continues the durable task and never acts as a create retry. Accepted plugin SSE
read/parse/hook errors preserve `NoRetry`. Detailed boundaries and sources:
[IDEMPOTENCY_IMPLEMENTATION_REPORT.md](IDEMPOTENCY_IMPLEMENTATION_REPORT.md).

## 3. Paid async operations discovered

The shared pipeline serves generic plugin tasks, video aliases/remix, native
Ali/Doubao/Jimeng/Kling/Suno routes, DFLOP media routes and task-claimed
Responses/image/speech bridges. The report lists all 27 public POST slots.
Ordinary built-in image/audio/Responses fallbacks, unclaimed built-in background
Responses and legacy Midjourney submission are additional mutation branches
outside this protection. SSE observation/WS creates are not silently treated as
idempotent task submissions.

## 4. Idempotency scope selected

**PARTIAL overall; 27/27 selected task-backed operation slots protected when
routed through the shared task pipeline.** This denominator is not all paid
mutations in every deployment. Optional `Idempotency-Key` is documented with that
condition. Unsupported fallback branches retain their prior behavior and may
ignore the header. No new catalog capability, public operation, universal cancel,
webhook backend or status backend was added. Native APIs remain conditional and
conservatively beta/experimental.

## 5. Storage/schema decision

Existing task rows cannot atomically claim an operation before they exist, so a
small primary-database `TaskIdempotency` table stores user/token IDs, operation/key/
fingerprint hashes, random owner nonce, state, exact resource-row reference and
timestamps. Unique `(token_id, operation_hash, key_hash)` uses bounded 64-character
columns and fits MySQL 5.7 utf8mb4 index limits. No raw credential, caller key,
body, upload or arbitrary provider response is stored. Insert-on-conflict and
nonce readback establish ownership even with `clientFoundRows=true`.

## 6. Request fingerprint design

Identity binds token, HTTP method, concrete path and SHA-256 key. Fingerprint
binds method/path/query, `Accept`, supported Content-Type/options and semantic
body. JSON key order/whitespace and equivalent exact decimal forms normalize
without float64 loss. Default JSON UTF-8 charset normalizes; other media options
remain significant. JSON bounds: 16 MiB, depth 32, 65,536 nodes, numeric lexeme
128 characters and exponent ±4,096; duplicate object members reject. Depth is
checked iteratively before recursive validation. Multipart binds form fields,
filename/type and streamed file-byte digests, ignoring transport boundary;
128 MiB/128 parts/1 MiB field bounds. Unsafe representations return explicit 400,
never a weak raw/filename-only fingerprint.

## 7. Durable acceptance semantics

`claimed → indeterminate` commits before adaptor outbound; failed marker storage
prevents outbound. Task insertion plus exact-row binding commits atomically.
Replay uses current task presentation, not stale serialized provider output.
Missing, contradictory or discarded resources fail closed. Background metadata is
persisted initially; replay never rewrites newer poll/private state.

Funding is attempted before outbound marking. Its in-memory flag is set before
pre-consume, including partial funding failures returning a nil session. Any
non-durable error after funding/outbound returns a safe 409 indeterminate warning
and suppresses blind retry. Async refund invocation does not prove completed
refund. Abandoned claimed/indeterminate/funding-attempted records remain blocked
for operator reconciliation; they are not automatically reacquired.

## 8. Concurrent duplicate handling

Database unique claim plus nonce CAS permits one owner. Same-key/different-body
returns 409 conflict; an active claim returns 409 in-progress. Different tokens,
including two keys for one user, have independent namespaces. Eight simultaneous
fixture callers produce one claim/submit/reservation/durable task. Exact resource
ownership is rechecked on replay; no cross-principal existence is disclosed.

## 9. Billing duplicate protection

Durable replay bypasses creation of BillingSession, reservation, settlement,
consume log and refund. Existing lifecycle owns all refunds. A known unfunded
preflight error can release its claim; funding-attempted errors retain it and
cannot run another pre-consume internally. DFLOP provider/journal identity derives
from claim ID/nonce via the existing request-header injection. Legacy own-token or
unknown token-zero journals fail closed; proven foreign positive-token journals
cannot deny another token's independent namespace. No billing formula changed.

## 10. Idempotency test results

Consolidated model/fingerprint/database tests and existing controller/native
fixtures cover sequential/concurrent replay, conflict, cross-user/token/same-user
isolation, completed/failed/refunded tasks, terminal expiry, active/abandoned
retention, unfunded versus uncertain funded failure, JSON/multipart,
video/plugin/background routes, atomic bind rollback, marker failure and real
TokenAuth invalid/expired/revoked/exhausted keys. Real local BillingSession
fixtures count provider posts, wallet/consume logs and reservations. SQLite core
race suite passes. Broad real-billing race run exposes an existing async cache
fixture-cleanup race, recorded under limitations rather than hidden.

## 11. SSE endpoints audited

Chat/completions, Responses built-in and task bridge, Gemini native/conversions,
built-in image and audio SSE, and internal plugin submit SSE. Source-specific
framing, terminal/error, heartbeat/idle, cancellation, accounting and retry
matrix: [STREAMING_CONTRACT_REPORT.md](STREAMING_CONTRACT_REPORT.md).

## 12. SSE contract/test results

**PARTIAL.** Fixed exact `[DONE]` matching, fatal callback versus queued EOF
classification, Responses flush-error handling while preserving observed usage,
nonpositive idle interval panic and failed-heartbeat reader cleanup. Local
fixtures cover LF/CRLF, limits, comments/event/id, multiline plugin frames,
malformed JSON, EOF, slow/idle/cancel and accepted submission no-retry.

The shared provider scanner remains line-oriented; it does not join legal
multiline SSE or resume by event ID. Its default physical-line cap is 128 MiB.
Plugin reader is framed, bounded to 1 MiB normalized event/snapshot, with 64 KiB
delta control and existing structural bounds. A universal parser migration was
not imposed on provider compatibility. Parallel helper race detects the existing
global `logger.logCount++` race; serialized helper race passes but does not fix it.

## 13. WebSocket audit

Existing `GET /v1/responses` uses explicit Bearer or browser API-key subprotocol,
per-create TokenAuth/permission/rate rechecks, one active generation and conditional
native channel/model/config support. No cookie/query authentication or universal
model support. Flat/wrapped create, stream identity, cancel/error/terminal frames,
limits, write bounds and close behavior are source-documented in
[WEBSOCKET_CONTRACT_REPORT.md](WEBSOCKET_CONTRACT_REPORT.md).

## 14. WebSocket tests

**PASS for the audited local contract.** Added inactive first/next-create timeout
using existing operator timeout. Local socket peers verify handshake/auth,
invalid/unknown messages without charge, size, model/channel eligibility,
revocation, create reuse, correlated errors/cancel, normal/abrupt close, observed
usage settlement/refund and cleanup with no stale policy callback. Idle regression
was reproduced before its fix; race-enabled targeted lifecycle tests pass.
No new WS idempotency/resumption or universal cancellation guarantee.

## 15. AsyncAPI decision

Adopted complementary `docs/asyncapi/public.yaml`, AsyncAPI 3.0.0/WebSocket binding
0.1.0, for the existing Responses connection only. HTTP stays in OpenAPI. Provider
server frames remain open where this fork forwards them. No fake webhook/status
channels. Exact validated YAML is copied to `/docs-contracts/responses-websocket.yaml`
by prerendering and linked from developer documentation.

## 16. AsyncAPI validation

**PASS:** pinned parser 3.4.0, complete schema/local refs/operations and five
AJV-validated examples. Four mutations reject unresolved ref, invalid action,
invalid example and remote ref. External refs reject before resolver use. Parser
emits an informational 3.1.0 recommendation; no validation errors. Tools live in
temporary verification directories, with no new application dependency.

## 17. Public OpenAPI changes

**107 callable operations preserved**, zero duplicate IDs, unresolved refs,
missing operations, backendless operations or internal leaks. Selected 27
conditional task POST entries describe header/error behavior; 14 existing header
entries updated, 13 optional headers added. Committed diff: **13 ADDITIVE, zero
BREAKING/POTENTIALLY_BREAKING**. Error status/prose reviewed separately because
structural diff is not exhaustive semantic verification. Full validator passes.

## 18. Breaking-change CI

Compatibility baseline now comes from `git show` at committed PR merge base,
with full history fetched, rather than mutable working-tree snapshot. BREAKING
fails; ADDITIVE passes; uncertain semantic/schema changes warn for review.
Deletion, required additions, auth changes, enum/bounds/type/nullability breaks
have regression mutations. No bypass variable. SDK/AsyncAPI workflow is required
on pull requests as well as manual dispatch. **Local checks PASS; GitHub-hosted
workflow was not dispatched** because no commit/push is authorized.

## 19. SDK streaming readiness

| Target | REST | SSE | WebSocket |
| --- | --- | --- | --- |
| Python | PASS | MANUAL_RECIPE | MANUAL_RECIPE |
| TypeScript | PASS | MANUAL_RECIPE | MANUAL_RECIPE |

Both clients generate 107/107 operations; 14 injected REST transport calls prove
auth/query/path/decimal/null/pagination/binary behavior. TypeScript raw streaming
transport is verified; Python generator buffers. No generated vendor code is
handpatched, committed or published. Exact evidence and generator pins:
[SDK_READINESS_REPORT.md](SDK_READINESS_REPORT.md).

## 20. Python SSE/WebSocket status

**MANUAL_RECIPE:** executable incremental UTF-8/SSE decoder and loopback WS
create/cancel/close recipe pass. These are reference fixtures, not native generated
client APIs. Strict whole-package mypy still has exactly 12 generator-template
`arg-type` errors in `client.py` lines 105/136/246/280 across 428 checked files.
Byte compilation and minimal typed sample pass; no suppression/handpatch.

## 21. TypeScript SSE/WebSocket status

**MANUAL_RECIPE:** raw generated stream transport plus manual decoder and Node
`ws` fixture verify split UTF-8/frame boundaries, cancellation and local socket
lifecycle. Browser recipe uses actual credential subprotocol. Production callers
must add bounds/timeouts and protocol-specific terminals; do not auto-reconnect or
resubmit ambiguous paid work. No native generated WS API is claimed.

## 22. Recipe validation

All five actual displayed builders execute against injected local transports:
model discovery/chat, Decimal balance guard, usage time window, saved video task
and binary download, task pagination. A jq shim exits 99, proving critical flows
use Python stdlib JSON/Decimal rather than jq. Failure/unknown status, malformed
JSON, insufficient balance and quotes/backticks/dollar-sign model IDs are tested.
Public contract/recipe test file: 26 PASS. No displayed mutation reaches a provider.

## 23. Webhook audit/design

**DESIGN_ONLY.** Existing user-notification webhooks and inbound payment callbacks
exist; they are not a durable public task-event service. Proposal v1 only includes
`task.completed`/`task.failed`, token-owned registrations, stable event and delivery
IDs, version/timestamp, durable outbox, bounded retry/dead-letter/replay/logs and
minimal payloads. No new backend, OpenAPI webhooks or AsyncAPI event channels.
See [WEBHOOK_DESIGN_REPORT.md](WEBHOOK_DESIGN_REPORT.md).

## 24. Webhook security design

Separate rotated signing secret, timestamp plus raw-body HMAC/replay window,
receiver challenge/ownership checks, delivery deduplication and protected
management. Mandatory future HTTPS/DNS/IP/redirect/metadata/private-network SSRF
controls, pinned protected egress, size/time/concurrency limits. Existing sender
optional signing lacks durable IDs/replay window; worker path sends secret as
Bearer and delegates URL safety. These gaps are reported, not declared fixed.

## 25. Public status audit/design

**DESIGN_ONLY.** `/api/status` is UI bootstrap, protected status/performance probes
are internal, and configured Uptime Kuma proxy/dashboard exists but can return
empty success on missing/failing remote sources. Unregistered `/health`/`live`
may be SPA 200, not health. Future customer page first, machine API only if needed:
sanitized component/incident/freshness/stale projection; no internal topology,
raw errors or invented domain. See
[PUBLIC_STATUS_DESIGN_REPORT.md](PUBLIC_STATUS_DESIGN_REPORT.md).

## 26. Release gate

[PRODUCTION_API_RELEASE_CHECKLIST.md](PRODUCTION_API_RELEASE_CHECKLIST.md) records
config, contracts, fixtures/security/DB, SDK, links/locales/prerender and deployment
checks. `.env.example` marks **DOCS_SITE_URL: REQUIRED_FOR_PRODUCTION_DOCS** at build
time. It remains unset here: relative canonical output and missing absolute
sitemap origin mean production docs release is not approved. Actual deployment
TLS/origin/limits/provider capability checks remain operator work.

## 27. Security findings

Token/path-scoped claim, hashed identity, bounded parser/input, auth before
mutation, exact ownership and safe static error messages are regression-tested.
No new-key suggestion after ambiguous funding/outbound; retained claims require
reconciliation. Legacy unknown provenance fails closed. Authentication/session/
authorization/WebSocket OWASP guidance and ASVS 5.0.0 were reviewed for focused
controls, not blanket conformance. Existing WS unrestricted Origin with explicit
API-key policy, credential subprotocol exposure and deployment TLS/connection
controls remain considerations. No credentials/private bodies are included in
new diagnostic decisions, source examples or public projections.

## 28. Performance

Claim lookup uses its unique index; replay reads exact primary task ID. JSON
normalization is bounded; multipart streams digests without persisting uploads.
Eight-way deterministic concurrency verifies correctness, not throughput/SLA.
No provider benchmark or production load test was run. A standalone first/replay/
concurrent performance benchmark remains a follow-up; no fabricated timing claim.

## 29. DB matrix

**PASS on real SQLite 3.50.4, MySQL 5.7.44 and PostgreSQL 15.19.** Disposable local
MySQL uses `clientFoundRows=true`; PostgreSQL uses loopback SSL-disabled test
connection. Each verifies fresh migration twice, representative Phase 4 task
schema upgrade twice preserving existing records, portable unique constraints,
eight-way atomic claim, token/path isolation, retention/expiry, marker-write
failure and atomic task bind rollback. Separate configured log handles/prefixes
are exercised; SQLite also uses a separate physical log file. Existing user dev
DB/Redis containers are untouched. No ORM/driver upgrade.

Final full command (DSNs supplied only through local fixture environment):

```sh
GOTOOLCHAIN=go1.25.1 \
PHASE5_MYSQL_DSN='<loopback MySQL 5.7 DSN; clientFoundRows=true>' \
PHASE5_POSTGRES_DSN='<loopback PostgreSQL 15 DSN>' \
CGO_ENABLED=0 go test -p 1 -gcflags=all=-dwarf=false \
  -ldflags='-s -w' -count=1 -json ./...
```

The compiler/linker flags avoid macOS debugger metadata limits; tests are not
filtered. No simultaneous frontend build mutates the embedded `web/dist` during
this final Go run. Real DB targeted evidence: `/tmp/phase5-idempotency-db.log`;
final full evidence: `/tmp/hotx-phase5-go125-verified.jsonl`.

## 30. Documentation changes

Added developer Idempotency and WebSocket articles, expanded existing Streaming
and Errors & Retry, updated Unreleased and five recipes, linked AsyncAPI source
and SDK fixtures. Existing data-driven renderer/shared Markdown/DocsCode/copy
components reused; no duplicate generic UI. Beginner sidebar stays unchanged.
29 new locale entries synchronized in all seven languages through sanctioned
script; source-generation references updated. 26 developer pages plus protocol/
reference pages prerender to 163 pages. No historical project attribution removed.

## 31. Files created

- `docs/asyncapi/public.yaml`
- `docs/internal/IDEMPOTENCY_IMPLEMENTATION_REPORT.md`
- `docs/internal/PRODUCTION_API_RELEASE_CHECKLIST.md`
- `docs/internal/PUBLIC_STATUS_DESIGN_REPORT.md`
- `docs/internal/STREAMING_CONTRACT_REPORT.md`
- `docs/internal/WEBHOOK_DESIGN_REPORT.md`
- `docs/internal/WEBSOCKET_CONTRACT_REPORT.md`
- `model/task_idempotency.go`
- `model/task_idempotency_test.go`
- `service/task_idempotency.go`
- `web/scripts/sdk-streaming-fixtures.mjs`
- `web/scripts/verify-public-asyncapi.mjs`

## 32. Files modified

- `.env.example`
- `.github/workflows/ci.yml`
- `.github/workflows/public-sdk.yml`
- `controller/plugin_native_e2e_test.go`
- `controller/plugin_protocol.go`
- `controller/plugin_protocol_image.go`
- `controller/plugin_protocol_test.go`
- `controller/relay.go`
- `controller/relay_task_plugin_test.go`
- `controller/responses_websocket_test.go`
- `docs/internal/OPENAPI_COVERAGE_REPORT.md`
- `docs/internal/SDK_READINESS_REPORT.md`
- `docs/openapi/public.json`
- `main.go`
- `model/main.go`
- `relay/channel/openai/helper.go`
- `relay/channel/openai/relay_responses.go`
- `relay/channel/openai/relay_responses_billing_test.go`
- `relay/channel/task/jsplugin/adaptor_test.go`
- `relay/common/relay_info.go`
- `relay/helper/stream_scanner.go`
- `relay/helper/stream_scanner_test.go`
- `relay/relay_task.go`
- `relay/responses_websocket.go`
- `relay/responses_websocket_test.go`
- `service/billing.go`
- `service/billing_reservation.go`
- `web/scripts/prerender-documentation.mjs`
- `web/scripts/public-openapi-contract.d.mts`
- `web/scripts/public-openapi-contract.mjs`
- `web/scripts/verify-documentation-browser.mjs`
- `web/scripts/verify-public-sdk.mjs`
- `web/src/features/documentation/__tests__/public-openapi.test.ts`
- `web/src/features/documentation/__tests__/reference.test.ts`
- `web/src/features/documentation/__tests__/seo.test.ts`
- `web/src/features/documentation/developer-recipe-code.ts`
- `web/src/features/documentation/developer-semantics-data.ts`
- `web/src/features/documentation/generated/reference.json`
- `web/src/i18n/locales/en.json`
- `web/src/i18n/locales/fr.json`
- `web/src/i18n/locales/ja.json`
- `web/src/i18n/locales/ru.json`
- `web/src/i18n/locales/vi.json`
- `web/src/i18n/locales/zh-TW.json`
- `web/src/i18n/locales/zh.json`

## 33. Tests

Final observed verification is recorded below after all source changes:

| Verification | Observed result |
| --- | --- |
| Full root Go, Go 1.25.1, final source and real three-DB DSNs | **5,494 PASS / 0 FAIL / 76 SKIP**, 46 passing test packages; 41 packages have no tests; exit 0 |
| Full frontend, bundled Node 24.19 via Bun/Vitest, `bun run test --maxWorkers=2` | **185 files / 2,380 PASS / 0 FAIL**, exit 0; 212.75s |
| Core SQLite idempotency race | PASS model/controller; broader cache/logger races explicitly fail below |
| Responses WebSocket targeted race/lifecycle | PASS |
| `bun run docs:check` | PASS, 405 registered operations / 46 error constants / 16 stale historical entries excluded |
| `bun run docs:contract` | PASS, 107 callable ops; 13 additive; zero breaking/potential |
| Full `openapi_spec_validator` | PASS |
| `node web/scripts/verify-public-asyncapi.mjs --tool-root /tmp/hotx-phase5-asyncapi` | PASS, 3.0.0; five examples |
| `node web/scripts/verify-public-sdk.mjs --tool-root /tmp/hotx-sdk-smoke.hdo1lH` | PASS generation/REST/manual stream and socket fixtures; known 12 strict template errors retained |
| Scoped oxlint on every changed/new TS/TSX/MJS/MTS source | PASS, zero new errors |
| `bun run typecheck` | PASS, exit 0 |
| `bun run build` | PASS, exit 0, 163 prerender pages; unset-origin warning retained |
| Final browser harness against localhost:5175 | PASS, responsive/locales/light-dark/links/search/copy; no page errors |
| Modified Go `gofmt -l`, `git diff --check` | PASS |

Go skips are actual fixture/capability skips, not hidden failures. In particular
SQLite/MySQL/PostgreSQL idempotency matrix subtests all PASS in the final JSON
log, with no database-matrix skip. Final logs:
`/tmp/hotx-phase5-go125-verified.jsonl` (stderr empty),
`/tmp/hotx-phase5-frontend-verified.log`,
`/tmp/hotx-phase5-browser-final.log`,
`/tmp/hotx-phase5-contract-verified.log`,
`/tmp/hotx-phase5-doccheck-verified.log`,
`/tmp/hotx-phase5-typecheck-verified.log`,
`/tmp/hotx-phase5-build-final.log`,
`/tmp/hotx-phase5-changed-lint-verified.log`,
`/tmp/hotx-phase5-sdk-final.log`,
`/tmp/hotx-phase5-asyncapi-verified.log`.

SDK smoke: 107/107 each target, 14 REST fixture calls; Python/TypeScript manual
SSE/WS fixtures PASS. AsyncAPI: five valid examples/four rejected mutations.
Changed-file lint: zero errors. Historical full lint remains 182 errors/66 warnings;
unrelated lint is not cleaned. Docs generated-source check and public-contract
coverage PASS. Typecheck/build PASS; 163 prerendered pages. Browser tests exercise
26 developer pages/seven locales, 14 reference domains, responsive widths
320/375/390/430/768/1440, light/dark, search/sidebar/copy and exact source links.
Local preview direct GETs return 200 for `/docs`, `/docs/`, quickstart, both
new developer articles and the AsyncAPI artifact. `/dashboard/overview` returns
404 on this static preview; dashboard SPA fallback is owned by the Gin production
server and was not demonstrated by this preview. This is preview/artifact
validation, not a launched production backend or operator deployment verification.
Existing dashboard/frontend and API regression suites pass; no dashboard-serving
production source was changed.

## 34. Known limitations

- Phase 5A conditional scope; unsupported fallbacks/legacy mutations may ignore
  the header. No universal paid-mutation denominator or universal replay promise.
- Abandoned/funding-attempted/indeterminate claims do not autoexpire. This can
  require manual reconciliation and leave retained records; canonical recovery
  needs verified settlement/refund history. Terminal cleanup is bounded by at
  least 72h (configured task lifetime plus 48h), and 48h since terminal finish.
- Phase 3D remains **BLOCKED_BY_OWNERSHIP_MODEL**; universal cancel **DESIGN_ONLY**.
- Shared SSE multiline framing gap, no universal resume, provider-specific terminal/
  disconnect accounting; Python/TypeScript streaming/socket use manual recipes.
- Parallel SSE race fails on unchanged `logger/logger.go:115–116` global counter.
  Broad billing race fails on fixture cache-flag cleanup versus existing async
  quota cache worker (`controller/relay_task_plugin_test.go:527`,
  `model/user.go:1353`, `model/user_cache.go:144`). Core idempotency and targeted
  WS races pass; no full race-clean claim.
- Strict generated-package mypy 12 template errors; no official published SDK.
- DOCS_SITE_URL unset; actual deployment checks and hosted CI not run.
- No new webhook/status service and no production throughput benchmark.

Earlier full Go attempts exposed concurrent frontend-build embed mutation and a
new fixture reusing a nonzero GORM destination primary key. Those were corrected
by serialized build/test and fixture reset, then rerun. Unresolved races above
are explicitly separate from successful non-race regression suites.

## 35. Recommended Phase 6

1. Canonical funding/reservation/refund ledger and proven operator recovery before
   releasing uncertain claims or reconsidering Phase 3D.
2. Explicit unsupported-header/capability policy for remaining mutation branches;
   maintain compatibility rather than assuming every `/v1` route is protected.
3. Provider-compatible framed SSE migration plus logger/cache synchronization,
   validated under parallel race; real controlled performance characterization.
4. Official SDK template/helper ownership, strict typing and bounded transports.
5. Only after ownership/SSRF/signature/operational decisions, durable task-event
   delivery and sanitized customer status projection. Configure production docs
   origin and verify actual deployed artifacts before release.

## 36. git diff --stat

```text
 .env.example                                       |   2 +-
 .github/workflows/ci.yml                           |   7 +-
 .github/workflows/public-sdk.yml                   |   8 +-
 controller/plugin_native_e2e_test.go               |  84 +++-
 controller/plugin_protocol.go                      |  16 +-
 controller/plugin_protocol_image.go                |   5 +
 controller/plugin_protocol_test.go                 |  73 +++
 controller/relay.go                                |  49 +-
 controller/relay_task_plugin_test.go               | 269 ++++++++++-
 controller/responses_websocket_test.go             |  99 +++-
 docs/internal/OPENAPI_COVERAGE_REPORT.md           | 508 +++++++++++++++++++++
 docs/internal/SDK_READINESS_REPORT.md              |  55 ++-
 docs/openapi/public.json                           | 377 ++++++++++++++-
 main.go                                            |   1 +
 model/main.go                                      |   1 +
 relay/channel/openai/helper.go                     |   7 -
 relay/channel/openai/relay_responses.go            |   4 +-
 .../channel/openai/relay_responses_billing_test.go |  29 ++
 relay/channel/task/jsplugin/adaptor_test.go        |  34 ++
 relay/common/relay_info.go                         |   5 +
 relay/helper/stream_scanner.go                     |  25 +-
 relay/helper/stream_scanner_test.go                |  77 ++++
 relay/relay_task.go                                |  13 +
 relay/responses_websocket.go                       |  42 +-
 relay/responses_websocket_test.go                  |  40 ++
 service/billing.go                                 |   2 +-
 service/billing_reservation.go                     |   6 +
 web/scripts/prerender-documentation.mjs            |   6 +
 web/scripts/public-openapi-contract.d.mts          |   5 +
 web/scripts/public-openapi-contract.mjs            | 187 +++++++-
 web/scripts/verify-documentation-browser.mjs       |  26 +-
 web/scripts/verify-public-sdk.mjs                  |  50 +-
 .../documentation/__tests__/public-openapi.test.ts | 357 ++++++++++++++-
 .../documentation/__tests__/reference.test.ts      |   4 +-
 .../features/documentation/__tests__/seo.test.ts   |  27 +-
 .../documentation/developer-recipe-code.ts         |  43 +-
 .../documentation/developer-semantics-data.ts      |  97 +++-
 .../documentation/generated/reference.json         |  84 +++-
 web/src/i18n/locales/en.json                       |  29 ++
 web/src/i18n/locales/fr.json                       |  29 ++
 web/src/i18n/locales/ja.json                       |  29 ++
 web/src/i18n/locales/ru.json                       |  29 ++
 web/src/i18n/locales/vi.json                       |  29 ++
 web/src/i18n/locales/zh-TW.json                    |  29 ++
 web/src/i18n/locales/zh.json                       |  29 ++
 45 files changed, 2795 insertions(+), 132 deletions(-)
```

The stat snapshot was captured before embedding its own block into this report.
`git diff --stat` describes tracked files; the created-file list above additionally
contains untracked source artifacts. No staging, commit or push occurred.

## Requested summary

| Item | Result |
| --- | --- |
| Phase 5A Idempotency | PARTIAL |
| Paid async endpoints protected | 27/27 selected **task-backed slots**, conditional; not universal gateway coverage |
| Sequential duplicate prevention | PASS within selected scope |
| Concurrent duplicate prevention | PASS within selected scope |
| Duplicate billing prevention | PASS within selected scope |
| Cross-key isolation | PASS |
| Post-acceptance blind retry prevented | PASS within selected scope |
| SSE contract | PARTIAL |
| SSE SDK Python / TypeScript | MANUAL_RECIPE / MANUAL_RECIPE |
| Responses WebSocket | PASS audited local contract, conditional availability |
| AsyncAPI | PASS |
| Python / TypeScript WebSocket | MANUAL_RECIPE / MANUAL_RECIPE |
| Public OpenAPI valid | PASS, 107 operations |
| Breaking-change CI | PASS local gate/tests; hosted execution not dispatched |
| SDK generation | PASS both targets; strict Python whole-package typing template-blocked |
| Webhook / Public status | DESIGN_ONLY / DESIGN_ONLY |
| Phase 3D | BLOCKED_BY_OWNERSHIP_MODEL |
| Paid upstream / provider network requests | 0 / 0 |
| Existing API regressions | PASS full non-race Go/frontend suites; race limitations explicit |
| SQLite / MySQL 5.7 / PostgreSQL 15 | PASS / PASS / PASS |
| Changed-files lint / Typecheck / Build | PASS / PASS / PASS |
| Prerender | 163 pages |
| Tests | Go 5,494 PASS / 0 FAIL / 76 SKIP; frontend 2,380 PASS / 0 FAIL; optional broader races FAIL as recorded |
| No commit / No push / Scope respected | YES / YES / YES |

Tool registries/downloads and local HTTP/WebSocket/DB fixtures are distinct from
provider network calls. Passing focused gates does not approve production release
while the listed deployment configuration and readiness limitations remain.
