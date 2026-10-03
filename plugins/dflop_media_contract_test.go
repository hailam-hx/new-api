package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDFLOPVideoPublicRequestNormalization(t *testing.T) {
	source, err := builtinplugins.Source("dflop-media")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-media"})
	require.NoError(t, err)
	text := map[string]any{"type": "text", "text": "test"}
	image := map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://example.invalid/image.png"}, "portrait_auth": true}
	video := map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://example.invalid/video.mp4"}}
	audio := map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://example.invalid/audio.wav"}}
	for _, tc := range []struct {
		name, model string
		fields      map[string]any
		content     []any
	}{
		{"prompt seconds", "doubao-seedance-2.0", map[string]any{"prompt": "test", "seconds": 4}, []any{text}},
		{"duration compatibility", "doubao-seedance-2.0", map[string]any{"prompt": "test", "duration": 4}, []any{text}},
		{"content only", "doubao-seedance-2.0", map[string]any{"content": []any{text}, "duration": 4}, []any{text}},
		{"same text", "doubao-seedance-2.0", map[string]any{"prompt": "test", "content": []any{text}, "duration": 4}, []any{text}},
		{"different text", "doubao-seedance-2.0", map[string]any{"prompt": "test", "content": []any{map[string]any{"type": "text", "text": "other"}}, "duration": 4}, []any{map[string]any{"type": "text", "text": "other"}, text}},
		{"image metadata", "doubao-seedance-2.0", map[string]any{"prompt": "test", "content": []any{image}, "seconds": 4}, []any{image, text}},
		{"video audio metadata", "doubao-seedance-2.0", map[string]any{"prompt": "test", "content": []any{video, audio}, "seconds": 4}, []any{video, audio, text}},
		{"equal duration fields", "doubao-seedance-2.0", map[string]any{"prompt": "test", "seconds": 4, "duration": 4}, []any{text}},
		{"Seedance family", "doubao-seedance-2.5", map[string]any{"prompt": "test", "seconds": 4}, []any{text}},
		{"Wan family", "wan3.0-video", map[string]any{"prompt": "test", "seconds": 4}, []any{text}},
		{"empty prompt", "doubao-seedance-2.0", map[string]any{"prompt": "  ", "content": []any{text}, "duration": 4}, []any{text}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"model": tc.model, "resolution": "720p", "ratio": "16:9", "seed": 0}
			minimal := tc.name == "prompt seconds" || tc.name == "duration compatibility"
			if minimal {
				request = map[string]any{"model": tc.model}
			}
			for key, value := range tc.fields {
				request[key] = value
			}
			if tc.name == "image metadata" {
				request["audio"] = false
				request["reference_image"] = "reference-extension"
				request["vendor_options"] = map[string]any{"portrait_auth": true}
			}
			before, err := common.Marshal(request)
			require.NoError(t, err)
			ctx := map[string]any{"model": tc.model, "operation": "create", "body": map[string]any{"kind": "json", "value": request}}
			intent, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, ctx)
			require.NoError(t, err)
			normalized := intent.(map[string]any)["requestBody"]
			driver := map[string]any{"model": tc.model, "requestBody": normalized, "baseUrl": "https://api.dflop.top", "apiKey": "fixture", "requestHeaders": map[string]any{"Idempotency-Key": "stable-host-intent"}}
			for range 2 {
				submit, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", driver)
				require.NoError(t, err)
				wire := submit.(map[string]any)
				body := wire["body"].(map[string]any)
				actual, err := common.Marshal(body["content"])
				require.NoError(t, err)
				expected, err := common.Marshal(tc.content)
				require.NoError(t, err)
				assert.JSONEq(t, string(expected), string(actual))
				assert.EqualValues(t, 4, body["duration"])
				assert.NotContains(t, body, "seconds")
				assert.Equal(t, "720p", body["resolution"])
				if !minimal {
					assert.Equal(t, "16:9", body["ratio"])
					assert.EqualValues(t, 0, body["seed"])
				}
				if tc.name == "image metadata" {
					assert.Equal(t, false, body["audio"])
					assert.Equal(t, "reference-extension", body["reference_image"])
					assert.Equal(t, map[string]any{"portrait_auth": true}, body["vendor_options"])
				}
				assert.Equal(t, "https://api.dflop.top/v1/videos/generations", wire["url"])
				assert.Equal(t, "stable-host-intent", wire["headers"].(map[string]any)["Idempotency-Key"])
			}
			reservation, err := plugin.Engine.Call(t.Context(), "extractUsage", driver)
			require.NoError(t, err)
			if tc.model == "wan3.0-video" {
				assert.EqualValues(t, 4, reservation.(map[string]any)["duration_sec"])
			} else {
				assert.Greater(t, reservation.(map[string]any)["completion_tokens"].(int64), int64(0))
			}
			after, err := common.Marshal(request)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after), "decoder must not mutate caller-owned content")
			ack, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", driver, map[string]any{"body": map[string]any{"id": "provider-task", "model": tc.model, "status": "queued", "progress": 0, "created_at": 123}})
			require.NoError(t, err)
			assert.Equal(t, "provider-task", ack.(map[string]any)["taskId"])
			assert.NotContains(t, ack.(map[string]any), "immediate")
			driver["state"] = ack.(map[string]any)["state"]
			driver["taskId"] = "provider-task"
			query, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", driver)
			require.NoError(t, err)
			assert.Equal(t, "https://api.dflop.top/v1/videos/generations/provider-task", query.(map[string]any)["url"])
			terminal := map[string]any{"id": "provider-task", "model": tc.model, "status": "succeeded", "resolution": "720p", "duration_sec": 4, "usage": map[string]any{"completion_tokens": 1000}, "video_url": "https://example.invalid/output.mp4"}
			result, err := plugin.Engine.Call(t.Context(), "parseTaskResult", driver, terminal)
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", result.(map[string]any)["status"])
			facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", driver, result, terminal)
			require.NoError(t, err)
			if tc.model == "wan3.0-video" {
				assert.EqualValues(t, 4, facts.(map[string]any)["duration_sec"])
			} else {
				assert.EqualValues(t, 1000, facts.(map[string]any)["completion_tokens"])
			}
		})
	}
	for name, fields := range map[string]map[string]any{"zero seconds": {"seconds": 0}, "zero duration": {"duration": 0}, "negative duration": {"duration": -1}, "malformed seconds": {"seconds": "bad"}, "null duration": {"duration": nil}, "conflicting durations": {"seconds": 4, "duration": 8}, "malformed content": {"seconds": 4, "content": "bad"}, "oversized duration": {"duration": 3601}} {
		t.Run(name, func(t *testing.T) {
			request := map[string]any{"model": "doubao-seedance-2.0", "prompt": "test"}
			for key, value := range fields {
				request[key] = value
			}
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"body": map[string]any{"kind": "json", "value": request}})
			require.Error(t, err)
		})
	}
}

func TestDFLOPPartialMediaFrozenSettlement(t *testing.T) {
	source, err := builtinplugins.Source("dflop-media")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-media"})
	require.NoError(t, err)
	for _, model := range []string{"clip-compose", "dh-avatar", "dh-lipsync", "dh-lipsync-pro", "dh-lipsync-max", "dh-motion", "dh-avatar-create"} {
		t.Run(model, func(t *testing.T) {
			request := map[string]any{"model": model, "duration": 10, "resolution": "standard", "source_video_url": "https://example.com/source.mp4", "audio_url": "https://example.com/audio.mp3", "avatar": "avatar-1", "video_url": "https://example.com/source.mp4", "asr_id": "fresh-same-source-asr", "name": "avatar", "source_url": "https://example.com/portrait.png", "face_count": 1, "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/face.png"}}}}
			ctx := map[string]any{"model": model, "upstreamModel": model, "requestBody": request, "baseUrl": "https://api.dflop.top", "apiKey": "fixture", "requestHeaders": map[string]any{"Idempotency-Key": "fixture-intent"}}
			endpoint := "/v1/videos/generations"
			if model == "dh-avatar-create" {
				endpoint = "/v1/videos/avatars"
			}
			submit, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			assert.Equal(t, "https://api.dflop.top"+endpoint, submit.(map[string]any)["url"])
			assert.Equal(t, true, submit.(map[string]any)["body"].(map[string]any)["async"])
			reservation, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			fixed := model == "clip-compose" || model == "dh-avatar-create"
			if model == "clip-compose" {
				assert.NotContains(t, submit.(map[string]any)["body"].(map[string]any), "duration")
			}
			if fixed {
				assert.Equal(t, map[string]any{"count": int64(1)}, reservation)
			} else {
				assert.Equal(t, int64(10), reservation.(map[string]any)["duration_sec"])
			}
			ack, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"body": map[string]any{"id": "task-1", "status": "pending"}})
			require.NoError(t, err)
			ctx["state"] = ack.(map[string]any)["state"]
			ctx["taskId"] = "task-1"
			poll, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
			require.NoError(t, err)
			assert.Equal(t, "GET", poll.(map[string]any)["method"])
			assert.Equal(t, "https://api.dflop.top"+endpoint+"/task-1", poll.(map[string]any)["url"])
			terminal := map[string]any{"id": "task-1", "status": "succeeded", "duration_sec": 4.5, "source_duration_sec": 7.25, "resolution": "standard", "video_url": "https://example.com/output.mp4", "avatar_id": "avatar-1"}
			result, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, terminal)
			require.NoError(t, err)
			assert.Equal(t, false, result.(map[string]any)["state"].(map[string]any)["billingPending"])
			usage, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, result, terminal)
			require.NoError(t, err)
			if fixed {
				assert.Equal(t, map[string]any{"count": int64(1)}, usage)
			} else if model == "dh-motion" {
				assert.Equal(t, 7.25, usage.(map[string]any)["duration_sec"])
			} else {
				assert.Equal(t, 4.5, usage.(map[string]any)["duration_sec"])
			}
			if model == "dh-motion" {
				for _, tier := range []string{"fast", "standard", "max"} {
					ctx["state"].(map[string]any)["resolution"] = tier
					terminal["resolution"] = tier
					facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, terminal)
					require.NoError(t, err)
					assert.Equal(t, tier, facts.(map[string]any)["resolution"])
					assert.Equal(t, 7.25, facts.(map[string]any)["duration_sec"])
				}
				terminal["resolution"] = "fast"
				_, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, terminal)
				require.ErrorContains(t, err, "SELECTED_TIER_CHANGED")
				terminal["resolution"] = "max"
				delete(terminal, "source_duration_sec")
				_, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, terminal)
				require.ErrorContains(t, err, "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION")
			}
			if !fixed {
				delete(terminal, "duration_sec")
				delete(terminal, "source_duration_sec")
				result, err = plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, terminal)
				require.NoError(t, err)
				assert.Equal(t, "SUCCESS", result.(map[string]any)["status"])
				assert.Equal(t, true, result.(map[string]any)["state"].(map[string]any)["billingPending"])
				_, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, result, terminal)
				require.Error(t, err)
			}
			failure, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, map[string]any{"id": "task-1", "status": "failed"})
			require.NoError(t, err)
			assert.Equal(t, "FAILURE", failure.(map[string]any)["status"])
			usage, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, failure, terminal)
			require.NoError(t, err)
			assert.Nil(t, usage)
		})
	}
}
