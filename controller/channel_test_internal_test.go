package controller

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
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
	channel := &model.Channel{Id: 987, Key: "offline-credential", Type: constant.ChannelTypeNewAPI, BaseURL: common.GetPointer("https://api.dflop.top")}
	// Tripwires: diagnostic evaluation must not touch a DB or any HTTP transport.
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousTransport := http.DefaultTransport
	calls := 0
	model.DB, model.LOG_DB = nil, nil
	http.DefaultTransport = diagnosticTripwireTransport{calls: &calls}
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB; http.DefaultTransport = previousTransport })
	resolved, blocked := 0, 0
	for _, row := range fixture.Models {
		t.Run(row.Model, func(t *testing.T) {
			_, genericPrice := ratio_setting.GetModelPrice(row.Model, false)
			_, genericRatio, _ := ratio_setting.GetModelRatio(row.Model)
			assert.False(t, genericPrice, "fixture must reproduce missing generic price")
			assert.False(t, genericRatio, "fixture must reproduce missing generic ratio")
			plan := resolveChannelTestTaskPricing(channel, registry.Generation(), row.Model, row.Model, "")
			diagnostic := taskChannelDiagnostic(channel, registry.Generation(), row.Model, "")
			if !row.Resolved && plan != nil {
				for _, check := range diagnostic.Checks {
					if check.Check == "pricing" {
						assert.Equal(t, plan.Reason, check.Reason)
					}
				}
			}
			assert.False(t, diagnostic.ConnectivityTested)
			assert.False(t, diagnostic.LiveGenerationTested)
			if row.Resolved {
				assert.Equal(t, "partial", diagnostic.Outcome)
				assert.Equal(t, "preflight_partial", diagnostic.Status)
				checks := map[string]service.TaskDiagnosticCheck{}
				for _, check := range diagnostic.Checks {
					checks[check.Check] = check
				}
				for _, name := range []string{"pricing", "ownership", "plugin_load"} {
					require.Contains(t, checks, name)
					assert.Equal(t, "pass", checks[name].Status)
				}
				assert.Equal(t, "not_tested", checks["credentials"].Status)
				assert.Equal(t, "not_tested", checks["request_constructibility"].Status)
				assert.Equal(t, "live_canary_requires_input", checks["request_constructibility"].Reason)
			} else {
				assert.Equal(t, "fail", diagnostic.Outcome)
			}
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
						assert.Equal(t, "PRICING_NOT_APPLIED", plan.Reason)
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
	assert.Zero(t, calls)
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
		{name: "no cross provider", model: "wan3.0-video", key: "alibaba", expression: `u("seconds") * 0.1`, reason: "PRICING_NOT_APPLIED"},
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
	channel := &model.Channel{Key: "offline-credential", Models: "tvod-veo-3.1", Type: constant.ChannelTypeNewAPI, BaseURL: common.GetPointer("https://api.dflop.top")}
	result := testChannel(context.Background(), channel, 123, "tvod-veo-3.1", "", false)
	require.NotNil(t, result.billingPlan)
	assert.True(t, result.billingPlan.Resolved)
	assert.Equal(t, "task_channel_test_unsupported", string(result.newAPIError.GetErrorCode()))
	assert.Equal(t, channelTestSummary{PreflightPartial: 1}, testChannelForHealthCheck(context.Background(), channel, 123, true, 0))
	channel.Type = constant.ChannelTypeTaskPlugin
	channel.Setting = common.GetPointer(`{"task_plugin_key":"dflop-media"}`)
	assert.Equal(t, channelTestSummary{PreflightPartial: 1}, testChannelForHealthCheck(context.Background(), channel, 123, true, 0))
	channel.Type = constant.ChannelTypeNewAPI
	channel.Setting = nil
	delete(settings.PluginBillingExpr, "dflop-media::tvod-veo-3.1")
	result = testChannel(context.Background(), channel, 123, "tvod-veo-3.1", "", false)
	require.NotNil(t, result.billingPlan)
	assert.False(t, result.billingPlan.Resolved)
	assert.Equal(t, "model_price_error", string(result.newAPIError.GetErrorCode()))
	channel.Models = "tvod-veo-3.1"
	assert.Equal(t, channelTestSummary{Failed: 1, PreflightFailed: 1}, testChannelForHealthCheck(context.Background(), channel, 123, true, 0))
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

func TestTaskChannelDiagnosticClassification(t *testing.T) {
	previousPrice := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrice)) })
	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("dflop-media")
	require.NoError(t, err)
	_, err = registry.RegisterFactory(source, jsplugin.Options{Key: "dflop-media"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name                              string
		channelType                       int
		binding, model, operation, status string
	}{
		{"type61", constant.ChannelTypeTaskPlugin, "dflop-media", "tvod-veo-3.1", "", "pricing_not_ready"},
		{"type60 task", constant.ChannelTypeNewAPI, "dflop-media", "tvod-veo-3.1", "", "pricing_not_ready"},
		{"type60 ordinary", constant.ChannelTypeNewAPI, "dflop-media", "gpt-4o", "", "ordinary_channel_test_required"},
		{"ordinary modifier", constant.ChannelTypeNewAPI, "dflop-media", "gpt-4o@temperature:0.2", "", "ordinary_channel_test_required"},
		{"missing plugin", constant.ChannelTypeTaskPlugin, "missing", "model", "", "plugin_load_failed"},
		{"unsupported operation", constant.ChannelTypeTaskPlugin, "dflop-media", "tvod-veo-3.1", "embedding", "operation_not_supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting, err := common.Marshal(map[string]any{"task_plugin_key": tc.binding})
			require.NoError(t, err)
			channel := &model.Channel{Key: "offline-credential", Type: tc.channelType, Setting: common.GetPointer(string(setting))}
			d := taskChannelDiagnostic(channel, registry.Generation(), tc.model, tc.operation)
			assert.Equal(t, tc.status, d.Status)
			assert.False(t, d.ConnectivityTested)
			assert.False(t, d.LiveGenerationTested)
		})
	}
}

type diagnosticTripwireTransport struct{ calls *int }

func (t diagnosticTripwireTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*t.calls++
	panic("offline preflight attempted network")
}

func TestTaskChannelDiagnosticRejectsUnsafeInput(t *testing.T) {
	for _, body := range []string{`{"mode":"live_canary"}`, `{"mode":"preflight","url":"https://example.com"}`, `{"mode":"preflight","input":{"image":"https://example.com"}}`, `{"mode":"preflight"} {}`, `{"mode":"preflight","model":42}`} {
		t.Run(body, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test/1/task", bytes.NewBufferString(body))
			TestTaskChannel(c)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.False(t, response.Success)
		})
	}
}

func TestTaskConnectivityHTTPMatrix(t *testing.T) {
	valid := `{"schema_version":"1.0","aliases":{"alias":"video"},"models":[{"id":"video"}]}`
	for _, tc := range []struct {
		name                              string
		code                              int
		contentType, body, status, access string
	}{
		{"valid", 200, "application/json", valid, "connectivity_pass", "confirmed"},
		{"missing model", 200, "application/json", `{"schema_version":"1.0","aliases":{},"models":[]}`, "connectivity_pass", "not_confirmed"},
		{"401", 401, "application/json", `{}`, "auth_failed", ""},
		{"403", 403, "application/json", `{}`, "auth_failed", ""},
		{"429", 429, "application/json", `{}`, "upstream_rate_limited", ""},
		{"500", 500, "application/json", `{}`, "upstream_service_error", ""},
		{"502", 502, "application/json", `{}`, "upstream_service_error", ""},
		{"invalid JSON", 200, "application/json", `{`, "connectivity_response_invalid", ""},
		{"wrong schema", 200, "application/json", `{"schema_version":"1.0","models":{}}`, "connectivity_response_invalid", ""},
		{"wrong version", 200, "application/json", `{"schema_version":"2.0","aliases":{},"models":[]}`, "connectivity_response_invalid", ""},
		{"wrong type", 200, "text/html", valid, "connectivity_response_invalid", ""},
		{"deceptive type", 200, "application/jsonjunk", valid, "connectivity_response_invalid", ""},
		{"oversized", 200, "application/json", strings.Repeat(" ", 4<<20+1), "connectivity_response_invalid", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/v1/catalog", r.URL.Path)
				assert.Equal(t, "Bearer offline-secret", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := dflop.Client{HTTP: &http.Client{Transport: catalogTestTransport{target: server.URL}}}
			channel := &model.Channel{Key: "offline-secret", BaseURL: common.GetPointer("https://api.dflop.top")}
			d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-media", Mode: "preflight", Outcome: "partial", Status: "preflight_partial", Model: "requested", MappedModel: "alias"}
			oldDB, oldLog := model.DB, model.LOG_DB
			model.DB, model.LOG_DB = nil, nil
			defer func() { model.DB, model.LOG_DB = oldDB, oldLog }()
			service.ProbeTaskConnectivity(context.Background(), d, channel, nil, client)
			assert.Equal(t, tc.status, d.ConnectivityStatus)
			assert.Equal(t, tc.access, d.ModelAccess)
			assert.True(t, d.ConnectivityTested)
			assert.False(t, d.LiveGenerationTested)
			assert.Equal(t, 1, calls)
			assert.Equal(t, "key_index:0", d.CredentialIdentity)
			assert.Zero(t, channel.ResponseTime)
			data, err := common.Marshal(d)
			require.NoError(t, err)
			assert.NotContains(t, string(data), "offline-secret")
			if tc.code == 429 || tc.access == "not_confirmed" {
				assert.Equal(t, "partial", d.Outcome)
			}
		})
	}
}

type catalogTestTransport struct {
	target          string
	bodyReadStarted chan struct{}
}

func (tr catalogTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != dflop.EffectiveCatalogURL {
		return nil, fmt.Errorf("unexpected outbound target")
	}
	target, err := url.Parse(tr.target)
	if err != nil {
		return nil, err
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = target.Scheme
	u.Host = target.Host
	clone.URL = &u
	clone.Host = target.Host
	response, err := http.DefaultTransport.RoundTrip(clone)
	if response != nil {
		response.Request = r
		if tr.bodyReadStarted != nil {
			response.Body = &catalogBodyReadSignal{ReadCloser: response.Body, started: tr.bodyReadStarted}
		}
	}
	return response, err
}

func TestTaskConnectivityProtectsCredentialsAndPreflight(t *testing.T) {
	calls := 0
	blockedCalls := 0
	blockedClient := dflop.Client{HTTP: &http.Client{Transport: diagnosticTripwireTransport{calls: &blockedCalls}}}
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; t.Error("credential reached attacker") }))
	defer attacker.Close()
	for _, base := range []string{attacker.URL, "https://api.dflop.top.evil.test", "https://api.dflop.top/private", "http://api.dflop.top"} {
		channel := &model.Channel{Key: "secret", BaseURL: &base}
		d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-media", Outcome: "partial", MappedModel: "video"}
		service.ProbeTaskConnectivity(context.Background(), d, channel, nil, blockedClient)
		assert.Equal(t, "connectivity_unavailable", d.ConnectivityStatus)
		assert.False(t, d.ConnectivityTested)
	}
	for _, location := range []string{dflop.EffectiveCatalogURL, attacker.URL} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Location", location); w.WriteHeader(302) }))
		channel := &model.Channel{Key: "secret", BaseURL: common.GetPointer("https://api.dflop.top")}
		d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-image", Outcome: "partial", MappedModel: "video"}
		service.ProbeTaskConnectivity(context.Background(), d, channel, nil, dflop.Client{HTTP: &http.Client{Transport: catalogTestTransport{target: server.URL}}})
		assert.Equal(t, "connectivity_response_invalid", d.ConnectivityStatus)
		server.Close()
	}
	assert.Zero(t, calls)
	channel := &model.Channel{Key: "disabled\nenabled", BaseURL: common.GetPointer("https://api.dflop.top"), ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 1, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}}}
	d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-tts", Outcome: "partial"}
	service.ProbeTaskConnectivity(context.Background(), d, channel, nil, blockedClient)
	assert.False(t, d.ConnectivityTested)
	assert.Equal(t, "connectivity_unavailable", d.ConnectivityStatus)
	for _, index := range []int{-1, 0, 2} {
		d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-tts", Outcome: "partial"}
		service.ProbeTaskConnectivity(context.Background(), d, channel, &index, blockedClient)
		assert.False(t, d.ConnectivityTested)
	}
	ordinary := &service.TaskChannelDiagnostic{Kind: "ordinary", Plugin: "dflop-media", Outcome: "untested"}
	service.ProbeTaskConnectivity(context.Background(), ordinary, channel, nil, blockedClient)
	assert.False(t, ordinary.ConnectivityTested)
	unreviewed := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "doubao", Outcome: "partial"}
	service.ProbeTaskConnectivity(context.Background(), unreviewed, channel, nil, blockedClient)
	assert.False(t, unreviewed.ConnectivityTested)
	assert.Zero(t, blockedCalls)
	index := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer enabled", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"1.0","aliases":{},"models":[]}`))
	}))
	defer server.Close()
	d = &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-tts", Outcome: "fail", Status: "pricing_not_ready", MappedModel: "video", Checks: []service.TaskDiagnosticCheck{{Check: "pricing", Status: "fail", Reason: "PLUGIN_PRICE_NOT_FOUND"}}}
	service.ProbeTaskConnectivity(context.Background(), d, channel, &index, dflop.Client{HTTP: &http.Client{Transport: catalogTestTransport{target: server.URL}}})
	assert.Equal(t, "connectivity_pass", d.ConnectivityStatus)
	assert.Equal(t, "fail", d.Outcome)
	assert.Equal(t, "pricing_not_ready", d.Status)
	assert.Equal(t, "key_index:1", d.CredentialIdentity)
	assert.Equal(t, 1, channel.ChannelInfo.MultiKeyPollingIndex)
	assert.Equal(t, common.ChannelStatusAutoDisabled, channel.ChannelInfo.MultiKeyStatusList[0])
	assert.Zero(t, channel.ResponseTime)
}

func TestTaskConnectivityCancellationAndConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	channel := &model.Channel{Key: "secret", BaseURL: common.GetPointer("https://api.dflop.top")}
	d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-media", Outcome: "partial"}
	client := dflop.Client{HTTP: &http.Client{Transport: catalogTestTransport{target: server.URL}}}
	service.ProbeTaskConnectivity(ctx, d, channel, nil, client)
	assert.Equal(t, "upstream_unreachable", d.ConnectivityStatus)
	server.Close()
	d = &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-media", Outcome: "partial"}
	service.ProbeTaskConnectivity(context.Background(), d, channel, nil, client)
	assert.Equal(t, "upstream_unreachable", d.ConnectivityStatus)
}

func TestTaskDiagnosticHTTPModes(t *testing.T) {
	oldDB, oldLog, oldMemory := model.DB, model.LOG_DB, common.MemoryCacheEnabled
	defer func() { model.DB, model.LOG_DB = oldDB, oldLog; common.MemoryCacheEnabled = oldMemory }()
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	channel := &model.Channel{Key: "do-not-expose", Type: constant.ChannelTypeTaskPlugin, Models: "tvod-veo-3.1", BaseURL: common.GetPointer("https://custom.invalid"), Setting: common.GetPointer(`{"task_plugin_key":"dflop-media"}`)}
	require.NoError(t, db.Create(channel).Error)
	for _, mode := range []string{"preflight", "connectivity"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(channel.Id)}}
		c.Request = httptest.NewRequest("POST", "/api/channel/test/1/task", strings.NewReader(fmt.Sprintf(`{"model":"tvod-veo-3.1","mode":%q,"input":null}`, mode)))
		TestTaskChannel(c)
		require.Equal(t, 200, w.Code)
		var response struct {
			Success bool                          `json:"success"`
			Data    service.TaskChannelDiagnostic `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		assert.True(t, response.Success)
		assert.Equal(t, mode, response.Data.Mode)
		assert.False(t, response.Data.ConnectivityTested)
		if mode == "connectivity" {
			assert.Equal(t, "connectivity_unavailable", response.Data.ConnectivityStatus)
		}
		assert.NotContains(t, w.Body.String(), channel.Key)
	}
}

func TestTaskConnectivityAlwaysVerifiesTLS(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"1.0","aliases":{},"models":[]}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // simulate gateway-wide legacy override
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, target.Host)
	}
	defer transport.CloseIdleConnections()
	d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-image", Outcome: "partial"}
	channel := &model.Channel{Key: "offline-secret", BaseURL: common.GetPointer("https://api.dflop.top")}
	service.ProbeTaskConnectivity(context.Background(), d, channel, nil, dflop.Client{HTTP: &http.Client{Transport: transport}})
	assert.Equal(t, "upstream_unreachable", d.ConnectivityStatus)
	assert.Zero(t, calls, "credential must not reach a server with an untrusted certificate")
}

type catalogBodyReadSignal struct {
	io.ReadCloser
	started chan struct{}
	once    sync.Once
}

func (r *catalogBodyReadSignal) Read(b []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(b)
}
func TestTaskConnectivityBodyCancellationIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-started; cancel() }()
	d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Plugin: "dflop-media", Outcome: "partial"}
	channel := &model.Channel{Key: "offline-secret", BaseURL: common.GetPointer("https://api.dflop.top")}
	service.ProbeTaskConnectivity(ctx, d, channel, nil, dflop.Client{HTTP: &http.Client{Transport: catalogTestTransport{target: server.URL, bodyReadStarted: started}}})
	assert.Equal(t, "upstream_unreachable", d.ConnectivityStatus)
}
func TestTaskConnectivityZeroLatencyEvidenceIsSerialized(t *testing.T) {
	data, err := common.Marshal(service.TaskChannelDiagnostic{Mode: "connectivity", ConnectivityTested: true, ConnectivityLatencyMS: 0})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"connectivity_latency_ms":0`)
}

func TestDiagnosticPreservesStructuredFailure(t *testing.T) {
	d := &service.TaskChannelDiagnostic{Model: "video", MappedModel: "canonical", Outcome: "partial"}
	d.Fail("pricing", "pricing_not_ready", "PLUGIN_PRICE_NOT_FOUND")
	d.CompleteDetails()
	raw, err := common.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"reason_code":"MISSING_PRICING"`)
	assert.Contains(t, string(raw), `"resolved_upstream_model":"canonical"`)
	assert.Contains(t, string(raw), `"overall_status":"fail"`)
	assert.Equal(t, "PLUGIN_PRICE_NOT_FOUND", d.Checks[0].Reason)
}

func TestDFLOPCatalogAndFailureEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, code, ledgerStatus, correlation, want string
		found                                                 bool
		status                                                int
	}{
		{name: "wrong protocol", protocol: "openai_chat", want: "PROTOCOL_MISMATCH", found: true, status: 503},
		{name: "healthy pool", protocol: "openai_image", code: "no_channel_available", want: "DFLOP_NO_HEALTHY_CHANNEL", found: true, status: 503},
		{name: "stale", want: "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG"},
		{name: "timeout ledger", protocol: "openai_image", ledgerStatus: "timeout", correlation: "EXACT_REQUEST_ID", want: "DFLOP_UPSTREAM_TIMEOUT", found: true, status: 524},
		{name: "timeout missing ledger", protocol: "openai_image", want: "INSUFFICIENT_EVIDENCE", found: true, status: 524},
		{name: "unreachable", protocol: "openai_image", code: "upstream_unreachable", want: "DFLOP_UPSTREAM_UNREACHABLE", found: true, status: 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := service.ChannelFailureEvidence{CatalogChecked: true, CatalogFound: tc.found, Callable: true, ExpectedProtocols: []string{"openai_image"}, Protocol: tc.protocol, HTTPStatus: tc.status, ProviderCode: tc.code, LedgerStatus: tc.ledgerStatus, Correlation: tc.correlation}
			assert.Equal(t, tc.want, service.ClassifyChannelFailure(evidence))
		})
	}
}

func TestDFLOPImageDiagnosticEndpoint(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.PricingSyncRun{}))
	channel := &model.Channel{Id: 1, Key: "offline-key", Type: constant.ChannelTypeNewAPI, BaseURL: lo.ToPtr("https://api.dflop.top")}
	names := []string{"tvod-seedream-4.5", "tvod-seedream-5.0-lite", "tvod-kling-expand", "tvod-kling-image-2.1", "tvod-kling-image-3.0", "tvod-kling-image-3.0-omni", "gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "tvod-qwen-image", "grok-imagine-image", "grok-imagine-image-quality", "tvod-vidu-image", "tvod-hunyuan-panorama", "nano-banana", "nano-banana-2", "nano-banana-pro"}
	models := []map[string]any{}
	for _, name := range names {
		models = append(models, map[string]any{"id": name, "pricing": map[string]any{"callable": true, "endpoint_type": "images_generations", "supported_protocols": []string{"openai_chat"}}})
	}
	models = append(models, map[string]any{"id": "chat", "pricing": map[string]any{"callable": true, "supported_protocols": []string{"openai_chat", "anthropic_messages", "openai_responses", "gemini_native"}}})
	raw, err := common.Marshal(map[string]any{"schema_version": "1.0", "currency": "points", "aliases": map[string]string{}, "models": models})
	require.NoError(t, err)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	record, err := common.Marshal(map[string]any{"source_mode": "AUTHENTICATED_EFFECTIVE", "source_channel": map[string]any{"id": 1}, "credential_fingerprint": fmt.Sprintf("%x", sha256.Sum256([]byte(channel.Key)))[:16], "effective_hash": fmt.Sprintf("%x", sha256.Sum256(raw)), "effective": map[string]any{"state": "FRESH"}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PricingSyncRun{ID: "offline-routing", Provider: "dflop", SourceSnapshot: base64.StdEncoding.EncodeToString(compressed.Bytes()), CurrencySnapshot: string(record)}).Error)
	previous := http.DefaultTransport
	calls := 0
	http.DefaultTransport = diagnosticTripwireTransport{calls: &calls}
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(channel, "", name))
			item, err := dflop.AuthenticatedCatalogModel(raw, name)
			require.NoError(t, err)
			binding, err := dflop.CatalogEndpoint(item)
			require.NoError(t, err)
			assert.Equal(t, "/v1/images/generations", binding.Path)
			_, err = dflop.CatalogEndpointForPath(item, "/v1/chat/completions")
			require.Error(t, err)
			d := service.TaskChannelDiagnostic{MappedModel: name, Outcome: "partial"}
			d.AttachAuthenticatedCatalog([]dflop.Item{item}, dflop.EffectiveMetadata{}, "fixture")
			assert.Equal(t, "ROUTING_FIXED_NOT_RUNTIME_REVERIFIED", d.RoutingStatus)
			assert.False(t, d.LiveGenerationTested)
		})
	}
	for _, name := range []string{"clip-mixcut", "clip-news", "clip-realman"} {
		_, err := dflop.AuthenticatedCatalogModel(raw, name)
		require.ErrorContains(t, err, "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG")
	}
	item, err := dflop.AuthenticatedCatalogModel(raw, "chat")
	require.NoError(t, err)
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1beta/models/{model}:generateContent"} {
		_, err := dflop.CatalogEndpointForPath(item, path)
		require.NoError(t, err)
	}
	channel.Key = "rotated"
	assert.Empty(t, normalizeChannelTestEndpoint(channel, "", "gpt-image-2"))
	assert.Zero(t, calls)
}

func TestDFLOPFailureAPIDataAndCatalogSemantics(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: lo.ToPtr("https://api.dflop.top")}
	for _, tc := range []struct {
		message, code, want string
		status              int
	}{
		{"no healthy channel for model `gpt-image-2`", "upstream_error", "DFLOP_NO_HEALTHY_CHANNEL", 503},
		{`{"error":{"code":"NO_HEALTHY_DEPLOYMENT"}}`, "upstream_error", "DFLOP_DEPLOYMENT_POOL_UNAVAILABLE", 503},
		{"error code: 524", "upstream_error", "INSUFFICIENT_EVIDENCE", 524},
	} {
		result := testResult{outboundAttempted: true, upstreamStatus: tc.status, newAPIError: kittypes.NewError(fmt.Errorf("%s", tc.message), kittypes.ErrorCode(tc.code), kittypes.ErrOptionWithStatusCode(tc.status))}
		d := channelFailureDiagnostic(channel, result, "gpt-image-2", string(constant.EndpointTypeImageGeneration))
		require.NotNil(t, d)
		require.Len(t, d.Checks, 1)
		assert.Equal(t, tc.want, d.Checks[0].ReasonCode)
		if tc.status == 524 {
			assert.Equal(t, "partial", d.Outcome)
		}
	}
	d := &service.TaskChannelDiagnostic{Model: "alias", MappedModel: "alias", Outcome: "partial"}
	d.AttachAuthenticatedCatalog([]dflop.Item{{ModelID: "canonical", Callable: true, ReasonCode: "UNKNOWN_BILLING_FEATURE", Reason: "unmapped vendor feature"}}, dflop.EffectiveMetadata{Aliases: map[string]string{"alias": "intermediate", "intermediate": "canonical"}}, "catalog-hash")
	assert.Equal(t, "canonical", d.CatalogModel)
	assert.Equal(t, "partial", d.Outcome)
	require.Len(t, d.Checks, 1)
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", d.Checks[0].ReasonCode)
	assert.Equal(t, "not_tested", d.Checks[0].Status)
	assert.Equal(t, "catalog-hash", d.Checks[0].Evidence)
}

func TestDFLOPRuntimeRejectionIsFailedWithoutInventingRootCause(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: lo.ToPtr("https://api.dflop.top")}
	for _, status := range []int{402, 403, 429, 502, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			upstream := service.RelayErrorHandler(context.Background(), &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"message":"provider rejected request"}`))}, true)
			result := testResult{outboundAttempted: true, upstreamStatus: status, newAPIError: kittypes.NewOpenAIError(upstream, kittypes.ErrorCodeBadResponse, 500)}
			d := channelFailureDiagnostic(channel, result, "doubao-seed-1-6-250615", "")
			require.NotNil(t, d)
			assert.Equal(t, "fail", d.Outcome)
			assert.Equal(t, "UPSTREAM_HTTP_ERROR", d.Checks[0].ReasonCode)
			assert.False(t, d.LiveGenerationTested)
		})
	}
	upstream := kittypes.WithOpenAIError(kittypes.OpenAIError{Message: "deployment unavailable", Code: "NO_HEALTHY_DEPLOYMENT"}, 503)
	d := channelFailureDiagnostic(channel, testResult{outboundAttempted: true, upstreamStatus: 503, newAPIError: kittypes.NewOpenAIError(upstream, kittypes.ErrorCodeBadResponse, 500)}, "doubao-seed-1-6-250615", "")
	assert.Equal(t, "DFLOP_DEPLOYMENT_POOL_UNAVAILABLE", d.Status)
	for _, status := range []int{0, 408, 504, 524} {
		d := channelFailureDiagnostic(channel, testResult{outboundAttempted: true, upstreamStatus: status, newAPIError: kittypes.NewOpenAIError(errors.New("network outcome unknown"), kittypes.ErrorCodeDoRequestFailed, 500)}, "chat", "")
		assert.Equal(t, "partial", d.Outcome)
		assert.Equal(t, "INSUFFICIENT_EVIDENCE", d.Checks[0].ReasonCode)
	}
}

func TestDFLOPLocalFailureRemainsFailedBeforeOutbound(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: lo.ToPtr("https://api.dflop.top")}
	err := fmt.Errorf("missing configured pricing")
	d := channelFailureDiagnostic(channel, testResult{localErr: err, newAPIError: kittypes.NewError(err, kittypes.ErrorCodeModelPriceError)}, "gpt-test", "")
	require.NotNil(t, d)
	assert.False(t, d.RuntimeAttempted)
	assert.Equal(t, "preflight", d.Mode)
	assert.Equal(t, "fail", d.Outcome)
	assert.Equal(t, "MISSING_PRICING", d.Checks[len(d.Checks)-1].ReasonCode)
}

func TestDFLOPDiagnosticKnownPricesAndBlockedQuantities(t *testing.T) {
	for _, tc := range []struct{ id, endpoint, reason, profile, formula string }{
		{"doubao-seedream-5-0-pro-260628", "images_generations", "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS", "IMAGE_PIXEL_TIER", ""},
		{"qwen-image-3.0-pro", "images_generations", "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS", "IMAGE_PIXEL_TIER", ""},
		{"tvod-subtitle-soft", "videos_generations", "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION", "SUBTITLE_SOURCE_SECONDS_BY_OPERATION", `u("source_duration_sec") * (1 + u("translation_units"))`},
	} {
		t.Run(tc.id, func(t *testing.T) {
			plugin := "dflop-image"
			features := []string{"image_size_bands", "input_images", "per_image"}
			if tc.endpoint == "videos_generations" {
				plugin = "dflop-media"
				features = []string{"video_second", "video_tiers"}
			}
			item := dflop.Item{ModelID: tc.id, Callable: true, EndpointType: tc.endpoint, TaskPlugin: plugin, BillingFeatures: features, Prices: map[string]dflop.Price{"known": {Credits: "1"}}, Expression: tc.formula, ReasonCode: tc.reason}
			d := service.TaskChannelDiagnostic{MappedModel: tc.id, Outcome: "fail", Checks: []service.TaskDiagnosticCheck{{Check: "pricing", Status: "fail", Reason: "PLUGIN_PRICE_NOT_FOUND"}}}
			d.AttachAuthenticatedCatalog([]dflop.Item{item}, dflop.EffectiveMetadata{}, "authenticated-fixture")
			assert.Equal(t, "known", d.CatalogPricingState)
			assert.Equal(t, "blocked", d.QuantityState)
			assert.Equal(t, tc.profile, d.UsageProfile)
			assert.Equal(t, tc.reason, d.Checks[0].ReasonCode)
			assert.Equal(t, "fail", d.ActivePricingState)
			assert.False(t, d.RuntimeAttempted)
			if tc.formula != "" {
				assert.Equal(t, "known", d.FormulaState)
			}
		})
	}
	item := dflop.Item{ModelID: "claude", Callable: true, Protocols: []string{"openai_chat", "anthropic_messages"}}
	d := service.TaskChannelDiagnostic{MappedModel: "claude", Kind: "ordinary", Outcome: "partial", Protocol: "anthropic", Endpoint: "/v1/messages"}
	d.AttachAuthenticatedCatalog([]dflop.Item{item}, dflop.EffectiveMetadata{}, "fixture")
	assert.Equal(t, "anthropic_messages", d.Protocol)
	assert.Equal(t, "/v1/messages", d.Endpoint)
	assert.NotEqual(t, "fail", d.Outcome)
}

func TestRuntimeVerificationReadOnlyScopeAndEmptyEvidence(t *testing.T) {
	previousDB, previousLog := model.DB, model.LOG_DB
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLog })
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RuntimeVerificationRun{}, &model.RuntimeVerificationItem{}))
	priorRun := &model.RuntimeVerificationRun{ChannelID: 1, Source: "dflop", CatalogHash: strings.Repeat("c", 64), CredentialFingerprint: strings.Repeat("d", 64), Mode: "prepare"}
	priorItems := []model.RuntimeVerificationItem{{Model: "image-model", Protocol: "openai_image", Operation: "generate", Mode: "default", Endpoint: "/v1/images/generations", ConfigStatus: "PASS"}}
	require.NoError(t, model.CreateRuntimeVerificationRun(priorRun, priorItems))
	require.NoError(t, db.Model(&model.RuntimeVerificationItem{}).Where("run_id = ?", priorRun.ID).Updates(map[string]any{"request_body_hash": strings.Repeat("e", 64), "request_id": "prior-request", "result": "RUNTIME_AMBIGUOUS"}).Error)
	run := &model.RuntimeVerificationRun{ChannelID: 1, Source: "dflop", CatalogHash: strings.Repeat("a", 64), CredentialFingerprint: strings.Repeat("b", 64), Mode: "prepare"}
	items := []model.RuntimeVerificationItem{{Model: "image-model", Protocol: "openai_image", Operation: "generate", Mode: "default", Endpoint: "/v1/images/generations", ConfigStatus: "PASS", ReasonCode: "READY_FOR_PAID_AUTHORIZATION"}}
	require.NoError(t, model.CreateRuntimeVerificationRun(run, items))
	for _, tc := range []struct {
		name        string
		channelID   string
		runID       string
		wantSuccess bool
		wantItems   int
	}{
		{"latest persisted evidence", "1", "", true, 1},
		{"exact persisted run", "1", strconv.FormatInt(run.ID, 10), true, 1},
		{"other channel cannot read the run", "2", strconv.FormatInt(run.ID, 10), false, 0},
		{"missing run has no evidence", "1", "99999", false, 0},
		{"untested channel is explicitly empty", "2", "", true, 0},
		{"nonpositive channel is rejected", "0", "", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: tc.channelID}, {Key: "run_id", Value: tc.runID}}
			c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+tc.channelID+"/runtime-verification", nil)
			GetChannelRuntimeVerification(c)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Run   *model.RuntimeVerificationRun `json:"run"`
					Items []struct {
						Model              string `json:"model"`
						Status             string `json:"status"`
						RequestStatus      string `json:"request_status"`
						HistoricalEvidence *struct {
							RunID  int64  `json:"run_id"`
							Result string `json:"result"`
						} `json:"historical_evidence"`
					} `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, tc.wantSuccess, response.Success)
			assert.Len(t, response.Data.Items, tc.wantItems)
			if tc.wantItems > 0 {
				assert.Equal(t, "CONFIG_READY_RUNTIME_UNTESTED", response.Data.Items[0].Status)
				assert.Equal(t, "NOT_TESTED", response.Data.Items[0].RequestStatus)
				if tc.runID == "" {
					require.NotNil(t, response.Data.Items[0].HistoricalEvidence)
					assert.Equal(t, priorRun.ID, response.Data.Items[0].HistoricalEvidence.RunID)
					assert.Equal(t, "RUNTIME_AMBIGUOUS", response.Data.Items[0].HistoricalEvidence.Result)
				} else {
					assert.Nil(t, response.Data.Items[0].HistoricalEvidence)
				}
			}
			assert.NotContains(t, w.Body.String(), "credential_fingerprint")
			assert.NotContains(t, w.Body.String(), "authorization_manifest_hash")
			assert.NotContains(t, w.Body.String(), "billing_snapshot_json")
		})
	}
	stored, err := model.GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Zero(t, stored.PaidRequests)
}

func TestRuntimeVerificationPrepareRejectsClientEvidenceAndPaidOverrides(t *testing.T) {
	oldDB, oldTransport := model.DB, http.DefaultTransport
	calls := 0
	model.DB = nil
	http.DefaultTransport = diagnosticTripwireTransport{calls: &calls}
	t.Cleanup(func() { model.DB, http.DefaultTransport = oldDB, oldTransport })
	for _, body := range []string{
		`{"model":"image-model","mode":"paid"}`,
		`{"model":"image-model","input":{}}`,
		`{"model":"image-model","config_status":"PASS"}`,
		`{"model":"image-model","approved":true}`,
		`{"model":123}`,
		`{"model":"` + strings.Repeat("a", 129) + `"}`,
		strings.Repeat("x", 4097),
		`null`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/runtime-verification", strings.NewReader(body))
		PrepareChannelRuntimeVerification(c)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		assert.False(t, response.Success, body)
	}
	assert.Zero(t, calls)
}

func TestRuntimeVerificationExecuteRejectsUnsignedAndUnconfirmedBeforeIO(t *testing.T) {
	oldDB, oldTransport := model.DB, http.DefaultTransport
	calls := 0
	model.DB = nil
	http.DefaultTransport = diagnosticTripwireTransport{calls: &calls}
	t.Cleanup(func() { model.DB, http.DefaultTransport = oldDB, oldTransport })
	t.Setenv("DFLOP_VERIFICATION_APPROVAL_PUBLIC_KEY", strings.Repeat("ab", 32))
	for _, body := range []string{`{}`, `null`, `{"manifest_json":"{\"approved\":true}","confirmed_manifest_hash":"wrong"}`, strings.Repeat("x", (4<<20)+1)} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set("id", 73)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/runtime-verification/execute", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		ExecuteChannelRuntimeVerification(c)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
	}
	assert.Zero(t, calls)
}

func TestRuntimeVerificationAdminRejectsSimpleAndCrossSiteRequests(t *testing.T) {
	for _, handler := range []gin.HandlerFunc{PlanChannelRuntimeVerification, ExecuteChannelRuntimeVerification, ResumeChannelRuntimeVerification} {
		for _, tc := range []struct{ contentType, site string }{{"text/plain", "same-origin"}, {"application/json", "cross-site"}, {"application/x-www-form-urlencoded", "same-origin"}} {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Params = gin.Params{{Key: "id", Value: "1"}}
			c.Set("id", 73)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/runtime-verification/execute", strings.NewReader(`{}`))
			c.Request.Header.Set("Content-Type", tc.contentType)
			c.Request.Header.Set("Sec-Fetch-Site", tc.site)
			handler(c)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
		}
	}
}
