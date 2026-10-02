package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
)

// The registry contains identities and quantities only. Prices belong to the
// authenticated, frozen pricing snapshot owned by the verification run.
//
//go:embed testdata/dflop-verification/registry-v1.json testdata/dflop-verification/media-v1.json testdata/dflop-verification/provider-docs-v1.json testdata/dflop-verification/*.png testdata/dflop-verification/*.wav testdata/dflop-verification/*.mp4
var dflopVerificationFiles embed.FS

func DFLOPVerificationDocumentationSnapshot() []byte {
	data, _ := dflopVerificationFiles.ReadFile("testdata/dflop-verification/provider-docs-v1.json")
	return data
}

type VerificationBound struct {
	Min    float64   `json:"min"`
	Max    float64   `json:"max"`
	Values []float64 `json:"values,omitempty"`
}

type VerificationContractPlan struct {
	ID                        string            `json:"id"`
	Version                   string            `json:"version"`
	RequiredFields            []string          `json:"required_fields"`
	OptionalFields            []string          `json:"optional_fields"`
	RequiredUsageFacts        []string          `json:"required_usage_facts"`
	TerminalQuantitySources   map[string]string `json:"terminal_quantity_sources"`
	ReservationQuantitySource string            `json:"reservation_quantity_source"`
	RequiredOutput            string            `json:"required_output"`
	SuccessStates             []string          `json:"success_states"`
	FailureStates             []string          `json:"failure_states"`
	PollMethod                string            `json:"poll_method"`
	PollIntervalSeconds       int               `json:"poll_interval_seconds"`
	MaxPollSeconds            int               `json:"max_poll_seconds"`
	IdempotencySupported      bool              `json:"idempotency_supported"`
	Documentation             []string          `json:"documentation"`
}

type VerificationMediaFixture struct {
	ID                   string  `json:"id"`
	Path                 string  `json:"path"`
	MIMEType             string  `json:"mime_type"`
	SHA256               string  `json:"sha256"`
	Bytes                int     `json:"bytes"`
	Width                int     `json:"width,omitempty"`
	Height               int     `json:"height,omitempty"`
	Seconds              float64 `json:"seconds,omitempty"`
	Synthetic            bool    `json:"synthetic"`
	PublicURL            string  `json:"public_url,omitempty"`
	RequiresFace         bool    `json:"requires_face,omitempty"`
	Provenance           string  `json:"provenance,omitempty"`
	RightsClassification string  `json:"rights_classification,omitempty"`
	AuthorizationRecord  string  `json:"authorization_record,omitempty"`
}

type VerificationFixture struct {
	ID                         string                       `json:"id"`
	Version                    string                       `json:"version"`
	Model                      string                       `json:"model"`
	Plugin                     string                       `json:"plugin"`
	Protocol                   string                       `json:"protocol"`
	Operation                  string                       `json:"operation"`
	Mode                       string                       `json:"mode"`
	Endpoint                   string                       `json:"endpoint"`
	SourceCatalogHash          string                       `json:"source_catalog_hash"`
	CatalogRowHash             string                       `json:"catalog_row_hash"`
	Request                    map[string]any               `json:"request"`
	Bounds                     map[string]VerificationBound `json:"bounds"`
	Selectors                  map[string][]string          `json:"selectors"`
	Media                      []VerificationMediaFixture   `json:"media,omitempty"`
	RequiredInputs             []string                     `json:"required_inputs,omitempty"`
	BlockedReason              string                       `json:"blocked_reason,omitempty"`
	RequiresPublishedFixture   bool                         `json:"requires_published_fixture"`
	OperatorAssetSpecification string                       `json:"operator_asset_specification,omitempty"`
	Plan                       VerificationContractPlan     `json:"plan"`
	verifyPublicMedia          func(context.Context, VerificationMediaFixture) error
	verifyClipSource           func(context.Context, string, string) error
}

type ProductionVerificationRequest struct {
	Body             []byte         `json:"body"`
	RequestBody      []byte         `json:"request_body"`
	BodyHash         string         `json:"body_hash"`
	Method           string         `json:"method"`
	URLPath          string         `json:"url_path"`
	Model            string         `json:"model"`
	Action           string         `json:"action"`
	ReservationFacts map[string]any `json:"reservation_facts"`
}

type ProductionVerificationSubmission struct {
	// TaskID is the production parser's replay identity. Synchronous images
	// may use a generated UUID or request_id here; neither proves a provider task.
	TaskID string `json:"task_id"`
	// ExactTaskID comes only from an explicit acknowledgement id.
	ExactTaskID     string `json:"exact_task_id,omitempty"`
	PluginState     []byte `json:"plugin_state"`
	ImmediateStatus string `json:"immediate_status,omitempty"`
	TaskData        []byte `json:"task_data"`
}

type ProductionVerificationPoll struct {
	Method  string `json:"method"`
	URLPath string `json:"url_path"`
}

type ProductionVerificationReplay struct {
	Status          string         `json:"status"`
	ReasonCode      string         `json:"reason_code,omitempty"`
	NormalizedUsage map[string]any `json:"normalized_usage,omitempty"`
	OutputValid     bool           `json:"output_valid"`
	TaskID          string         `json:"task_id"`
	PluginState     []byte         `json:"plugin_state"`
	ProviderFacts   map[string]any `json:"provider_facts"`
}

// DFLOPVerificationContractPlans supplies reusable semantics; a successful
// fixture replay never promotes any other model or mode to runtime verified.
func DFLOPVerificationContractPlans() []VerificationContractPlan {
	const mediaDocs = "https://model.dflop.top/en/docs/reference/media-apis"
	const humanDocs = "https://model.dflop.top/en/docs/reference/digital-human-apis"
	plans := []VerificationContractPlan{
		{ID: "OPENAI_IMAGE_PER_OUTPUT", RequiredFields: []string{"model", "prompt", "n"}, RequiredUsageFacts: []string{"image_count"}, TerminalQuantitySources: map[string]string{"image_count": "authoritative unit_count or delivered image payloads"}, RequiredOutput: "image"},
		{ID: "OPENAI_IMAGE_OUTPUT_PLUS_REFERENCE", RequiredFields: []string{"model", "prompt", "n"}, RequiredUsageFacts: []string{"image_count", "input_image_count"}, TerminalQuantitySources: map[string]string{"image_count": "authoritative unit_count or delivered image payloads", "input_image_count": "frozen references actually submitted"}, RequiredOutput: "image"},
		{ID: "MIDJOURNEY_FIXED_GENERATION", RequiredFields: []string{"model", "prompt", "n"}, RequiredUsageFacts: []string{"image_count"}, TerminalQuantitySources: map[string]string{"image_count": "authoritative unit_count; never grid candidate URL count"}, RequiredOutput: "image"},
		{ID: "VIDEO_OUTPUT_SECONDS", RequiredFields: []string{"model", "prompt", "duration"}, RequiredUsageFacts: []string{"duration_sec"}, TerminalQuantitySources: map[string]string{"duration_sec": "duration_sec"}, RequiredOutput: "video"},
		{ID: "VIDEO_TOKEN_PLUS_OUTPUT_SECONDS", RequiredFields: []string{"model", "prompt", "duration", "resolution"}, RequiredUsageFacts: []string{"completion_tokens", "resolution", "input_mode"}, TerminalQuantitySources: map[string]string{"completion_tokens": "usage.completion_tokens", "duration_sec": "delivered duration_sec for two-stage lite only", "resolution": "resolution confirmed against frozen delivery selector", "input_mode": "frozen reference video selector"}, RequiredOutput: "video"},
		{ID: "VIDEO_INPUT_PLUS_OUTPUT_SECONDS", RequiredFields: []string{"model", "prompt", "duration", "resolution"}, RequiredUsageFacts: []string{"duration_sec", "input_video_duration_sec", "resolution"}, TerminalQuantitySources: map[string]string{"duration_sec": "duration_sec", "input_video_duration_sec": "input_video_duration_sec required in reference video mode"}, RequiredOutput: "video"},
		{ID: "SUNO_PER_GENERATION", RequiredFields: []string{"model", "prompt"}, RequiredUsageFacts: []string{"generation_count"}, TerminalQuantitySources: map[string]string{"generation_count": "one succeeded generation with at least one successful track"}, RequiredOutput: "music"},
		{ID: "ASYNC_TTS_PER_CHARACTER", RequiredFields: []string{"model", "input", "async"}, RequiredUsageFacts: []string{"character_count"}, TerminalQuantitySources: map[string]string{"character_count": "characters"}, RequiredOutput: "audio"},
		{ID: "VOICE_CLONE_FIXED_UNIT", RequiredFields: []string{"model", "name", "audio_url"}, RequiredUsageFacts: []string{"count"}, TerminalQuantitySources: map[string]string{"count": "one ready voice resource"}, RequiredOutput: "voice"},
		{ID: "CLIP_COMPOSE_FIXED_TASK", RequiredFields: []string{"model", "video_url", "asr_id"}, RequiredUsageFacts: []string{"count"}, TerminalQuantitySources: map[string]string{"count": "one successful compose task"}, RequiredOutput: "video"},
		{ID: "AVATAR_SECONDS", RequiredFields: []string{"model", "avatar", "audio_url", "duration"}, RequiredUsageFacts: []string{"duration_sec"}, TerminalQuantitySources: map[string]string{"duration_sec": "duration_sec"}, RequiredOutput: "video"},
		{ID: "AVATAR_CREATE_FIXED_UNIT", RequiredFields: []string{"model", "name", "source_url", "source_kind"}, RequiredUsageFacts: []string{"count"}, TerminalQuantitySources: map[string]string{"count": "one ready avatar resource"}, RequiredOutput: "avatar"},
		{ID: "LIPSYNC_SECONDS", RequiredFields: []string{"model", "source_video_url", "audio_url", "duration"}, RequiredUsageFacts: []string{"duration_sec"}, TerminalQuantitySources: map[string]string{"duration_sec": "duration_sec"}, RequiredOutput: "video"},
		{ID: "MOTION_SOURCE_SECONDS", RequiredFields: []string{"model", "source_video_url", "face_count", "content", "duration", "resolution"}, RequiredUsageFacts: []string{"duration_sec", "resolution"}, TerminalQuantitySources: map[string]string{"duration_sec": "source_duration_sec, never output duration_sec"}, RequiredOutput: "video"},
	}
	for i := range plans {
		plan := &plans[i]
		plan.Version = "2"
		plan.OptionalFields = []string{"async"}
		plan.ReservationQuantitySource = "production extractUsage from validated bounded request"
		plan.SuccessStates = []string{"succeeded"}
		plan.FailureStates = []string{"failed", "expired", "cancelled"}
		plan.PollMethod = http.MethodGet
		plan.PollIntervalSeconds = 3
		plan.MaxPollSeconds = 1800
		plan.IdempotencySupported = true
		plan.Documentation = []string{mediaDocs}
		if slices.Contains([]string{"voice", "avatar"}, plan.RequiredOutput) {
			plan.SuccessStates = []string{"ready"}
			plan.PollIntervalSeconds = 15
		}
		if strings.HasPrefix(plan.ID, "AVATAR") || plan.ID == "LIPSYNC_SECONDS" || plan.ID == "MOTION_SOURCE_SECONDS" || plan.ID == "CLIP_COMPOSE_FIXED_TASK" {
			plan.Documentation = append(plan.Documentation, humanDocs)
		}
		if plan.RequiredOutput == "image" {
			plan.SuccessStates = []string{"completed", "succeeded", "HTTP_200_IMAGE_PAYLOAD"}
			plan.OptionalFields = []string{"size", "image", "response_format"}
			plan.MaxPollSeconds = 300
		}
	}
	return plans
}

// DFLOPVerificationFixtures uses fresh authenticated capabilities for request
// bounds. Missing capabilities remain blockers rather than guessed defaults.
type VerificationFixtureOptions struct {
	SourceCatalogHash  string
	PublishedMedia     map[string]VerificationMediaFixture
	PresetAvatar       string
	PresetVoice        string
	ClipASRID          string
	ClipSourceVideoURL string
	ClipStyleID        string
	VerifyPublicMedia  func(context.Context, VerificationMediaFixture) error
	VerifyClipSource   func(context.Context, string, string) error
}

func DFLOPVerificationFixtures(catalog []dflop.Item) []VerificationFixture {
	return DFLOPVerificationFixturesWithOptions(catalog, VerificationFixtureOptions{})
}

func DFLOPVerificationFixturesWithOptions(catalog []dflop.Item, options VerificationFixtureOptions) []VerificationFixture {
	var registry struct {
		Version           string   `json:"version"`
		SourceCatalogHash string   `json:"source_catalog_hash"`
		Models            []string `json:"models"`
	}
	data, _ := dflopVerificationFiles.ReadFile("testdata/dflop-verification/registry-v1.json")
	if common.Unmarshal(data, &registry) != nil {
		return nil
	}
	items := make(map[string]dflop.Item, len(catalog))
	for _, item := range catalog {
		items[item.ModelID] = item
	}
	catalogData, _ := common.Marshal(catalog)
	registry.SourceCatalogHash = fmt.Sprintf("%x", sha256.Sum256(catalogData))
	if options.SourceCatalogHash != "" {
		registry.SourceCatalogHash = options.SourceCatalogHash
	}
	plans := DFLOPVerificationContractPlans()
	fixtures := make([]VerificationFixture, 0, len(registry.Models))
	for _, model := range registry.Models {
		item, exists := items[model]
		fixture := VerificationFixture{ID: registry.Version + ":" + model + ":minimal", Version: registry.Version, Model: model, Plugin: item.TaskPlugin, Protocol: "openai_video", Operation: "create", Mode: "t2v", Endpoint: "/v1/videos/generations", SourceCatalogHash: registry.SourceCatalogHash, CatalogRowHash: fmt.Sprintf("%x", sha256.Sum256(item.Raw)), Request: map[string]any{"model": model, "prompt": "A blue square on a plain background"}, Bounds: map[string]VerificationBound{}, Selectors: map[string][]string{}}
		planID := "VIDEO_OUTPUT_SECONDS"
		switch {
		case strings.HasPrefix(model, "tvod-midjourney-"):
			planID = "MIDJOURNEY_FIXED_GENERATION"
		case model == "qwen-image-3.0":
			planID = "OPENAI_IMAGE_OUTPUT_PLUS_REFERENCE"
		case item.Category == "image" || strings.HasPrefix(model, "doubao-seedream-"):
			planID = "OPENAI_IMAGE_PER_OUTPUT"
		case strings.HasPrefix(model, "doubao-seedance-"):
			planID = "VIDEO_TOKEN_PLUS_OUTPUT_SECONDS"
		case strings.HasPrefix(model, "wan3.0-"):
			planID = "VIDEO_INPUT_PLUS_OUTPUT_SECONDS"
		case strings.HasPrefix(model, "suno-"):
			planID = "SUNO_PER_GENERATION"
		case model == "voice-tts-pro":
			planID = "ASYNC_TTS_PER_CHARACTER"
		case model == "voice-clone-pro":
			planID = "VOICE_CLONE_FIXED_UNIT"
		case model == "clip-compose":
			planID = "CLIP_COMPOSE_FIXED_TASK"
		case model == "dh-avatar":
			planID = "AVATAR_SECONDS"
		case model == "dh-avatar-create":
			planID = "AVATAR_CREATE_FIXED_UNIT"
		case strings.HasPrefix(model, "dh-lipsync"):
			planID = "LIPSYNC_SECONDS"
		case model == "dh-motion":
			planID = "MOTION_SOURCE_SECONDS"
		}
		for _, plan := range plans {
			if plan.ID == planID {
				fixture.Plan = plan
				break
			}
		}
		fixture.Plan.Documentation = append(fixture.Plan.Documentation, "https://model.dflop.top/models/"+model)
		if len(item.RequiredFacts) > 0 {
			fixture.Plan.RequiredUsageFacts = slices.Clone(item.RequiredFacts)
		}
		if !exists || !item.Callable {
			fixture.BlockedReason = "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG"
			fixtures = append(fixtures, fixture)
			continue
		}
		if item.TaskPlugin == "" {
			fixture.BlockedReason = "NO_EXACT_RUNTIME_BINDING"
		}
		var source struct {
			Caps struct {
				Image *struct {
					MaxOutputs   int `json:"max_outputs"`
					FixedOutputs int `json:"fixed_outputs"`
					Size         struct {
						MinPixels   int    `json:"min_pixels"`
						MaxPixels   int    `json:"max_pixels"`
						MaxSide     int    `json:"max_side"`
						DefaultSize string `json:"default_size"`
					} `json:"size"`
				} `json:"image"`
				Video *struct {
					Modes       map[string]bool `json:"modes"`
					Resolutions []string        `json:"resolutions"`
					Ratios      []string        `json:"ratios"`
					Requires    []string        `json:"requires"`
					Duration    struct {
						Min           float64   `json:"min"`
						Max           float64   `json:"max"`
						Values        []float64 `json:"values"`
						FollowsSource bool      `json:"follows_source"`
					} `json:"duration"`
				} `json:"video"`
			} `json:"caps"`
		}
		if common.Unmarshal(item.Raw, &source) != nil {
			fixture.BlockedReason = "PROVIDER_CAPABILITIES_MISSING"
		}
		switch fixture.Plan.RequiredOutput {
		case "image":
			fixture.Protocol, fixture.Operation, fixture.Mode, fixture.Endpoint = "openai_image", "generate", "text_to_image", "/v1/images/generations"
			fixture.Request["n"] = 1
			fixture.Request["response_format"] = "url"
			if source.Caps.Image == nil || source.Caps.Image.MaxOutputs < 1 {
				fixture.BlockedReason = "PROVIDER_CAPABILITIES_MISSING"
				break
			}
			fixture.Bounds["n"] = VerificationBound{Min: 1, Max: float64(source.Caps.Image.MaxOutputs)}
			if source.Caps.Image.FixedOutputs > 0 {
				fixture.Bounds["output_count"] = VerificationBound{Min: float64(source.Caps.Image.FixedOutputs), Max: float64(source.Caps.Image.FixedOutputs)}
			}
			if fixture.Plan.ID != "MIDJOURNEY_FIXED_GENERATION" {
				size := source.Caps.Image.Size.DefaultSize
				if size == "" && source.Caps.Image.Size.MinPixels > 0 {
					for side := 64; side <= source.Caps.Image.Size.MaxSide; side += 64 {
						if side*side >= source.Caps.Image.Size.MinPixels {
							size = fmt.Sprintf("%dx%d", side, side)
							break
						}
					}
				}
				if size == "" {
					fixture.BlockedReason = "PROVIDER_VALID_IMAGE_SIZE_MISSING"
				} else {
					fixture.Request["size"] = size
				}
			}
		case "music":
			fixture.Protocol, fixture.Operation, fixture.Mode, fixture.Endpoint = "native", "music", "music", "/v1/music/generations"
			fixture.Request = map[string]any{"model": model, "prompt": "A short calm instrumental melody", "instrumental": true}
			fixture.Bounds["generation_count"] = VerificationBound{Min: 1, Max: 1}
		case "audio":
			fixture.Protocol, fixture.Operation, fixture.Mode, fixture.Endpoint = "openai_audio_speech", "create", "speech", "/v1/audio/speech"
			fixture.Request = map[string]any{"model": model, "input": "Hello.", "async": true}
			if options.PresetVoice != "" {
				fixture.Request["voice"] = options.PresetVoice
			}
			fixture.Bounds["characters"] = VerificationBound{Min: 1, Max: 6}
		case "voice":
			fixture.Protocol, fixture.Operation, fixture.Mode, fixture.Endpoint = "native", "voice", "synthetic_clone", "/v1/audio/voices"
			fixture.Request = map[string]any{"model": model, "name": "Verification voice", "async": true, "language": "en"}
			fixture.RequiredInputs = []string{"published_synthetic_speech_audio_url"}
			fixture.Media = []VerificationMediaFixture{dflopVerificationMedia("synthetic-speech-v1")}
		case "avatar":
			fixture.Protocol, fixture.Operation, fixture.Mode, fixture.Endpoint = "native", "avatar", "avatar_image", "/v1/videos/avatars"
			fixture.Request = map[string]any{"model": model, "name": "Verification avatar", "source_kind": "image"}
			fixture.RequiredInputs = []string{"published_authorized_portrait_source_url"}
			fixture.BlockedReason = "OPERATOR_ASSET_REQUIRED"
		default:
			if source.Caps.Video != nil && source.Caps.Video.Duration.Min > 0 && source.Caps.Video.Duration.Max >= source.Caps.Video.Duration.Min {
				for _, required := range source.Caps.Video.Requires {
					if required != "ref_image" && required != "ref_video" {
						fixture.BlockedReason = "PROVIDER_REQUIRED_INPUT_UNSUPPORTED"
					}
				}
				fixture.Request["duration"] = source.Caps.Video.Duration.Min
				fixture.Bounds["duration"] = VerificationBound{Min: source.Caps.Video.Duration.Min, Max: source.Caps.Video.Duration.Max, Values: source.Caps.Video.Duration.Values}
				if len(source.Caps.Video.Resolutions) > 0 {
					fixture.Request["resolution"] = source.Caps.Video.Resolutions[0]
					fixture.Selectors["resolution"] = slices.Clone(source.Caps.Video.Resolutions)
				}
				if len(source.Caps.Video.Ratios) > 0 {
					fixture.Selectors["ratio"] = slices.Clone(source.Caps.Video.Ratios)
					fixture.Request["ratio"] = source.Caps.Video.Ratios[0]
				}
				if slices.Contains(source.Caps.Video.Requires, "ref_image") || !source.Caps.Video.Modes["t2v"] && source.Caps.Video.Modes["i2v"] {
					fixture.Mode = "i2v"
					fixture.RequiredInputs = []string{"published_reference_image_url"}
					fixture.Media = []VerificationMediaFixture{dflopVerificationMedia("reference-grid-v1")}
				}
				if slices.Contains(source.Caps.Video.Requires, "ref_video") {
					fixture.Mode = "reference_video"
					fixture.RequiredInputs = []string{"published_source_video_url"}
					fixture.Media = []VerificationMediaFixture{dflopVerificationMedia("moving-square-v1")}
				}
			} else if planID == "CLIP_COMPOSE_FIXED_TASK" || planID == "AVATAR_SECONDS" || planID == "LIPSYNC_SECONDS" || planID == "MOTION_SOURCE_SECONDS" {
				delete(fixture.Request, "prompt")
				fixture.RequiredInputs = []string{"published_source_video_url"}
				fixture.Media = []VerificationMediaFixture{dflopVerificationMedia("moving-square-v1")}
				if planID != "CLIP_COMPOSE_FIXED_TASK" {
					fixture.Request["duration"] = 3
					fixture.Bounds["duration"] = VerificationBound{Min: 3, Max: 3}
				}
				if planID == "LIPSYNC_SECONDS" || planID == "AVATAR_SECONDS" {
					fixture.RequiredInputs = append(fixture.RequiredInputs, "published_driving_audio_url")
					fixture.Media = append(fixture.Media, dflopVerificationMedia("synthetic-speech-v1"))
					fixture.BlockedReason = "FIXTURE_MATCHING_AUDIO_VIDEO_REQUIRED"
				}
				if planID == "LIPSYNC_SECONDS" {
					fixture.Mode = "audio_driven"
					fixture.RequiredInputs = []string{"published_authorized_human_face_video_url", "published_matching_driving_audio_url"}
					fixture.BlockedReason = "OPERATOR_ASSET_REQUIRED"
				}
				if planID == "CLIP_COMPOSE_FIXED_TASK" {
					fixture.Mode = "speech_clip"
					fixture.RequiredInputs = []string{"published_video_with_clear_speech_url", "fresh_asr_id_for_exact_source_video", "active_clip_template_style_id"}
					fixture.BlockedReason = "PROVIDER_DOCUMENTATION_CONFLICT"
				}
				if planID == "MOTION_SOURCE_SECONDS" {
					fixture.Mode = "video_edit"
					fixture.Request["face_count"] = 1
					fixture.Request["resolution"] = "fast"
					fixture.Selectors["resolution"] = []string{"fast", "standard", "max"}
					fixture.RequiredInputs = []string{"published_authorized_human_motion_video_url", "published_authorized_portrait_url"}
					fixture.BlockedReason = "OPERATOR_ASSET_REQUIRED"
				}
				if planID == "AVATAR_SECONDS" {
					fixture.Mode = "avatar_audio"
					fixture.RequiredInputs = append(fixture.RequiredInputs, "ready_avatar_resource")
					fixture.BlockedReason = "PROVIDER_DOCUMENTATION_CONFLICT"
				}
			} else {
				fixture.BlockedReason = "PROVIDER_BOUNDED_DURATION_MISSING"
			}
		}
		// Exact model pages distinguish omni-reference lists from first-frame inputs.
		switch model {
		case "happyhorse-1.0-r2v", "happyhorse-1.1-r2v":
			fixture.Mode = "r2v"
			fixture.RequiredInputs = []string{"published_reference_image_list_1_to_9"}
			fixture.Media = []VerificationMediaFixture{dflopVerificationMedia("reference-grid-v1")}
		case "happyhorse-1.0-video-edit":
			fixture.Mode = "video_edit"
		case "tvod-jimeng-1.0-lite-i2v":
			fixture.Mode = "i2v"
			fixture.RequiredInputs = []string{"published_reference_image_url"}
			fixture.Media = []VerificationMediaFixture{dflopVerificationMedia("reference-grid-v1")}
		}
		if planID == "CLIP_COMPOSE_FIXED_TASK" {
			fixture.BlockedReason = "FRESH_SAME_SOURCE_ASR_REQUIRED"
			fixture.Media = nil
			fixture.Plan.RequiredFields = []string{"model", "video_url", "asr_id"}
			if options.ClipASRID != "" && options.ClipSourceVideoURL != "" && options.VerifyClipSource != nil {
				fixture.Request["asr_id"] = options.ClipASRID
				fixture.Request["video_url"] = options.ClipSourceVideoURL
				fixture.Request["video_style_id"] = options.ClipStyleID
				fixture.BlockedReason = ""
				fixture.RequiredInputs = nil
			}
		}
		if planID == "AVATAR_SECONDS" {
			fixture.Mode = "avatar"
			fixture.Media = nil
			fixture.Plan.RequiredFields = []string{"model", "avatar", "voice", "text", "duration"}
			fixture.RequiredInputs = []string{"free_ready_preset_avatar", "free_preset_voice"}
			fixture.BlockedReason = "PROVIDER_PRESET_AVATAR_VOICE_REQUIRED"
			if options.PresetAvatar != "" && options.PresetVoice != "" {
				fixture.Request["avatar"] = options.PresetAvatar
				fixture.Request["voice"] = options.PresetVoice
				fixture.Request["text"] = "Hello."
				fixture.BlockedReason = ""
				fixture.RequiredInputs = nil
			}
		}
		if fixture.Mode == "i2v" || fixture.Mode == "r2v" || fixture.Mode == "video_edit" || fixture.Mode == "reference_video" {
			fixture.Plan.RequiredFields = []string{"model", "content", "duration"}
			delete(fixture.Request, "prompt")
		}
		if fixture.BlockedReason == "OPERATOR_ASSET_REQUIRED" {
			switch planID {
			case "AVATAR_CREATE_FIXED_UNIT":
				fixture.OperatorAssetSpecification = "Rights-cleared front-facing human portrait PNG/JPEG, or short same-identity human video; source provenance, explicit organization consent or fictional synthetic identity classification, SHA-256, MIME, pixel dimensions; public HTTPS direct URL with immutable checksum verified"
			case "LIPSYNC_SECONDS":
				fixture.OperatorAssetSpecification = "Rights-cleared human face source video MP4 with visible speaking face; synthetic or explicitly authorized matching driving audio; both bounded short duration and public HTTPS direct URLs; provenance/consent, SHA-256, MIME, dimensions, duration required"
			case "MOTION_SOURCE_SECONDS":
				fixture.OperatorAssetSpecification = "Rights-cleared short human motion source MP4 plus 1-7 authorized or fictional synthetic human portraits with face_count matching portrait count; SHA-256, MIME, dimensions, duration, provenance/consent and verified immutable public HTTPS direct URLs required"
			}
		}
		fixture.verifyPublicMedia = options.VerifyPublicMedia
		fixture.verifyClipSource = options.VerifyClipSource
		for index := range fixture.Media {
			media := &fixture.Media[index]
			if published, exists := options.PublishedMedia[media.ID]; exists && published.SHA256 == media.SHA256 && published.Bytes == media.Bytes && published.MIMEType == media.MIMEType {
				media.PublicURL = published.PublicURL
			}
		}
		if len(fixture.RequiredInputs) > 0 && fixture.BlockedReason == "" {
			fixture.RequiresPublishedFixture = true
			fixture.BlockedReason = "FIXTURE_PUBLIC_URL_REQUIRED"
		}
		if fixture.BlockedReason == "FIXTURE_PUBLIC_URL_REQUIRED" && len(fixture.Media) > 0 && options.VerifyPublicMedia != nil {
			ready := true
			for _, media := range fixture.Media {
				ready = ready && media.PublicURL != ""
			}
			if ready {
				fixture.BlockedReason = ""
				fixture.RequiresPublishedFixture = true
				url := fixture.Media[0].PublicURL
				switch fixture.Mode {
				case "i2v", "r2v":
					prompt := "A blue square moves on a plain background"
					if fixture.Mode == "r2v" {
						prompt = "character1 moves gently on a plain background"
					}
					fixture.Request["content"] = []any{map[string]any{"type": "text", "text": prompt}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}}
				case "reference_video", "video_edit":
					fixture.Request["content"] = []any{map[string]any{"type": "text", "text": "Make the moving square green"}, map[string]any{"type": "video_url", "video_url": map[string]any{"url": url}}}
				}
				if fixture.Plan.ID == "VOICE_CLONE_FIXED_UNIT" {
					fixture.Request["audio_url"] = url
				}
			}
		}
		// Explicit reviewed overrides are evidence, not blanket conflicts.
		if item.OverrideStale {
			fixture.BlockedReason = "PROVIDER_DOCUMENTED_OVERRIDE_STALE"
		}

		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

// ValidateDFLOPVerificationMedia validates immutable local fixture bytes before
// any canary can be submitted. Public hosting is a separate explicit blocker.
func ValidateDFLOPVerificationMedia(fixture VerificationMediaFixture) error {
	if fixture.Provenance == "" || fixture.RightsClassification == "" {
		return fmt.Errorf("FIXTURE_PROVENANCE_REQUIRED: %s", fixture.ID)
	}
	if fixture.RequiresFace {
		fictional := fixture.Synthetic && fixture.RightsClassification == "fictional-human-test-only"
		consented := !fixture.Synthetic && fixture.RightsClassification == "organization-performer-explicit-consent" && fixture.AuthorizationRecord != ""
		if !fictional && !consented {
			return fmt.Errorf("OPERATOR_ASSET_REQUIRED: %s", fixture.ID)
		}
	} else if !fixture.Synthetic || fixture.RightsClassification != "synthetic-test-only" {
		return fmt.Errorf("FIXTURE_RIGHTS_CLASSIFICATION_REQUIRED: %s", fixture.ID)
	}

	data, err := dflopVerificationFiles.ReadFile(fixture.Path)
	if err != nil || len(data) != fixture.Bytes || fmt.Sprintf("%x", sha256.Sum256(data)) != fixture.SHA256 {
		return fmt.Errorf("FIXTURE_CHECKSUM_MISMATCH: %s", fixture.ID)
	}
	switch fixture.MIMEType {
	case "image/png":
		image, decodeErr := png.Decode(bytes.NewReader(data))
		if decodeErr != nil || image.Bounds().Dx() != fixture.Width || image.Bounds().Dy() != fixture.Height {
			return fmt.Errorf("FIXTURE_INVALID_IMAGE: %s", fixture.ID)
		}
	case "audio/wav":
		if len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
			return fmt.Errorf("FIXTURE_INVALID_AUDIO: %s", fixture.ID)
		}
		var rate, blockAlign, dataSize uint32
		for offset := 12; offset+8 <= len(data); {
			size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			start := offset + 8
			if size < 0 || size > len(data)-start {
				return fmt.Errorf("FIXTURE_INVALID_AUDIO: %s", fixture.ID)
			}
			switch string(data[offset : offset+4]) {
			case "fmt ":
				if size < 16 || binary.LittleEndian.Uint16(data[start:start+2]) != 1 {
					return fmt.Errorf("FIXTURE_INVALID_AUDIO: %s", fixture.ID)
				}
				rate = binary.LittleEndian.Uint32(data[start+4 : start+8])
				blockAlign = uint32(binary.LittleEndian.Uint16(data[start+12 : start+14]))
			case "data":
				dataSize += uint32(size)
			}
			offset = start + size + size%2
		}
		if rate == 0 || blockAlign == 0 || dataSize == 0 || float64(dataSize)/float64(rate*blockAlign) != fixture.Seconds {
			return fmt.Errorf("FIXTURE_INVALID_AUDIO: %s", fixture.ID)
		}
	case "video/mp4":
		if len(data) < 16 || string(data[4:8]) != "ftyp" {
			return fmt.Errorf("FIXTURE_INVALID_VIDEO: %s", fixture.ID)
		}
		atoms, atomErr := dflopVideoAtoms(data)
		if atomErr != nil || len(atoms["mdat"]) == 0 || len(atoms["mvhd"]) == 0 || len(atoms["tkhd"]) == 0 {
			return fmt.Errorf("FIXTURE_INVALID_VIDEO: %s", fixture.ID)
		}
		movie, track := atoms["mvhd"][0], atoms["tkhd"][0]
		if len(movie) < 20 || movie[0] != 0 || len(track) < 8 {
			return fmt.Errorf("FIXTURE_INVALID_VIDEO: %s", fixture.ID)
		}
		scale := binary.BigEndian.Uint32(movie[12:16])
		duration := binary.BigEndian.Uint32(movie[16:20])
		width := int(binary.BigEndian.Uint32(track[len(track)-8:len(track)-4]) >> 16)
		height := int(binary.BigEndian.Uint32(track[len(track)-4:]) >> 16)
		if scale == 0 || float64(duration)/float64(scale) != fixture.Seconds || width != fixture.Width || height != fixture.Height {
			return fmt.Errorf("FIXTURE_INVALID_VIDEO: %s", fixture.ID)
		}
	default:
		return fmt.Errorf("FIXTURE_UNSUPPORTED_MEDIA: %s", fixture.ID)
	}
	return nil
}

func dflopVideoAtoms(data []byte) (map[string][][]byte, error) {
	atoms := make(map[string][][]byte)
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, fmt.Errorf("truncated video atom")
		}
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		kind := string(data[offset+4 : offset+8])
		header := 8
		if size == 1 {
			if len(data)-offset < 16 {
				return nil, fmt.Errorf("truncated extended video atom")
			}
			extended := binary.BigEndian.Uint64(data[offset+8 : offset+16])
			if extended > uint64(len(data)-offset) {
				return nil, fmt.Errorf("invalid extended video atom")
			}
			size, header = int(extended), 16
		}
		if size < header || size > len(data)-offset {
			return nil, fmt.Errorf("invalid video atom")
		}
		body := data[offset+header : offset+size]
		atoms[kind] = append(atoms[kind], body)
		if kind == "moov" || kind == "trak" {
			children, err := dflopVideoAtoms(body)
			if err != nil {
				return nil, err
			}
			for key, values := range children {
				atoms[key] = append(atoms[key], values...)
			}
		}
		offset += size
	}
	return atoms, nil
}

func ValidateDFLOPVerificationFixture(ctx context.Context, fixture VerificationFixture, plugin *jsplugin.LoadedPlugin) (ProductionVerificationRequest, error) {
	var request ProductionVerificationRequest
	for _, media := range fixture.Media {
		if err := ValidateDFLOPVerificationMedia(media); err != nil {
			return request, err
		}
	}
	if fixture.BlockedReason != "" {
		return request, fmt.Errorf("%s", fixture.BlockedReason)
	}
	if fixture.RequiresPublishedFixture {
		if fixture.verifyPublicMedia == nil || len(fixture.Media) == 0 {
			return request, fmt.Errorf("FIXTURE_PUBLIC_URL_REQUIRED")
		}
		for _, media := range fixture.Media {
			if media.PublicURL == "" {
				return request, fmt.Errorf("FIXTURE_PUBLIC_URL_REQUIRED")
			}
			if err := fixture.verifyPublicMedia(ctx, media); err != nil {
				return request, err
			}
		}
	}
	if fixture.Plan.ID == "CLIP_COMPOSE_FIXED_TASK" {
		if fixture.verifyClipSource == nil {
			return request, fmt.Errorf("FRESH_SAME_SOURCE_ASR_REQUIRED")
		}
		source, _ := fixture.Request["video_url"].(string)
		asrID, _ := fixture.Request["asr_id"].(string)
		if err := fixture.verifyClipSource(ctx, source, asrID); err != nil {
			return request, err
		}
	}
	if plugin == nil || plugin.Engine == nil || plugin.Meta.Key != fixture.Plugin || !slices.Contains(plugin.Meta.Models, fixture.Model) {
		return request, fmt.Errorf("PLUGIN_BINDING_MISMATCH")
	}
	schema, _ := plugin.Meta.UsageForModels(fixture.Model)
	for _, key := range fixture.Plan.RequiredUsageFacts {
		if _, declared := schema[key]; !declared {
			return request, fmt.Errorf("PRODUCTION_USAGE_SCHEMA_MISMATCH: %s", key)
		}
	}
	for _, field := range fixture.Plan.RequiredFields {
		if value, present := fixture.Request[field]; !present || value == nil || value == "" {
			return request, fmt.Errorf("FIXTURE_REQUIRED_FIELD_MISSING: %s", field)
		}
	}
	for field, bound := range fixture.Bounds {
		value, present := fixture.Request[field]
		if !present {
			continue
		}
		number, valid := jsplugin.UsageNumber(value, false)
		if !valid || number < bound.Min || number > bound.Max || len(bound.Values) > 0 && !slices.Contains(bound.Values, number) {
			return request, fmt.Errorf("FIXTURE_QUANTITY_OUT_OF_BOUNDS: %s", field)
		}
	}
	for field, choices := range fixture.Selectors {
		if value, present := fixture.Request[field]; present {
			text, ok := value.(string)
			if !ok || !slices.Contains(choices, text) {
				return request, fmt.Errorf("FIXTURE_SELECTOR_INVALID: %s", field)
			}
		}
	}
	clientBody, err := common.Marshal(fixture.Request)
	if err != nil {
		return request, err
	}
	var body map[string]any
	if err = common.Unmarshal(clientBody, &body); err != nil {
		return request, err
	}
	decodeContext := map[string]any{"model": fixture.Model, "upstreamModel": fixture.Model, "operation": fixture.Operation, "protocol": fixture.Protocol, "stream": false, "path": fixture.Endpoint, "method": "POST", "body": map[string]any{"kind": "json", "value": body}}
	var decoded any
	if fixture.Protocol == "native" {
		decoded, err = plugin.Engine.CallMember(ctx, "native", fixture.Operation, decodeContext)
	} else {
		decoded, err = plugin.Engine.CallPath(ctx, "protocols", []string{fixture.Protocol, "decodeRequest"}, decodeContext)
	}
	if err != nil {
		return request, fmt.Errorf("PRODUCTION_REQUEST_DECODER_REJECTED: %w", err)
	}
	intent, ok := decoded.(map[string]any)
	if !ok || intent["kind"] != "submit" || intent["model"] != fixture.Model {
		return request, fmt.Errorf("PRODUCTION_REQUEST_MODEL_MISMATCH")
	}
	requestBody, ok := intent["requestBody"].(map[string]any)
	if !ok {
		return request, fmt.Errorf("PRODUCTION_REQUEST_BODY_MISSING")
	}
	request.Action, _ = intent["action"].(string)
	driverContext := dflopVerificationDriverContext(fixture, requestBody)
	driverContext["action"] = request.Action
	driverContext["requestHeaders"] = map[string]any{"Idempotency-Key": "offline-fixture-" + fixture.ID}
	descriptorValue, err := plugin.Engine.Call(ctx, "buildSubmitRequest", driverContext)
	if err != nil {
		return request, fmt.Errorf("PRODUCTION_REQUEST_SERIALIZATION_REJECTED: %w", err)
	}
	descriptor, ok := descriptorValue.(map[string]any)
	if !ok {
		return request, fmt.Errorf("PRODUCTION_REQUEST_DESCRIPTOR_INVALID")
	}
	request.Method, _ = descriptor["method"].(string)
	address, _ := descriptor["url"].(string)
	parsed, err := url.Parse(address)
	if err != nil || request.Method != http.MethodPost || parsed.Path != fixture.Endpoint || parsed.Hostname() != "api.dflop.top" {
		return request, fmt.Errorf("PRODUCTION_ENDPOINT_MISMATCH")
	}
	if err = jsplugin.ValidateRequestURL(address, "https://api.dflop.top", plugin.Meta.AllowedHosts); err != nil {
		return request, err
	}
	if headers, ok := descriptor["headers"].(map[string]any); !ok || headers["Idempotency-Key"] != "offline-fixture-"+fixture.ID {
		return request, fmt.Errorf("PRODUCTION_IDEMPOTENCY_MISSING")
	}
	request.Body, err = common.Marshal(descriptor["body"])
	if err != nil {
		return request, err
	}
	request.RequestBody, err = common.Marshal(requestBody)
	if err != nil {
		return request, err
	}
	request.BodyHash = fmt.Sprintf("%x", sha256.Sum256(request.Body))
	request.Model, request.URLPath = fixture.Model, parsed.Path
	driverContext["usagePurpose"] = "facts"
	usage, err := plugin.Engine.Call(ctx, "extractUsage", driverContext)
	if err != nil {
		return request, fmt.Errorf("PRODUCTION_RESERVATION_REJECTED: %w", err)
	}
	facts, ok := usage.(map[string]any)
	if !ok {
		return request, fmt.Errorf("BILLING_QUANTITY_MISSING")
	}
	request.ReservationFacts, err = jsplugin.ValidateUsageFacts(facts, schema)
	if err != nil {
		return request, fmt.Errorf("PRODUCTION_USAGE_SCHEMA_REJECTED: %w", err)
	}
	for _, key := range fixture.Plan.RequiredUsageFacts {
		if value, present := request.ReservationFacts[key]; !present || value == nil {
			return request, fmt.Errorf("BILLING_QUANTITY_MISSING: %s", key)
		}
	}
	return request, nil
}

func dflopVerificationDriverContext(fixture VerificationFixture, requestBody map[string]any) map[string]any {
	return map[string]any{"model": fixture.Model, "upstreamModel": fixture.Model, "baseUrl": "https://api.dflop.top", "apiKey": "offline-fixture", "upstream": map[string]any{"kind": "vendor"}, "requestBody": requestBody, "action": fixture.Operation}
}

func ParseDFLOPVerificationSubmission(ctx context.Context, fixture VerificationFixture, plugin *jsplugin.LoadedPlugin, requestBody, response []byte) (ProductionVerificationSubmission, error) {
	var submission ProductionVerificationSubmission
	var body, acknowledgement map[string]any
	if err := common.Unmarshal(requestBody, &body); err != nil {
		return submission, err
	}
	if err := common.Unmarshal(response, &acknowledgement); err != nil {
		return submission, err
	}
	value, err := plugin.Engine.Call(ctx, "parseSubmitResponse", dflopVerificationDriverContext(fixture, body), map[string]any{"statusCode": 200, "headers": map[string]string{}, "body": acknowledgement})
	if err != nil {
		return submission, fmt.Errorf("PRODUCTION_ACKNOWLEDGEMENT_REJECTED: %w", err)
	}
	parsed, ok := value.(map[string]any)
	if !ok {
		return submission, fmt.Errorf("PRODUCTION_ACKNOWLEDGEMENT_INVALID")
	}
	submission.TaskID, _ = parsed["taskId"].(string)
	if submission.TaskID == "" {
		return submission, fmt.Errorf("PROVIDER_TASK_ID_MISSING")
	}
	if exactID, present := acknowledgement["id"]; present && exactID != nil {
		id, valid := exactID.(string)
		if !valid || id == "" || id != submission.TaskID {
			return submission, fmt.Errorf("PROVIDER_TASK_ID_MISMATCH")
		}
		submission.ExactTaskID = id
	}
	state, _ := parsed["state"].(map[string]any)
	state = maps.Clone(state)
	if state == nil {
		state = make(map[string]any)
	}
	state["_verification_task_id"] = submission.TaskID
	submission.PluginState, err = common.Marshal(state)
	if err != nil {
		return submission, err
	}
	submission.TaskData, err = common.Marshal(parsed["taskData"])
	if immediate, ok := parsed["immediate"].(map[string]any); ok {
		submission.ImmediateStatus, _ = immediate["status"].(string)
	}
	return submission, err
}

func BuildDFLOPVerificationPoll(ctx context.Context, fixture VerificationFixture, plugin *jsplugin.LoadedPlugin, taskID string, state []byte) (ProductionVerificationPoll, error) {
	var poll ProductionVerificationPoll
	queryContext := dflopVerificationDriverContext(fixture, nil)
	delete(queryContext, "requestBody")
	queryContext["taskId"] = taskID
	var pluginState map[string]any
	if len(state) > 0 {
		if err := common.Unmarshal(state, &pluginState); err != nil {
			return poll, err
		}
	}
	queryContext["state"] = pluginState
	value, err := plugin.Engine.Call(ctx, "buildQueryRequest", queryContext)
	if err != nil {
		return poll, fmt.Errorf("PRODUCTION_POLL_REQUEST_REJECTED: %w", err)
	}
	descriptor, ok := value.(map[string]any)
	if !ok {
		return poll, fmt.Errorf("PRODUCTION_POLL_DESCRIPTOR_INVALID")
	}
	poll.Method, _ = descriptor["method"].(string)
	address, _ := descriptor["url"].(string)
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() != "api.dflop.top" || poll.Method != http.MethodGet || parsed.Path != fixture.Endpoint+"/"+taskID {
		return poll, fmt.Errorf("PRODUCTION_POLL_ENDPOINT_MISMATCH")
	}
	poll.URLPath = parsed.Path
	return poll, nil
}

func ReplayDFLOPVerificationTerminal(ctx context.Context, fixture VerificationFixture, plugin *jsplugin.LoadedPlugin, requestBody, state, terminal []byte) (ProductionVerificationReplay, error) {
	var pluginState map[string]any
	if err := common.Unmarshal(state, &pluginState); err != nil {
		return ProductionVerificationReplay{}, err
	}
	taskID, _ := pluginState["_verification_task_id"].(string)
	return ReplayDFLOPVerificationTaskTerminal(ctx, fixture, plugin, taskID, requestBody, state, terminal)
}

func ReplayDFLOPVerificationTaskTerminal(ctx context.Context, fixture VerificationFixture, plugin *jsplugin.LoadedPlugin, taskID string, requestBody, state, terminal []byte) (ProductionVerificationReplay, error) {
	replay := ProductionVerificationReplay{TaskID: taskID}
	if taskID == "" {
		replay.ReasonCode = "PROVIDER_TASK_ID_MISSING"
		return replay, fmt.Errorf("%s", replay.ReasonCode)
	}
	var body, pluginState map[string]any
	if err := common.Unmarshal(terminal, &body); err != nil {
		return replay, err
	}
	if err := common.Unmarshal(state, &pluginState); err != nil {
		return replay, err
	}
	if pinned, ok := pluginState["_verification_task_id"].(string); ok && pinned != taskID {
		replay.ReasonCode = "PROVIDER_TASK_ID_MISMATCH"
		return replay, fmt.Errorf("%s", replay.ReasonCode)
	}
	// Image polls deliver the synchronous envelope without an id. Other task
	// endpoints must retain their exact task identity in the terminal response.
	if fixture.Plan.RequiredOutput != "image" && body["id"] != taskID || fixture.Plan.RequiredOutput == "image" && body["id"] != nil && body["id"] != taskID {
		replay.ReasonCode = "PROVIDER_TASK_ID_MISMATCH"
		return replay, fmt.Errorf("%s", replay.ReasonCode)
	}
	queryContext := dflopVerificationDriverContext(fixture, nil)
	delete(queryContext, "requestBody")
	queryContext["taskId"], queryContext["state"] = taskID, pluginState
	value, err := plugin.Engine.Call(ctx, "parseTaskResult", queryContext, body, map[string]any{"status": 200, "headers": map[string]string{}})
	if err != nil {
		replay.ReasonCode = "PRODUCTION_TERMINAL_PARSER_REJECTED"
		if outputErr := validateDFLOPVerificationOutput(fixture, taskID, body); outputErr != nil {
			replay.ReasonCode = "GENERATION_INVALID_OUTPUT"
		}
		return replay, fmt.Errorf("%s: %w", replay.ReasonCode, err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		replay.ReasonCode = "PRODUCTION_TERMINAL_PARSER_REJECTED"
		return replay, fmt.Errorf("%s", replay.ReasonCode)
	}
	replay.Status, _ = result["status"].(string)
	if returned, ok := result["state"].(map[string]any); ok {
		pluginState = returned
		queryContext["state"] = pluginState
	}
	replay.PluginState, err = common.Marshal(pluginState)
	if err != nil {
		return replay, err
	}
	replay.ProviderFacts, err = DFLOPVerificationProviderFacts(terminal)
	if err != nil {
		replay.ReasonCode = "PROVIDER_USAGE_INVALID"
		return replay, err
	}
	if replay.Status != "SUCCESS" {
		return replay, nil
	}
	if err = validateDFLOPVerificationOutput(fixture, taskID, body); err != nil {
		replay.ReasonCode = "GENERATION_INVALID_OUTPUT"
		return replay, err
	}
	replay.OutputValid = true
	if fixture.Plan.ID == "MIDJOURNEY_FIXED_GENERATION" && body["unit_count"] == nil {
		usage, _ := body["usage"].(map[string]any)
		if usage["unit_count"] == nil && usage["output_image_count"] == nil {
			replay.ReasonCode = "BILLING_QUANTITY_MISSING"
			return replay, fmt.Errorf("%s: authoritative Midjourney generation quantity required", replay.ReasonCode)
		}
	}
	usageValue, err := plugin.Engine.Call(ctx, "extractUsageOnComplete", queryContext, result, body)
	if err != nil || pluginState["billingPending"] == true {
		replay.ReasonCode = "BILLING_QUANTITY_MISSING"
		return replay, fmt.Errorf("%s: production terminal usage remains unavailable", replay.ReasonCode)
	}
	facts, ok := usageValue.(map[string]any)
	if !ok {
		replay.ReasonCode = "BILLING_QUANTITY_MISSING"
		return replay, fmt.Errorf("%s", replay.ReasonCode)
	}
	schema, _ := plugin.Meta.UsageForModels(fixture.Model)
	replay.NormalizedUsage, err = jsplugin.ValidateUsageFacts(facts, schema)
	if err != nil {
		replay.ReasonCode = "PRODUCTION_USAGE_SCHEMA_REJECTED"
		return replay, fmt.Errorf("%s: %w", replay.ReasonCode, err)
	}
	for _, key := range fixture.Plan.RequiredUsageFacts {
		if value, present := replay.NormalizedUsage[key]; !present || value == nil {
			replay.ReasonCode = "BILLING_QUANTITY_MISSING"
			replay.NormalizedUsage = nil
			return replay, fmt.Errorf("%s: %s", replay.ReasonCode, key)
		}
	}
	return replay, nil
}

// DFLOPVerificationProviderFacts records an immutable numeric projection of
// the terminal response. Decimal text keeps the original quantity precision;
// output payloads, signed URLs, prompts and credentials are never included.
func DFLOPVerificationProviderFacts(terminal []byte) (map[string]any, error) {
	var body map[string]common.RawMessage
	if err := common.Unmarshal(terminal, &body); err != nil {
		return nil, err
	}
	facts := make(map[string]any)
	for _, key := range []string{"model", "status", "resolution", "service_tier", "unit_type"} {
		if raw, present := body[key]; present && string(raw) != "null" {
			var value string
			if err := common.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("PROVIDER_USAGE_INVALID: %s", key)
			}
			facts[key] = value
		}
	}
	quantityKeys := []string{"duration_sec", "source_duration_sec", "input_video_duration_sec", "characters", "character_count", "unit_count", "image_count", "input_image_count", "generated_images", "output_image_count", "count", "generation_count", "input_tokens", "prompt_tokens", "output_tokens", "completion_tokens", "total_tokens", "cached_tokens"}
	for _, key := range quantityKeys {
		if raw, present := body[key]; present && string(raw) != "null" {
			value, err := verificationLedgerDecimal(raw)
			if err != nil {
				return nil, fmt.Errorf("PROVIDER_USAGE_INVALID: %s", key)
			}
			facts[key] = value.String()
		}
	}
	if raw, present := body["usage"]; present && string(raw) != "null" {
		var usage map[string]common.RawMessage
		if err := common.Unmarshal(raw, &usage); err != nil {
			return nil, fmt.Errorf("PROVIDER_USAGE_INVALID: usage")
		}
		projection := make(map[string]any)
		for _, key := range quantityKeys {
			if raw, present := usage[key]; present && string(raw) != "null" {
				value, err := verificationLedgerDecimal(raw)
				if err != nil {
					return nil, fmt.Errorf("PROVIDER_USAGE_INVALID: usage.%s", key)
				}
				projection[key] = value.String()
			}
		}
		if len(projection) > 0 {
			facts["usage"] = projection
		}
	}
	return facts, nil
}

func validateDFLOPVerificationOutput(fixture VerificationFixture, taskID string, body map[string]any) error {
	valid := false
	switch fixture.Plan.RequiredOutput {
	case "image":
		entries, ok := body["data"].([]any)
		if !ok {
			if entry, ok := body["data"].(map[string]any); ok {
				entries = []any{entry}
			}
		}
		for _, value := range entries {
			if entry, ok := value.(map[string]any); ok {
				if verificationOutputURL(entry["url"]) {
					valid = true
				}
				if encoded, ok := entry["b64_json"].(string); ok && encoded != "" {
					payload, err := base64.StdEncoding.DecodeString(encoded)
					if err == nil {
						_, _, err = image.Decode(bytes.NewReader(payload))
						valid = valid || err == nil
					}
				}
			}
		}
	case "video":
		valid = verificationOutputURL(body["video_url"])
		if content, ok := body["content"].(map[string]any); ok {
			valid = valid || verificationOutputURL(content["video_url"])
		}
	case "audio":
		valid = verificationOutputURL(body["audio_url"])
	case "music":
		tracks, _ := body["tracks"].([]any)
		for _, value := range tracks {
			if track, ok := value.(map[string]any); ok && verificationOutputURL(track["audio_url"]) && (track["status"] == nil || track["status"] == "succeeded" || track["status"] == "success") {
				valid = true
			}
		}
	case "voice", "avatar":
		valid = body["status"] == "ready" && body["id"] == taskID
	}
	if !valid {
		return fmt.Errorf("GENERATION_INVALID_OUTPUT: %s", fixture.Plan.RequiredOutput)
	}
	return nil
}

func verificationOutputURL(value any) bool {
	text, ok := value.(string)
	if !ok || text == "" {
		return false
	}
	parsed, err := url.Parse(text)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil
}

func dflopVerificationMedia(id string) VerificationMediaFixture {
	data, _ := dflopVerificationFiles.ReadFile("testdata/dflop-verification/media-v1.json")
	var media []VerificationMediaFixture
	if common.Unmarshal(data, &media) == nil {
		for _, fixture := range media {
			if fixture.ID == id {
				return fixture
			}
		}
	}
	return VerificationMediaFixture{ID: id}
}

// DFLOPVerificationMediaInventory returns the test-only immutable fixture allowlist.
func DFLOPVerificationMediaInventory() []VerificationMediaFixture {
	data, _ := dflopVerificationFiles.ReadFile("testdata/dflop-verification/media-v1.json")
	var media []VerificationMediaFixture
	if common.Unmarshal(data, &media) != nil {
		return nil
	}
	return media
}

func DFLOPVerificationMediaBytes(id string) ([]byte, error) {
	fixture := dflopVerificationMedia(id)
	if err := ValidateDFLOPVerificationMedia(fixture); err != nil {
		return nil, err
	}
	return dflopVerificationFiles.ReadFile(fixture.Path)
}
