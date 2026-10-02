// DFLOP Images endpoint contract, distinct from Alibaba's native image API.
// Prices come from the authenticated catalog; this plugin reports quantities.
// Quantity contracts from authenticated GET /v1/catalog, 2026-10-02.
// SHA-256: 00824e700ae039bf6ceefeacc176bec8838d59abdfb13ce57e8ae1de95a37c80.
// This registry contains no prices. Public image-tier documentation conflicts
// with this catalog and must not supply thresholds or output-size guarantees.
const CONTRACT_SOURCE = "00824e700ae039bf6ceefeacc176bec8838d59abdfb13ce57e8ae1de95a37c80";
const IMAGE_PER_OUTPUT = "IMAGE_PER_OUTPUT";
const IMAGE_PER_OUTPUT_PLUS_REFERENCE = "IMAGE_PER_OUTPUT_PLUS_REFERENCE";
const IMAGE_PIXEL_TIER = "IMAGE_PIXEL_TIER";
const CONTRACTS = {
  "doubao-seedream-4-0-250828": { profile: IMAGE_PER_OUTPUT, max_outputs: 15, max_refs: 14, refs_plus_outputs_max: 15 },
  "doubao-seedream-4-5-251128": { profile: IMAGE_PER_OUTPUT, max_outputs: 15, max_refs: 14, refs_plus_outputs_max: 15 },
  "doubao-seedream-5-0-260128": { profile: IMAGE_PER_OUTPUT, max_outputs: 15, max_refs: 14, refs_plus_outputs_max: 15 },
  "doubao-seedream-5-0-pro-260628": { profile: IMAGE_PIXEL_TIER, max_outputs: 1, max_refs: 10, free_input_images: 1, large_pixel_threshold: 2610000 },
  "qwen-image-3.0": { profile: IMAGE_PER_OUTPUT_PLUS_REFERENCE, max_outputs: 9, max_refs: 50, free_input_images: 0 },
  "qwen-image-3.0-pro": { profile: IMAGE_PIXEL_TIER, max_outputs: 9, max_refs: 50, free_input_images: 0, large_pixel_threshold: 2097152 },
  "tvod-midjourney-v7": { profile: IMAGE_PER_OUTPUT, max_outputs: 10, max_refs: 3, fixed_outputs: 4 },
  "tvod-midjourney-v8.1": { profile: IMAGE_PER_OUTPUT, max_outputs: 10, max_refs: 3, fixed_outputs: 4 },
};
const MODELS = Object.keys(CONTRACTS);
const IMAGE_FIELD = { type: "number", unit: "count", unitLabel: { en: "image", zh: "张" }, description: { en: "Image generation unit price", zh: "图片生成单价" } };
export const meta = {
  apiVersion: 1, key: "dflop-image", name: "DFLOP Images", version: "1.0.0",
  author: { name: "QuantumNous" }, models: MODELS, fetchMode: "per_task", upstreams: ["vendor", "new_api"],
  baseUrl: "https://api.dflop.top",
  protocols: ["openai_image"],
  routes: [
    { method: "POST", path: "/dflop-image/v1/images/generations", type: "submit", decode: "generate", render: "created", models: MODELS },
    { method: "POST", path: "/dflop-image/v1/images/edits", type: "submit", decode: "edit", render: "created", models: MODELS },
    { method: "GET", path: "/dflop-image/v1/images/generations/:task_id", type: "query", render: "status" },
  ],
  usageSchema: { image_count: IMAGE_FIELD },
  usageProfiles: [
    { models: MODELS.filter((model) => CONTRACTS[model].profile === IMAGE_PER_OUTPUT), schema: { image_count: IMAGE_FIELD } },
    { models: MODELS.filter((model) => CONTRACTS[model].profile === IMAGE_PER_OUTPUT_PLUS_REFERENCE), schema: {
      image_count: IMAGE_FIELD,
      input_image_count: { ...IMAGE_FIELD, description: { en: "Reference image unit price", zh: "参考图片单价" } },
    } },
    { models: MODELS.filter((model) => CONTRACTS[model].profile === IMAGE_PIXEL_TIER), schema: {
      image_count: IMAGE_FIELD,
      small_image_count: { ...IMAGE_FIELD, description: { en: "Small image generation unit price", zh: "小尺寸图片生成单价" } },
      large_image_count: { ...IMAGE_FIELD, description: { en: "Large image generation unit price", zh: "大尺寸图片生成单价" } },
      input_image_count: { ...IMAGE_FIELD, description: { en: "Reference image unit price", zh: "参考图片单价" } },
    } },
  ],
};

function imageDimensions(value) {
  const match = typeof value === "string" && /^(\d+)[x*](\d+)$/.exec(value);
  if (!match) throw new Error("authoritative image dimensions are missing");
  const width = Number(match[1]), height = Number(match[2]);
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width < 1 || height < 1 || width > 4096 || height > 4096) throw new Error("invalid image dimensions");
  return width * height;
}

// Reusable output-count rule: authoritative unit_count first; valid image
// payloads next. The nominal fixed-output contract reserves submission only.
// OpenAI-compatible split url/b64 entries represent one image, not two.
function actualOutputCount(body, contract) {
  const entries = Array.isArray(body.data) ? body.data : body.data && typeof body.data === "object" ? [body.data] : [];
  const urls = entries.filter((entry) => entry && typeof entry.url === "string" && entry.url.length > 0);
  const base64 = entries.filter((entry) => entry && typeof entry.b64_json === "string" && entry.b64_json.length > 0);
  const payloadCount = Math.max(urls.length, base64.length);
  const usage = body.usage || {};
  const reported = body.unit_count !== undefined ? body.unit_count : usage.unit_count !== undefined ? usage.unit_count : usage.output_image_count;
  for (const count of [body.unit_count, usage.unit_count, usage.output_image_count]) {
    if (count !== undefined && count !== reported) throw new Error("authoritative output counts disagree");
  }
  if (reported !== undefined) {
    if (!Number.isSafeInteger(reported) || reported < 1 || reported > contract.max_outputs || payloadCount === 0 || payloadCount > contract.max_outputs || reported < payloadCount) throw new Error("invalid authoritative output count");
    return { count: reported, images: urls.length >= base64.length ? urls : base64 };
  }
  if (!payloadCount || payloadCount > contract.max_outputs) throw new Error("authoritative image output count is missing");
  return { count: payloadCount, images: urls.length >= base64.length ? urls : base64 };
}

function imageUsage(ctx, body) {
  const model = ctx.upstreamModel || ctx.model;
  if (!MODELS.includes(model) || (body.model !== undefined && body.model !== model)) throw new Error("DFLOP image model mismatch");
  const state = ctx.state || {};
  const contract = state.contract;
  if (!contract || contract.model !== model) throw new Error("frozen image contract is missing");
  const actual = actualOutputCount(body, contract);
  const facts = { image_count: actual.count };
  if (contract.profile === IMAGE_PER_OUTPUT) return facts;
  // Reference count is the number actually sent at submit. The frozen pricing
  // expression applies its catalog free allowance; quantity stays unadjusted.
  const input = state.input_image_count;
  if (!Number.isSafeInteger(input) || input < 0 || input > contract.max_refs) throw new Error("authoritative input image count is missing");
  const usage = body.usage || {};
  for (const reported of [body.input_image_count, usage.input_image_count]) {
    if (reported !== undefined && reported !== input) throw new Error("authoritative input image counts disagree");
  }
  facts.input_image_count = input;
  if (contract.profile === IMAGE_PER_OUTPUT_PLUS_REFERENCE) return facts;
  if (contract.profile !== IMAGE_PIXEL_TIER || !Number.isSafeInteger(contract.large_pixel_threshold) || contract.large_pixel_threshold < 1) throw new Error("authoritative image threshold is missing");
  const hasOutputSize = usage.output_width !== undefined || usage.output_height !== undefined;
  if (hasOutputSize && (!Number.isSafeInteger(usage.output_width) || !Number.isSafeInteger(usage.output_height))) throw new Error("invalid authoritative output dimensions");
  const outputSize = hasOutputSize ? usage.output_width + "x" + usage.output_height : undefined;
  if (actual.count !== actual.images.length && outputSize === undefined) throw new Error("authoritative output dimensions do not cover delivered images");
  let small = 0, large = 0;
  for (const image of actual.images) {
    // Only terminal provider dimensions are authoritative; neither omitted
    // sizes nor explicit request sizes can supply final billed dimensions.
    const size = image.size !== undefined ? image.size : (image.width !== undefined || image.height !== undefined ? image.width + "x" + image.height : outputSize);
    const pixels = imageDimensions(size);
    if (actual.count !== actual.images.length && pixels !== imageDimensions(outputSize)) throw new Error("authoritative output dimensions disagree");
    if (pixels <= contract.large_pixel_threshold) small++; else large++;
  }
  if (actual.count !== actual.images.length) {
    const isSmall = imageDimensions(outputSize) <= contract.large_pixel_threshold;
    small = isSmall ? actual.count : 0;
    large = isSmall ? 0 : actual.count;
  }
  return { ...facts, small_image_count: small, large_image_count: large };
}

export const protocols = { openai_image: {
  decodeRequest(ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !ctx.body.value || typeof ctx.body.value !== "object") throw new Error("DFLOP Images requires a JSON request");
    const source = ctx.body.value;
    const model = ctx.upstreamModel || ctx.model;
    if (!MODELS.includes(model)) throw new Error("unsupported DFLOP image model");
    if (typeof source.prompt !== "string" || !source.prompt.trim()) throw new Error("prompt is required");
    const contract = CONTRACTS[model];
    const n = source.n === undefined ? 1 : source.n;
    if (!Number.isSafeInteger(n) || n < 1 || n > contract.max_outputs) throw new Error("invalid image count");
    if (source.size !== undefined) imageDimensions(source.size);
    let images = source.image === undefined ? [] : Array.isArray(source.image) ? source.image.slice() : [source.image];
    if (source.image_urls !== undefined) {
      if (!Array.isArray(source.image_urls)) throw new Error("image_urls must be an array");
      images = images.concat(source.image_urls);
    }
    if (images.length > contract.max_refs || images.some((image) => typeof image !== "string" || !image)) throw new Error("invalid input image references");
    if (contract.refs_plus_outputs_max && images.length + n > contract.refs_plus_outputs_max) throw new Error("input references and outputs exceed provider limit");
    if (ctx.operation === "edit" && !images.length) throw new Error("image is required for edits");
    if (source.stream === true) throw new Error("stream is unsupported");
    if (source.response_format !== undefined && !["url", "b64_json"].includes(source.response_format)) throw new Error("invalid response_format");
    const request = { model: ctx.model, prompt: source.prompt, n, "async": true };
    for (const key of ["size", "quality", "style", "seed"]) if (source[key] !== undefined) request[key] = source[key];
    if (images.length) request.image = images;
    // Host owns b64 conversion; URLs allow provider idempotent replay.
    request.response_format = "url";
    return { kind: "submit", model: ctx.model, action: ctx.operation === "edit" ? "edit" : "generate", requestBody: request };
  },
  render(_ctx, task) {
    const body = task.data || {};
    if (body.data && !Array.isArray(body.data) && typeof body.data === "object") return { ...body, data: [body.data] };
    return body;
  },
} };

export const native = {
  generate(ctx) { return protocols.openai_image.decodeRequest({ ...ctx, model: ctx.body.value.model, operation: "generate" }); },
  edit(ctx) { return protocols.openai_image.decodeRequest({ ...ctx, model: ctx.body.value.model, operation: "edit" }); },
  created(_ctx, task) { return task.status === "SUCCESS" ? task.data : { id: task.task_id, model: (task.data || {}).model || (task.properties || {}).origin_model_name, status: "queued" }; },
  status(_ctx, task) { return task.status === "SUCCESS" ? task.data : { id: task.task_id, model: (task.data || {}).model || (task.properties || {}).origin_model_name, status: task.status === "FAILURE" ? "failed" : "running" }; },
};

function imageEndpoint(ctx) {
  const base = ctx.baseUrl.replace(/\/$/, "");
  const direct = /^https:\/\/api\.dflop\.top(?:\/v1)?$/.test(base);
  if (direct) return "https://api.dflop.top/v1/images";
  return base + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/dflop-image" : "") + "/v1/images";
}

export function buildSubmitRequest(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  if (!MODELS.includes(model)) throw new Error("unsupported mapped DFLOP image model");
  const key = (ctx.requestHeaders || {})["Idempotency-Key"];
  if (!key) throw new Error("Idempotency-Key is required");
  return { method: "POST", url: imageEndpoint(ctx) + "/" + (ctx.action === "edit" ? "edits" : "generations"),
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
    body: { ...ctx.requestBody, model }, action: ctx.action };
}
export function extractUsage(ctx) {
  const request = ctx.requestBody || {};
  const model = ctx.upstreamModel || ctx.model;
  const contract = CONTRACTS[model];
  if (!contract) throw new Error("unsupported DFLOP image model");
  const count = contract.fixed_outputs || (request.n === undefined ? 1 : request.n);
  const input = (request.image || []).length;
  if (!Number.isSafeInteger(count) || count < 1 || count > contract.max_outputs || input > contract.max_refs || (contract.refs_plus_outputs_max && count + input > contract.refs_plus_outputs_max)) throw new Error("invalid image reservation count");
  if (ctx.usagePurpose === "billing_ratios" || contract.profile === IMAGE_PER_OUTPUT) return { image_count: count };
  const facts = { image_count: count, input_image_count: input };
  if (contract.profile === IMAGE_PER_OUTPUT_PLUS_REFERENCE) return facts;
  // Reserve the expensive band: requested dimensions are not guaranteed to
  // equal delivered dimensions. Terminal metadata determines the settlement.
  return { ...facts, small_image_count: 0, large_image_count: count };
}
export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  const request = ctx.requestBody || {};
  const model = ctx.upstreamModel || ctx.model;
  if (!CONTRACTS[model]) throw new Error("unsupported DFLOP image model");
  const state = { input_image_count: (request.image || []).length, contract: { ...CONTRACTS[model], model, source: CONTRACT_SOURCE } };
  if (body.data !== undefined) {
    const immediate = parseTaskResult({ ...ctx, state }, body, { status: response.statusCode });
    return { taskId: body.request_id || utils.uuid(), taskData: body, state: immediate.state || state, immediate };
  }
  if (typeof body.id !== "string" || !body.id || body.model !== (ctx.upstreamModel || ctx.model)) throw new Error("invalid DFLOP image acknowledgement");
  return { taskId: body.id, taskData: body, state };
}
export function buildQueryRequest(ctx) {
  return { method: "GET", url: imageEndpoint(ctx) + "/generations/" + encodeURIComponent(ctx.taskId), headers: { Authorization: "Bearer " + ctx.apiKey } };
}
export function parseTaskResult(ctx, body, response) {
  if (body.model !== undefined && body.model !== (ctx.upstreamModel || ctx.model)) throw new Error("DFLOP image model mismatch");
  if (response && response.status >= 400 || ["failed", "expired", "cancelled"].includes(body.status)) return { status: "FAILURE", reason: String((body.error || {}).message || "image generation failed") };
  if (["queued", "running"].includes(body.status)) return { status: body.status === "queued" ? "QUEUED" : "IN_PROGRESS" };
  if (!["completed", "succeeded"].includes(body.status) && Array.isArray(body.data) && body.data.length === 0 && (body.unit_count === undefined || body.unit_count === 0)) return { status: "FAILURE", reason: "image generation returned zero outputs" };
  if (body.data !== undefined || ["completed", "succeeded"].includes(body.status)) {
    try { imageUsage(ctx, body); }
    catch (error) {
      // Successful delivery with missing metering is held for operator review.
      // It must never be refunded merely because the poller times out.
      let blocker = "MISSING_AUTHORITATIVE_IMAGE_USAGE";
      if (error.message.includes("threshold")) blocker = "PROVIDER_CONTRACT_IMAGE_THRESHOLD_CONFLICT";
      else if (error.message.includes("dimensions")) blocker = "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS";
      return { status: "SUCCESS", reason: error.message, state: { ...ctx.state, billingPending: true, blocker } };
    }
    return { status: "SUCCESS", state: { ...ctx.state, billingPending: false } };
  }
  return { status: "UNKNOWN" };
}
export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS") return {};
  if (ctx.state && ctx.state.billingPending === true) return {};
  return imageUsage(ctx, body);
}
