/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export const developerSemantics = [
  {
    slug: 'developer-authentication',
    title: 'API Authentication',
    description:
      'Use gateway API keys and understand their access restrictions.',
    sources: [
      'middleware/auth.go',
      'middleware/distributor.go',
      'model/token.go',
      'router/relay-router.go',
      'router/api-router.go',
      'controller/token.go',
    ],
    sections: [
      {
        id: 'gateway-key',
        title: 'Gateway API Key',
        body: 'Send your gateway API key as Authorization: Bearer YOUR_API_KEY. Use the gateway base URL configured for your deployment. Upstream provider credentials belong to administrator channel configuration and cannot replace your gateway key. Browser sessions and personal access tokens for management APIs are separate from relay API keys.',
      },
      {
        id: 'protocol-headers',
        title: 'Protocol Authentication Headers',
        body: 'Messages routes accept x-api-key. Anthropic-style model listing uses x-api-key together with anthropic-version. Gemini model paths accept x-goog-api-key or the key query parameter; prefer a header so the key is not embedded in a URL. Use the authentication form documented for your endpoint and send one credential source to avoid conflicting values.',
      },
      {
        id: 'key-restrictions',
        title: 'Key Restrictions',
        body: 'Relay authentication checks key status, expiry and remaining key quota unless unlimited. Requests also depend on the enabled account, allowed IP ranges, selected group and model restrictions. A valid key does not grant every model or group. Model-access checks and funding checks can reject the request after authentication; inspect the status and error message before retrying.',
      },
      {
        id: 'public-read-auth',
        title: 'Public Catalog',
        body: 'These public reads require an Authorization Bearer API key and retain account, IP and group restrictions. Browser cookies and dashboard credentials are not accepted. Zero allowance does not block these reads; expired keys still return 401. Responses are private and no-store. The configured global API IP limiter can return an empty 429 body with Retry-After; no inference billing limiter is used.',
      },
      {
        id: 'credential-handling',
        title: 'Protecting Credentials',
        body: 'Keep API keys on a trusted server, send authenticated traffic over HTTPS and exclude keys from logs, screenshots and shared examples. Disable a key when it is no longer needed. Read-only token usage and log endpoints allow expired or exhausted keys to inspect records but reject explicitly disabled keys and disabled accounts; this does not authorize new relay requests.',
      },
    ],
  },
  {
    slug: 'developer-streaming',
    title: 'Streaming Responses',
    description: 'Parse each protocol stream and recognize partial results.',
    sources: [
      'relay/helper/common.go',
      'relay/helper/stream_scanner.go',
      'relay/channel/openai/relay_responses.go',
      'relay/channel/claude/relay-claude.go',
      'relay/channel/gemini/relay-gemini-native.go',
      'relaykit/dto/gemini.go',
      'controller/plugin_protocol.go',
    ],
    sections: [
      {
        id: 'sse-framing',
        title: 'Reading SSE',
        body: 'For supported streaming requests, read text/event-stream incrementally with an SSE parser. Network reads do not correspond to complete events: buffer until the event boundary, handle data fields and ignore comment heartbeats. Check HTTP status and Content-Type before parsing. A successful HTTP status alone does not establish that generation completed.',
      },
      {
        id: 'openai-events',
        title: 'Chat and Responses Events',
        body: 'OpenAI Chat streams carry JSON data chunks with choices and delta fields, followed by data: [DONE] on normal completion. Accumulate content and tool-call arguments by their indices rather than parsing each fragment as a complete result. Responses streams use named events such as response.output_text.delta and terminal response.completed, response.failed or response.incomplete. Use the terminal response status; [DONE] is not a substitute for every protocol lifecycle.',
      },
      {
        id: 'provider-events',
        title: 'Messages and Gemini Events',
        body: 'Anthropic Messages streams use named lifecycle events including message_start, content_block_delta, message_delta and message_stop. Track content block indices and finish state. Native Gemini generateContent returns JSON; streamGenerateContent or alt=sse selects streaming and the gateway emits SSE data containing Gemini response objects. This does not promise an unframed JSON-array stream. Parse candidates, parts and finish reasons using the endpoint protocol.',
      },
      {
        id: 'partial-output',
        title: 'Disconnects and Partial Output',
        body: 'Keep partial output separate from a completed result. A disconnect, malformed stream or missing terminal event can leave final usage unavailable and can occur after billable work began. There is no general stream-resume or duplicate-submission guarantee. For durable tasks, retrieve the saved response or task ID before resubmitting; for other requests, retain X-Oneapi-Request-Id and check consumption records.',
      },
    ],
  },
  {
    slug: 'developer-billing-usage',
    title: 'Billing and Usage',
    description: 'Understand usage categories, reservations and final charges.',
    sources: [
      'relay/request_billing.go',
      'service/text_quota.go',
      'service/tiered_settle.go',
      'service/task_billing.go',
      'service/log_info_generate.go',
      'pkg/billingexpr/expr.md',
      'controller/billing.go',
      'controller/token.go',
      'controller/developer_catalog.go',
      'controller/developer_balance.go',
      'dto/developer_catalog.go',
      'dto/developer_balance.go',
    ],
    sections: [
      {
        id: 'public-catalog',
        title: 'Public Catalog',
        body: 'GET /v1/catalog provides rich key-scoped model discovery. GET /v1/models remains the OpenAI-compatible model list. Use catalog for configured endpoints and customer pricing; a listed local route is not a guarantee of every upstream feature or current upstream availability.',
      },
      {
        id: 'public-catalog-pricing',
        title: 'Pricing',
        body: 'Catalog prices are customer unit rates for this key, represented as decimal strings in currency. Missing fields are not free prices; an explicit "0" is a configured zero price. Different eligible routes or groups, request options and unsupported complex expressions can produce variable=true instead of an inaccurate flat price. Unknown model capabilities are omitted.',
      },
      {
        id: 'public-key-balance',
        title: 'Public Key Balance',
        body: 'Balance is the authenticated key allowance, not the account wallet or lifetime purchased funds. For limited keys, total is used plus remaining. For unlimited keys, total and remaining are null while used remains the recorded amount. expires_at is Unix seconds or null when there is no expiry. Query parameters cannot select another key.',
      },
      {
        id: 'public-currency',
        title: 'Currency',
        body: 'Catalog and balance use the configured customer display unit. HOTX points configuration returns points with its configured conversion; USD, CNY and another configured custom currency retain their own units. Monetary values are decimal strings. Console raw-quota TOKENS display is unsupported by these public monetary contracts and returns an error instead of exposing internal quota.',
      },
      {
        id: 'legacy-quota',
        title: 'Compatibility and Versioning',
        body: 'Legacy quota inspection is retained for compatibility. New public integrations should use GET /v1/key/balance; raw quota from /api/usage/token/ is not a points amount.',
      },
      {
        id: 'token-categories',
        title: 'Input, Output and Cache',
        body: 'Input and output tokens are separate usage categories. Cached input is a cache read; cache creation is a cache write, which may have a separate duration category. These counts differ by protocol: OpenAI-style input totals can include cached tokens, while Claude reports cache counts separately. Do not add every usage detail to the total or assume cached input is free. Effective pricing determines which categories receive separate prices.',
      },
      {
        id: 'media-units',
        title: 'Image, Video and Audio Usage',
        body: 'Image pricing can use generated image count or input and output tokens. Video and asynchronous media can use seconds, counts, tokens, characters or provider-specific usage units. Audio input and output can have separate prices. Use the model pricing unit and returned usage together; a token total alone cannot explain every media charge. Resolution, quality and other supported request options can affect the price.',
      },
      {
        id: 'reservation',
        title: 'Reservation and Settlement',
        body: 'The gateway can reserve estimated funds before submitting work, then reconcile the reservation with the final charge. An asynchronous submission is not final billing: completion facts replace available estimates and the remaining estimates are retained. Missing usage or a settlement evaluation failure can retain the reservation. A client disconnect or image-task timeout does not prove that upstream work stopped.',
      },
      {
        id: 'refunds',
        title: 'Failures and Refunds',
        body: 'Final request failures normally release the reservation, and failed asynchronous tasks follow the task refund path. This is not a promise that every unsuccessful client response has zero cost: configured violation fees, completed output and still-running durable tasks can require separate reconciliation. Review the final task status and consumption or refund records before submitting duplicate work.',
      },
      {
        id: 'final-charge',
        title: 'Checking the Final Charge',
        body: 'Use GET /v1/catalog for key-specific customer unit rates and GET /v1/key/balance for the current key allowance. Use Console Usage Logs for recorded consumption and adjustments. Keep X-Oneapi-Request-Id and the public task ID for correlation. Legacy /api/usage/token/ and dashboard billing URLs retain compatibility behavior but are not the recommended new public read contract.',
      },
    ],
  },
  {
    slug: 'developer-errors-retries',
    title: 'Errors and Retries',
    description: 'Handle protocol errors and retry without duplicating work.',
    sources: [
      'controller/relay.go',
      'middleware/utils.go',
      'middleware/task_plugin.go',
      'controller/task.go',
      'controller/plugin_protocol_image.go',
      'common/constants.go',
    ],
    sections: [
      {
        id: 'envelopes',
        title: 'Error Envelopes',
        body: 'OpenAI-style relay errors use an error object; Claude relay errors also include top-level type: error. Middleware can return an OpenAI-style error even on a provider protocol path. Native plugin routes use their plugin presenter, and some rate limits return an empty body. Check the HTTP status before parsing JSON, then inspect message, type and code when present. A code can be empty.',
      },
      {
        id: 'status-actions',
        title: 'Status and Next Action',
        body: 'Correct invalid requests for 400, oversized bodies for 413 and unsupported body formats for 415. Resolve credential or access problems before retrying 401 or 403. A missing route or task can return 404; explicitly unimplemented relay routes return 501 with api_not_implemented. For 429, wait and reduce concurrency. For transport errors and 5xx, use a limited retry policy and first consider whether the operation could already have been accepted.',
      },
      {
        id: 'bounded-retry',
        title: 'Bounded Backoff',
        body: 'Honor Retry-After when supplied. Otherwise apply exponential backoff with jitter, a maximum delay and a small attempt budget. Do not loop on validation, model pricing or quota errors. General relay and task submission endpoints do not establish a universal Idempotency-Key contract. Asynchronous speech specifically requires that header; follow its endpoint contract instead of assuming the same behavior elsewhere.',
      },
      {
        id: 'async-recovery',
        title: 'Recovering an Async Request',
        body: 'Save the public task or response ID as soon as submission succeeds. After a timeout or disconnect, retrieve that resource and check its status before creating another request. Plugin-backed image requests can return 504 task_timeout while the durable task continues. Artifact content can return 409 artifact_not_ready until the task succeeds. If no ID was received, keep the request ID and consult Usage Logs or the administrator before repeating costly work.',
      },
    ],
  },
  {
    slug: 'developer-rate-limits',
    title: 'Rate Limits',
    description: 'Read limit responses and adapt client concurrency.',
    sources: [
      'middleware/rate-limit.go',
      'middleware/model-rate-limit.go',
      'middleware/task_artifact_access.go',
      'router/relay-router.go',
      'router/task-plugin-protocol-router.go',
    ],
    sections: [
      {
        id: 'limit-scopes',
        title: 'Limit Scope',
        body: 'Limits depend on deployment settings and the route. Global API, critical-operation and model-request limits are separate controls; model limits can vary by user group and count total attempts as well as successful requests. Failed attempts can therefore still consume a total-request allowance. Do not hardcode a requests-per-minute value from another deployment.',
      },
      {
        id: 'limit-headers',
        title: 'Headers and Bodies',
        body: 'The generic IP and user limiter emits HTTP 429 with an empty body and Retry-After when its wait is positive. The model-request limiter emits an error object with type new_api_error and can omit Retry-After. Invalid artifact-access credentials have a separate 429 rate_limit_error response and Retry-After. There is no uniform remaining-quota or reset-header contract across these paths.',
      },
      {
        id: 'client-throttle',
        title: 'Client Throttling',
        body: 'Limit parallel submissions, queue excess work and back off after 429. Use Retry-After when present and bounded jittered backoff otherwise. Poll asynchronous resources at a controlled interval and stop at a terminal status. Raising concurrency while already limited usually increases rejected attempts; ask the operator about your group limits when sustained traffic requires more capacity.',
      },
    ],
  },
  {
    slug: 'developer-compatibility-versioning',
    title: 'Compatibility and Versioning',
    description:
      'Separate protocol support, model access and installed features.',
    sources: [
      'router/relay-router.go',
      'router/task-plugin-protocol-router.go',
      'pkg/jsplugin/routing.go',
      'docs/plugin-api/v1.md',
      'docs/openapi/relay.json',
      'README.md',
    ],
    sections: [
      {
        id: 'protocol-support',
        title: 'Protocol Compatibility',
        body: 'A compatible URL does not guarantee every upstream feature. Match the endpoint, request format and model to a configured channel. Check accessible models with your API key. Endpoint pages derive host protocol bindings from the registry and native task routes from shipped manifests; enabled plugins, channel bindings and pricing determine runtime availability. Shipped metadata is not a live model-access list.',
      },
      {
        id: 'request-modes',
        title: 'Streaming and Background Modes',
        body: 'For plugin-backed Responses, stream, sync and background are declared capabilities of the selected plugin. Unsupported forms are rejected before submission. Retrieval is separate from create modes. Plugin-backed image protocols wait for the result, video protocols return a task, and asynchronous speech has its own request contract. Do not infer mode support from another model on the same URL.',
      },
      {
        id: 'unsupported',
        title: 'Unsupported Operations',
        body: 'Registered file, fine-tune and image-variation placeholders return 501 rather than implementing those APIs. Claude count_tokens is disabled and Gemini countTokens is rejected as a missing route. Use the endpoint reference and actual response instead of assuming complete provider API coverage.',
      },
      {
        id: 'catalog-schema',
        title: 'Catalog Schema Compatibility',
        body: 'The catalog schema_version is "1.0". Additive fields may appear and clients should ignore unknown fields. Existing field meanings remain unchanged within the same schema major; a breaking catalog change requires a new major schema version. This policy applies to the catalog contract and does not create a global API SLA or deprecation schedule.',
      },
      {
        id: 'versions',
        title: 'Versions and Upgrade Notes',
        body: 'The /v1 and /v1beta prefixes identify protocol route families; they do not establish a repository-wide compatibility or deprecation schedule. Task Plugin API version 1 and billing expression versions are separate contracts. Pin your deployment version and review its release notes before upgrading. The repository README links to https://github.com/QuantumNous/new-api/releases. Existing OpenAPI files are partial references and should be checked against the implemented endpoint inventory.',
      },
    ],
  },
]
