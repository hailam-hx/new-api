package service

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVerificationBudgetAndExactLedger(t *testing.T) {
	item := dflop.Item{ModelID: "qwen-image-3.0", PriceSemantics: dflop.PriceSemantics{SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"}, BillingFeatures: []string{"input_images", "per_image"}, Prices: map[string]dflop.Price{
		"price_per_image":       {EffectiveCredits: "0.100000000001", SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"},
		"price_per_input_image": {EffectiveCredits: "0.200000000002", SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"},
	}, Raw: []byte(`{"free_input_images":1}`)}
	points, err := DFLOPVerificationPoints(item, map[string]any{"image_count": 1, "input_image_count": 2})
	require.NoError(t, err)
	assert.Equal(t, "0.300000000003", points.String())
	_, err = DFLOPVerificationPoints(item, map[string]any{"image_count": 1})
	require.ErrorContains(t, err, "BILLING_QUANTITY_MISSING")
	_, err = DFLOPVerificationPoints(item, map[string]any{"image_count": -1, "input_image_count": 0})
	require.Error(t, err)
	_, err = DFLOPVerificationPoints(item, map[string]any{"image_count": 129, "input_image_count": 0})
	require.Error(t, err)

	execution := model.RuntimeVerificationItem{Model: item.ModelID, RequestID: "request-exact", TaskID: "task-exact", TraceID: "trace-exact"}
	for _, tc := range []struct{ name, body, result string }{
		{"exact request", `{"model":"qwen-image-3.0","request_id":"request-exact","cost":"0.300000000003","status":"succeeded"}`, "EXACT_MATCH"},
		{"exact task", `{"model":"qwen-image-3.0","task_id":"task-exact","cost":"0.300000000003","status":"succeeded"}`, "EXACT_MATCH"},
		{"exact trace", `{"model":"qwen-image-3.0","x_gateway_trace":"trace-exact","cost":"0.300000000003","status":"succeeded"}`, "EXACT_MATCH"},
		{"same model weak", `{"model":"qwen-image-3.0","cost":"0.300000000003","status":"succeeded"}`, "INSUFFICIENT_EVIDENCE"},
		{"conflicting task", `{"model":"qwen-image-3.0","task_id":"wrong","request_id":"request-exact","cost":"0.300000000003","status":"succeeded"}`, "INSUFFICIENT_EVIDENCE"},
		{"wrong model", `{"model":"qwen-image-3.0-pro","request_id":"request-exact","cost":"0.300000000003","status":"succeeded"}`, "INSUFFICIENT_EVIDENCE"},
		{"no tolerance", `{"model":"qwen-image-3.0","request_id":"request-exact","cost":"0.3","status":"succeeded"}`, "LEDGER_MISMATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ReconcileDFLOPVerificationLedger(execution, []byte(tc.body), points.String())
			assert.Equal(t, tc.result, result.Result)
		})
	}
}

func TestVerificationLedgerDeploymentCodeRequiresExactTerminalIdentity(t *testing.T) {
	item := model.RuntimeVerificationItem{Model: "test", TaskID: "task", TraceID: "trace"}
	for _, test := range []struct{ name, body, result, code string }{
		{"exact terminal rejected zero cost", `{"model":"test","task_id":"task","status":"rejected","cost":"0","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "EXACT_MATCH", "NO_HEALTHY_DEPLOYMENT"},
		{"weak identity cannot classify deployment", `{"model":"test","status":"rejected","cost":"0","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "INSUFFICIENT_EVIDENCE", ""},
		{"conflicting task defeats trace", `{"model":"test","task_id":"different-task","request_id":"trace","status":"rejected","cost":"0","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "INSUFFICIENT_EVIDENCE", ""},
		{"nonterminal cannot classify deployment", `{"model":"test","task_id":"task","status":"running","cost":"0","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "INSUFFICIENT_EVIDENCE", ""},
		{"invalid currency cannot prove refund", `{"model":"test","task_id":"task","status":"error","cost":"0","currency":"USD","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "INSUFFICIENT_EVIDENCE", ""},
		{"charged rejection cannot prove refund", `{"model":"test","task_id":"task","status":"rejected","cost":"0.1","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "LEDGER_MISMATCH", ""},
		{"success cannot classify provider block", `{"model":"test","task_id":"task","status":"success","cost":"0","error_code":"NO_HEALTHY_DEPLOYMENT"}`, "EXACT_MATCH", ""},
		{"unrecognized error text excluded", `{"model":"test","task_id":"task","status":"error","cost":"0","error_code":"private-provider-detail"}`, "EXACT_MATCH", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := ReconcileDFLOPVerificationLedger(item, []byte(test.body), "0")
			assert.Equal(t, test.result, result.Result)
			assert.Equal(t, test.code, result.ProviderErrorCode)
		})
	}
}

func TestVerificationAuthorizationCannotBeInferredOrRolledOver(t *testing.T) {
	now := time.Unix(1800000000, 0)
	run := model.RuntimeVerificationRun{ID: 9, ChannelID: 1, CatalogHash: "catalog", CredentialFingerprint: "fingerprint"}
	item := model.RuntimeVerificationItem{Model: "voice-tts-pro", FixtureID: "tts-v1", PricingSnapshotHash: "price", BillingExprHash: "expr", RequestBodyHash: "body", Protocol: "openai_audio_speech", Mode: "default"}
	manifest := DFLOPVerificationAuthorization{Approved: true, ChannelID: 1, CatalogHash: "catalog", CredentialFingerprint: "fingerprint", Models: []string{"voice-tts-pro"}, MaxRequests: 1, MaxTotalProviderPoints: "0.2", MaxCostPerRequest: map[string]string{"voice-tts-pro": "0.2"}, ExpiresAt: now.Add(time.Hour).Unix(), ApprovedBy: "admin", ApprovalReference: "explicit-user-approval", Targets: []DFLOPVerificationTarget{{Model: item.Model, FixtureID: item.FixtureID, Protocol: item.Protocol, Mode: item.Mode, PricingSnapshotHash: item.PricingSnapshotHash, BillingExprHash: item.BillingExprHash, RequestBodyHash: item.RequestBodyHash}}}
	require.NoError(t, ValidateDFLOPVerificationAuthorization(manifest, run, item, "0.1", now))
	for _, mutate := range []func(*DFLOPVerificationAuthorization){
		func(a *DFLOPVerificationAuthorization) { a.Approved = false },
		func(a *DFLOPVerificationAuthorization) { a.CatalogHash = "changed" },
		func(a *DFLOPVerificationAuthorization) { a.CredentialFingerprint = "other-key" },
		func(a *DFLOPVerificationAuthorization) { a.FundingUserID = 1 },
		func(a *DFLOPVerificationAuthorization) { a.ExpiresAt = now.Unix() },
		func(a *DFLOPVerificationAuthorization) { a.MaxRequests = 0 },
		func(a *DFLOPVerificationAuthorization) { a.ApprovalReference = "" },
		func(a *DFLOPVerificationAuthorization) { a.MaxTotalProviderPoints = "0.099999999999" },
	} {
		copy := manifest
		mutate(&copy)
		require.Error(t, ValidateDFLOPVerificationAuthorization(copy, run, item, "0.1", now))
	}
	run.PaidRequests = 1
	require.ErrorContains(t, ValidateDFLOPVerificationAuthorization(manifest, run, item, "0.1", now), "REQUEST_LIMIT")
}

func TestVerificationProviderPointsUseProductionCharacterFact(t *testing.T) {
	item := dflop.Item{ModelID: "voice-tts-pro", EndpointType: "tts_synthesize", PriceSemantics: dflop.PriceSemantics{SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"}, BillingFeatures: []string{"tts_char"}, Prices: map[string]dflop.Price{"price_per_tts_char": {EffectiveCredits: "0.13207", SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"}}}
	points, err := DFLOPVerificationPoints(item, map[string]any{"character_count": float64(6)})
	require.NoError(t, err)
	assert.Equal(t, "0.79242", points.String())
	_, err = DFLOPVerificationPoints(item, map[string]any{"character_count": float64(6), "characters": float64(7)})
	require.ErrorContains(t, err, "BILLING_QUANTITY_INVALID")
}

func TestVerificationLedgerPreservesDecimalAndChecksProductionQuantities(t *testing.T) {
	for _, test := range []struct {
		name, normalized, provider, body, expected, result string
	}{
		{"numeric twelve decimal places", "", "", `{"model":"test","task_id":"task","cost":12345.123456789012,"status":"success"}`, "12345.123456789012", "EXACT_MATCH"},
		{"numeric unit precision", `{"duration_sec":"3.123456789012"}`, `{"unit_count":"3.123456789012"}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_type":"video","unit_count":3.123456789012}`, "1", "EXACT_MATCH"},
		{"same cost wrong image count", `{"image_count":4}`, `{"unit_count":4}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_type":"image","unit_count":1}`, "1", "PROVIDER_USAGE_MISMATCH"},
		{"same cost wrong tokens", `{"completion_tokens":123}`, `{"usage":{"completion_tokens":123}}`, `{"model":"test","task_id":"task","cost":"1","status":"success","output_tokens":124}`, "1", "PROVIDER_USAGE_MISMATCH"},
		{"wrong production extraction", `{"character_count":6}`, `{"characters":7}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_type":"audio","unit_count":7}`, "1", "PARSER_MISMATCH"},
		{"wrong reference video seconds", `{"duration_sec":2,"input_video_duration_sec":3}`, `{"duration_sec":2,"input_video_duration_sec":3}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_type":"video","unit_count":2}`, "1", "PROVIDER_USAGE_MISMATCH"},
		{"missing ledger units", `{"character_count":6}`, `{"characters":6}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_type":"audio","unit_count":null}`, "1", "INSUFFICIENT_EVIDENCE"},
		{"missing ledger unit subject", `{"character_count":6}`, `{"characters":6}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_count":6}`, "1", "INSUFFICIENT_EVIDENCE"},
		{"wrong ledger unit subject", `{"image_count":4}`, `{"unit_count":4}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_type":"audio","unit_count":4}`, "1", "PROVIDER_USAGE_MISMATCH"},
		{"missing ledger output tokens", `{"completion_tokens":123}`, `{"usage":{"completion_tokens":123}}`, `{"model":"test","task_id":"task","cost":"1","status":"success","unit_count":null}`, "1", "INSUFFICIENT_EVIDENCE"},
		{"zero refund documented error", "", "", `{"model":"test","task_id":"task","cost":"0","status":"error"}`, "0", "EXACT_MATCH"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := model.RuntimeVerificationItem{Model: "test", TaskID: "task", NormalizedUsageJSON: test.normalized, ProviderUsageJSON: test.provider}
			result := ReconcileDFLOPVerificationLedger(item, []byte(test.body), test.expected)
			assert.Equal(t, test.result, result.Result)
			if test.name == "numeric twelve decimal places" {
				assert.Equal(t, test.expected, result.ProviderCostPoints)
			}
			if test.name == "numeric unit precision" {
				assert.Equal(t, "3.123456789012", result.ProviderUnitCount)
			}
		})
	}
}

func TestVerificationLedgerRequestIDCanBeExactGatewayTrace(t *testing.T) {
	item := model.RuntimeVerificationItem{Model: "test", TraceID: "gateway-trace"}
	body := []byte(`{"model":"test","request_id":"gateway-trace","cost":"0","status":"rejected"}`)
	result := ReconcileDFLOPVerificationLedger(item, body, "0")
	assert.Equal(t, "EXACT_MATCH", result.Result)
	assert.Equal(t, "EXACT_TRACE_ID", result.CorrelationQuality)
	item.RequestID = "provider-request"
	result = ReconcileDFLOPVerificationLedger(item, body, "0")
	assert.Equal(t, "EXACT_MATCH", result.Result)
	assert.Equal(t, "EXACT_TRACE_ID", result.CorrelationQuality)
	result = ReconcileDFLOPVerificationLedger(item, []byte(`{"model":"test","trace_id":"gateway-trace","cost":"0","status":"rejected"}`), "0")
	assert.Equal(t, "INSUFFICIENT_EVIDENCE", result.Result)
}

// All outbound traffic goes through this transport; an unexpected URL fails
// the fixture rather than reaching a real provider or fetching output media.
type verificationEngineTransport struct {
	t             *testing.T
	catalog       string
	catalogStatus int
	terminal      string
	ledger        string
	postStatus    int
	postBody      string
	omitSubmitIDs bool
	postError     error
	pollError     error
	ledgerStatus  int
	posts         int
	polls         int
	ledgerReads   int
	onPost        func()
	onPoll        func()
}

func (transport *verificationEngineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	assert.Equal(transport.t, "https", request.URL.Scheme)
	assert.Equal(transport.t, "api.dflop.top", request.URL.Host)
	status, body := http.StatusOK, ""
	switch {
	case request.Method == http.MethodGet && request.URL.String() == dflop.EffectiveCatalogURL:
		status, body = transport.catalogStatus, transport.catalog
	case request.Method == http.MethodGet && request.URL.String() == dflop.CurrencyURL:
		body = `{"unit":"points","points_per_cny":60}`
	case request.Method == http.MethodPost && request.URL.Path == "/v1/audio/speech":
		transport.posts++
		assert.NotEmpty(transport.t, request.Header.Get("Idempotency-Key"))
		payload, err := io.ReadAll(request.Body)
		require.NoError(transport.t, err)
		var fields map[string]any
		require.NoError(transport.t, common.Unmarshal(payload, &fields))
		assert.Equal(transport.t, "Hello.", fields["input"])
		assert.Equal(transport.t, true, fields["async"])
		if transport.onPost != nil {
			transport.onPost()
		}
		if transport.postError != nil {
			return nil, transport.postError
		}
		status, body = transport.postStatus, transport.postBody
		if body == "" {
			body = `{"id":"engine-task","model":"voice-tts-pro","status":"pending"}`
		}
	case request.Method == http.MethodGet && request.URL.Path == "/v1/audio/speech/engine-task":
		transport.polls++
		if transport.onPoll != nil {
			transport.onPoll()
		}
		if transport.pollError != nil {
			return nil, transport.pollError
		}
		body = transport.terminal
	case request.Method == http.MethodGet && request.URL.Path == "/v1/logs":
		transport.ledgerReads++
		assert.Contains(transport.t, []string{"engine-task", "engine-request"}, request.URL.Query().Get("ref"))
		assert.Equal(transport.t, "key", request.URL.Query().Get("scope"))
		status, body = transport.ledgerStatus, transport.ledger
	default:
		transport.t.Errorf("unexpected verification request: %s %s", request.Method, request.URL.Path)
		return nil, errors.New("unexpected verification fixture request")
	}
	headers := http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"engine-request"}, "X-Gateway-Trace": {"engine-trace"}}
	if request.Method == http.MethodGet && request.URL.Path == "/v1/audio/speech/engine-task" {
		headers.Set("X-Gateway-Trace", "engine-poll-trace")
	}
	if transport.omitSubmitIDs && request.Method == http.MethodPost {
		headers.Del("X-Request-Id")
		headers.Del("X-Gateway-Trace")
	}
	return &http.Response{StatusCode: status, Request: request, Header: headers, Body: io.NopCloser(strings.NewReader(body))}, nil
}

type verificationEngineRig struct {
	engine        DFLOPVerificationEngine
	transport     *verificationEngineTransport
	run           *model.RuntimeVerificationRun
	item          model.RuntimeVerificationItem
	fixture       VerificationFixture
	authorization DFLOPVerificationAuthorization
	dbPath        string
}

func newVerificationEngineRig(t *testing.T) *verificationEngineRig {
	t.Helper()
	priorDB, priorLog, priorRegistry := model.DB, model.LOG_DB, jsplugin.DefaultRegistry
	priorRedis, priorBatch, priorConsume, priorMemory, priorQuotaUnit, priorMaster := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.MemoryCacheEnabled, common.QuotaPerUnit, common.IsMasterNode
	billing := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	priorExpressions := maps.Clone(billing.PluginBillingExpr)
	priorGroupRatios := ratio_setting.GroupRatio2JSONString()
	rig := &verificationEngineRig{dbPath: filepath.Join(t.TempDir(), "verification.db")}
	db, err := gorm.Open(sqlite.Open(rig.dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.MemoryCacheEnabled, common.QuotaPerUnit = false, false, true, false, 500000
	jsplugin.DefaultRegistry = jsplugin.NewRegistry()
	t.Cleanup(func() {
		current, err := model.DB.DB()
		require.NoError(t, err)
		require.NoError(t, current.Close())
		model.DB, model.LOG_DB, jsplugin.DefaultRegistry = priorDB, priorLog, priorRegistry
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.MemoryCacheEnabled, common.QuotaPerUnit = priorRedis, priorBatch, priorConsume, priorMemory, priorQuotaUnit
		common.IsMasterNode = priorMaster
		billing.PluginBillingExpr = priorExpressions
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(priorGroupRatios))
	})
	// Service TestMain opens GORM directly; the startup initializer supplies the
	// reserved-column quoting used by config reads without migrations or I/O.
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false
	require.NoError(t, model.InitLogDB())
	common.IsMasterNode = priorMaster
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Channel{}, &model.Token{}, &model.Task{}, &model.Log{}, &model.UserSubscription{}, &model.RuntimeVerificationRun{}, &model.RuntimeVerificationItem{}))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	seedUser(t, 73, 100000)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 73).Update("group", "default").Error)
	baseURL := "https://api.dflop.top"
	channel := &model.Channel{Id: 1, Name: "verification fixture", Type: constant.ChannelTypeNewAPI, Status: common.ChannelStatusEnabled, BaseURL: &baseURL, Key: "synthetic-verification-key", Models: "voice-tts-pro"}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "dflop-tts"})
	require.NoError(t, db.Create(channel).Error)
	settings := model.DefaultDFLOPConfig()
	settings.SourceChannelID, settings.CNYToUSD = 1, "0.15"
	settingsJSON, err := common.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Option{Key: model.DFLOPConfigOption, Value: string(settingsJSON)}).Error)
	pluginSource, err := os.ReadFile("../plugins/tasks/dflop-tts/plugin.js")
	require.NoError(t, err)
	_, err = jsplugin.DefaultRegistry.RegisterFactory(string(pluginSource), jsplugin.Options{Key: "dflop-tts"})
	require.NoError(t, err)
	rig.transport = &verificationEngineTransport{t: t, catalogStatus: 200, postStatus: 200, ledgerStatus: 200,
		catalog:  `{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"voice-tts-pro","pricing":{"category":"audio","endpoint_type":"tts_synthesize","callable":true,"price_per_tts_char":"1"},"billing":{"features":["tts_char"]},"caps":{"surfaces":["voice"]}}]}`,
		terminal: `{"id":"engine-task","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://example.invalid/synthetic.wav"}`,
		ledger:   `{"currency":"points","scope":"key","logs":[{"id":"engine-ledger","model":"voice-tts-pro","task_id":"engine-task","request_id":"engine-request","x_gateway_trace":"engine-trace","cost":"6","status":"succeeded","unit_type":"audio","unit_count":6}]}`,
	}
	rig.engine.HTTP = &http.Client{Transport: rig.transport}
	source, err := rig.engine.source(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, source.items, 1)
	billing.PluginBillingExpr = map[string]string{billing_setting.PluginBillingExprKey("dflop-tts", "voice-tts-pro"): source.items[0].TaskExpression}
	fixtures := DFLOPVerificationFixtures(source.items)
	fixtureIndex := slices.IndexFunc(fixtures, func(f VerificationFixture) bool { return f.Model == "voice-tts-pro" })
	require.NotEqual(t, -1, fixtureIndex)
	rig.fixture = fixtures[fixtureIndex]
	rig.fixture.SourceCatalogHash = source.hash
	prepared, items, err := rig.engine.Prepare(t.Context(), 1, "voice-tts-pro", 73)
	require.NoError(t, err)
	itemIndex := slices.IndexFunc(items, func(i model.RuntimeVerificationItem) bool { return i.Model == "voice-tts-pro" })
	require.NotEqual(t, -1, itemIndex)
	preparedItem := items[itemIndex]
	require.NotEmpty(t, preparedItem.BillingSnapshotJSON)
	plugin, ok := jsplugin.DefaultRegistry.Generation().Get("dflop-tts")
	require.True(t, ok)
	request, err := ValidateDFLOPVerificationFixture(t.Context(), rig.fixture, plugin)
	require.NoError(t, err)
	rig.authorization = DFLOPVerificationAuthorization{Approved: true, ChannelID: 1, FundingUserID: 73, CatalogHash: prepared.CatalogHash, CredentialFingerprint: prepared.CredentialFingerprint, Models: []string{"voice-tts-pro"}, MaxRequests: 1, MaxCostPerRequest: map[string]string{"voice-tts-pro": "6"}, MaxTotalProviderPoints: "6", ExpiresAt: time.Now().Add(time.Hour).Unix(), ApprovedBy: "fixture-admin", ApprovalReference: "synthetic-test-only", Targets: []DFLOPVerificationTarget{{Model: preparedItem.Model, Protocol: preparedItem.Protocol, Mode: preparedItem.Mode, FixtureID: preparedItem.FixtureID, PricingSnapshotHash: preparedItem.PricingSnapshotHash, BillingExprHash: preparedItem.BillingExprHash, RequestBodyHash: request.BodyHash}}}
	rig.run, items, err = rig.engine.CreateAuthorizedRun(t.Context(), 1, 73, rig.authorization)
	require.NoError(t, err)
	require.Len(t, items, 1)
	rig.item = items[0]
	assert.Zero(t, rig.transport.posts, "source and preparation must be free GETs only")
	return rig
}

func (rig *verificationEngineRig) savedItem(t *testing.T) model.RuntimeVerificationItem {
	t.Helper()
	items, err := model.ListRuntimeVerificationItems(rig.run.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	return items[0]
}

func (rig *verificationEngineRig) reopen(t *testing.T) {
	t.Helper()
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	db, err := gorm.Open(sqlite.Open(rig.dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err = db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
}

func TestVerificationEngineSuccessDebitsOnceWithExactLedger(t *testing.T) {
	rig := newVerificationEngineRig(t)
	require.NoError(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
	saved := rig.savedItem(t)
	assert.Equal(t, "RUNTIME_VERIFIED", saved.Result, saved.ReasonCode)
	assert.Equal(t, "EXACT_TASK_ID", saved.CorrelationQuality)
	assert.Equal(t, "6", saved.ProviderCostPoints)
	var evidence model.RuntimeVerificationEvidence
	require.NoError(t, common.UnmarshalJsonStr(saved.EvidenceJSON, &evidence))
	assert.Equal(t, 1, evidence.PollAttempts)
	assert.Equal(t, []string{"engine-poll-trace"}, evidence.PollTraceIDs)
	assert.Equal(t, "engine-trace", saved.TraceID, "a free poll trace must not replace the billed submit identity")
	assert.Equal(t, 92500, getUserQuota(t, 73))
	require.NotNil(t, saved.NewapiQuota)
	assert.Equal(t, 7500, *saved.NewapiQuota)
	require.NotNil(t, saved.WalletDelta)
	assert.Equal(t, -7500, *saved.WalletDelta)
	journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "BILLING_RESERVATION_SETTLED", journal.Content)
	var consumeLogs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 73, model.LogTypeConsume).Count(&consumeLogs).Error)
	assert.Equal(t, int64(1), consumeLogs)
	rig.reopen(t)
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	require.ErrorContains(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture), "SUBMIT_ALREADY_CLAIMED")
	assert.Equal(t, 92500, getUserQuota(t, 73))
	assert.Equal(t, 1, rig.transport.posts)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 73, model.LogTypeConsume).Count(&consumeLogs).Error)
	assert.Equal(t, int64(1), consumeLogs)
}

func TestVerificationEngineRestartResumesGETAndUsesFrozenPrice(t *testing.T) {
	rig := newVerificationEngineRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rig.transport.pollError, rig.transport.onPoll = errors.New("temporary provider poll transport failure"), cancel
	require.NoError(t, rig.engine.Execute(ctx, rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
	saved := rig.savedItem(t)
	assert.Equal(t, "RUNTIME_AMBIGUOUS", saved.Result)
	assert.Equal(t, "POLL_INTERRUPTED", saved.ReasonCode)
	assert.Equal(t, "engine-task", saved.TaskID)
	var evidence model.RuntimeVerificationEvidence
	require.NoError(t, common.UnmarshalJsonStr(saved.EvidenceJSON, &evidence))
	assert.Equal(t, 1, evidence.PollAttempts)
	assert.Empty(t, evidence.PollTraceIDs, "a transport failure has no response trace")
	assert.Equal(t, 92500, getUserQuota(t, 73))
	rig.reopen(t)
	config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting).PluginBillingExpr = map[string]string{"dflop-tts::voice-tts-pro": `u("character_count") * 999`}
	rig.transport.pollError, rig.transport.onPoll = nil, nil
	rig.transport.terminal = strings.Replace(rig.transport.terminal, `"characters":6`, `"characters":4`, 1)
	rig.transport.ledger = strings.ReplaceAll(strings.Replace(rig.transport.ledger, `"cost":"6"`, `"cost":"4"`, 1), `"unit_count":6`, `"unit_count":4`)
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	saved = rig.savedItem(t)
	assert.Equal(t, "RUNTIME_VERIFIED", saved.Result, saved.ReasonCode)
	assert.Equal(t, "4", saved.ProviderCostPoints)
	require.NoError(t, common.UnmarshalJsonStr(saved.EvidenceJSON, &evidence))
	assert.Equal(t, 2, evidence.PollAttempts, "the first failed GET remains counted after reopening the database")
	assert.Equal(t, []string{"engine-poll-trace"}, evidence.PollTraceIDs)
	assert.Equal(t, "engine-trace", saved.TraceID)
	assert.Equal(t, 95000, getUserQuota(t, 73), "only the authoritative four characters use the frozen tariff")
	assert.Equal(t, 1, rig.transport.posts)
	used, requests := getUserUsageAccounting(t, 73)
	assert.Equal(t, 5000, used)
	assert.Equal(t, 1, requests)
}

func TestVerificationEngineTerminalFailureRefundsOnce(t *testing.T) {
	rig := newVerificationEngineRig(t)
	rig.transport.terminal = `{"id":"engine-task","model":"voice-tts-pro","status":"failed","error":{"message":"synthetic rejection"}}`
	rig.transport.ledger = `{"currency":"points","scope":"key","logs":[{"model":"voice-tts-pro","task_id":"engine-task","cost":"0","status":"error"}]}`
	require.NoError(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
	rig.reopen(t)
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	saved := rig.savedItem(t)
	assert.Equal(t, "RUNTIME_FAILED", saved.Result)
	assert.Equal(t, "FAIL", saved.GenerationStatus)
	assert.Equal(t, "PASS", saved.BillingStatus)
	assert.Equal(t, 100000, getUserQuota(t, 73))
	journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "BILLING_RESERVATION_REFUNDED", journal.Content)
	assert.Equal(t, 1, rig.transport.posts)
	used, requests := getUserUsageAccounting(t, 73)
	assert.Zero(t, used)
	assert.Zero(t, requests)
}

func TestVerificationEngineIncompleteTerminalOrLedgerRetainsHold(t *testing.T) {
	for _, tc := range []struct {
		name, terminal, ledger, reason string
		ledgerStatus                   int
	}{
		{name: "missing authoritative usage", terminal: `{"id":"engine-task","model":"voice-tts-pro","status":"succeeded","audio_url":"https://example.invalid/synthetic.wav"}`, reason: "BILLING_QUANTITY_MISSING"},
		{name: "missing output", terminal: `{"id":"engine-task","model":"voice-tts-pro","status":"succeeded","characters":6}`, reason: "GENERATION_INVALID_OUTPUT"},
		{name: "ledger unavailable", ledgerStatus: 500, reason: "LEDGER_TEMPORARILY_UNAVAILABLE"},
		{name: "wrong ledger currency", ledger: `{"currency":"USD","scope":"key","logs":[]}`, reason: "LEDGER_TEMPORARILY_UNAVAILABLE"},
		{name: "different task cannot prove charge", ledger: `{"currency":"points","scope":"key","logs":[{"model":"voice-tts-pro","task_id":"another-task","cost":"6","status":"succeeded","unit_type":"audio","unit_count":6}]}`, reason: "INSUFFICIENT_EVIDENCE"},
		{name: "nonterminal ledger status", ledger: `{"currency":"points","scope":"key","logs":[{"model":"voice-tts-pro","task_id":"engine-task","cost":"6","status":"pending","unit_type":"audio","unit_count":6}]}`, reason: "INSUFFICIENT_EVIDENCE"},
		{name: "duplicate exact paid ledger rows", ledger: `{"currency":"points","scope":"key","logs":[{"id":"engine-ledger-1","model":"voice-tts-pro","task_id":"engine-task","request_id":"engine-request","x_gateway_trace":"engine-trace","cost":"6","status":"succeeded","unit_type":"audio","unit_count":6},{"id":"engine-ledger-2","model":"voice-tts-pro","task_id":"engine-task","request_id":"engine-request","x_gateway_trace":"engine-trace","cost":"6","status":"succeeded","unit_type":"audio","unit_count":6}]}`, reason: "LEDGER_MISMATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newVerificationEngineRig(t)
			if tc.terminal != "" {
				rig.transport.terminal = tc.terminal
			}
			if tc.ledger != "" {
				rig.transport.ledger = tc.ledger
			}
			if tc.ledgerStatus != 0 {
				rig.transport.ledgerStatus = tc.ledgerStatus
			}
			require.NoError(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
			saved := rig.savedItem(t)
			assert.NotEqual(t, "RUNTIME_VERIFIED", saved.Result)
			assert.Equal(t, tc.reason, saved.ReasonCode)
			assert.Empty(t, saved.ProviderCostPoints)
			assert.Nil(t, saved.WalletDelta)
			assert.Zero(t, saved.VerifiedAt)
			assert.Equal(t, 92500, getUserQuota(t, 73))
			journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
			require.NoError(t, err)
			require.NotNil(t, journal)
			assert.Equal(t, "BILLING_RESERVATION_HELD", journal.Content)
			run, err := model.GetRuntimeVerificationRun(rig.run.ID)
			require.NoError(t, err)
			assert.Equal(t, "6", run.ReservedProviderCost)
			assert.Equal(t, "0", run.ActualProviderCost)
			assert.Equal(t, 1, rig.transport.posts)
		})
	}
}

func TestVerificationEngineUnknownAcceptanceNeverResubmits(t *testing.T) {
	for _, timeout := range []bool{true, false} {
		name := "HTTP 500"
		if timeout {
			name = "transport timeout"
		}
		t.Run(name, func(t *testing.T) {
			rig := newVerificationEngineRig(t)
			rig.transport.ledger = `{"currency":"points","scope":"key","logs":[]}`
			if timeout {
				rig.transport.postError = context.DeadlineExceeded
			} else {
				rig.transport.postStatus = 500
			}
			require.NoError(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
			rig.reopen(t)
			require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
			require.ErrorContains(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture), "SUBMIT_ALREADY_CLAIMED")
			saved := rig.savedItem(t)
			assert.Equal(t, "AMBIGUOUS", saved.RequestStatus)
			assert.Equal(t, "RUNTIME_AMBIGUOUS", saved.Result)
			assert.Empty(t, saved.ProviderCostPoints)
			assert.Equal(t, 92500, getUserQuota(t, 73))
			assert.Equal(t, 1, rig.transport.posts)
			journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
			require.NoError(t, err)
			require.NotNil(t, journal)
			assert.Equal(t, "BILLING_RESERVATION_PENDING", journal.Content)
		})
	}
}

func TestVerificationEngineAuthorizationAndSourceAuthBlockPOST(t *testing.T) {
	for _, sourceAuth := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing paid approval", true: "source authentication rejected"}[sourceAuth], func(t *testing.T) {
			rig := newVerificationEngineRig(t)
			authorization := rig.authorization
			if sourceAuth {
				rig.transport.catalogStatus = 401
			} else {
				authorization.Approved = false
			}
			require.Error(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, authorization, rig.fixture))
			assert.Zero(t, rig.transport.posts)
			assert.Equal(t, 100000, getUserQuota(t, 73))
			assert.Empty(t, rig.savedItem(t).RequestBodyHash)
			run, err := model.GetRuntimeVerificationRun(rig.run.ID)
			require.NoError(t, err)
			assert.Zero(t, run.PaidRequests)
		})
	}
}

func TestVerificationEngineMissingTaskCannotPromoteLedgerSuccess(t *testing.T) {
	for _, omitIDs := range []bool{false, true} {
		name := "submit request and trace cannot prove output"
		if omitIDs {
			name = "all submit correlation identifiers missing"
		}
		t.Run(name, func(t *testing.T) {
			rig := newVerificationEngineRig(t)
			rig.transport.postBody = `{"model":"voice-tts-pro","status":"pending"}`
			rig.transport.omitSubmitIDs = omitIDs
			rig.transport.ledger = `{"currency":"points","scope":"key","logs":[{"model":"voice-tts-pro","request_id":"engine-request","x_gateway_trace":"engine-trace","cost":"6","status":"succeeded","unit_type":"audio","unit_count":6}]}`
			require.NoError(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
			rig.reopen(t)
			require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
			saved := rig.savedItem(t)
			assert.Empty(t, saved.TaskID)
			assert.Equal(t, "RUNTIME_AMBIGUOUS", saved.Result)
			assert.Equal(t, "AMBIGUOUS", saved.RequestStatus)
			assert.NotEqual(t, "PASS", saved.GenerationStatus)
			assert.NotEqual(t, "PASS", saved.LedgerStatus)
			assert.Empty(t, saved.ProviderCostPoints)
			assert.Nil(t, saved.WalletDelta)
			assert.Zero(t, saved.VerifiedAt)
			assert.Equal(t, 92500, getUserQuota(t, 73))
			assert.Equal(t, 1, rig.transport.posts)
			assert.Zero(t, rig.transport.polls)
			if omitIDs {
				assert.Empty(t, saved.RequestID)
				assert.Empty(t, saved.TraceID)
				assert.Zero(t, rig.transport.ledgerReads, "no identifier means no ledger lookup")
			} else {
				assert.Equal(t, "engine-request", saved.RequestID)
				assert.Equal(t, "engine-trace", saved.TraceID)
				assert.Greater(t, rig.transport.ledgerReads, 0)
			}
			journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
			require.NoError(t, err)
			require.NotNil(t, journal)
			assert.Equal(t, "BILLING_RESERVATION_PENDING", journal.Content)
		})
	}
}

func TestVerificationEngineReservationJournalFailureBlocksOutbound(t *testing.T) {
	for _, afterFunds := range []bool{false, true} {
		name := "claim receipt creation failed"
		if afterFunds {
			name = "hold receipt write failed after debit"
		}
		t.Run(name, func(t *testing.T) {
			rig := newVerificationEngineRig(t)
			injected := false
			if afterFunds {
				require.NoError(t, model.LOG_DB.Callback().Update().Before("gorm:update").Register("verification_hold_journal_failure", func(tx *gorm.DB) {
					updates, ok := tx.Statement.Dest.(map[string]any)
					if ok && tx.Statement.Table == "logs" && updates["content"] == "BILLING_RESERVATION_HELD" {
						injected = true
						tx.AddError(errors.New("injected hold journal write failure"))
					}
				}))
			} else {
				require.NoError(t, model.LOG_DB.Callback().Create().Before("gorm:create").Register("verification_claim_journal_failure", func(tx *gorm.DB) {
					log, ok := tx.Statement.Dest.(*model.Log)
					if ok && log.Content == "BILLING_RESERVATION_CLAIMED" {
						injected = true
						tx.AddError(errors.New("injected claim journal write failure"))
					}
				}))
			}
			require.ErrorContains(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture), "RESERVATION_JOURNAL_FAILED")
			require.True(t, injected)
			saved := rig.savedItem(t)
			assert.Equal(t, "AMBIGUOUS", saved.RequestStatus)
			assert.Equal(t, "RUNTIME_AMBIGUOUS", saved.Result)
			assert.Empty(t, saved.TaskID)
			assert.Zero(t, saved.VerifiedAt)
			assert.Zero(t, rig.transport.posts)
			wallet := 100000
			journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
			require.NoError(t, err)
			if afterFunds {
				wallet = 92500
				require.NotNil(t, journal)
				assert.Equal(t, "BILLING_RESERVATION_CLAIMED", journal.Content)
			} else {
				assert.Nil(t, journal)
			}
			assert.Equal(t, wallet, getUserQuota(t, 73))
			rig.reopen(t)
			require.ErrorIs(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture), gorm.ErrRecordNotFound)
			require.ErrorContains(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture), "SUBMIT_ALREADY_CLAIMED")
			assert.Equal(t, wallet, getUserQuota(t, 73))
			assert.Zero(t, rig.transport.posts)
			assert.Zero(t, rig.savedItem(t).VerifiedAt)
		})
	}
}

func TestVerificationSchedulerSkipsOwnedTaskAndTimeoutRefund(t *testing.T) {
	rig := newVerificationEngineRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rig.transport.pollError, rig.transport.onPoll = context.Canceled, cancel
	require.NoError(t, rig.engine.Execute(ctx, rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
	saved := rig.savedItem(t)
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", saved.LocalTaskID).Update("submit_time", time.Now().Add(-2*time.Hour).Unix()).Error)
	priorAdaptor, priorTimeout := GetTaskAdaptorFunc, constant.TaskTimeoutMinutes
	t.Cleanup(func() { GetTaskAdaptorFunc, constant.TaskTimeoutMinutes = priorAdaptor, priorTimeout })
	calls := 0
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { calls++; return nil }
	constant.TaskTimeoutMinutes = 1
	summary := RunTaskPollingOnce(t.Context(), nil)
	assert.Zero(t, calls)
	assert.Zero(t, summary.PlatformsScanned)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", saved.LocalTaskID).First(&task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), task.Status)
	assert.Equal(t, 92500, getUserQuota(t, 73))
	journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "BILLING_RESERVATION_HELD", journal.Content)
}

func TestVerificationEngineRestartRecoversAcknowledgementAfterItemWriteFailure(t *testing.T) {
	rig := newVerificationEngineRig(t)
	injected := false
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("verification_acknowledgement_failure", func(tx *gorm.DB) {
		item, ok := tx.Statement.Dest.(*model.RuntimeVerificationItem)
		if ok && item.TaskID == "engine-task" && !injected {
			injected = true
			tx.AddError(errors.New("injected acknowledgement item write failure"))
		}
	}))
	require.ErrorContains(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture), "injected acknowledgement")
	require.True(t, injected)
	saved := rig.savedItem(t)
	assert.Empty(t, saved.TaskID)
	assert.Equal(t, "AMBIGUOUS", saved.RequestStatus)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", saved.LocalTaskID).First(&task).Error)
	assert.Equal(t, "engine-task", task.PrivateData.UpstreamTaskID)
	assert.NotEmpty(t, task.PrivateData.PluginState)
	assert.Equal(t, 92500, getUserQuota(t, 73))
	rig.reopen(t)
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	saved = rig.savedItem(t)
	assert.Equal(t, "RUNTIME_VERIFIED", saved.Result)
	assert.Equal(t, "engine-task", saved.TaskID)
	assert.Equal(t, 92500, getUserQuota(t, 73))
	assert.Equal(t, 1, rig.transport.posts)
}

func TestVerificationEnginePartialWalletFinalizationRequiresManualRecovery(t *testing.T) {
	for _, afterFunds := range []bool{false, true} {
		name := "wallet adjustment failed"
		if afterFunds {
			name = "funds adjusted but quota persistence failed"
		}
		t.Run(name, func(t *testing.T) {
			rig := newVerificationEngineRig(t)
			rig.transport.terminal = strings.Replace(rig.transport.terminal, `"characters":6`, `"characters":4`, 1)
			rig.transport.ledger = strings.ReplaceAll(strings.Replace(rig.transport.ledger, `"cost":"6"`, `"cost":"4"`, 1), `"unit_count":6`, `"unit_count":4`)
			faultActive, injected := false, false
			rig.transport.onPost = func() { faultActive = true }
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("verification_finalization_failure", func(tx *gorm.DB) {
				if !faultActive || injected {
					return
				}
				changes, ok := tx.Statement.Dest.(map[string]any)
				if !ok || len(changes) != 1 || changes["quota"] == nil {
					return
				}
				if !afterFunds && tx.Statement.Table == "users" || afterFunds && tx.Statement.Table == "tasks" {
					injected = true
					tx.AddError(errors.New("injected wallet finalization failure"))
				}
			}))
			require.NoError(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture))
			require.True(t, injected)
			saved := rig.savedItem(t)
			assert.Equal(t, "RUNTIME_AMBIGUOUS", saved.Result)
			assert.Equal(t, "BLOCKED", saved.BillingStatus)
			assert.Equal(t, "BILLING_JOURNAL_FINALIZATION_FAILED", saved.ReasonCode)
			assert.Nil(t, saved.WalletDelta)
			journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
			require.NoError(t, err)
			require.NotNil(t, journal)
			assert.Equal(t, "BILLING_RESERVATION_SETTLING", journal.Content)
			wallet := 92500
			if afterFunds {
				wallet = 95000
			}
			assert.Equal(t, wallet, getUserQuota(t, 73))
			rig.reopen(t)
			require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
			assert.Equal(t, wallet, getUserQuota(t, 73), "restart must not repeat an uncertain wallet delta")
			assert.Equal(t, 1, rig.transport.posts)
			used, requests := getUserUsageAccounting(t, 73)
			assert.Zero(t, used)
			assert.Zero(t, requests)
			assert.Zero(t, rig.savedItem(t).VerifiedAt)
		})
	}
}

func TestVerificationEngineMissingConsumeLogCannotVerifySettlement(t *testing.T) {
	rig := newVerificationEngineRig(t)
	injected := false
	require.NoError(t, model.LOG_DB.Callback().Create().Before("gorm:create").Register("verification_consume_log_failure", func(tx *gorm.DB) {
		log, ok := tx.Statement.Dest.(*model.Log)
		if ok && log.Type == model.LogTypeConsume {
			injected = true
			tx.AddError(errors.New("injected consume log failure"))
		}
	}))
	_ = rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture)
	require.True(t, injected)
	saved := rig.savedItem(t)
	assert.NotEqual(t, "RUNTIME_VERIFIED", saved.Result, "a journal tombstone alone cannot prove the missing consume log")
	assert.Zero(t, saved.VerifiedAt)
	var consumeLogs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 73, model.LogTypeConsume).Count(&consumeLogs).Error)
	assert.Zero(t, consumeLogs)
	assert.Equal(t, 92500, getUserQuota(t, 73))
	rig.reopen(t)
	_ = rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture)
	assert.Equal(t, 92500, getUserQuota(t, 73), "a missing accounting receipt cannot authorize another debit or refund")
	assert.NotEqual(t, "RUNTIME_VERIFIED", rig.savedItem(t).Result)
	assert.Equal(t, 1, rig.transport.posts)
}

func TestVerificationEngineFinalProofWriteFailureResumesWithoutDebit(t *testing.T) {
	rig := newVerificationEngineRig(t)
	injected := false
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("verification_final_proof_failure", func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]any)
		if ok && tx.Statement.Table == "runtime_verification_items" && updates["result"] == "RUNTIME_VERIFIED" {
			injected = true
			tx.AddError(errors.New("injected final proof write failure"))
		}
	}))
	require.ErrorContains(t, rig.engine.Execute(t.Context(), rig.run.ID, rig.item.ID, rig.authorization, rig.fixture), "injected final proof")
	require.True(t, injected)
	saved := rig.savedItem(t)
	assert.NotEqual(t, "RUNTIME_VERIFIED", saved.Result)
	assert.Zero(t, saved.VerifiedAt)
	assert.Positive(t, saved.ProviderCostSettledAt)
	journal, err := model.FindBillingReservationLog(saved.LocalRequestID, 73)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "BILLING_RESERVATION_SETTLED", journal.Content)
	assert.Equal(t, 92500, getUserQuota(t, 73))
	polls := rig.transport.polls
	rig.reopen(t)
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	require.NoError(t, rig.engine.Resume(t.Context(), rig.run.ID, rig.item.ID, rig.fixture))
	saved = rig.savedItem(t)
	assert.Equal(t, "RUNTIME_VERIFIED", saved.Result, saved.ReasonCode)
	assert.Positive(t, saved.VerifiedAt)
	assert.Equal(t, 92500, getUserQuota(t, 73))
	assert.Equal(t, 1, rig.transport.posts)
	assert.Equal(t, polls, rig.transport.polls, "durable terminal facts permit ledger-only recovery")
	used, requests := getUserUsageAccounting(t, 73)
	assert.Equal(t, 7500, used)
	assert.Equal(t, 1, requests)
	var consumeLogs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 73, model.LogTypeConsume).Count(&consumeLogs).Error)
	assert.EqualValues(t, 1, consumeLogs)
}

func TestVerificationProviderPointsSelectExactFamilyRateAndQuantity(t *testing.T) {
	for _, test := range []struct {
		model, endpoint, rateKey, expected string
		features                           []string
		facts                              map[string]any
	}{
		{"suno-v5", "music_generations", "price_per_music_generation", "2", []string{"music"}, map[string]any{"generation_count": 1}},
		{"voice-clone-pro", "voice_clone", "price_per_voice_clone", "2", []string{"voice_clone"}, map[string]any{"count": 1}},
		{"dh-avatar-create", "avatar_create", "price_per_avatar", "2", []string{"avatar"}, map[string]any{"count": 1}},
		{"clip-compose", "videos_generations", "price_per_video_task", "2", []string{"video_task"}, map[string]any{"count": 1}},
		{"dh-avatar", "videos_generations", "price_per_video_second", "6", []string{"video_second"}, map[string]any{"duration_sec": 3}},
		{"dh-lipsync-pro", "videos_generations", "price_per_video_second", "6", []string{"video_second"}, map[string]any{"duration_sec": 3}},
		{"dh-motion", "videos_generations", "video_tier:standard", "6", []string{"video_second", "video_tiers"}, map[string]any{"duration_sec": 3, "resolution": "standard"}},
		{"wan3.0-video", "videos_generations", "video_tier:480p", "10", []string{"video_second", "video_tiers", "video_input_seconds"}, map[string]any{"duration_sec": 2, "input_video_duration_sec": 3, "resolution": "480p"}},
		{"doubao-seedance-2.5", "videos_generations", "video_token_tier:default", "0.000246", []string{"video_token"}, map[string]any{"completion_tokens": 123, "resolution": "480p", "input_mode": "default"}},
	} {
		t.Run(test.model, func(t *testing.T) {
			item := dflop.Item{ModelID: test.model, EndpointType: test.endpoint, BillingFeatures: test.features, PriceSemantics: dflop.PriceSemantics{SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"}, Prices: map[string]dflop.Price{test.rateKey: {EffectiveCredits: "2", SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"}}}
			points, err := DFLOPVerificationPoints(item, test.facts)
			require.NoError(t, err)
			assert.Equal(t, test.expected, points.String())
			price := item.Prices[test.rateKey]
			price.SourcePriceKind = "DOCUMENTED_CONTRACT_OVERRIDE"
			item.Prices[test.rateKey] = price
			_, err = DFLOPVerificationPoints(item, test.facts)
			require.ErrorContains(t, err, "PROVIDER_DOCUMENTATION_CONFLICT")
		})
	}
}
