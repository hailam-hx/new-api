package dflop

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// MusicGenerationContract is reviewed evidence from the authoritative media
// contract, not an inferred unit from a public model-list price label.
type MusicGenerationContract struct {
	BillingUnit        string `json:"billing_unit"`
	SongsPerGeneration int    `json:"songs_per_generation"`
	Source             string `json:"source"`
}

type MusicContractWatch struct {
	Safe               bool   `json:"safe"`
	ReasonCode         string `json:"reason_code,omitempty"`
	Detail             string `json:"detail"`
	BillingUnit        string `json:"billing_unit,omitempty"`
	SongsPerGeneration int    `json:"songs_per_generation,omitempty"`
}

// CheckMusicGenerationContract audits one authenticated catalog object without
// changing pricing or routing. One generation includes its two output songs;
// output track count must never multiply the generation price. A conflict is
// evidence for manual review, not permission to replace the saved pricing.
func CheckMusicGenerationContract(raw json.RawMessage, mediaContract MusicGenerationContract) MusicContractWatch {
	result := MusicContractWatch{ReasonCode: "PROVIDER_CONTRACT_MUSIC_UNIT_CONFLICT", Detail: "authenticated billing unit and authoritative media contract must identify one generation including two songs"}
	if mediaContract.BillingUnit != "generation" || mediaContract.SongsPerGeneration != 2 || strings.TrimSpace(mediaContract.Source) == "" {
		return result
	}
	var source struct {
		Pricing map[string]json.RawMessage `json:"pricing"`
		Billing map[string]json.RawMessage `json:"billing"`
	}
	if common.Unmarshal(raw, &source) != nil {
		return result
	}
	var endpoint string
	if common.Unmarshal(source.Pricing["endpoint_type"], &endpoint) != nil || endpoint != "music_generations" {
		return result
	}
	var features []string
	if common.Unmarshal(source.Billing["features"], &features) != nil || len(features) != 1 || features[0] != "music" {
		return result
	}
	var priceText string
	if common.Unmarshal(source.Pricing["price_per_music_generation"], &priceText) != nil {
		return result
	}
	price, err := decimal.NewFromString(priceText)
	if err != nil || price.IsNegative() {
		return result
	}
	for key, value := range source.Pricing {
		if string(value) == "null" {
			continue
		}
		// A newly introduced price component cannot be silently ignored, including
		// future per-track or duration pricing alongside the old generation rate.
		if strings.HasPrefix(key, "price_per_") && key != "price_per_music_generation" {
			return result
		}
	}
	for key := range source.Billing {
		if key != "features" && key != "unit" && key != "billing_unit" && key != "music_billing_unit" {
			return result
		}
	}
	for _, section := range []map[string]json.RawMessage{source.Pricing, source.Billing} {
		for key, value := range section {
			if key != "unit" && key != "billing_unit" && key != "music_billing_unit" {
				continue
			}
			var unit string
			if common.Unmarshal(value, &unit) != nil || unit != "generation" {
				return result
			}
		}
	}
	result.Safe = true
	result.ReasonCode = ""
	result.Detail = "one generation is billed once and includes two songs; output track count is not a billing multiplier"
	result.BillingUnit = "generation"
	result.SongsPerGeneration = 2
	return result
}
