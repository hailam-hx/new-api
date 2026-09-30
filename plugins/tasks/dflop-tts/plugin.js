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
  upstreams: ["vendor"],
  protocols: [{ name: "openai_audio_speech", models: ["voice-tts-pro"] }],
  usageSchema: {
    characters: {
      type: "number",
      unit: "count",
      unitLabel: { en: "character", zh: "字符" },
      description: { en: "Speech synthesis unit price", zh: "语音合成单价" },
    },
  },
};

function speechRequest(ctx) {
  const source = ctx && ctx.body && ctx.body.value;
  if (!source || ctx.body.kind !== "json" || source.model !== "voice-tts-pro") throw new Error("voice-tts-pro JSON request required");
  if (source["async"] !== true) throw new Error("voice-tts-pro requires async=true; synchronous audio is not supported");
  if (typeof source.input !== "string" || !source.input || Array.from(source.input).length > 5000) throw new Error("input must contain 1 to 5000 characters");
  // The current host count unit is capped at 128. Keep the conservative
  // reservation within that contract before accepting the experimental path.
  const estimatedCharacters = /^[\x00-\x7F]*$/.test(source.input) ? source.input.length : source.input.length * 3;
  if (estimatedCharacters > 128) throw new Error("experimental speech input must reserve at most 128 billable characters (128 ASCII or 42 non-ASCII UTF-16 code units)");
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
      return { kind: "submit", model: "voice-tts-pro", action: "speech", requestBody: speechRequest(ctx) };
    },
  },
};

export function buildSubmitRequest(ctx) {
  const body = ctx.requestBody || {};
  if (ctx.upstreamModel !== "voice-tts-pro" || body.model !== "voice-tts-pro" || body["async"] !== true) throw new Error("invalid DFLOP speech model or mode");
  const key = (ctx.requestHeaders || {})["Idempotency-Key"];
  if (!key) throw new Error("missing idempotency key");
  return {
    url: ctx.baseUrl + "/v1/audio/speech",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
    body,
    action: "speech",
  };
}

export function parseSubmitResponse(_ctx, resp) {
  const body = resp.body || {};
  if (body.model !== "voice-tts-pro" || typeof body.id !== "string" || !body.id) throw new Error("invalid DFLOP speech acknowledgement");
  const result = { taskId: body.id, taskData: body };
  if (body.status === "succeeded" || body.status === "failed") result.immediate = parseTaskResult(null, body);
  return result;
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const input = (ctx.requestBody || {}).input;
  if (typeof input !== "string" || !input) throw new Error("missing speech input");
  // UTF-16 length * 3 bounds UTF-8 bytes (and therefore code points).
  // ASCII is exact; final settlement always uses upstream characters.
  return { characters: /^[\x00-\x7F]*$/.test(input) ? input.length : input.length * 3 };
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
  if (body.status === "pending") return { taskId: body.id, status: "IN_PROGRESS" };
  if (body.status === "failed") return { taskId: body.id, status: "FAILURE", reason: String((body.error || {}).message || "speech synthesis failed") };
  if (body.status === "succeeded") return { taskId: body.id, status: "SUCCESS", url: String(body.audio_url || "") };
  return { taskId: body.id, status: "UNKNOWN" };
}

export function extractUsageOnComplete(_task, result, body) {
  if (!result || result.status !== "SUCCESS" || !body || !Object.prototype.hasOwnProperty.call(body, "characters")) return null;
  if (typeof body.characters !== "number" || !Number.isSafeInteger(body.characters) || body.characters < 0) throw new Error("invalid authoritative characters");
  return { characters: body.characters };
}
