package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetChannelRuntimeVerification only reads durable evidence. A run id is always
// scoped to its channel before any model, usage, or ledger facts are returned.
func GetChannelRuntimeVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorMsg(c, "invalid channel id")
		return
	}
	if runIDValue := c.Param("run_id"); runIDValue != "" {
		runID, err := strconv.ParseInt(runIDValue, 10, 64)
		if err != nil || runID <= 0 {
			common.ApiErrorMsg(c, "invalid verification run id")
			return
		}
		run, err := model.GetRuntimeVerificationRun(runID)
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && run.ChannelID != channelID) {
			common.ApiErrorMsg(c, "verification run not found for channel")
			return
		}
		if err != nil {
			common.ApiError(c, err)
			return
		}
		items, err := model.ListRuntimeVerificationItems(run.ID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		writeChannelRuntimeVerification(c, run, items)
		return
	}
	run, items, err := model.LatestRuntimeVerificationEvidence(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeChannelRuntimeVerification(c, run, items)
}

// PrepareChannelRuntimeVerification cannot accept a paid execution intent,
// request payload, authorization, or client-authored evidence. The service is
// restricted to configuration evaluation and the free authenticated catalog GET.
func PrepareChannelRuntimeVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorMsg(c, "invalid channel id")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4097))
	var fields map[string]json.RawMessage
	if err != nil || len(body) > 4096 || common.Unmarshal(body, &fields) != nil || fields == nil {
		common.ApiErrorMsg(c, "invalid verification preparation request")
		return
	}
	for field := range fields {
		if field != "model" {
			common.ApiErrorMsg(c, "unsupported verification preparation field")
			return
		}
	}
	var request struct {
		Model string `json:"model"`
	}
	if common.Unmarshal(body, &request) != nil || len(request.Model) > 128 {
		common.ApiErrorMsg(c, "invalid verification model")
		return
	}
	run, items, err := service.PrepareDFLOPRuntimeVerification(c.Request.Context(), channelID, strings.TrimSpace(request.Model), c.GetInt("id"))
	if err != nil {
		reason := err.Error()
		response := gin.H{"success": false, "message": reason}
		if reason != "" && len(reason) <= 128 && strings.Trim(reason, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") == "" {
			response["reason_code"] = reason
		}
		c.JSON(http.StatusOK, response)
		return
	}
	writeChannelRuntimeVerification(c, run, items)
}

// This administrator read projection excludes stored credentials, authorization
// manifests, request bodies, billing snapshots, and private plugin state.
func writeChannelRuntimeVerification(c *gin.Context, run *model.RuntimeVerificationRun, items []model.RuntimeVerificationItem) {
	var runView any
	if run != nil {
		runView = gin.H{
			"id": run.ID, "channel_id": run.ChannelID, "source": run.Source,
			"catalog_hash": run.CatalogHash, "started_at": run.StartedAt,
			"completed_at": run.CompletedAt, "mode": run.Mode, "status": run.Status,
			"paid_requests": run.PaidRequests,
		}
	}
	itemViews := make([]gin.H, 0, len(items))
	for _, item := range items {
		itemViews = append(itemViews, channelRuntimeVerificationItemView(item))
	}
	common.ApiSuccess(c, gin.H{"run": runView, "items": itemViews})
}

// Historical rows use the same safe projection while retaining their original
// run and hashes; their paid outcomes never replace the current run's states.
func channelRuntimeVerificationItemView(item model.RuntimeVerificationItem) gin.H {
	view := gin.H{
		"id": item.ID, "run_id": item.RunID, "model": item.Model,
		"protocol": item.Protocol, "operation": item.Operation, "mode": item.Mode,
		"fixture_id": item.FixtureID, "endpoint": item.Endpoint,
		"catalog_hash": item.CatalogHash, "pricing_snapshot_hash": item.PricingSnapshotHash,
		"billing_expr_hash": item.BillingExprHash, "billing_source": item.BillingSource,
		"config_status": item.ConfigStatus, "connectivity_status": item.ConnectivityStatus,
		"request_status": item.RequestStatus, "generation_status": item.GenerationStatus,
		"parser_status": item.ParserStatus, "billing_status": item.BillingStatus,
		"ledger_status": item.LedgerStatus, "status": item.Result,
		"result": item.Result, "reason_code": item.ReasonCode,
		"request_id": item.RequestID, "task_id": item.TaskID, "trace_id": item.TraceID,
		"idempotency_key_hash": item.IdempotencyKeyHash, "terminal_status": item.TerminalStatus,
		"normalized_usage_json": item.NormalizedUsageJSON, "provider_usage_json": item.ProviderUsageJSON,
		"provider_unit_count": item.ProviderUnitCount, "provider_cost_points": item.ProviderCostPoints,
		"newapi_raw_cost": item.NewapiRawCost, "evidence_json": item.EvidenceJSON,
		"correlation_quality": item.CorrelationQuality, "verified_at": item.VerifiedAt,
	}
	var frozen model.RuntimeVerificationProviderSnapshot
	_ = common.UnmarshalJsonStr(item.FrozenProviderJSON, &frozen)
	var evidence struct {
		FixtureHash            string   `json:"fixture_hash"`
		ConfigHash             string   `json:"config_hash"`
		WarningCodes           []string `json:"warning_codes"`
		HistoricalRuntimeState string   `json:"historical_runtime_state"`
		HistoricalReasonCode   string   `json:"historical_reason_code"`
		HistoricalRunID        int64    `json:"historical_run_id"`
		CurrentCanaryReadiness string   `json:"current_canary_readiness"`
	}
	_ = common.UnmarshalJsonStr(item.EvidenceJSON, &evidence)
	if frozen.FixtureHash != "" {
		evidence.FixtureHash = frozen.FixtureHash
	}
	if frozen.ConfigHash != "" {
		evidence.ConfigHash = frozen.ConfigHash
	}
	view["fixture_hash"], view["config_hash"], view["warning_codes"] = evidence.FixtureHash, evidence.ConfigHash, evidence.WarningCodes
	view["usage_state"], view["current_run_id"] = item.ParserStatus, item.RunID
	view["historical_runtime_state"], view["historical_reason_code"], view["historical_run_id"], view["current_canary_readiness"] = evidence.HistoricalRuntimeState, evidence.HistoricalReasonCode, evidence.HistoricalRunID, evidence.CurrentCanaryReadiness

	if item.NewapiQuota != nil {
		view["newapi_quota"] = *item.NewapiQuota
	}
	if item.WalletDelta != nil {
		view["wallet_delta"] = *item.WalletDelta
	}
	if item.HistoricalEvidence != nil {
		view["historical_evidence"] = channelRuntimeVerificationItemView(*item.HistoricalEvidence)
	}
	return view
}
