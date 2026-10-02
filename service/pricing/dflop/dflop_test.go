package dflop

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
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
	items, _, _, err = BuildEffective([]byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"future-basis","pricing":{"category":"text","callable":true,"input_per_1m":"1","output_per_1m":"2"},"billing":{"features":["token"],"additional_per_second":"5"},"caps":{"surfaces":["chat"]}}]}`), []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, items[0].Status)
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", items[0].ReasonCode)
	assert.Contains(t, items[0].BillingFeatures, "unmapped_billing_field:additional_per_second")
	assert.Empty(t, items[0].Expression)
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

func TestEffectiveMediaPricesRemainSourceDrivenWithExactBindings(t *testing.T) {
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
			if tc.name == "voice-tts-pro" {
				assert.Equal(t, SupportedAuto, planned[0].Status)
				assert.Empty(t, planned[0].ReasonCode)
				assert.Equal(t, "dflop-tts", planned[0].PluginKey)
				assert.Contains(t, planned[0].Expression, `u("character_count")`)
			} else {
				assert.Equal(t, SupportedAuto, planned[0].Status)
				assert.Empty(t, planned[0].ReasonCode)
				assert.Equal(t, "dflop-media", planned[0].PluginKey)
			}
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

func TestDFLOPTaskProfilesKeepDistinctBillingUnits(t *testing.T) {
	unitPrice := "6"
	for _, tc := range []struct {
		name   string
		source Model
		prices map[string]Price
		usage  map[string]any
		want   float64
		plugin string
	}{
		{"delivered seconds", Model{ID: "grok-imagine-video", Category: "video", EndpointType: "videos_generations", BillingFeatures: []string{"video_second"}, PricePerVideoSecond: &unitPrice}, map[string]Price{"price_per_video_second": {SellingUSD: "0.1"}}, map[string]any{"duration_sec": 4.25}, 0.425, "dflop-media"},
		{"one music generation", Model{ID: "suno-v5", Category: "audio", EndpointType: "music_generations", BillingFeatures: []string{"music"}, PricePerMusicGeneration: &unitPrice}, map[string]Price{"price_per_music_generation": {SellingUSD: "0.1"}}, map[string]any{"generation_count": 1}, 0.1, "dflop-media"},
		{"successful fixed compose", Model{ID: "clip-compose", Category: "video", EndpointType: "videos_generations", BillingFeatures: []string{"video_task"}, PricePerVideoTask: &unitPrice}, map[string]Price{"price_per_video_task": {SellingUSD: "0.1"}}, map[string]any{"count": 1}, 0.1, "dflop-media"},
		{"terminal characters", Model{ID: "voice-tts-pro", Category: "audio", EndpointType: "tts_synthesize", BillingFeatures: []string{"tts_char"}, PricePerTTSChar: &unitPrice}, map[string]Price{"price_per_tts_char": {SellingUSD: "0.1"}}, map[string]any{"character_count": 26}, 2.6, "dflop-tts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := Item{ModelID: tc.source.ID, Prices: tc.prices}
			classifyTaskPricing(&item, tc.source)
			assert.Equal(t, tc.plugin, item.TaskPlugin)
			require.NotEmpty(t, item.TaskExpression)
			plugin, ok := jsplugin.DefaultRegistry.Generation().Get(tc.plugin)
			require.True(t, ok)
			schema, _ := plugin.Meta.UsageForModel(tc.source.ID)
			_, missing, reason := taskPricingCompatibility(item, schema)
			assert.Empty(t, missing)
			assert.Empty(t, reason)
			cost, _, err := billingexpr.RunExprWithRequest(item.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: tc.usage})
			require.NoError(t, err)
			assert.InDelta(t, tc.want, cost, 1e-12)
		})
	}
}

func TestDFLOPSeedanceFinalTokenTierAndLitePriceGate(t *testing.T) {
	base := "60"
	for _, tc := range []struct {
		model      string
		formula    string
		rates      map[string]string
		resolution string
		mode       string
		want       float64
	}{
		{"doubao-seedance-2.0", "video_token_formula_seedance_2_0", map[string]string{"default@480p": "1", "default@720p": "2", "default@1080p": "3", "with_video_input@480p": "4", "with_video_input@720p": "5", "with_video_input@1080p": "6"}, "720p", "default", 0.002},
		{"doubao-seedance-2.0", "video_token_formula_seedance_2_0", map[string]string{"default@480p": "1", "default@720p": "2", "default@1080p": "3", "with_video_input@480p": "4", "with_video_input@720p": "5", "with_video_input@1080p": "6"}, "1080p", "with_video_input", 0.006},
		{"doubao-seedance-2.5", "video_token_formula_seedance_2_5", map[string]string{"default": "1", "default@1080p": "2", "with_video_input": "3", "with_video_input@1080p": "4"}, "480p", "default", 0.001},
		{"doubao-seedance-2.5", "video_token_formula_seedance_2_5", map[string]string{"default": "1", "default@1080p": "2", "with_video_input": "3", "with_video_input@1080p": "4"}, "720p", "with_video_input", 0.003},
	} {
		t.Run(tc.model+tc.resolution+tc.mode, func(t *testing.T) {
			source := Model{ID: tc.model, Category: "video", EndpointType: "videos_generations", PricePerVideoSecond: &base, VideoPriceTiers: map[string]string{"480p": "60", "720p": "60", "1080p": "60"}, VideoTokenPricePer1M: tc.rates, BillingFeatures: []string{"video_second", "video_tiers", "video_token", tc.formula}}
			item := Item{ModelID: source.ID, Prices: map[string]Price{"price_per_video_second": {SellingUSD: "99"}}}
			for tier := range source.VideoPriceTiers {
				item.Prices["video_tier:"+tier] = Price{SellingUSD: "99"}
			}
			for tier, rate := range tc.rates {
				item.Prices["video_token_tier:"+tier] = Price{SellingUSD: rate}
			}
			classifyTaskPricing(&item, source)
			require.Equal(t, "dflop-media", item.TaskPlugin)
			plugin, _ := jsplugin.DefaultRegistry.Generation().Get("dflop-media")
			schema, _ := plugin.Meta.UsageForModel(tc.model)
			_, missing, reason := taskPricingCompatibility(item, schema)
			require.Empty(t, missing)
			require.Empty(t, reason)
			cost, _, err := billingexpr.RunExprWithRequest(item.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"completion_tokens": 1000.0, "resolution": tc.resolution, "input_mode": tc.mode}})
			require.NoError(t, err)
			assert.InDelta(t, tc.want, cost, 1e-12)
			assert.NotContains(t, item.TaskExpression, "duration_sec")
		})
	}
	source := Model{ID: "doubao-seedance-2.0-lite", Category: "video", EndpointType: "videos_generations", PricePerVideoSecond: &base, VideoPriceTiers: map[string]string{"720p": "1", "1080p": "1"}, VideoTokenPricePer1M: map[string]string{"default@720p": "1", "default@1080p": "1", "with_video_input@720p": "1", "with_video_input@1080p": "1"}, BillingFeatures: []string{"video_second", "video_tiers", "video_token", "video_token_formula_seedance_2_0", "video_two_stage"}}
	item := Item{ModelID: source.ID}
	classifyTaskPricing(&item, source)
	assert.Equal(t, "PROVIDER_CATALOG_MISSING_UPSCALE_RATE", item.ReasonCode)
	assert.Empty(t, item.TaskExpression)
	// Machine-readable authenticated add-on rates are a separate delivery leg.
	for _, model := range []string{"doubao-seedance-2.0-lite", "doubao-seedance-2.0-fast-lite", "doubao-seedance-2.0-mini-lite", "doubao-seedance-2.5-lite"} {
		source.ID = model
		if model == "doubao-seedance-2.5-lite" {
			source.BillingFeatures = []string{"video_second", "video_tiers", "video_token", "video_token_formula_seedance_2_5", "video_two_stage"}
		}
		source.VideoSecondStagePerSecond = map[string]string{"720p": "7.125", "1080p": "11.25"}
		for _, tc := range []struct {
			tier string
			want float64
		}{{"720p", 21.376}, {"1080p", 33.751}} {
			t.Run(model+tc.tier, func(t *testing.T) {
				item := Item{ModelID: model, Prices: map[string]Price{"price_per_video_second": {SellingUSD: "99"}}}
				for tier := range source.VideoPriceTiers {
					item.Prices["video_tier:"+tier] = Price{SellingUSD: "99"}
				}
				for tier := range source.VideoTokenPricePer1M {
					item.Prices["video_token_tier:"+tier] = Price{SellingUSD: "1"}
				}
				for tier, rate := range source.VideoSecondStagePerSecond {
					item.Prices["video_second_stage:"+tier] = Price{SellingUSD: rate}
				}
				classifyTaskPricing(&item, source)
				require.NotEmpty(t, item.TaskExpression)
				cost, _, err := billingexpr.RunExprWithRequest(item.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"completion_tokens": 1000.0, "duration_sec": 3.0, "resolution": tc.tier, "input_mode": "default"}})
				require.NoError(t, err)
				assert.InDelta(t, tc.want, cost, 1e-12)
			})
		}
	}
}

func TestDFLOPH3InputDurationRequiresExplicitBillingBasis(t *testing.T) {
	for _, tc := range []struct {
		input  bool
		reason string
	}{{false, "NO_EXACT_VIDEO_PLUGIN_BINDING"}, {true, "MISSING_INPUT_VIDEO_DURATION"}} {
		t.Run(tc.reason, func(t *testing.T) {
			source := Model{ID: "minimax-h3", Category: "video", BillingFeatures: []string{"video_second", "video_tiers"}, VideoBillsInputSeconds: &tc.input}
			if tc.input {
				source.BillingFeatures = append(source.BillingFeatures, "video_input_seconds")
			}
			item := Item{}
			classifyUnsupportedReason(&item, source)
			assert.Equal(t, tc.reason, item.ReasonCode)
			assert.Equal(t, tc.input, slices.Contains(item.RequiredFacts, "input_video_duration_sec"))
		})
	}
}

func TestDFLOPSubtitleOperationTiersCannotBecomeVideoResolutionPricing(t *testing.T) {
	price := "0.1011"
	source := Model{ID: "tvod-subtitle-soft", Category: "video", EndpointType: "videos_generations", BillingFeatures: []string{"video_second", "video_tiers"}, PricePerVideoSecond: &price, VideoPriceTiers: map[string]string{"asr": "0.06066", "translate": "0.04044"}}
	item := Item{Prices: map[string]Price{"price_per_video_second": {SellingUSD: "0.1011"}, "video_tier:asr": {SellingUSD: "0.06066"}, "video_tier:translate": {SellingUSD: "0.04044"}}}
	classifyTaskPricing(&item, source)
	assert.Empty(t, item.TaskExpression)
	classifyUnsupportedReason(&item, source)
	assert.Equal(t, "MISSING_SUBTITLE_SOURCE_DURATION", item.ReasonCode)
	assert.Equal(t, []string{"source_duration_sec", "asr_units", "translation_units"}, item.RequiredFacts)
}

func TestDFLOPAlreadyMappedTokenModelReceivesRouteAwarePreviewExpression(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"grok-3-mini","pricing":{"category":"text","callable":true,"endpoint_type":"chat","supported_protocols":["openai_chat","openai_responses"],"input_per_1m":"60","output_per_1m":"120","cached_input_per_1m":"6"},"billing":{"features":["token"]},"caps":{"surfaces":["chat"]}}]}`)
	items, _, _, err := BuildEffective(catalog, []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, SupportedAuto, items[0].Status)
	assert.True(t, HasResponsesEndpointBillingProfile(items[0].Expression))
	assert.NoError(t, DFLOPEndpointRequestContract("https://api.dflop.top", items[0].Expression, "/v1/responses", billingexpr.RequestInput{Body: []byte(`{"model":"grok-3-mini","input":"hello"}`)}))
}

// These cases prevent catalog changes from silently authorizing billing.
func TestProviderContractAuditFailClosed(t *testing.T) {
	models := []string{"doubao-seedance-2.0-lite", "doubao-seedance-2.0-fast-lite", "doubao-seedance-2.0-mini-lite", "doubao-seedance-2.5-lite"}
	catalog := func(component string, extra string) []byte {
		entries := make([]string, 0, len(models))
		for _, id := range models {
			entries = append(entries, fmt.Sprintf(`{"id":%q,"pricing":{"category":"video","endpoint_type":"videos_generations","callable":true,"supported_protocols":["openai_video"],"video_second_stage_per_second":%s%s},"billing":{"features":["video_token","video_two_stage"]},"caps":{"surfaces":["video"]}}`, id, component, extra))
		}
		return []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[` + strings.Join(entries, ",") + `]}`)
	}
	previous := catalog("null", "")
	for _, tc := range []struct{ name, component, extra, status string }{
		{"missing", "null", "", "UNCHANGED"},
		{"effective rates added", `{"720p":"1.5","1080p":"2"}`, "", "RESOLVED_BY_CATALOG"},
		{"partial rate", `{"720p":"1.5"}`, "", "STILL_CONFLICTING"},
		{"wrong decimal type", `{"720p":1.5,"1080p":"2"}`, "", "NEW_CONFLICT"},
		{"negative rate", `{"720p":"-1","1080p":"2"}`, "", "NEW_CONFLICT"},
		{"zero rate", `{"720p":"0","1080p":"2"}`, "", "NEW_CONFLICT"},
		{"oversized rate", `{"720p":"1e1000000000","1080p":"2"}`, "", "NEW_CONFLICT"},
		{"unknown component", `{"720p":"1.5","1080p":"2"}`, `,"new_billable_component":"3"`, "NEW_CONFLICT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := EvaluateProviderContracts(catalog(tc.component, tc.extra), previous)
			require.NoError(t, err)
			require.NotEmpty(t, report.Blockers)
			assert.Equal(t, tc.status, report.Blockers[0].Status)
			assert.True(t, report.FreshPreviewRequired)
			assert.False(t, report.AutoPromotes)
			assert.False(t, report.PricingApplied)
			assert.False(t, report.AutoApply)
			assert.Equal(t, 4, report.CallableCount)
			assert.Len(t, report.CatalogHash, 64)
			assert.Len(t, report.Models, 4)
		})
	}
}

func TestProviderContractAuditDoesNotTrustGuessedProviderFields(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"minimax-h3","pricing":{"callable":true,"video_bills_input_seconds":true},"billing":{"features":["video_input_seconds"],"billing_basis":"output_seconds","terminal_quantity":"duration_sec"},"caps":{}},{"id":"qwen-image-3.0-pro","pricing":{"callable":true,"large_pixel_threshold":2097152},"billing":{"features":["per_image","image_size_bands"]},"caps":{}}]}`)
	report, err := EvaluateProviderContracts(catalog, nil)
	require.NoError(t, err)
	for _, blocker := range report.Blockers {
		assert.NotEqual(t, "RESOLVED_BY_CATALOG", blocker.Status)
		require.NotEmpty(t, blocker.MissingFacts)
		require.NotEmpty(t, blocker.UnblockPredicates)
		if blocker.ID == "minimax_h3" {
			assert.Equal(t, "NEW_CONFLICT", blocker.Status)
		}
		if blocker.ID == "qwen_threshold_domain" {
			assert.Equal(t, "STILL_CONFLICTING", blocker.Status)
		}
	}
}

func TestProviderContractAuditRetainsDisappearingProfileBlockers(t *testing.T) {
	previous := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"gpt-6-sol","pricing":{"callable":true,"price_per_image":"2"},"billing":{"features":["token","fast_mode"]},"caps":{}}]}`)
	current := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"gpt-6-sol","pricing":{"callable":true},"billing":{"features":["token"]},"caps":{}}]}`)
	report, err := EvaluateProviderContracts(current, previous)
	require.NoError(t, err)
	for _, blocker := range report.Blockers {
		if blocker.ID == "gpt_fast" || blocker.ID == "dedicated_image" {
			assert.Equal(t, []string{"gpt-6-sol"}, blocker.Models)
			assert.Equal(t, "STILL_CONFLICTING", blocker.Status)
		}
	}
}

func TestProviderContractAuditRejectsSemanticTypeChanges(t *testing.T) {
	for _, field := range []string{`"video_bills_input_seconds":"true"`, `"video_max_input_seconds":"100"`, `"large_pixel_threshold":2.5`} {
		catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"minimax-h3","pricing":{"callable":true,` + field + `},"billing":{"features":["video_input_seconds"]},"caps":{}}]}`)
		report, err := EvaluateProviderContracts(catalog, nil)
		require.NoError(t, err)
		assert.Equal(t, "NEW_CONFLICT", report.Blockers[1].Status)
	}
}

func TestProviderContractAuditRequiresLiteBindingAndIgnoresNullProfilePrices(t *testing.T) {
	models := []string{"doubao-seedance-2.0-lite", "doubao-seedance-2.0-fast-lite", "doubao-seedance-2.0-mini-lite", "doubao-seedance-2.5-lite"}
	entries := []string{`{"id":"grok-3-mini","pricing":{"callable":true,"price_per_server_tool_call":null},"billing":{"features":["token"]},"caps":{}}`, `{"id":"gpt-6-sol","pricing":{"callable":true,"price_per_image":null},"billing":{"features":["token"]},"caps":{}}`}
	for _, id := range models {
		entries = append(entries, fmt.Sprintf(`{"id":%q,"pricing":{"callable":true,"endpoint_type":"chat","video_second_stage_per_second":{"720p":"1","1080p":"2"}},"billing":{"features":["video_token","video_two_stage"]},"caps":{}}`, id))
	}
	report, err := EvaluateProviderContracts([]byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[`+strings.Join(entries, ",")+`]}`), nil)
	require.NoError(t, err)
	assert.NotEqual(t, "RESOLVED_BY_CATALOG", report.Blockers[0].Status)
	assert.Empty(t, report.Blockers[5].Models)
	assert.Empty(t, report.Blockers[6].Models)
}

func TestDocumentedContractOverridesAndRuntimeQuantities(t *testing.T) {
	source, err := builtinplugins.Source("dflop-media")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-media"})
	require.NoError(t, err)
	for _, id := range []string{"doubao-seedance-2.0-fast-lite", "doubao-seedance-2.0-lite", "doubao-seedance-2.0-mini-lite", "doubao-seedance-2.5-lite"} {
		t.Run(id, func(t *testing.T) {
			catalog := fmt.Sprintf(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":%q,"pricing":{"category":"video","endpoint_type":"videos_generations","callable":true,"price_per_video_second":"60","video_price_tiers":{"720p":"60","1080p":"120"},"video_token_price_per_1m":{"default@720p":"60","default@1080p":"120","with_video_input@720p":"30","with_video_input@1080p":"90"},"video_second_stage_per_second":null},"billing":{"features":["video_second","video_tiers","video_token","video_token_formula_seedance_%s","video_two_stage"]},"caps":{}}]}`, id, map[bool]string{false: "2_0", true: "2_5"}[strings.Contains(id, "2.5")])
			items, hash, _, err := BuildEffective([]byte(catalog), []byte(`{"points_per_cny":60}`), "1", "1")
			require.NoError(t, err)
			require.Len(t, items, 1)
			item := items[0]
			_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{"model": id, "duration": 5, "prompt": "test"}}})
			require.ErrorContains(t, err, "MISSING_ORDERED_DELIVERY_TIER")
			require.NotEmpty(t, item.TaskExpression)
			assert.Equal(t, DocumentedContractOverride, item.Prices["video_second_stage:720p"].SourcePriceKind)
			assert.Equal(t, "60", item.Prices["video_token_tier:default@720p"].Credits)
			assert.NotEqual(t, DocumentedContractOverride, item.Prices["video_token_tier:default@720p"].SourcePriceKind)
			require.Len(t, item.ContractOverrides, 1)
			assert.False(t, item.ContractOverrides[0].AutoApplyAllowed)
			assert.NotEmpty(t, item.ContractOverrides[0].SourceURL)
			config := model.DefaultDFLOPConfig()
			config.IncludeNewCallableModels = true
			plan, err := Plan(items, config, nil, nil)
			require.NoError(t, err)
			assert.Equal(t, SupportedWithProviderOverride, plan[0].Status)
			assert.Equal(t, "ADD", plan[0].Action)
			for _, tc := range []struct {
				tier, mode     string
				token, upscale float64
			}{
				{"720p", "default", 1, 2.5 / 60}, {"1080p", "default", 2, 5.0 / 60},
				{"720p", "with_video_input", 0.5, 2.5 / 60}, {"1080p", "with_video_input", 1.5, 5.0 / 60},
			} {
				ctx := map[string]any{"model": id, "state": map[string]any{"resolution": tc.tier, "input_mode": tc.mode}}
				body := map[string]any{"duration_sec": 3.5, "input_video_duration_sec": 999, "resolution": tc.tier, "usage": map[string]any{"completion_tokens": 1000}}
				facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, body)
				require.NoError(t, err)
				assert.NotContains(t, facts.(map[string]any), "input_video_duration_sec")
				cost, _, err := billingexpr.RunExprWithRequest(item.TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts.(map[string]any)})
				require.NoError(t, err)
				assert.InDelta(t, tc.token*0.001+3.5*tc.upscale, cost, 1e-10)
			}
			for _, missing := range []string{"duration_sec", "usage", "resolution"} {
				body := map[string]any{"duration_sec": 3, "resolution": "720p", "usage": map[string]any{"completion_tokens": 1000}}
				delete(body, missing)
				_, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": id, "state": map[string]any{"resolution": "720p", "input_mode": "default"}}, map[string]any{"status": "SUCCESS"}, body)
				require.Error(t, err, missing)
			}
			_, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": id, "state": map[string]any{"resolution": "unknown", "input_mode": "default"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 0, "resolution": "720p", "usage": map[string]any{"completion_tokens": 0}})
			require.Error(t, err)
			zero, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": id, "state": map[string]any{"resolution": "720p", "input_mode": "default"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 0, "resolution": "720p", "usage": map[string]any{"completion_tokens": 0}})
			require.NoError(t, err)
			assert.EqualValues(t, 0, zero.(map[string]any)["duration_sec"])
			for _, status := range []string{"failed", "cancelled"} {
				result, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"model": id, "taskId": "exact-task"}, map[string]any{"id": "exact-task", "model": id, "status": status})
				require.NoError(t, err)
				assert.Equal(t, "FAILURE", result.(map[string]any)["status"])
			}
			stale := strings.Replace(catalog, `"video_second_stage_per_second":null`, `"video_second_stage_per_second":{"720p":"2.5","1080p":"5"}`, 1)
			changed, nextHash, _, err := BuildEffective([]byte(stale), []byte(`{"points_per_cny":60}`), "1", "1")
			require.NoError(t, err)
			assert.NotEqual(t, hash, nextHash)
			assert.Equal(t, "STALE_PROVIDER_OVERRIDE", changed[0].ReasonCode)
			assert.Empty(t, changed[0].TaskExpression)
		})
	}
	for _, id := range []string{"minimax-h3", "wan3.0-video", "wan3.0-video-prime"} {
		t.Run(id, func(t *testing.T) {
			resolution := "720p"
			if id == "minimax-h3" {
				resolution = "768p"
			}
			body := map[string]any{"duration_sec": 4.5, "input_video_duration_sec": 7, "resolution": resolution}
			facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": id, "state": map[string]any{"resolution": resolution, "requested_duration_sec": 10, "reference_mode": "video"}}, map[string]any{"status": "SUCCESS"}, body)
			require.NoError(t, err)
			if id == "minimax-h3" {
				assert.NotContains(t, facts.(map[string]any), "input_video_duration_sec")
			} else {
				assert.EqualValues(t, 7, facts.(map[string]any)["input_video_duration_sec"])
			}
		})
	}
	alias, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{"model": "MiniMax-H3", "duration": 6, "resolution": "768p", "prompt": "test"}}})
	require.NoError(t, err)
	assert.Equal(t, "minimax-h3", alias.(map[string]any)["requestBody"].(map[string]any)["model"])
	aliasCtx := map[string]any{"model": "MiniMax-H3", "upstreamModel": "MiniMax-H3", "baseUrl": "https://api.dflop.top", "requestHeaders": map[string]any{"Idempotency-Key": "test"}, "requestBody": alias.(map[string]any)["requestBody"]}
	descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", aliasCtx)
	require.NoError(t, err)
	assert.Equal(t, "minimax-h3", descriptor.(map[string]any)["rewriteModel"])
	ack, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", aliasCtx, map[string]any{"body": map[string]any{"id": "exact-task", "model": "minimax-h3", "status": "queued"}})
	require.NoError(t, err)
	aliasCtx["taskId"], aliasCtx["state"] = "exact-task", ack.(map[string]any)["state"]
	result, err := plugin.Engine.Call(t.Context(), "parseTaskResult", aliasCtx, map[string]any{"id": "exact-task", "model": "minimax-h3", "status": "succeeded", "duration_sec": 4.5, "resolution": "768p"})
	require.NoError(t, err)
	assert.False(t, result.(map[string]any)["state"].(map[string]any)["billingPending"].(bool))
}

func TestSubtitleUniqueOperationsNeverInventSourceDuration(t *testing.T) {
	source, err := builtinplugins.Source("dflop-media")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-media"})
	require.NoError(t, err)
	for _, targets := range [][]any{{}, {"en"}, {"en", "vi"}, {"en", "en", "vi"}} {
		value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{"model": "tvod-subtitle-soft", "source_language": "zh", "target_languages": targets, "source_video_url": "https://example.test/source.mp4", "duration": 999}}})
		require.NoError(t, err)
		req := value.(map[string]any)["requestBody"].(map[string]any)
		assert.NotContains(t, req, "duration")
		ctx := map[string]any{"model": "tvod-subtitle-soft", "requestBody": req, "taskId": "exact-task", "baseUrl": "https://api.dflop.top", "apiKey": "test"}
		ack, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"statusCode": 202, "body": map[string]any{"id": "exact-task"}})
		require.NoError(t, err)
		state := ack.(map[string]any)["state"].(map[string]any)
		expected := len(targets)
		if expected == 3 {
			expected = 2
		}
		assert.EqualValues(t, 1, state["asr_units"])
		assert.EqualValues(t, expected, state["translation_units"])
		_, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, map[string]any{"duration_sec": 10, "unit_count": 10})
		require.ErrorContains(t, err, "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION")
		poll, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
		require.NoError(t, err)
		assert.Equal(t, "GET", poll.(map[string]any)["method"])
		for _, status := range []string{"failed", "cancelled"} {
			result, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, map[string]any{"id": "exact-task", "status": status})
			require.NoError(t, err)
			assert.Equal(t, "FAILURE", result.(map[string]any)["status"])
		}
	}
}

func TestDocumentedBasisAndThresholdInvalidateWhenCatalogChanges(t *testing.T) {
	for _, tc := range []struct{ id, pricing, features, field, replacement string }{
		{"minimax-h3", `"price_per_video_second":"30","video_price_tiers":{"768p":"30","2k":"48"},"video_bills_input_seconds":true`, `"video_input_seconds","video_second","video_tiers"`, `"video_bills_input_seconds":true`, `"video_bills_input_seconds":false`},
	} {
		t.Run(tc.id, func(t *testing.T) {
			category, endpoint := "video", "videos_generations"
			if tc.id == "qwen-image-3.0-pro" {
				category, endpoint = "image", "images_generations"
			}
			catalog := fmt.Sprintf(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":%q,"pricing":{"category":%q,"endpoint_type":%q,"callable":true,%s},"billing":{"features":[%s]},"caps":{}}]}`, tc.id, category, endpoint, tc.pricing, tc.features)
			items, hash, _, err := BuildEffective([]byte(catalog), []byte(`{"points_per_cny":60}`), "1", "1")
			require.NoError(t, err)
			require.Len(t, items[0].ContractOverrides, 1)
			if tc.id == "minimax-h3" {
				require.NotEmpty(t, items[0].TaskExpression)
				cost, _, err := billingexpr.RunExprWithRequest(items[0].TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"duration_sec": 4.5, "resolution": "768p", "input_video_duration_sec": 999}})
				require.NoError(t, err)
				assert.InDelta(t, 2.25, cost, 1e-12)
				assert.Equal(t, "30", items[0].Prices["video_tier:768p"].Credits)
			} else {
				assert.Equal(t, "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS", items[0].ReasonCode)
			}
			changed, nextHash, _, err := BuildEffective([]byte(strings.Replace(catalog, tc.field, tc.replacement, 1)), []byte(`{"points_per_cny":60}`), "1", "1")
			require.NoError(t, err)
			assert.NotEqual(t, hash, nextHash)
			assert.Equal(t, "STALE_PROVIDER_OVERRIDE", changed[0].ReasonCode)
			assert.Empty(t, changed[0].TaskExpression)
		})
	}
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"tvod-subtitle-soft","pricing":{"category":"video","endpoint_type":"videos_generations","callable":true,"price_per_video_second":"0.1011","video_price_tiers":{"asr":"0.06066","translate":"0.04044"}},"billing":{"features":["video_second","video_tiers"]},"caps":{}}]}`)
	items, _, _, err := BuildEffective(catalog, []byte(`{"points_per_cny":60}`), "1", "1")
	require.NoError(t, err)
	assert.Equal(t, "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION", items[0].ReasonCode)
	assert.Empty(t, items[0].ContractOverrides)
	assert.Empty(t, items[0].TaskExpression, "formula alone must not make the model selectable")
	for _, translations := range []int{0, 1, 3} {
		cost, _, err := billingexpr.RunExprWithRequest(items[0].Expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"source_duration_sec": 10, "translation_units": translations}})
		require.NoError(t, err)
		assert.InDelta(t, 10*(0.06066+float64(translations)*0.04044)/60, cost, 1e-10)
	}
}

func TestAuthenticatedDFLOPImageBindingAndQuantityBlockers(t *testing.T) {
	for _, tc := range []struct{ id, fields, features, profile, blocker string }{
		{"doubao-seedream-4-0-250828", `"price_per_image":"12"`, `"per_image"`, "IMAGE_PER_OUTPUT", ""},
		{"doubao-seedream-4-5-251128", `"price_per_image":"15"`, `"per_image"`, "IMAGE_PER_OUTPUT", ""},
		{"doubao-seedream-5-0-260128", `"price_per_image":"13.2"`, `"per_image"`, "IMAGE_PER_OUTPUT", ""},
		{"doubao-seedream-5-0-pro-260628", `"price_per_image":"18","price_per_image_large":"36","price_per_input_image":"1.2","free_input_images":1,"large_pixel_threshold":2610000`, `"image_size_bands","input_images","per_image"`, "IMAGE_PIXEL_TIER", "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS"},
		{"qwen-image-3.0", `"price_per_image":"10.8","price_per_input_image":"1.2"`, `"input_images","per_image"`, "IMAGE_PER_OUTPUT_PLUS_REFERENCE", ""},
		{"qwen-image-3.0-pro", `"price_per_image":"15","price_per_image_large":"30","price_per_input_image":"1.2","large_pixel_threshold":2097152`, `"image_size_bands","input_images","per_image"`, "IMAGE_PIXEL_TIER", "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			catalog := fmt.Sprintf(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":%q,"pricing":{"category":"image","endpoint_type":"images_generations","callable":true,%s},"billing":{"features":[%s]},"caps":{}}]}`, tc.id, tc.fields, tc.features)
			items, _, _, err := BuildEffective([]byte(catalog), []byte(`{"points_per_cny":60}`), "1", "1")
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, "dflop-image", items[0].TaskPlugin)
			assert.Equal(t, tc.blocker, items[0].ReasonCode)
			assert.Empty(t, items[0].ContractOverrides, "authenticated threshold must not be replaced by public docs")
			if tc.blocker != "" {
				assert.Equal(t, UnsupportedMapping, items[0].Status)
				assert.Empty(t, items[0].TaskExpression)
			} else {
				assert.NotEmpty(t, items[0].TaskExpression)
			}
		})
	}
}

func TestCatalogEndpointPrecedence(t *testing.T) {
	for _, tc := range []struct{ endpoint, protocol, path string }{
		{"images_generations", "openai_image", "/v1/images/generations"},
		{"videos_generations", "openai_video", "/v1/videos/generations"},
		{"", "openai_chat", "/v1/chat/completions"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			item := Item{EndpointType: tc.endpoint, Protocols: []string{"openai_chat"}, Callable: true}
			binding, err := CatalogEndpoint(item)
			require.NoError(t, err)
			assert.Equal(t, tc.protocol, binding.Protocol)
			assert.Equal(t, tc.path, binding.Path)
		})
	}
	_, err := CatalogEndpoint(Item{Callable: true, EndpointType: "unknown"})
	require.ErrorContains(t, err, "PROTOCOL_MISMATCH")
	_, err = CatalogEndpoint(Item{EndpointType: "images_generations"})
	require.ErrorContains(t, err, "SOURCE_MODEL_NOT_CALLABLE")
}

func TestAuthenticatedWanUsageProfilesUseExactQuantityContract(t *testing.T) {
	for _, id := range []string{"wan2.7-t2v", "wan3.0-video", "wan3.0-video-prime"} {
		t.Run(id, func(t *testing.T) {
			features := `"video_second","video_tiers"`
			input := false
			tiers := `"720p":"60","1080p":"120"`
			if id != "wan2.7-t2v" {
				features += `,"video_input_seconds"`
				input = true
				tiers += `,"480p":"60"`
			}
			raw := fmt.Sprintf(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":%q,"pricing":{"category":"video","endpoint_type":"videos_generations","callable":true,"price_per_video_second":"60","video_price_tiers":{%s},"video_bills_input_seconds":%t,"video_max_input_seconds":15},"billing":{"features":[%s]},"caps":{}}]}`, id, tiers, input, features)
			items, _, _, err := BuildEffective([]byte(raw), []byte(`{"points_per_cny":60}`), "1", "1")
			require.NoError(t, err)
			require.Equal(t, "dflop-media", items[0].TaskPlugin)
			assert.Empty(t, items[0].ReasonCode)
			require.NotEmpty(t, items[0].TaskExpression)
			for _, seconds := range []float64{0, 7} {
				cost, _, err := billingexpr.RunExprWithRequest(items[0].TaskExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"duration_sec": 4.5, "input_video_duration_sec": seconds, "resolution": "720p"}})
				require.NoError(t, err)
				expected := 4.5
				if input {
					expected += seconds
				}
				assert.InDelta(t, expected, cost, 1e-12)
			}
		})
	}
}
