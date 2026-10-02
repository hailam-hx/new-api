// Endpoint contracts: https://model.dflop.top/en/docs/reference/media-apis
// and digital-human-apis. This map freezes identities and selectors, never prices.
const contracts = {
  "tvod-subtitle-soft": {"profile": "SUBTITLE_SOURCE_SECONDS_BY_OPERATION", "shape": "subtitle", "endpoint": "/v1/videos/generations", "tiers": []},
  "clip-compose": {"shape": "fixed", "endpoint": "/v1/videos/generations", "tiers": []},
  "dh-avatar": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": []},
  "dh-lipsync": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": []},
  "dh-lipsync-max": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": []},
  "dh-lipsync-pro": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": []},
  "dh-motion": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["fast", "max", "standard"]},
  "doubao-seedance-2.0": {"shape": "token", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "480p", "720p"]},
  "doubao-seedance-2.0-fast": {"shape": "token", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "480p", "720p"]},
  "doubao-seedance-2.0-fast-lite": {"shape": "token_lite", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "doubao-seedance-2.0-lite": {"shape": "token_lite", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "doubao-seedance-2.0-mini": {"shape": "token", "endpoint": "/v1/videos/generations", "tiers": ["480p", "720p"]},
  "doubao-seedance-2.0-mini-lite": {"shape": "token_lite", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "doubao-seedance-2.5": {"shape": "token", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "480p", "720p"]},
  "doubao-seedance-2.5-lite": {"shape": "token_lite", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "grok-imagine-video": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": []},
  "grok-imagine-video-1.5-preview": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": []},
  "happyhorse-1.0-i2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "happyhorse-1.0-r2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "happyhorse-1.0-t2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "happyhorse-1.0-video-edit": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "happyhorse-1.1-i2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "happyhorse-1.1-r2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "happyhorse-1.1-t2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "minimax-h3": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["2k", "768p"]},
  "minimax-h3-max": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["480p", "768p"]},
  "tvod-hailuo-02": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-hailuo-2.3": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-hailuo-2.3-fast": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-hailuo-h3": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["2k", "4k"]},
  "tvod-hunyuan-video-1.0": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-hunyuan-video-1.1": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-jimeng-1.0-lite-i2v": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-jimeng-1.0-pro": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-jimeng-1.0-pro-fast": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-jimeng-1.5-pro": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-jimeng-3.0": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-jimeng-3.0-pro": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-jimeng-4.0": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-1.6": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-2.0": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-2.1": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-2.5-turbo": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-2.6": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-3.0": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-kling-video-3.0-omni": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-kling-video-master": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-kling-video-o1": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-pixverse-c1": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-pixverse-v5.6": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-pixverse-v6": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-sora-2": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-veo-3.1": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-veo-3.1-fast": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-veo-3.1-lite": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-2.0": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q1": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q1-classic": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q2": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q2-pro": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q2-pro-fast": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q2-turbo": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "tvod-vidu-video-q3": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-vidu-video-q3-ad": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-vidu-video-q3-drama": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-vidu-video-q3-pro": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "4k", "720p"]},
  "tvod-vidu-video-q3-turbo": {"shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "wan2.7-t2v": {"profile": "VIDEO_OUTPUT_SECONDS_BY_RESOLUTION", "minDuration": 2, "maxDuration": 15, "maxInputDuration": 0, "shape": "seconds", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "720p"]},
  "wan3.0-video": {"profile": "VIDEO_INPUT_PLUS_OUTPUT_SECONDS_BY_RESOLUTION", "minDuration": 2, "maxDuration": 30, "maxInputDuration": 15, "shape": "input_output", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "480p", "720p"]},
  "wan3.0-video-prime": {"profile": "VIDEO_INPUT_PLUS_OUTPUT_SECONDS_BY_RESOLUTION", "minDuration": 2, "maxDuration": 30, "maxInputDuration": 15, "shape": "input_output", "endpoint": "/v1/videos/generations", "tiers": ["1080p", "480p", "720p"]},
  "suno-v3.5": {"shape": "music", "endpoint": "/v1/music/generations", "tiers": []},
  "suno-v4": {"shape": "music", "endpoint": "/v1/music/generations", "tiers": []},
  "suno-v4.5": {"shape": "music", "endpoint": "/v1/music/generations", "tiers": []},
  "suno-v5": {"shape": "music", "endpoint": "/v1/music/generations", "tiers": []},
  "suno-v5.5": {"shape": "music", "endpoint": "/v1/music/generations", "tiers": []},
  "voice-clone-pro": {"shape": "fixed", "endpoint": "/v1/audio/voices", "tiers": []},
  "dh-avatar-create": {"shape": "fixed", "endpoint": "/v1/videos/avatars", "tiers": []}
};
const partialClosureModels = ["clip-compose", "dh-avatar", "dh-lipsync", "dh-lipsync-pro", "dh-lipsync-max", "dh-motion", "dh-avatar-create"];
const models = Object.keys(contracts);
const videoModels = models.filter(model => contracts[model].endpoint === "/v1/videos/generations");
const musicModels = models.filter(model => contracts[model].shape === "music");
const durationField = { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } };
const inputDurationField = { type: "number", unit: "second", description: { en: "Input video processing unit price", zh: "输入视频处理单价" } };
const tokenField = { type: "number", unit: "token", description: { en: "Video generation token unit price", zh: "视频生成词元单价" } };
const countField = { type: "number", unit: "count", description: { en: "Task execution unit price", zh: "任务执行单价" } };

// Stable usage profiles are selected by the exact authenticated model contract.
// Numeric values remain measured quantities; catalog rates stay in host pricing.
const usageProfileSchemas = {
  VIDEO_OUTPUT_SECONDS_BY_RESOLUTION: { duration_sec: durationField },
  VIDEO_INPUT_PLUS_OUTPUT_SECONDS_BY_RESOLUTION: { duration_sec: durationField, input_video_duration_sec: inputDurationField },
  SUBTITLE_SOURCE_SECONDS_BY_OPERATION: {
    source_duration_sec: { type: "number", unit: "second", description: { en: "Source video processing unit price", zh: "源视频处理单价" } },
    asr_units: { type: "number", unit: "count", description: { en: "Speech recognition unit price", zh: "语音识别单价" } },
    translation_units: { type: "number", unit: "count", description: { en: "Subtitle translation unit price", zh: "字幕翻译单价" } },
  },
};

export const meta = {
  apiVersion: 1, key: "dflop-media", name: "DFLOP Media", version: "1.0.0",
  author: { name: "QuantumNous" }, fetchMode: "per_task", upstreams: ["vendor", "new_api"], models,
  protocols: [{ name: "openai_video", models: videoModels }],
  routes: [
    { method: "POST", path: "/dflop/v1/videos/generations", type: "submit", decode: "video", render: "task", models: videoModels },
    { method: "GET", path: "/dflop/v1/videos/generations/:task_id", type: "query", render: "task" },
    { method: "POST", path: "/dflop/v1/music/generations", type: "submit", decode: "music", render: "task", models: musicModels },
    { method: "GET", path: "/dflop/v1/music/generations/:task_id", type: "query", render: "task" },
    { method: "POST", path: "/dflop/v1/audio/voices", type: "submit", decode: "voice", render: "task", models: ["voice-clone-pro"] },
    { method: "GET", path: "/dflop/v1/audio/voices/:task_id", type: "query", render: "task" },
    { method: "POST", path: "/dflop/v1/videos/avatars", type: "submit", decode: "avatar", render: "task", models: ["dh-avatar-create"] },
    { method: "GET", path: "/dflop/v1/videos/avatars/:task_id", type: "query", render: "task" },
  ],
  usageSchema: { duration_sec: durationField },
  usageProfiles: models.map(model => {
    const contract = contracts[model];
    const schema = Object.assign({}, usageProfileSchemas[contract.profile] || {});
    if (!contract.profile && ["seconds", "input_output", "token_lite"].includes(contract.shape)) schema.duration_sec = durationField;
    if (["token", "token_lite"].includes(contract.shape)) {
      schema.completion_tokens = tokenField;
      schema.input_mode = { enum: ["default", "with_video_input"], description: { en: "Reference video mode", zh: "参考视频模式" } };
    }
    if (model === "dh-motion") schema.duration_sec = inputDurationField;
    if (contract.tiers.length) schema.resolution = { enum: contract.tiers, description: model === "dh-motion" ? { en: "Motion quality tier", zh: "动作质量档位" } : { en: "Output video resolution", zh: "输出视频分辨率" } };
    if (contract.shape === "fixed") schema.count = Object.assign({}, countField, { unitLabel: { en: model === "voice-clone-pro" ? "voice" : model === "dh-avatar-create" ? "avatar" : "task", zh: model === "voice-clone-pro" ? "声音" : model === "dh-avatar-create" ? "数字人" : "任务" } });
    if (contract.shape === "music") schema.generation_count = { type: "number", unit: "count", unitLabel: { en: "generation", zh: "生成" }, description: { en: "Music generation unit price", zh: "音乐生成单价" } };
    const profile = { models: [model], schema };
    if (schema.completion_tokens) {
      const facts = { completion_tokens: 1000, input_mode: "default", resolution: contract.tiers[0] };
      if (schema.duration_sec) facts.duration_sec = 5;
      profile.examples = [{ label: "Video generation", facts }];
    }
    return profile;
  }),
};

function upstreamEndpoint(ctx, endpoint) {
  const directDFLOP = /^https:\/\/api\.dflop\.top(?::443)?(?:\/|$)/i.test(ctx.baseUrl);
  return ctx.upstream && ctx.upstream.kind === "new_api" && !directDFLOP ? "/dflop" + endpoint : endpoint;
}

function canonicalModel(model) {
  return typeof model === "string" && model.toLowerCase() === "minimax-h3" ? "minimax-h3" : model;
}

function modelContract(ctx) {
  const model = canonicalModel(ctx.upstreamModel || ctx.model);
  if (!Object.prototype.hasOwnProperty.call(contracts, model)) throw new Error("Unknown DFLOP media model");
  return contracts[model];
}

function nonNegativeFact(value, name, limit, integer) {
  if (value === undefined || value === null) throw new Error(name);
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0 || value > limit || (integer && !Number.isSafeInteger(value))) throw new Error("Invalid " + name);
  return value;
}

// DFLOP's typed content references are forwarded without conversion. Reject
// unknown wire forms before reserving or sending, so absence is proven rather
// than inferred from an unrecognized reference spelling.
function wanReferenceMode(body, contract) {
  const fields = ["model", "prompt", "content", "duration", "resolution", "ratio", "watermark", "seed", "generate_audio", "negative_prompt"];
  if (Object.keys(body).some(key => !fields.includes(key))) throw new Error("Unknown Wan request field or reference representation");
  if (body.content !== undefined && !Array.isArray(body.content)) throw new Error("Wan content must be an array");
  const counts = { image_url: 0, video_url: 0, audio_url: 0 };
  for (const item of body.content || []) {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("Unknown Wan reference representation");
    if (item.type === "text") {
      if (typeof item.text !== "string" || Object.keys(item).some(key => !["type", "text"].includes(key))) throw new Error("Invalid Wan text content");
      continue;
    }
    if (!Object.prototype.hasOwnProperty.call(counts, item.type)) throw new Error("Unknown Wan reference representation");
    if (Object.keys(item).some(key => !["type", "role", item.type].includes(key))) throw new Error("Unknown Wan reference representation");
    const reference = item[item.type];
    if (!reference || typeof reference !== "object" || Array.isArray(reference) || Object.keys(reference).some(key => key !== "url") || typeof reference.url !== "string" || !/^https?:\/\//.test(reference.url)) throw new Error("Public Wan reference URL required");
    const roles = item.type === "image_url" ? ["first_frame", "last_frame", "reference_image"] : item.type === "video_url" ? ["reference_video"] : ["reference_audio"];
    if (item.role !== undefined && !roles.includes(item.role)) throw new Error("Unknown Wan reference role");
    counts[item.type]++;
  }
  if (contract.maxInputDuration === 0 && (counts.video_url || counts.audio_url || counts.image_url > 1)) throw new Error("Unsupported Wan reference mode");
  if (counts.image_url > 10 || counts.video_url > 5 || counts.audio_url > 5 || (counts.audio_url && !counts.image_url && !counts.video_url)) throw new Error("Unsupported Wan reference mode");
  return counts.video_url ? "video" : "none";
}

// Exact subtitle model-card vocabulary. Freeze unique targets in the forwarded
// request so duplicates cannot cause duplicate translation billing.
function subtitleOperations(body) {
  const languages = ["zh", "en", "ja", "ko", "fr", "es", "de", "ru", "pt", "vi", "id", "th", "ms", "ar", "hi", "it", "tr"];
  if (!languages.includes(body.source_language)) throw new Error("Unsupported subtitle source language");
  if (!Array.isArray(body.target_languages) || body.target_languages.length > languages.length || body.target_languages.some(language => !languages.includes(language))) throw new Error("Unsupported subtitle target languages");
  body.target_languages = Array.from(new Set(body.target_languages));
  if (typeof body.source_video_url !== "string" || !/^https?:\/\//.test(body.source_video_url)) throw new Error("Public subtitle source video URL required");
  if (body.burn_in !== undefined || body.subtitle_mode !== undefined || body.mode !== undefined || body.hard_subtitles !== undefined) throw new Error("Subtitle burn-in mode is unverified");
  return { source_language: body.source_language, target_languages: body.target_languages.slice(), asr_units: 1, translation_units: body.target_languages.length, processing_units: 1 + body.target_languages.length };
}

function requestIntent(ctx, endpoint) {
  if (!ctx.body || ctx.body.kind !== "json" || !ctx.body.value || typeof ctx.body.value !== "object" || Array.isArray(ctx.body.value)) throw new Error("DFLOP media requires a JSON object");
  const source = ctx.body.value;
  const model = canonicalModel(ctx.upstreamModel || source.model);
  if (!Object.prototype.hasOwnProperty.call(contracts, model) || contracts[model].endpoint !== endpoint) throw new Error("Model is not served on this DFLOP endpoint");
  const requestBody = Object.assign({}, source, {model});
  if (contracts[model].shape === "subtitle") {
    subtitleOperations(requestBody);
    // This operation ignores duration/prompt. Do not manufacture source seconds
    // from ignored client fields or download the source media for billing.
    delete requestBody.duration;
    delete requestBody.seconds;
    delete requestBody.prompt;
    return { kind: "submit", model: ctx.model || source.model, action: endpoint, requestBody };
  }
  if (ctx.operation === "remix") throw new Error("DFLOP video remix is not supported");
  // OpenAI Video calls express requested duration as seconds. Normalize once;
  // native duration and all frozen selectors describe the actual forwarded body.
  if (requestBody.seconds !== undefined) {
    if (requestBody.duration !== undefined && requestBody.duration !== requestBody.seconds) throw new Error("Conflicting video duration fields");
    requestBody.duration = requestBody.seconds;
    delete requestBody.seconds;
  }
  const contract = contracts[model];
  if (!["fixed", "music"].includes(contract.shape)) {
    if (typeof requestBody.duration !== "number" || requestBody.duration <= 0) throw new Error("A positive requested duration is required for reservation");
    nonNegativeFact(requestBody.duration, "requested duration", contract.maxDuration || 3600, false);
  }
  if (requestBody.duration !== undefined) nonNegativeFact(requestBody.duration, "requested duration", contract.maxDuration || 3600, false);
  if (contract.maxDuration) {
    if (requestBody.duration < contract.minDuration) throw new Error("Requested duration is below the Wan contract minimum");
    wanReferenceMode(requestBody, contract);
  }
  if (contract.shape === "music") {
    if (requestBody.instrumental !== true && !requestBody.prompt && !requestBody.lyrics) throw new Error("Music prompt or lyrics required");
    if (requestBody.prompt !== undefined && (typeof requestBody.prompt !== "string" || Array.from(requestBody.prompt).length > 200)) throw new Error("Music prompt must contain at most 200 characters");
    if (requestBody.lyrics !== undefined && (typeof requestBody.lyrics !== "string" || Array.from(requestBody.lyrics).length > 3000)) throw new Error("Lyrics must contain at most 3000 characters");
  }
  if (model === "voice-clone-pro" || model === "dh-avatar-create") {
    if (typeof requestBody.name !== "string" || !requestBody.name.trim()) throw new Error("A name is required");
    const url = model === "voice-clone-pro" ? requestBody.audio_url : requestBody.source_url;
    if (typeof url !== "string" || !/^https?:\/\//.test(url)) throw new Error("Public source URL required");
    if (model === "voice-clone-pro") requestBody["async"] = true;
  }
  if (["token", "token_lite"].includes(contract.shape)) {
    if (requestBody.service_tier !== undefined && requestBody.service_tier !== "default") throw new Error("Unsupported Seedance service tier");
    if (requestBody.content !== undefined && !Array.isArray(requestBody.content)) throw new Error("Seedance content must be an array");
    // Only the documented content representation is accepted for reference
    // video. No adapter transforms it before submission.
    if (requestBody.video_url !== undefined || requestBody.video_urls !== undefined || requestBody.source_video_url !== undefined) throw new Error("Seedance video references must use content video_url items");
    const videoInput = (requestBody.content || []).some(item => item && item.type === "video_url");
    if (contract.shape === "token_lite" && !requestBody.resolution) throw new Error("MISSING_ORDERED_DELIVERY_TIER");
    if (videoInput && !requestBody.resolution) throw new Error("Reference video requires explicit resolution");
    if (!requestBody.resolution) requestBody.resolution = "720p";
  }
  if (partialClosureModels.includes(model)) {
    requestBody["async"] = true;
    if (model === "dh-avatar") {
      const audio = typeof requestBody.audio_url === "string" && /^https?:\/\//.test(requestBody.audio_url);
      const speech = typeof requestBody.voice === "string" && requestBody.voice && typeof requestBody.text === "string" && requestBody.text;
      if (typeof requestBody.avatar !== "string" || !requestBody.avatar || (!audio && !speech)) throw new Error("Avatar and driving audio or voice/text required");
    }
    if (["dh-lipsync", "dh-lipsync-pro", "dh-lipsync-max", "dh-motion"].includes(model) && (typeof requestBody.source_video_url !== "string" || !/^https?:\/\//.test(requestBody.source_video_url))) throw new Error("Public source video URL required");
    if (model === "clip-compose" && (typeof requestBody.video_url !== "string" || !/^https?:\/\//.test(requestBody.video_url) || typeof requestBody.asr_id !== "string" || !requestBody.asr_id)) throw new Error("Public compose video and same-source ASR required");
    if (model === "clip-compose") { delete requestBody.duration; delete requestBody.seconds; }
    if (model.startsWith("dh-lipsync") && (typeof requestBody.audio_url !== "string" || !/^https?:\/\//.test(requestBody.audio_url))) throw new Error("Public driving audio URL required");
    if (model === "dh-motion") {
      if (requestBody.resolution === undefined) requestBody.resolution = "standard";
      if (!Number.isSafeInteger(requestBody.face_count) || requestBody.face_count < 1 || requestBody.face_count > 7) throw new Error("Motion face_count must be 1 to 7");
      if (!Array.isArray(requestBody.content) || requestBody.content.length !== requestBody.face_count || requestBody.content.some(item => !item || item.type !== "image_url" || !item.image_url || typeof item.image_url.url !== "string" || !/^https?:\/\//.test(item.image_url.url))) throw new Error("Motion requires one public portrait per face");
    }
    if (model === "dh-avatar-create") {
      if (Array.from(requestBody.name).length > 20) throw new Error("Avatar name must contain at most 20 characters");
      if (requestBody.source_kind === undefined) requestBody.source_kind = "image";
      if (!["image", "video"].includes(requestBody.source_kind)) throw new Error("Unknown avatar source kind");
    }
  }
  if (contract.tiers.length && !contract.tiers.includes(requestBody.resolution)) throw new Error("MISSING_SELECTED_TIER: explicit supported resolution required");
  return { kind: "submit", model: ctx.model || source.model, action: endpoint, requestBody };
}

export const native = {
  video(ctx) { return requestIntent(ctx, "/v1/videos/generations"); },
  music(ctx) { return requestIntent(ctx, "/v1/music/generations"); },
  voice(ctx) { return requestIntent(ctx, "/v1/audio/voices"); },
  avatar(ctx) { return requestIntent(ctx, "/v1/videos/avatars"); },
  task(_ctx, task) { return task.data; },
};
export const protocols = { openai_video: {
  decodeRequest(ctx) { return requestIntent(ctx, "/v1/videos/generations"); },
  render(_ctx, task) { return task.data; },
} };

export function buildSubmitRequest(ctx) {
  const contract = modelContract(ctx);
  if (contract.shape === "subtitle") {
    subtitleOperations(ctx.requestBody || {});
    throw new Error("MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION");
  }
  let body = Object.assign({}, ctx.requestBody, { model: ctx.upstreamModel || ctx.model });
  // Revalidate normalized body immediately before any billed upstream request.
  const normalized = requestIntent({ body: { kind: "json", value: body } }, contract.endpoint);
  body = normalized.requestBody;
  const headers = ctx.requestHeaders || {};
  const key = headers["Idempotency-Key"] || headers["idempotency-key"];
  if (typeof key !== "string" || !/^[\x20-\x7e]{1,200}$/.test(key)) throw new Error("A valid Idempotency-Key is required");
  // Voice/avatar endpoints select their fixed SKU implicitly.
  if (contract.endpoint === "/v1/audio/voices" || contract.endpoint === "/v1/videos/avatars") delete body.model;
  return { url: ctx.baseUrl + upstreamEndpoint(ctx, contract.endpoint), method: "POST", headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key }, body, rewriteModel: body.model === "minimax-h3" ? "minimax-h3" : undefined, action: contract.endpoint };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const contract = modelContract(ctx);
  const req = ctx.requestBody || {};
  if (contract.shape === "subtitle") throw new Error("MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION");
  const facts = {};
  if (contract.shape === "fixed") return { count: 1 };
  if (contract.shape === "music") return { generation_count: 1 };
  const seconds = nonNegativeFact(req.duration, "requested duration", contract.maxDuration || 3600, false);
  if (contract.maxDuration && seconds < contract.minDuration) throw new Error("Requested duration is below the Wan contract minimum");
  const referenceMode = contract.maxDuration ? wanReferenceMode(req, contract) : undefined;
  if (["seconds", "input_output", "token_lite"].includes(contract.shape)) facts.duration_sec = seconds;
  if (contract.shape === "input_output") facts.input_video_duration_sec = referenceMode === "video" ? contract.maxInputDuration : 0;
  if (contract.tiers.length) {
    if (!contract.tiers.includes(req.resolution)) throw new Error("MISSING_SELECTED_TIER");
    facts.resolution = req.resolution;
  }
  if (["token", "token_lite"].includes(contract.shape)) {
    const hasVideo = (req.content || []).some(item => item && item.type === "video_url");
    facts.input_mode = hasVideo ? "with_video_input" : "default";
    const model = ctx.upstreamModel || ctx.model;
    const inputCap = model.includes("2.5") ? 30 : 15;
    if (!Number.isInteger(seconds) || seconds < 4 || seconds > inputCap) throw new Error("Requested Seedance duration must be an integer within the documented model bounds");
    // Official Ark ratio tables, linked from the DFLOP endpoint contract:
    // https://docs.volcengine.com/docs/ark/create-video-generation-task-api?lang=zh
    // Lite generates one tier down; the price selector stays at delivery resolution.
    const generationResolution = contract.shape === "token_lite" ? (req.resolution === "720p" ? "480p" : "720p") : req.resolution;
    const framesByResolution = {
      "480p": model.includes("2.5")
        ? [[854, 480], [752, 560], [640, 640], [560, 752], [480, 854], [992, 432]]
        : [[864, 496], [752, 560], [640, 640], [560, 752], [496, 864], [992, 432]],
      "720p": [[1280, 720], [1112, 834], [960, 960], [834, 1112], [720, 1280], [1470, 630]],
      "1080p": [[1920, 1080], [1664, 1248], [1440, 1440], [1248, 1664], [1080, 1920], [2206, 946]],
    };
    const frames = framesByResolution[generationResolution];
    if (!frames) throw new Error("PROVIDER_TOKEN_CEILING_REQUIRED");
    let pixels = 0;
    for (const frame of frames) pixels = Math.max(pixels, frame[0] * frame[1]);
    // All validated durations and integer pixel products remain below 2^53;
    // division by 1024 is exact in IEEE-754. Round the integer-token hold upward.
    // Final settlement always consumes authoritative usage.completion_tokens.
    facts.completion_tokens = Math.ceil((seconds + (hasVideo ? inputCap : 0)) * pixels * 24 / 1024);
    nonNegativeFact(facts.completion_tokens, "reserved video tokens", 2147483647, true);
  }
  return facts;
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  if (typeof body.id !== "string" || !body.id || (body.model !== undefined && canonicalModel(body.model) !== canonicalModel(ctx.upstreamModel || ctx.model))) throw new Error("Invalid DFLOP media acknowledgement");
  const contract = modelContract(ctx);
  const subtitle = contract.shape === "subtitle";
  const facts = subtitle ? {} : extractUsage(ctx);
  const state = subtitle ? subtitleOperations(ctx.requestBody || {}) : {};
  if (contract.maxDuration) {
    state.requested_duration_sec = facts.duration_sec;
    state.reference_mode = wanReferenceMode(ctx.requestBody || {}, contract);
  }
  if (partialClosureModels.includes(canonicalModel(ctx.upstreamModel || ctx.model))) {
    state.billing_contract = "DFLOP_PARTIAL_CLOSURE_V1";
    state.reservation = facts;
  }
  if (facts.resolution !== undefined) state.resolution = facts.resolution;
  if (facts.input_mode !== undefined) state.input_mode = facts.input_mode;
  const result = { taskId: body.id, taskData: body, state };
  if (["succeeded", "ready", "failed", "expired", "cancelled"].includes(body.status)) {
    result.immediate = parseTaskResult(Object.assign({}, ctx, { taskId: body.id, state }), body);
    result.state = result.immediate.state;
  }
  return result;
}

export function buildQueryRequest(ctx) {
  if (typeof ctx.taskId !== "string" || !ctx.taskId) throw new Error("Missing upstream DFLOP task ID");
  return { url: ctx.baseUrl + upstreamEndpoint(ctx, modelContract(ctx).endpoint) + "/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

export function extractUsageOnComplete(ctx, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const contract = modelContract(ctx);
  // No authenticated historical subtitle terminal quantity was available.
  // Generic duration_sec and ledger unit_count do not prove source duration or
  // processing-pass semantics for this SKU. Preserve a successful billing hold.
  if (contract.shape === "subtitle") throw new Error("MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION");
  const facts = {};
  if (contract.shape === "fixed") return { count: 1 };
  if (contract.shape === "music") {
    if (!Array.isArray(body.tracks) || !body.tracks.some(track => track && typeof track.audio_url === "string" && track.audio_url)) throw new Error("MISSING_SUCCESSFUL_SONG");
    return { generation_count: 1 };
  }
  const model = canonicalModel(ctx.upstreamModel || ctx.model);
  if (partialClosureModels.includes(model) && contract.shape === "seconds") {
    if ((ctx.state || {}).billing_contract !== "DFLOP_PARTIAL_CLOSURE_V1") throw new Error("MISSING_FROZEN_BILLING_CONTRACT");
    // Motion charges source processing seconds; delivered output length is not evidence of this quantity.
    const value = model === "dh-motion" ? body.source_duration_sec : body.duration_sec;
    facts.duration_sec = nonNegativeFact(value, model === "dh-motion" ? "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION" : "MISSING_FINAL_DURATION", 3600, false);
    if (facts.duration_sec <= 0) throw new Error("Invalid authoritative video duration");
  } else if (["seconds", "input_output", "token_lite"].includes(contract.shape)) facts.duration_sec = nonNegativeFact(body.duration_sec, "MISSING_FINAL_DURATION", 3600, false);
  if (contract.maxDuration) {
    const state = ctx.state || {};
    const ceiling = nonNegativeFact(state.requested_duration_sec, "MISSING_FROZEN_DURATION_CEILING", contract.maxDuration, false);
    if (ceiling < contract.minDuration) throw new Error("Invalid frozen duration ceiling");
    if (facts.duration_sec <= 0 || facts.duration_sec > ceiling) throw new Error("Invalid delivered duration for frozen request ceiling");
    if (!contract.tiers.includes(state.resolution)) throw new Error("MISSING_SELECTED_TIER");
    if (!["none", "video"].includes(state.reference_mode) || (contract.maxInputDuration === 0 && state.reference_mode !== "none")) throw new Error("MISSING_VIDEO_INPUT_SELECTOR");
    if (contract.shape === "input_output") {
      if (state.reference_mode === "video") {
        facts.input_video_duration_sec = nonNegativeFact(body.input_video_duration_sec, "MISSING_INPUT_VIDEO_DURATION", contract.maxInputDuration, false);
        if (facts.input_video_duration_sec <= 0) throw new Error("Invalid reference video duration");
      } else {
        if (body.input_video_duration_sec !== undefined && nonNegativeFact(body.input_video_duration_sec, "input video duration", contract.maxInputDuration, false) !== 0) throw new Error("VIDEO_INPUT_SELECTOR_CHANGED");
        facts.input_video_duration_sec = 0;
      }
    }
  }
  if (["token", "token_lite"].includes(contract.shape) && body.service_tier !== undefined && body.service_tier !== "default") throw new Error("MISSING_SELECTED_TIER");
  if (["token", "token_lite"].includes(contract.shape)) facts.completion_tokens = nonNegativeFact((body.usage || {}).completion_tokens, "MISSING_VIDEO_TOKEN_USAGE", 2147483647, true);
  if (contract.shape === "token_lite" && !contract.tiers.includes((ctx.state || {}).resolution)) throw new Error("MISSING_ORDERED_DELIVERY_TIER");
  if (contract.tiers.length) {
    if (!contract.tiers.includes(body.resolution)) throw new Error("MISSING_SELECTED_TIER");
    // Frozen delivered-tier selector may be confirmed, never silently changed.
    if (ctx.state && ctx.state.resolution && body.resolution !== ctx.state.resolution) throw new Error("SELECTED_TIER_CHANGED");
    facts.resolution = contract.shape === "token_lite" ? ctx.state.resolution : body.resolution;
  }
  if (["token", "token_lite"].includes(contract.shape)) {
    const inputMode = (ctx.state || {}).input_mode;
    if (!["default", "with_video_input"].includes(inputMode)) throw new Error("MISSING_VIDEO_INPUT_SELECTOR");
    facts.input_mode = inputMode;
  }
  return facts;
}

export function parseTaskResult(ctx, body) {
  const contract = modelContract(ctx);
  if (!body || typeof body.id !== "string" || body.id !== ctx.taskId || (body.model !== undefined && canonicalModel(body.model) !== canonicalModel(ctx.upstreamModel || ctx.model))) throw new Error("Invalid DFLOP task identity");
  if (["failed", "expired", "cancelled"].includes(body.status)) return { taskId: body.id, status: "FAILURE", reason: String((body.error || {}).message || body.error_message || "DFLOP task failed") };
  if (["queued", "running", "processing", "pending"].includes(body.status)) return { taskId: body.id, status: "IN_PROGRESS" };
  if (body.status !== "succeeded" && !(body.status === "ready" && ["/v1/audio/voices", "/v1/videos/avatars"].includes(contract.endpoint))) return { taskId: body.id, status: "UNKNOWN" };
  const state = Object.assign({}, ctx.state || {}, { billingPending: false });
  try { extractUsageOnComplete(ctx, { status: "SUCCESS" }, body); }
  catch (error) { state.billingPending = true; state.blocker = String(error.message); }
  return { taskId: body.id, status: "SUCCESS", url: String(body.video_url || (body.content || {}).video_url || ""), state };
}

export function listArtifacts(task) {
  if (task.status !== "SUCCESS") return [];
  const body = task.data || {};
  const artifacts = [];
  const video = body.video_url || (body.content || {}).video_url;
  if (typeof video === "string" && video) artifacts.push({ key: "video", type: "video" });
  // Official VideoTask output_files carry {lang, kind, url}; file quantities
  // never participate in generation billing. Respect the host artifact ceiling.
  for (const [index, file] of (body.output_files || []).entries()) {
    if (artifacts.length >= 64) break;
    if (file && typeof file.url === "string" && file.url) artifacts.push({ key: "file_" + index, type: "file" });
  }
  for (const [index, track] of (body.tracks || []).entries()) {
    if (artifacts.length >= 64) break;
    if (track && typeof track.audio_url === "string" && track.audio_url) artifacts.push({ key: "audio_" + index, type: "audio" });
  }
  return artifacts;
}

export function buildContentRequest(ctx) {
  const body = ctx.data || {};
  let url;
  if (ctx.artifactKey === "video") url = body.video_url || (body.content || {}).video_url;
  else if (/^file_[0-9]+$/.test(ctx.artifactKey)) url = ((body.output_files || [])[Number(ctx.artifactKey.slice(5))] || {}).url;
  else if (/^audio_[0-9]+$/.test(ctx.artifactKey)) url = ((body.tracks || [])[Number(ctx.artifactKey.slice(6))] || {}).audio_url;
  if (typeof url !== "string" || !url) throw new Error("artifact_not_found");
  return { url, method: ctx.clientRequest.method, credentialless: true };
}
