package dflop

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// ProviderContractAudit is evidence only. It never changes pricing or coverage.
type ProviderContractAudit struct {
	CatalogHash          string                          `json:"catalog_hash"`
	PreviousCatalogHash  string                          `json:"previous_catalog_hash,omitempty"`
	SchemaVersion        string                          `json:"schema_version"`
	CallableCount        int                             `json:"callable_count"`
	Models               []ProviderContractModelEvidence `json:"models"`
	Blockers             []ProviderContractBlocker       `json:"blockers"`
	Conflicts            []string                        `json:"conflicts"`
	FreshPreviewRequired bool                            `json:"fresh_preview_required"`
	AutoPromotes         bool                            `json:"auto_promotes"`
	PricingApplied       bool                            `json:"pricing_applied"`
	AutoApply            bool                            `json:"auto_apply"`
}

type ProviderContractModelEvidence struct {
	Model              string          `json:"model"`
	Callable           bool            `json:"callable"`
	PricingKeys        []string        `json:"raw_pricing_keys"`
	BillingFeatures    []string        `json:"billing_features"`
	EndpointType       string          `json:"endpoint_type"`
	SupportedProtocols []string        `json:"supported_protocols"`
	RawModel           json.RawMessage `json:"raw_model"`
	Conflicts          []string        `json:"conflicts"`
}

type ProviderContractBlocker struct {
	ID                string   `json:"id"`
	Models            []string `json:"models"`
	Profile           string   `json:"profile,omitempty"`
	Status            string   `json:"status"`
	MissingFacts      []string `json:"missing_facts"`
	UnblockPredicates []string `json:"unblock_predicates"`
	Conflicts         []string `json:"conflicts"`
}

// EvaluateProviderContracts accepts authenticated /v1/catalog snapshots only.
// Unknown provider extensions are recorded for schema review, never interpreted
// as authoritative selectors, quantities, bounds, or canonical bindings.
func EvaluateProviderContracts(catalogJSON, previousCatalogJSON []byte) (ProviderContractAudit, error) {
	report, current, err := parseProviderContractCatalog(catalogJSON)
	if err != nil {
		return report, err
	}
	var previous map[string]map[string]json.RawMessage
	var previousReport ProviderContractAudit
	if len(previousCatalogJSON) != 0 {
		previousReport, previous, err = parseProviderContractCatalog(previousCatalogJSON)
		if err != nil {
			return report, fmt.Errorf("previous catalog: %w", err)
		}
		report.PreviousCatalogHash = fmt.Sprintf("%x", sha256.Sum256(previousCatalogJSON))
	}
	seedance := []string{"doubao-seedance-2.0-lite", "doubao-seedance-2.0-fast-lite", "doubao-seedance-2.0-mini-lite", "doubao-seedance-2.5-lite"}
	report.Blockers = []ProviderContractBlocker{
		{ID: "seedance_lite", Models: seedance, MissingFacts: []string{"authenticated effective 720p and 1080p second-stage rates for all four Lite SKUs"}, UnblockPredicates: []string{"every Lite SKU is callable; pricing.video_second_stage_per_second has strictly positive decimal-string 720p and 1080p rates; no unknown billing fields, features, pricing fields, or schema conflicts"}},
		{ID: "minimax_h3", Models: []string{"minimax-h3"}, MissingFacts: []string{"provider-confirmed billing basis", "canonical gateway/settlement binding for minimax-h3 versus MiniMax-H3", "authoritative terminal quantity for every billed component"}, UnblockPredicates: []string{"authenticated billing basis and canonical model binding are unambiguous and consistent with provider clarification; each required terminal quantity has a reviewed authoritative field mapping"}},
		{ID: "qwen_threshold_domain", Models: []string{"qwen-image-3.0-pro"}, MissingFacts: []string{"authoritative threshold, comparison operator and dimension source", "accepted width/height domain", "guaranteed omitted-size settlement dimensions"}, UnblockPredicates: []string{"catalog/public threshold and size-domain conflicts are removed, or provider confirmation establishes the authoritative settlement rule; reviewed mapping covers threshold inclusivity, requested/output size and omitted-size behavior"}},
		{ID: "subtitle_duration", Models: []string{"tvod-subtitle-soft"}, MissingFacts: []string{"authoritative source_video_seconds", "provider-enforced maximum source duration for reservation", "ledger unit_count semantics"}, UnblockPredicates: []string{"authoritative source duration field and provider-enforced hard duration bound are documented and mapped; processing units and source seconds are distinct"}},
		{ID: "gpt_fast", Profile: "chat_fast", Models: []string{}, MissingFacts: []string{"provider-enforced fast selector", "actually served tier", "authenticated effective fast component prices", "fallback and additive/replacement semantics"}, UnblockPredicates: []string{"each affected model has a reviewed provider-enforced selector, authoritative served-tier field and authenticated effective prices for every billable component including fallback"}},
		{ID: "dedicated_image", Profile: "dedicated_image", Models: []string{}, MissingFacts: []string{"canonical endpoint/model binding", "authenticated effective per-image prices", "authoritative actual output-count and reservation semantics"}, UnblockPredicates: []string{"each affected text ID has an explicit canonical image endpoint/model binding, authenticated effective prices and reviewed actual-output-count semantics; aliases are never inferred"}},
		{ID: "grok_server_tools", Profile: "server_tools", Models: []string{}, MissingFacts: []string{"provider-enforced hard upper bound on billable server-side tool calls"}, UnblockPredicates: []string{"each affected Grok profile has an explicit provider-enforced finite integer upper bound covering all enabled server tools, mapped to reservation; a client preference is insufficient"}},
	}
	for _, evidence := range append(slices.Clone(report.Models), previousReport.Models...) {
		var rawEntry struct {
			Pricing map[string]json.RawMessage `json:"pricing"`
		}
		_ = common.Unmarshal(evidence.RawModel, &rawEntry)
		if strings.HasPrefix(evidence.Model, "gpt-") && !strings.HasPrefix(evidence.Model, "gpt-image-") {
			if slices.Contains(evidence.BillingFeatures, "fast_mode") {
				report.Blockers[4].Models = append(report.Blockers[4].Models, evidence.Model)
			}
			if validProviderContractPrice(rawEntry.Pricing["price_per_image"], false) {
				report.Blockers[5].Models = append(report.Blockers[5].Models, evidence.Model)
			}
		}
		if evidence.Model == "codex-auto-review" && validProviderContractPrice(rawEntry.Pricing["price_per_image"], false) {
			report.Blockers[5].Models = append(report.Blockers[5].Models, evidence.Model)
		}
		if strings.HasPrefix(evidence.Model, "grok-") && !strings.HasPrefix(evidence.Model, "grok-imagine-") && validProviderContractPrice(rawEntry.Pricing["price_per_server_tool_call"], false) {
			report.Blockers[6].Models = append(report.Blockers[6].Models, evidence.Model)
		}
	}
	for i := range report.Blockers {
		slices.Sort(report.Blockers[i].Models)
		report.Blockers[i].Models = slices.Compact(report.Blockers[i].Models)
		blocker := &report.Blockers[i]
		blocker.Status = "STILL_CONFLICTING"
		blocker.Conflicts = append([]string{}, report.Conflicts...)
		unchanged := previous != nil
		resolved := i == 0
		for _, id := range blocker.Models {
			entry, exists := current[id]
			old, wasPresent := previous[id]
			if !exists || !wasPresent {
				unchanged = false
			}
			if exists && wasPresent {
				a, _ := common.Marshal(entry)
				b, _ := common.Marshal(old)
				if !bytes.Equal(a, b) {
					unchanged = false
				}
			}
			if !exists {
				resolved = false
				continue
			}
			for _, evidence := range report.Models {
				if evidence.Model == id {
					blocker.Conflicts = append(blocker.Conflicts, evidence.Conflicts...)
					if !evidence.Callable || i == 0 && (evidence.EndpointType != "videos_generations" || !slices.Contains(evidence.BillingFeatures, "video_two_stage")) {
						resolved = false
					}
				}
			}
			if i == 0 {
				var pricing map[string]json.RawMessage
				_ = common.Unmarshal(entry["pricing"], &pricing)
				var rates map[string]json.RawMessage
				if common.Unmarshal(pricing["video_second_stage_per_second"], &rates) != nil || rates == nil {
					resolved = false
					continue
				}
				for _, tier := range []string{"720p", "1080p"} {
					rate, present := rates[tier]
					if !present || bytes.Equal(bytes.TrimSpace(rate), []byte("null")) {
						resolved = false
						continue
					}
					if !validProviderContractPrice(rate, true) {
						resolved = false
					}
				}
			}
		}
		if len(blocker.Conflicts) > 0 {
			blocker.Status = "NEW_CONFLICT"
		} else if resolved {
			blocker.Status = "RESOLVED_BY_CATALOG"
			blocker.MissingFacts = []string{}
		} else if unchanged && len(blocker.Models) > 0 {
			blocker.Status = "UNCHANGED"
		}
	}
	return report, nil
}

// parseProviderContractCatalog preserves unknown fields in evidence while
// identifying unsupported semantics. No typed catalog DTO may drop them.
func parseProviderContractCatalog(raw []byte) (ProviderContractAudit, map[string]map[string]json.RawMessage, error) {
	report := ProviderContractAudit{CatalogHash: fmt.Sprintf("%x", sha256.Sum256(raw)), FreshPreviewRequired: true, Models: []ProviderContractModelEvidence{}, Conflicts: []string{}}
	var catalog struct {
		SchemaVersion string            `json:"schema_version"`
		Currency      string            `json:"currency"`
		Aliases       map[string]string `json:"aliases"`
		Models        []json.RawMessage `json:"models"`
	}
	if err := common.Unmarshal(raw, &catalog); err != nil || catalog.Models == nil || catalog.Aliases == nil {
		return report, nil, errors.New("invalid authenticated DFLOP catalog")
	}
	report.SchemaVersion = catalog.SchemaVersion
	if catalog.SchemaVersion != "1.0" {
		report.Conflicts = append(report.Conflicts, "SCHEMA_VERSION_CHANGED: reviewed schema 1.0 required")
	}
	if catalog.Currency != "points" {
		report.Conflicts = append(report.Conflicts, "CURRENCY_SOURCE_CONFLICT: authenticated points currency required")
	}
	models := make(map[string]map[string]json.RawMessage, len(catalog.Models))
	for _, rawModel := range catalog.Models {
		var entry map[string]json.RawMessage
		if common.Unmarshal(rawModel, &entry) != nil || entry == nil {
			return report, nil, errors.New("invalid authenticated model object")
		}
		var id string
		if common.Unmarshal(entry["id"], &id) != nil || id == "" {
			return report, nil, errors.New("invalid authenticated model ID")
		}
		if _, exists := models[id]; exists {
			return report, nil, fmt.Errorf("duplicate authenticated model %q", id)
		}
		models[id] = entry
		evidence := ProviderContractModelEvidence{Model: id, PricingKeys: []string{}, BillingFeatures: []string{}, SupportedProtocols: []string{}, Conflicts: []string{}}
		var pricing, billing map[string]json.RawMessage
		if common.Unmarshal(entry["pricing"], &pricing) != nil || pricing == nil {
			evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_PRICING_OBJECT")
		}
		if common.Unmarshal(entry["billing"], &billing) != nil || billing == nil {
			evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_BILLING_OBJECT")
		}
		if common.Unmarshal(billing["features"], &evidence.BillingFeatures) != nil || evidence.BillingFeatures == nil {
			evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_BILLING_FEATURES")
		}
		for _, feature := range evidence.BillingFeatures {
			if !knownBillingFeatures[feature] {
				evidence.Conflicts = append(evidence.Conflicts, id+": UNKNOWN_BILLING_FEATURE:"+feature)
			}
		}
		for key := range billing {
			if key != "features" {
				evidence.Conflicts = append(evidence.Conflicts, id+": UNREVIEWED_BILLING_FIELD:"+key)
			}
		}
		for key, value := range pricing {
			evidence.PricingKeys = append(evidence.PricingKeys, key)
			if !knownEffectivePricingKeys[key] {
				evidence.Conflicts = append(evidence.Conflicts, id+": UNREVIEWED_PRICING_FIELD:"+key)
				continue
			}
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				continue
			}
			if key == "video_bills_input_seconds" {
				var flag bool
				if common.Unmarshal(value, &flag) != nil {
					evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_BOOLEAN:"+key)
				}
			}
			if slices.Contains([]string{"video_max_input_seconds", "large_pixel_threshold", "long_context_threshold_tokens", "free_input_images", "images_per_request"}, key) {
				var quantity int64
				if common.Unmarshal(value, &quantity) != nil || quantity < 0 {
					evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_INTEGER_BOUND:"+key)
				}
			}
			if strings.HasPrefix(key, "price_per_") || strings.Contains(key, "per_1m") && key != "video_token_price_per_1m" {
				if !validProviderContractPrice(value, false) {
					evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_PRICE:"+key)
				}
			}
			if slices.Contains([]string{"video_price_tiers", "video_token_price_per_1m", "video_second_stage_per_second", "image_price_tiers"}, key) {
				var rates map[string]json.RawMessage
				if common.Unmarshal(value, &rates) != nil || rates == nil {
					evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_PRICE_MAP:"+key)
					continue
				}
				for tier, amount := range rates {
					if !validProviderContractPrice(amount, key == "video_second_stage_per_second") {
						evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_PRICE:"+key+"."+tier)
					}
				}
			}
		}
		if value, exists := pricing["callable"]; exists {
			if common.Unmarshal(value, &evidence.Callable) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_CALLABLE")
			}
		}
		if evidence.Callable {
			report.CallableCount++
		}
		if value, exists := pricing["endpoint_type"]; exists && common.Unmarshal(value, &evidence.EndpointType) != nil {
			evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_ENDPOINT_TYPE")
		}
		if value, exists := pricing["supported_protocols"]; exists && common.Unmarshal(value, &evidence.SupportedProtocols) != nil {
			evidence.Conflicts = append(evidence.Conflicts, id+": INVALID_SUPPORTED_PROTOCOLS")
		}
		evidence.RawModel, _ = RedactCanaryJSON(rawModel, "")
		slices.Sort(evidence.PricingKeys)
		slices.Sort(evidence.Conflicts)
		report.Models = append(report.Models, evidence)
	}
	return report, models, nil
}

// Catalog money is a decimal string, not an arbitrary coercible JSON scalar.
// Bound text and exponent before decimal arithmetic to avoid huge allocations.
func validProviderContractPrice(raw json.RawMessage, positive bool) bool {
	var text string
	if common.Unmarshal(raw, &text) != nil || text == "" || len(text) > 64 || strings.TrimSpace(text) != text || strings.ContainsAny(text, "eE") {
		return false
	}
	amount, err := decimal.NewFromString(text)
	if err != nil || amount.IsNegative() || positive && !amount.IsPositive() {
		return false
	}
	return amount.LessThanOrEqual(decimal.NewFromInt(1_000_000_000_000)) && amount.Exponent() >= -18
}
