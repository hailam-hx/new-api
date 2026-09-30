package dflop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// pricingShape records the source contract independently of the local provider.
// A change in this value needs a new preview even when component prices match.
func pricingShape(source Model) string {
	features := slices.Clone(source.BillingFeatures)
	slices.Sort(features)
	return source.Category + ":" + strings.Join(features, "+")
}

func mediaPricesMatch(item Item, required ...string) bool {
	for _, key := range required {
		if _, ok := item.Prices[key]; !ok {
			return false
		}
	}
	for key, price := range item.Prices {
		if slices.Contains(required, key) {
			continue
		}
		if (key == "input_per_1m" || key == "output_per_1m") && price.Credits == "0" {
			continue
		}
		return false
	}
	return true
}

func videoPricesMatch(item Item, tiers map[string]string) bool {
	required := make([]string, 0, len(tiers)+1)
	required = append(required, "price_per_video_second")
	for tier := range tiers {
		required = append(required, "video_tier:"+tier)
	}
	return mediaPricesMatch(item, required...)
}

// classifyTaskPricing creates a candidate only for source shapes whose
// quantities are reported by an exact task-plugin model binding.
func classifyTaskPricing(item *Item, source Model) {
	features := slices.Clone(source.BillingFeatures)
	slices.Sort(features)
	shape := strings.Join(features, "+")
	price := func(name string) string { return item.Prices[name].SellingUSD }
	switch {
	case source.ID == "voice-tts-pro" && source.Category == "audio" && source.EndpointType == "tts_synthesize" &&
		shape == "tts_char" && source.PricePerTTSChar != nil && mediaPricesMatch(*item, "price_per_tts_char"):
		item.TaskPlugin = "dflop-tts"
		item.RequiredFacts = []string{"characters"}
		item.TaskExpression = fmt.Sprintf("u(\"characters\") * %s", price("price_per_tts_char"))
		item.ReasonCode = "NO_ASYNC_TTS_BINDING"
	case source.Category == "video" && source.EndpointType == "videos_generations" &&
		(shape == "video_second+video_tiers" || shape == "video_input_seconds+video_second+video_tiers") &&
		((shape == "video_input_seconds+video_second+video_tiers" && source.VideoBillsInputSeconds != nil && *source.VideoBillsInputSeconds) ||
			(shape == "video_second+video_tiers" && (source.VideoBillsInputSeconds == nil || !*source.VideoBillsInputSeconds))) &&
		len(source.VideoPriceTiers) > 0 && source.PricePerVideoSecond != nil &&
		source.VideoPriceTiers["1080p"] == *source.PricePerVideoSecond && videoPricesMatch(*item, source.VideoPriceTiers):
		tierPrices := make(map[string]string, len(source.VideoPriceTiers))
		tiers := make([]string, 0, len(source.VideoPriceTiers))
		for tier := range source.VideoPriceTiers {
			normalized := strings.ToUpper(tier)
			if _, exists := tierPrices[normalized]; exists {
				return
			}
			tiers = append(tiers, normalized)
			tierPrices[normalized] = price("video_tier:" + tier)
		}
		item.TaskPlugin = "alibaba"
		item.RequiredFacts = []string{"seconds", "resolution"}
		slices.Sort(tiers)
		var expression strings.Builder
		for i, tier := range tiers {
			if i < len(tiers)-1 {
				fmt.Fprintf(&expression, "u(\"resolution\") == %q ? tier(%q, u(\"seconds\") * %s) : ", tier, tier, tierPrices[tier])
			} else {
				fmt.Fprintf(&expression, "tier(%q, u(\"seconds\") * %s)", tier, tierPrices[tier])
			}
		}
		item.TaskExpression = expression.String()
		item.ReasonCode = "NO_PLUGIN_USAGE_PROFILE"
	case source.Category == "image" && source.EndpointType == "images_generations" &&
		shape == "input_images+per_image" && mediaPricesMatch(*item, "price_per_image", "price_per_input_image") &&
		source.PricePerImage != nil && source.PricePerInputImage != nil:
		item.TaskPlugin = "alibaba"
		item.RequiredFacts = []string{"image_count", "input_image_count"}
		item.TaskExpression = fmt.Sprintf("tier(\"image\", u(\"image_count\") * %s + u(\"input_image_count\") * %s)", price("price_per_image"), price("price_per_input_image"))
		item.ReasonCode = "NO_PLUGIN_USAGE_PROFILE"
	case source.ID == "doubao-seedream-5-0-pro-260628" && source.Category == "image" &&
		shape == "image_size_bands+input_images+per_image" && mediaPricesMatch(*item, "price_per_image", "price_per_image_large", "price_per_input_image") &&
		source.LargePixelThreshold != nil && *source.LargePixelThreshold == 2610000 &&
		source.PricePerImage != nil && source.PricePerImageLarge != nil && source.PricePerInputImage != nil:
		// This model's source contract makes the first input reference free.
		item.TaskPlugin = "doubao"
		item.RequiredFacts = []string{"images_up_to_1_5k", "images_above_1_5k", "input_images"}
		item.TaskExpression = fmt.Sprintf("tier(\"image\", u(\"images_up_to_1_5k\") * %s + u(\"images_above_1_5k\") * %s + max(u(\"input_images\") - 1, 0) * %s)", price("price_per_image"), price("price_per_image_large"), price("price_per_input_image"))
		item.ReasonCode = "NO_PLUGIN_USAGE_PROFILE"
	}
}

func classifyUnsupportedReason(item *Item, source Model) {
	switch {
	case source.ID == "qwen-image-3.0-pro" && source.LargePixelThreshold != nil:
		item.ReasonCode = "IMAGE_TIER_THRESHOLD_MISMATCH"
		item.RequiredFacts = []string{"image_count", "output_pixel_tier", "input_image_count"}
	case source.PricePerTTSChar != nil:
		item.ReasonCode = "UNKNOWN_CHARACTER_COUNT_SEMANTICS"
		item.RequiredFacts = []string{"billable_characters"}
	case source.PricePerServerToolCall != nil:
		item.ReasonCode = "MISSING_SERVER_TOOL_USAGE"
		item.RequiredFacts = []string{"server_tool_calls"}
	case len(source.VideoTokenPricePer1M) > 0:
		item.ReasonCode = "UNVERIFIED_VIDEO_TOKEN_USAGE"
		item.RequiredFacts = []string{"video_tokens", "resolution"}
	case source.Category == "video" && (source.PricePerVideoSecond != nil || source.PricePerVideoTask != nil || source.PricePerAvatar != nil):
		item.ReasonCode = "NO_PLUGIN_USAGE_PROFILE"
		item.RequiredFacts = []string{"successful_output_quantity"}
	case source.Category == "audio" && (source.PricePerMusicGeneration != nil || source.PricePerVoiceClone != nil):
		item.ReasonCode = "NO_PLUGIN_USAGE_PROFILE"
		item.RequiredFacts = []string{"successful_output_quantity"}
	case source.Category == "image" && source.PricePerImage != nil:
		item.ReasonCode = "UNVERIFIED_OUTPUT_COUNT"
		item.RequiredFacts = []string{"image_count"}
	}
}

func missingTaskBindingReason(item Item) string {
	switch {
	case slices.Contains(item.BillingFeatures, "music"):
		return "NO_EXACT_MUSIC_PLUGIN_BINDING"
	case slices.Contains(item.BillingFeatures, "voice_clone"):
		return "NO_EXACT_VOICE_CLONE_BINDING"
	case slices.Contains(item.BillingFeatures, "video_task"):
		return "NO_EXACT_FIXED_TASK_BINDING"
	case slices.Contains(item.BillingFeatures, "avatar"):
		return "NO_EXACT_AVATAR_PLUGIN_BINDING"
	case item.Category == "video":
		return "NO_EXACT_VIDEO_PLUGIN_BINDING"
	case item.Category == "image":
		return "NO_EXACT_IMAGE_PLUGIN_BINDING"
	default:
		return "NO_PLUGIN_USAGE_PROFILE"
	}
}

// taskPricingCompatibility checks declared facts and every source tier. The
// plugin key alone is insufficient: usage profiles vary by model.
func taskPricingCompatibility(item Item, schema map[string]jsplugin.UsageFieldSchema) (available, missing []string, reason string) {
	for _, name := range item.RequiredFacts {
		field, ok := schema[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if name == "resolution" {
			if len(field.Enum) == 0 {
				missing = append(missing, name)
				continue
			}
			for key := range item.Prices {
				if tier, ok := strings.CutPrefix(key, "video_tier:"); ok && !slices.Contains(field.Enum, strings.ToUpper(tier)) {
					missing = append(missing, name)
					break
				}
			}
			tierCount := 0
			for key := range item.Prices {
				if strings.HasPrefix(key, "video_tier:") {
					tierCount++
				}
			}
			if slices.Contains(missing, name) || len(field.Enum) != tierCount {
				if !slices.Contains(missing, name) {
					missing = append(missing, name)
				}
				continue
			}
		} else if field.Type != "number" || (name == "seconds" && field.Unit != "second") || (name != "seconds" && field.Unit != "count") {
			missing = append(missing, name)
			continue
		}
		available = append(available, name)
	}
	if len(missing) > 0 {
		for _, name := range missing {
			switch name {
			case "seconds":
				return available, missing, "MISSING_USAGE_SECONDS"
			case "resolution":
				return available, missing, "MISSING_RESOLUTION_FACT"
			}
		}
		return available, missing, "MISSING_USAGE_FACT"
	}
	if err := billing_setting.SmokeTestTaskExpr(item.TaskExpression, schema); err != nil {
		return available, []string{"validated_expression"}, "INCOMPATIBLE_PLUGIN_SCHEMA"
	}
	return available, nil, ""
}
