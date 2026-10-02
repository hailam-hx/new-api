package service

import (
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

// DFLOPSeedanceTokenEstimate evaluates the documented token formula exactly.
// It is a reservation estimate, never a replacement for terminal completion_tokens.
// Pixel tables: https://docs.volcengine.com/docs/ark/create-video-generation-task-api?lang=zh
// DFLOP hold and Lite downgrade: https://model.dflop.top/en/docs/reference/media-apis
func DFLOPSeedanceTokenEstimate(model, deliveryResolution, ratio string, outputSeconds, inputSeconds decimal.Decimal) (decimal.Decimal, error) {
	base := strings.TrimSuffix(model, "-lite")
	capSeconds := int64(15)
	switch base {
	case "doubao-seedance-2.0", "doubao-seedance-2.0-fast", "doubao-seedance-2.0-mini":
	case "doubao-seedance-2.5":
		capSeconds = 30
	default:
		return decimal.Zero, errors.New("PROVIDER_TOKEN_CEILING_REQUIRED")
	}
	cap := decimal.NewFromInt(capSeconds)
	if outputSeconds.LessThan(decimal.NewFromInt(4)) || outputSeconds.GreaterThan(cap) || inputSeconds.IsNegative() || inputSeconds.GreaterThan(cap) {
		return decimal.Zero, errors.New("BILLING_QUANTITY_INVALID: Seedance duration")
	}
	if inputSeconds.IsPositive() {
		inputSeconds = decimal.Max(inputSeconds, decimal.NewFromInt(4))
	}
	resolution := deliveryResolution
	if strings.HasSuffix(model, "-lite") {
		switch resolution {
		case "720p":
			resolution = "480p"
		case "1080p":
			resolution = "720p"
		default:
			return decimal.Zero, errors.New("PROVIDER_TOKEN_CEILING_REQUIRED: undocumented delivery resolution")
		}
	}
	// Order matches the official ratio table; equal-area frames preserve that order.
	ratios := []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9"}
	var frames [][2]int64
	switch resolution {
	case "480p":
		frames = [][2]int64{{864, 496}, {752, 560}, {640, 640}, {560, 752}, {496, 864}, {992, 432}}
		if base == "doubao-seedance-2.5" {
			frames[0], frames[4] = [2]int64{854, 480}, [2]int64{480, 854}
		}
	case "720p":
		frames = [][2]int64{{1280, 720}, {1112, 834}, {960, 960}, {834, 1112}, {720, 1280}, {1470, 630}}
	case "1080p":
		if base == "doubao-seedance-2.0-mini" {
			return decimal.Zero, errors.New("PROVIDER_TOKEN_CEILING_REQUIRED: undocumented delivery resolution")
		}
		frames = [][2]int64{{1920, 1080}, {1664, 1248}, {1440, 1440}, {1248, 1664}, {1080, 1920}, {2206, 946}}
	default:
		return decimal.Zero, errors.New("PROVIDER_TOKEN_CEILING_REQUIRED: undocumented generation resolution")
	}
	var pixels int64
	for i, frame := range frames {
		area := frame[0] * frame[1]
		if ratio == "" || ratio == "adaptive" {
			pixels = max(pixels, area)
		} else if ratio == ratios[i] {
			pixels = area
			break
		}
	}
	if pixels == 0 {
		return decimal.Zero, errors.New("PROVIDER_TOKEN_CEILING_REQUIRED: undocumented ratio")
	}
	return outputSeconds.Add(inputSeconds).Mul(decimal.NewFromInt(pixels)).Mul(decimal.NewFromInt(24)).Div(decimal.NewFromInt(1024)), nil
}
