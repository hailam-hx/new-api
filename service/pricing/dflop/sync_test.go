package dflop

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerPreviewUsesSelectedChannelAndDoesNotPersistSecret(t *testing.T) {
	originalDB := model.DB
	originalPath := common.SQLitePath
	t.Cleanup(func() { model.DB = originalDB; common.SQLitePath = originalPath })
	common.SQLitePath = t.TempDir() + "/pricing.db"
	t.Setenv("SQL_DSN", "local")
	require.NoError(t, model.InitDB())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.PricingSyncRun{}, &model.PricingSyncItem{}, &model.PricingSyncManaged{}))
	model.InitOptionMap()
	base := "https://api.dflop.top"
	require.NoError(t, db.Create(&model.Channel{Id: 7, Name: "Source", Status: common.ChannelStatusEnabled, BaseURL: &base, Key: "private-test-key"}).Error)
	config := model.DefaultDFLOPConfig()
	config.Enabled, config.SourceChannelID, config.CNYToUSD = true, 7, "1"
	require.NoError(t, model.SaveDFLOPConfig(config))
	catalog := `{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"text","pricing":{"category":"text","callable":true,"input_per_1m":"60","output_per_1m":"120"},"billing":{"features":["token"]},"caps":{"surfaces":["chat"]}}]}`
	transport := catalogTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"unit":"points","points_per_cny":60}`
		status := http.StatusOK
		if req.URL.Path == "/v1/catalog" {
			assert.Equal(t, "Bearer private-test-key", req.Header.Get("Authorization"))
			body = catalog
			if req.Header.Get("If-None-Match") == `"cat1"` {
				status, body = http.StatusNotModified, ""
			}
		} else {
			assert.Empty(t, req.Header.Get("Authorization"))
			if req.URL.Path == "/api/v1/models/public" {
				body = `{"models":[{"id":"text","callable":true}]}`
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Etag": {`"cat1"`}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	run, rows, err := (Manager{HTTP: &http.Client{Transport: transport}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, SupportedAuto, rows[0].Status)
	assert.NotContains(t, run.CurrencySnapshot, "private-test-key")
	assert.NotContains(t, run.ConfigSnapshot, "private-test-key")
	var record SourceRecord
	require.NoError(t, common.UnmarshalJsonStr(run.CurrencySnapshot, &record))
	assert.Equal(t, "AUTHENTICATED_EFFECTIVE", record.SourceMode)
	assert.Equal(t, 7, record.SourceChannel.ID)
	assert.Equal(t, "1.0", record.CatalogSchemaVersion)
	var prices map[string]Price
	require.NoError(t, common.UnmarshalJsonStr(rows[0].Prices, &prices))
	assert.Equal(t, "60", prices["input_per_1m"].Credits)
	config.MarkupMultiplier = "2"
	require.NoError(t, model.SaveDFLOPConfig(config))
	second, secondRows, err := (Manager{HTTP: &http.Client{Transport: transport}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, secondRows, 1)
	assert.Equal(t, run.SourceHash, second.SourceHash)
	require.NoError(t, common.UnmarshalJsonStr(second.CurrencySnapshot, &record))
	assert.Equal(t, "NOT_MODIFIED", record.Effective.State)
	require.NoError(t, common.UnmarshalJsonStr(secondRows[0].Prices, &prices))
	assert.Equal(t, "2", prices["input_per_1m"].SellingUSD)
	applied, err := (Manager{HTTP: &http.Client{Transport: transport}}).Apply(context.Background(), second.ID, second.PricingVersionBefore, []string{"text"}, false, 1, false)
	require.NoError(t, err)
	assert.Equal(t, "applied", applied.Status)
	live, err := model.GetModelPricingSnapshot([]string{"text"})
	require.NoError(t, err)
	assert.Equal(t, `tier("dflop", p * 2 + c * 4)`, live.Entries[0].Configured["billing_setting.billing_expr"])
	assert.NotContains(t, applied.CurrencySnapshot, "private-test-key")
	legacy := config
	legacy.SourceChannelID, legacy.AutoSyncEnabled, legacy.AutoApplyEnabled = 0, true, true
	legacyJSON, err := common.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", model.DFLOPConfigOption).Update("value", string(legacyJSON)).Error)
	loaded, err := model.GetDFLOPConfig()
	require.NoError(t, err)
	assert.False(t, loaded.AutoApplyEnabled)
	config.SourceChannelID = 0
	require.NoError(t, model.SaveDFLOPConfig(config))
	client := Client{HTTP: &http.Client{Transport: transport}}
	diagnostic, diagnosticRows, err := (Manager{Source: client}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, diagnosticRows, 1)
	assert.Equal(t, "PUBLIC_DIAGNOSTIC", diagnosticRows[0].Status)
	_, err = (Manager{Source: client}).Apply(context.Background(), diagnostic.ID, diagnostic.PricingVersionBefore, []string{"text"}, false, 1, false)
	require.Error(t, err)
	config.SourceChannelID = 7
	require.NoError(t, model.SaveDFLOPConfig(config))
	unknown := catalogTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/catalog" {
			return transport(req)
		}
		body := `{"schema_version":"2.0","currency":"points","aliases":{},"models":[{"id":"text"}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "Etag": {`"cat2"`}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	inspection, inspectionRows, err := (Manager{HTTP: &http.Client{Transport: unknown}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	assert.Empty(t, inspectionRows)
	require.NoError(t, common.UnmarshalJsonStr(inspection.CurrencySnapshot, &record))
	assert.Equal(t, "2.0", record.CatalogSchemaVersion)
	assert.Contains(t, record.Integrity, "SCHEMA_VERSION_CHANGED")
	lastBody, lastRecord, err := lastGoodSource(7, sourceDigest([]byte("private-test-key"))[:16])
	require.NoError(t, err)
	assert.Equal(t, "1.0", lastRecord.CatalogSchemaVersion)
	assert.JSONEq(t, catalog, string(lastBody))
	shrunk := catalogTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/catalog" {
			return transport(req)
		}
		body := `{"schema_version":"1.0","currency":"points","aliases":{},"models":[]}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "Etag": {`"cat3"`}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	collapsedRun, collapsedRows, err := (Manager{HTTP: &http.Client{Transport: shrunk}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, collapsedRows, 1)
	assert.Equal(t, "NOT_AVAILABLE_TO_SOURCE_KEY", collapsedRows[0].Status)
	require.NoError(t, common.UnmarshalJsonStr(collapsedRun.CurrencySnapshot, &record))
	assert.Contains(t, record.Integrity, "MODEL_COUNT_COLLAPSE")
	_, err = (Manager{HTTP: &http.Client{Transport: shrunk}}).Apply(context.Background(), collapsedRun.ID, collapsedRun.PricingVersionBefore, []string{"text"}, false, 1, false)
	require.ErrorContains(t, err, "MODEL_COUNT_COLLAPSE")
	_, lastRecord, err = lastGoodSource(7, sourceDigest([]byte("private-test-key"))[:16])
	require.NoError(t, err)
	assert.Equal(t, 1, lastRecord.EffectiveCount)
	rotatedBody, rotatedRecord, err := lastGoodSource(7, sourceDigest([]byte("rotated-test-key"))[:16])
	require.NoError(t, err)
	assert.Empty(t, rotatedBody)
	assert.Empty(t, rotatedRecord.Effective.ETag)
	config.MarkupMultiplier = "2.1"
	require.NoError(t, model.SaveDFLOPConfig(config))
	healthyRun, healthyRows, err := (Manager{HTTP: &http.Client{Transport: transport}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, healthyRows, 1)
	publicGap := catalogTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/models/public" {
			return transport(req)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"models":[]}`)), Request: req}, nil
	})
	gapRun, gapRows, err := (Manager{HTTP: &http.Client{Transport: publicGap}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, gapRows, 1)
	assert.Equal(t, healthyRun.SourceHash, gapRun.SourceHash)
	assert.Equal(t, healthyRows[0].ProposedPricing, gapRows[0].ProposedPricing)
	require.NoError(t, db.Model(&model.PricingSyncRun{}).Where("id = ?", healthyRun.ID).Update("started_at", time.Now().Add(-11*time.Minute).Unix()).Error)
	_, err = (Manager{HTTP: &http.Client{Transport: transport}}).Apply(context.Background(), healthyRun.ID, healthyRun.PricingVersionBefore, []string{"text"}, false, 1, false)
	require.ErrorIs(t, err, model.ErrModelPricingConflict)
	record = SourceRecord{}
	require.NoError(t, common.UnmarshalJsonStr(gapRun.CurrencySnapshot, &record))
	assert.Contains(t, record.Integrity, "PUBLIC_CATALOG_ANOMALY")
	assert.Empty(t, record.Policy.ManualBlockReason)
	assert.Equal(t, "PUBLIC_CATALOG_ANOMALY", record.Policy.AutoBlockReason)
	assert.True(t, record.Policy.ManualConfirmationRequired)
	_, err = (Manager{HTTP: &http.Client{Transport: publicGap}}).Apply(context.Background(), gapRun.ID, gapRun.PricingVersionBefore, []string{"text"}, false, 1, false)
	require.ErrorContains(t, err, "PUBLIC_CATALOG_ANOMALY")
	_, err = (Manager{HTTP: &http.Client{Transport: publicGap}}).Apply(context.Background(), gapRun.ID, gapRun.PricingVersionBefore, []string{"text"}, false, 0, true)
	require.ErrorContains(t, err, "automatic apply")
	_, err = (Manager{HTTP: &http.Client{Transport: publicGap}}).Apply(context.Background(), gapRun.ID, gapRun.PricingVersionBefore, []string{"text"}, false, 1, true)
	require.NoError(t, err)
	config.MarkupMultiplier = "2.2"
	require.NoError(t, model.SaveDFLOPConfig(config))
	unknownFeature := catalogTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"unit":"points","points_per_cny":60}`
		if req.URL.Path == "/v1/catalog" {
			body = `{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"text","pricing":{"category":"text","callable":true,"input_per_1m":"60","output_per_1m":"120"},"billing":{"features":["token"]},"caps":{"surfaces":["chat"]}},{"id":"future","pricing":{"category":"text","callable":true,"input_per_1m":"60","output_per_1m":"120"},"billing":{"features":["future_charge"]},"caps":{"surfaces":["chat"]}}]}`
		} else if req.URL.Path == "/api/v1/models/public" {
			body = `{"models":[{"id":"text","callable":true},{"id":"future","callable":true}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	featureRun, featureRows, err := (Manager{HTTP: &http.Client{Transport: unknownFeature}}).Preview(context.Background(), 1, "manual")
	require.NoError(t, err)
	require.Len(t, featureRows, 2)
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", featureRows[0].ReasonCode)
	assert.Equal(t, "SKIP", featureRows[0].Action)
	require.NoError(t, common.UnmarshalJsonStr(featureRun.CurrencySnapshot, &record))
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", EvaluateSourceIntegrity(record).AutoBlockReason)
	_, err = (Manager{HTTP: &http.Client{Transport: unknownFeature}}).Apply(context.Background(), featureRun.ID, featureRun.PricingVersionBefore, []string{"text"}, false, 0, false)
	require.ErrorContains(t, err, "UNKNOWN_BILLING_FEATURE")
	_, err = (Manager{HTTP: &http.Client{Transport: unknownFeature}}).Apply(context.Background(), featureRun.ID, featureRun.PricingVersionBefore, []string{"future"}, false, 1, false)
	require.ErrorContains(t, err, "no applicable change")
	_, err = (Manager{HTTP: &http.Client{Transport: unknownFeature}}).Apply(context.Background(), featureRun.ID, featureRun.PricingVersionBefore, []string{"text"}, false, 1, false)
	require.NoError(t, err)
	info, secret, err := LoadSourceChannel(7)
	require.NoError(t, err)
	assert.Equal(t, "Source", info.Name)
	assert.Equal(t, "private-test-key", secret)
	encodedInfo, err := common.Marshal(info)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedInfo), secret)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 7).Update("channel_info", model.ChannelInfo{IsMultiKey: true}).Error)
	_, _, err = LoadSourceChannel(7)
	require.ErrorContains(t, err, "MULTI_KEY_SOURCE_UNSUPPORTED")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 7).Update("status", 0).Error)
	_, _, err = LoadSourceChannel(7)
	require.ErrorContains(t, err, "SOURCE_CHANNEL_UNAVAILABLE")
}

func TestSourceIntegrityApplyPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, mode             string
		integrity              []string
		manualBlock, autoBlock string
		confirm                bool
	}{
		{"healthy", "AUTHENTICATED_EFFECTIVE", []string{"HEALTHY"}, "", "", false},
		{"public warning", "AUTHENTICATED_EFFECTIVE", []string{"PUBLIC_CATALOG_ANOMALY"}, "", "PUBLIC_CATALOG_ANOMALY", true},
		{"effective anomaly", "AUTHENTICATED_EFFECTIVE", []string{"EFFECTIVE_CATALOG_ANOMALY"}, "EFFECTIVE_CATALOG_ANOMALY", "EFFECTIVE_CATALOG_ANOMALY", false},
		{"collapse", "AUTHENTICATED_EFFECTIVE", []string{"EFFECTIVE_CATALOG_ANOMALY", "MODEL_COUNT_COLLAPSE"}, "MODEL_COUNT_COLLAPSE", "MODEL_COUNT_COLLAPSE", false},
		{"public and collapse", "AUTHENTICATED_EFFECTIVE", []string{"PUBLIC_CATALOG_ANOMALY", "EFFECTIVE_CATALOG_ANOMALY", "MODEL_COUNT_COLLAPSE"}, "MODEL_COUNT_COLLAPSE", "MODEL_COUNT_COLLAPSE", true},
		{"schema", "AUTHENTICATED_EFFECTIVE", []string{"SCHEMA_VERSION_CHANGED"}, "SCHEMA_VERSION_CHANGED", "SCHEMA_VERSION_CHANGED", false},
		{"unknown feature", "AUTHENTICATED_EFFECTIVE", []string{"UNKNOWN_BILLING_FEATURE"}, "", "UNKNOWN_BILLING_FEATURE", false},
		{"missing status", "AUTHENTICATED_EFFECTIVE", nil, "INTEGRITY_STATUS_MISSING", "INTEGRITY_STATUS_MISSING", false},
		{"no source", "PUBLIC_DIAGNOSTIC", []string{"SOURCE_CHANNEL_UNAVAILABLE"}, "SOURCE_CHANNEL_UNAVAILABLE", "SOURCE_CHANNEL_UNAVAILABLE", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := EvaluateSourceIntegrity(SourceRecord{SourceMode: tc.mode, Integrity: tc.integrity})
			assert.Equal(t, tc.manualBlock, policy.ManualBlockReason)
			assert.Equal(t, tc.autoBlock, policy.AutoBlockReason)
			assert.Equal(t, tc.confirm, policy.ManualConfirmationRequired)
			require.NotEmpty(t, policy.Conditions)
			if tc.name == "public warning" {
				assert.Equal(t, "AUTO_APPLY_BLOCK", policy.Conditions[0].Severity)
			}
		})
	}
}

func TestPlanCreatesOnlyCompatiblePluginOverride(t *testing.T) {
	config := model.DefaultDFLOPConfig()
	item := Item{ModelID: "shared-video", Category: "video", Status: UnsupportedMapping,
		PricingShape: "video:video_second+video_tiers", TaskPlugin: "alibaba", RequiredFacts: []string{"seconds", "resolution"},
		TaskExpression: `u("resolution") == "720P" ? tier("720P", u("seconds") * 0.6) : tier("1080P", u("seconds") * 1)`,
		Prices:         map[string]Price{"price_per_video_second": {SellingUSD: "1"}, "video_tier:720p": {SellingUSD: "0.6"}, "video_tier:1080p": {SellingUSD: "1"}}}
	compatible := model.ModelPricingPluginVariant{PluginKey: "alibaba", UsageSchema: map[string]jsplugin.UsageFieldSchema{"seconds": {Type: "number", Unit: "second"}, "resolution": {Enum: []string{"720P", "1080P"}}}}
	incompatible := model.ModelPricingPluginVariant{PluginKey: "other", UsageSchema: map[string]jsplugin.UsageFieldSchema{"seconds": {Type: "number", Unit: "second"}}}
	entry := model.ModelPricingEntry{ModelName: item.ModelID, Version: model.ModelPricingVersion(model.PricingValues{}), Configured: model.PricingValues{}, PluginVariants: []model.ModelPricingPluginVariant{compatible, incompatible}}
	planned, err := Plan([]Item{item}, config, []model.ModelPricingEntry{entry}, nil)
	require.NoError(t, err)
	require.Len(t, planned, 1)
	assert.Equal(t, SupportedAuto, planned[0].Status)
	assert.Equal(t, "ADD", planned[0].Action)
	assert.Equal(t, "PLUGIN_OVERRIDE", planned[0].PricingScope)
	assert.Equal(t, "alibaba", planned[0].PluginKey)
	var proposed model.PricingValues
	require.NoError(t, common.UnmarshalJsonStr(planned[0].ProposedPricing, &proposed))
	assert.Equal(t, map[string]any{"alibaba": item.TaskExpression}, proposed["billing_setting.plugin_billing_expr"])
	assert.NotContains(t, proposed, "billing_setting.billing_expr")
	entry.Configured = model.PricingValues{"billing_setting.plugin_billing_expr": map[string]any{"other": `tier("other", u("seconds") * 1)`}}
	entry.Version = model.ModelPricingVersion(entry.Configured)
	planned, err = Plan([]Item{item}, config, []model.ModelPricingEntry{entry}, nil)
	require.NoError(t, err)
	assert.Equal(t, "MANUAL_OVERRIDE", planned[0].Status)
	require.NoError(t, common.UnmarshalJsonStr(planned[0].ProposedPricing, &proposed))
	assert.Equal(t, map[string]any{"alibaba": item.TaskExpression, "other": `tier("other", u("seconds") * 1)`}, proposed["billing_setting.plugin_billing_expr"])

	entry.PluginVariants[0].UsageSchema = incompatible.UsageSchema
	planned, err = Plan([]Item{item}, config, []model.ModelPricingEntry{entry}, nil)
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, planned[0].Status)
	assert.Equal(t, "MISSING_RESOLUTION_FACT", planned[0].ReasonCode)
	assert.Equal(t, `["seconds"]`, planned[0].AvailableFacts)
	assert.Equal(t, `["resolution"]`, planned[0].MissingFacts)
}

func TestPlanBlocksChangedPriceShape(t *testing.T) {
	config := model.DefaultDFLOPConfig()
	configured := model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("base", p * 1)`}
	version := model.ModelPricingVersion(configured)
	entry := model.ModelPricingEntry{ModelName: "shape", Version: version, Configured: configured}
	item := Item{ModelID: "shape", Category: "text", Status: SupportedAuto, Expression: `tier("base", p * 2)`, PricingShape: "text:token+cache", Prices: map[string]Price{"input_per_1m": {SellingUSD: "2"}}}
	owner := model.PricingSyncManaged{ModelID: "shape", LastAppliedHash: version, PricingShape: "text:token", Prices: `{"input_per_1m":{"selling_usd":"1"}}`}
	planned, err := Plan([]Item{item}, config, []model.ModelPricingEntry{entry}, map[string]model.PricingSyncManaged{"shape": owner})
	require.NoError(t, err)
	assert.Equal(t, "PRICE_SHAPE_CHANGED", planned[0].Status)
	assert.Equal(t, "BLOCK", planned[0].Action)
}

func TestPlanBlocksManualDriftAndLargeIncrease(t *testing.T) {
	config := model.DefaultDFLOPConfig()
	entries := []model.ModelPricingEntry{{ModelName: "manual", Configured: model.PricingValues{"billing_setting.billing_expr": "custom"}}, {ModelName: "drift", Configured: model.PricingValues{"billing_setting.billing_expr": "changed"}}, {ModelName: "increase", Configured: model.PricingValues{"billing_setting.billing_expr": "old"}}}
	for i := range entries {
		entries[i].Version = model.ModelPricingVersion(entries[i].Configured)
	}
	items := []Item{
		{ModelID: "manual", Category: "text", Status: SupportedAuto, Expression: `tier("dflop", p * 2)`, Prices: map[string]Price{"input_per_1m": {SellingUSD: "2"}}},
		{ModelID: "drift", Category: "text", Status: SupportedAuto, Expression: `tier("dflop", p * 2)`, Prices: map[string]Price{"input_per_1m": {SellingUSD: "2"}}},
		{ModelID: "increase", Category: "text", Status: SupportedAuto, Expression: `tier("dflop", p * 2)`, Prices: map[string]Price{"input_per_1m": {SellingUSD: "2"}}},
	}
	managed := map[string]model.PricingSyncManaged{
		"drift":    {ModelID: "drift", LastAppliedHash: "stale"},
		"increase": {ModelID: "increase", LastAppliedHash: entries[2].Version, Prices: `{"input_per_1m":{"selling_usd":"1"}}`},
	}
	planned, err := Plan(items, config, entries, managed)
	require.NoError(t, err)
	assert.Equal(t, "MANUAL_OVERRIDE", planned[0].Status)
	assert.Equal(t, "MANUAL_DRIFT", planned[1].Status)
	assert.Equal(t, "SUPPORTED_MANUAL", planned[2].Status)
	assert.Equal(t, "100", planned[2].DeltaPercent)
}

func TestPlanLeavesPluginBackedImageForManualMapping(t *testing.T) {
	config := model.DefaultDFLOPConfig()
	items := []Item{{ModelID: "image-task", Category: "image", Status: SupportedAuto, Expression: `tier("image", fixed(1)) * image_count`}}
	entries := []model.ModelPricingEntry{{ModelName: "image-task", Configured: model.PricingValues{}, PluginVariants: []model.ModelPricingPluginVariant{{PluginKey: "image-plugin"}}}}
	planned, err := Plan(items, config, entries, nil)
	require.NoError(t, err)
	assert.Equal(t, UnsupportedMapping, planned[0].Status)
	assert.Equal(t, "SKIP", planned[0].Action)
}

func TestPlanIsIdempotentAndBlocksLargeDecrease(t *testing.T) {
	config := model.DefaultDFLOPConfig()
	configured := model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("dflop", p * 1)`}
	version := model.ModelPricingVersion(configured)
	entry := model.ModelPricingEntry{ModelName: "text", Configured: configured, Version: version}
	owner := model.PricingSyncManaged{ModelID: "text", LastAppliedHash: version, Prices: `{"input_per_1m":{"selling_usd":"1"}}`}
	item := Item{ModelID: "text", Category: "text", Status: SupportedAuto, Expression: `tier("dflop", p * 1)`, Prices: map[string]Price{"input_per_1m": {SellingUSD: "1"}}}
	planned, err := Plan([]Item{item}, config, []model.ModelPricingEntry{entry}, map[string]model.PricingSyncManaged{"text": owner})
	require.NoError(t, err)
	assert.Equal(t, "UNCHANGED", planned[0].Action)
	item.Expression = `tier("dflop", p * 0.4)`
	item.Prices["input_per_1m"] = Price{SellingUSD: "0.4"}
	planned, err = Plan([]Item{item}, config, []model.ModelPricingEntry{entry}, map[string]model.PricingSyncManaged{"text": owner})
	require.NoError(t, err)
	assert.Equal(t, "SUPPORTED_MANUAL", planned[0].Status)
	assert.Equal(t, "-60", planned[0].DeltaPercent)
}

func TestPlanRequiresNewModelOptIn(t *testing.T) {
	config := model.DefaultDFLOPConfig()
	config.IncludeNewCallableModels = false
	item := Item{ModelID: "new-text", Category: "text", Status: SupportedAuto, Expression: `tier("dflop", p * 1)`, Prices: map[string]Price{"input_per_1m": {SellingUSD: "1"}}}
	planned, err := Plan([]Item{item}, config, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "SKIPPED_NEW", planned[0].Status)
	assert.Equal(t, "SKIP", planned[0].Action)
	config.IncludeNewCallableModels = true
	planned, err = Plan([]Item{item}, config, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "ADD", planned[0].Action)
}
