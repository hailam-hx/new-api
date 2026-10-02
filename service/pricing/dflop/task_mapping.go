package dflop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// completedQuantityContractModels have reviewed reservation and terminal
// metering paths; only authenticated catalog rows can close their blockers.
var completedQuantityContractModels = []string{
	"tvod-midjourney-v7", "tvod-midjourney-v8.1", "voice-tts-pro", "clip-compose",
	"dh-avatar", "dh-lipsync", "dh-lipsync-pro", "dh-lipsync-max", "dh-motion", "dh-avatar-create",
}

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
	if slices.Contains(completedQuantityContractModels, source.ID) && item.PriceSemantics.SourcePriceKind != "AUTHENTICATED_EFFECTIVE_PRICE" {
		return
	}
	if item.PriceSemantics.SourcePriceKind == "AUTHENTICATED_EFFECTIVE_PRICE" && classifyDFLOPImagePricing(item, source) {
		return
	}
	if classifyDFLOPMediaPricing(item, source) {
		return
	}
	features := slices.Clone(source.BillingFeatures)
	slices.Sort(features)
	shape := strings.Join(features, "+")
	price := func(name string) string { return item.Prices[name].SellingUSD }
	switch {
	case slices.Contains([]string{"tvod-midjourney-v7", "tvod-midjourney-v8.1"}, source.ID) && source.Category == "image" && source.EndpointType == "images_generations" && shape == "fixed_output_count+per_image" && source.ImagesPerRequest != nil && *source.ImagesPerRequest == 4 && source.PricePerImage != nil && mediaPricesMatch(*item, "price_per_image"):
		var contract struct {
			Caps struct {
				Image struct {
					FixedOutputs *int `json:"fixed_outputs"`
				} `json:"image"`
			} `json:"caps"`
		}
		if common.Unmarshal(item.Raw, &contract) != nil || contract.Caps.Image.FixedOutputs == nil || *contract.Caps.Image.FixedOutputs != 4 {
			item.ReasonCode = "PROVIDER_CONTRACT_REQUIRED"
			return
		}
		item.TaskPlugin = "dflop-image"
		item.RequiredFacts = []string{"image_count"}
		item.TaskExpression = fmt.Sprintf("tier(\"image\", u(\"image_count\") * %s)", price("price_per_image"))
		item.ReasonCode = "NO_PLUGIN_USAGE_PROFILE"
	case source.ID == "voice-tts-pro" && source.Category == "audio" && source.EndpointType == "tts_synthesize" &&
		shape == "tts_char" && source.PricePerTTSChar != nil && mediaPricesMatch(*item, "price_per_tts_char"):
		item.TaskPlugin = "dflop-tts"
		item.RequiredFacts = []string{"character_count"}
		item.TaskExpression = fmt.Sprintf("u(\"character_count\") * %s", price("price_per_tts_char"))
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
	if item.ReasonCode == "PROVIDER_CATALOG_MISSING_UPSCALE_RATE" {
		return
	}
	if source.ID == "minimax-h3" {
		item.RequiredFacts = []string{"duration_sec", "resolution"}
		item.ReasonCode = "NO_EXACT_VIDEO_PLUGIN_BINDING"
		if slices.Contains(source.BillingFeatures, "video_input_seconds") || source.VideoBillsInputSeconds != nil && *source.VideoBillsInputSeconds {
			item.ReasonCode = "MISSING_INPUT_VIDEO_DURATION"
			item.RequiredFacts = append(item.RequiredFacts, "input_video_duration_sec")
		}
		return
	}
	if source.ID == "tvod-subtitle-soft" {
		item.ReasonCode = "MISSING_SUBTITLE_SOURCE_DURATION"
		item.RequiredFacts = []string{"source_duration_sec", "asr_units", "translation_units"}
		return
	}
	switch {
	case source.ID == "qwen-image-3.0-pro" && source.LargePixelThreshold != nil:
		item.ReasonCode = "PROVIDER_CONTRACT_IMAGE_THRESHOLD_CONFLICT"
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
				tier, ok := strings.CutPrefix(key, "video_tier:")
				if !ok {
					continue
				}
				if item.TaskPlugin != "dflop-media" {
					tier = strings.ToUpper(tier)
				}
				if !slices.Contains(field.Enum, tier) {
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
		} else if name == "input_mode" {
			if !slices.Equal(field.Enum, []string{"default", "with_video_input"}) {
				missing = append(missing, name)
				continue
			}
		} else {
			unit := "count"
			switch name {
			case "seconds", "duration_sec", "input_video_duration_sec":
				unit = "second"
			case "completion_tokens":
				unit = "token"
			case "character_count":
				unit = "character"
			}
			if field.Type != "number" || field.Unit != unit {
				missing = append(missing, name)
				continue
			}
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

// classifyDFLOPMediaPricing selects only an exact factory contract. The
// existing Alibaba/Doubao profiles keep their independently verified bindings.
// Subtitle ASR/translation tiers are operations, not output resolutions; its
// authoritative source duration contract remains unavailable.
func classifyDFLOPMediaPricing(item *Item, source Model) bool {
	if item.PriceSemantics.SourcePriceKind != "AUTHENTICATED_EFFECTIVE_PRICE" && slices.Contains([]string{"wan2.7-t2v", "wan3.0-video", "wan3.0-video-prime", "tvod-subtitle-soft"}, source.ID) {
		return false
	}
	plugin, exists := jsplugin.DefaultRegistry.Generation().Get("dflop-media")
	if !exists || !slices.Contains(plugin.Meta.Models, source.ID) {
		return false
	}
	schema, _ := plugin.Meta.UsageForModel(source.ID)
	features := slices.Clone(source.BillingFeatures)
	slices.Sort(features)
	shape := strings.Join(features, "+")
	price := func(name string) string { return item.Prices[name].SellingUSD }
	requiredPrices := []string{}
	facts := []string{}
	quantity := ""
	switch {
	case source.Category == "audio" && source.EndpointType == "music_generations" && shape == "music" && source.PricePerMusicGeneration != nil:
		requiredPrices = []string{"price_per_music_generation"}
		facts = []string{"generation_count"}
		quantity = "u(\"generation_count\") * " + price("price_per_music_generation")
	case source.ID == "voice-clone-pro" && source.EndpointType == "voice_clone" && shape == "voice_clone" && source.PricePerVoiceClone != nil:
		requiredPrices = []string{"price_per_voice_clone"}
		facts = []string{"count"}
		quantity = "u(\"count\") * " + price("price_per_voice_clone")
	case source.ID == "dh-avatar-create" && source.EndpointType == "avatar_create" && shape == "avatar" && source.PricePerAvatar != nil:
		requiredPrices = []string{"price_per_avatar"}
		facts = []string{"count"}
		quantity = "u(\"count\") * " + price("price_per_avatar")
	case source.ID == "clip-compose" && source.EndpointType == "videos_generations" && shape == "video_task" && source.PricePerVideoTask != nil:
		requiredPrices = []string{"price_per_video_task"}
		facts = []string{"count"}
		quantity = "u(\"count\") * " + price("price_per_video_task")
	case source.Category == "video" && source.EndpointType == "videos_generations" && len(source.VideoTokenPricePer1M) > 0:
		if source.PricePerVideoSecond == nil || len(source.VideoPriceTiers) == 0 {
			return false
		}
		formula := "video_token_formula_seedance_2_0"
		if slices.Contains(source.BillingFeatures, "video_token_formula_seedance_2_5") {
			formula = "video_token_formula_seedance_2_5"
		}
		wanted := []string{"video_second", "video_tiers", "video_token", formula}
		lite := slices.Contains(source.BillingFeatures, "video_two_stage")
		if lite {
			wanted = append(wanted, "video_two_stage")
		}
		slices.Sort(wanted)
		if !slices.Equal(features, wanted) {
			return false
		}
		tiers := slices.Clone(schema["resolution"].Enum)
		slices.Sort(tiers)
		expectedTokenTiers := 2 * len(tiers)
		if source.ID == "doubao-seedance-2.5" {
			expectedTokenTiers = 4
		}
		if len(tiers) == 0 || len(source.VideoTokenPricePer1M) != expectedTokenTiers {
			return false
		}
		requiredPrices = append(requiredPrices, "price_per_video_second")
		facts = []string{"completion_tokens", "resolution", "input_mode"}
		for tier := range source.VideoPriceTiers {
			requiredPrices = append(requiredPrices, "video_tier:"+tier)
		}
		for key := range source.VideoTokenPricePer1M {
			requiredPrices = append(requiredPrices, "video_token_tier:"+key)
		}
		if lite {
			if len(source.VideoSecondStagePerSecond) != len(tiers) {
				item.ReasonCode = "PROVIDER_CATALOG_MISSING_UPSCALE_RATE"
				item.RequiredFacts = []string{"completion_tokens", "duration_sec", "resolution", "input_mode"}
				return true
			}
			for tier := range source.VideoSecondStagePerSecond {
				requiredPrices = append(requiredPrices, "video_second_stage:"+tier)
			}
			facts = append(facts, "duration_sec")
		} else if len(source.VideoSecondStagePerSecond) > 0 {
			return false
		}
		var expression strings.Builder
		branches := 0
		for _, tier := range tiers {
			for _, mode := range []string{"default", "with_video_input"} {
				key := mode + "@" + tier
				// Seedance2.5 endpoint card defines the bare mode rate for480p/720p
				// and an explicit1080p delivery override. Amounts stay catalog-derived.
				if source.ID == "doubao-seedance-2.5" && (tier == "480p" || tier == "720p") {
					key = mode
				}
				if _, ok := source.VideoTokenPricePer1M[key]; !ok {
					return false
				}
				if branches > 0 {
					expression.WriteString(" : ")
				}
				if branches < len(tiers)*2-1 {
					fmt.Fprintf(&expression, "u(\"resolution\") == %q && u(\"input_mode\") == %q ? ", tier, mode)
				}
				fmt.Fprintf(&expression, "tier(%q, u(\"completion_tokens\") * %s / 1000000", key, price("video_token_tier:"+key))
				if lite {
					if _, ok := source.VideoSecondStagePerSecond[tier]; !ok {
						return false
					}
					fmt.Fprintf(&expression, " + u(\"duration_sec\") * %s", price("video_second_stage:"+tier))
				}
				expression.WriteString(")")
				branches++
			}
		}
		// The final leaf covers the last validated enum combination.
		quantity = expression.String()
	case source.Category == "video" && source.EndpointType == "videos_generations" && source.PricePerVideoSecond != nil && ((shape == "video_second" || shape == "video_second+video_tiers") && (source.VideoBillsInputSeconds == nil || !*source.VideoBillsInputSeconds) || shape == "video_input_seconds+video_second+video_tiers" && source.VideoBillsInputSeconds != nil && *source.VideoBillsInputSeconds):
		facts = []string{"duration_sec"}
		requiredPrices = []string{"price_per_video_second"}
		if shape == "video_second" && len(source.VideoPriceTiers) == 0 {
			quantity = "u(\"duration_sec\") * " + price("price_per_video_second")
			break
		}
		if (shape != "video_second+video_tiers" && shape != "video_input_seconds+video_second+video_tiers") || len(source.VideoPriceTiers) == 0 {
			return false
		}
		tiers := make([]string, 0, len(source.VideoPriceTiers))
		for tier := range source.VideoPriceTiers {
			tiers = append(tiers, tier)
			requiredPrices = append(requiredPrices, "video_tier:"+tier)
		}
		slices.Sort(tiers)
		facts = append(facts, "resolution")
		duration := `u("duration_sec")`
		if shape == "video_input_seconds+video_second+video_tiers" {
			// Only the exact DFLOP Wan3 contract bills reference-video input.
			if !slices.Contains([]string{"wan3.0-video", "wan3.0-video-prime"}, source.ID) || source.VideoMaxInputSeconds == nil || *source.VideoMaxInputSeconds != 15 {
				item.ReasonCode = "PROVIDER_CONTRACT_REQUIRED"
				return true
			}
			facts = append(facts, "input_video_duration_sec")
			duration = `(u("duration_sec") + u("input_video_duration_sec"))`
		}
		var expression strings.Builder
		for index, tier := range tiers {
			if index > 0 {
				expression.WriteString(" : ")
			}
			if index < len(tiers)-1 {
				fmt.Fprintf(&expression, "u(\"resolution\") == %q ? ", tier)
			}
			fmt.Fprintf(&expression, "tier(%q, %s * %s)", tier, duration, price("video_tier:"+tier))
		}
		quantity = expression.String()
	default:
		return false
	}
	if !mediaPricesMatch(*item, requiredPrices...) {
		return false
	}
	item.TaskPlugin = "dflop-media"
	item.RequiredFacts = facts
	if strings.Contains(quantity, "tier(") {
		item.TaskExpression = quantity
	} else {
		item.TaskExpression = "tier(\"base\", " + quantity + ")"
	}
	_, _, reason := taskPricingCompatibility(*item, schema)
	item.ReasonCode = reason
	if reason == "" {
		item.Reason = ""
	}
	return true
}

// classifyDFLOPImagePricing binds authenticated image contracts to the DFLOP
// executor. Prices and reference exemptions come only from that source row.
func classifyDFLOPImagePricing(item *Item, source Model) bool {
	plugin, ok := jsplugin.DefaultRegistry.Generation().Get("dflop-image")
	if !ok || !slices.Contains(plugin.Meta.Models, source.ID) || source.Category != "image" || source.EndpointType != "images_generations" {
		return false
	}
	features := slices.Clone(source.BillingFeatures)
	slices.Sort(features)
	shape := strings.Join(features, "+")
	if !slices.Contains([]string{"per_image", "input_images+per_image", "image_size_bands+input_images+per_image"}, shape) {
		return false
	}
	// Midjourney's fixed-output rule is handled by its existing exact contract.
	if source.PricePerImage == nil {
		return false
	}
	item.TaskPlugin = "dflop-image"
	item.RequiredFacts = []string{"image_count"}
	quantity := fmt.Sprintf(`u("image_count") * %s`, item.Prices["price_per_image"].SellingUSD)
	required := []string{"price_per_image"}
	if slices.Contains(features, "input_images") {
		if source.PricePerInputImage == nil {
			return false
		}
		required = append(required, "price_per_input_image")
		item.RequiredFacts = append(item.RequiredFacts, "input_image_count")
		free := 0
		if source.FreeInputImages != nil {
			free = *source.FreeInputImages
		}
		if free < 0 || free > 128 {
			item.ReasonCode = "PROVIDER_CONTRACT_REQUIRED"
			return true
		}
		quantity += fmt.Sprintf(` + max(u("input_image_count") - %d, 0) * %s`, free, item.Prices["price_per_input_image"].SellingUSD)
	}
	if slices.Contains(features, "image_size_bands") {
		// Public threshold disagreement is retained as evidence, never as a price
		// override. A tiered binding exists, while terminal dimensions stay blocked.
		item.RequiredFacts = append(item.RequiredFacts, "small_image_count", "large_image_count")
		item.ReasonCode = "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS"
		item.Reason = "authenticated image rates are known; authoritative delivered output dimensions are required"
		if source.LargePixelThreshold == nil || *source.LargePixelThreshold <= 0 || source.PricePerImageLarge == nil {
			item.ReasonCode = "PROVIDER_CONTRACT_IMAGE_THRESHOLD_CONFLICT"
			item.Reason = "authenticated catalog lacks authoritative tier threshold; public docs cannot resolve it"
		}
		item.Status = UnsupportedMapping
		item.Expression = ""
		item.TaskExpression = ""
		return true
	}
	if !mediaPricesMatch(*item, required...) {
		item.ReasonCode = "UNKNOWN_BILLING_FEATURE"
		return true
	}
	item.TaskExpression = `tier("image", ` + quantity + `)`
	item.ReasonCode = ""
	return true
}
