// Async DFLOP speech binding. A channel must explicitly bind this factory
// plugin; matching a model name alone never selects it.
export const meta = {
  apiVersion: 1,
  key: "dflop-tts",
  name: "DFLOP Speech",
  version: "1.0.0",
  author: { name: "QuantumNous" },
  models: ["voice-tts-pro"],
  fetchMode: "per_task",
  upstreams: ["vendor", "new_api"],
  protocols: [{ name: "openai_audio_speech", models: ["voice-tts-pro"] }],
  usageSchema: {
    character_count: {
      type: "number", unit: "character",
      description: { en: "Speech synthesis unit price", zh: "语音合成单价" },
    },
    characters: {
      type: "number",
      unit: "character",
      description: { en: "Speech synthesis unit price", zh: "语音合成单价" },
    },
  },
};

function speechRequest(ctx) {
  const source = ctx.requestBody || (ctx.body && ctx.body.kind === "json" && ctx.body.value);
  const model = ctx.upstreamModel || ctx.model || (source || {}).model;
  if (!source || model !== "voice-tts-pro") throw new Error("voice-tts-pro JSON request required");
  if (source["async"] !== true) throw new Error("voice-tts-pro requires async=true; synchronous audio is not supported");
  if (typeof source.input !== "string" || !source.input || Array.from(source.input).length > 5000) throw new Error("input must contain 1 to 5000 characters");
  if (source.voice !== undefined && (typeof source.voice !== "string" || !source.voice.trim())) throw new Error("voice must be a non-empty string");
  if (source.speed !== undefined && (typeof source.speed !== "number" || !Number.isFinite(source.speed) || source.speed <= 0)) throw new Error("speed must be positive");
  const body = { model: "voice-tts-pro", input: source.input, "async": true };
  if (source.voice !== undefined) body.voice = source.voice;
  if (source.speed !== undefined) body.speed = source.speed;
  return body;
}

export const protocols = {
  openai_audio_speech: {
    decodeRequest(ctx) {
      return { kind: "submit", model: ctx.model || "voice-tts-pro", action: "speech", requestBody: speechRequest(ctx) };
    },
  },
};

export function buildSubmitRequest(ctx) {
  const body = speechRequest(ctx);
  const key = (ctx.requestHeaders || {})["Idempotency-Key"] || (ctx.requestHeaders || {})["idempotency-key"];
  if (typeof key !== "string" || !/^[\x20-\x7e]{1,200}$/.test(key)) throw new Error("missing or invalid idempotency key");
  return {
    url: ctx.baseUrl + "/v1/audio/speech",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
    body,
    action: "speech",
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  if (body.model !== "voice-tts-pro" || typeof body.id !== "string" || !body.id || !["pending", "succeeded", "failed"].includes(body.status)) throw new Error("invalid DFLOP speech acknowledgement");
  const reservedCharacters = extractUsage(ctx).characters;
  const state = {reservedCharacters, upstreamModel: "voice-tts-pro", billingPending: false};
  const result = { taskId: body.id, taskData: body, state };
  if (body.status === "succeeded" || body.status === "failed") {
    result.immediate = parseTaskResult(Object.assign({}, ctx, {taskId: body.id, state}), body);
    if (result.immediate.state) result.state = result.immediate.state;
  }
  return result;
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const input = speechRequest(ctx).input;
  const characters = Array.from(input).length;
  return { characters, character_count: characters };
}

export function buildQueryRequest(ctx) {
  if (!ctx.taskId) throw new Error("missing upstream speech task id");
  return {
    url: ctx.baseUrl + "/v1/audio/speech/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey },
  };
}

export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || body.model !== "voice-tts-pro" || !body.id || (ctx && body.id !== ctx.taskId)) throw new Error("invalid DFLOP speech task response");
  if (body.status === "pending") return { taskId: body.id, status: "IN_PROGRESS", state: Object.assign({}, (ctx || {}).state || {}) };
  if (body.status === "failed") return { taskId: body.id, status: "FAILURE", reason: String((body.error || {}).message || "speech synthesis failed") };
  if (body.status === "succeeded") {
    if (typeof body.audio_url !== "string" || !body.audio_url.trim()) throw new Error("successful speech task is missing audio_url");
    const state = Object.assign({}, (ctx || {}).state || {}, {billingPending: false});
    delete state.blocker;
    try { extractUsageOnComplete(ctx, {status: "SUCCESS"}, body); }
    catch (error) { state.billingPending = true; state.blocker = String(error.message); }
    return { taskId: body.id, status: "SUCCESS", url: String(body.audio_url || ""), state };
  }
  return { taskId: body.id, status: "UNKNOWN" };
}

export function extractUsageOnComplete(_task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  if (!body || !Object.prototype.hasOwnProperty.call(body, "characters")) throw new Error("MISSING_CHARACTER_COUNT");
  if (typeof body.characters !== "number" || !Number.isSafeInteger(body.characters) || body.characters < 0 || body.characters > 5000) throw new Error("invalid authoritative characters");
  if (typeof body.audio_url !== "string" || !body.audio_url.trim()) throw new Error("successful speech task is missing audio_url");
  return { characters: body.characters, character_count: body.characters };
}
