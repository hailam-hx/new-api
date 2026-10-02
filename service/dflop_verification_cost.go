package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/shopspring/decimal"
)

type DFLOPVerificationTarget struct {
	Model               string `json:"model"`
	Protocol            string `json:"protocol"`
	Mode                string `json:"mode"`
	FixtureID           string `json:"fixture_id"`
	PricingSnapshotHash string `json:"pricing_snapshot_hash"`
	BillingExprHash     string `json:"billing_expr_hash"`
	RequestBodyHash     string `json:"request_body_hash"`
}

type DFLOPVerificationAuthorization struct {
	Approved               bool                      `json:"approved"`
	ChannelID              int                       `json:"channel_id"`
	FundingUserID          int                       `json:"funding_user_id"`
	CatalogHash            string                    `json:"catalog_hash"`
	CredentialFingerprint  string                    `json:"credential_fingerprint"`
	Models                 []string                  `json:"models"`
	Targets                []DFLOPVerificationTarget `json:"targets"`
	MaxRequests            int                       `json:"max_requests"`
	MaxCostPerRequest      map[string]string         `json:"max_cost_per_request"`
	MaxTotalProviderPoints string                    `json:"max_total_provider_points"`
	ExpiresAt              int64                     `json:"expires_at"`
	ApprovedBy             string                    `json:"approved_by"`
	ApprovalReference      string                    `json:"approval_reference"`
}

// ValidateDFLOPVerificationAuthorization does not infer consent from a test,
// catalog access, a family name or a previous invocation. Persistence also
// atomically claims the cumulative hold/count before any paid submission.
func ValidateDFLOPVerificationAuthorization(a DFLOPVerificationAuthorization, run model.RuntimeVerificationRun, item model.RuntimeVerificationItem, maximum string, now time.Time) error {
	if !a.Approved || a.ApprovedBy == "" || a.ApprovalReference == "" || a.ExpiresAt <= now.Unix() {
		return errors.New("PAID_AUTHORIZATION_REQUIRED")
	}
	if a.ChannelID != run.ChannelID || a.FundingUserID != run.FundingUserID || a.CatalogHash != run.CatalogHash || a.CredentialFingerprint != run.CredentialFingerprint {
		return errors.New("DRIFT_REVIEW_REQUIRED")
	}
	if a.MaxRequests <= 0 || run.PaidRequests >= a.MaxRequests {
		return errors.New("PAID_REQUEST_LIMIT")
	}
	if !slices.Contains(a.Models, item.Model) || !slices.ContainsFunc(a.Targets, func(target DFLOPVerificationTarget) bool {
		return target.Model == item.Model && target.Protocol == item.Protocol && target.Mode == item.Mode && target.FixtureID == item.FixtureID && target.PricingSnapshotHash == item.PricingSnapshotHash && target.BillingExprHash == item.BillingExprHash && target.RequestBodyHash == item.RequestBodyHash
	}) {
		return errors.New("PAID_TARGET_NOT_AUTHORIZED")
	}
	maximumCost, err := decimal.NewFromString(maximum)
	perRequest, perErr := decimal.NewFromString(a.MaxCostPerRequest[item.Model])
	total, totalErr := decimal.NewFromString(a.MaxTotalProviderPoints)
	if err != nil || perErr != nil || totalErr != nil || !maximumCost.IsPositive() || !perRequest.IsPositive() || !total.IsPositive() || maximumCost.GreaterThan(perRequest) {
		return errors.New("PAID_BUDGET_INVALID")
	}
	spent, spentErr := decimal.NewFromString(run.ActualProviderCost)
	if run.ActualProviderCost == "" {
		spent, spentErr = decimal.Zero, nil
	}
	held, heldErr := decimal.NewFromString(run.ReservedProviderCost)
	if run.ReservedProviderCost == "" {
		held, heldErr = decimal.Zero, nil
	}
	if spentErr != nil || heldErr != nil || spent.IsNegative() || held.IsNegative() || spent.Add(held).Add(maximumCost).GreaterThan(total) {
		return errors.New("PAID_BUDGET_EXCEEDED")
	}
	return nil
}

func verificationQuantity(facts map[string]any, key string, ceiling int64, integral bool) (decimal.Decimal, error) {
	value, present := facts[key]
	if !present || value == nil {
		return decimal.Zero, fmt.Errorf("BILLING_QUANTITY_MISSING: %s", key)
	}
	text := fmt.Sprint(value)
	if number, ok := value.(float64); ok {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return decimal.Zero, errors.New("BILLING_QUANTITY_INVALID")
		}
		text = strconv.FormatFloat(number, 'f', -1, 64)
	}
	quantity, err := decimal.NewFromString(text)
	if err != nil || quantity.IsNegative() || quantity.GreaterThan(decimal.NewFromInt(ceiling)) || integral && !quantity.Equal(quantity.Truncate(0)) {
		return decimal.Zero, fmt.Errorf("BILLING_QUANTITY_INVALID: %s", key)
	}
	return quantity, nil
}

func verificationRate(item dflop.Item, key string) (decimal.Decimal, error) {
	price, present := item.Prices[key]
	if !present || price.SourcePriceKind != "AUTHENTICATED_EFFECTIVE_PRICE" {
		return decimal.Zero, fmt.Errorf("PROVIDER_DOCUMENTATION_CONFLICT: authenticated rate %s unavailable", key)
	}
	value := price.EffectiveCredits
	if value == "" {
		value = price.Credits
	}
	rate, err := decimal.NewFromString(value)
	if err != nil || rate.IsNegative() {
		return decimal.Zero, errors.New("PROVIDER_PRICE_INVALID")
	}
	return rate, nil
}

// DFLOPVerificationPoints audits production-normalized quantities against the
// frozen authenticated tariff. This is not another usage extractor or wallet
// evaluator: authoritative facts come only from the production task hooks.
func DFLOPVerificationPoints(item dflop.Item, facts map[string]any) (decimal.Decimal, error) {
	if item.PriceSemantics.SourcePriceKind != "AUTHENTICATED_EFFECTIVE_PRICE" {
		return decimal.Zero, errors.New("AUTHENTICATED_PRICING_REQUIRED")
	}
	var source struct {
		FreeInputImages *int `json:"free_input_images"`
	}
	if len(item.Raw) > 0 && common.Unmarshal(item.Raw, &source) != nil {
		return decimal.Zero, errors.New("PROVIDER_CONTRACT_INVALID")
	}
	var quantity, rate decimal.Decimal
	var err error
	switch {
	case slices.Contains(item.BillingFeatures, "per_image"):
		quantity, err = verificationQuantity(facts, "image_count", int64(dto.MaxImageN), true)
		if err != nil || !quantity.IsPositive() {
			return decimal.Zero, errors.New("BILLING_QUANTITY_MISSING: image_count")
		}
		rate, err = verificationRate(item, "price_per_image")
		if err != nil {
			return decimal.Zero, err
		}
		cost := quantity.Mul(rate)
		if slices.Contains(item.BillingFeatures, "image_size_bands") {
			return decimal.Zero, errors.New("MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS")
		}
		if !slices.Contains(item.BillingFeatures, "input_images") {
			return cost, nil
		}
		input, err := verificationQuantity(facts, "input_image_count", int64(dto.MaxImageN), true)
		if err != nil {
			return decimal.Zero, err
		}
		free := 0
		if source.FreeInputImages != nil {
			free = *source.FreeInputImages
		}
		if free < 0 || free > dto.MaxImageN {
			return decimal.Zero, errors.New("PROVIDER_CONTRACT_INVALID")
		}
		rate, err = verificationRate(item, "price_per_input_image")
		return cost.Add(decimal.Max(input.Sub(decimal.NewFromInt(int64(free))), decimal.Zero).Mul(rate)), err
	case item.EndpointType == "tts_synthesize":
		quantity, err = verificationQuantity(facts, "character_count", 5000, true)
		if err != nil {
			return decimal.Zero, err
		}
		if _, present := facts["characters"]; present {
			characters, charactersErr := verificationQuantity(facts, "characters", 5000, true)
			if charactersErr != nil || !characters.Equal(quantity) {
				return decimal.Zero, errors.New("BILLING_QUANTITY_INVALID: speech character facts disagree")
			}
		}
		rate, err = verificationRate(item, "price_per_tts_char")
	case item.EndpointType == "music_generations":
		quantity, err = verificationQuantity(facts, "generation_count", 1, true)
		if err != nil || !quantity.Equal(decimal.NewFromInt(1)) {
			return decimal.Zero, errors.New("BILLING_QUANTITY_MISSING: generation_count")
		}
		rate, err = verificationRate(item, "price_per_music_generation")
	case item.EndpointType == "voice_clone" || item.EndpointType == "avatar_create" || slices.Contains(item.BillingFeatures, "video_task"):
		quantity, err = verificationQuantity(facts, "count", 1, true)
		if err != nil || !quantity.Equal(decimal.NewFromInt(1)) {
			return decimal.Zero, errors.New("BILLING_QUANTITY_MISSING: count")
		}
		key := "price_per_video_task"
		if item.EndpointType == "voice_clone" {
			key = "price_per_voice_clone"
		}
		if item.EndpointType == "avatar_create" {
			key = "price_per_avatar"
		}
		rate, err = verificationRate(item, key)
	case slices.Contains(item.BillingFeatures, "video_token"):
		quantity, err = verificationQuantity(facts, "completion_tokens", math.MaxInt32, true)
		if err != nil {
			return decimal.Zero, err
		}
		resolution, ok := facts["resolution"].(string)
		mode, modeOK := facts["input_mode"].(string)
		if !ok || !modeOK || mode != "default" && mode != "with_video_input" {
			return decimal.Zero, errors.New("BILLING_QUANTITY_MISSING: selected token tier")
		}
		key := mode + "@" + resolution
		if item.ModelID == "doubao-seedance-2.5" && (resolution == "480p" || resolution == "720p") {
			key = mode
		}
		rate, err = verificationRate(item, "video_token_tier:"+key)
		if err != nil {
			return decimal.Zero, err
		}
		cost := quantity.Mul(rate).Div(decimal.NewFromInt(1000000))
		if !slices.Contains(item.BillingFeatures, "video_two_stage") {
			return cost, nil
		}
		seconds, err := verificationQuantity(facts, "duration_sec", relaycommon.MaxTaskDurationSeconds, false)
		if err != nil {
			return decimal.Zero, err
		}
		rate, err = verificationRate(item, "video_second_stage:"+resolution)
		return cost.Add(seconds.Mul(rate)), err
	case item.EndpointType == "videos_generations":
		quantity, err = verificationQuantity(facts, "duration_sec", relaycommon.MaxTaskDurationSeconds, false)
		if err != nil || !quantity.IsPositive() {
			return decimal.Zero, errors.New("BILLING_QUANTITY_MISSING: duration_sec")
		}
		if slices.Contains(item.BillingFeatures, "video_input_seconds") {
			if !slices.Contains([]string{"wan3.0-video", "wan3.0-video-prime"}, item.ModelID) {
				return decimal.Zero, errors.New("PROVIDER_DOCUMENTATION_CONFLICT")
			}
			input, err := verificationQuantity(facts, "input_video_duration_sec", 15, false)
			if err != nil {
				return decimal.Zero, err
			}
			quantity = quantity.Add(input)
		}
		key := "price_per_video_second"
		if slices.Contains(item.BillingFeatures, "video_tiers") {
			resolution, ok := facts["resolution"].(string)
			if !ok || resolution == "" {
				return decimal.Zero, errors.New("BILLING_QUANTITY_MISSING: resolution")
			}
			key = "video_tier:" + resolution
		}
		rate, err = verificationRate(item, key)
	default:
		return decimal.Zero, errors.New("PROVIDER_CONTRACT_REQUIRED")
	}
	if err != nil {
		return decimal.Zero, err
	}
	return quantity.Mul(rate), nil
}

type DFLOPVerificationLedgerResult struct {
	Result             string            `json:"result"`
	CorrelationQuality string            `json:"correlation_quality"`
	ProviderCostPoints string            `json:"provider_cost_points,omitempty"`
	ProviderUnitCount  string            `json:"provider_unit_count,omitempty"`
	ProviderStatus     string            `json:"provider_status,omitempty"`
	ProviderErrorCode  string            `json:"provider_error_code,omitempty"`
	LedgerID           string            `json:"ledger_id,omitempty"`
	LedgerFacts        map[string]string `json:"ledger_facts,omitempty"`
}

// Conflicting identities fail even when another ID matches. Model/time hints
// cannot authorize settlement. Provider precision is never guessed.
func ReconcileDFLOPVerificationLedger(item model.RuntimeVerificationItem, raw []byte, expected string) DFLOPVerificationLedgerResult {
	result := DFLOPVerificationLedgerResult{Result: "INSUFFICIENT_EVIDENCE", CorrelationQuality: "NONE"}
	var row map[string]common.RawMessage
	if common.Unmarshal(raw, &row) != nil || common.JsonRawMessageToString(row["model"]) != item.Model {
		return result
	}
	for _, pair := range []struct {
		value   string
		keys    []string
		quality string
	}{
		{item.TaskID, []string{"task_id"}, "EXACT_TASK_ID"},
		{item.RequestID, []string{"request_id"}, "EXACT_REQUEST_ID"},
		{item.TraceID, []string{"x_gateway_trace", "x-gateway-trace"}, "EXACT_TRACE_ID"},
	} {
		for _, key := range pair.keys {
			var value string
			if common.Unmarshal(row[key], &value) == nil && value != "" {
				if key == "request_id" && item.TraceID != "" && value == item.TraceID {
					if result.CorrelationQuality == "NONE" {
						result.CorrelationQuality = "EXACT_TRACE_ID"
					}
					continue
				}
				if pair.value != "" && value != pair.value {
					return DFLOPVerificationLedgerResult{Result: "INSUFFICIENT_EVIDENCE", CorrelationQuality: "NONE"}
				}
				if pair.value != "" && result.CorrelationQuality == "NONE" {
					result.CorrelationQuality = pair.quality
				}
			}
		}
	}
	if result.CorrelationQuality == "NONE" {
		return result
	}
	if id := common.JsonRawMessageToString(row["id"]); len(id) <= 128 && verificationSecretSafeID(id) {
		result.LedgerID = id
	}
	result.LedgerFacts = map[string]string{}
	for _, key := range []string{"input_tokens", "output_tokens", "cached_tokens", "unit_count", "cost", "latency_ms", "latency", "attempt_count"} {
		if value, err := verificationLedgerDecimal(row[key]); err == nil {
			result.LedgerFacts[key] = value.String()
		}
	}
	var attempts []common.RawMessage
	if common.Unmarshal(row["attempts"], &attempts) == nil && attempts != nil {
		result.LedgerFacts["attempt_count"] = strconv.Itoa(len(attempts))
	}
	status := common.JsonRawMessageToString(row["status"])
	result.ProviderStatus = status
	if !slices.Contains([]string{"success", "error", "interrupted", "rejected", "succeeded", "completed", "failed", "cancelled", "expired"}, status) {
		return result
	}
	if currency := common.JsonRawMessageToString(row["currency"]); currency != "" && currency != "points" {
		return result
	}
	cost, err := verificationLedgerDecimal(row["cost"])
	if err != nil {
		return result
	}
	result.ProviderCostPoints = cost.String()
	if count, exists := row["unit_count"]; exists && string(count) != "null" {
		quantity, err := verificationLedgerDecimal(count)
		if err != nil {
			return result
		}
		result.ProviderUnitCount = quantity.String()
	}
	want, err := decimal.NewFromString(expected)
	if err != nil || want.IsNegative() {
		return result
	}
	if quantityResult := reconcileDFLOPVerificationQuantities(item, row); quantityResult != "EXACT_MATCH" {
		result.Result = quantityResult
		return result
	}
	result.Result = "LEDGER_MISMATCH"
	if cost.Equal(want) {
		result.Result = "EXACT_MATCH"
		if cost.IsZero() && slices.Contains([]string{"error", "interrupted", "rejected", "failed", "cancelled", "expired"}, status) {
			if code := common.JsonRawMessageToString(row["error_code"]); code == "NO_HEALTHY_DEPLOYMENT" {
				result.ProviderErrorCode = code
			}
		}
	}
	return result
}

// Decimal ledger literals are read before any conversion to an IEEE-754 value.
func verificationLedgerDecimal(raw common.RawMessage) (decimal.Decimal, error) {
	if kind := common.GetJsonType(raw); kind != "number" && kind != "string" {
		return decimal.Zero, errors.New("PROVIDER_QUANTITY_INVALID")
	}
	value, err := decimal.NewFromString(common.JsonRawMessageToString(raw))
	if err != nil || value.IsNegative() {
		return decimal.Zero, errors.New("PROVIDER_QUANTITY_INVALID")
	}
	return value, nil
}

func verificationLedgerUsage(raw string) (map[string]common.RawMessage, error) {
	facts := make(map[string]common.RawMessage)
	if raw == "" {
		return facts, nil
	}
	if err := common.UnmarshalJsonStr(raw, &facts); err != nil {
		return nil, err
	}
	if usage, exists := facts["usage"]; exists {
		var nested map[string]common.RawMessage
		if err := common.Unmarshal(usage, &nested); err != nil {
			return nil, err
		}
		for key, value := range nested {
			if prior, exists := facts[key]; exists {
				left, leftErr := verificationLedgerDecimal(prior)
				right, rightErr := verificationLedgerDecimal(value)
				if leftErr != nil || rightErr != nil || !left.Equal(right) {
					return nil, errors.New("PROVIDER_USAGE_CONFLICT")
				}
			}
			facts[key] = value
		}
		delete(facts, "usage")
	}
	return facts, nil
}

// Cost equality alone cannot prove that the terminal quantities were parsed
// correctly. Compare terminal facts, production facts, and ledger units exactly.
func reconcileDFLOPVerificationQuantities(item model.RuntimeVerificationItem, row map[string]common.RawMessage) string {
	normalized, err := verificationLedgerUsage(item.NormalizedUsageJSON)
	if err != nil {
		return "INSUFFICIENT_EVIDENCE"
	}
	provider, err := verificationLedgerUsage(item.ProviderUsageJSON)
	if err != nil {
		return "INSUFFICIENT_EVIDENCE"
	}
	if normalized["completion_tokens"] != nil && row["output_tokens"] == nil && row["completion_tokens"] == nil {
		return "INSUFFICIENT_EVIDENCE"
	}
	for _, pair := range []struct {
		normalized string
		provider   []string
		ledger     []string
	}{
		{"image_count", []string{"generated_images", "output_image_count"}, []string{"image_count", "generated_images", "output_image_count"}},
		{"input_image_count", []string{"input_image_count"}, []string{"input_image_count"}},
		{"character_count", []string{"characters", "character_count"}, []string{"characters", "character_count"}},
		{"completion_tokens", []string{"completion_tokens", "output_tokens"}, []string{"output_tokens", "completion_tokens"}},
		{"prompt_tokens", []string{"prompt_tokens", "input_tokens"}, []string{"input_tokens", "prompt_tokens"}},
		{"total_tokens", []string{"total_tokens"}, []string{"total_tokens"}},
		{"cached_tokens", []string{"cached_tokens"}, []string{"cached_tokens"}},
		{"input_video_duration_sec", []string{"input_video_duration_sec"}, []string{"input_video_duration_sec"}},
		{"duration_sec", []string{"duration_sec"}, []string{"duration_sec"}},
		{"generation_count", []string{"generation_count"}, []string{"generation_count"}},
		{"count", []string{"count"}, []string{"count"}},
	} {
		if pair.normalized == "duration_sec" && provider["source_duration_sec"] != nil {
			pair.provider = []string{"source_duration_sec"}
		}
		raw, present := normalized[pair.normalized]
		if !present {
			continue
		}
		quantity, err := verificationLedgerDecimal(raw)
		if err != nil {
			return "PARSER_MISMATCH"
		}
		for _, key := range pair.provider {
			if raw, present := provider[key]; present {
				value, err := verificationLedgerDecimal(raw)
				if err != nil || !value.Equal(quantity) {
					return "PARSER_MISMATCH"
				}
			}
		}
		for _, key := range pair.ledger {
			if raw, present := row[key]; present {
				value, err := verificationLedgerDecimal(raw)
				if err != nil || !value.Equal(quantity) {
					return "PROVIDER_USAGE_MISMATCH"
				}
			}
		}
	}
	var units []common.RawMessage
	if item.ProviderUnitCount != "" {
		units = append(units, common.RawMessage(item.ProviderUnitCount))
	}
	if provider["unit_count"] != nil && string(provider["unit_count"]) != "null" {
		units = append(units, provider["unit_count"])
	}
	unitType := common.JsonRawMessageToString(row["unit_type"])
	if provider["unit_type"] != nil && common.JsonRawMessageToString(provider["unit_type"]) != unitType {
		return "PROVIDER_USAGE_MISMATCH"
	}
	expectedType := ""
	switch {
	case normalized["image_count"] != nil:
		expectedType = "image"
	case normalized["generation_count"] != nil:
		expectedType = "music"
	case normalized["character_count"] != nil:
		expectedType = "audio"
	case normalized["count"] != nil:
		expectedType = "video"
		if item.Model == "voice-clone-pro" || item.Endpoint == "/v1/audio/voices" {
			expectedType = "voice"
		}
		if item.Model == "dh-avatar-create" || item.Endpoint == "/v1/videos/avatars" {
			expectedType = "avatar"
		}
	case normalized["duration_sec"] != nil && normalized["completion_tokens"] == nil:
		expectedType = "video"
	}
	if expectedType != "" {
		if unitType == "" || unitType == "null" {
			return "INSUFFICIENT_EVIDENCE"
		}
		if expectedType != unitType {
			return "PROVIDER_USAGE_MISMATCH"
		}
	}
	unitKey := map[string]string{"image": "image_count", "music": "generation_count", "audio": "character_count", "voice": "count", "avatar": "count"}[unitType]
	if unitType == "video" {
		unitKey = "duration_sec"
		if normalized["count"] != nil {
			unitKey = "count"
		}
	}
	if raw, present := normalized[unitKey]; present {
		quantity, err := verificationLedgerDecimal(raw)
		if err != nil {
			return "PARSER_MISMATCH"
		}
		if unitKey == "duration_sec" && normalized["input_video_duration_sec"] != nil {
			input, err := verificationLedgerDecimal(normalized["input_video_duration_sec"])
			if err != nil {
				return "PARSER_MISMATCH"
			}
			quantity = quantity.Add(input)
		}
		units = append(units, common.RawMessage(quantity.String()))
	}
	if len(units) == 0 {
		return "EXACT_MATCH"
	}
	ledgerUnits, err := verificationLedgerDecimal(row["unit_count"])
	if err != nil {
		return "INSUFFICIENT_EVIDENCE"
	}
	for _, raw := range units {
		quantity, err := verificationLedgerDecimal(raw)
		if err != nil {
			return "INSUFFICIENT_EVIDENCE"
		}
		if !quantity.Equal(ledgerUnits) {
			return "PROVIDER_USAGE_MISMATCH"
		}
	}
	return "EXACT_MATCH"
}

func verificationHash(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

// Kept separate from monetary values: IDs contain no raw credentials.
func verificationIntentKey(runID, itemID int64, bodyHash string) string {
	return "dflop-verify-" + verificationHash([]byte(fmt.Sprintf("%d:%d:%s", runID, itemID, bodyHash)))
}

func verificationSecretSafeID(value string) bool {
	return len(value) <= 200 && !strings.ContainsAny(value, "\r\n\t /\\?%#") && !strings.Contains(value, ":")
}
