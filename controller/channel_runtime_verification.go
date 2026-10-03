package controller

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

// PlanChannelRuntimeVerification prepares a reviewable zero-paid-cost plan.
// Fixture origins and preset resources come from server configuration only.
func PlanChannelRuntimeVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		Model string `json:"model"`
	}
	if c.ContentType() != "application/json" || c.GetHeader("Sec-Fetch-Site") == "cross-site" || err != nil || channelID <= 0 || c.GetInt("id") <= 0 || common.DecodeJson(io.LimitReader(c.Request.Body, 4096), &request) != nil || request.Model == "" || len(request.Model) > 128 {
		common.ApiErrorMsg(c, "INVALID_VERIFICATION_PLAN_REQUEST")
		return
	}
	engine, err := service.NewDFLOPVerificationEngine(c.Request.Context(), channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	run, _, err := engine.Prepare(c.Request.Context(), channelID, request.Model, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plan, err := engine.Plan(c.Request.Context(), run.ID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

// ExecuteChannelRuntimeVerification cannot authorize from an approved flag or
// a UI click alone. An operator-signed exact manifest is mandatory.
func ExecuteChannelRuntimeVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		Plan          service.DFLOPVerificationPlan `json:"plan"`
		ManifestJSON  string                        `json:"manifest_json"`
		ConfirmedHash string                        `json:"confirmed_manifest_hash"`
	}
	body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, (4<<20)+1))
	if c.ContentType() != "application/json" || c.GetHeader("Sec-Fetch-Site") == "cross-site" || err != nil || channelID <= 0 || c.GetInt("id") <= 0 || readErr != nil || len(body) > 4<<20 || common.Unmarshal(body, &request) != nil {
		common.ApiErrorMsg(c, "INVALID_VERIFICATION_EXECUTION_REQUEST")
		return
	}
	trusted, err := hex.DecodeString(os.Getenv("DFLOP_VERIFICATION_APPROVAL_PUBLIC_KEY"))
	if err != nil || len(trusted) != ed25519.PublicKeySize {
		common.ApiErrorMsg(c, "TRUSTED_APPROVAL_KEY_REQUIRED")
		return
	}
	authorization, err := service.ValidateDFLOPAdminExecution(channelID, c.GetInt("id"), request.Plan, request.ManifestJSON, request.ConfirmedHash, trusted, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Cancellation leaves a durable claim/reservation recoverable by GET, never
	// a retryable paid POST. Do not detach work from the administrator request.
	run, executionErr := (service.DFLOPVerificationEngine{}).ExecuteAdminPlan(c.Request.Context(), channelID, c.GetInt("id"), request.Plan, authorization)
	if run == nil {
		common.ApiError(c, executionErr)
		return
	}
	writeRuntimeExecutionResult(c, run.ID, executionErr)
}

// ResumeChannelRuntimeVerification recovers only this administrator's claimed
// intent on this channel. It cannot adopt arbitrary operator-supplied task IDs.
func ResumeChannelRuntimeVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		RunID   int64                       `json:"run_id"`
		ItemID  int64                       `json:"item_id"`
		Fixture service.VerificationFixture `json:"fixture"`
	}
	body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if c.ContentType() != "application/json" || c.GetHeader("Sec-Fetch-Site") == "cross-site" || err != nil || channelID <= 0 || c.GetInt("id") <= 0 || readErr != nil || len(body) > 1<<20 || common.Unmarshal(body, &request) != nil || request.RunID <= 0 || request.ItemID <= 0 {
		common.ApiErrorMsg(c, "INVALID_VERIFICATION_RECOVERY_REQUEST")
		return
	}
	run, recoveryErr := (service.DFLOPVerificationEngine{}).ResumeAdminItem(c.Request.Context(), channelID, c.GetInt("id"), request.RunID, request.ItemID, request.Fixture)
	if run == nil {
		common.ApiError(c, recoveryErr)
		return
	}
	writeRuntimeExecutionResult(c, run.ID, recoveryErr)
}

func writeRuntimeExecutionResult(c *gin.Context, runID int64, executionErr error) {
	run, err := model.GetRuntimeVerificationRun(runID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, err := model.ListRuntimeVerificationItems(run.ID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Return the durable run even for unresolved execution, so the UI can recover
	// the existing intent instead of encouraging a second paid submission.
	views := make([]gin.H, 0, len(items))
	for _, item := range items {
		views = append(views, channelRuntimeVerificationItemView(item))
	}
	reason := ""
	if executionErr != nil {
		reason = "VERIFICATION_UNRESOLVED_USE_GET_RECOVERY"
	}
	common.ApiSuccess(c, gin.H{"run": gin.H{"id": run.ID, "channel_id": run.ChannelID, "catalog_hash": run.CatalogHash, "started_at": run.StartedAt, "status": run.Status, "source": run.Source, "paid_requests": run.PaidRequests}, "items": views, "execution_reason": reason})
}
