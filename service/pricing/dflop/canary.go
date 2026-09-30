package dflop

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// CanaryCase describes an evidence request, not permission to make one.
type CanaryCase struct {
	ID                    string            `json:"id"`
	Priority              int               `json:"priority"`
	Model                 string            `json:"model"`
	Endpoint              string            `json:"endpoint"`
	Purpose               string            `json:"purpose"`
	BlockedReason         string            `json:"blocked_reason"`
	BillingFeatures       []string          `json:"billing_features"`
	PriceComponentsPoints map[string]string `json:"price_components_points"`
	RequiredFacts         []string          `json:"required_facts"`
	FixtureInputs         []string          `json:"fixture_inputs,omitempty"`
	ExternalFixtures      []string          `json:"external_fixtures,omitempty"`
	PotentialUnlocks      int               `json:"potential_unlocks"`
	PriceUnit             string            `json:"price_unit,omitempty"`
	UnitPricePoints       string            `json:"unit_price_points,omitempty"`
	EstimatedPoints       string            `json:"estimated_points,omitempty"`
	EstimatedMaxPoints    string            `json:"estimated_max_points,omitempty"`
	EstimatedMaxUSD       string            `json:"estimated_max_usd,omitempty"`
	CostPerUnlockPoints   string            `json:"cost_per_potential_unlock_points,omitempty"`
	Status                string            `json:"status"`
	Risk                  string            `json:"risk"`
}

type CanaryPlan struct {
	ChannelID       int          `json:"channel_id"`
	CreatedAt       time.Time    `json:"created_at"`
	CatalogHash     string       `json:"catalog_hash"`
	PricingHash     string       `json:"pricing_hash"`
	SchemaVersion   string       `json:"schema_version"`
	ETag            string       `json:"etag"`
	PointsPerCNY    string       `json:"points_per_cny"`
	CNYToUSD        string       `json:"cny_to_usd"`
	CallableModels  int          `json:"callable_models"`
	DirectSupported int          `json:"direct_supported_auto"`
	Cases           []CanaryCase `json:"cases"`
}

// BuildCanaryPlan uses only the authenticated effective catalog. Unknown cost
// ceilings remain empty, even if a minimum sample cost can be displayed.
func BuildCanaryPlan(channelID int, catalog, currency []byte, etag, cnyToUSD string) (CanaryPlan, error) {
	items, hash, meta, err := BuildEffective(catalog, currency, cnyToUSD, "1")
	if err != nil {
		return CanaryPlan{}, err
	}
	var unit Currency
	if err := common.Unmarshal(currency, &unit); err != nil {
		return CanaryPlan{}, err
	}
	pointsPerCNY, err := decimal.NewFromString(string(unit.PointsPerCNY))
	if err != nil || !pointsPerCNY.IsPositive() {
		return CanaryPlan{}, errors.New("invalid points_per_cny")
	}
	rate, err := decimal.NewFromString(cnyToUSD)
	if err != nil || !rate.IsPositive() {
		return CanaryPlan{}, errors.New("invalid CNY-to-USD rate")
	}
	plan := CanaryPlan{ChannelID: channelID, CreatedAt: time.Now().UTC(), CatalogHash: fmt.Sprintf("%x", sha256.Sum256(catalog)), PricingHash: hash, SchemaVersion: meta.SchemaVersion, ETag: etag, PointsPerCNY: pointsPerCNY.String(), CNYToUSD: rate.String()}
	byID := make(map[string]Item, len(items))
	for i, item := range items {
		if item.ReasonCode == "NO_PLUGIN_USAGE_PROFILE" {
			item.ReasonCode = missingTaskBindingReason(item)
		}
		items[i] = item
		byID[item.ModelID] = item
		if item.Callable {
			plan.CallableModels++
		}
		if item.Status == SupportedAuto {
			plan.DirectSupported++
		}
	}
	add := func(id string, priority int, item Item, purpose string, facts, fixtures []string, unlocks int, fixedPrice string, risk string) {
		c := CanaryCase{ID: id, Priority: priority, Model: item.ModelID, Endpoint: item.EndpointType, Purpose: purpose, BlockedReason: item.ReasonCode, BillingFeatures: slices.Clone(item.BillingFeatures), PriceComponentsPoints: map[string]string{}, RequiredFacts: facts, ExternalFixtures: fixtures, PotentialUnlocks: unlocks, Status: "CANARY_COST_UNBOUNDED", Risk: risk}
		for name, price := range item.Prices {
			c.PriceComponentsPoints[name] = price.Credits
		}
		if price, ok := item.Prices[fixedPrice]; ok {
			c.UnitPricePoints = price.Credits
			c.PriceUnit = string(price.Unit)
			// Only one-operation fixed prices have a catalog-derived ceiling.
			if fixedPrice == "price_per_voice_clone" || fixedPrice == "price_per_video_task" || fixedPrice == "price_per_avatar" {
				c.EstimatedPoints = price.Credits
				c.EstimatedMaxPoints = price.Credits
				amount, _ := decimal.NewFromString(price.Credits)
				c.EstimatedMaxUSD = amount.DivRound(pointsPerCNY, 24).Mul(rate).Round(12).String()
			}
			if unlocks > 0 && c.EstimatedMaxPoints != "" {
				amount, _ := decimal.NewFromString(price.Credits)
				c.CostPerUnlockPoints = amount.DivRound(decimal.NewFromInt(int64(unlocks)), 12).String()
			}
		}
		if len(fixtures) > 0 {
			c.Status = "BLOCKED_MISSING_FIXTURE"
		}
		if c.EstimatedMaxPoints != "" && len(fixtures) == 0 {
			c.Status = "CANARY_DESIGN_UNVERIFIED"
		}
		plan.Cases = append(plan.Cases, c)
	}
	if item, ok := byID["voice-tts-pro"]; ok {
		add("tts-async", 1, item, "submit, poll, final character and duration facts", []string{"task_id", "terminal_status", "characters", "duration_sec", "poll_cost_zero"}, nil, 1, "price_per_tts_char", "Async route and character upper bound unverified; Hello. is only a proposed input")
		plan.Cases[len(plan.Cases)-1].Endpoint = "async speech route unverified"
	}
	var grok []Item
	for _, item := range items {
		if item.ReasonCode == "LIVE_CANARY_REQUIRED" && slices.Contains(item.BillingFeatures, "server_tool_call") {
			grok = append(grok, item)
		}
	}
	if len(grok) > 0 {
		slices.SortFunc(grok, func(a, b Item) int {
			pa, _ := decimal.NewFromString(a.Prices["price_per_server_tool_call"].Credits)
			pb, _ := decimal.NewFromString(b.Prices["price_per_server_tool_call"].Credits)
			if cmp := pa.Cmp(pb); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.ModelID, b.ModelID)
		})
		for _, protocol := range []string{"chat", "responses", "stream"} {
			add("grok-tool-"+protocol, 2, grok[0], "prove an actual server-side tool call and terminal usage", []string{"num_server_side_tools_used", "input_tokens", "output_tokens"}, nil, len(grok), "price_per_server_tool_call", "Forced server-tool contract and total token/tool ceiling unverified")
			plan.Cases[len(plan.Cases)-1].Status = "CANARY_DESIGN_UNVERIFIED"
			switch protocol {
			case "chat":
				plan.Cases[len(plan.Cases)-1].Endpoint = "/v1/chat/completions (candidate)"
			case "responses":
				plan.Cases[len(plan.Cases)-1].Endpoint = "/v1/responses (candidate)"
			case "stream":
				plan.Cases[len(plan.Cases)-1].Endpoint = "/v1/chat/completions stream (candidate)"
			}
		}
	}
	var suno []Item
	for _, item := range items {
		if item.ReasonCode == "NO_EXACT_MUSIC_PLUGIN_BINDING" {
			suno = append(suno, item)
		}
	}
	if len(suno) > 0 {
		slices.SortFunc(suno, func(a, b Item) int {
			pa, _ := decimal.NewFromString(a.Prices["price_per_music_generation"].Credits)
			pb, _ := decimal.NewFromString(b.Prices["price_per_music_generation"].Credits)
			if cmp := pa.Cmp(pb); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.ModelID, b.ModelID)
		})
		add("suno-generation", 3, suno[0], "reconcile generation versus track billing", []string{"task_id", "terminal_status", "tracks", "generation_billing_unit"}, nil, len(suno), "price_per_music_generation", "SEMANTICS_CONFLICT_REQUIRES_RUNTIME_EVIDENCE")
		plan.Cases[len(plan.Cases)-1].Status = "SEMANTICS_CONFLICT_REQUIRES_RUNTIME_EVIDENCE"
	}
	if item, ok := byID["voice-clone-pro"]; ok {
		add("voice-clone", 4, item, "clone submit, voice id, ready or failed", []string{"voice_id", "terminal_status"}, []string{"authorized_reference_audio_url"}, 1, "price_per_voice_clone", "Expensive; authorized synthetic or user-owned voice required")
	}
	for _, id := range []string{"tvod-midjourney-v7", "tvod-midjourney-v8.1"} {
		if item, ok := byID[id]; ok {
			add("output-count-"+id, 5, item, "count usable returned image payloads", []string{"terminal_status", "usable_image_outputs"}, nil, 1, "price_per_image", "Exact task route and output ceiling unverified")
		}
	}
	seedanceShapes := map[string]bool{}
	for _, item := range items {
		if item.ReasonCode != "UNVERIFIED_VIDEO_TOKEN_USAGE" {
			continue
		}
		shape := strings.Join(item.BillingFeatures, "+") + "/" + item.PricingShape
		if seedanceShapes[shape] {
			continue
		}
		seedanceShapes[shape] = true
		facts := []string{"terminal_status", "usage.completion_tokens", "resolution", "service_tier", "has_video_input"}
		if slices.Contains(item.BillingFeatures, "video_two_stage") {
			facts = append(facts, "duration_sec", "second_stage_component")
		}
		add("seedance-"+item.ModelID, 6, item, "verify all catalog billing components", facts, []string{"user_owned_image_or_video_url_if_required"}, 1, "", "Composite video-token ceiling and exact route unverified")
		plan.Cases[len(plan.Cases)-1].FixtureInputs = []string{"text-only", "image-reference:user-owned-url", "video-reference:user-owned-url"}
	}
	for _, item := range items {
		if item.ReasonCode == "NO_EXACT_VIDEO_PLUGIN_BINDING" || item.ReasonCode == "NO_EXACT_FIXED_TASK_BINDING" || item.ReasonCode == "NO_EXACT_AVATAR_PLUGIN_BINDING" {
			// These three exact Wan bindings were already verified in the
			// pricing preview; they are not Phase 7 blocker candidates.
			if slices.Contains([]string{"wan2.7-t2v", "wan3.0-video", "wan3.0-video-prime"}, item.ModelID) {
				continue
			}
			fixed := ""
			for _, key := range []string{"price_per_video_task", "price_per_avatar", "price_per_video_second"} {
				if _, ok := item.Prices[key]; ok {
					fixed = key
					break
				}
			}
			add("task-"+item.ModelID, 7, item, "discover exact task route and final usage", []string{"task_id", "terminal_status", "billable_quantity"}, []string{"route_specific_fixture_if_required"}, 1, fixed, "Route, duration or input bounds unverified")
		}
	}
	slices.SortFunc(plan.Cases, func(a, b CanaryCase) int {
		if a.Priority != b.Priority {
			return a.Priority - b.Priority
		}
		if a.Priority == 7 {
			if a.EstimatedMaxPoints != "" && b.EstimatedMaxPoints == "" {
				return -1
			}
			if a.EstimatedMaxPoints == "" && b.EstimatedMaxPoints != "" {
				return 1
			}
			if a.EstimatedMaxPoints != "" && b.EstimatedMaxPoints != "" {
				pa, _ := decimal.NewFromString(a.EstimatedMaxPoints)
				pb, _ := decimal.NewFromString(b.EstimatedMaxPoints)
				if cmp := pa.Cmp(pb); cmp != 0 {
					return cmp
				}
			}
			if len(a.BillingFeatures) != len(b.BillingFeatures) {
				return len(a.BillingFeatures) - len(b.BillingFeatures)
			}
		}
		return strings.Compare(a.Model, b.Model)
	})
	return plan, nil
}

type CanaryAuthorization struct {
	Execute     bool
	CaseID      string
	MaxCostUSD  string
	ConfirmPaid bool
}

// CheckCanaryAuthorization is deliberately a final, same-invocation gate. A
// caller must re-fetch and re-plan immediately before any future paid transport.
func CheckCanaryAuthorization(auth CanaryAuthorization, oldCase, freshCase CanaryCase) error {
	if !auth.Execute || !auth.ConfirmPaid || auth.CaseID == "" || auth.MaxCostUSD == "" || auth.CaseID != oldCase.ID || auth.CaseID != freshCase.ID {
		return errors.New("CANARY_AUTHORIZATION_REQUIRED")
	}
	limit, err := decimal.NewFromString(auth.MaxCostUSD)
	if err != nil || !limit.IsPositive() || oldCase.EstimatedMaxUSD == "" || freshCase.EstimatedMaxUSD == "" {
		return errors.New("CANARY_COST_UNBOUNDED")
	}
	oldMax, err := decimal.NewFromString(oldCase.EstimatedMaxUSD)
	if err != nil {
		return errors.New("CANARY_COST_UNBOUNDED")
	}
	freshMax, err := decimal.NewFromString(freshCase.EstimatedMaxUSD)
	if err != nil {
		return errors.New("CANARY_COST_UNBOUNDED")
	}
	if freshCase.Model != oldCase.Model || freshCase.Endpoint != oldCase.Endpoint || freshMax.GreaterThan(oldMax) || freshCase.EstimatedMaxPoints != oldCase.EstimatedMaxPoints {
		return errors.New("CANARY_PRICE_CHANGED")
	}
	if freshMax.GreaterThan(limit) {
		return errors.New("BLOCKED_BY_BUDGET")
	}
	if freshCase.Status != "READY_FOR_AUTHORIZATION" {
		return errors.New("CANARY_DESIGN_UNVERIFIED")
	}
	return nil
}

// CanaryCapture is a non-secret envelope for manually supplied response evidence.
type CanaryCapture struct {
	EvidenceClass      string          `json:"evidence_class"`
	CaseID             string          `json:"case_id"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         time.Time       `json:"finished_at"`
	ChannelID          int             `json:"channel_id"`
	Model              string          `json:"model"`
	Endpoint           string          `json:"endpoint"`
	HTTPStatus         int             `json:"http_status"`
	GatewayTrace       string          `json:"x-gateway-trace,omitempty"`
	RequestID          string          `json:"request_id,omitempty"`
	CatalogHash        string          `json:"catalog_hash"`
	SchemaVersion      string          `json:"schema_version"`
	ETag               string          `json:"etag"`
	BillingFeatures    []string        `json:"billing_features"`
	EstimatedMaxPoints string          `json:"estimated_max_points,omitempty"`
	TerminalStatus     string          `json:"terminal_status"`
	ObservedFacts      map[string]any  `json:"observed_facts,omitempty"`
	Response           json.RawMessage `json:"response"`
	VerificationResult string          `json:"verification_result,omitempty"`
}

// RedactCanaryJSON removes credential and identity fields recursively and
// strips URL query strings; it does not claim to prove arbitrary text is safe.
func RedactCanaryJSON(raw []byte, credential string) ([]byte, error) {
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var redact func(any) any
	redact = func(v any) any {
		switch value := v.(type) {
		case map[string]any:
			clean := make(map[string]any, len(value))
			for key, child := range value {
				lower := strings.ToLower(key)
				if slices.Contains([]string{"authorization", "api_key", "apikey", "cookie", "set-cookie", "password", "token", "secret", "email", "phone", "name", "reference_audio_url", "prompt", "text", "content", "transcript", "b64_json", "audio", "image", "video", "user"}, lower) || strings.Contains(lower, "api-key") || strings.Contains(lower, "credential") || strings.Contains(lower, "auth") || strings.Contains(lower, "cookie") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "signed") || strings.Contains(lower, "private") || strings.HasSuffix(lower, "_key") {
					clean[key] = "[REDACTED]"
					continue
				}
				clean[key] = redact(child)
			}
			return clean
		case []any:
			clean := make([]any, len(value))
			for i := range value {
				clean[i] = redact(value[i])
			}
			return clean
		case string:
			if strings.HasPrefix(strings.ToLower(value), "bearer ") || strings.HasPrefix(value, "sk-") {
				return "[REDACTED]"
			}
			if credential != "" && strings.Contains(value, credential) {
				return "[REDACTED]"
			}
			if strings.HasPrefix(value, "/Users/") || strings.HasPrefix(value, "/home/") || strings.HasPrefix(value, "file:") {
				return "[REDACTED_PATH]"
			}
			if parsed, err := url.Parse(value); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
				return "[REDACTED_URL]"
			}
		}
		return v
	}
	return common.Marshal(redact(value))
}

func WriteCanaryCapture(dir, invocationID string, capture CanaryCapture, credential string) (string, error) {
	if invocationID == "" || strings.ContainsAny(invocationID, `/\\.`) {
		return "", errors.New("invalid invocation ID")
	}
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "new-api-dflop-canary")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("capture directory must be private (0700)")
	}
	response, err := RedactCanaryJSON(capture.Response, credential)
	if err != nil {
		return "", err
	}
	capture.Response = response
	capture.EvidenceClass = "UNKNOWN_ORIGIN"
	encoded, err := common.Marshal(capture)
	if err != nil {
		return "", err
	}
	encoded, err = RedactCanaryJSON(encoded, credential)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, invocationID+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(encoded); err != nil {
		return "", err
	}
	return path, nil
}

// CanaryInvocation stores a provider task ID independently of any credential.
// A resumed invocation must poll this task; it must never submit again.
type CanaryInvocation struct {
	ID          string    `json:"id"`
	CaseID      string    `json:"case_id"`
	TaskID      string    `json:"task_id"`
	Model       string    `json:"model"`
	Endpoint    string    `json:"endpoint"`
	ChannelID   int       `json:"channel_id"`
	SubmittedAt time.Time `json:"submitted_at"`
}

func (i CanaryInvocation) NextAction() string {
	if i.TaskID != "" {
		return "POLL_EXISTING_TASK"
	}
	return "SUBMIT_ONLY_AFTER_AUTHORIZATION"
}

func WriteCanaryInvocation(dir string, invocation CanaryInvocation) (string, error) {
	if invocation.ID == "" || strings.ContainsAny(invocation.ID, `/\\.`) || invocation.CaseID == "" || invocation.TaskID == "" || invocation.ChannelID <= 0 {
		return "", errors.New("incomplete canary invocation")
	}
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "new-api-dflop-canary", "invocations")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("invocation directory must be private (0700)")
	}
	encoded, err := common.Marshal(invocation)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, invocation.ID+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	_, err = file.Write(encoded)
	return path, err
}

func ReadCanaryInvocation(path string) (CanaryInvocation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CanaryInvocation{}, err
	}
	var invocation CanaryInvocation
	if err := common.Unmarshal(data, &invocation); err != nil {
		return CanaryInvocation{}, err
	}
	if invocation.ID == "" || invocation.TaskID == "" || invocation.CaseID == "" || invocation.ChannelID <= 0 {
		return CanaryInvocation{}, errors.New("incomplete canary invocation")
	}
	return invocation, nil
}

func BillingShapeHash(raw []byte) (string, error) {
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	allowed := map[string]bool{"status": true, "state": true, "task_id": true, "voice_id": true, "characters": true, "duration_sec": true, "resolution": true, "service_tier": true, "usage": true, "prompt_tokens": true, "completion_tokens": true, "input_tokens": true, "output_tokens": true, "num_server_side_tools_used": true, "tracks": true, "data": true, "url": true, "b64_json": true, "model": true, "cost": true, "cost_usd": true, "points": true}
	var project func(any) any
	project = func(v any) any {
		switch typed := v.(type) {
		case map[string]any:
			result := map[string]any{}
			for key, child := range typed {
				if !allowed[key] {
					continue
				}
				if key == "url" || key == "b64_json" || key == "voice_id" || key == "task_id" || key == "model" {
					text, ok := child.(string)
					result[key] = ok && text != ""
					continue
				}
				result[key] = project(child)
			}
			return result
		case []any:
			if len(typed) == 0 {
				return map[string]any{"array": "empty"}
			}
			return map[string]any{"array_item": project(typed[0])}
		case string:
			return "string"
		case float64:
			return "number"
		case bool:
			return "boolean"
		case nil:
			return "null"
		default:
			return v
		}
	}
	encoded, err := common.Marshal(project(value))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}
