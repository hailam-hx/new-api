package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preparationTransport func(*http.Request) (*http.Response, error)

func (f preparationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerificationPresetRevalidation(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, tc := range []struct {
		name, body, status string
		count              int
	}{{"empty", `{"avatars":[]}`, "READY_AVATAR_RESOURCE_REQUIRED", 0}, {"ready", `{"avatars":[{"id":"preset-1","name":"Test"}]}`, "PRESET_GET_VERIFIED", 1}, {"pending", `{"avatars":[{"id":"preset-1","name":"Test","status":"pending"}]}`, "READY_AVATAR_RESOURCE_REQUIRED", 0}, {"unusable", `{"avatars":[{"id":"preset-1"}]}`, "READY_AVATAR_RESOURCE_REQUIRED", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			e := verificationPresetEvidence("/v1/videos/avatars/presets", []byte(tc.body), now)
			assert.Equal(t, tc.status, e.Status)
			assert.Equal(t, tc.count, e.Available)
			assert.Equal(t, now.Unix(), e.RetrievedAt)
			assert.NotEmpty(t, e.ResponseHash)
		})
	}
	engine := DFLOPVerificationEngine{HTTP: &http.Client{Transport: preparationTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "Bearer source-key", r.Header.Get("Authorization"))
		body := `{"avatars":[{"id":"avatar","name":"Test"}]}`
		if r.URL.Path == "/v1/audio/voices" {
			body = `{"presets":[{"id":"voice","name":"Speech"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}}
	require.NoError(t, engine.verifyPresetResources(context.Background(), "source-key", "avatar", "voice"))
	require.ErrorContains(t, engine.verifyPresetResources(context.Background(), "source-key", "withdrawn-avatar", "voice"), "PRESET_NO_LONGER_AVAILABLE")
}

func TestVerificationClipPreparationBinding(t *testing.T) {
	now := time.Unix(1000, 0)
	key := "source-key"
	p := VerificationClipPreparation{Version: "clip-zero-cost-prepare-v1", CatalogHash: "catalog", CredentialFingerprint: verificationHash([]byte(key)), SourceSHA256: "source-sha", SourceURL: "https://public.example/source.mp4", TemplatePolicy: "first-listed-valid-style-v1", TemplateResponseHash: "template-response", StyleID: "style", ASRID: "asr-fresh", ASRResponseHash: "response", DocumentationHash: "docs", MaximumPreparationPoints: "0", PreparedAt: now.Unix(), ExpiresAt: now.Add(30 * time.Minute).Unix()}
	p.SourceURLHash = verificationHash([]byte(p.SourceURL))
	body, err := common.Marshal(p)
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	p.Proof = hex.EncodeToString(mac.Sum(nil))
	require.NoError(t, p.validate(key, p.SourceURL, p.ASRID, "catalog", now))
	require.ErrorContains(t, p.validate(key, "https://public.example/other.mp4", p.ASRID, "catalog", now), "SOURCE_MISMATCH")
	require.ErrorContains(t, p.validate(key, p.SourceURL, "injected-asr", "catalog", now), "SOURCE_MISMATCH")
	require.ErrorContains(t, p.validate(key, p.SourceURL, p.ASRID, "changed-catalog", now), "SOURCE_MISMATCH")
	require.ErrorContains(t, p.validate(key, p.SourceURL, p.ASRID, "catalog", now.Add(30*time.Minute)), "EXPIRED")
	tampered := p
	tampered.ASRID = "injected"
	require.ErrorContains(t, tampered.validate(key, tampered.SourceURL, tampered.ASRID, "catalog", now), "PROOF_INVALID")
	require.ErrorContains(t, p.validate("other-key", p.SourceURL, p.ASRID, "catalog", now), "PROOF_INVALID")
	for _, tc := range []struct {
		name, row string
		valid     bool
	}{{"zero", `<tr><td>POST /v1/videos/clip-subtitles</td><td>per call (currently 0)</td></tr>`, true}, {"charged", `<tr><td>POST /v1/videos/clip-subtitles</td><td>per call (currently 1)</td></tr>`, false}, {"prefix endpoint", `<tr><td>POST /v1/videos/clip-subtitles-paid</td><td>per call (currently 0)</td></tr>`, false},
		{"different endpoint", `<tr><td>POST /v1/videos/generations</td><td>per call (currently 0)</td></tr>`, false}, {"removed", `<p>currently 0</p>`, false}} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.valid, verificationClipZeroCostDocument([]byte(tc.row)))
			price, err := verificationClipPreparationPrice([]byte(tc.row))
			if tc.valid {
				require.NoError(t, err)
				assert.True(t, price.IsZero())
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestVerificationClipArbitraryIDRequiresPreparation(t *testing.T) {
	options := VerificationFixtureOptions{ClipASRID: "arbitrary", ClipSourceVideoURL: "https://public.example/source.mp4", ClipStyleID: "style", VerifyClipSource: func(context.Context, string, string) error { return nil }}
	for _, f := range DFLOPVerificationFixturesWithOptions(verificationFixtureCatalog(t), options) {
		if f.Model == "clip-compose" {
			assert.Equal(t, "FRESH_SAME_SOURCE_ASR_REQUIRED", f.BlockedReason)
			assert.NotContains(t, f.Request, "asr_id")
			return
		}
	}
	t.Fatal("clip fixture absent")
}

func TestVerificationHumanIdentityMetadata(t *testing.T) {
	media := dflopVerificationMedia("reference-grid-v1")
	media.RequiresFace = true
	media.AssetType = "portrait"
	media.Synthetic = true
	media.RightsClassification = "fictional-human-test-only"
	media.IdentityClassification = "SYNTHETIC_FICTIONAL_PERSON"
	media.RightsBasis = "Wholly fictional generated test identity"
	media.Provenance = "Generated test identity, no intended real person"
	media.CreatedAt = "2026-10-02T00:00:00Z"
	require.NoError(t, ValidateDFLOPVerificationMedia(media))
	bad := media
	bad.IdentityClassification = ""
	require.ErrorContains(t, ValidateDFLOPVerificationMedia(bad), "AUTHORIZATION_REQUIRED")
	bad = media
	bad.Provenance = ""
	require.ErrorContains(t, ValidateDFLOPVerificationMedia(bad), "PROVENANCE_REQUIRED")
	bad = media
	bad.IdentityClassification = "UNKNOWN_WEB_PERSON"
	require.ErrorContains(t, ValidateDFLOPVerificationMedia(bad), "AUTHORIZATION_REQUIRED")
	bad = media
	bad.Synthetic = false
	bad.IdentityClassification = "CONSENTED_TEST_PERFORMER"
	bad.RightsClassification = "organization-performer-explicit-consent"
	require.ErrorContains(t, ValidateDFLOPVerificationMedia(bad), "AUTHORIZATION_REQUIRED")
	bad.AuthorizationRecord = "organization-explicit-test-consent-record"
	require.NoError(t, ValidateDFLOPVerificationMedia(bad))
}

func TestVerificationPreparationOutboundBudgetGate(t *testing.T) {
	engine := DFLOPVerificationEngine{HTTP: &http.Client{Transport: preparationTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("blocked request reached network")
		return nil, nil
	})}}
	_, err := engine.preparationRequest(t.Context(), "key", http.MethodPost, "/v1/videos/generations", []byte(`{}`), nil)
	require.ErrorContains(t, err, "ENDPOINT_INVALID")
	_, err = engine.preparationRequest(t.Context(), "key", http.MethodPost, "/v1/videos/clip-subtitles", []byte(`{}`), nil)
	require.ErrorContains(t, err, "ENDPOINT_INVALID")
	positive := decimal.NewFromInt(1)
	_, err = engine.preparationRequest(t.Context(), "key", http.MethodPost, "/v1/videos/clip-subtitles", []byte(`{}`), &positive)
	require.ErrorContains(t, err, "ENDPOINT_INVALID")
}

func TestVerificationApprovedPreparationSurvivesPlanReload(t *testing.T) {
	rig := newVerificationEngineRig(t)
	source, err := rig.engine.source(t.Context(), 1)
	require.NoError(t, err)
	voice := VerificationPresetEvidence{ID: "preset-voice", Status: "PRESET_GET_VERIFIED", RetrievedAt: time.Now().Unix(), ResponseHash: "preset-response", IDFingerprint: verificationHash([]byte("preset-voice"))}
	options := VerificationFixtureOptions{PresetVoice: voice.ID, VoicePresetEvidence: &voice}
	var fixture VerificationFixture
	for _, f := range DFLOPVerificationFixturesWithOptions(source.items, options) {
		if f.Model == "voice-tts-pro" {
			fixture = f
			break
		}
	}
	fixture.SourceCatalogHash = source.hash
	encoded, err := common.Marshal(fixture)
	require.NoError(t, err)
	authorization := rig.authorization
	authorization.Targets[0].FixtureHash = verificationHash(encoded)
	plan := DFLOPVerificationPlan{Version: DFLOPVerificationPlanVersion, Targets: []DFLOPVerificationPlannedTarget{{Model: fixture.Model, Fixture: fixture, RequestBodyHash: authorization.Targets[0].RequestBodyHash}}}
	data, err := common.Marshal(plan)
	require.NoError(t, err)
	var reloaded DFLOPVerificationPlan
	require.NoError(t, common.Unmarshal(data, &reloaded))
	restored, err := rig.engine.RestoreApprovedPreparation(t.Context(), 1, reloaded, authorization)
	require.NoError(t, err)
	assert.Equal(t, voice.ID, restored.PresetVoice)
	assert.Equal(t, voice, *restored.VoicePresetEvidence)
	for _, f := range DFLOPVerificationFixturesWithOptions(source.items, restored) {
		if f.Model == fixture.Model {
			f.SourceCatalogHash = source.hash
			regenerated, err := common.Marshal(f)
			require.NoError(t, err)
			assert.Equal(t, verificationHash(encoded), verificationHash(regenerated))
		}
	}
	reloaded.Targets[0].Fixture.Request["voice"] = "injected-preset"
	_, err = rig.engine.RestoreApprovedPreparation(t.Context(), 1, reloaded, authorization)
	require.ErrorContains(t, err, "PREPARATION_MISMATCH")
	assert.Zero(t, rig.transport.posts)
}
