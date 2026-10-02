package plugins_test

import (
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

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
