package main

import (
	"context"
	"encoding/json"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalProductionReplayPreservesQuantityAndBindingGate(t *testing.T) {
	plan := dflop.CanaryPlan{PointsPerCNY: "6000"}
	// These are synthetic parser regressions, never captured historical evidence.
	t.Run("actual image response count", func(t *testing.T) {
		item := dflop.Item{ModelID: "tvod-midjourney-v7", PricingShape: "image:per_image", Prices: map[string]dflop.Price{"price_per_image": {Credits: "2.1"}}}
		raw := json.RawMessage(`{"data":[{"url":"https://example.test/1"},{"url":"https://example.test/2"},{"url":"https://example.test/3"},{"url":"https://example.test/4"}]}`)
		replay, err := replayHistoricalTask(context.Background(), item, raw, plan, "0.15")
		require.NoError(t, err)
		assert.True(t, replay.QuantityVerified)
		assert.True(t, replay.SettlementReplayed)
		assert.False(t, replay.BindingVerified)
		assert.False(t, replay.SettlementVerified)
		result, ok := replay.Result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 4, result["image_count"])
		assert.Equal(t, 105, result["quota"])
	})
	t.Run("terminal speech usage and log", func(t *testing.T) {
		item := dflop.Item{ModelID: "voice-tts-pro", Prices: map[string]dflop.Price{"price_per_tts_char": {Credits: "1"}}}
		replay, err := replayHistoricalTask(context.Background(), item, json.RawMessage(`{"id":"existing-task","model":"voice-tts-pro","status":"succeeded","characters":26}`), plan, "0.15")
		require.NoError(t, err)
		assert.True(t, replay.QuantityVerified)
		assert.True(t, replay.SettlementReplayed)
		assert.False(t, replay.BindingVerified)
		result, ok := replay.Result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 325, result["quota"])
		assert.Contains(t, result["consume_log_other"], "usage_facts")
		_, err = replayHistoricalTask(context.Background(), item, json.RawMessage(`{"id":"existing-task","model":"voice-tts-pro","status":"succeeded"}`), plan, "0.15")
		require.ErrorContains(t, err, "PRODUCTION_TTS_USAGE_REJECTED")
	})
}

func TestLocalHistoricalCorrelationDatabaseMatrix(t *testing.T) {
	original := model.DB
	t.Cleanup(func() { model.DB = original })
	for _, tc := range []struct {
		name, dsn string
		dialect   gorm.Dialector
	}{
		{"sqlite", "", sqlite.Open(":memory:")},
		{"mysql", os.Getenv("DFLOP_TEST_MYSQL_DSN"), mysql.Open(os.Getenv("DFLOP_TEST_MYSQL_DSN"))},
		{"postgres", os.Getenv("DFLOP_TEST_PG_DSN"), postgres.Open(os.Getenv("DFLOP_TEST_PG_DSN"))},
	} {
		if tc.name != "sqlite" && tc.dsn == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(tc.dialect, &gorm.Config{})
			require.NoError(t, err)
			model.DB = db
			require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Log{}))
			require.NoError(t, db.Where("channel_id = ?", 991).Delete(&model.Task{}).Error)
			task := model.Task{TaskID: "local-task", ChannelId: 991, Platform: "dflop-tts", Properties: model.Properties{OriginModelName: "client", UpstreamModelName: "voice-tts-pro"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "real-task", Execution: &model.TaskExecutionSnapshot{RequestID: "local-request", Passive: &model.RuntimeEvidence{TaskID: "real-task", RequestID: "provider-request", TraceID: "provider-trace", ClientModel: "client", UpstreamModel: "voice-tts-pro", ChannelID: 991, Terminal: map[string]any{"model": "voice-tts-pro", "status": "succeeded", "characters": 26}}, TaskPlugin: &model.TaskPluginSnapshot{Key: "dflop-tts"}}}}
			require.NoError(t, db.Create(&task).Error)
			t.Cleanup(func() {
				require.NoError(t, db.Delete(&task).Error)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
			})
			raw := json.RawMessage(`{"id":"real-task","model":"voice-tts-pro","status":"succeeded","characters":26}`)
			report := dflop.HistoricalReport{Captures: []dflop.HistoricalCapture{{Model: "voice-tts-pro", EvidenceID: "task-poll:real-task", Response: raw}}, Models: []dflop.HistoricalModelReport{{Model: "voice-tts-pro", BillingFeatures: []string{"tts_char"}, CatalogPrices: map[string]dflop.Price{"price_per_tts_char": {Credits: "0.01"}}, QuantityVerified: true, ReconciliationResult: "INSUFFICIENT_EVIDENCE"}}}
			bindings, status, err := correlateLocalHistory(&report, 991)
			require.NoError(t, err)
			assert.Equal(t, "LOCAL_RECORDS_READ", status)
			require.Len(t, bindings, 1)
			assert.Equal(t, "EXACT_TASK_ID", bindings[0].Confidence)
			assert.True(t, bindings[0].BindingVerified)
			assert.True(t, bindings[0].RuntimeUsageVerified)
			assert.False(t, bindings[0].SettledCostReconciled)
			localReport := report
			localReport.Captures = nil
			passive, _, err := correlateLocalHistory(&localReport, 991)
			require.NoError(t, err)
			require.Len(t, passive, 1)
			assert.Equal(t, "EXACT_TASK_ID", passive[0].Confidence)
			assert.True(t, passive[0].RuntimeUsageVerified)
			assert.False(t, localReport.Models[0].CanUnlock)
			require.Len(t, localReport.Captures, 1)
			assert.Equal(t, "CAPTURED_LOCAL_PASSIVE", localReport.Captures[0].EvidenceClass)

			var version string
			query := "SELECT version()"
			if tc.name == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Log(version)
		})
	}
}
