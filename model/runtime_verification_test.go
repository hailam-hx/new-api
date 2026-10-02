package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func runtimeVerificationDatabase(t *testing.T) string {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	path := filepath.Join(t.TempDir(), "verification.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		current, err := DB.DB()
		if err == nil {
			_ = current.Close()
		}
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(mainType, logType)
		initCol()
	})
	require.NoError(t, DB.AutoMigrate(&RuntimeVerificationRun{}, &RuntimeVerificationItem{}))
	return path
}

func runtimeVerificationRunFixture(t *testing.T, budget string, requests int) (*RuntimeVerificationRun, []RuntimeVerificationItem) {
	t.Helper()
	expression := `tier("image", u("image_count") * 0.1)`
	snapshot, err := common.Marshal(billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ModelName: "image-a", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, EstimatedTier: "default@720p", UsageFacts: map[string]any{"image_count": 1}})
	require.NoError(t, err)
	var runCount int64
	require.NoError(t, DB.Model(&RuntimeVerificationRun{}).Count(&runCount).Error)
	run := &RuntimeVerificationRun{ChannelID: 1, FundingUserID: 7, Source: "dflop", CatalogHash: strings.Repeat("a", 64), CredentialFingerprint: strings.Repeat("b", 64), Mode: "paid", AuthorizationManifestHash: billingexpr.ExprHashString(t.Name() + decimal.NewFromInt(runCount).String()), TotalBudgetLimit: budget, MaxRequests: requests}
	items := []RuntimeVerificationItem{
		{Model: "image-a", Protocol: "openai_image", Operation: "generate", Mode: "default", FixtureID: "image-v1", Endpoint: "/v1/images/generations", PricingSnapshotHash: strings.Repeat("d", 64), BillingExprHash: billingexpr.ExprHashString(expression), BillingSnapshotJSON: string(snapshot)},
		{Model: "image-b", Protocol: "openai_image", Operation: "generate", Mode: "default", FixtureID: "image-v1", Endpoint: "/v1/images/generations", PricingSnapshotHash: strings.Repeat("d", 64), BillingExprHash: billingexpr.ExprHashString(expression), BillingSnapshotJSON: string(snapshot)},
		{Model: "image-c", Protocol: "openai_image", Operation: "generate", Mode: "default", FixtureID: "image-v1", Endpoint: "/v1/images/generations", PricingSnapshotHash: strings.Repeat("d", 64), BillingExprHash: billingexpr.ExprHashString(expression), BillingSnapshotJSON: string(snapshot)},
	}
	for i := range items {
		provider, err := common.Marshal(RuntimeVerificationProviderSnapshot{Model: items[i].Model, EndpointType: "image", Features: []string{"image_count"}, Rates: map[string]string{"image_count": "0.1", "default@720p": "0.2"}, RateProvenance: map[string]string{"image_count": "AUTHENTICATED_EFFECTIVE_PRICE", "default@720p": "DFLOP_DOCUMENTED_CONTRACT_OVERRIDE"}, ContractOverrides: []RuntimeVerificationContractOverride{{Provider: "dflop", Model: items[i].Model, Feature: "second_stage_upscale_rate", Source: "official_dflop_contract", SourceReferenceHash: strings.Repeat("f", 64), Value: "720p=2.5 points/s;1080p=5 points/s;delivered output only"}}, PointsPerCNY: "100", CNYToUSD: "0.15", Markup: "1", ConfigHash: strings.Repeat("a", 64), PluginHash: strings.Repeat("b", 64), FixtureHash: strings.Repeat("c", 64)})
		require.NoError(t, err)
		items[i].FrozenProviderJSON = string(provider)
		items[i].ConfigStatus, items[i].ConnectivityStatus = "PASS", "PASS"
	}
	require.NoError(t, CreateRuntimeVerificationRun(run, items))
	items, err = ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	return run, items
}

func TestRuntimeVerificationBudgetIntentSurvivesRestart(t *testing.T) {
	path := runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "0.3", 2)
	bodyHash, keyHash := strings.Repeat("e", 64), strings.Repeat("f", 64)
	require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, bodyHash, keyHash, "0.3"))
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, items[1].ID, bodyHash, keyHash, "0.2"))
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, items[2].ID, bodyHash, keyHash, "0.000000000000000001"))
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, DB.AutoMigrate(&RuntimeVerificationRun{}, &RuntimeVerificationItem{}))
	stored, savedItems, err := LatestRuntimeVerification(1)
	require.NoError(t, err)
	assert.Equal(t, "0.3", stored.ReservedProviderCost)
	assert.Equal(t, 1, stored.PaidRequests)
	assert.Equal(t, "AMBIGUOUS", savedItems[0].RequestStatus)
	assert.Equal(t, "RUNTIME_AMBIGUOUS", savedItems[0].Result)
	assert.Equal(t, "SUBMIT_INTENT_CLAIMED", savedItems[0].ReasonCode)
	assert.Equal(t, bodyHash, savedItems[0].RequestBodyHash)
	assert.Equal(t, keyHash, savedItems[0].IdempotencyKeyHash)
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, bodyHash, keyHash, "0.1"), "replaying the durable intent must not authorize another POST")
	savedItems[0].RequestStatus = "AMBIGUOUS"
	savedItems[0].ReasonCode = "RUNTIME_AMBIGUOUS"
	require.NoError(t, UpdateRuntimeVerificationItem(&savedItems[0]))
	require.Error(t, SettleRuntimeVerificationProviderCost(run.ID, items[0].ID, "0"))
	stored, err = GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "0.3", stored.ReservedProviderCost, "unknown provider outcome retains its hold")
	stale := items[0]
	stale.LocalTaskID, stale.ProviderCostPoints = "local-task", "0.20"
	require.NoError(t, UpdateRuntimeVerificationItem(&stale))
	storedItems, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "AMBIGUOUS", storedItems[0].RequestStatus)
	assert.Equal(t, "RUNTIME_AMBIGUOUS", storedItems[0].Result)
	assert.Equal(t, "0.2", storedItems[0].ProviderCostPoints, "known ledger cost remains visible while its hold is unresolved")
}

func TestRuntimeVerificationSettlementIsExactAndIdempotent(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "0.3", 1)
	require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
	saved, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	item := &saved[0]
	item.LedgerStatus, item.CorrelationQuality, item.TraceID = "PASS", "NONE", "trace-1"
	item.TerminalStatus = "succeeded"
	require.NoError(t, UpdateRuntimeVerificationItem(item))
	require.Error(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.1"))
	item.CorrelationQuality = "EXACT_TRACE_ID"
	require.NoError(t, UpdateRuntimeVerificationItem(item))
	require.NoError(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.1"))
	require.NoError(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.10"))
	require.Error(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.2"))
	stored, err := GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "0.1", stored.ActualProviderCost)
	assert.Equal(t, "0", stored.ReservedProviderCost)
	assert.Equal(t, 1, stored.PaidRequests)
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, items[1].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.1"), "releasing a hold cannot roll over the request allowance")
}

func TestRuntimeVerificationConcurrentClaimOnlyPermitsOneSubmit(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 2)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			results <- ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.2")
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)
	stored, err := GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, stored.PaidRequests)
	assert.Equal(t, "0.2", stored.ReservedProviderCost)
}

func TestRuntimeVerificationRejectsSecretsAndUnprovenPromotion(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	item := items[0]
	for _, raw := range []string{`{"Authorization":"Bearer secret"}`, `{"output_url":"https://private.example/result?token=secret"}`, `{"nested":{"prompt":"secret prompt"}}`, `{"journal_state":"Bearer secret"}`} {
		item.EvidenceJSON = raw
		require.Error(t, UpdateRuntimeVerificationItem(&item))
	}
	item.EvidenceJSON = `{"output_valid":true,"journal_state":"RESERVED","poll_trace_ids":["poll-trace-1","poll-trace-2"],"ledger_trace_id":"ledger-trace","catalog_trace_id":"catalog-trace","currency_trace_id":"currency-trace","ledger_facts":{"cost":"0.10","unit_count":"1.0"},"unknown_field":{"count":2}}`
	item.NormalizedUsageJSON = `{"image_count":1,"input_image_count":0,"unknown_field":123}`
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	stored, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.NotContains(t, stored[0].EvidenceJSON, "unknown_field")
	assert.Contains(t, stored[0].EvidenceJSON, `"poll_trace_ids":["poll-trace-1","poll-trace-2"]`)
	assert.Contains(t, stored[0].EvidenceJSON, `"ledger_trace_id":"ledger-trace"`)
	assert.Contains(t, stored[0].EvidenceJSON, `"catalog_trace_id":"catalog-trace"`)
	assert.Contains(t, stored[0].EvidenceJSON, `"currency_trace_id":"currency-trace"`)
	assert.Contains(t, stored[0].EvidenceJSON, `"ledger_facts":{"cost":"0.1","unit_count":"1"}`)
	assert.JSONEq(t, `{"image_count":1,"input_image_count":0}`, stored[0].NormalizedUsageJSON)
	item.NormalizedUsageJSON = `{"image_count":1,"prompt":"do not store"}`
	require.Error(t, UpdateRuntimeVerificationItem(&item))
	item.NormalizedUsageJSON = `{"image_count":1}`
	item.Result = "RUNTIME_VERIFIED"
	require.Error(t, UpdateRuntimeVerificationItem(&item))
	require.Error(t, FinalizeRuntimeVerificationItem(run.ID, item.ID))
	item.Result, item.EvidenceJSON = "RUNTIME_PARTIAL", `{"output_valid":true,"replay_idempotent":true,"journal_state":"SETTLED"}`
	item.ConfigStatus, item.ConnectivityStatus, item.RequestStatus, item.GenerationStatus, item.ParserStatus, item.BillingStatus, item.LedgerStatus = "PASS", "PASS", "PASS", "PASS", "PASS", "PASS", "PASS"
	item.TerminalStatus, item.CorrelationQuality, item.TraceID = "succeeded", "EXACT_TRACE_ID", "trace-1"
	item.ProviderUsageJSON = `{"image_count":1}`
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.Error(t, FinalizeRuntimeVerificationItem(run.ID, item.ID), "a fabricated pass has no durable paid intent or settlement")
	require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, item.ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.2"))
	item.RequestStatus = "PASS"
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.NoError(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.1"))
	require.NoError(t, FinalizeRuntimeVerificationItem(run.ID, item.ID))
	stored, err = ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "RUNTIME_VERIFIED", stored[0].Result)
	assert.Positive(t, stored[0].VerifiedAt)
}

func TestRuntimeVerificationIdentityAndAtomicRollback(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	duplicate := items[0]
	duplicate.ID = 0
	require.Error(t, DB.Create(&duplicate).Error)
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("verification_fail_item", func(tx *gorm.DB) {
		if tx.Statement.Table == "runtime_verification_items" {
			tx.AddError(assert.AnError)
		}
	}))
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.2"))
	require.NoError(t, DB.Callback().Update().Remove("verification_fail_item"))
	stored, err := GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "0", stored.ReservedProviderCost)
	assert.Zero(t, stored.PaidRequests)
	missing, missingItems, err := LatestRuntimeVerification(999)
	require.NoError(t, err)
	assert.Nil(t, missing)
	assert.Nil(t, missingItems)
}

func TestRuntimeVerificationRejectsInvalidUsageAndUnsafeTypedMetadata(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	for _, raw := range []string{`{"image_count":-1}`, `{"unit_count":"-1"}`, `{"status":"raw prompt with spaces"}`} {
		item := items[0]
		item.NormalizedUsageJSON = raw
		require.Error(t, UpdateRuntimeVerificationItem(&item), raw)
	}
	for _, raw := range []string{`{"open_reasons":{"generation":"raw prompt with spaces"}}`, `{"layers":{"generation":"not a status"}}`, `{"output_count":-1}`, `{"poll_trace_ids":["raw trace with spaces"]}`, `{"ledger_trace_id":"raw trace with spaces"}`, `{"catalog_trace_id":"raw trace with spaces"}`, `{"currency_trace_id":"raw trace with spaces"}`, `{"ledger_facts":{"cost":"-1"}}`, `{"ledger_facts":{"attempts":"1"}}`} {
		item := items[0]
		item.EvidenceJSON = raw
		require.Error(t, UpdateRuntimeVerificationItem(&item), raw)
	}
	tooManyTraces := make([]string, 602)
	for i := range tooManyTraces {
		tooManyTraces[i] = "poll-trace"
	}
	tracesJSON, err := common.Marshal(map[string]any{"poll_trace_ids": tooManyTraces})
	require.NoError(t, err)
	excessive := items[0]
	excessive.EvidenceJSON = string(tracesJSON)
	require.Error(t, UpdateRuntimeVerificationItem(&excessive))
	item := items[0]
	item.BillingSnapshotJSON = strings.ReplaceAll(item.BillingSnapshotJSON, `u(\"image_count\")`, `param(\"prompt\")`)
	require.Error(t, UpdateRuntimeVerificationItem(&item), "a changed expression cannot retain its old hash")
	for _, mutate := range []func(*billingexpr.BillingSnapshot){
		func(s *billingexpr.BillingSnapshot) { s.ModelName = "raw prompt with spaces" },
		func(s *billingexpr.BillingSnapshot) { s.EstimatedTier = "raw prompt with spaces" },
		func(s *billingexpr.BillingSnapshot) { negative := -1; s.EstimatedImageCount = &negative },
		func(s *billingexpr.BillingSnapshot) { s.PreConsumeMultiplier = -1 },
		func(s *billingexpr.BillingSnapshot) { negative := -1.0; s.EstimatedFixedPrice = &negative },
	} {
		var snapshot billingexpr.BillingSnapshot
		require.NoError(t, common.UnmarshalJsonStr(items[0].BillingSnapshotJSON, &snapshot))
		mutate(&snapshot)
		body, err := common.Marshal(snapshot)
		require.NoError(t, err)
		invalid := items[0]
		invalid.BillingSnapshotJSON = string(body)
		require.Error(t, UpdateRuntimeVerificationItem(&invalid))
	}
	stored, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.Empty(t, stored[0].NormalizedUsageJSON)
}

func TestRuntimeVerificationSettlementFailureRetainsIntentAndHold(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "0.3", 1)
	require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
	item := items[0]
	item.LedgerStatus, item.CorrelationQuality, item.TaskID, item.TerminalStatus = "PASS", "EXACT_TASK_ID", "task-1", "succeeded"
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("verification_fail_settlement", func(tx *gorm.DB) {
		if tx.Statement.Table == "runtime_verification_items" {
			tx.AddError(assert.AnError)
		}
	}))
	require.Error(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.1"))
	require.NoError(t, DB.Callback().Update().Remove("verification_fail_settlement"))
	stored, err := GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "0", stored.ActualProviderCost)
	assert.Equal(t, "0.3", stored.ReservedProviderCost)
	assert.Equal(t, 1, stored.PaidRequests)
	require.NoError(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, "0.1"))
}

func TestRuntimeVerificationFrozenProviderSnapshotSurvivesAndCannotDriftAfterClaim(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	snapshot := RuntimeVerificationProviderSnapshot{Model: items[0].Model, EndpointType: "image", Features: []string{"image_count"}, Rates: map[string]string{"image_count": "0.1"}, PointsPerCNY: "100", CNYToUSD: "0.15", Markup: "1", ConfigHash: strings.Repeat("a", 64), PluginHash: strings.Repeat("b", 64), FixtureHash: strings.Repeat("c", 64)}
	body, err := common.Marshal(snapshot)
	require.NoError(t, err)
	item := items[0]
	item.FrozenProviderJSON = string(body)
	item.LocalRequestID = "local-request-1"
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, item.ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
	item.LocalTaskID = "local-task-1"
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	snapshot.Rates["image_count"] = "0.2"
	body, err = common.Marshal(snapshot)
	require.NoError(t, err)
	item.FrozenProviderJSON = string(body)
	require.Error(t, UpdateRuntimeVerificationItem(&item))
	stored, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.Contains(t, stored[0].FrozenProviderJSON, `"image_count":"0.1"`)
	assert.Equal(t, "local-task-1", stored[0].LocalTaskID)
	assert.Equal(t, "local-request-1", stored[0].LocalRequestID)
	item = stored[0]
	item.LocalTaskID = "different-local-task"
	require.Error(t, UpdateRuntimeVerificationItem(&item), "a durable journal task identity cannot be replaced")
}

func TestRuntimeVerificationClaimRequiresFrozenOfflineReadinessAndFunding(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	item := items[0]
	item.ConfigStatus = "BLOCKED"
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, item.ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
	item.ConfigStatus, item.FrozenProviderJSON = "PASS", ""
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, item.ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
	item.FrozenProviderJSON = items[0].FrozenProviderJSON
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	require.NoError(t, DB.Model(run).Update("funding_user_id", 0).Error)
	require.Error(t, ClaimRuntimeVerificationSubmit(run.ID, item.ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
}

func TestRuntimeVerificationHistoryDoesNotPromoteOldCatalogEvidence(t *testing.T) {
	runtimeVerificationDatabase(t)
	oldRun, oldItems := runtimeVerificationRunFixture(t, "1", 1)
	require.NoError(t, ClaimRuntimeVerificationSubmit(oldRun.ID, oldItems[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
	oldItems[0].RequestID = "provider-request-old"
	oldItems[0].Result = "RUNTIME_AMBIGUOUS"
	require.NoError(t, UpdateRuntimeVerificationItem(&oldItems[0]))
	current := &RuntimeVerificationRun{ChannelID: 1, Source: "dflop", CatalogHash: strings.Repeat("d", 64), CredentialFingerprint: strings.Repeat("b", 64), Mode: "connectivity"}
	require.NoError(t, CreateRuntimeVerificationRun(current, []RuntimeVerificationItem{{Model: "image-a", Protocol: "openai_image", Mode: "default", ConfigStatus: "PASS", ConnectivityStatus: "PASS"}}))
	run, items, err := LatestRuntimeVerificationEvidence(1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, current.ID, run.ID)
	assert.Equal(t, current.ID, items[0].RunID)
	assert.Equal(t, current.CatalogHash, items[0].CatalogHash)
	assert.Equal(t, "CONFIG_READY_RUNTIME_UNTESTED", items[0].Result)
	assert.Empty(t, items[0].RequestID)
	require.NotNil(t, items[0].HistoricalEvidence)
	assert.Equal(t, oldRun.ID, items[0].HistoricalEvidence.RunID)
	assert.Equal(t, oldRun.CatalogHash, items[0].HistoricalEvidence.CatalogHash)
	assert.Equal(t, "provider-request-old", items[0].HistoricalEvidence.RequestID)
	assert.Equal(t, "RUNTIME_AMBIGUOUS", items[0].HistoricalEvidence.Result)
	stored, err := ListRuntimeVerificationItems(current.ID)
	require.NoError(t, err)
	assert.Nil(t, stored[0].HistoricalEvidence, "the composition must never write old evidence to the new item")
}

func TestRuntimeVerificationPluginRecoverySelectorsRemainSanitizedAndDurable(t *testing.T) {
	path := runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	item := items[0]
	item.PluginStateJSON = `{"resolution":"720p","input_mode":"default","_verification_task_id":"provider-task-1","reservation":{"completion_tokens":1200,"duration_sec":3,"resolution":"720p","input_mode":"default"}}`
	require.NoError(t, UpdateRuntimeVerificationItem(&item))
	stored, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.JSONEq(t, item.PluginStateJSON, stored[0].PluginStateJSON)
	assert.Contains(t, stored[0].PluginStateJSON, `"_verification_task_id":"provider-task-1"`)
	assert.Contains(t, stored[0].PluginStateJSON, `"reservation"`)
	item.PluginStateJSON = `{"reservation":{"completion_tokens":1200,"prompt":"secret"}}`
	require.Error(t, UpdateRuntimeVerificationItem(&item))
	runtimeVerificationCanonicalUsageFixture(t, run, &items[1])
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	LOG_DB = DB
	require.NoError(t, DB.AutoMigrate(&RuntimeVerificationRun{}, &RuntimeVerificationItem{}))
	stored, err = ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	assert.JSONEq(t, items[1].NormalizedUsageJSON, stored[1].NormalizedUsageJSON)
	assert.JSONEq(t, items[1].ProviderUsageJSON, stored[1].ProviderUsageJSON)
}

func runtimeVerificationCanonicalUsageFixture(t *testing.T, run *RuntimeVerificationRun, item *RuntimeVerificationItem) {
	t.Helper()
	canonical := `{"image_count":1,"input_image_count":0,"small_image_count":1,"large_image_count":0,"images_up_to_1_5k":1,"images_above_1_5k":0,"input_images":0,"seconds":"3","duration_sec":"3","source_duration_sec":"3","input_video_duration_sec":"1","completion_tokens":1200,"resolution":"720p","input_mode":"default","generation_count":1,"characters":6,"character_count":6,"count":1,"asr_units":1,"translation_units":0,"output_pixel_tier":"small","billable_characters":6,"successful_output_quantity":1,"generated_images":1}`
	item.NormalizedUsageJSON, item.ProviderUsageJSON = canonical, `{"usage":`+canonical+`}`
	require.NoError(t, UpdateRuntimeVerificationItem(item))
	stored, err := ListRuntimeVerificationItems(run.ID)
	require.NoError(t, err)
	for _, saved := range stored {
		if saved.ID == item.ID {
			var frozen RuntimeVerificationProviderSnapshot
			require.NoError(t, common.UnmarshalJsonStr(saved.FrozenProviderJSON, &frozen))
			assert.Equal(t, "DFLOP_DOCUMENTED_CONTRACT_OVERRIDE", frozen.RateProvenance["default@720p"])
			require.Len(t, frozen.ContractOverrides, 1)
			assert.Equal(t, strings.Repeat("f", 64), frozen.ContractOverrides[0].SourceReferenceHash)
			assert.JSONEq(t, canonical, saved.NormalizedUsageJSON)
			assert.JSONEq(t, `{"usage":`+canonical+`}`, saved.ProviderUsageJSON)
			return
		}
	}
	require.FailNow(t, "saved canonical usage item is missing")
}

func TestRuntimeVerificationOutstandingIntentStopsFurtherPaidCalls(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "0.4", 3)
	require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.1"))
	require.ErrorIs(t, ClaimRuntimeVerificationSubmit(run.ID, items[1].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.2"), ErrRuntimeVerificationConflict)
	for i, cost := range []string{"0.1", "0.2", "0.1"} {
		if i > 0 {
			if i == 2 {
				require.ErrorIs(t, ClaimRuntimeVerificationSubmit(run.ID, items[i].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.100000000000000001"), ErrRuntimeVerificationBudget)
			}
			require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[i].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), cost))
		}
		item := items[i]
		item.LedgerStatus, item.CorrelationQuality, item.TaskID, item.TerminalStatus = "PASS", "EXACT_TASK_ID", "provider-task", "succeeded"
		require.NoError(t, UpdateRuntimeVerificationItem(&item))
		require.NoError(t, SettleRuntimeVerificationProviderCost(run.ID, item.ID, cost))
		if i == 0 {
			require.ErrorIs(t, ClaimRuntimeVerificationSubmit(run.ID, items[1].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.2"), ErrRuntimeVerificationConflict, "financial settlement alone cannot bypass unresolved runtime proof")
		}
		item.RequestStatus, item.GenerationStatus, item.ParserStatus, item.BillingStatus = "PASS", "PASS", "PASS", "PASS"
		item.NormalizedUsageJSON, item.ProviderUsageJSON = `{"image_count":1}`, `{"image_count":1}`
		item.EvidenceJSON = `{"output_valid":true,"replay_idempotent":true,"journal_state":"SETTLED"}`
		require.NoError(t, UpdateRuntimeVerificationItem(&item))
		require.NoError(t, FinalizeRuntimeVerificationItem(run.ID, item.ID))
		stored, err := GetRuntimeVerificationRun(run.ID)
		require.NoError(t, err)
		if i < 2 {
			assert.Equal(t, "RUNNING", stored.Status)
			assert.Zero(t, stored.CompletedAt)
		} else {
			assert.Equal(t, "COMPLETED", stored.Status)
			assert.Positive(t, stored.CompletedAt)
		}
	}
	stored, err := GetRuntimeVerificationRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "0.4", stored.ActualProviderCost)
	assert.Equal(t, "0", stored.ReservedProviderCost)
	assert.Equal(t, 3, stored.PaidRequests)
	failedRun, failedItems := runtimeVerificationRunFixture(t, "1", 3)
	require.NoError(t, ClaimRuntimeVerificationSubmit(failedRun.ID, failedItems[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.1"))
	failed := failedItems[0]
	failed.LedgerStatus, failed.CorrelationQuality, failed.TaskID, failed.TerminalStatus, failed.Result = "PASS", "EXACT_TASK_ID", "failed-task", "failed", "RUNTIME_FAILED"
	require.NoError(t, UpdateRuntimeVerificationItem(&failed))
	require.NoError(t, SettleRuntimeVerificationProviderCost(failedRun.ID, failed.ID, "0"))
	require.ErrorIs(t, ClaimRuntimeVerificationSubmit(failedRun.ID, failedItems[1].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.1"), ErrRuntimeVerificationConflict, "a refunded failure must stop further paid calls")
}

func TestRuntimeVerificationAuthorizationCannotRollOverIntoAnotherRun(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, _ := runtimeVerificationRunFixture(t, "1", 1)
	duplicate := *run
	duplicate.ID = 0
	require.Error(t, CreateRuntimeVerificationRun(&duplicate, nil))
	for range 2 {
		zeroCost := &RuntimeVerificationRun{ChannelID: 1, Source: "dflop", CatalogHash: strings.Repeat("a", 64), CredentialFingerprint: strings.Repeat("b", 64), Mode: "connectivity"}
		require.NoError(t, CreateRuntimeVerificationRun(zeroCost, nil))
		assert.Nil(t, zeroCost.AuthorizationIdentityHash)
	}
}

func runtimeVerificationConsumeReceiptFixture(t *testing.T, run *RuntimeVerificationRun, item *RuntimeVerificationItem) {
	t.Helper()
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	item.LocalTaskID, item.LocalRequestID, item.TaskID, item.RequestID = fmt.Sprintf("local-task-receipt-%d", item.ID), fmt.Sprintf("local-request-receipt-%d", item.ID), fmt.Sprintf("provider-task-receipt-%d", item.ID), fmt.Sprintf("provider-request-receipt-%d", item.ID)
	other := NewLogOther()
	other.SetPublic("task_id", item.LocalTaskID)
	other.SetPublic("billing_state", "SETTLED")
	other.SetPublic("settlement_verified", true)
	other.SetRoot("upstream_task_id", "wrong-task")
	wrong := &Log{UserId: run.FundingUserID, ChannelId: run.ChannelID, ModelName: item.Model, Type: LogTypeConsume, Quota: 123, RequestId: "unrelated-log-request", Other: other.JSONString()}
	require.NoError(t, LOG_DB.Create(wrong).Error)
	matched, err := FindRuntimeVerificationConsumeReceipt(run, item, 123)
	require.NoError(t, err)
	assert.False(t, matched, "an equal-cost receipt for a conflicting upstream task is not proof")
	other.SetRoot("upstream_task_id", item.TaskID)
	other.SetRoot("request_id", item.LocalRequestID)
	other.SetRoot("upstream_request_id", item.RequestID)
	exact := *wrong
	exact.Id, exact.Other = 0, other.JSONString()
	wrongScope := exact
	wrongScope.UserId++
	require.NoError(t, LOG_DB.Create(&wrongScope).Error)
	matched, err = FindRuntimeVerificationConsumeReceipt(run, item, 123)
	require.NoError(t, err)
	assert.False(t, matched, "a matching task receipt charged to another user is not proof")
	require.NoError(t, LOG_DB.Create(&exact).Error)
	matched, err = FindRuntimeVerificationConsumeReceipt(run, item, 123)
	require.NoError(t, err)
	assert.True(t, matched, "exact task metadata identifies the receipt independently of generated Log.RequestId")
	matched, err = FindRuntimeVerificationConsumeReceipt(run, item, 124)
	require.NoError(t, err)
	assert.False(t, matched)
	duplicate := exact
	duplicate.Id = 0
	require.NoError(t, LOG_DB.Create(&duplicate).Error)
	matched, err = FindRuntimeVerificationConsumeReceipt(run, item, 123)
	require.ErrorIs(t, err, ErrRuntimeVerificationConflict)
	assert.False(t, matched, "duplicate consume receipts cannot prove a single settlement")
}

func TestRuntimeVerificationConsumeReceiptUsesSeparateLogDatabaseAndExactTaskIdentity(t *testing.T) {
	runtimeVerificationDatabase(t)
	run, items := runtimeVerificationRunFixture(t, "1", 1)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "separate-logs.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	LOG_DB = logDB
	sqlDB, err := logDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	runtimeVerificationConsumeReceiptFixture(t, run, &items[0])
	assert.False(t, DB.Migrator().HasTable(&Log{}), "the receipt must come from the configured log database")
}

func TestRuntimeVerificationDatabaseMatrix(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { DB, LOG_DB = previousDB, previousLogDB; common.SetDatabaseTypes(mainType, logType); initCol() })
	for _, tc := range []struct {
		name, env string
		dialector gorm.Dialector
		dbType    common.DatabaseType
	}{
		{"sqlite", "", sqlite.Open(filepath.Join(t.TempDir(), "matrix.db")), common.DatabaseTypeSQLite},
		{"mysql", "TEST_MYSQL_DSN", mysql.Open(os.Getenv("TEST_MYSQL_DSN")), common.DatabaseTypeMySQL},
		{"postgres", "TEST_POSTGRES_DSN", postgres.Open(os.Getenv("TEST_POSTGRES_DSN")), common.DatabaseTypePostgreSQL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" && os.Getenv(tc.env) == "" {
				t.Skip("real database unavailable: " + tc.env + " not configured")
			}
			db, err := gorm.Open(tc.dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(tc.dbType, tc.dbType)
			initCol()
			sqlDB, err := DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			var version string
			query := "SELECT version()"
			if tc.name == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, DB.Raw(query).Scan(&version).Error)
			t.Logf("real database version: %s", version)
			// Seed a representative pre-existing table; verification migrations
			// must preserve data outside their own two tables.
			require.NoError(t, DB.AutoMigrate(&Option{}))
			require.NoError(t, DB.Where(&Option{Key: "runtime-verification-preserve"}).FirstOrCreate(&Option{Key: "runtime-verification-preserve", Value: "previous-value"}).Error)
			require.NoError(t, migrateDB())
			recorder := &migrationSQLRecorder{}
			DB = db.Session(&gorm.Session{Logger: recorder})
			require.NoError(t, migrateDB())
			for _, statement := range recorder.schemaMutations() {
				assert.NotContains(t, statement, "runtime_verification", "restart must not churn the verification schema")
			}
			var preserved Option
			require.NoError(t, DB.Where(commonKeyCol+" = ?", "runtime-verification-preserve").First(&preserved).Error)
			assert.Equal(t, "previous-value", preserved.Value)
			run, items := runtimeVerificationRunFixture(t, "0.3", 1)
			require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
			require.NoError(t, migrateDB())
			saved, err := GetRuntimeVerificationRun(run.ID)
			require.NoError(t, err)
			assert.Equal(t, "0.3", saved.ReservedProviderCost)
			duplicate := items[0]
			duplicate.ID = 0
			assert.Error(t, DB.Create(&duplicate).Error)
			duplicateRun := *run
			duplicateRun.ID = 0
			assert.Error(t, DB.Create(&duplicateRun).Error, "database uniqueness must also reject authorization rollover")
			if tc.name != "sqlite" {
				sqlDB.SetMaxOpenConns(2)
			}
			concurrentRun, concurrentItems := runtimeVerificationRunFixture(t, "1", 3)
			results := make(chan error, 2)
			var claims sync.WaitGroup
			for _, item := range concurrentItems[:2] {
				claims.Go(func() {
					results <- ClaimRuntimeVerificationSubmit(concurrentRun.ID, item.ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3")
				})
			}
			claims.Wait()
			close(results)
			successes := 0
			for err := range results {
				if err == nil {
					successes++
				} else {
					assert.ErrorIs(t, err, ErrRuntimeVerificationConflict)
				}
			}
			assert.Equal(t, 1, successes, "run locking must serialize different item submit claims")
			runtimeVerificationConsumeReceiptFixture(t, concurrentRun, &concurrentItems[0])
			runtimeVerificationCanonicalUsageFixture(t, concurrentRun, &concurrentItems[2])
		})
	}
}

// These disposable databases are seeded by the actual upstream release before
// this test runs; no hand-written schema substitutes for the released migrator.
func TestRuntimeVerificationReleasedUpgrade(t *testing.T) {
	previousDB := DB
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { DB = previousDB; common.SetDatabaseTypes(mainType, logType); initCol() })
	for _, tc := range []struct {
		name, env string
		open      func(string) gorm.Dialector
		dbType    common.DatabaseType
	}{
		{"sqlite", "RUNTIME_VERIFICATION_UPGRADE_SQLITE", func(dsn string) gorm.Dialector { return sqlite.Open(dsn) }, common.DatabaseTypeSQLite},
		{"mysql", "RUNTIME_VERIFICATION_UPGRADE_MYSQL_DSN", func(dsn string) gorm.Dialector { return mysql.Open(dsn) }, common.DatabaseTypeMySQL},
		{"postgres", "RUNTIME_VERIFICATION_UPGRADE_POSTGRES_DSN", func(dsn string) gorm.Dialector { return postgres.Open(dsn) }, common.DatabaseTypePostgreSQL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skip("released database fixture not configured: " + tc.env)
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			DB = db
			common.SetDatabaseTypes(tc.dbType, tc.dbType)
			initCol()
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.False(t, DB.Migrator().HasTable(&RuntimeVerificationRun{}), "upgrade fixture must begin with the released schema")
			var option Option
			require.NoError(t, DB.Where(commonKeyCol+" = ?", "runtime-verification-preserve").First(&option).Error)
			require.Equal(t, "release-rc41", option.Value)
			var channel Channel
			require.NoError(t, DB.First(&channel, 91).Error)
			require.NoError(t, migrateDB())
			recorder := &migrationSQLRecorder{}
			DB = db.Session(&gorm.Session{Logger: recorder})
			require.NoError(t, migrateDB())
			for _, statement := range recorder.schemaMutations() {
				assert.NotContains(t, statement, "runtime_verification")
			}
			var after Channel
			require.NoError(t, DB.First(&after, 91).Error)
			assert.Equal(t, channel.Name, after.Name)
			assert.Equal(t, channel.Models, after.Models)
			assert.Equal(t, channel.Key, after.Key)
			assert.Error(t, DB.Create(&Option{Key: "runtime-verification-preserve", Value: "duplicate"}).Error)
			require.NoError(t, DB.Where(commonKeyCol+" = ?", "runtime-verification-preserve").First(&option).Error)
			assert.Equal(t, "release-rc41", option.Value)
			run, items := runtimeVerificationRunFixture(t, "0.3", 1)
			require.NoError(t, ClaimRuntimeVerificationSubmit(run.ID, items[0].ID, strings.Repeat("e", 64), strings.Repeat("f", 64), "0.3"))
			require.NoError(t, migrateDB())
			saved, err := GetRuntimeVerificationRun(run.ID)
			require.NoError(t, err)
			assert.Equal(t, "0.3", saved.ReservedProviderCost)
			duplicate := items[0]
			duplicate.ID = 0
			assert.Error(t, DB.Create(&duplicate).Error)
		})
	}
}
