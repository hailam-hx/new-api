package dflop

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMusicGenerationContractWatch(t *testing.T) {
	contract := MusicGenerationContract{BillingUnit: "generation", SongsPerGeneration: 2, Source: "https://model.dflop.top/en/docs/reference/media-apis"}
	cases := []struct {
		name     string
		pricing  map[string]any
		billing  map[string]any
		contract MusicGenerationContract
		safe     bool
	}{
		{"two songs cost one generation", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music"}}, contract, true},
		{"explicit free generation", map[string]any{"price_per_music_generation": "0"}, map[string]any{"features": []string{"music"}}, contract, true},
		{"per track price conflicts", map[string]any{"price_per_music_generation": "20.4", "price_per_track": "10.2"}, map[string]any{"features": []string{"music"}}, contract, false},
		{"unknown future music component", map[string]any{"price_per_music_generation": "20.4", "price_per_music_second": "1"}, map[string]any{"features": []string{"music"}}, contract, false},
		{"missing feature", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{}}, contract, false},
		{"unknown billing component", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music"}, "track_multiplier": 2}, contract, false},
		{"unknown feature", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music", "music_track"}}, contract, false},
		{"changed unit", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music"}, "unit": "track"}, contract, false},
		{"changed unit type", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music"}, "unit": 2}, contract, false},
		{"missing effective price", map[string]any{}, map[string]any{"features": []string{"music"}}, contract, false},
		{"price changes type", map[string]any{"price_per_music_generation": 20.4}, map[string]any{"features": []string{"music"}}, contract, false},
		{"negative price", map[string]any{"price_per_music_generation": "-1"}, map[string]any{"features": []string{"music"}}, contract, false},
		{"media contract becomes ambiguous", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music"}}, MusicGenerationContract{Source: contract.Source}, false},
		{"media contract changes to tracks", map[string]any{"price_per_music_generation": "20.4"}, map[string]any{"features": []string{"music"}}, MusicGenerationContract{BillingUnit: "track", SongsPerGeneration: 2, Source: contract.Source}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.pricing["endpoint_type"] = "music_generations"
			raw, err := common.Marshal(map[string]any{"id": "suno-v5", "pricing": tc.pricing, "billing": tc.billing})
			require.NoError(t, err)
			result := CheckMusicGenerationContract(json.RawMessage(raw), tc.contract)
			assert.Equal(t, tc.safe, result.Safe)
			if tc.safe {
				assert.Equal(t, "generation", result.BillingUnit)
				assert.Equal(t, 2, result.SongsPerGeneration)
				assert.Empty(t, result.ReasonCode)
			} else {
				assert.Equal(t, "PROVIDER_CONTRACT_MUSIC_UNIT_CONFLICT", result.ReasonCode)
			}
		})
	}
}
