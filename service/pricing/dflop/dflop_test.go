package dflop

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	_ "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestEffectiveCatalogHTTPUsesCredentialAndETag(t *testing.T) {
	client := Client{HTTP: &http.Client{Transport: catalogTransport(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.dflop.top/v1/catalog", req.URL.String())
		assert.Equal(t, "Bearer secret", req.Header.Get("Authorization"))
		assert.Equal(t, `"old"`, req.Header.Get("If-None-Match"))
		return &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{"Etag": {`"old"`}, "Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})}}
	result, err := client.FetchEffective(context.Background(), "secret", `"old"`)
	require.NoError(t, err)
	assert.Equal(t, "NOT_MODIFIED", result.State)
	assert.Empty(t, result.Body)
	assert.Equal(t, `"old"`, result.ETag)
}

func TestEffectiveCatalogHTTP200AndMalformedResponse(t *testing.T) {
	status := http.StatusOK
	contentType := "application/json"
	client := Client{HTTP: &http.Client{Transport: catalogTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}, "Etag": {`"cat1"`}}, Body: io.NopCloser(strings.NewReader(`{"schema_version":"1.0"}`)), Request: req}, nil
	})}}
	result, err := client.FetchEffective(context.Background(), "secret", "")
	require.NoError(t, err)
	assert.Equal(t, "FRESH", result.State)
	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.Equal(t, `"cat1"`, result.ETag)
	assert.JSONEq(t, `{"schema_version":"1.0"}`, string(result.Body))
	contentType = "text/html"
	_, err = client.FetchEffective(context.Background(), "secret", "")
	require.ErrorContains(t, err, "non-JSON")
	contentType, status = "application/json", http.StatusUnauthorized
	_, err = client.FetchEffective(context.Background(), "secret", "")
	require.ErrorContains(t, err, "HTTP 401")
}

func TestCatalogCollapseRequiresMaterialShrink(t *testing.T) {
	assert.False(t, collapsed(220, 219))
	assert.True(t, collapsed(220, 106))
	assert.False(t, collapsed(106, 106))
}

func TestSourceChannelBaseURLRejectsArbitraryHosts(t *testing.T) {
	assert.True(t, dflopBaseURL("https://api.dflop.top"))
	assert.True(t, dflopBaseURL("https://api.dflop.top/"))
	for _, target := range []string{"http://api.dflop.top", "https://api.dflop.top.evil.test", "https://api.dflop.top@evil.test", "https://api.dflop.top/private", "https://api.dflop.top?provider=x1"} {
		assert.False(t, dflopBaseURL(target), target)
	}
}

func TestTaskPricingShapesUseExactUsageFacts(t *testing.T) {
	catalog := []byte(`{"models":[
		{"id":"wan2.7-t2v","category":"video","endpoint_type":"videos_generations","callable":true,"billing_features":["video_second","video_tiers"],"price_per_video_second":"60","video_price_tiers":{"720p":"36","1080p":"60"}},
		{"id":"wan3.0-video","category":"video","endpoint_type":"videos_generations","callable":true,"billing_features":["video_input_seconds","video_second","video_tiers"],"video_bills_input_seconds":true,"price_per_video_second":"72","video_price_tiers":{"480p":"18","720p":"36","1080p":"72"}},
		{"id":"qwen-image-3.0","category":"image","endpoint_type":"images_generations","callable":true,"billing_features":["input_images","per_image"],"price_per_image":"10.8","price_per_input_image":"1.2"},
		{"id":"doubao-seedream-4-0-250828","category":"image","endpoint_type":"images_generations","callable":true,"billing_features":["per_image"],"price_per_image":"12"},
		{"id":"doubao-seedream-5-0-pro-260628","category":"image","endpoint_type":"images_generations","callable":true,"billing_features":["image_size_bands","input_images","per_image"],"price_per_image":"18","price_per_image_large":"36","price_per_input_image":"1.2","large_pixel_threshold":2610000}
	]}`)
	items, _, err := Build(catalog, []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ModelID] = item
	}
	video := byID["wan2.7-t2v"]
	assert.Equal(t, UnsupportedMapping, video.Status) // Provider capability is checked during planning.
	assert.Equal(t, []string{"seconds", "resolution"}, video.RequiredFacts)
	videoSchema := map[string]jsplugin.UsageFieldSchema{"seconds": {Type: "number", Unit: "second"}, "resolution": {Enum: []string{"720P", "1080P"}}}
	_, missing, reason := taskPricingCompatibility(video, videoSchema)
	assert.Empty(t, missing)
	assert.Empty(t, reason)
	for _, tc := range []struct {
		name  string
		usage map[string]any
		want  float64
	}{
		{"requested ten but delivered eight", map[string]any{"seconds": 8.0, "resolution": "720P"}, 4.8},
		{"1080p tier", map[string]any{"seconds": 8.0, "resolution": "1080P"}, 8},
		{"zero seconds", map[string]any{"seconds": 0.0, "resolution": "720P"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, _, err := billingexpr.RunExprWithRequest(video.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: tc.usage})
			require.NoError(t, err)
			assert.InDelta(t, tc.want, cost, 1e-9)
		})
	}
	_, missing, reason = taskPricingCompatibility(video, map[string]jsplugin.UsageFieldSchema{"resolution": videoSchema["resolution"]})
	assert.Equal(t, []string{"seconds"}, missing)
	assert.Equal(t, "MISSING_USAGE_SECONDS", reason)
	_, missing, reason = taskPricingCompatibility(video, map[string]jsplugin.UsageFieldSchema{"seconds": videoSchema["seconds"]})
	assert.Equal(t, []string{"resolution"}, missing)
	assert.Equal(t, "MISSING_RESOLUTION_FACT", reason)

	wan3 := byID["wan3.0-video"]
	_, _, reason = taskPricingCompatibility(wan3, map[string]jsplugin.UsageFieldSchema{"seconds": videoSchema["seconds"], "resolution": {Enum: []string{"480P", "720P", "1080P"}}})
	assert.Empty(t, reason)
	cost, _, err := billingexpr.RunExprWithRequest(wan3.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"seconds": 8.0, "resolution": "720P"}})
	require.NoError(t, err)
	assert.InDelta(t, 4.8, cost, 1e-9) // Plugin supplies input + output seconds.

	image := byID["qwen-image-3.0"]
	_, _, reason = taskPricingCompatibility(image, map[string]jsplugin.UsageFieldSchema{"image_count": {Type: "number", Unit: "count"}, "input_image_count": {Type: "number", Unit: "count"}})
	assert.Empty(t, reason)
	cost, _, err = billingexpr.RunExprWithRequest(image.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"image_count": 2.0, "input_image_count": 1.0}})
	require.NoError(t, err)
	assert.InDelta(t, 0.38, cost, 1e-9)

	pro := byID["doubao-seedream-5-0-pro-260628"]
	_, _, reason = taskPricingCompatibility(pro, map[string]jsplugin.UsageFieldSchema{"images_up_to_1_5k": {Type: "number", Unit: "count"}, "images_above_1_5k": {Type: "number", Unit: "count"}, "input_images": {Type: "number", Unit: "count"}})
	assert.Empty(t, reason)
	cost, _, err = billingexpr.RunExprWithRequest(pro.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"images_up_to_1_5k": 1.0, "images_above_1_5k": 2.0, "input_images": 3.0}})
	require.NoError(t, err)
	assert.InDelta(t, 1.54, cost, 1e-9)

	planned, err := Plan(items, model.DefaultDFLOPConfig(), nil, nil)
	require.NoError(t, err)
	for _, row := range planned {
		assert.Equal(t, SupportedAuto, row.Status, row.ModelID)
		assert.Equal(t, "PLUGIN_OVERRIDE", row.PricingScope, row.ModelID)
		assert.Equal(t, "ADD", row.Action, row.ModelID)
	}
	for _, row := range planned {
		if row.ModelID == "doubao-seedream-4-0-250828" {
			assert.Equal(t, `tier("image", u("image_count") * 0.2)`, row.Expression)
		}
	}
}

func TestBuildCatalogPreservesUnitsAndPrices(t *testing.T) {
	catalog := []byte(`{"models":[
		{"id":"text","category":"text","callable":true,"supported_protocols":["openai_chat"],"billing_features":["token"],"input_per_1m":"120","output_per_1m":"240","cached_input_per_1m":"60"},
		{"id":"free","category":"text","callable":true,"billing_features":["token"],"input_per_1m":"0","output_per_1m":"0"},
		{"id":"image","category":"image","callable":true,"endpoint_type":"images_generations","billing_features":["per_image"],"price_per_image":"24"},
		{"id":"video","category":"video","callable":true,"endpoint_type":"videos_generations","billing_features":["video_second"],"price_per_video_second":"10"},
		{"id":"placeholder","category":"video","callable":false,"price_per_video_second":"10"}
	]}`)
	items, _, err := Build(catalog, []byte(`{"unit":"points","points_per_cny":60}`), "0.15", "1.2")
	require.NoError(t, err)
	require.Len(t, items, 5)
	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ModelID] = item
	}
	assert.Equal(t, SupportedAuto, byID["text"].Status)
	assert.Equal(t, "tier(\"dflop\", p * 0.36 + c * 0.72 + cr * 0.18)", byID["text"].Expression)
	assert.Equal(t, "tier(\"dflop\", p * 0 + c * 0)", byID["free"].Expression)
	assert.Equal(t, "tier(\"image\", fixed(0.072)) * image_count", byID["image"].Expression)
	assert.Equal(t, UnsupportedMapping, byID["video"].Status)
	assert.Equal(t, SkippedNonCallable, byID["placeholder"].Status)
}

func TestBuildEffectiveCatalogUsesChannelPriceWithoutPromotion(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{"sonnet":"claude-sonnet-4-6"},"models":[
		{"id":"claude-sonnet-4-6","vendor_slug":"anthropic","pricing":{"category":"text","callable":true,"input_per_1m":"0.7279","output_per_1m":"1.4558","discount":null},"billing":{"features":["token"]},"caps":{"surfaces":["chat"]}},
		{"id":"gpt-6-sol","vendor_slug":"openai","pricing":{"category":"text","callable":true,"input_per_1m":"3","output_per_1m":"6","discount":null},"billing":{"features":["token"]},"caps":{"surfaces":["chat"]}}
	]}`)
	items, hash, metadata, err := BuildEffective(catalog, []byte(`{"unit":"points","points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	assert.Equal(t, "1.0", metadata.SchemaVersion)
	assert.Equal(t, "claude-sonnet-4-6", metadata.Aliases["sonnet"])
	for _, item := range items {
		assert.Equal(t, SupportedAuto, item.Status)
		assert.Equal(t, "AUTHENTICATED_EFFECTIVE_PRICE", item.PriceSemantics.SourcePriceKind)
		assert.Empty(t, item.Prices["input_per_1m"].PromotionMultiplier)
	}
	assert.Equal(t, "0.7279", items[0].Prices["input_per_1m"].Credits)
	assert.Equal(t, "0.012131666667", items[0].Prices["input_per_1m"].CostCNY)
	assert.Equal(t, "3", items[1].Prices["input_per_1m"].Credits)
}

func TestBuildEffectiveCatalogFailsClosedOnUnknownContract(t *testing.T) {
	base := `{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"future","pricing":{"category":"text","callable":true,"input_per_1m":"1","output_per_1m":"2"},"billing":{"features":["future_charge"]},"caps":{"surfaces":["chat"]}}]}`
	items, _, _, err := BuildEffective([]byte(base), []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", items[0].ReasonCode)
	assert.Equal(t, UnsupportedMapping, items[0].Status)
	items, _, _, err = BuildEffective([]byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"future-price","pricing":{"category":"text","callable":true,"input_per_1m":"1","output_per_1m":"2","future_price":"3"},"billing":{"features":["token"]},"caps":{"surfaces":["chat"]}}]}`), []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", items[0].ReasonCode)
	_, _, _, err = BuildEffective([]byte(`{"schema_version":"2.0","currency":"points","aliases":{},"models":[]}`), []byte(`{"points_per_cny":60}`), "1", "1")
	require.ErrorContains(t, err, "SCHEMA_VERSION_CHANGED")
	_, _, _, err = BuildEffective([]byte(`{"schema_version":"1.0","currency":"usd","aliases":{},"models":[]}`), []byte(`{"unit":"points","points_per_cny":60}`), "1", "1")
	require.ErrorContains(t, err, "CURRENCY_SOURCE_CONFLICT")
}

func TestBuildEffectiveCatalogPreservesCompositeVideoComponents(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"seedance-lite","pricing":{"category":"video","endpoint_type":"videos_generations","callable":true,"price_per_video_second":"60","video_price_tiers":{"720p":"60","1080p":"120"},"video_token_price_per_1m":{"default@720p":"2400"}},"billing":{"features":["video_second","video_tiers","video_token","video_two_stage"]},"caps":{"surfaces":["video"]}}]}`)
	items, _, _, err := BuildEffective(catalog, []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, UnsupportedMapping, items[0].Status)
	assert.Equal(t, "60", items[0].Prices["price_per_video_second"].Credits)
	assert.Equal(t, "120", items[0].Prices["video_tier:1080p"].Credits)
	assert.Equal(t, "2400", items[0].Prices["video_token_tier:default@720p"].Credits)
	assert.Empty(t, items[0].Expression)
}

func TestEffectiveMediaPricesRemainSourceDrivenWhileBindingsAreUnverified(t *testing.T) {
	for _, tc := range []struct {
		name, category, endpoint, feature, priceField, points, usd, reason string
	}{
		{"voice-tts-pro", "audio", "tts_synthesize", "tts_char", "price_per_tts_char", "0.132066", "0.000330165", "NO_ASYNC_TTS_BINDING"},
		{"voice-clone-pro", "audio", "voice_clone", "voice_clone", "price_per_voice_clone", "1800", "4.5", "NO_EXACT_VOICE_CLONE_BINDING"},
		{"voice-clone-pro", "audio", "voice_clone", "voice_clone", "price_per_voice_clone", "2400", "6", "NO_EXACT_VOICE_CLONE_BINDING"},
		{"suno-v5", "audio", "music_generations", "music", "price_per_music_generation", "20.4", "0.051", "NO_EXACT_MUSIC_PLUGIN_BINDING"},
		{"clip-compose", "video", "videos_generations", "video_task", "price_per_video_task", "48", "0.12", "NO_EXACT_RUNTIME_BINDING"},
	} {
		t.Run(tc.name+"/"+tc.points, func(t *testing.T) {
			catalog := fmt.Sprintf(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":%q,"pricing":{"category":%q,"endpoint_type":%q,"callable":true,%q:%q},"billing":{"features":[%q]},"caps":{"surfaces":["task"]}}]}`, tc.name, tc.category, tc.endpoint, tc.priceField, tc.points, tc.feature)
			items, _, _, err := BuildEffective([]byte(catalog), []byte(`{"unit":"points","points_per_cny":60}`), "0.15", "1")
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, tc.points, items[0].Prices[tc.priceField].Credits)
			assert.Equal(t, tc.usd, items[0].Prices[tc.priceField].CostUSD)
			planned, err := Plan(items, model.DefaultDFLOPConfig(), nil, nil)
			require.NoError(t, err)
			assert.Equal(t, UnsupportedMapping, planned[0].Status)
			assert.Equal(t, tc.reason, planned[0].ReasonCode)
		})
	}
}

func TestBuildRejectsMalformedMoneyWithoutChangingCatalog(t *testing.T) {
	_, _, err := Build([]byte(`{"models":[{"id":"bad","category":"text","callable":true,"input_per_1m":"oops","output_per_1m":"2"}]}`), []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.Error(t, err)
	_, _, err = Build([]byte(`{"models":[]}`), []byte(`{"points_per_cny":0}`), "0.15", "1")
	require.Error(t, err)
	_, _, err = Build([]byte(`{"models":[{"id":"future","category":"text","callable":true,"billing_features":["token"],"input_per_1m":"1","output_per_1m":"2","price_per_new_feature":"3"}]}`), []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.ErrorContains(t, err, "unknown pricing field")
}

func TestSourceHashIgnoresJSONFieldAndModelOrder(t *testing.T) {
	first := []byte(`{"models":[{"id":"a","category":"video","callable":false},{"id":"b","category":"video","callable":false}]}`)
	second := []byte(`{"models":[{"callable":false,"category":"video","id":"b"},{"category":"video","id":"a","callable":false}]}`)
	currency := []byte(`{"points_per_cny":60}`)
	_, firstHash, err := Build(first, currency, "0.15", "1")
	require.NoError(t, err)
	_, secondHash, err := Build(second, currency, "0.15", "1")
	require.NoError(t, err)
	assert.Equal(t, firstHash, secondHash)
}

func TestBuildDoesNotGuessClaudeOneHourCachePrice(t *testing.T) {
	items, _, err := Build([]byte(`{"models":[{"id":"claude-example","category":"text","callable":true,"billing_features":["token"],"input_per_1m":"1","output_per_1m":"2","cache_creation_per_1m":"3"}]}`), []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, items[0].Status)
	assert.Empty(t, items[0].Expression)
}

func TestBuildEffectiveMapsDFLOPFiveMinuteCacheContract(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"claude-example","pricing":{"category":"text","callable":true,"input_per_1m":"1","output_per_1m":"2","cache_creation_per_1m":"3"},"billing":{"features":["token"]},"caps":{}}]}`)
	items, _, _, err := BuildEffective(catalog, []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, SupportedAuto, items[0].Status)
	assert.Equal(t, "", items[0].ReasonCode)
	assert.Contains(t, items[0].Expression, "cc * ")
	assert.NotContains(t, items[0].Expression, "cc1h")
}

func TestSourceHashDoesNotChangeWithSellingPolicy(t *testing.T) {
	catalog := []byte(`{"models":[{"id":"text","category":"text","callable":true,"billing_features":["token"],"input_per_1m":"100","output_per_1m":"200"}]}`)
	currency := []byte(`{"points_per_cny":60}`)
	_, first, err := Build(catalog, currency, "0.15", "1")
	require.NoError(t, err)
	_, second, err := Build(catalog, currency, "0.2", "1.25")
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestSourceHashChangesWithBillingSemantics(t *testing.T) {
	currency := []byte(`{"points_per_cny":60}`)
	first := []byte(`{"models":[{"id":"tiered","category":"image","callable":true,"billing_features":["image_size_bands","per_image"],"price_per_image":"18","price_per_image_large":"36","large_pixel_threshold":2610000}]}`)
	second := []byte(`{"models":[{"id":"tiered","category":"image","callable":true,"billing_features":["image_size_bands","per_image"],"price_per_image":"18","price_per_image_large":"36","large_pixel_threshold":2250000}]}`)
	_, oldHash, err := Build(first, currency, "0.15", "1")
	require.NoError(t, err)
	_, newHash, err := Build(second, currency, "0.15", "1")
	require.NoError(t, err)
	assert.NotEqual(t, oldHash, newHash)
}

func TestUnknownCharacterUnitRemainsUnsupported(t *testing.T) {
	items, _, err := Build([]byte(`{"models":[{"id":"voice","category":"audio","callable":true,"billing_features":["tts_char"],"price_per_tts_char":"1"}]}`), []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, items[0].Status)
	assert.Equal(t, "UNKNOWN_CHARACTER_COUNT_SEMANTICS", items[0].ReasonCode)
}

func TestKnownVideoShapeWithoutExactPluginBindingStaysUnsupported(t *testing.T) {
	items, _, err := Build([]byte(`{"models":[{"id":"unbound-wan-like-video","category":"video","endpoint_type":"videos_generations","callable":true,"billing_features":["video_second","video_tiers"],"price_per_video_second":"60","video_price_tiers":{"720p":"36","1080p":"60"}}]}`), []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.NoError(t, err)
	require.NotEmpty(t, items[0].TaskExpression)
	planned, err := Plan(items, model.DefaultDFLOPConfig(), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, planned[0].Status)
	assert.Equal(t, "NO_EXACT_VIDEO_PLUGIN_BINDING", planned[0].ReasonCode)
}

func TestBuildDoesNotAssumeDiscountSemantics(t *testing.T) {
	items, _, err := Build([]byte(`{"models":[{"id":"discounted","category":"text","callable":true,"billing_features":["token"],"discount":"0.3","input_per_1m":"100","output_per_1m":"200"}]}`), []byte(`{"points_per_cny":60}`), "0.15", "1")
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, items[0].Status)
	assert.Empty(t, items[0].Expression)
}

func TestPromotionSemanticsFailClosedOnChangedOrMissingBadge(t *testing.T) {
	catalog := []byte(`{"models":[
		{"id":"gpt-5.5","vendor_slug":"openai","category":"text","callable":true,"billing_features":["token","per_image","fast_mode"],"discount":"0.3","input_per_1m":"2022","cached_input_per_1m":"202.2","output_per_1m":"12132","price_per_image":"50"},
		{"id":"gpt-6-sol","vendor_slug":"openai","category":"text","callable":true,"billing_features":["token"],"discount":"0.3","input_per_1m":"1","cached_input_per_1m":"80.88","output_per_1m":"4044"},
		{"id":"claude-sonnet-4-6","vendor_slug":"anthropic","category":"text","callable":true,"billing_features":["token"],"discount":"0.8","input_per_1m":"100","cached_input_per_1m":"10","cache_creation_per_1m":"125","output_per_1m":"200"},
		{"id":"claude-sonnet-5","vendor_slug":"anthropic","category":"text","callable":true,"billing_features":["token"],"input_per_1m":"100","output_per_1m":"200"},
		{"id":"codex-auto-review","vendor_slug":"openai","category":"text","callable":true,"billing_features":["token"],"discount":"0.3","input_per_1m":"100","output_per_1m":"200"}
	]}`)
	items, _, err := Build(catalog, []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		byID[item.ModelID] = item
	}
	gpt := byID["gpt-5.5"]
	assert.Equal(t, "LIST_PRICE_WITH_VERIFIED_MULTIPLIER", gpt.PriceSemantics.SourcePriceKind)
	assert.Equal(t, "MULTIMODAL_PROMOTION_MAPPING_REQUIRED", gpt.ReasonCode)
	assert.Equal(t, "606.6", gpt.Prices["input_per_1m"].EffectiveCredits)
	assert.Equal(t, "60.66", gpt.Prices["cached_input_per_1m"].EffectiveCredits)
	assert.Equal(t, "3639.6", gpt.Prices["output_per_1m"].EffectiveCredits)
	assert.Empty(t, gpt.Prices["price_per_image"].EffectiveCredits)
	for _, id := range []string{"claude-sonnet-4-6", "claude-sonnet-5", "gpt-6-sol"} {
		item := byID[id]
		assert.Equal(t, UnsupportedMapping, item.Status)
		assert.Equal(t, "PROMOTION_STATE_CHANGED", item.ReasonCode)
		assert.Empty(t, item.Expression)
		assert.Empty(t, item.Prices["input_per_1m"].EffectiveCredits)
	}
	assert.Equal(t, "AMBIGUOUS_DISCOUNT", byID["codex-auto-review"].ReasonCode)
}
