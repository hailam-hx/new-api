package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
			for _, prefix := range []string{"log:", "technical:"} {
				providerReport := report
				providerReport.Captures = []dflop.HistoricalCapture{{Model: "voice-tts-pro", EvidenceID: prefix + "1", Response: json.RawMessage(`{"request_id":"provider-request","model":"voice-tts-pro"}`)}}
				exact, _, err := correlateLocalHistory(&providerReport, 991)
				require.NoError(t, err)
				require.Len(t, exact, 2)
				assert.Equal(t, "EXACT_REQUEST_ID", exact[0].Confidence)
				assert.True(t, exact[0].BindingVerified)
			}
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

func TestProviderContractAuditRejectsPaidFlagsBeforeDatabase(t *testing.T) {
	for _, args := range [][]string{
		{"provider-contract-audit", "--execute", "--sqlite-db", "/missing"},
		{"provider-contract-audit", "--confirm-paid-canary", "never", "--sqlite-db", "/missing"},
	} {
		require.ErrorContains(t, run(args), "PROVIDER_CONTRACT_GET_ONLY")
	}
}

type contractAuditTransport struct{ paths []string }

func (tr *contractAuditTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.paths = append(tr.paths, req.Method+" "+req.URL.Path)
	body := `{ "schema_version":"1.0", "currency":"points", "aliases":{}, "models":[{"id":"doubao-seedance-2.0-lite","billing":{"features":["video_two_stage"]},"pricing":{"callable":true,"category":"video","endpoint_type":"videos_generations","supported_protocols":[],"video_second_stage_per_second":null},"caps":{},"authorization":"Bearer fixture-secret","prompt":"private prompt","source_video_url":"https://private.example/media?token=private"}]}`
	if req.URL.Path != "/v1/catalog" {
		body = `{"models":[]}`
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestProviderContractAuditGETOnlyRedactedNoPromotion(t *testing.T) {
	tr := &contractAuditTransport{}
	path := filepath.Join(t.TempDir(), "audit.json")
	require.NoError(t, runProviderContractAudit(context.Background(), dflop.Client{HTTP: &http.Client{Transport: tr}}, 1, "fixture-secret", "", path))
	assert.Equal(t, []string{"GET /v1/catalog", "GET /api/v1/models/public"}, tr.paths)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, secret := range []string{"fixture-secret", "private prompt", "private.example"} {
		assert.False(t, bytes.Contains(raw, []byte(secret)))
	}
	var report map[string]any
	require.NoError(t, common.Unmarshal(raw, &report))
	assert.Equal(t, "PROVIDER_CONTRACT_GET_ONLY", report["mode"])
	assert.Equal(t, false, report["auto_promotes"])
	assert.Equal(t, false, report["pricing_applied"])
	assert.Equal(t, true, report["fresh_preview_required"])
	assert.Equal(t, false, report["paid_requests_executed"])
	assert.Greater(t, len(report["blockers"].([]any)), 0)
}
