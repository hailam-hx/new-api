# DFLOP /v1/catalog billing-contract clarification for 7 models and endpoint profiles

Audit: 2026-10-01T00:19:21Z · authenticated hash `68b3ae335ece9e10b9a1b4541ca9578fdd0fb2a125184075564b13d37eeb614c` · schema `1.0` · callable `212`

Status: draft for support; not sent. No paid POST, pricing apply, adoption or Auto Apply.

## Chinese

主题：DFLOP /v1/catalog：7个模型及端点配置的计费合同澄清

请工程团队确认下列合同并在认证目录暴露实际账户价格/数量语义。我们不会从公开文档价格推断账户生效价格；未知数量不会按0处理。标准Chat/Responses保持现有支持，Fast/图片/服务端工具按独立配置阻断。

| Issue | Model/profile | Catalog says | Docs/runtime says | Exact field needed |
|---|---|---|---|---|
| SEEDANCE_LITE_UPSCALE | doubao-seedance-2.0-fast-lite, doubao-seedance-2.0-lite, doubao-seedance-2.0-mini-lite, doubao-seedance-2.5-lite | {"video_second_stage_per_second":{"doubao-seedance-2.0-fast-lite":null,"doubao-seedance-2.0-lite":null,"doubao-seedance-2.0-mini-lite":null,"doubao-seedance-2.5-lite":null}} | Lite token charge plus delivery-resolution second-stage/upscale component; docs amount is not an authenticated account price | effective second-stage 720p rate, effective second-stage 1080p rate, documented catalog field names |
| MINIMAX_H3_INPUT_SECONDS | minimax-h3 | {"features":["video_input_seconds","video_second","video_tiers"],"video_bills_input_seconds":true,"video_max_input_seconds":15} | H3 model card describes delivered output seconds; media contract identifies Wan3/WanPrime as special input+output basis | aligned billing.features/video_bills_input_seconds, canonical settlement model ID, authoritative terminal billed durations |
| QWEN_THRESHOLD_DOMAIN | qwen-image-3.0-pro | {"large_pixel_threshold":2097152,"caps":{"chat":null,"image":"[REDACTED]","surfaces":["chat","image_studio","canvas_image"],"ui":{"label":null,"rank":{},"vendor_group":null},"video":"[REDACTED]"}} | Model card lower tier <=2,360,000 pixels; 1536×1536=2,359,296 lies in disputed band; catalog min_pixels=3,686,400 conflicts with custom-size domain | authoritative threshold/operator, requested versus actual-output dimension source, enforced width/height domain, guaranteed omitted-size settlement dimensions |
| SUBTITLE_SOURCE_DURATION | tvod-subtitle-soft | {"video_price_tiers":{"asr":"0.06066","translate":"0.04044"},"price_per_video_second":"0.1011"} | source_seconds × (ASR passes × ASR rate + translation passes × translation rate); request duration is not authoritative | source_duration_sec, ledger unit_count/unit_type semantics, provider-enforced maximum source duration, reservation/probe semantics |
| GPT_FAST | gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol | {"supports_fast_mode":{"gpt-5.5":true,"gpt-5.6-luna":true,"gpt-5.6-sol":true,"gpt-5.6-terra":true,"gpt-6":true,"gpt-6-astra":true,"gpt-6-luna":true,"gpt-6-sol":true},"fast_mode_multiplier":{"gpt-5.5":"1.5","gpt-5.6-luna":"1.5","gpt-5.6-sol":"1.5","gpt-5.6-terra":"1.5","gpt-6":"1.5","gpt-6-astra":"1.5","gpt-6-luna":"2","gpt-6-sol":"2"}} | Fast feature exists but selector, served tier and effective component tariff are not sufficient for bounded route billing | provider-enforced fast selector, served tier, effective input/cached-input/output/image/tool prices, fallback tariff semantics |
| DEDICATED_IMAGE_MAPPING | codex-auto-review, gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol | {"price_per_image":{"codex-auto-review":"80.88","gpt-5.5":"80.88","gpt-5.6-luna":"80.88","gpt-5.6-sol":"80.88","gpt-5.6-terra":"80.88","gpt-6":"80.88","gpt-6-astra":"80.88","gpt-6-luna":"80.88","gpt-6-sol":"80.88"},"endpoint_type":{"codex-auto-review":null,"gpt-5.5":null,"gpt-5.6-luna":null,"gpt-5.6-sol":null,"gpt-5.6-terra":null,"gpt-6":null,"gpt-6-astra":null,"gpt-6-luna":null,"gpt-6-sol":null}} | Dedicated image endpoints use image canonical IDs; conversational image_generation is token-billed | canonical image ID and endpoint binding, effective per-image price, actual output count field, reference charge, Idempotency-Key support |
| GROK_SERVER_TOOL_CAP | grok-3-mini, grok-3-mini-fast, grok-4.20-0309-non-reasoning, grok-4.20-0309-reasoning, grok-4.20-multi-agent-0309, grok-4.3, grok-4.5, grok-4.6, grok-4.7 | {"price_per_server_tool_call":{"grok-3-mini":"2.022","grok-3-mini-fast":"2.022","grok-4.20-0309-non-reasoning":"2.022","grok-4.20-0309-reasoning":"2.022","grok-4.20-multi-agent-0309":"2.022","grok-4.3":"2.022","grok-4.5":"2.022","grok-4.6":"2.022","grok-4.7":"2.022"}} | Actual tool charge uses usage.num_server_side_tools_used; a client expectation is not a hard bound | provider-enforced maximum tool calls, exact request parameter/range, bound coverage for web_search/other/multi-agent tools |
| CLAUDE_CACHE_AFFINITY | claude-sonnet-5 | {"cache_creation_per_1m":"808.8"} | Native /v1/messages cache affinity/stickiness across upstream lanes is not established | documented cache-affinity header/guarantee, upstream lane/account correlation, protocol-translation metadata |
| SUNO_MUSIC_UNIT_WATCH | suno-v5, suno-v5.5 | {"billing_features":"music","price_field":"price_per_music_generation"} | Media contract bills one generation containing two songs; reference model list labels price per track | consistent documented generation billing unit |

### SEEDANCE_LITE_UPSCALE

Models: doubao-seedance-2.0-fast-lite, doubao-seedance-2.0-lite, doubao-seedance-2.0-mini-lite, doubao-seedance-2.5-lite

1. 请提供该账户实际生效的720p和1080p二阶段/超分单价，并在/v1/catalog中以机器可读字段暴露；请给出准确字段名。
2. 结算公式是否始终为：按交付档位/输入模式选择的token单价 × usage.completion_tokens + 按交付档位选择的二阶段单价 × 终态duration_sec？
3. 四个Lite SKU是否都叠加二阶段费用？请确认选择交付分辨率，而不是生成阶段降档分辨率。

### MINIMAX_H3_INPUT_SECONDS

Models: minimax-h3

1. minimax-h3实际按输出duration_sec、输入input_video_duration_sec加输出duration_sec，还是其他数量结算？
2. 如输入视频收费，哪个终态字段是权威数量？参考视频、视频转视频、全能参考、所有输入视频，还是特定模式收费？
3. 请统一/v1/catalog的billing.features与实际结算；并澄清minimax-h3与MiniMax-H3哪个规范ID被网关接受、返回和结算。

### QWEN_THRESHOLD_DOMAIN

Models: qwen-image-3.0-pro

1. 网关结算实际使用哪个精确像素阈值？比较运算是<=还是<？
2. 档位依据请求尺寸还是上游实际输出尺寸？DFLOP强制执行的宽高范围是什么，1536×1536是否接受？
3. min_pixels与threshold分别代表什么？省略size是否保证按2048×2048结算？请统一认证目录与公开模型卡。

### SUBTITLE_SOURCE_DURATION

Models: tvod-subtitle-soft

1. 哪个响应/任务/日志/技术字段提供权威源视频秒数？终态任务是否返回该字段？
2. 账本unit_count是源秒数、处理次数、源秒数×处理次数，还是其他单位？ASR和翻译数量如何分别表示？
3. 远程source_video_url未提供时长时如何预留额度？是否在预留前由服务端探测媒体？
4. 接受的最长源视频时长是多少，是否由提供方强制执行？公开任务响应缺失时长时，技术日志是否仍包含该字段？

### GPT_FAST

Models: gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol

1. 哪个请求字段启用Fast？是强制执行还是路由偏好？哪个响应/usage字段证明实际服务档位？
2. 实际生效的输入、缓存输入、输出及适用图片/工具价格是什么？是替换单价还是叠加费用？
3. 请求Fast能否回退Standard，回退按哪个价格结算？请提供机器可读的选择器、实际档位与生效价格。

### DEDICATED_IMAGE_MAPPING

Models: codex-auto-review, gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol

1. 这九个准确文本ID是否直接支持/v1/images/generations或/v1/images/edits？如不支持，规范图片ID及文本→图片映射（如有）是什么？
2. 认证后实际生效的每张价格是什么？n仅用于预留还是实际计费？哪个响应/账本字段提供权威实际出图数量？
3. 参考图是否单独收费？这些提交是否支持Idempotency-Key？

### GROK_SERVER_TOOL_CAP

Models: grok-3-mini, grok-3-mini-fast, grok-4.20-0309-non-reasoning, grok-4.20-0309-reasoning, grok-4.20-multi-agent-0309, grok-4.3, grok-4.5, grok-4.6, grok-4.7

1. 是否有由提供方强制执行的服务端工具调用上限参数？准确名称、最小值和最大值是什么？
2. 该上限是否覆盖web_search、其他服务端工具及多智能体工具？usage.num_server_side_tools_used能否超过客户端声明上限？
3. 如无请求上限，单次请求/账户/模型有哪些保证执行的硬上限？目录哪个字段描述最大可计费工具次数？

### CLAUDE_CACHE_AFFINITY — non-blocking

Models: claude-sonnet-5

1. 下列B/C准确请求ID的客户端缓存前缀相同，间隔89.226498秒，为何两次均创建4178 token且读取0？
2. 原生/v1/messages跨路由通道是否保持上游账户/缓存亲和性？是否有文档化粘性请求头或保证？请按准确ID检查上游账户/通道及协议转换。

### SUNO_MUSIC_UNIT_WATCH — non-blocking

Models: suno-v5, suno-v5.5

1. 请统一模型参考列表的“每首”与媒体合同的“每次生成”措辞：包含两首歌的一次生成是否只计费一次（包括部分成功情形）？

### Claude exact correlation evidence

```json
{
  "model": "claude-sonnet-5",
  "endpoint": "/v1/messages",
  "B_request_id": "dc1cc675-c379-4b12-b044-eb89c738d340",
  "C_request_id": "91abf82d-e339-4596-a2d5-d0164f2dea36",
  "prefix_hash": "608dc35570131bd44582c099c5876161683f1cf41da2373301ed413774212891",
  "prefix_identical": true,
  "cache_creation_tokens_B": 4178,
  "cache_creation_tokens_C": 4178,
  "cache_read_tokens_B": 0,
  "cache_read_tokens_C": 0,
  "seconds_B_terminal_to_C_start": "89.226498",
  "correlation": "EXACT_REQUEST_ID",
  "same_upstream_proven": false,
  "root_cause_proven": false,
  "blocking_model_coverage": false
}
```

Same client prefix does not prove the same provider-rendered prompt or upstream account. Cache-affinity incident does not block model coverage.

## English

Subject: DFLOP /v1/catalog billing-contract clarification for 7 models and endpoint profiles

Please confirm the contracts below and expose effective account prices and quantity semantics in the authenticated catalog. Public documentation prices do not substitute for account-specific amounts; missing usage is not zero. Standard Chat/Responses remain supported; unsafe Fast/image/server-tool profiles are independently blocked.

| Issue | Model/profile | Catalog says | Docs/runtime says | Exact field needed |
|---|---|---|---|---|
| SEEDANCE_LITE_UPSCALE | doubao-seedance-2.0-fast-lite, doubao-seedance-2.0-lite, doubao-seedance-2.0-mini-lite, doubao-seedance-2.5-lite | {"video_second_stage_per_second":{"doubao-seedance-2.0-fast-lite":null,"doubao-seedance-2.0-lite":null,"doubao-seedance-2.0-mini-lite":null,"doubao-seedance-2.5-lite":null}} | Lite token charge plus delivery-resolution second-stage/upscale component; docs amount is not an authenticated account price | effective second-stage 720p rate, effective second-stage 1080p rate, documented catalog field names |
| MINIMAX_H3_INPUT_SECONDS | minimax-h3 | {"features":["video_input_seconds","video_second","video_tiers"],"video_bills_input_seconds":true,"video_max_input_seconds":15} | H3 model card describes delivered output seconds; media contract identifies Wan3/WanPrime as special input+output basis | aligned billing.features/video_bills_input_seconds, canonical settlement model ID, authoritative terminal billed durations |
| QWEN_THRESHOLD_DOMAIN | qwen-image-3.0-pro | {"large_pixel_threshold":2097152,"caps":{"chat":null,"image":"[REDACTED]","surfaces":["chat","image_studio","canvas_image"],"ui":{"label":null,"rank":{},"vendor_group":null},"video":"[REDACTED]"}} | Model card lower tier <=2,360,000 pixels; 1536×1536=2,359,296 lies in disputed band; catalog min_pixels=3,686,400 conflicts with custom-size domain | authoritative threshold/operator, requested versus actual-output dimension source, enforced width/height domain, guaranteed omitted-size settlement dimensions |
| SUBTITLE_SOURCE_DURATION | tvod-subtitle-soft | {"video_price_tiers":{"asr":"0.06066","translate":"0.04044"},"price_per_video_second":"0.1011"} | source_seconds × (ASR passes × ASR rate + translation passes × translation rate); request duration is not authoritative | source_duration_sec, ledger unit_count/unit_type semantics, provider-enforced maximum source duration, reservation/probe semantics |
| GPT_FAST | gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol | {"supports_fast_mode":{"gpt-5.5":true,"gpt-5.6-luna":true,"gpt-5.6-sol":true,"gpt-5.6-terra":true,"gpt-6":true,"gpt-6-astra":true,"gpt-6-luna":true,"gpt-6-sol":true},"fast_mode_multiplier":{"gpt-5.5":"1.5","gpt-5.6-luna":"1.5","gpt-5.6-sol":"1.5","gpt-5.6-terra":"1.5","gpt-6":"1.5","gpt-6-astra":"1.5","gpt-6-luna":"2","gpt-6-sol":"2"}} | Fast feature exists but selector, served tier and effective component tariff are not sufficient for bounded route billing | provider-enforced fast selector, served tier, effective input/cached-input/output/image/tool prices, fallback tariff semantics |
| DEDICATED_IMAGE_MAPPING | codex-auto-review, gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol | {"price_per_image":{"codex-auto-review":"80.88","gpt-5.5":"80.88","gpt-5.6-luna":"80.88","gpt-5.6-sol":"80.88","gpt-5.6-terra":"80.88","gpt-6":"80.88","gpt-6-astra":"80.88","gpt-6-luna":"80.88","gpt-6-sol":"80.88"},"endpoint_type":{"codex-auto-review":null,"gpt-5.5":null,"gpt-5.6-luna":null,"gpt-5.6-sol":null,"gpt-5.6-terra":null,"gpt-6":null,"gpt-6-astra":null,"gpt-6-luna":null,"gpt-6-sol":null}} | Dedicated image endpoints use image canonical IDs; conversational image_generation is token-billed | canonical image ID and endpoint binding, effective per-image price, actual output count field, reference charge, Idempotency-Key support |
| GROK_SERVER_TOOL_CAP | grok-3-mini, grok-3-mini-fast, grok-4.20-0309-non-reasoning, grok-4.20-0309-reasoning, grok-4.20-multi-agent-0309, grok-4.3, grok-4.5, grok-4.6, grok-4.7 | {"price_per_server_tool_call":{"grok-3-mini":"2.022","grok-3-mini-fast":"2.022","grok-4.20-0309-non-reasoning":"2.022","grok-4.20-0309-reasoning":"2.022","grok-4.20-multi-agent-0309":"2.022","grok-4.3":"2.022","grok-4.5":"2.022","grok-4.6":"2.022","grok-4.7":"2.022"}} | Actual tool charge uses usage.num_server_side_tools_used; a client expectation is not a hard bound | provider-enforced maximum tool calls, exact request parameter/range, bound coverage for web_search/other/multi-agent tools |
| CLAUDE_CACHE_AFFINITY | claude-sonnet-5 | {"cache_creation_per_1m":"808.8"} | Native /v1/messages cache affinity/stickiness across upstream lanes is not established | documented cache-affinity header/guarantee, upstream lane/account correlation, protocol-translation metadata |
| SUNO_MUSIC_UNIT_WATCH | suno-v5, suno-v5.5 | {"billing_features":"music","price_field":"price_per_music_generation"} | Media contract bills one generation containing two songs; reference model list labels price per track | consistent documented generation billing unit |

### SEEDANCE_LITE_UPSCALE

Models: doubao-seedance-2.0-fast-lite, doubao-seedance-2.0-lite, doubao-seedance-2.0-mini-lite, doubao-seedance-2.5-lite

1. What are the effective account-specific second-stage/upscale rates for 720p and 1080p? Please expose both in /v1/catalog and specify exact field names.
2. Is settlement always token_rate(selected delivery tier/input mode) × usage.completion_tokens + second_stage_rate(delivery tier) × terminal duration_sec?
3. Is the second-stage component additive for all four Lite SKUs? Please confirm delivery tier rather than downgraded generation-stage tier selects its rate.

### MINIMAX_H3_INPUT_SECONDS

Models: minimax-h3

1. Does minimax-h3 settle output duration_sec only, input_video_duration_sec + output duration_sec, or another basis?
2. If input is billable, which terminal field is authoritative, and which modes are charged: reference video, video-to-video, omni-reference, all video inputs, or specific modes?
3. Please align /v1/catalog billing.features with settlement and clarify minimax-h3 versus MiniMax-H3: which canonical ID is accepted, returned and settled?

### QWEN_THRESHOLD_DOMAIN

Models: qwen-image-3.0-pro

1. Which exact pixel threshold does gateway settlement use, and is comparison <= or <?
2. Is tier selected from requested dimensions or actual upstream output dimensions? What exact width/height domain does DFLOP enforce, including whether 1536×1536 is accepted?
3. What do min_pixels and threshold mean? Is omitted size guaranteed to settle as 2048×2048? Please align authenticated catalog and public model card.

### SUBTITLE_SOURCE_DURATION

Models: tvod-subtitle-soft

1. Which response/task/log/technical field contains authoritative source video seconds, and does the terminal task return it?
2. Does ledger unit_count mean source seconds, processing passes, source seconds × processing units, or something else? How are ASR and translation quantities represented separately?
3. How is reserve determined for a remote source_video_url without supplied duration: is media probed server-side before reservation?
4. What maximum source duration is accepted, is it provider-enforced, and is source duration in technical logs when absent from public task responses?

### GPT_FAST

Models: gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol

1. What request field enables Fast, and is it provider-enforced or only a routing preference? What response/usage field proves the actually served tier?
2. What authenticated effective input, cached-input, output, and applicable image/tool prices apply? Are they replacement or additive rates?
3. Can requested Fast fall back to Standard, and which tariff is charged? Please expose selector, served tier and effective Fast prices machine-readably.

### DEDICATED_IMAGE_MAPPING

Models: codex-auto-review, gpt-5.5, gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra, gpt-6, gpt-6-astra, gpt-6-luna, gpt-6-sol

1. Do these exact nine text IDs directly support /v1/images/generations or /v1/images/edits? If not, what canonical image IDs and text→image mapping, if any, apply?
2. What are the authenticated effective per-image prices? Does n control reserve only or actual billing, and which response/ledger field is authoritative actual output count?
3. Are reference images billed separately, and is Idempotency-Key supported on these submits?

### GROK_SERVER_TOOL_CAP

Models: grok-3-mini, grok-3-mini-fast, grok-4.20-0309-non-reasoning, grok-4.20-0309-reasoning, grok-4.20-multi-agent-0309, grok-4.3, grok-4.5, grok-4.6, grok-4.7

1. Is there a provider-enforced maximum server-tool-call request parameter? What exact name, minimum and maximum apply?
2. Is the bound enforced for web_search, other server tools and multi-agent tools? Can usage.num_server_side_tools_used exceed a client-declared limit?
3. If no request bound exists, what guaranteed per-request/account/model hard ceiling applies, and which catalog field exposes maximum billable tool calls?

### CLAUDE_CACHE_AFFINITY — non-blocking

Models: claude-sonnet-5

1. For the exact B/C request IDs below, why did identical client cache prefixes produce 4178 cache-creation tokens and zero reads twice within 89.226498 seconds?
2. Does native /v1/messages preserve upstream-account/cache affinity across routed lanes? Is there a documented stickiness header or guarantee? Please inspect upstream account/lane and protocol translation for these exact IDs.

### SUNO_MUSIC_UNIT_WATCH — non-blocking

Models: suno-v5, suno-v5.5

1. Please align reference-model-list per-track wording with the authoritative media contract: is one generation including two songs billed exactly once, including partial-success behavior?

### Claude exact correlation evidence

```json
{
  "model": "claude-sonnet-5",
  "endpoint": "/v1/messages",
  "B_request_id": "dc1cc675-c379-4b12-b044-eb89c738d340",
  "C_request_id": "91abf82d-e339-4596-a2d5-d0164f2dea36",
  "prefix_hash": "608dc35570131bd44582c099c5876161683f1cf41da2373301ed413774212891",
  "prefix_identical": true,
  "cache_creation_tokens_B": 4178,
  "cache_creation_tokens_C": 4178,
  "cache_read_tokens_B": 0,
  "cache_read_tokens_C": 0,
  "seconds_B_terminal_to_C_start": "89.226498",
  "correlation": "EXACT_REQUEST_ID",
  "same_upstream_proven": false,
  "root_cause_proven": false,
  "blocking_model_coverage": false
}
```

Same client prefix does not prove the same provider-rendered prompt or upstream account. Cache-affinity incident does not block model coverage.

## Shared authenticated evidence appendix

Complete relevant raw objects are included in `dflop-provider-contract-closure.json`; the exact H3 object is reproduced below. Fields are preserved except secrets/private content. The JSON also records all catalog pricing key sets, billing features, endpoints/protocols and unblock predicates.

```json
{
  "billing": {
    "features": [
      "video_input_seconds",
      "video_second",
      "video_tiers"
    ]
  },
  "brand": "minimax",
  "caps": {
    "chat": null,
    "image": "[REDACTED]",
    "surfaces": [
      "video_workbench",
      "canvas_video"
    ],
    "ui": {
      "label": null,
      "rank": {
        "video_order": 32
      },
      "vendor_group": "海螺 MiniMax"
    },
    "video": "[REDACTED]"
  },
  "fast_mode_multiplier": null,
  "id": "minimax-h3",
  "list_pricing": {
    "adaptive_thinking": false,
    "cache_creation_per_1m": null,
    "cache_read_per_1m": null,
    "cached_input_per_1m": null,
    "cached_input_per_1m_long": null,
    "callable": true,
    "category": "video",
    "context_window": 0,
    "default_max_tokens": 0,
    "description": "MiniMax 海螺 H3 视频生成(异步任务),原生立体声音频。4-15 秒,768P / 2K,画幅 21:9 / 16:9 / 4:3 / 1:1 / 3:4 / 9:16 / adaptive。支持文生视频、首帧 / 首尾帧图生视频、全能参考(参考图 ≤5 张、参考视频 ≤3 段、参考音频 ≤3 段)。",
    "discount": null,
    "display_name": "海螺 H3",
    "endpoint": null,
    "endpoint_type": "videos_generations",
    "free_input_images": null,
    "image_price_tiers": null,
    "images_per_request": null,
    "input_per_1m": "0",
    "input_per_1m_long": null,
    "is_new": true,
    "is_open_source": false,
    "large_pixel_threshold": null,
    "long_context_threshold_tokens": null,
    "output_per_1m": "0",
    "output_per_1m_long": null,
    "price_per_avatar": null,
    "price_per_image": null,
    "price_per_image_large": null,
    "price_per_input_image": null,
    "price_per_music_generation": null,
    "price_per_server_tool_call": null,
    "price_per_tts_char": null,
    "price_per_video_second": "48",
    "price_per_video_task": null,
    "price_per_voice_clone": null,
    "protocol": null,
    "released_at": "2026-09-24",
    "supported_protocols": [],
    "supports_fast_mode": false,
    "supports_file_input": false,
    "supports_image_gen": false,
    "supports_json_mode": false,
    "supports_prefix": false,
    "supports_thinking": false,
    "supports_tools": false,
    "supports_video_generation": false,
    "supports_video_input": false,
    "supports_vision": false,
    "supports_web_search": false,
    "thoughts_per_1m": null,
    "video_bills_input_seconds": true,
    "video_max_input_seconds": 15,
    "video_price_tiers": {
      "2k": "48",
      "768p": "30"
    },
    "video_second_stage_per_second": null,
    "video_token_price_per_1m": null
  },
  "official_model_id": null,
  "pricing": {
    "adaptive_thinking": false,
    "cache_creation_per_1m": null,
    "cache_read_per_1m": null,
    "cached_input_per_1m": null,
    "cached_input_per_1m_long": null,
    "callable": true,
    "category": "video",
    "context_window": 0,
    "default_max_tokens": 0,
    "description": "MiniMax 海螺 H3 视频生成(异步任务),原生立体声音频。4-15 秒,768P / 2K,画幅 21:9 / 16:9 / 4:3 / 1:1 / 3:4 / 9:16 / adaptive。支持文生视频、首帧 / 首尾帧图生视频、全能参考(参考图 ≤5 张、参考视频 ≤3 段、参考音频 ≤3 段)。",
    "discount": null,
    "display_name": "海螺 H3",
    "endpoint": null,
    "endpoint_type": "videos_generations",
    "free_input_images": null,
    "image_price_tiers": null,
    "images_per_request": null,
    "input_per_1m": "0",
    "input_per_1m_long": null,
    "is_new": true,
    "is_open_source": false,
    "large_pixel_threshold": null,
    "long_context_threshold_tokens": null,
    "output_per_1m": "0",
    "output_per_1m_long": null,
    "price_per_avatar": null,
    "price_per_image": null,
    "price_per_image_large": null,
    "price_per_input_image": null,
    "price_per_music_generation": null,
    "price_per_server_tool_call": null,
    "price_per_tts_char": null,
    "price_per_video_second": "48",
    "price_per_video_task": null,
    "price_per_voice_clone": null,
    "protocol": null,
    "released_at": "2026-09-24",
    "supported_protocols": [],
    "supports_fast_mode": false,
    "supports_file_input": false,
    "supports_image_gen": false,
    "supports_json_mode": false,
    "supports_prefix": false,
    "supports_thinking": false,
    "supports_tools": false,
    "supports_video_generation": false,
    "supports_video_input": false,
    "supports_vision": false,
    "supports_web_search": false,
    "thoughts_per_1m": null,
    "video_bills_input_seconds": true,
    "video_max_input_seconds": 15,
    "video_price_tiers": {
      "2k": "48",
      "768p": "30"
    },
    "video_second_stage_per_second": null,
    "video_token_price_per_1m": null
  },
  "provider_slug": "x2",
  "vendor_slug": "minimax"
}
```

## Response ingestion / safety

Audit candidates require schema/contract review, tests, a fresh Pricing Preview on a DB copy, admin review and explicit manual apply. Audit never auto-promotes. PUBLIC_CATALOG_ANOMALY continues to block Auto Apply. No canary is authorized by this package.
