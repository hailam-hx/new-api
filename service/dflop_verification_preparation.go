package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

type VerificationPresetEvidence struct {
	Endpoint      string `json:"endpoint"`
	RetrievedAt   int64  `json:"retrieved_at"`
	ResponseHash  string `json:"response_hash"`
	Available     int    `json:"available"`
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	IDFingerprint string `json:"id_fingerprint,omitempty"`
	Status        string `json:"status"`
}

type VerificationClipPreparation struct {
	Version                  string            `json:"version"`
	CatalogHash              string            `json:"catalog_hash"`
	CredentialFingerprint    string            `json:"credential_fingerprint"`
	SourceSHA256             string            `json:"source_sha256"`
	SourceURL                string            `json:"source_url"`
	SourceURLHash            string            `json:"source_url_hash"`
	TemplatePolicy           string            `json:"template_policy"`
	TemplateResponseHash     string            `json:"template_response_hash"`
	StyleID                  string            `json:"style_id"`
	TaskID                   string            `json:"task_id,omitempty"`
	ASRID                    string            `json:"asr_id"`
	ASRSegments              common.RawMessage `json:"segments,omitempty"`
	RequestID                string            `json:"request_id,omitempty"`
	TraceID                  string            `json:"trace_id,omitempty"`
	ASRResponseHash          string            `json:"asr_response_hash"`
	DocumentationHash        string            `json:"documentation_hash"`
	MaximumPreparationPoints string            `json:"maximum_preparation_points"`
	PreparedAt               int64             `json:"prepared_at"`
	ExpiresAt                int64             `json:"expires_at"`
	Proof                    string            `json:"proof"`
}

type VerificationPreparationCorrelation struct {
	RequestID string `json:"request_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

// Only fixed provider paths are accepted. Redirects cannot leak the source key.
func (engine DFLOPVerificationEngine) preparationRequest(ctx context.Context, key, method, endpoint string, body []byte, preparationBudget *decimal.Decimal, correlation ...*VerificationPreparationCorrelation) ([]byte, error) {
	pollID := strings.TrimPrefix(endpoint, "/v1/videos/clip-subtitles/")
	allowedPoll := strings.HasPrefix(endpoint, "/v1/videos/clip-subtitles/") && verificationSecretSafeID(pollID) && !strings.Contains(pollID, "/")
	allowed := method == http.MethodGet && (allowedPoll || endpoint == "/v1/videos/avatars/presets" || endpoint == "/v1/audio/voices" || endpoint == "/v1/videos/clip-templates" || endpoint == "/v1/videos/clip-templates/categories") || method == http.MethodPost && endpoint == "/v1/videos/clip-subtitles" && preparationBudget != nil && preparationBudget.IsZero()
	if !allowed || !strings.HasPrefix(endpoint, "/v1/") || strings.ContainsAny(endpoint, "?#") {
		return nil, errors.New("PREPARATION_ENDPOINT_INVALID")
	}
	request, err := http.NewRequestWithContext(ctx, method, "https://api.dflop.top"+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		request.GetBody = nil
	}
	request.Header.Set("Authorization", "Bearer "+key)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{Timeout: 90 * time.Second}
	if engine.HTTP != nil {
		client = *engine.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("PREPARATION_NETWORK_OUTCOME_UNKNOWN")
	}
	defer response.Body.Close()
	if len(correlation) == 1 && correlation[0] != nil {
		requestID := response.Header.Get("X-Gateway-Trace")
		traceID := response.Header.Get("X-Gateway-Trace")
		if verificationSecretSafeID(requestID) && (key == "" || !strings.Contains(requestID, key)) {
			correlation[0].RequestID = requestID
		}
		if verificationSecretSafeID(traceID) && (key == "" || !strings.Contains(traceID, key)) {
			correlation[0].TraceID = traceID
		}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, errors.New("PREPARATION_RESPONSE_INVALID")
	}
	if response.StatusCode != http.StatusOK && !(method == http.MethodPost && response.StatusCode == http.StatusAccepted) {
		return nil, fmt.Errorf("PREPARATION_HTTP_%d", response.StatusCode)
	}
	return data, nil
}

func verificationPresetEvidence(endpoint string, body []byte, now time.Time) VerificationPresetEvidence {
	evidence := VerificationPresetEvidence{Endpoint: endpoint, RetrievedAt: now.Unix(), ResponseHash: verificationHash(body), Status: "READY_AVATAR_RESOURCE_REQUIRED"}
	var response struct {
		Avatars []struct{ ID, Name, Status string } `json:"avatars"`
		Presets []struct{ ID, Name, Status string } `json:"presets"`
	}
	if common.Unmarshal(body, &response) != nil {
		return evidence
	}
	rows := response.Avatars
	if endpoint == "/v1/audio/voices" {
		rows = response.Presets
		evidence.Status = "READY_VOICE_RESOURCE_REQUIRED"
	}
	for _, row := range rows {
		if row.ID == "" || row.Name == "" || !verificationSecretSafeID(row.ID) || row.Status != "" && row.Status != "ready" {
			continue
		}
		evidence.Available++
		if evidence.ID == "" {
			evidence.ID, evidence.Name = row.ID, row.Name
			evidence.IDFingerprint = verificationHash([]byte(row.ID))
			evidence.Status = "PRESET_GET_VERIFIED"
		}
	}
	return evidence
}

// Revalidate lists with the selected source key; never infer usability from docs.
func (engine DFLOPVerificationEngine) PreparePresetResources(ctx context.Context, channel int) (VerificationPresetEvidence, VerificationPresetEvidence, error) {
	source, err := engine.source(ctx, channel)
	if err != nil {
		return VerificationPresetEvidence{}, VerificationPresetEvidence{}, err
	}
	avatarBody, avatarErr := engine.preparationRequest(ctx, source.key, http.MethodGet, "/v1/videos/avatars/presets", nil, nil)
	voiceBody, voiceErr := engine.preparationRequest(ctx, source.key, http.MethodGet, "/v1/audio/voices", nil, nil)
	avatar := verificationPresetEvidence("/v1/videos/avatars/presets", avatarBody, time.Now())
	voice := verificationPresetEvidence("/v1/audio/voices", voiceBody, time.Now())
	if avatarErr != nil {
		avatar.Status = verificationReason(avatarErr)
	}
	if voiceErr != nil {
		voice.Status = verificationReason(voiceErr)
	}
	return avatar, voice, nil
}

func (engine DFLOPVerificationEngine) verifyPresetResources(ctx context.Context, key, avatar, voice string) error {
	for _, target := range []struct{ endpoint, id string }{{"/v1/videos/avatars/presets", avatar}, {"/v1/audio/voices", voice}} {
		if target.id == "" {
			if target.endpoint == "/v1/videos/avatars/presets" && voice != "" {
				continue
			}
			return errors.New("READY_VOICE_RESOURCE_REQUIRED")
		}
		body, err := engine.preparationRequest(ctx, key, http.MethodGet, target.endpoint, nil, nil)
		if err != nil {
			return err
		}
		var data struct {
			Avatars []struct{ ID, Name, Status string } `json:"avatars"`
			Presets []struct{ ID, Name, Status string } `json:"presets"`
		}
		if common.Unmarshal(body, &data) != nil {
			return errors.New("PRESET_RESPONSE_INVALID")
		}
		rows := data.Avatars
		if target.endpoint == "/v1/audio/voices" {
			rows = data.Presets
		}
		found := false
		for _, row := range rows {
			found = found || row.ID == target.id && row.Name != "" && (row.Status == "" || row.Status == "ready")
		}
		if !found {
			return errors.New("PRESET_NO_LONGER_AVAILABLE")
		}
	}
	return nil
}

func (p VerificationClipPreparation) validate(key, sourceURL, asrID, catalogHash string, now time.Time) error {
	proof := p.Proof
	p.Proof = ""
	body, _ := common.Marshal(p)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	signature, err := hex.DecodeString(proof)
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return errors.New("CLIP_PREPARATION_PROOF_INVALID")
	}
	if p.Version != "clip-zero-cost-prepare-v1" || p.CredentialFingerprint != verificationHash([]byte(key)) || p.CatalogHash != catalogHash || p.SourceURL != sourceURL || p.SourceURLHash != verificationHash([]byte(sourceURL)) || p.ASRID != asrID || p.ASRID == "" || p.StyleID == "" || p.SourceSHA256 == "" || p.ASRResponseHash == "" || p.TemplateResponseHash == "" || p.DocumentationHash == "" || p.MaximumPreparationPoints != "0" {
		return errors.New("CLIP_PREPARATION_SOURCE_MISMATCH")
	}
	if p.PreparedAt > now.Unix() || p.ExpiresAt <= now.Unix() || p.ExpiresAt-p.PreparedAt != 1800 {
		return errors.New("CLIP_PREPARATION_EXPIRED")
	}
	return nil
}

// Fallback contract: prepare free ASR first, then approve the exact body in a NEW
// 30-minute manifest. No dynamic fields are injected after signed approval.
func (engine DFLOPVerificationEngine) PrepareClip(ctx context.Context, channel int, media VerificationMediaFixture) (VerificationClipPreparation, error) {
	var prepared VerificationClipPreparation
	if media.ID != "speaking-square-v1" || !media.Synthetic || media.Seconds <= 0 || media.Seconds > 300 {
		return prepared, errors.New("CLIP_PUBLIC_SPEECH_SOURCE_REQUIRED")
	}
	if err := ValidateDFLOPVerificationMedia(media); err != nil {
		return prepared, err
	}
	if err := VerifyDFLOPPublicMedia(ctx, media); err != nil {
		return prepared, err
	}
	source, err := engine.source(ctx, channel)
	if err != nil {
		return prepared, err
	}
	// Re-read current official endpoint table; absence/changed price fails closed.
	docRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://model.dflop.top/en/docs/reference/digital-human-apis", nil)
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	docResponse, err := client.Do(docRequest)
	if err != nil {
		return prepared, errors.New("CLIP_ZERO_COST_CONTRACT_REQUIRED")
	}
	doc, readErr := io.ReadAll(io.LimitReader(docResponse.Body, 2<<20))
	docResponse.Body.Close()
	if readErr != nil || docResponse.StatusCode != 200 || !verificationClipZeroCostDocument(doc) {
		return prepared, errors.New("CLIP_ZERO_COST_CONTRACT_REQUIRED")
	}
	_, err = engine.preparationRequest(ctx, source.key, http.MethodGet, "/v1/videos/clip-templates/categories", nil, nil)
	if err != nil {
		return prepared, err
	}
	templates, err := engine.preparationRequest(ctx, source.key, http.MethodGet, "/v1/videos/clip-templates", nil, nil)
	if err != nil {
		return prepared, err
	}
	var list struct {
		List []struct {
			StyleID string `json:"style_id"`
		} `json:"list"`
	}
	if common.Unmarshal(templates, &list) != nil || len(list.List) == 0 || !verificationSecretSafeID(list.List[0].StyleID) {
		return prepared, errors.New("CLIP_CURRENT_TEMPLATE_REQUIRED")
	}
	// Hard guard belongs immediately before the only allowed preparation POST.
	maximumPreparationPoints, priceErr := verificationClipPreparationPrice(doc)
	if priceErr != nil || !maximumPreparationPoints.IsZero() {
		return prepared, errors.New("ZERO_COST_BUDGET_REQUIRED")
	}
	body, _ := common.Marshal(map[string]any{"video_url": media.PublicURL, "async": true})
	var correlation VerificationPreparationCorrelation
	asrResponse, err := engine.preparationRequest(ctx, source.key, http.MethodPost, "/v1/videos/clip-subtitles", body, &maximumPreparationPoints, &correlation)
	prepared = VerificationClipPreparation{CatalogHash: source.hash, CredentialFingerprint: source.fingerprint, SourceSHA256: media.SHA256, SourceURL: media.PublicURL, SourceURLHash: verificationHash([]byte(media.PublicURL)), DocumentationHash: verificationHash(doc), MaximumPreparationPoints: maximumPreparationPoints.String(), StyleID: list.List[0].StyleID, TemplateResponseHash: verificationHash(templates), RequestID: correlation.RequestID, TraceID: correlation.TraceID}
	if err != nil {
		return prepared, err
	}
	var asr struct {
		ID       string            `json:"id"`
		Status   string            `json:"status"`
		ASRID    string            `json:"asr_id"`
		Segments common.RawMessage `json:"segments"`
	}
	if common.Unmarshal(asrResponse, &asr) != nil {
		return prepared, errors.New("CLIP_ASR_RESULT_REQUIRED")
	}
	taskID := asr.ID
	if taskID != "" && (!verificationSecretSafeID(taskID) || strings.Contains(taskID, source.key)) || asr.ASRID != "" && (!verificationSecretSafeID(asr.ASRID) || strings.Contains(asr.ASRID, source.key)) {
		return prepared, errors.New("PROVIDER_CORRELATION_ID_INVALID")
	}
	prepared.TaskID = taskID
	prepared.ASRID = asr.ASRID
	deadline := time.Now().Add(25 * time.Minute)
	for asr.Status == "running" || asr.Status == "queued" || asr.Status == "pending" {
		if !verificationSecretSafeID(taskID) || strings.Contains(taskID, "/") {
			return prepared, errors.New("CLIP_ASR_TASK_ID_REQUIRED")
		}
		if time.Now().After(deadline) {
			return prepared, errors.New("CLIP_ASR_TERMINAL_TIMEOUT")
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return prepared, ctx.Err()
		case <-timer.C:
		}
		asrResponse, err = engine.preparationRequest(ctx, source.key, http.MethodGet, "/v1/videos/clip-subtitles/"+taskID, nil, nil)
		if err != nil {
			return prepared, err
		}
		if common.Unmarshal(asrResponse, &asr) != nil || asr.ID != taskID {
			return prepared, errors.New("CLIP_ASR_TASK_MISMATCH")
		}
	}
	if asr.Status != "" && asr.Status != "succeeded" {
		return prepared, errors.New("CLIP_ASR_TERMINAL_FAILED")
	}
	if asr.ASRID == "" || !verificationSecretSafeID(asr.ASRID) || strings.Contains(asr.ASRID, source.key) || len(asr.Segments) == 0 || string(asr.Segments) == "[]" || string(asr.Segments) == "null" {
		return prepared, errors.New("CLIP_ASR_RESULT_REQUIRED")
	}
	now := time.Now().Unix()
	prepared = VerificationClipPreparation{Version: "clip-zero-cost-prepare-v1", CatalogHash: source.hash, CredentialFingerprint: source.fingerprint, SourceSHA256: media.SHA256, SourceURL: media.PublicURL, SourceURLHash: verificationHash([]byte(media.PublicURL)), TemplatePolicy: "first-listed-valid-style-v1", TemplateResponseHash: verificationHash(templates), StyleID: list.List[0].StyleID, TaskID: taskID, ASRID: asr.ASRID, ASRSegments: asr.Segments, RequestID: correlation.RequestID, TraceID: correlation.TraceID, ASRResponseHash: verificationHash(asrResponse), DocumentationHash: verificationHash(doc), MaximumPreparationPoints: maximumPreparationPoints.String(), PreparedAt: now, ExpiresAt: now + 1800}
	proofBody, _ := common.Marshal(prepared)
	mac := hmac.New(sha256.New, []byte(source.key))
	mac.Write(proofBody)
	prepared.Proof = hex.EncodeToString(mac.Sum(nil))
	return prepared, nil
}

func verificationClipZeroCostDocument(doc []byte) bool {
	rows := regexp.MustCompile(`(?s)<tr\b[^>]*>.*?</tr>`).FindAllString(string(doc), -1)
	tags := regexp.MustCompile(`<[^>]+>`)
	for _, row := range rows {
		text := strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(row, " "))), " ")
		if regexp.MustCompile(`(?:^| )POST /v1/videos/clip-subtitles(?: |$)`).MatchString(text) && strings.Contains(text, "per call (currently 0)") {
			return true
		}
	}
	return false
}

// ClipPreparationVerifier validates provider-issued proof against a fresh selected
// source. Serialized ASR IDs alone never authorize a compose request.
func (engine DFLOPVerificationEngine) ClipPreparationVerifier(channel int, prepared VerificationClipPreparation) func(context.Context, string, string) error {
	return func(ctx context.Context, url, id string) error {
		source, err := engine.source(ctx, channel)
		if err != nil {
			return err
		}
		if err := prepared.validate(source.key, url, id, source.hash, time.Now()); err != nil {
			return err
		}
		media := dflopVerificationMedia("speaking-square-v1")
		if media.SHA256 != prepared.SourceSHA256 {
			return errors.New("CLIP_PREPARATION_SOURCE_MISMATCH")
		}
		templates, err := engine.preparationRequest(ctx, source.key, http.MethodGet, "/v1/videos/clip-templates", nil, nil)
		if err != nil {
			return err
		}
		if verificationHash(templates) != prepared.TemplateResponseHash {
			return errors.New("CLIP_PREPARATION_TEMPLATE_CHANGED")
		}
		return nil
	}
}

func verificationClipPreparationPrice(doc []byte) (decimal.Decimal, error) {
	if !verificationClipZeroCostDocument(doc) {
		return decimal.Zero, errors.New("CLIP_ZERO_COST_CONTRACT_REQUIRED")
	}
	// Only the exact helper endpoint row establishes this documented preparation
	// rate. The paid subtitle-generation model is a different endpoint contract.
	return decimal.Zero, nil
}

// RestoreApprovedPreparation rehydrates only prerequisites bound by signed
// target fixture hashes. It never creates a replacement ASR after approval.
func (engine DFLOPVerificationEngine) RestoreApprovedPreparation(ctx context.Context, channel int, plan DFLOPVerificationPlan, authorization DFLOPVerificationAuthorization) (VerificationFixtureOptions, error) {
	options := engine.FixtureOptions
	source, err := engine.source(ctx, channel)
	if err != nil {
		return options, err
	}
	if source.hash != authorization.CatalogHash || source.fingerprint != authorization.CredentialFingerprint {
		return options, errors.New("DRIFT_REVIEW_REQUIRED")
	}
	if options.PublishedMedia == nil {
		options.PublishedMedia = map[string]VerificationMediaFixture{}
	}
	for _, target := range authorization.Targets {
		found := false
		for _, planned := range plan.Targets {
			if planned.Model != target.Model {
				continue
			}
			encoded, _ := common.Marshal(planned.Fixture)
			if verificationHash(encoded) != target.FixtureHash || planned.RequestBodyHash != target.RequestBodyHash {
				return options, errors.New("APPROVED_FIXTURE_PREPARATION_MISMATCH")
			}
			found = true
			fixture := planned.Fixture
			for _, media := range fixture.Media {
				if err := ValidateDFLOPVerificationMedia(media); err != nil {
					return options, err
				}
				options.PublishedMedia[media.ID] = media
			}
			if fixture.AvatarPresetEvidence != nil {
				options.AvatarPresetEvidence = fixture.AvatarPresetEvidence
				options.PresetAvatar = fixture.AvatarPresetEvidence.ID
			}
			if fixture.VoicePresetEvidence != nil {
				options.VoicePresetEvidence = fixture.VoicePresetEvidence
				options.PresetVoice = fixture.VoicePresetEvidence.ID
			}
			if fixture.ClipPreparation != nil {
				p := *fixture.ClipPreparation
				url, _ := fixture.Request["video_url"].(string)
				id, _ := fixture.Request["asr_id"].(string)
				if err := p.validate(source.key, url, id, source.hash, time.Now()); err != nil {
					return options, err
				}
				options.ClipPreparation = &p
				options.ClipASRID = p.ASRID
				options.ClipSourceVideoURL = p.SourceURL
				options.ClipStyleID = p.StyleID
				options.VerifyClipSource = engine.ClipPreparationVerifier(channel, p)
			}
			break
		}
		if !found {
			return options, errors.New("APPROVED_TARGET_MISSING_FROM_PLAN")
		}
	}
	options.VerifyPublicMedia = VerifyDFLOPPublicMedia
	return options, nil
}
