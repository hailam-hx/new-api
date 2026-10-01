// DFLOP Images endpoint contract, distinct from Alibaba's native image API.
// Prices come from the authenticated catalog; this plugin reports quantities.
const MODELS = ["qwen-image-3.0-pro", "tvod-midjourney-v7", "tvod-midjourney-v8.1"];
// Authenticated catalog machine threshold. Qwen remains pricing-blocked while
// the public model card's different threshold is unresolved by the provider.
const QWEN_LARGE_PIXEL_THRESHOLD = 2097152;
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
  usageProfiles: [{ models: ["qwen-image-3.0-pro"], schema: {
    image_count: IMAGE_FIELD,
    small_image_count: { ...IMAGE_FIELD, description: { en: "Small image generation unit price", zh: "小尺寸图片生成单价" } },
    large_image_count: { ...IMAGE_FIELD, description: { en: "Large image generation unit price", zh: "大尺寸图片生成单价" } },
    input_image_count: { ...IMAGE_FIELD, description: { en: "Input image unit price", zh: "输入图片单价" } },
  } }],
};

function imageDimensions(value) {
  const match = typeof value === "string" && /^(\d+)[x*](\d+)$/.exec(value);
  if (!match) throw new Error("authoritative image dimensions are missing");
  const width = Number(match[1]), height = Number(match[2]);
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width < 1 || height < 1 || width > 4096 || height > 4096) throw new Error("invalid image dimensions");
  return width * height;
}

// Reusable output-count rule: authoritative unit_count first; valid image
// payloads next; a fixed-output contract only for the exact Midjourney SKUs.
// OpenAI-compatible split url/b64 entries represent one image, not two.
function actualOutputCount(body, fixedCount) {
  const entries = Array.isArray(body.data) ? body.data : body.data && typeof body.data === "object" ? [body.data] : [];
  const urls = entries.filter((entry) => entry && typeof entry.url === "string" && entry.url.length > 0);
  const base64 = entries.filter((entry) => entry && typeof entry.b64_json === "string" && entry.b64_json.length > 0);
  const payloadCount = Math.max(urls.length, base64.length);
  const usage = body.usage || {};
  const reported = body.unit_count !== undefined ? body.unit_count : usage.unit_count !== undefined ? usage.unit_count : usage.output_image_count;
  if (usage.output_image_count !== undefined && reported !== usage.output_image_count) throw new Error("authoritative output counts disagree");
  if (reported !== undefined) {
    if (!Number.isSafeInteger(reported) || reported < 1 || reported > 10 || payloadCount === 0) throw new Error("invalid authoritative output count");
    return { count: reported, images: urls.length >= base64.length ? urls : base64 };
  }
  if (!payloadCount || payloadCount > 10) throw new Error("authoritative image output count is missing");
  return { count: fixedCount || payloadCount, images: urls.length >= base64.length ? urls : base64 };
}

function imageUsage(ctx, body) {
  const model = ctx.upstreamModel || ctx.model;
  if (!MODELS.includes(model) || (body.model !== undefined && body.model !== model)) throw new Error("DFLOP image model mismatch");
  const fixed = model === "qwen-image-3.0-pro" ? null : 4;
  const actual = actualOutputCount(body, fixed);
  if (fixed) return { image_count: actual.count };
  const usage = body.usage || {};
  // Alibaba's OpenAI-compatible usage describes all generated outputs. DFLOP
  // preserves upstream usage. Explicit terminal dimensions override its SKU's
  // guaranteed omitted-size default; requested explicit sizes are never facts.
  const hasOutputSize = usage.output_width !== undefined || usage.output_height !== undefined;
  if (hasOutputSize && (!Number.isSafeInteger(usage.output_width) || !Number.isSafeInteger(usage.output_height))) throw new Error("invalid authoritative output dimensions");
  const outputSize = hasOutputSize ? usage.output_width + "x" + usage.output_height : undefined;
  const fallbackSize = outputSize !== undefined ? outputSize : (ctx.state || {}).default_output_size;
  if (actual.count !== actual.images.length && fallbackSize === undefined) throw new Error("image count and dimension payloads disagree");
  let small = 0, large = 0;
  for (const image of actual.images) {
    const size = image.size !== undefined ? image.size : (image.width !== undefined && image.height !== undefined ? image.width + "x" + image.height : fallbackSize);
    const pixels = imageDimensions(size);
    if (actual.count !== actual.images.length && pixels !== imageDimensions(fallbackSize)) throw new Error("image count and dimension payloads disagree");
    if (pixels <= QWEN_LARGE_PIXEL_THRESHOLD) small++; else large++;
  }
  if (actual.count !== actual.images.length) {
    const isSmall = imageDimensions(fallbackSize) <= QWEN_LARGE_PIXEL_THRESHOLD;
    small = isSmall ? actual.count : 0;
    large = isSmall ? 0 : actual.count;
  }
  // Reference charges are per image actually sent, frozen by the submit parser.
  const input = body.input_image_count !== undefined ? body.input_image_count : (body.usage || {}).input_image_count !== undefined ? body.usage.input_image_count : (ctx.state || {}).input_image_count;
  if (!Number.isSafeInteger(input) || input < 0 || input > 3) throw new Error("authoritative input image count is missing");
  return { image_count: actual.count, small_image_count: small, large_image_count: large, input_image_count: input };
}

export const protocols = { openai_image: {
  decodeRequest(ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !ctx.body.value || typeof ctx.body.value !== "object") throw new Error("DFLOP Images requires a JSON request");
    const source = ctx.body.value;
    const model = ctx.upstreamModel || ctx.model;
    if (!MODELS.includes(model)) throw new Error("unsupported DFLOP image model");
    if (typeof source.prompt !== "string" || !source.prompt.trim()) throw new Error("prompt is required");
    const n = source.n === undefined ? 1 : source.n;
    if (!Number.isSafeInteger(n) || n < 1 || n > (model === "qwen-image-3.0-pro" ? 6 : 10)) throw new Error("invalid image count");
    if (source.size !== undefined) imageDimensions(source.size);
    let images = source.image === undefined ? [] : Array.isArray(source.image) ? source.image.slice() : [source.image];
    if (source.image_urls !== undefined) {
      if (!Array.isArray(source.image_urls)) throw new Error("image_urls must be an array");
      images = images.concat(source.image_urls);
    }
    if (images.length > 3 || images.some((image) => typeof image !== "string" || !image)) throw new Error("invalid input image references");
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
  render(_ctx, task) { return task.data; },
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
  const count = model === "qwen-image-3.0-pro" ? request.n : 4;
  if (!Number.isSafeInteger(count) || count < 1 || count > 10) throw new Error("invalid image reservation count");
  if (ctx.usagePurpose === "billing_ratios") return { image_count: count };
  if (model !== "qwen-image-3.0-pro") return { image_count: count };
  const small = request.size !== undefined && imageDimensions(request.size) <= QWEN_LARGE_PIXEL_THRESHOLD;
  return { image_count: count, small_image_count: small ? count : 0, large_image_count: small ? 0 : count, input_image_count: (request.image || []).length };
}
export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  const request = ctx.requestBody || {};
  const state = { input_image_count: (request.image || []).length };
  // DFLOP's exact SKU card guarantees 2048² when size is omitted. Freeze only
  // that contract; explicit request dimensions cannot supply terminal metering.
  if ((ctx.upstreamModel || ctx.model) === "qwen-image-3.0-pro" && request.size === undefined) state.default_output_size = "2048x2048";
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
  if (Array.isArray(body.data) && body.data.length === 0 && (body.unit_count === undefined || body.unit_count === 0)) return { status: "FAILURE", reason: "image generation returned zero outputs" };
  if (body.data !== undefined) {
    try { imageUsage(ctx, body); }
    catch (error) {
      // Successful delivery with missing metering is held for operator review.
      // It must never be refunded merely because the poller times out.
      return { status: "SUCCESS", reason: error.message, state: { ...ctx.state, billingPending: true, blocker: "MISSING_AUTHORITATIVE_IMAGE_USAGE" } };
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
