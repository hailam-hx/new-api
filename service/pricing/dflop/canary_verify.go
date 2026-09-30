package dflop

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/shopspring/decimal"
)

type CanaryVerification struct {
	CaseID           string         `json:"case_id"`
	Status           string         `json:"status"`
	ObservedFacts    map[string]any `json:"observed_facts,omitempty"`
	SchemaHash       string         `json:"schema_hash"`
	ExpectedPoints   string         `json:"expected_points,omitempty"`
	ProviderPoints   string         `json:"provider_points,omitempty"`
	CostStatus       string         `json:"cost_status,omitempty"`
	Note             string         `json:"note"`
	ProductionReplay any            `json:"production_replay,omitempty"`
}

// CompareCanaryProviderPoints uses the effective catalog unit price retained
// in the plan. Exact decimal comparison intentionally does not hide variance.
// Cases with unresolved quantity semantics do not produce expected cost.
func CompareCanaryProviderPoints(c CanaryCase, verification *CanaryVerification, providerPoints string) error {
	if verification == nil || verification.Status != "RUNTIME_FACT_VERIFIED" || c.UnitPricePoints == "" {
		return errors.New("CANARY_INCOMPLETE")
	}
	unitPrice, err := decimal.NewFromString(c.UnitPricePoints)
	if err != nil || unitPrice.IsNegative() {
		return errors.New("invalid source-key unit price")
	}
	quantity := int64(1)
	switch {
	case c.ID == "tts-async":
		fact, ok := verification.ObservedFacts["characters"].(int64)
		if !ok || fact < 0 {
			return errors.New("CANARY_INCOMPLETE")
		}
		quantity = fact
	case strings.HasPrefix(c.ID, "output-count-"):
		fact, ok := verification.ObservedFacts["usable_image_outputs"].(int)
		if !ok || fact <= 0 {
			return errors.New("CANARY_INCOMPLETE")
		}
		quantity = int64(fact)
	case c.ID == "suno-generation":
		if verification.ObservedFacts["generations"] != 1 {
			return errors.New("CANARY_INCOMPLETE")
		}
	case c.ID == "voice-clone" || strings.HasPrefix(c.ID, "task-"):
		// One successful operation; task families still require a validated
		// binding before this can establish settlement evidence.
		if c.EstimatedMaxPoints == "" {
			return errors.New("CANARY_INCOMPLETE")
		}
	default:
		return errors.New("CANARY_INCOMPLETE")
	}
	verification.ExpectedPoints = unitPrice.Mul(decimal.NewFromInt(quantity)).String()
	verification.CostStatus = "EXPECTED_ONLY"
	if providerPoints == "" {
		return nil
	}
	reported, err := decimal.NewFromString(providerPoints)
	if err != nil || reported.IsNegative() {
		return errors.New("invalid provider points")
	}
	verification.ProviderPoints = reported.String()
	if !reported.Equal(unitPrice.Mul(decimal.NewFromInt(quantity))) {
		verification.CostStatus = "UNEXPLAINED_PROVIDER_CHARGE"
		return nil
	}
	verification.CostStatus = "PROVIDER_POINTS_MATCH"
	return nil
}

// VerifyCanaryFixture validates billing-relevant facts offline. It accepts a
// raw response or a CanaryCapture envelope. Synthetic data proves only this
// verifier's behavior, never provider semantics or model eligibility.
func VerifyCanaryFixture(caseID string, raw []byte) (CanaryVerification, error) {
	var envelope CanaryCapture
	if err := common.Unmarshal(raw, &envelope); err == nil && envelope.CaseID != "" && len(envelope.Response) > 0 {
		raw = envelope.Response
	}
	var response map[string]json.RawMessage
	if err := common.Unmarshal(raw, &response); err != nil || response == nil {
		return CanaryVerification{}, errors.New("invalid response fixture")
	}
	hash, err := BillingShapeHash(raw)
	if err != nil {
		return CanaryVerification{}, err
	}
	result := CanaryVerification{CaseID: caseID, Status: "RUNTIME_FACT_MISSING", ObservedFacts: map[string]any{}, SchemaHash: hash, Note: "Offline fixture validation only; real route, settlement and provider charge are not established"}
	read := func(name string, target any) bool {
		field, ok := response[name]
		return ok && common.Unmarshal(field, target) == nil
	}
	var status string
	read("status", &status)
	if status == "" {
		read("state", &status)
	}
	status = strings.ToLower(status)
	if strings.HasPrefix(caseID, "grok-tool-") {
		var terminal struct {
			Usage    dto.Usage `json:"usage"`
			Response struct {
				Usage dto.Usage `json:"usage"`
			} `json:"response"`
		}
		if err := common.Unmarshal(raw, &terminal); err != nil {
			return result, err
		}
		usage := terminal.Usage
		if caseID == "grok-tool-responses" {
			usage = terminal.Response.Usage
		}
		if usage.NumServerSideToolsUsed != nil && *usage.NumServerSideToolsUsed >= 1 {
			result.ObservedFacts["num_server_side_tools_used"] = *usage.NumServerSideToolsUsed
			result.ObservedFacts["input_tokens"] = max(usage.PromptTokens, usage.InputTokens)
			result.ObservedFacts["output_tokens"] = max(usage.CompletionTokens, usage.OutputTokens)
			result.Status = "RUNTIME_FACT_VERIFIED"
		}
		return result, nil
	}
	if caseID == "tts-async" {
		var taskID, model, audioURL string
		var characters, duration json.Number
		read("id", &taskID)
		read("model", &model)
		read("audio_url", &audioURL)
		if status == "failed" {
			result.Status = "EXECUTED_FAILURE"
			return result, nil
		}
		if taskID != "" && model == "voice-tts-pro" && audioURL != "" && status == "succeeded" && read("characters", &characters) {
			if count, err := characters.Int64(); err == nil && count >= 0 && count <= 6 {
				result.ObservedFacts["task_id"] = taskID
				result.ObservedFacts["characters"] = count
				if read("duration_sec", &duration) {
					result.ObservedFacts["duration_sec"] = duration.String()
				}
				result.Status = "RUNTIME_FACT_VERIFIED"
			} else if err == nil && count > 6 {
				result.Status = "CANARY_COST_MODEL_INVALID"
			}
		}
		return result, nil
	}
	if caseID == "suno-generation" {
		var taskID, model string
		var tracks []struct {
			ClipID   string      `json:"clip_id"`
			AudioURL string      `json:"audio_url"`
			Duration json.Number `json:"duration_sec"`
		}
		read("id", &taskID)
		read("model", &model)
		if status == "failed" || status == "expired" {
			result.Status = "EXECUTED_FAILURE"
			return result, nil
		}
		if taskID != "" && strings.HasPrefix(model, "suno-") && status == "succeeded" && read("tracks", &tracks) {
			valid := 0
			for _, track := range tracks {
				if track.ClipID != "" && track.AudioURL != "" {
					valid++
				}
			}
			if valid == 0 {
				return result, nil
			}
			result.ObservedFacts["tracks"] = valid
			result.ObservedFacts["generations"] = 1
			result.Status = "RUNTIME_FACT_VERIFIED"
			result.Note = "Offline generation unit proof only; synthetic fixture cannot establish provider settlement"
		}
		return result, nil
	}
	if caseID == "voice-clone" {
		var voiceID string
		if read("voice_id", &voiceID) && voiceID != "" && status == "ready" {
			result.ObservedFacts["voice_id_present"] = true
			result.Status = "RUNTIME_FACT_VERIFIED"
		} else if status == "failed" {
			result.Status = "EXECUTED_FAILURE"
		}
		return result, nil
	}
	if strings.HasPrefix(caseID, "output-count-") {
		var outputs []map[string]json.RawMessage
		if read("data", &outputs) && (status == "succeeded" || status == "success") {
			urls, b64s := 0, 0
			for _, output := range outputs {
				var payload string
				if field, ok := output["url"]; ok && common.Unmarshal(field, &payload) == nil && payload != "" {
					urls++
				}
				if field, ok := output["b64_json"]; ok && common.Unmarshal(field, &payload) == nil && payload != "" {
					b64s++
				}
			}
			usable := max(urls, b64s)
			result.ObservedFacts["usable_image_outputs"] = usable
			if usable > 0 {
				result.Status = "RUNTIME_FACT_VERIFIED"
			}
		}
		return result, nil
	}
	if strings.HasPrefix(caseID, "seedance-") {
		var usage struct {
			CompletionTokens json.Number `json:"completion_tokens"`
		}
		var duration json.Number
		if read("usage", &usage) && usage.CompletionTokens != "" && read("duration_sec", &duration) && (status == "succeeded" || status == "success") {
			if tokens, err := usage.CompletionTokens.Int64(); err == nil && tokens >= 0 {
				result.ObservedFacts["completion_tokens"] = tokens
				result.ObservedFacts["duration_sec"] = duration.String()
				result.Status = "RUNTIME_FACT_VERIFIED"
			}
		}
		return result, nil
	}
	return result, nil
}
