package dflop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/shopspring/decimal"
)

const (
	SupportedAuto       = "SUPPORTED_AUTO"
	UnsupportedMapping  = "UNSUPPORTED_MAPPING"
	SkippedNonCallable  = "SKIPPED_NON_CALLABLE"
	InvalidSource       = "INVALID_SOURCE"
	CatalogURL          = "https://api.dflop.top/api/v1/models/public"
	EffectiveCatalogURL = "https://api.dflop.top/v1/catalog"
	CurrencyURL         = "https://api.dflop.top/api/v1/config/currency"
)

// Model retains the source's distinct billing units. Empty values mean absent,
// while the string "0" remains an explicit free price.
type Model struct {
	ID                        string            `json:"id"`
	OfficialModelID           string            `json:"official_model_id"`
	VendorSlug                string            `json:"vendor_slug"`
	Category                  string            `json:"category"`
	EndpointType              string            `json:"endpoint_type"`
	Callable                  bool              `json:"callable"`
	SupportedProtocols        []string          `json:"supported_protocols"`
	BillingFeatures           []string          `json:"billing_features"`
	Discount                  *string           `json:"discount"`
	InputPer1M                *string           `json:"input_per_1m"`
	OutputPer1M               *string           `json:"output_per_1m"`
	CachedInputPer1M          *string           `json:"cached_input_per_1m"`
	CacheCreationPer1M        *string           `json:"cache_creation_per_1m"`
	PricePerImage             *string           `json:"price_per_image"`
	PricePerImageLarge        *string           `json:"price_per_image_large"`
	PricePerInputImage        *string           `json:"price_per_input_image"`
	PricePerVideoSecond       *string           `json:"price_per_video_second"`
	PricePerVideoTask         *string           `json:"price_per_video_task"`
	PricePerTTSChar           *string           `json:"price_per_tts_char"`
	PricePerMusicGeneration   *string           `json:"price_per_music_generation"`
	PricePerAvatar            *string           `json:"price_per_avatar"`
	PricePerVoiceClone        *string           `json:"price_per_voice_clone"`
	PricePerServerToolCall    *string           `json:"price_per_server_tool_call"`
	VideoPriceTiers           map[string]string `json:"video_price_tiers"`
	VideoSecondStagePerSecond map[string]string `json:"video_second_stage_per_second"`
	VideoTokenPricePer1M      map[string]string `json:"video_token_price_per_1m"`
	InputPer1MLong            *string           `json:"input_per_1m_long"`
	OutputPer1MLong           *string           `json:"output_per_1m_long"`
	CachedInputPer1MLong      *string           `json:"cached_input_per_1m_long"`
	FastModeMultiplier        *string           `json:"fast_mode_multiplier"`
	LargePixelThreshold       *int              `json:"large_pixel_threshold"`
	ImagesPerRequest          *int              `json:"images_per_request"`
	VideoBillsInputSeconds    *bool             `json:"video_bills_input_seconds"`
}

type Currency struct {
	Unit         string          `json:"unit"`
	PointsPerCNY json.RawMessage `json:"points_per_cny"`
	USDToCNYPeg  json.RawMessage `json:"usd_to_cny_peg"`
}

type PricingUnit string

const (
	UnitTokenPer1M PricingUnit = "token_per_1m"
	UnitImage      PricingUnit = "image"
	UnitSecond     PricingUnit = "second"
	UnitCharacter  PricingUnit = "character"
	UnitRequest    PricingUnit = "request"
	UnitCount      PricingUnit = "count"
	UnitCredit     PricingUnit = "credit"
	UnitOther      PricingUnit = "other"
)

type Price struct {
	Unit                PricingUnit `json:"unit"`
	Credits             string      `json:"credits"`
	CostCNY             string      `json:"cost_cny"`
	CostUSD             string      `json:"cost_usd"`
	SellingUSD          string      `json:"selling_usd"`
	SourcePriceKind     string      `json:"source_price_kind"`
	PromotionMultiplier string      `json:"promotion_multiplier,omitempty"`
	PromotionRuleID     string      `json:"promotion_rule_id,omitempty"`
	PromotionSource     string      `json:"promotion_source,omitempty"`
	PromotionState      string      `json:"promotion_state"`
	EffectiveCredits    string      `json:"effective_credits,omitempty"`
	EffectiveCostCNY    string      `json:"effective_cost_cny,omitempty"`
	EffectiveCostUSD    string      `json:"effective_cost_usd,omitempty"`
	EffectiveSellingUSD string      `json:"effective_selling_usd,omitempty"`
}

type PriceSemantics struct {
	SourcePriceKind     string `json:"source_price_kind"`
	EffectiveMultiplier string `json:"effective_multiplier,omitempty"`
	PromotionRuleID     string `json:"promotion_rule_id,omitempty"`
	PromotionSource     string `json:"promotion_source,omitempty"`
	VerifiedPriceVector string `json:"verified_price_vector,omitempty"`
	EffectiveState      string `json:"effective_state"`
}

// These list-price vectors were captured from the public catalog under the
// documented September 2026 GPT promotion. A price change
// needs a new rule review before any effective price is inferred.
var gptPromotionListPrices = map[string]string{
	"gpt-5.5":       "2022,12132,202.2,,,",
	"gpt-5.6-luna":  "80.88,485.28,8.088,,,",
	"gpt-5.6-sol":   "2022,12132,202.2,,,",
	"gpt-5.6-terra": "2022,12132,202.2,,,",
	"gpt-6":         "4044,20220,404.4,8088,30330,808.8",
	"gpt-6-astra":   "4044,20220,404.4,8088,30330,808.8",
	"gpt-6-luna":    "40.44,202.2,4.044,80.88,303.3,8.088",
	"gpt-6-sol":     "808.8,4044,80.88,1617.6,6066,161.76",
}

// Promotion rules are deliberately narrow. A changed catalog badge stops the
// preview instead of silently changing the amount charged to customers.
func classifyPriceSemantics(source Model) PriceSemantics {
	semantics := PriceSemantics{SourcePriceKind: "UNKNOWN_PROMOTION", EffectiveState: "UNKNOWN"}
	const documentation = "https://model.dflop.top/en/docs/reference/models"
	if source.Category == "text" && source.VendorSlug == "anthropic" && strings.HasPrefix(source.ID, "claude-") {
		semantics.PromotionRuleID = "dflop-claude-text-2026-09"
		semantics.PromotionSource = documentation
		semantics.EffectiveMultiplier = "0.6"
	} else if source.Category == "text" && source.VendorSlug == "openai" && strings.HasPrefix(source.ID, "gpt-") {
		semantics.PromotionRuleID = "dflop-gpt-text-2026-09"
		semantics.PromotionSource = documentation
		semantics.EffectiveMultiplier = "0.3"
		listPrice, known := gptPromotionListPrices[source.ID]
		if !known {
			semantics.EffectiveState = "CHANGED"
			return semantics
		}
		semantics.VerifiedPriceVector = listPrice
		fields := []*string{source.InputPer1M, source.OutputPer1M, source.CachedInputPer1M, source.InputPer1MLong, source.OutputPer1MLong, source.CachedInputPer1MLong}
		actual := make([]string, len(fields))
		for i, field := range fields {
			if field != nil {
				actual[i] = *field
			}
		}
		if strings.Join(actual, ",") != listPrice {
			semantics.EffectiveState = "CHANGED"
			return semantics
		}
	} else {
		if source.Discount == nil {
			return PriceSemantics{SourcePriceKind: "EFFECTIVE_PRICE", EffectiveState: "VERIFIED"}
		}
		return semantics
	}
	if source.Discount == nil || *source.Discount != semantics.EffectiveMultiplier {
		semantics.EffectiveState = "CHANGED"
		return semantics
	}
	semantics.SourcePriceKind = "LIST_PRICE_WITH_VERIFIED_MULTIPLIER"
	semantics.EffectiveState = "VERIFIED"
	return semantics
}

type Item struct {
	ModelID         string           `json:"model_id"`
	CanonicalID     string           `json:"canonical_id"`
	Category        string           `json:"category"`
	EndpointType    string           `json:"endpoint_type"`
	Callable        bool             `json:"callable"`
	Protocols       []string         `json:"supported_protocols"`
	BillingFeatures []string         `json:"billing_features"`
	Discount        *string          `json:"discount,omitempty"`
	PriceSemantics  PriceSemantics   `json:"price_semantics"`
	Status          string           `json:"status"`
	Reason          string           `json:"reason,omitempty"`
	Prices          map[string]Price `json:"prices"`
	Expression      string           `json:"expression,omitempty"`
	TaskExpression  string           `json:"task_expression,omitempty"`
	PricingShape    string           `json:"pricing_shape,omitempty"`
	ReasonCode      string           `json:"reason_code,omitempty"`
	RequiredFacts   []string         `json:"required_facts,omitempty"`
	TaskPlugin      string           `json:"task_plugin,omitempty"`
	Raw             json.RawMessage  `json:"raw"`
}

type Source interface {
	Fetch(context.Context) ([]byte, []byte, error)
}

type Client struct{ HTTP *http.Client }

type EffectiveResponse struct {
	Body         []byte `json:"-"`
	State        string `json:"state"`
	RequestedURL string `json:"requested_url"`
	FinalURL     string `json:"final_url"`
	HTTPStatus   int    `json:"http_status"`
	ContentType  string `json:"content_type"`
	ETag         string `json:"etag"`
	FetchedAt    int64  `json:"fetched_at"`
}

func (c Client) FetchEffective(ctx context.Context, key, etag string) (EffectiveResponse, error) {
	if strings.TrimSpace(key) == "" {
		return EffectiveResponse{RequestedURL: EffectiveCatalogURL, State: "FETCH_FAILED"}, errors.New("SOURCE_CHANNEL_UNAVAILABLE: missing credential")
	}
	return c.fetchResponse(ctx, EffectiveCatalogURL, key, etag)
}

func (c Client) FetchPublic(ctx context.Context) (EffectiveResponse, error) {
	return c.fetchResponse(ctx, CatalogURL, "", "")
}

func (c Client) FetchCurrency(ctx context.Context) ([]byte, error) {
	result, err := c.fetchResponse(ctx, CurrencyURL, "", "")
	return result.Body, err
}

func (c Client) fetchResponse(ctx context.Context, targetURL, key, etag string) (EffectiveResponse, error) {
	result := EffectiveResponse{RequestedURL: targetURL, FetchedAt: time.Now().Unix(), State: "FETCH_FAILED"}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	client := *httpClient
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != "api.dflop.top" || (key != "" && req.URL.Path != "/v1/catalog") {
			return errors.New("DFLOP redirected outside the source host")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return result, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("User-Agent", "new-api-dflop-pricing-sync/1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	result.FinalURL = res.Request.URL.String()
	result.HTTPStatus = res.StatusCode
	result.ContentType = res.Header.Get("Content-Type")
	result.ETag = res.Header.Get("ETag")
	if res.StatusCode == http.StatusNotModified {
		result.State = "NOT_MODIFIED"
		return result, nil
	}
	if res.StatusCode != http.StatusOK {
		return result, fmt.Errorf("DFLOP %s: HTTP %d", targetURL, res.StatusCode)
	}
	if !strings.HasPrefix(strings.ToLower(result.ContentType), "application/json") {
		return result, errors.New("DFLOP catalog has non-JSON content type")
	}
	result.Body, err = io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if err != nil {
		return result, err
	}
	if len(result.Body) > 4<<20 {
		return result, errors.New("DFLOP catalog exceeds 4 MiB")
	}
	result.State = "FRESH"
	return result, nil
}

func (c Client) Fetch(ctx context.Context) ([]byte, []byte, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	client := *httpClient
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Hostname() != "api.dflop.top" {
			return errors.New("DFLOP redirected outside the source host")
		}
		return nil
	}
	fetch := func(url string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "new-api-dflop-pricing-sync/1")
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("DFLOP %s: HTTP %d", url, res.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
		if err != nil {
			return nil, err
		}
		if len(body) > 4<<20 {
			return nil, errors.New("DFLOP response exceeds 4 MiB")
		}
		return body, nil
	}
	catalog, err := fetch(CatalogURL)
	if err != nil {
		return nil, nil, err
	}
	currency, err := fetch(CurrencyURL)
	if err != nil {
		return nil, nil, err
	}
	return catalog, currency, nil
}

func Build(catalogJSON, currencyJSON []byte, cnyToUSD, markup string) ([]Item, string, error) {
	return build(catalogJSON, currencyJSON, cnyToUSD, markup, false)
}

type EffectiveMetadata struct {
	SchemaVersion string            `json:"schema_version"`
	Currency      string            `json:"currency"`
	Aliases       map[string]string `json:"aliases"`
}

var knownBillingFeatures = map[string]bool{
	"token": true, "video_second": true, "video_tiers": true, "per_image": true,
	"server_tool_call": true, "video_token": true, "fast_mode": true,
	"token_long_context": true, "video_token_formula_seedance_2_0": true,
	"video_token_formula_seedance_2_5": true, "music": true,
	"video_two_stage": true, "input_images": true, "video_input_seconds": true,
	"image_size_bands": true, "fixed_output_count": true, "video_task": true,
	"avatar": true, "voice_clone": true, "tts_char": true,
}

var knownEffectivePricingKeys = map[string]bool{
	"display_name": true, "input_per_1m": true, "output_per_1m": true, "cached_input_per_1m": true,
	"context_window": true, "default_max_tokens": true, "supports_tools": true, "supports_vision": true,
	"supports_web_search": true, "supports_image_gen": true, "supports_video_generation": true,
	"supports_video_input": true, "supports_file_input": true, "supports_fast_mode": true,
	"category": true, "supports_thinking": true, "adaptive_thinking": true, "supports_prefix": true,
	"supports_json_mode": true, "is_open_source": true, "released_at": true, "is_new": true,
	"supported_protocols": true, "description": true, "protocol": true, "endpoint": true,
	"cache_creation_per_1m": true, "cache_read_per_1m": true, "thoughts_per_1m": true,
	"long_context_threshold_tokens": true, "input_per_1m_long": true, "cached_input_per_1m_long": true,
	"output_per_1m_long": true, "price_per_server_tool_call": true, "callable": true,
	"endpoint_type": true, "price_per_image": true, "price_per_input_image": true,
	"free_input_images": true, "price_per_image_large": true, "large_pixel_threshold": true,
	"images_per_request": true, "price_per_video_second": true, "price_per_video_task": true,
	"image_price_tiers": true, "video_price_tiers": true, "video_token_price_per_1m": true,
	"video_second_stage_per_second": true, "video_bills_input_seconds": true,
	"video_max_input_seconds": true, "price_per_voice_clone": true, "price_per_tts_char": true,
	"price_per_avatar": true, "price_per_music_generation": true, "discount": true,
}

// BuildEffective flattens only the authenticated catalog's nested pricing DTO.
// The public list-price path remains separate and is never used for this cost.
func BuildEffective(catalogJSON, currencyJSON []byte, cnyToUSD, markup string) ([]Item, string, EffectiveMetadata, error) {
	var catalog struct {
		SchemaVersion string            `json:"schema_version"`
		Currency      string            `json:"currency"`
		Aliases       map[string]string `json:"aliases"`
		Models        []json.RawMessage `json:"models"`
	}
	if err := common.Unmarshal(catalogJSON, &catalog); err != nil || catalog.Models == nil || catalog.Aliases == nil {
		return nil, "", EffectiveMetadata{}, errors.New("invalid authenticated DFLOP catalog")
	}
	meta := EffectiveMetadata{SchemaVersion: catalog.SchemaVersion, Currency: catalog.Currency, Aliases: catalog.Aliases}
	if catalog.SchemaVersion != "1.0" {
		return nil, "", meta, fmt.Errorf("SCHEMA_VERSION_CHANGED: unsupported catalog schema %q", catalog.SchemaVersion)
	}
	if catalog.Currency != "points" {
		return nil, "", meta, fmt.Errorf("CURRENCY_SOURCE_CONFLICT: catalog currency %q", catalog.Currency)
	}
	flat := make([]map[string]any, 0, len(catalog.Models))
	for _, raw := range catalog.Models {
		var entry struct {
			ID                 string          `json:"id"`
			VendorSlug         string          `json:"vendor_slug"`
			OfficialModelID    string          `json:"official_model_id"`
			FastModeMultiplier *string         `json:"fast_mode_multiplier"`
			Pricing            json.RawMessage `json:"pricing"`
			Billing            struct {
				Features []string `json:"features"`
			} `json:"billing"`
			Caps json.RawMessage `json:"caps"`
		}
		if err := common.Unmarshal(raw, &entry); err != nil || entry.ID == "" || len(entry.Pricing) == 0 || entry.Billing.Features == nil || len(entry.Caps) == 0 || string(entry.Caps) == "null" {
			return nil, "", meta, errors.New("invalid authenticated DFLOP model contract")
		}
		var rawContract map[string]json.RawMessage
		var billingFields map[string]json.RawMessage
		if err := common.Unmarshal(raw, &rawContract); err != nil {
			return nil, "", meta, fmt.Errorf("model %s: invalid contract", entry.ID)
		}
		if err := common.Unmarshal(rawContract["billing"], &billingFields); err != nil || billingFields == nil {
			return nil, "", meta, fmt.Errorf("model %s: invalid billing contract", entry.ID)
		}
		for name := range billingFields {
			if name != "features" {
				entry.Billing.Features = append(entry.Billing.Features, "unmapped_billing_field:"+name)
			}
		}
		var pricing map[string]any
		if err := common.Unmarshal(entry.Pricing, &pricing); err != nil || pricing == nil {
			return nil, "", meta, fmt.Errorf("model %s: invalid pricing", entry.ID)
		}
		for name := range pricing {
			if !knownEffectivePricingKeys[name] {
				entry.Billing.Features = append(entry.Billing.Features, "unmapped_price_field:"+name)
				delete(pricing, name)
			}
		}
		slices.Sort(entry.Billing.Features)
		pricing["id"] = entry.ID
		pricing["vendor_slug"] = entry.VendorSlug
		pricing["official_model_id"] = entry.OfficialModelID
		pricing["fast_mode_multiplier"] = entry.FastModeMultiplier
		pricing["billing_features"] = entry.Billing.Features
		pricing["caps"] = entry.Caps
		for _, name := range []string{"cache_read_per_1m", "thoughts_per_1m", "image_price_tiers"} {
			if pricing[name] != nil {
				features := pricing["billing_features"].([]string)
				pricing["billing_features"] = append(features, "unmapped_price_field:"+name)
			}
			delete(pricing, name)
		}
		flat = append(flat, pricing)
	}
	encoded, err := common.Marshal(map[string]any{"models": flat})
	if err != nil {
		return nil, "", meta, err
	}
	items, hash, err := build(encoded, currencyJSON, cnyToUSD, markup, true)
	return items, hash, meta, err
}

func build(catalogJSON, currencyJSON []byte, cnyToUSD, markup string, authenticated bool) ([]Item, string, error) {
	var catalog struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := common.Unmarshal(catalogJSON, &catalog); err != nil || catalog.Models == nil {
		return nil, "", errors.New("invalid DFLOP catalog")
	}
	var currency Currency
	if err := common.Unmarshal(currencyJSON, &currency); err != nil {
		return nil, "", fmt.Errorf("invalid DFLOP currency: %w", err)
	}
	if currency.Unit != "" && currency.Unit != "points" {
		return nil, "", fmt.Errorf("unsupported DFLOP currency unit: %s", currency.Unit)
	}
	points, err := decimal.NewFromString(strings.TrimSpace(string(currency.PointsPerCNY)))
	if err != nil || !points.GreaterThan(decimal.Zero) {
		return nil, "", errors.New("invalid DFLOP points_per_cny")
	}
	rate, err := decimal.NewFromString(cnyToUSD)
	if err != nil || !rate.GreaterThan(decimal.Zero) {
		return nil, "", errors.New("invalid CNY to USD rate")
	}
	multiplier, err := decimal.NewFromString(markup)
	if err != nil || !multiplier.GreaterThan(decimal.Zero) {
		return nil, "", errors.New("invalid markup")
	}
	items := make([]Item, 0, len(catalog.Models))
	seen := make(map[string]bool, len(catalog.Models))
	for _, raw := range catalog.Models {
		var source Model
		if err := common.Unmarshal(raw, &source); err != nil {
			return nil, "", fmt.Errorf("invalid DFLOP model: %w", err)
		}
		var canonical map[string]any
		if err := common.Unmarshal(raw, &canonical); err != nil || canonical == nil {
			return nil, "", errors.New("invalid DFLOP model object")
		}
		raw, err = common.Marshal(canonical)
		if err != nil {
			return nil, "", err
		}
		if source.ID == "" || seen[source.ID] {
			return nil, "", fmt.Errorf("missing or duplicate DFLOP model ID: %s", source.ID)
		}
		seen[source.ID] = true
		item := Item{ModelID: source.ID, CanonicalID: source.ID, Category: source.Category, EndpointType: source.EndpointType, Callable: source.Callable, Protocols: source.SupportedProtocols, BillingFeatures: source.BillingFeatures, Discount: source.Discount, Prices: map[string]Price{}, Raw: raw}
		if authenticated {
			item.PriceSemantics = PriceSemantics{SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE", EffectiveState: "VERIFIED"}
		} else {
			item.PriceSemantics = classifyPriceSemantics(source)
		}
		if !source.Callable {
			item.Status, item.Reason = SkippedNonCallable, "model is not callable"
			items = append(items, item)
			continue
		}
		for key := range canonical {
			if !strings.HasPrefix(key, "price_per_") && !strings.HasSuffix(key, "_per_1m") && !strings.Contains(key, "_price_") {
				continue
			}
			switch key {
			case "input_per_1m", "output_per_1m", "cached_input_per_1m", "cache_creation_per_1m",
				"price_per_image", "price_per_image_large", "price_per_input_image", "price_per_video_second", "price_per_video_task",
				"price_per_tts_char", "price_per_music_generation", "price_per_avatar", "price_per_voice_clone", "price_per_server_tool_call",
				"input_per_1m_long", "output_per_1m_long", "cached_input_per_1m_long", "video_price_tiers", "video_token_price_per_1m":
			default:
				return nil, "", fmt.Errorf("model %s: unknown pricing field %s", source.ID, key)
			}
		}
		add := func(name string, unit PricingUnit, value *string) error {
			if value == nil {
				return nil
			}
			credits, err := decimal.NewFromString(*value)
			if err != nil || credits.IsNegative() {
				return fmt.Errorf("model %s: invalid %s", source.ID, name)
			}
			costCNY := credits.DivRound(points, 24)
			costUSD := costCNY.Mul(rate)
			selling := costUSD.Mul(multiplier)
			price := Price{Unit: unit, Credits: credits.String(), CostCNY: costCNY.Round(12).String(), CostUSD: costUSD.Round(12).String(), SellingUSD: selling.Round(12).String(), SourcePriceKind: item.PriceSemantics.SourcePriceKind, PromotionMultiplier: item.PriceSemantics.EffectiveMultiplier, PromotionRuleID: item.PriceSemantics.PromotionRuleID, PromotionSource: item.PriceSemantics.PromotionSource, PromotionState: item.PriceSemantics.EffectiveState}
			isTokenPrice := strings.HasSuffix(name, "_per_1m")
			if item.PriceSemantics.SourcePriceKind == "LIST_PRICE_WITH_VERIFIED_MULTIPLIER" && isTokenPrice {
				effective, _ := decimal.NewFromString(item.PriceSemantics.EffectiveMultiplier)
				effectiveCredits := credits.Mul(effective)
				effectiveCNY := effectiveCredits.DivRound(points, 24)
				price.EffectiveCredits = effectiveCredits.String()
				price.EffectiveCostCNY = effectiveCNY.Round(12).String()
				price.EffectiveCostUSD = effectiveCNY.Mul(rate).Round(12).String()
				price.EffectiveSellingUSD = effectiveCNY.Mul(rate).Mul(multiplier).Round(12).String()
			} else if item.PriceSemantics.SourcePriceKind == "EFFECTIVE_PRICE" || authenticated {
				price.EffectiveCredits, price.EffectiveCostCNY = price.Credits, price.CostCNY
				price.EffectiveCostUSD, price.EffectiveSellingUSD = price.CostUSD, price.SellingUSD
			}
			item.Prices[name] = price
			return nil
		}
		fields := []struct {
			name  string
			unit  PricingUnit
			value *string
		}{
			{"input_per_1m", "token_per_1m", source.InputPer1M}, {"output_per_1m", "token_per_1m", source.OutputPer1M}, {"cached_input_per_1m", "token_per_1m", source.CachedInputPer1M}, {"cache_creation_per_1m", "token_per_1m", source.CacheCreationPer1M},
			{"price_per_image", "image", source.PricePerImage}, {"price_per_image_large", "image", source.PricePerImageLarge}, {"price_per_input_image", "image", source.PricePerInputImage},
			{"price_per_video_second", "second", source.PricePerVideoSecond}, {"price_per_video_task", "request", source.PricePerVideoTask}, {"price_per_tts_char", "character", source.PricePerTTSChar},
			{"price_per_music_generation", "count", source.PricePerMusicGeneration}, {"price_per_avatar", "count", source.PricePerAvatar}, {"price_per_voice_clone", "count", source.PricePerVoiceClone},
			{"price_per_server_tool_call", "request", source.PricePerServerToolCall}, {"input_per_1m_long", "token_per_1m", source.InputPer1MLong}, {"output_per_1m_long", "token_per_1m", source.OutputPer1MLong}, {"cached_input_per_1m_long", "token_per_1m", source.CachedInputPer1MLong},
		}
		for _, field := range fields {
			if err := add(field.name, field.unit, field.value); err != nil {
				return nil, "", err
			}
		}
		for tier, value := range source.VideoPriceTiers {
			if err := add("video_tier:"+tier, "second", &value); err != nil {
				return nil, "", err
			}
		}
		for tier, value := range source.VideoSecondStagePerSecond {
			if err := add("video_second_stage:"+tier, "second", &value); err != nil {
				return nil, "", err
			}
		}
		for tier, value := range source.VideoTokenPricePer1M {
			if err := add("video_token_tier:"+tier, "token_per_1m", &value); err != nil {
				return nil, "", err
			}
		}
		item.Status, item.Reason = UnsupportedMapping, "pricing unit or billing feature needs manual mapping"
		item.ReasonCode = "UNSUPPORTED_PRICE_SHAPE"
		item.PricingShape = pricingShape(source)
		switch source.Category {
		case "text":
			if !authenticated && source.CacheCreationPer1M != nil && strings.Contains(strings.ToLower(source.ID), "claude") {
				item.Reason = "Claude one-hour cache write price is absent"
			} else if len(source.BillingFeatures) == 1 && source.BillingFeatures[0] == "token" {
				input, hasInput := item.Prices["input_per_1m"]
				output, hasOutput := item.Prices["output_per_1m"]
				if hasInput && hasOutput {
					parts := []string{"p * " + input.SellingUSD, "c * " + output.SellingUSD}
					if cache, ok := item.Prices["cached_input_per_1m"]; ok {
						parts = append(parts, "cr * "+cache.SellingUSD)
					}
					if cache, ok := item.Prices["cache_creation_per_1m"]; ok {
						parts = append(parts, "cc * "+cache.SellingUSD)
					}
					item.Expression = "tier(\"dflop\", " + strings.Join(parts, " + ") + ")"
				}
			}
		case "image":
			if len(source.BillingFeatures) == 1 && source.BillingFeatures[0] == "per_image" {
				if price, ok := item.Prices["price_per_image"]; ok {
					item.Expression = "tier(\"image\", fixed(" + price.SellingUSD + ")) * image_count"
				}
			}
		}
		// The catalog does not define whether discount is already reflected in
		// the component prices. Do not charge from an ambiguous effective cost.
		if !authenticated && (source.Discount != nil || item.PriceSemantics.EffectiveState == "CHANGED") {
			item.Expression = ""
			item.Reason = "discounted source price needs manual mapping"
			item.ReasonCode = "AMBIGUOUS_DISCOUNT"
			if item.PriceSemantics.EffectiveState == "CHANGED" {
				item.Reason = "catalog promotion differs from the documented billing multiplier"
				item.ReasonCode = "PROMOTION_STATE_CHANGED"
			} else if item.PriceSemantics.SourcePriceKind == "LIST_PRICE_WITH_VERIFIED_MULTIPLIER" {
				item.Reason = "promoted text model has other billable features requiring runtime mapping"
				item.ReasonCode = "MULTIMODAL_PROMOTION_MAPPING_REQUIRED"
			}
		}
		if item.Expression != "" {
			if err := billing_setting.SmokeTestExpr(item.Expression); err != nil {
				return nil, "", fmt.Errorf("model %s: generated expression: %w", source.ID, err)
			}
			item.Status, item.Reason = SupportedAuto, ""
			item.ReasonCode = ""
		}
		if authenticated && source.CacheCreationPer1M != nil && strings.Contains(strings.ToLower(source.ID), "claude") && item.Expression != "" && item.Status == SupportedAuto {
			// The ordinary 5m path uses the host's durable reservation journal.
			// Unsupported 1h requests are rejected and anomalous usage cannot
			// settle or refund the hold. Runtime captures never promote pricing.
			item.Status, item.ReasonCode, item.Reason = SupportedAuto, "", ""
			item.RequiredFacts = []string{"cache_creation_5m_tokens", "unsupported_1h_guard"}
		}
		if item.Status == UnsupportedMapping && source.Discount == nil {
			classifyTaskPricing(&item, source)
			if item.TaskExpression == "" && item.Expression == "" {
				classifyUnsupportedReason(&item, source)
			}
		}
		if authenticated && item.Status == UnsupportedMapping && item.ReasonCode == "UNSUPPORTED_PRICE_SHAPE" && source.Category == "text" {
			switch {
			case source.CacheCreationPer1M != nil && strings.Contains(strings.ToLower(source.ID), "claude"):
				item.ReasonCode = "DFLOP_CACHE_CONTRACT_RUNTIME_UNVERIFIED"
				item.RequiredFacts = []string{"cache_creation_5m_tokens", "unsupported_1h_guard"}
			case slices.Contains(source.BillingFeatures, "fast_mode"):
				item.ReasonCode = "MISSING_FAST_MODE_MAPPING"
				item.RequiredFacts = []string{"fast_mode_selection", "image_output_count"}
				if slices.Contains(source.BillingFeatures, "token_long_context") {
					item.RequiredFacts = append(item.RequiredFacts, "long_context_price")
				}
			case slices.Contains(source.BillingFeatures, "per_image"):
				item.ReasonCode = "MISSING_IMAGE_TOKEN_MAPPING"
				item.RequiredFacts = []string{"image_output_count"}
			}
		}
		if authenticated && strings.Contains(source.ID, "midjourney") && item.ReasonCode == "UNVERIFIED_OUTPUT_COUNT" {
			item.ReasonCode = "PROVIDER_CONTRACT_FIXED_4_RUNTIME_UNSEEN"
		}
		if authenticated && (strings.HasPrefix(source.ID, "dh-") || source.ID == "clip-compose") && item.Status == UnsupportedMapping {
			item.ReasonCode = "NO_EXACT_RUNTIME_BINDING"
		}
		if authenticated && item.ReasonCode == "UNKNOWN_CHARACTER_COUNT_SEMANTICS" {
			item.ReasonCode = "NO_ASYNC_TTS_BINDING"
			item.RequiredFacts = []string{"completed_characters"}
		}
		if authenticated && item.ReasonCode == "MISSING_SERVER_TOOL_USAGE" {
			item.ReasonCode = "LIVE_CANARY_REQUIRED"
		}
		if authenticated {
			profiles := BuildEndpointBillingProfiles(item)
			if len(profiles) > 0 && profiles[0].Status == SupportedAuto {
				profile := profiles[0]
				item.Expression = profile.Expression
				item.RequiredFacts = slices.Clone(profile.RequiredFacts)
				item.Status, item.ReasonCode, item.Reason = SupportedAuto, "", ""
			}
		}
		if authenticated {
			for _, feature := range source.BillingFeatures {
				if !knownBillingFeatures[feature] {
					item.Status, item.ReasonCode, item.Reason = UnsupportedMapping, "UNKNOWN_BILLING_FEATURE", "unrecognized authenticated billing feature: "+feature
					item.Expression, item.TaskExpression = "", ""
					break
				}
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModelID < items[j].ModelID })
	// Raw source is retained for audit. The hash covers source prices and
	// currency, while selling policy is tied separately by the config hash.
	projection := make([]map[string]any, len(items))
	for i, item := range items {
		prices := make(map[string]map[string]string, len(item.Prices))
		for name, price := range item.Prices {
			prices[name] = map[string]string{"unit": string(price.Unit), "credits": price.Credits}
		}
		protocols := slices.Clone(item.Protocols)
		features := slices.Clone(item.BillingFeatures)
		slices.Sort(protocols)
		slices.Sort(features)
		projection[i] = map[string]any{"id": item.ModelID, "category": item.Category, "endpoint_type": item.EndpointType, "callable": item.Callable, "protocols": protocols, "billing_features": features, "prices": prices, "price_semantics": item.PriceSemantics}
		if item.Discount != nil {
			projection[i]["discount"] = *item.Discount
		}
		var source map[string]any
		if err := common.Unmarshal(item.Raw, &source); err != nil {
			return nil, "", err
		}
		for _, key := range []string{"large_pixel_threshold", "fast_mode_multiplier", "images_per_request", "video_bills_input_seconds", "long_context_threshold_tokens", "supports_fast_mode"} {
			if value, ok := source[key]; ok {
				projection[i][key] = value
			}
		}
	}
	encoded, err := common.Marshal(map[string]any{"points_per_cny": points.String(), "models": projection})
	if err != nil {
		return nil, "", err
	}
	return items, fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}
