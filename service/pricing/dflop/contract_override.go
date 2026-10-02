package dflop

import (
	"slices"

	"github.com/shopspring/decimal"
)

const SupportedWithProviderOverride = "SUPPORTED_WITH_PROVIDER_OVERRIDE"
const DocumentedContractOverride = "DFLOP_DOCUMENTED_CONTRACT_OVERRIDE"

// ContractOverride is frozen with price provenance in preview, managed pricing,
// and rollback snapshots. Catalog prices are never relabeled as documentation.
type ContractOverride struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Feature          string `json:"feature"`
	Source           string `json:"source"`
	SourceURL        string `json:"source_url"`
	ObservedAt       string `json:"observed_at"`
	Version          string `json:"version"`
	CatalogConflict  string `json:"catalog_conflict"`
	Value            string `json:"value"`
	AutoApplyAllowed bool   `json:"auto_apply_allowed"`
}

func applyDocumentedContract(item *Item, source *Model, points, rate, markup decimal.Decimal) {
	override := ContractOverride{Provider: "dflop", Model: source.ID, Source: "official_dflop_contract", ObservedAt: "2026-10-01", Version: "2026-10-01-v1"}
	stale := false
	switch source.ID {
	case "doubao-seedance-2.0-fast-lite", "doubao-seedance-2.0-lite", "doubao-seedance-2.0-mini-lite", "doubao-seedance-2.5-lite":
		if source.Category != "video" || source.EndpointType != "videos_generations" || !slices.Contains(source.BillingFeatures, "video_two_stage") {
			return
		}
		override.Feature, override.SourceURL, override.CatalogConflict, override.Value = "second_stage_upscale_rate", "https://model.dflop.top/en/docs/reference/media-apis", "missing authenticated second-stage rates", "720p=2.5 points/s;1080p=5 points/s;delivered output only"
		stale = len(source.VideoSecondStagePerSecond) > 0
		if !stale {
			source.VideoSecondStagePerSecond = map[string]string{"720p": "2.5", "1080p": "5"}
			for tier, value := range source.VideoSecondStagePerSecond {
				credits, _ := decimal.NewFromString(value)
				cny := credits.DivRound(points, 24)
				usd := cny.Mul(rate)
				selling := usd.Mul(markup)
				item.Prices["video_second_stage:"+tier] = Price{Unit: UnitSecond, Credits: value, CostCNY: cny.Round(12).String(), CostUSD: usd.Round(12).String(), SellingUSD: selling.Round(12).String(), EffectiveCredits: value, EffectiveCostCNY: cny.Round(12).String(), EffectiveCostUSD: usd.Round(12).String(), EffectiveSellingUSD: selling.Round(12).String(), SourcePriceKind: DocumentedContractOverride, PromotionState: "VERIFIED", ContractOverrides: []ContractOverride{override}}
			}
		}
	case "minimax-h3":
		override.Feature, override.SourceURL, override.CatalogConflict, override.Value = "billing_basis", "https://model.dflop.top/models/minimax-h3", "video_input_seconds;video_bills_input_seconds=true", "OUTPUT_DELIVERED_SECONDS_ONLY"
		stale = source.VideoBillsInputSeconds == nil || !*source.VideoBillsInputSeconds || !slices.Contains(source.BillingFeatures, "video_input_seconds")
		if !stale {
			source.BillingFeatures = slices.DeleteFunc(slices.Clone(source.BillingFeatures), func(s string) bool { return s == "video_input_seconds" })
			value := false
			source.VideoBillsInputSeconds = &value
		}
	default:
		return
	}
	item.ContractOverrides = []ContractOverride{override}
	if stale {
		item.OverrideStale = true
	}
	// Semantics provenance accompanies authenticated amounts without changing them.
	for key, price := range item.Prices {
		if price.SourcePriceKind != DocumentedContractOverride {
			price.ContractOverrides = []ContractOverride{override}
			item.Prices[key] = price
		}
	}
}
