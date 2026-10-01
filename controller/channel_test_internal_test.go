package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTaskPricingRCAReplay(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"dflop-media", "dflop-image", "dflop-tts", "doubao", "alibaba"} {
		source, err := plugins.Source(key)
		require.NoError(t, err)
		_, err = registry.RegisterFactory(source, jsplugin.Options{Key: key})
		require.NoError(t, err)
	}
	data, err := os.ReadFile("testdata/dflop-channel-pricing-rca.json")
	require.NoError(t, err)
	var fixture struct {
		Models []struct {
			Model      string `json:"model"`
			Plugin     string `json:"plugin"`
			Expression string `json:"expression"`
			RootCause  string `json:"root_cause"`
			Resolved   bool   `json:"resolved"`
		} `json:"models"`
	}
	require.NoError(t, common.Unmarshal(data, &fixture))
	require.Len(t, fixture.Models, 89)
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	previous := *settings
	previousSelfUse := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = false
	*settings = billing_setting.BillingSetting{BillingMode: map[string]string{}, BillingExpr: map[string]string{}, PluginBillingExpr: map[string]string{}}
	t.Cleanup(func() { *settings = previous; operation_setting.SelfUseModeEnabled = previousSelfUse })
	for _, row := range fixture.Models {
		if row.Expression != "" {
			settings.PluginBillingExpr[billing_setting.PluginBillingExprKey(row.Plugin, row.Model)] = row.Expression
		}
	}
	channel := &model.Channel{Id: 987, Type: constant.ChannelTypeNewAPI, BaseURL: common.GetPointer("https://api.dflop.top")}
	resolved, blocked := 0, 0
	for _, row := range fixture.Models {
		t.Run(row.Model, func(t *testing.T) {
			_, genericPrice := ratio_setting.GetModelPrice(row.Model, false)
			_, genericRatio, _ := ratio_setting.GetModelRatio(row.Model)
			assert.False(t, genericPrice, "fixture must reproduce missing generic price")
			assert.False(t, genericRatio, "fixture must reproduce missing generic ratio")
			plan := resolveChannelTestTaskPricing(channel, registry.Generation(), row.Model, row.Model, "")
			if plan != nil && plan.Resolved {
				resolved++
			} else {
				blocked++
			}
			if row.Resolved {
				require.NotNil(t, plan)
				assert.True(t, plan.Resolved, "reason=%s", plan.Reason)
				assert.Equal(t, row.Plugin, plan.Plugin)
				assert.Equal(t, "plugin_expression", plan.BillingSource)
				assert.Equal(t, row.Expression, plan.Expression)
				t.Logf("PRICING_RESOLVED model=%s plugin=%s source=%s", row.Model, plan.Plugin, plan.BillingSource)
			} else {
				if plan != nil {
					assert.False(t, plan.Resolved, "RCA=%s", row.RootCause)
					if row.RootCause == "PRICING_PROVIDER_BINDING_MISMATCH" {
						assert.Equal(t, "PROVIDER_SCOPE_MISMATCH", plan.Reason)
					} else {
						assert.Equal(t, "PLUGIN_PRICE_NOT_FOUND", plan.Reason)
					}
				} else {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					info := &relaycommon.RelayInfo{OriginModelName: row.Model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: row.Model}}
					_, err := helper.ModelPriceHelper(c, info, 0, nil)
					assert.Error(t, err, "unclaimed model must still fail generic pricing")
				}
				reason := "GENERIC_PRICE_NOT_FOUND"
				if plan != nil {
					reason = plan.Reason
				}
				t.Logf("PRICING_UNRESOLVED model=%s reason=%s RCA=%s", row.Model, reason, row.RootCause)
			}
		})
	}
	assert.Equal(t, 71, resolved)
	assert.Equal(t, 18, blocked)
	t.Logf("OFFLINE_REPLAY resolved=%d blocked=%d total=%d", resolved, blocked, len(fixture.Models))
}

func TestChannelTaskPricingScopeAndValidation(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"dflop-media", "dflop-image", "dflop-tts", "alibaba"} {
		source, err := plugins.Source(key)
		require.NoError(t, err)
		_, err = registry.RegisterFactory(source, jsplugin.Options{Key: key})
		require.NoError(t, err)
	}
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: common.GetPointer("https://api.dflop.top")}
	for _, tc := range []struct {
		name, model, mapped, key, expression, path, reason string
		resolved                                           bool
	}{
		{name: "video", model: "tvod-veo-3.1", key: "dflop-media", expression: `u("duration_sec") * 0.1`, resolved: true},
		{name: "image", model: "tvod-midjourney-v7", key: "dflop-image", expression: `u("image_count") * 0.1`, resolved: true},
		{name: "tts", model: "voice-tts-pro", key: "dflop-tts", expression: `u("character_count") * 0.1`, resolved: true},
		{name: "music native capability", model: "suno-v5", key: "dflop-media", expression: `u("generation_count") * 0.1`, resolved: true},
		{name: "async digital human", model: "dh-avatar", key: "dflop-media", expression: `u("duration_sec") * 0.1`, resolved: true},
		{name: "missing", model: "tvod-veo-3.1", key: "dflop-media", reason: "PLUGIN_PRICE_NOT_FOUND"},
		{name: "syntax invalid", model: "tvod-veo-3.1", key: "dflop-media", expression: `u(`, reason: "PLUGIN_EXPR_INVALID"},
		{name: "wrong usage schema", model: "tvod-veo-3.1", key: "dflop-media", expression: `u("seconds")`, reason: "PLUGIN_EXPR_INVALID"},
		{name: "no quantity", model: "tvod-veo-3.1", key: "dflop-media", expression: `0`, reason: "REQUIRED_USAGE_UNRESOLVED"},
		{name: "explicit zero quantity price", model: "tvod-veo-3.1", key: "dflop-media", expression: `u("duration_sec") * 0`, resolved: true},
		{name: "mapped override", model: "tvod-veo-3.1", mapped: "vendor-endpoint-id", key: "dflop-media", expression: `u("duration_sec") * 0.1`, resolved: true},
		{name: "mapped client alias", model: "my-video-alias", mapped: "tvod-veo-3.1", key: "dflop-media", expression: `u("duration_sec") * 0.1`, resolved: true},
		{name: "no cross provider", model: "wan3.0-video", key: "alibaba", expression: `u("seconds") * 0.1`, reason: "PROVIDER_SCOPE_MISMATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			*settings = billing_setting.BillingSetting{BillingMode: map[string]string{}, BillingExpr: map[string]string{}, PluginBillingExpr: map[string]string{}}
			mapped := tc.mapped
			if mapped == "" {
				mapped = tc.model
			}
			if tc.expression != "" {
				settings.PluginBillingExpr[billing_setting.PluginBillingExprKey(tc.key, mapped)] = tc.expression
			}
			plan := resolveChannelTestTaskPricing(channel, registry.Generation(), tc.model, mapped, tc.path)
			require.NotNil(t, plan)
			assert.Equal(t, tc.resolved, plan.Resolved)
			assert.Equal(t, tc.reason, plan.Reason)
			// Compare the real production selector and gate, not a second pricing implementation.
			if tc.key != "alibaba" {
				candidates := registry.Generation().LookupEndpointCandidates(http.MethodPost, "/v1/videos", tc.model)
				if len(candidates) > 0 {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: registry.Generation(), Plugin: candidates[0].Plugin, Candidates: candidates})
					selected, bound := middleware.PinnedEndpointCandidateForChannel(c, channel, candidates[0].Plugin.Meta.Key)
					require.True(t, bound)
					production := billing_setting.ResolveTaskBillingPlan(selected.Plugin.Meta.Key, tc.model, mapped, selected.Plugin, true)
					assert.Equal(t, production, *plan)
				}
			}
		})
	}
	assert.Nil(t, resolveChannelTestTaskPricing(channel, registry.Generation(), "tvod-veo-3.1", "tvod-veo-3.1", "/v1/chat/completions"))
	assert.Nil(t, resolveChannelTestTaskPricing(channel, jsplugin.NewRegistry().Generation(), "tvod-veo-3.1", "tvod-veo-3.1", ""))
}

func TestChannelTaskPreflightDoesNotSubmitOrUseDatabase(t *testing.T) {
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	previous := *settings
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = nil, nil
	t.Cleanup(func() { *settings = previous; model.DB, model.LOG_DB = previousDB, previousLogDB })
	*settings = billing_setting.BillingSetting{BillingMode: map[string]string{}, BillingExpr: map[string]string{}, PluginBillingExpr: map[string]string{"dflop-media::tvod-veo-3.1": `u("duration_sec") * 0.1`}}
	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: common.GetPointer("https://api.dflop.top")}
	result := testChannel(context.Background(), channel, 123, "tvod-veo-3.1", "", false)
	require.NotNil(t, result.billingPlan)
	assert.True(t, result.billingPlan.Resolved)
	assert.Equal(t, "task_channel_test_unsupported", string(result.newAPIError.GetErrorCode()))
	delete(settings.PluginBillingExpr, "dflop-media::tvod-veo-3.1")
	result = testChannel(context.Background(), channel, 123, "tvod-veo-3.1", "", false)
	require.NotNil(t, result.billingPlan)
	assert.False(t, result.billingPlan.Resolved)
	assert.Equal(t, "model_price_error", string(result.newAPIError.GetErrorCode()))
	channel.Models = "tvod-veo-3.1"
	assert.Equal(t, channelTestSummary{Tested: 1, Failed: 1}, testChannelForHealthCheck(context.Background(), channel, 123, true, 0))
}

func TestGetChannelDefaultBaseURLsUsesBuiltInDefaults(t *testing.T) {
	originalBaseURLs := constant.ChannelBaseURLs
	constant.ChannelBaseURLs = append([]string(nil), originalBaseURLs...)
	constant.ChannelBaseURLs[constant.ChannelTypeDeepSeek] = "https://deepseek.server.example"
	t.Cleanup(func() {
		constant.ChannelBaseURLs = originalBaseURLs
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/default_base_urls", nil)
	GetChannelDefaultBaseURLs(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[int]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, "https://deepseek.server.example", response.Data[constant.ChannelTypeDeepSeek])
	assert.Equal(t, "https://api.openai.com", response.Data[constant.ChannelTypeOpenAI])
	assert.NotContains(t, response.Data, constant.ChannelTypeAzure)
	assert.NotContains(t, response.Data, constant.ChannelTypeNewAPI)
	assert.NotContains(t, response.Data, constant.ChannelTypeTaskPlugin)
}

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRequiresNewAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL *string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "blank", baseURL: common.GetPointer("  "), wantErr: true},
		{name: "configured", baseURL: common.GetPointer("https://new-api.example")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{
				Type:    constant.ChannelTypeNewAPI,
				BaseURL: test.baseURL,
			}

			err := validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "New API channel base URL cannot be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewAPIChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeNewAPI)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeNewAPI, apiType)
	assert.Equal(t, "New API", constant.GetChannelTypeName(constant.ChannelTypeNewAPI))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeNewAPI)
	assert.Empty(t, constant.ChannelBaseURLs[constant.ChannelTypeNewAPI])
}

func TestResponsesCompactChannelSupport(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		apiType     int
		want        bool
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI, apiType: constant.APITypeOpenAI, want: true},
		{name: "Azure", channelType: constant.ChannelTypeAzure, apiType: constant.APITypeOpenAI, want: true},
		{name: "Codex", channelType: constant.ChannelTypeCodex, apiType: constant.APITypeCodex, want: true},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom, apiType: constant.APITypeAdvancedCustom, want: true},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API, apiType: constant.APITypeSub2API, want: true},
		{name: "New API", channelType: constant.ChannelTypeNewAPI, apiType: constant.APITypeNewAPI, want: true},
		{name: "Anthropic", channelType: constant.ChannelTypeAnthropic, apiType: constant.APITypeAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, common.SupportsResponsesCompact(test.channelType, test.apiType))
		})
	}
}

func TestMultiprotocolGatewayEndpointTypes(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
		constant.EndpointTypeAnthropic,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAIAlphaSearch,
	}

	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeNewAPI, "gpt-5"))
	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeSub2API, "gpt-5"))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.AuditLog{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.AuditLog{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.AuditLog
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	encodedAudit, err := common.Marshal(auditLog.Other)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encodedAudit, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	requestRules := []billingexpr.RequestRuleTrace{{
		Cond:       `param("service_tier") == "fast"`,
		Multiplier: 2,
		Matched:    true,
	}}
	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier:  "base",
		RequestRules: requestRules,
	})

	fields := other.Snapshot()
	require.Equal(t, "tiered_expr", fields["billing_mode"])
	require.Equal(t, "base", fields["matched_tier"])
	require.Equal(t, requestRules, fields["request_rules"])
	require.NotEmpty(t, fields["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestAutoBanOnlyUsesEligibleChannels(t *testing.T) {
	autoBanEnabled := 1
	autoBanDisabled := 0
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, AutoBan: &autoBanEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled, AutoBan: &autoBanDisabled},
		{Id: 3, Status: common.ChannelStatusAutoDisabled, AutoBan: &autoBanEnabled},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled, AutoBan: &autoBanEnabled},
		{Id: 5, Status: common.ChannelStatusEnabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeAutoBanOnly)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 3, selected[1].Id)
}

func TestRunChannelTestWorkersHonorsConfiguredConcurrency(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	progress := make([]int, 0, len(channels)+1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			context.Background(),
			channels,
			2,
			func(_ context.Context, _ *model.Channel) channelTestSummary {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	select {
	case <-started:
		t.Fatal("started more channel tests than the configured concurrency")
	default:
	}
	close(release)

	summary := <-summaryResult

	assert.Equal(t, int32(2), maxActive.Load())
	assert.Equal(t, channelTestSummary{Tested: 4, Succeeded: 4}, summary)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, progress)
}

func TestRunChannelTestWorkersStopsAfterCancellation(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	ctx, cancel := context.WithCancel(context.Background())
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	progress := make([]int, 0, 1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			ctx,
			channels,
			2,
			func(ctx context.Context, _ *model.Channel) channelTestSummary {
				started <- struct{}{}
				<-ctx.Done()
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	cancel()

	summary := <-summaryResult

	select {
	case <-started:
		t.Fatal("started another channel test after cancellation")
	default:
	}
	assert.Equal(t, channelTestSummary{Tested: 2, Succeeded: 2}, summary)
	assert.Equal(t, []int{0}, progress)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
