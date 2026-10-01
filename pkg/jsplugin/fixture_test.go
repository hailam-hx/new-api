package jsplugin

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixturePlugin = `
export const meta = { apiVersion: 1, key: "fixture", name: "Fixture", version: "1.0.0", author: {name: "Test"}, channelTypes: [1002], models: ["fixture-model"], fetchMode: "per_task", protocols: [{name: "openai_responses", supports: ["sync", "background"]}] };
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl + "/submit"}; }
export function parseSubmitResponse(ctx, resp) { return {taskId: resp.body.id}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl + "/task"}; }
export function parseTaskResult(ctx, body) { return body; }
export function stamp(value) { return {value: value, now: utils.unixNow()}; }
export function fail() { throw new Error("fixture failure"); }
export const native = { compact: function(value) { return {id: value.task_id}; } };
export const protocols = { openai_responses: {
  decodeRequest: function(value) { return {kind: "submit", model: value.model}; },
  renderFinal: function(ctx, task) { return task; }
} };
`

func TestReplayFixture(t *testing.T) {
	t.Parallel()
	report, err := ReplayFixture(context.Background(), fixturePlugin, []byte(`{
  "unixNow": 1700000000,
  "cases": [
    {"name":"deterministic time","hook":"stamp","args":["ok"],"expected":{"value":"ok","now":1700000000}},
    {"name":"member call","hook":"native","member":"compact","args":[{"task_id":"task-1"}],"expected":{"id":"task-1"}},
    {"name":"nested path call","hook":"protocols","path":["openai_responses","decodeRequest"],"args":[{"model":"video-1"}],"expected":{"kind":"submit","model":"video-1"}},
    {"name":"expected failure","hook":"fail","args":[],"expectedError":"fixture failure"}
  ]
}`))
	require.NoError(t, err)
	assert.Equal(t, FixtureReport{Total: 4, Passed: 4}, report)
}

func TestReplayFixtureReportsMismatch(t *testing.T) {
	t.Parallel()
	report, err := ReplayFixture(context.Background(), fixturePlugin, []byte(`{
  "cases": [{"name":"wrong output","hook":"stamp","args":["ok"],"expected":{"value":"different"}}]
}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong output: result mismatch")
	assert.Equal(t, FixtureReport{Total: 1}, report)
}

// These cases replay factory source through the same Sobek hooks as production.
func TestDFLOPMediaTerminalUsage(t *testing.T) {
	source, err := os.ReadFile("../../plugins/tasks/dflop-media/plugin.js")
	require.NoError(t, err)
	cases := []map[string]any{
		{"name": "delivered fraction", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "grok-imagine-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 4.25}}, "expected": map[string]any{"duration_sec": 4.25}},
		{"name": "missing delivered seconds", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "grok-imagine-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{}}, "expectedError": "MISSING_FINAL_DURATION"},
		{"name": "input plus output", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "wan3.0-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 4.25, "input_video_duration_sec": 2.5, "resolution": "720p"}}, "expected": map[string]any{"duration_sec": 4.25, "input_video_duration_sec": 2.5, "resolution": "720p"}},
		{"name": "input zero is present", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "wan3.0-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 4, "input_video_duration_sec": 0, "resolution": "720p"}}, "expected": map[string]any{"duration_sec": 4, "input_video_duration_sec": 0, "resolution": "720p"}},
		{"name": "missing input duration", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "wan3.0-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 4, "resolution": "720p"}}, "expectedError": "MISSING_INPUT_VIDEO_DURATION"},
		{"name": "seedance authoritative tokens", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0", "state": map[string]any{"input_mode": "default", "resolution": "720p"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"completion_tokens": 123}, "resolution": "720p"}}, "expected": map[string]any{"completion_tokens": 123, "input_mode": "default", "resolution": "720p"}},
		{"name": "unexpected served tier quarantines", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0", "state": map[string]any{"input_mode": "default", "resolution": "720p"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"completion_tokens": 123}, "resolution": "720p", "service_tier": "priority"}}, "expectedError": "MISSING_SELECTED_TIER"},
		{"name": "seedance missing tokens", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0", "state": map[string]any{"input_mode": "default", "resolution": "720p"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"resolution": "720p"}}, "expectedError": "MISSING_VIDEO_TOKEN_USAGE"},
		{"name": "seedance Lite seconds plus tokens", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0-lite", "state": map[string]any{"input_mode": "with_video_input", "resolution": "1080p"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"completion_tokens": 456}, "resolution": "1080p", "duration_sec": 5.5}}, "expected": map[string]any{"completion_tokens": 456, "input_mode": "with_video_input", "resolution": "1080p", "duration_sec": 5.5}},
		{"name": "two songs one generation", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "suno-v5"}, map[string]any{"status": "SUCCESS"}, map[string]any{"tracks": []any{map[string]any{"audio_url": "https://example.com/a"}, map[string]any{"audio_url": "https://example.com/b"}}}}, "expected": map[string]any{"generation_count": 1}},
		{"name": "fixed successful compose", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "clip-compose"}, map[string]any{"status": "SUCCESS"}, map[string]any{}}, "expected": map[string]any{"count": 1}},
		{"name": "failure has no usage", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "clip-compose"}, map[string]any{"status": "FAILURE"}, map[string]any{}}, "expected": nil},
		{"name": "missing tier", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "wan3.0-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 4, "input_video_duration_sec": 0}}, "expectedError": "MISSING_SELECTED_TIER"},
	}
	cases = append(cases,
		map[string]any{"name": "one song still one generation", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "suno-v5"}, map[string]any{"status": "SUCCESS"}, map[string]any{"tracks": []any{map[string]any{"audio_url": "https://example.com/a"}}}}, "expected": map[string]any{"generation_count": 1}},
		map[string]any{"name": "no successful song is missing", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "suno-v5"}, map[string]any{"status": "SUCCESS"}, map[string]any{"tracks": []any{}}}, "expectedError": "MISSING_SUCCESSFUL_SONG"},
		map[string]any{"name": "Lite requires delivered seconds", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0-lite", "state": map[string]any{"input_mode": "default", "resolution": "720p"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"completion_tokens": 1}, "resolution": "720p"}}, "expectedError": "MISSING_FINAL_DURATION"},
		map[string]any{"name": "unknown delivery tier", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0", "state": map[string]any{"input_mode": "default"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"completion_tokens": 1}, "resolution": "4k"}}, "expectedError": "MISSING_SELECTED_TIER"},
		map[string]any{"name": "missing frozen video selector", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "doubao-seedance-2.0"}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"completion_tokens": 1}, "resolution": "720p"}}, "expectedError": "MISSING_VIDEO_INPUT_SELECTOR"},
		map[string]any{"name": "provider success cannot become refund on missing facts", "hook": "parseTaskResult", "args": []any{map[string]any{"upstreamModel": "grok-imagine-video", "taskId": "video-1"}, map[string]any{"id": "video-1", "model": "grok-imagine-video", "status": "succeeded", "video_url": "https://example.com/a"}}, "expected": map[string]any{"taskId": "video-1", "status": "SUCCESS", "url": "https://example.com/a", "state": map[string]any{"billingPending": true, "blocker": "MISSING_FINAL_DURATION"}}},
		map[string]any{"name": "voice creation ready one count", "hook": "parseTaskResult", "args": []any{map[string]any{"upstreamModel": "voice-clone-pro", "taskId": "voice-1"}, map[string]any{"id": "voice-1", "status": "ready"}}, "expected": map[string]any{"taskId": "voice-1", "status": "SUCCESS", "url": "", "state": map[string]any{"billingPending": false}}},
		map[string]any{"name": "avatar creation ready one count", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "dh-avatar-create"}, map[string]any{"status": "SUCCESS"}, map[string]any{}}, "expected": map[string]any{"count": 1}},
		map[string]any{"name": "cancelled refund", "hook": "parseTaskResult", "args": []any{map[string]any{"upstreamModel": "clip-compose", "taskId": "fixed-1"}, map[string]any{"id": "fixed-1", "status": "cancelled", "error": map[string]any{"message": "cancelled"}}}, "expected": map[string]any{"taskId": "fixed-1", "status": "FAILURE", "reason": "cancelled"}},
		map[string]any{"name": "request reservation is separate", "hook": "extractUsage", "args": []any{map[string]any{"upstreamModel": "grok-imagine-video", "requestBody": map[string]any{"duration": 10}}}, "expected": map[string]any{"duration_sec": 10}},
		map[string]any{"name": "poll does not reserve", "hook": "buildQueryRequest", "args": []any{map[string]any{"upstreamModel": "suno-v5", "taskId": "task-1", "baseUrl": "https://api.dflop.top", "apiKey": "test"}}, "expected": map[string]any{"url": "https://api.dflop.top/v1/music/generations/task-1", "method": "GET", "headers": map[string]any{"Authorization": "Bearer test"}}},
		map[string]any{"name": "host gateway query prefix", "hook": "buildQueryRequest", "args": []any{map[string]any{"upstreamModel": "suno-v5", "taskId": "task-1", "baseUrl": "https://gateway.example.com", "apiKey": "test", "upstream": map[string]any{"kind": "new_api"}}}, "expected": map[string]any{"url": "https://gateway.example.com/dflop/v1/music/generations/task-1", "method": "GET", "headers": map[string]any{"Authorization": "Bearer test"}}},
		map[string]any{"name": "DFLOP type60 direct path", "hook": "buildQueryRequest", "args": []any{map[string]any{"upstreamModel": "suno-v5", "taskId": "task-1", "baseUrl": "https://api.dflop.top", "apiKey": "test", "upstream": map[string]any{"kind": "new_api"}}}, "expected": map[string]any{"url": "https://api.dflop.top/v1/music/generations/task-1", "method": "GET", "headers": map[string]any{"Authorization": "Bearer test"}}},
	)
	cases = append(cases,
		map[string]any{"name": "output files are accessible artifacts", "hook": "listArtifacts", "args": []any{map[string]any{"status": "SUCCESS", "data": map[string]any{"output_files": []any{map[string]any{"lang": "en", "kind": "subtitle", "url": "https://example.com/en.vtt"}}}}}, "expected": []any{map[string]any{"key": "file_0", "type": "file"}}},
		map[string]any{"name": "output artifact never sends channel credentials", "hook": "buildContentRequest", "args": []any{map[string]any{"artifactKey": "file_0", "data": map[string]any{"output_files": []any{map[string]any{"url": "https://example.com/en.vtt"}}}, "clientRequest": map[string]any{"method": "GET"}, "apiKey": "secret"}}, "expected": map[string]any{"url": "https://example.com/en.vtt", "method": "GET", "credentialless": true}},
	)
	fixture, err := common.Marshal(map[string]any{"cases": cases})
	require.NoError(t, err)
	report, err := ReplayFixture(t.Context(), string(source), fixture)
	require.NoError(t, err)
	assert.Equal(t, len(cases), report.Passed)
}
func TestDFLOPSpeechMissingCharacters(t *testing.T) {
	source, err := os.ReadFile("../../plugins/tasks/dflop-tts/plugin.js")
	require.NoError(t, err)
	report, err := ReplayFixture(t.Context(), string(source), []byte(`{"cases":[{"name":"missing terminal characters","hook":"extractUsageOnComplete","args":[{}, {"status":"SUCCESS"}, {}],"expectedError":"MISSING_CHARACTER_COUNT"}]}`))
	require.NoError(t, err)
	assert.Equal(t, 1, report.Passed)
}

func TestDFLOPMediaSubmitIdentityAndReplay(t *testing.T) {
	source, err := os.ReadFile("../../plugins/tasks/dflop-media/plugin.js")
	require.NoError(t, err)
	plugin, err := NewRegistry().RegisterFactory(string(source), Options{Key: "dflop-media"})
	require.NoError(t, err)
	ctx := map[string]any{"model": "suno-v5", "upstreamModel": "suno-v5", "baseUrl": "https://api.dflop.top", "apiKey": "test", "requestHeaders": map[string]any{"Idempotency-Key": "intent-1"}, "requestBody": map[string]any{"model": "suno-v5", "prompt": "A short song"}}
	first, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	second, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	request := first.(map[string]any)
	assert.Equal(t, "https://api.dflop.top/v1/music/generations", request["url"])
	assert.Equal(t, "POST", request["method"])
	assert.Equal(t, "intent-1", request["headers"].(map[string]any)["Idempotency-Key"])
	assert.Equal(t, map[string]any{"model": "suno-v5", "prompt": "A short song"}, request["body"])
	for _, model := range []string{"suno-v50", "unknown"} {
		ctx["upstreamModel"] = model
		_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
		assert.Error(t, err)
	}
}

func TestDFLOPSubtitleOperationsAndMissingSourceDuration(t *testing.T) {
	source, err := os.ReadFile("../../plugins/tasks/dflop-media/plugin.js")
	require.NoError(t, err)
	plugin, err := NewRegistry().RegisterFactory(string(source), Options{Key: "dflop-media"})
	require.NoError(t, err)
	for _, languages := range [][]string{{}, {"en"}, {"en", "ja", "vi"}} {
		body := map[string]any{"model": "tvod-subtitle-soft", "source_video_url": "https://example.com/source.mp4", "source_language": "zh", "target_languages": languages, "duration": 999}
		intent, err := plugin.Engine.CallMember(t.Context(), "native", "video", map[string]any{"body": map[string]any{"kind": "json", "value": body}})
		require.NoError(t, err)
		requestBody := intent.(map[string]any)["requestBody"]
		ctx := map[string]any{"model": "tvod-subtitle-soft", "upstreamModel": "tvod-subtitle-soft", "requestBody": requestBody, "baseUrl": "https://api.dflop.top", "requestHeaders": map[string]any{"Idempotency-Key": "intent"}}
		acknowledgement, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"body": map[string]any{"id": "subtitle-1", "model": "tvod-subtitle-soft", "status": "queued"}})
		require.NoError(t, err)
		state := acknowledgement.(map[string]any)["state"].(map[string]any)
		assert.EqualValues(t, 1, state["asr_units"])
		assert.EqualValues(t, len(languages), state["translation_units"])
		assert.EqualValues(t, 1+len(languages), state["processing_units"])
		_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
		require.ErrorContains(t, err, "MISSING_SUBTITLE_SOURCE_DURATION")
		query := map[string]any{"upstreamModel": "tvod-subtitle-soft", "taskId": "subtitle-1", "state": state, "baseUrl": "https://api.dflop.top", "apiKey": "test", "data": map[string]any{"duration_sec": 999}}
		polled, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", query)
		require.NoError(t, err)
		assert.Equal(t, "GET", polled.(map[string]any)["method"])
		parsed, err := plugin.Engine.Call(t.Context(), "parseTaskResult", query, map[string]any{"id": "subtitle-1", "model": "tvod-subtitle-soft", "status": "succeeded", "duration_sec": 999})
		require.NoError(t, err)
		parsedResult := parsed.(map[string]any)
		assert.Equal(t, "SUCCESS", parsedResult["status"])
		pending := parsedResult["state"].(map[string]any)
		assert.Equal(t, true, pending["billingPending"])
		assert.Equal(t, "MISSING_SUBTITLE_SOURCE_DURATION", pending["blocker"])
		assert.EqualValues(t, len(languages), pending["translation_units"])

		_, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", query, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 999, "unit_count": 999})
		require.ErrorContains(t, err, "MISSING_SUBTITLE_SOURCE_DURATION")
		usage, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", query, map[string]any{"status": "FAILURE"}, map[string]any{})
		require.NoError(t, err)
		assert.Nil(t, usage)
	}
	for _, invalid := range []map[string]any{{"target_languages": []string{"en", "en"}}, {"target_languages": []string{"EN"}}, {"source_language": "auto"}, {"burn_in": true}} {
		body := map[string]any{"model": "tvod-subtitle-soft", "source_video_url": "https://example.com/source.mp4", "source_language": "zh", "target_languages": []string{"en"}}
		for key, value := range invalid {
			body[key] = value
		}
		_, err = plugin.Engine.CallMember(t.Context(), "native", "video", map[string]any{"body": map[string]any{"kind": "json", "value": body}})
		assert.Error(t, err)
	}
}

func TestDFLOPReferenceVideoBillingBasis(t *testing.T) {
	source, err := os.ReadFile("../../plugins/tasks/dflop-media/plugin.js")
	require.NoError(t, err)
	fixture, err := common.Marshal(map[string]any{"cases": []any{
		map[string]any{"name": "ordinary reference keeps output-only basis", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "grok-imagine-video", "data": map[string]any{"reference_video": "https://example.com/ref.mp4"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 5, "input_video_duration_sec": 100}}, "expected": map[string]any{"duration_sec": 5}},
		map[string]any{"name": "Wan explicit input basis requires input seconds", "hook": "extractUsageOnComplete", "args": []any{map[string]any{"upstreamModel": "wan3.0-video"}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 5, "resolution": "720p"}}, "expectedError": "MISSING_INPUT_VIDEO_DURATION"},
		map[string]any{"name": "fresh H3 cannot use output-only assumption", "hook": "buildSubmitRequest", "args": []any{map[string]any{"upstreamModel": "minimax-h3"}}, "expectedError": "Unknown DFLOP media model"},
	}})
	require.NoError(t, err)
	report, err := ReplayFixture(t.Context(), string(source), fixture)
	require.NoError(t, err)
	assert.Equal(t, 3, report.Passed)
}
