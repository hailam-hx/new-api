package service

import (
	"encoding/base64"
	"maps"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDFLOPVerificationFixtureRegistryCoversEveryTaskTarget(t *testing.T) {
	data, err := os.ReadFile("testdata/dflop-verification/catalog-v1.json")
	require.NoError(t, err)
	var catalog []dflop.Item
	require.NoError(t, common.Unmarshal(data, &catalog))
	fixtures := DFLOPVerificationFixtures(catalog)
	require.Len(t, fixtures, 83)
	seen := make(map[string]bool)
	for _, fixture := range fixtures {
		assert.False(t, seen[fixture.Model], fixture.Model)
		seen[fixture.Model] = true
		assert.NotEmpty(t, fixture.Plan.ID, fixture.Model)
		assert.NotEmpty(t, fixture.Plan.RequiredUsageFacts, fixture.Model)
		assert.NotEmpty(t, fixture.Endpoint, fixture.Model)
		assert.NotEmpty(t, fixture.CatalogRowHash, fixture.Model)
		assert.LessOrEqual(t, len(fixture.Mode), 16, fixture.Model)
	}
	assert.False(t, seen["doubao-seedream-5-0-pro-260628"])
	assert.False(t, seen["qwen-image-3.0-pro"])
	assert.False(t, seen["tvod-subtitle-soft"])
	assert.False(t, seen["clip-mixcut"])
	assert.False(t, seen["clip-news"])
	assert.False(t, seen["clip-realman"])
}

func TestDFLOPVerificationCapabilitiesSetProviderMinimumAndRejectChangedBounds(t *testing.T) {
	fixtures := map[string]VerificationFixture{}
	for _, fixture := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		fixtures[fixture.Model] = fixture
	}
	for _, test := range []struct {
		model      string
		duration   float64
		resolution string
	}{
		{"tvod-sora-2", 8, "720p"}, {"doubao-seedance-2.0", 4, "480p"}, {"wan2.7-t2v", 2, "720p"}, {"minimax-h3-max", 5, "480p"},
	} {
		t.Run(test.model, func(t *testing.T) {
			fixture := fixtures[test.model]
			assert.Equal(t, test.duration, fixture.Request["duration"])
			assert.Equal(t, test.resolution, fixture.Request["resolution"])
			plugin := verificationFixturePlugin(t, fixture.Plugin)
			fixture.Request["duration"] = test.duration - 1
			_, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, plugin)
			require.ErrorContains(t, err, "FIXTURE_QUANTITY_OUT_OF_BOUNDS")
		})
	}
	qwen := fixtures["qwen-image-3.0"]
	qwen.Request["n"] = 10
	_, err := ValidateDFLOPVerificationFixture(t.Context(), qwen, verificationFixturePlugin(t, qwen.Plugin))
	require.ErrorContains(t, err, "FIXTURE_QUANTITY_OUT_OF_BOUNDS")
}

func TestDFLOPVerificationReferenceVideoAndMotionDoNotSubstituteOutputSeconds(t *testing.T) {
	fixtures := map[string]VerificationFixture{}
	for _, fixture := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		fixtures[fixture.Model] = fixture
	}
	for _, test := range []struct {
		model string
		body  string
		state string
		want  map[string]any
	}{
		{"wan3.0-video", `{"id":"task-1","model":"wan3.0-video","status":"succeeded","video_url":"https://fixtures.invalid/video.mp4","duration_sec":2,"input_video_duration_sec":3,"resolution":"480p"}`, `{"requested_duration_sec":2,"resolution":"480p","reference_mode":"video"}`, map[string]any{"duration_sec": float64(2), "input_video_duration_sec": float64(3), "resolution": "480p"}},
		{"dh-motion", `{"id":"task-1","model":"dh-motion","status":"succeeded","video_url":"https://fixtures.invalid/video.mp4","duration_sec":2,"source_duration_sec":3,"resolution":"standard"}`, `{"billing_contract":"DFLOP_PARTIAL_CLOSURE_V1","resolution":"standard"}`, map[string]any{"duration_sec": float64(3), "resolution": "standard"}},
	} {
		t.Run(test.model, func(t *testing.T) {
			fixture := fixtures[test.model]
			plugin := verificationFixturePlugin(t, fixture.Plugin)
			replay, err := ReplayDFLOPVerificationTaskTerminal(t.Context(), fixture, plugin, "task-1", []byte(`{}`), []byte(test.state), []byte(test.body))
			require.NoError(t, err)
			assert.Equal(t, test.want, replay.NormalizedUsage)
			var body map[string]any
			require.NoError(t, common.Unmarshal([]byte(test.body), &body))
			if test.model == "wan3.0-video" {
				delete(body, "input_video_duration_sec")
			} else {
				delete(body, "source_duration_sec")
			}
			incomplete, err := common.Marshal(body)
			require.NoError(t, err)
			replay, err = ReplayDFLOPVerificationTaskTerminal(t.Context(), fixture, plugin, "task-1", []byte(`{}`), []byte(test.state), incomplete)
			require.Error(t, err)
			assert.Equal(t, "BILLING_QUANTITY_MISSING", replay.ReasonCode)
			assert.Empty(t, replay.NormalizedUsage)
		})
	}
}

func TestDFLOPVerificationReplayRequiresExactIdentityAndPreservesFailure(t *testing.T) {
	var fixture VerificationFixture
	for _, candidate := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		if candidate.Model == "voice-tts-pro" {
			fixture = candidate
		}
	}
	plugin := verificationFixturePlugin(t, fixture.Plugin)
	for _, test := range []struct{ name, body, status, reason string }{
		{"wrong exact id", `{"id":"wrong","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://fixtures.invalid/voice.mp3"}`, "", "PROVIDER_TASK_ID_MISMATCH"},
		{"terminal failure", `{"id":"task-1","model":"voice-tts-pro","status":"failed","error":{"message":"provider rejected input"}}`, "FAILURE", ""},
		{"malformed output location", `{"id":"task-1","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"not-a-media-url"}`, "", "GENERATION_INVALID_OUTPUT"},
		{"explicit zero is present", `{"id":"task-1","model":"voice-tts-pro","status":"succeeded","characters":0,"audio_url":"https://fixtures.invalid/voice.mp3"}`, "SUCCESS", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			replay, err := ReplayDFLOPVerificationTaskTerminal(t.Context(), fixture, plugin, "task-1", []byte(`{}`), []byte(`{}`), []byte(test.body))
			if test.reason != "" {
				require.Error(t, err)
				assert.Equal(t, test.reason, replay.ReasonCode)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.status, replay.Status)
			if test.status == "FAILURE" {
				assert.Empty(t, replay.NormalizedUsage)
				assert.False(t, replay.OutputValid)
			} else {
				assert.Equal(t, float64(0), replay.NormalizedUsage["character_count"])
			}
		})
	}
}

func TestDFLOPVerificationRejectsBindingWhoseUsageMetadataChanged(t *testing.T) {
	var fixture VerificationFixture
	for _, candidate := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		if candidate.Model == "voice-tts-pro" {
			fixture = candidate
		}
	}
	plugin := *verificationFixturePlugin(t, fixture.Plugin)
	plugin.Meta.UsageSchema = map[string]jsplugin.UsageFieldSchema{}
	plugin.Meta.UsageProfiles = nil
	_, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, &plugin)
	require.ErrorContains(t, err, "PRODUCTION_USAGE_SCHEMA_MISMATCH")
}

func TestDFLOPVerificationProviderFactsPreserveLiteralPrecisionAndRemovePayloads(t *testing.T) {
	facts, err := DFLOPVerificationProviderFacts([]byte(`{"model":"test","status":"succeeded","unit_count":12345.123456789012,"duration_sec":3.123456789012,"video_url":"https://secret.invalid/result?token=usable","usage":{"completion_tokens":123,"unit_count":12345.123456789012,"api_key":"secret","prompt":"private"},"api_key":"secret"}`))
	require.NoError(t, err)
	assert.Equal(t, "12345.123456789012", facts["unit_count"])
	assert.Equal(t, "3.123456789012", facts["duration_sec"])
	assert.Equal(t, map[string]any{"completion_tokens": "123", "unit_count": "12345.123456789012"}, facts["usage"])
	assert.NotContains(t, facts, "video_url")
	assert.NotContains(t, facts, "api_key")
}

func TestDFLOPVerificationImageSubmissionSeparatesParserIdentityFromProviderTask(t *testing.T) {
	var fixture VerificationFixture
	for _, candidate := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		if candidate.Model == "qwen-image-3.0" {
			fixture = candidate
		}
	}
	plugin := verificationFixturePlugin(t, fixture.Plugin)
	request, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, plugin)
	require.NoError(t, err)
	for _, test := range []struct{ name, response, exactID, parserID, immediate string }{
		{"synchronous generated internal id", `{"model":"qwen-image-3.0","data":[{"url":"https://fixtures.invalid/image.png"}]}`, "", "", "SUCCESS"},
		{"synchronous request id is not task id", `{"model":"qwen-image-3.0","request_id":"exact-request","data":[{"url":"https://fixtures.invalid/image.png"}]}`, "", "exact-request", "SUCCESS"},
		{"asynchronous exact provider id", `{"id":"exact-provider-task","model":"qwen-image-3.0","status":"queued"}`, "exact-provider-task", "exact-provider-task", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			submission, err := ParseDFLOPVerificationSubmission(t.Context(), fixture, plugin, request.RequestBody, []byte(test.response))
			require.NoError(t, err)
			assert.NotEmpty(t, submission.TaskID)
			assert.Equal(t, test.exactID, submission.ExactTaskID)
			assert.Equal(t, test.immediate, submission.ImmediateStatus)
			if test.parserID != "" {
				assert.Equal(t, test.parserID, submission.TaskID)
			}
			var state map[string]any
			require.NoError(t, common.Unmarshal(submission.PluginState, &state))
			assert.Equal(t, submission.TaskID, state["_verification_task_id"])
			if test.immediate != "" {
				replay, err := ReplayDFLOPVerificationTerminal(t.Context(), fixture, plugin, request.RequestBody, submission.PluginState, []byte(test.response))
				require.NoError(t, err)
				assert.Equal(t, "SUCCESS", replay.Status)
				assert.True(t, replay.OutputValid)
				assert.Equal(t, map[string]any{"image_count": float64(1), "input_image_count": float64(0)}, replay.NormalizedUsage)
			}
		})
	}
}

func verificationFixtureCatalog(t *testing.T) []dflop.Item {
	t.Helper()
	data, err := os.ReadFile("testdata/dflop-verification/catalog-v1.json")
	require.NoError(t, err)
	var catalog []dflop.Item
	require.NoError(t, common.Unmarshal(data, &catalog))
	return catalog
}

func verificationFixturePlugin(t *testing.T, key string) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := os.ReadFile("../plugins/tasks/" + key + "/plugin.js")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(string(source), jsplugin.Options{Key: key})
	require.NoError(t, err)
	return plugin
}

func TestDFLOPVerificationMediaValidatesImmutableBoundedAssets(t *testing.T) {
	for _, id := range []string{"reference-grid-v1", "synthetic-speech-v1", "moving-square-v1"} {
		t.Run(id, func(t *testing.T) {
			media := dflopVerificationMedia(id)
			require.NoError(t, ValidateDFLOPVerificationMedia(media))
			media.SHA256 = "wrong"
			require.ErrorContains(t, ValidateDFLOPVerificationMedia(media), "FIXTURE_CHECKSUM_MISMATCH")
		})
	}
}

func TestDFLOPVerificationEveryRequestUsesProductionBindingOrExplicitFixtureBlocker(t *testing.T) {
	plugins := map[string]*jsplugin.LoadedPlugin{}
	for _, key := range []string{"dflop-image", "dflop-media", "dflop-tts"} {
		plugins[key] = verificationFixturePlugin(t, key)
	}
	for _, fixture := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		t.Run(fixture.Model, func(t *testing.T) {
			request, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, plugins[fixture.Plugin])
			if fixture.BlockedReason != "" {
				require.ErrorContains(t, err, fixture.BlockedReason)
				assert.Empty(t, request.Body)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "POST", request.Method)
			assert.Equal(t, fixture.Endpoint, request.URLPath)
			var body map[string]any
			require.NoError(t, common.Unmarshal(request.Body, &body))
			assert.Equal(t, fixture.Model, body["model"])
			assert.NotEmpty(t, request.ReservationFacts)
			repeated, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, plugins[fixture.Plugin])
			require.NoError(t, err)
			assert.Equal(t, request.Body, repeated.Body)
			assert.Equal(t, request.BodyHash, repeated.BodyHash)
			ackStatus := "queued"
			if fixture.Plan.RequiredOutput == "music" {
				ackStatus = "processing"
			}
			if fixture.Plan.RequiredOutput == "audio" {
				ackStatus = "pending"
			}
			ack, err := common.Marshal(map[string]any{"id": "task-verification-1", "model": fixture.Model, "status": ackStatus})
			require.NoError(t, err)
			submission, err := ParseDFLOPVerificationSubmission(t.Context(), fixture, plugins[fixture.Plugin], request.RequestBody, ack)
			require.NoError(t, err)
			poll, err := BuildDFLOPVerificationPoll(t.Context(), fixture, plugins[fixture.Plugin], submission.TaskID, submission.PluginState)
			require.NoError(t, err)
			assert.Equal(t, "GET", poll.Method)
			assert.Equal(t, fixture.Endpoint+"/task-verification-1", poll.URLPath)
		})
	}
}

func TestDFLOPVerificationProductionReplayUsesAuthoritativeTerminalQuantities(t *testing.T) {
	fixtures := map[string]VerificationFixture{}
	for _, fixture := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		fixtures[fixture.Model] = fixture
	}
	tests := []struct {
		name               string
		model              string
		terminal           map[string]any
		want               map[string]any
		missingField       string
		missingOutputField string
	}{
		{name: "Suno two tracks remain one generation", model: "suno-v5", terminal: map[string]any{"tracks": []any{map[string]any{"audio_url": "https://fixtures.invalid/one.mp3"}, map[string]any{"audio_url": "https://fixtures.invalid/two.mp3"}}}, want: map[string]any{"generation_count": float64(1)}, missingOutputField: "tracks"},
		{name: "Seedance Lite delivered seconds differ from reference seconds", model: "doubao-seedance-2.0-lite", terminal: map[string]any{"video_url": "https://fixtures.invalid/out.mp4", "duration_sec": float64(3), "input_video_duration_sec": float64(14), "resolution": "720p", "usage": map[string]any{"completion_tokens": float64(123)}}, want: map[string]any{"duration_sec": float64(3), "completion_tokens": float64(123), "resolution": "720p", "input_mode": "default"}, missingField: "duration_sec", missingOutputField: "video_url"},
		{name: "Midjourney grid URL does not define generation quantity", model: "tvod-midjourney-v7", terminal: map[string]any{"data": []any{map[string]any{"url": "https://fixtures.invalid/grid.png"}}, "unit_count": float64(4)}, want: map[string]any{"image_count": float64(4)}, missingField: "unit_count", missingOutputField: "data"},
		{name: "TTS terminal characters replace six-character reservation", model: "voice-tts-pro", terminal: map[string]any{"audio_url": "https://fixtures.invalid/speech.mp3", "characters": float64(4)}, want: map[string]any{"characters": float64(4), "character_count": float64(4)}, missingField: "characters", missingOutputField: "audio_url"},
		{name: "WAN no reference selector proves zero input duration", model: "wan3.0-video", terminal: map[string]any{"video_url": "https://fixtures.invalid/out.mp4", "duration_sec": float64(2), "resolution": "480p"}, want: map[string]any{"duration_sec": float64(2), "input_video_duration_sec": float64(0), "resolution": "480p"}, missingField: "duration_sec", missingOutputField: "video_url"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := fixtures[test.model]
			plugin := verificationFixturePlugin(t, fixture.Plugin)
			request, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, plugin)
			require.NoError(t, err)
			status := "queued"
			if fixture.Plan.RequiredOutput == "audio" {
				status = "pending"
			}
			if fixture.Plan.RequiredOutput == "music" {
				status = "processing"
			}
			ack, err := common.Marshal(map[string]any{"id": "exact-task-1", "model": fixture.Model, "status": status})
			require.NoError(t, err)
			submission, err := ParseDFLOPVerificationSubmission(t.Context(), fixture, plugin, request.RequestBody, ack)
			require.NoError(t, err)
			terminal := maps.Clone(test.terminal)
			terminal["id"] = "exact-task-1"
			terminal["model"] = fixture.Model
			terminal["status"] = "succeeded"
			data, err := common.Marshal(terminal)
			require.NoError(t, err)
			replay, err := ReplayDFLOPVerificationTerminal(t.Context(), fixture, plugin, request.RequestBody, submission.PluginState, data)
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", replay.Status)
			assert.True(t, replay.OutputValid)
			assert.Equal(t, test.want, replay.NormalizedUsage)
			if test.missingField != "" {
				incomplete := maps.Clone(terminal)
				delete(incomplete, test.missingField)
				data, err = common.Marshal(incomplete)
				require.NoError(t, err)
				replay, err = ReplayDFLOPVerificationTerminal(t.Context(), fixture, plugin, request.RequestBody, submission.PluginState, data)
				require.Error(t, err)
				assert.Equal(t, "BILLING_QUANTITY_MISSING", replay.ReasonCode)
				assert.Empty(t, replay.NormalizedUsage)
			}
			invalid := maps.Clone(terminal)
			delete(invalid, test.missingOutputField)
			data, err = common.Marshal(invalid)
			require.NoError(t, err)
			replay, err = ReplayDFLOPVerificationTerminal(t.Context(), fixture, plugin, request.RequestBody, submission.PluginState, data)
			require.Error(t, err)
			assert.Equal(t, "GENERATION_INVALID_OUTPUT", replay.ReasonCode)
			assert.False(t, replay.OutputValid)
		})
	}
}

func TestDFLOPVerificationReferenceCountUsesActualSubmittedImage(t *testing.T) {
	var fixture VerificationFixture
	for _, candidate := range DFLOPVerificationFixtures(verificationFixtureCatalog(t)) {
		if candidate.Model == "qwen-image-3.0" {
			fixture = candidate
		}
	}
	image, err := dflopVerificationFiles.ReadFile("testdata/dflop-verification/reference-grid-v1.png")
	require.NoError(t, err)
	fixture.Request["image"] = []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(image)}
	plugin := verificationFixturePlugin(t, fixture.Plugin)
	request, err := ValidateDFLOPVerificationFixture(t.Context(), fixture, plugin)
	require.NoError(t, err)
	assert.Equal(t, float64(1), request.ReservationFacts["input_image_count"])
	ack, err := common.Marshal(map[string]any{"id": "qwen-task", "model": fixture.Model, "status": "queued"})
	require.NoError(t, err)
	submission, err := ParseDFLOPVerificationSubmission(t.Context(), fixture, plugin, request.RequestBody, ack)
	require.NoError(t, err)
	terminal, err := common.Marshal(map[string]any{"data": []any{map[string]any{"url": "https://fixtures.invalid/image.png"}}, "input_image_count": float64(1)})
	require.NoError(t, err)
	replay, err := ReplayDFLOPVerificationTerminal(t.Context(), fixture, plugin, request.RequestBody, submission.PluginState, terminal)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": float64(1), "input_image_count": float64(1)}, replay.NormalizedUsage)
}
