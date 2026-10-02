package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func (engine DFLOPVerificationEngine) request(ctx context.Context, key, method, path, intentKey string, body []byte) ([]byte, http.Header, int, error) {
	parsed, err := url.Parse(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/v1/") || strings.Contains(parsed.Path, "..") {
		return nil, nil, 0, errors.New("PROVIDER_ENDPOINT_REJECTED")
	}
	if method != http.MethodGet && (method != http.MethodPost || intentKey == "" || parsed.RawQuery != "") {
		return nil, nil, 0, errors.New("PROVIDER_METHOD_REJECTED")
	}
	request, err := http.NewRequestWithContext(ctx, method, "https://api.dflop.top"+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	if intentKey != "" {
		request.Header.Set("Idempotency-Key", intentKey)
	}
	client := http.Client{Timeout: 45 * time.Second}
	if engine.HTTP != nil {
		client = *engine.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// Never retry a POST, including a body-read error after response headers.
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, 0, errors.New("PROVIDER_TRANSPORT_AMBIGUOUS")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return nil, response.Header, response.StatusCode, errors.New("PROVIDER_RESPONSE_AMBIGUOUS")
	}
	return data, response.Header, response.StatusCode, nil
}

func verificationCaptureIDs(item *model.RuntimeVerificationItem, headers http.Header, body []byte, key string) error {
	var envelope struct {
		RequestID string `json:"request_id"`
		TaskID    string `json:"task_id"`
	}
	_ = common.Unmarshal(body, &envelope)
	for _, capture := range []struct {
		destination *string
		values      []string
	}{
		{&item.TraceID, []string{headers.Get("x-gateway-trace")}},
		{&item.RequestID, []string{headers.Get("x-request-id"), headers.Get("request-id"), envelope.RequestID}},
		{&item.TaskID, []string{envelope.TaskID}},
	} {
		for _, value := range capture.values {
			if value == "" {
				continue
			}
			if !verificationSecretSafeID(value) || len(value) > 128 || key != "" && strings.Contains(value, key) {
				return errors.New("PROVIDER_CORRELATION_ID_INVALID")
			}
			if *capture.destination != "" && *capture.destination != value {
				return errors.New("PROVIDER_CORRELATION_ID_CONFLICT")
			}
			*capture.destination = value
		}
	}
	return nil
}

func verificationUpdate(item *model.RuntimeVerificationItem, result, reason string) error {
	item.Result, item.ReasonCode = result, reason
	return model.UpdateRuntimeVerificationItem(item)
}

// Execute claims one durable intent before the sole billed POST. A previously
// claimed intent can only be resumed through free GETs, never resubmitted here.
func (engine DFLOPVerificationEngine) Execute(ctx context.Context, runID, itemID int64, authorization DFLOPVerificationAuthorization, fixture VerificationFixture) error {
	if fixture.verifyPublicMedia == nil {
		fixture.verifyPublicMedia = engine.FixtureOptions.VerifyPublicMedia
	}
	if fixture.verifyClipSource == nil {
		fixture.verifyClipSource = engine.FixtureOptions.VerifyClipSource
	}

	if common.BatchUpdateEnabled {
		return errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED")
	}
	if !common.LogConsumeEnabled || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return errors.New("BILLING_CONSUME_RECEIPT_UNAVAILABLE")
	}
	run, err := model.GetRuntimeVerificationRun(runID)
	if err != nil {
		return err
	}
	var item model.RuntimeVerificationItem
	if err = model.DB.Where("id = ? AND run_id = ?", itemID, runID).First(&item).Error; err != nil {
		return err
	}
	manifest, _ := common.Marshal(authorization)
	if run.AuthorizationManifestHash == "" || run.AuthorizationManifestHash != verificationHash(manifest) {
		return errors.New("PAID_AUTHORIZATION_REQUIRED")
	}
	if item.RequestBodyHash != "" {
		return errors.New("SUBMIT_ALREADY_CLAIMED_USE_GET_RECOVERY")
	}
	if fixture.Model != item.Model || fixture.ID != item.FixtureID || fixture.Protocol != item.Protocol || fixture.Mode != item.Mode {
		return errors.New("FIXTURE_IDENTITY_MISMATCH")
	}
	source, err := engine.source(ctx, run.ChannelID)
	if err != nil {
		return err
	}
	if fixture.Plan.ID == "AVATAR_SECONDS" {
		avatar, _ := fixture.Request["avatar"].(string)
		voice, _ := fixture.Request["voice"].(string)
		if err = engine.verifyPresetResources(ctx, source.key, avatar, voice); err != nil {
			return err
		}
	}
	if voice, _ := fixture.Request["voice"].(string); fixture.Plan.ID != "AVATAR_SECONDS" && voice != "" {
		if err = engine.verifyPresetResources(ctx, source.key, "", voice); err != nil {
			return err
		}
	}
	if fixture.Plan.ID == "CLIP_COMPOSE_FIXED_TASK" {
		if fixture.ClipPreparation == nil {
			return errors.New("FRESH_SAME_SOURCE_ASR_REQUIRED")
		}
		prepared := *fixture.ClipPreparation
		fixture.verifyClipSource = func(ctx context.Context, url, id string) error {
			return prepared.validate(source.key, url, id, source.hash, time.Now())
		}
	}
	if source.hash != run.CatalogHash || source.fingerprint != run.CredentialFingerprint {
		return errors.New("DRIFT_REVIEW_REQUIRED")
	}
	group, err := model.GetUserGroup(run.FundingUserID, true)
	if err != nil {
		return err
	}
	providerIndex := slices.IndexFunc(source.items, func(i dflop.Item) bool { return i.ModelID == item.Model })
	if providerIndex < 0 {
		return errors.New("SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG")
	}
	fixture.catalogContract = &source.items[providerIndex]
	audit := AuditDFLOPVerificationPlanContract(fixture, source.items[providerIndex])
	if audit.Blocker != "" {
		return errors.New(audit.Blocker)
	}
	plugin, ok := jsplugin.DefaultRegistry.Generation().Get(fixture.Plugin)
	if !ok {
		return errors.New("PLUGIN_BINDING_MISMATCH")
	}
	snapshot, frozen, request, maximum, err := verificationFrozenSnapshot(source, fixture, source.items[providerIndex], plugin, group)
	if err != nil {
		return err
	}
	frozenJSON, _ := common.Marshal(frozen)
	snapshotJSON, _ := common.Marshal(snapshot)
	if item.PricingSnapshotHash != verificationHash(frozenJSON) || item.BillingSnapshotJSON != string(snapshotJSON) {
		return errors.New("PRICING_SNAPSHOT_DRIFT")
	}
	if authorization.PlanVersion == DFLOPVerificationPlanVersion {
		urls := []string{}
		for _, media := range fixture.Media {
			if media.PublicURL != "" {
				urls = append(urls, media.PublicURL)
			}
		}
		intentJSON, _ := common.Marshal(fixture.Request)
		hashes := []string{}
		for _, url := range urls {
			hashes = append(hashes, verificationHash([]byte(url)))
		}
		if !slices.ContainsFunc(authorization.Targets, func(target DFLOPVerificationTarget) bool {
			return target.Model == item.Model && target.StaticIntentHash == verificationHash(intentJSON) && slices.Equal(target.FixturePublicURLHashes, hashes) && slices.Equal(target.FixturePublicURLs, urls)
		}) {
			return errors.New("PAID_FIXTURE_URL_BINDING_MISMATCH")
		}
	}
	item.RequestBodyHash = request.BodyHash
	if err = ValidateDFLOPVerificationAuthorization(authorization, *run, item, maximum, time.Now()); err != nil {
		return err
	}
	intentKey := verificationIntentKey(run.ID, item.ID, request.BodyHash)
	if err = model.ClaimRuntimeVerificationSubmit(run.ID, item.ID, request.BodyHash, verificationHash([]byte(intentKey)), maximum); err != nil {
		return err
	}
	item.IdempotencyKeyHash, item.ReservedProviderCost = verificationHash([]byte(intentKey)), maximum
	item.RequestStatus, item.Result, item.ReasonCode = "AMBIGUOUS", "RUNTIME_AMBIGUOUS", "SUBMIT_INTENT_CLAIMED"
	contextGin, _ := gin.CreateTestContext(httptest.NewRecorder())
	contextGin.Request = httptest.NewRequest(http.MethodPost, fixture.Endpoint, bytes.NewReader(request.Body)).WithContext(ctx)
	contextGin.Request.Header.Set("Idempotency-Key", intentKey)
	info := &relaycommon.RelayInfo{UserId: run.FundingUserID, IsPlayground: true, UsingGroup: group, UserGroup: group, OriginModelName: item.Model, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: run.ChannelID, ChannelBaseUrl: "https://api.dflop.top", UpstreamModelName: item.Model}, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, TieredBillingSnapshot: snapshot, PriceData: types.PriceData{Quota: snapshot.EstimatedQuotaAfterGroup}}
	if err = PrepareDFLOPTaskBillingIdentity(contextGin, info, fixture.Plugin); err != nil {
		return err
	}
	item.LocalRequestID, item.LocalTaskID = info.RequestId, fmt.Sprintf("dflop-verification-%d-%d", run.ID, item.ID)
	if err = model.UpdateRuntimeVerificationItem(&item); err != nil {
		return err
	}
	if apiErr := PreConsumeBilling(contextGin, snapshot.EstimatedQuotaAfterGroup, info); apiErr != nil {
		return errors.New("RESERVATION_JOURNAL_FAILED")
	}
	if err = EnsureDFLOPTaskReservation(contextGin, info); err != nil {
		return err
	}
	task := &model.Task{TaskID: item.LocalTaskID, Platform: constant.TaskPlatform(fixture.Plugin), UserId: run.FundingUserID, Group: group, ChannelId: run.ChannelID, Quota: snapshot.EstimatedQuotaAfterGroup, Action: request.Action, Status: model.TaskStatusSubmitted, SubmitTime: time.Now().Unix(), Progress: "0%", Properties: model.Properties{UpstreamModelName: item.Model}, PrivateData: model.TaskPrivateData{BillingSource: BillingSourceWallet, BillingContext: &model.TaskBillingContext{OriginModelName: item.Model, TieredSnapshot: snapshot, GroupRatio: snapshot.GroupRatio}, Execution: &model.TaskExecutionSnapshot{RequestID: info.RequestId, RequestPath: fixture.Endpoint, RuntimeVerificationItemID: item.ID}}}
	if err = task.Insert(); err != nil {
		return errors.New("RESERVATION_TASK_PERSISTENCE_FAILED")
	}
	if err = LinkDFLOPTaskReservation(info, task); err != nil {
		return err
	}
	// The journal is durable before outbound. A crash from here permanently
	// retains the intent and reservation until exact GET evidence resolves it.
	info.DFLOPTaskOutbound = true
	response, headers, status, requestErr := engine.request(ctx, source.key, http.MethodPost, request.URLPath, intentKey, request.Body)
	identityErr := verificationCaptureIDs(&item, headers, response, source.key)
	info.DFLOPTaskHTTPStatus = status
	var evidence model.RuntimeVerificationEvidence
	_ = common.UnmarshalJsonStr(item.EvidenceJSON, &evidence)
	if status > 0 {
		evidence.ProviderStatusCode = &status
	}
	evidence.JournalState = "HELD"
	evidence.CatalogTraceID, evidence.CurrencyTraceID = source.catalogTraceID, source.currencyTraceID
	encoded, _ := common.Marshal(evidence)
	item.EvidenceJSON = string(encoded)
	if requestErr != nil || identityErr != nil || status < 200 || status >= 300 {
		item.RequestStatus = "AMBIGUOUS"
		if holdErr := HoldDFLOPTaskReservation(info, "VERIFICATION_PROVIDER_ACCEPTANCE_UNKNOWN"); holdErr != nil {
			return holdErr
		}
		reason := "PROVIDER_ACCEPTANCE_UNKNOWN"
		if identityErr != nil {
			reason = verificationReason(identityErr)
		}
		if err = verificationUpdate(&item, "RUNTIME_AMBIGUOUS", reason); err != nil {
			return err
		}
		if identityErr != nil {
			return nil // Conflicting identities cannot select a ledger receipt.
		}
		return engine.Resume(ctx, run.ID, item.ID, fixture)
	}
	submission, parseErr := ParseDFLOPVerificationSubmission(ctx, fixture, plugin, request.RequestBody, response)
	if parseErr != nil {
		item.RequestStatus = "AMBIGUOUS"
		if err = HoldDFLOPTaskReservation(info, "PROVIDER_TASK_ID_MISSING"); err != nil {
			return err
		}
		if err = verificationUpdate(&item, "RUNTIME_AMBIGUOUS", verificationReason(parseErr)); err != nil {
			return err
		}
		return engine.Resume(ctx, run.ID, item.ID, fixture)
	}
	if !verificationSecretSafeID(submission.TaskID) || item.TaskID != "" && item.TaskID != submission.ExactTaskID {
		item.RequestStatus = "AMBIGUOUS"
		if err = HoldDFLOPTaskReservation(info, "PROVIDER_CORRELATION_ID_CONFLICT"); err != nil {
			return err
		}
		return verificationUpdate(&item, "RUNTIME_AMBIGUOUS", "PROVIDER_CORRELATION_ID_CONFLICT")
	}
	item.TaskID, item.RequestStatus, item.PluginStateJSON = submission.ExactTaskID, "PASS", string(submission.PluginState)
	task.PrivateData.UpstreamTaskID = submission.ExactTaskID
	task.PrivateData.PluginState = submission.PluginState
	task.PrivateData.Execution.RuntimeVerificationTraceID = item.TraceID
	task.PrivateData.Execution.RuntimeVerificationRequestID = item.RequestID
	// The terminal body is not retained: it is replayed now, or retrieved freely
	// by exact task ID after a restart. Persist only normalized redacted facts.
	if _, err = task.UpdateWithStatus(task.Status); err != nil {
		return err
	}
	if err = model.UpdateRuntimeVerificationItem(&item); err != nil {
		return err
	}
	if submission.ImmediateStatus != "" {
		return engine.terminal(ctx, run, &item, fixture, plugin, request.RequestBody, response, source.key)
	}
	return engine.Resume(ctx, run.ID, item.ID, fixture)
}

// Resume never performs a POST. A changed catalog after submission cannot
// change the frozen tariff or BillingExpr used for a running task.
func (engine DFLOPVerificationEngine) Resume(ctx context.Context, runID, itemID int64, fixture VerificationFixture) error {
	if fixture.verifyPublicMedia == nil {
		fixture.verifyPublicMedia = engine.FixtureOptions.VerifyPublicMedia
	}
	if fixture.verifyClipSource == nil {
		fixture.verifyClipSource = engine.FixtureOptions.VerifyClipSource
	}

	run, err := model.GetRuntimeVerificationRun(runID)
	if err != nil {
		return err
	}
	var item model.RuntimeVerificationItem
	if err = model.DB.Where("id = ? AND run_id = ?", itemID, runID).First(&item).Error; err != nil {
		return err
	}
	if item.Result == "RUNTIME_VERIFIED" {
		return nil
	}
	if strings.HasPrefix(item.ReasonCode, "PROVIDER_CORRELATION_ID_") {
		return errors.New(item.ReasonCode)
	}
	if common.BatchUpdateEnabled {
		return errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED")
	}
	_, key, err := dflop.LoadSourceChannel(run.ChannelID)
	if err != nil {
		return err
	}
	if verificationHash([]byte(key)) != run.CredentialFingerprint {
		return errors.New("SOURCE_CREDENTIAL_DRIFT")
	}
	var frozen model.RuntimeVerificationProviderSnapshot
	if common.UnmarshalJsonStr(item.FrozenProviderJSON, &frozen) != nil {
		return errors.New("FROZEN_PRICING_REQUIRED")
	}
	fixtureJSON, _ := common.Marshal(fixture)
	plugin, ok := jsplugin.DefaultRegistry.Generation().Get(fixture.Plugin)
	if !ok {
		return errors.New("PLUGIN_BINDING_MISMATCH")
	}
	metaJSON, _ := common.Marshal(map[string]any{"meta": plugin.Meta, "source_hash": plugin.Engine.SourceHash()})
	if verificationHash(fixtureJSON) != frozen.FixtureHash || verificationHash(metaJSON) != frozen.PluginHash {
		return errors.New("FROZEN_CONTRACT_DRIFT")
	}
	request, err := ValidateDFLOPVerificationFixture(ctx, fixture, plugin)
	if err != nil {
		return err
	}
	if request.BodyHash != item.RequestBodyHash {
		return errors.New("FROZEN_REQUEST_BODY_MISMATCH")
	}
	if item.LocalTaskID != "" {
		var task model.Task
		if err = model.DB.Where("task_id = ? AND user_id = ?", item.LocalTaskID, run.FundingUserID).First(&task).Error; err != nil {
			return err
		}
		if task.PrivateData.Execution == nil || task.PrivateData.Execution.RuntimeVerificationItemID != item.ID || task.PrivateData.Execution.RequestID != item.LocalRequestID {
			return errors.New("BILLING_JOURNAL_IDENTITY_MISMATCH")
		}
		if item.TaskID == "" && task.PrivateData.UpstreamTaskID != "" {
			item.TaskID, item.TraceID, item.RequestID, item.RequestStatus = task.PrivateData.UpstreamTaskID, task.PrivateData.Execution.RuntimeVerificationTraceID, task.PrivateData.Execution.RuntimeVerificationRequestID, "PASS"
			item.PluginStateJSON = string(task.PrivateData.PluginState)
			if err = model.UpdateRuntimeVerificationItem(&item); err != nil {
				return err
			}
		}
	}
	if item.ParserStatus == "PASS" && item.TerminalStatus != "" {
		// Production parser evidence is durable; recovery needs only the exact
		// ledger and frozen tariff, never another POST or a private output URL.
		return engine.reconcileTerminal(ctx, run, &item, key)
	}
	if item.TaskID == "" {
		// Exact terminal ledger failure may release a reservation. A success log
		// alone cannot prove output and is never a runtime verification shortcut.
		return engine.resolveAmbiguous(ctx, run, &item, fixture, key)
	}
	deadline := time.NewTimer(time.Duration(fixture.Plan.MaxPollSeconds) * time.Second)
	defer deadline.Stop()
	var evidence model.RuntimeVerificationEvidence
	if err = common.UnmarshalJsonStr(item.EvidenceJSON, &evidence); err != nil {
		return err
	}
	for evidence.PollAttempts < 601 {
		poll, err := BuildDFLOPVerificationPoll(ctx, fixture, plugin, item.TaskID, []byte(item.PluginStateJSON))
		if err != nil {
			return err
		}
		body, headers, status, readErr := engine.request(ctx, key, http.MethodGet, poll.URLPath, "", nil)
		// Poll traces belong to distinct GETs, never the billed submit identity.
		evidence.PollAttempts++
		if traceID := headers.Get("x-gateway-trace"); traceID != "" && len(traceID) <= 128 && verificationSecretSafeID(traceID) && !strings.Contains(traceID, key) {
			evidence.PollTraceIDs = append(evidence.PollTraceIDs, traceID)
		}
		if readErr == nil && status >= 200 && status < 300 {
			replay, parseErr := ReplayDFLOPVerificationTaskTerminal(ctx, fixture, plugin, item.TaskID, request.RequestBody, []byte(item.PluginStateJSON), body)
			if len(replay.PluginState) > 0 {
				item.PluginStateJSON = string(replay.PluginState)
			}
			encoded, _ := common.Marshal(evidence)
			item.EvidenceJSON = string(encoded)
			if err = model.UpdateRuntimeVerificationItem(&item); err != nil {
				return err
			}
			if replay.Status == "SUCCESS" || replay.Status == "FAILURE" || parseErr != nil {
				return engine.terminal(ctx, run, &item, fixture, plugin, request.RequestBody, body, key)
			}
		} else {
			encoded, _ := common.Marshal(evidence)
			item.EvidenceJSON = string(encoded)
			if err = model.UpdateRuntimeVerificationItem(&item); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			item.GenerationStatus = "AMBIGUOUS"
			return verificationUpdate(&item, "RUNTIME_AMBIGUOUS", "POLL_INTERRUPTED")
		case <-deadline.C:
			item.GenerationStatus = "AMBIGUOUS"
			return verificationUpdate(&item, "RUNTIME_AMBIGUOUS", "POLL_DEADLINE_EXCEEDED")
		case <-time.After(time.Duration(max(fixture.Plan.PollIntervalSeconds, 1)) * time.Second):
		}
	}
	item.GenerationStatus = "AMBIGUOUS"
	return verificationUpdate(&item, "RUNTIME_AMBIGUOUS", "POLL_ATTEMPTS_EXCEEDED")
}

func (engine DFLOPVerificationEngine) lookupLedger(ctx context.Context, item *model.RuntimeVerificationItem, key, expected string) (DFLOPVerificationLedgerResult, error) {
	query := url.Values{"scope": {"key"}, "period": {"all"}, "include": {"attempts"}, "limit": {"200"}}
	if item.TaskID != "" {
		query.Set("ref", item.TaskID)
	} else if item.RequestID != "" {
		query.Set("ref", item.RequestID)
	} else if item.TraceID != "" {
		query.Set("ref", item.TraceID)
	} else {
		return DFLOPVerificationLedgerResult{Result: "INSUFFICIENT_EVIDENCE"}, nil
	}
	var evidence model.RuntimeVerificationEvidence
	_ = common.UnmarshalJsonStr(item.EvidenceJSON, &evidence)
	var rows []json.RawMessage
	for page := range 10 {
		body, headers, status, err := engine.request(ctx, key, http.MethodGet, "/v1/logs?"+query.Encode(), "", nil)
		if traceID := headers.Get("x-gateway-trace"); traceID != "" && len(traceID) <= 128 && verificationSecretSafeID(traceID) && !strings.Contains(traceID, key) {
			evidence.LedgerTraceID = traceID
			encoded, _ := common.Marshal(evidence)
			item.EvidenceJSON = string(encoded)
			if persistErr := model.UpdateRuntimeVerificationItem(item); persistErr != nil {
				return DFLOPVerificationLedgerResult{}, persistErr
			}
		}
		if err != nil || status != 200 {
			return DFLOPVerificationLedgerResult{}, errors.New("LEDGER_TEMPORARILY_UNAVAILABLE")
		}
		var envelope struct {
			Logs       []json.RawMessage `json:"logs"`
			Data       []json.RawMessage `json:"data"`
			Currency   string            `json:"currency"`
			Scope      string            `json:"scope"`
			HasMore    bool              `json:"has_more"`
			NextCursor string            `json:"next_cursor"`
		}
		if common.Unmarshal(body, &envelope) != nil || envelope.Currency != "points" || envelope.Scope != "key" {
			return DFLOPVerificationLedgerResult{}, errors.New("LEDGER_SCHEMA_UNSUPPORTED")
		}
		if envelope.Logs != nil {
			rows = append(rows, envelope.Logs...)
		} else {
			rows = append(rows, envelope.Data...)
		}
		if !envelope.HasMore {
			break
		}
		if page == 9 || envelope.NextCursor == "" || len(envelope.NextCursor) > 512 || envelope.NextCursor == query.Get("cursor") {
			return DFLOPVerificationLedgerResult{Result: "INSUFFICIENT_EVIDENCE", CorrelationQuality: "NONE"}, nil
		}
		query.Set("cursor", envelope.NextCursor)
	}
	matches := 0
	for _, row := range rows {
		if result := ReconcileDFLOPVerificationLedger(*item, row, expected); result.CorrelationQuality != "NONE" {
			matches++
		}
	}
	if matches > 1 {
		return DFLOPVerificationLedgerResult{Result: "LEDGER_MISMATCH", CorrelationQuality: "NONE"}, nil
	}
	for _, row := range rows {
		result := ReconcileDFLOPVerificationLedger(*item, row, expected)
		if result.CorrelationQuality == "NONE" {
			continue
		}
		// A log summary is insufficient when it omits terminal cost. Fetch exact
		// detail using its provider id, and still validate all identities.
		if result.ProviderCostPoints == "" {
			var fields map[string]any
			_ = common.Unmarshal(row, &fields)
			id := fmt.Sprint(fields["id"])
			if id != "" && verificationSecretSafeID(id) && id != "<nil>" {
				detail, headers, status, err := engine.request(ctx, key, http.MethodGet, "/v1/logs/"+url.PathEscape(id), "", nil)
				if traceID := headers.Get("x-gateway-trace"); len(traceID) <= 128 && verificationSecretSafeID(traceID) && !strings.Contains(traceID, key) {
					evidence.LedgerTraceID = traceID
				}
				if err == nil && status == 200 {
					row = detail
					result = ReconcileDFLOPVerificationLedger(*item, row, expected)
				}
			}
		}
		if result.LedgerID != "" && !strings.Contains(result.LedgerID, key) {
			evidence.LedgerID = result.LedgerID
		}
		evidence.LedgerModel, evidence.LedgerStatus, evidence.LedgerFacts = item.Model, result.ProviderStatus, result.LedgerFacts
		encoded, _ := common.Marshal(evidence)
		item.EvidenceJSON = string(encoded)
		if persistErr := model.UpdateRuntimeVerificationItem(item); persistErr != nil {
			return DFLOPVerificationLedgerResult{}, persistErr
		}
		return result, nil
	}
	return DFLOPVerificationLedgerResult{Result: "INSUFFICIENT_EVIDENCE", CorrelationQuality: "NONE"}, nil
}

func (engine DFLOPVerificationEngine) resolveAmbiguous(ctx context.Context, run *model.RuntimeVerificationRun, item *model.RuntimeVerificationItem, fixture VerificationFixture, key string) error {
	ledger, err := engine.lookupLedger(ctx, item, key, "0")
	if err != nil {
		return err
	}
	if slices.Contains([]string{"error", "rejected", "failed", "cancelled", "expired"}, ledger.ProviderStatus) && ledger.Result == "EXACT_MATCH" {
		item.TerminalStatus, item.GenerationStatus, item.ParserStatus = "failed", "FAIL", "PASS"
		item.LedgerStatus, item.CorrelationQuality, item.ProviderCostPoints = "PASS", ledger.CorrelationQuality, ledger.ProviderCostPoints
		if ledger.ProviderErrorCode == "NO_HEALTHY_DEPLOYMENT" {
			item.Result, item.ReasonCode = "PROVIDER_BLOCKED", "DFLOP_DEPLOYMENT_POOL_UNAVAILABLE"
		}
		if err = model.UpdateRuntimeVerificationItem(item); err != nil {
			return err
		}
		return engine.settle(ctx, run, item, nil, false)
	}
	item.LedgerStatus = "NOT_TESTED"
	return verificationUpdate(item, "RUNTIME_AMBIGUOUS", "EXACT_TERMINAL_EVIDENCE_REQUIRED")
}

func (engine DFLOPVerificationEngine) terminal(ctx context.Context, run *model.RuntimeVerificationRun, item *model.RuntimeVerificationItem, fixture VerificationFixture, plugin *jsplugin.LoadedPlugin, requestBody, body []byte, key string) error {
	replay, err := ReplayDFLOPVerificationTerminal(ctx, fixture, plugin, requestBody, []byte(item.PluginStateJSON), body)
	var evidence model.RuntimeVerificationEvidence
	_ = common.UnmarshalJsonStr(item.EvidenceJSON, &evidence)
	evidence.OutputHash = verificationHash(body)
	evidence.OutputValid = &replay.OutputValid
	item.PluginStateJSON = string(replay.PluginState)
	if err != nil {
		item.GenerationStatus, item.ParserStatus, item.BillingStatus = "BLOCKED", "BLOCKED", "BLOCKED"
		reason := verificationReason(err)
		if replay.ReasonCode != "" {
			reason = replay.ReasonCode
		}
		state, _ := TaskBillingPendingState(replay.PluginState, reason)
		item.PluginStateJSON = string(state)
		encoded, _ := common.Marshal(evidence)
		item.EvidenceJSON = string(encoded)
		return verificationUpdate(item, "CONTRACT_BLOCKED", reason)
	}
	if replay.Status != "SUCCESS" && replay.Status != "FAILURE" {
		return errors.New("TERMINAL_EVIDENCE_REQUIRED")
	}
	item.TerminalStatus = "succeeded"
	item.GenerationStatus, item.ParserStatus = "PASS", "PASS"
	if replay.Status == "FAILURE" {
		item.TerminalStatus, item.GenerationStatus = "failed", "FAIL"
	}
	if replay.Status == "SUCCESS" {
		factsJSON, _ := common.Marshal(replay.NormalizedUsage)
		item.NormalizedUsageJSON = string(factsJSON)
		factsJSON, _ = common.Marshal(replay.ProviderFacts)
		item.ProviderUsageJSON = string(factsJSON)
	}
	encoded, _ := common.Marshal(evidence)
	item.EvidenceJSON = string(encoded)
	if err = model.UpdateRuntimeVerificationItem(item); err != nil {
		return err
	}
	return engine.reconcileTerminal(ctx, run, item, key)
}

// Terminal replay facts were produced and persisted by the production parser.
// Recover the ledger and settlement from these safe facts after a restart.
func (engine DFLOPVerificationEngine) reconcileTerminal(ctx context.Context, run *model.RuntimeVerificationRun, item *model.RuntimeVerificationItem, key string) error {
	var evidence model.RuntimeVerificationEvidence
	if common.UnmarshalJsonStr(item.EvidenceJSON, &evidence) != nil || item.ParserStatus != "PASS" || evidence.OutputValid == nil {
		return errors.New("PRODUCTION_PARSER_EVIDENCE_REQUIRED")
	}
	expected := "0"
	var projection *billingexpr.TieredResult
	if item.TerminalStatus == "succeeded" {
		if item.GenerationStatus != "PASS" || !*evidence.OutputValid || evidence.OutputHash == "" {
			return errors.New("GENERATION_INVALID_OUTPUT")
		}
		var facts map[string]any
		if common.UnmarshalJsonStr(item.NormalizedUsageJSON, &facts) != nil || len(facts) == 0 {
			return errors.New("BILLING_QUANTITY_MISSING")
		}
		var frozen model.RuntimeVerificationProviderSnapshot
		var snapshot billingexpr.BillingSnapshot
		if common.UnmarshalJsonStr(item.FrozenProviderJSON, &frozen) != nil || common.UnmarshalJsonStr(item.BillingSnapshotJSON, &snapshot) != nil {
			return errors.New("FROZEN_PRICING_REQUIRED")
		}
		provider, restoreErr := verificationProviderFromSnapshot(frozen)
		if restoreErr != nil {
			return restoreErr
		}

		points, pointsErr := DFLOPVerificationPoints(provider, facts)
		computed, _, billingErr := EvaluateDFLOPTaskCompletionUsage(&snapshot, facts, []byte(item.PluginStateJSON))
		if pointsErr != nil || billingErr != nil || computed.Clamp != nil {
			item.BillingStatus = "BLOCKED"
			return verificationUpdate(item, "CONTRACT_BLOCKED", "BILLING_QUANTITY_MISSING")
		}
		// Compare the catalog tariff with the frozen production expression using
		// only the documented integer quota conversion, never a tolerance.
		ppc, e1 := decimal.NewFromString(frozen.PointsPerCNY)
		conversion, e2 := decimal.NewFromString(frozen.CNYToUSD)
		markup, e3 := decimal.NewFromString(frozen.Markup)
		if e1 != nil || e2 != nil || e3 != nil || !ppc.IsPositive() {
			return errors.New("FROZEN_CONVERSION_INVALID")
		}
		quota := points.Div(ppc).Mul(conversion).Mul(markup).Mul(decimal.NewFromFloat(snapshot.QuotaPerUnit)).Mul(decimal.NewFromFloat(snapshot.GroupRatio)).Round(0)
		if !quota.Equal(decimal.NewFromInt(int64(computed.ActualQuotaAfterGroup))) {
			item.BillingStatus = "FAIL"
			return verificationUpdate(item, "CONTRACT_BLOCKED", "BILLING_EXPR_MISMATCH")
		}
		expected = points.String()
		projection = &computed
		item.NewapiRawCost = points.Div(ppc).Mul(conversion).Mul(markup).String()
		item.NewapiQuota = &computed.ActualQuotaAfterGroup
	}
	ledger, ledgerErr := engine.lookupLedger(ctx, item, key, expected)
	if ledgerErr != nil {
		item.LedgerStatus = "NOT_TESTED"
		return verificationUpdate(item, "RUNTIME_AMBIGUOUS", "LEDGER_TEMPORARILY_UNAVAILABLE")
	}
	item.CorrelationQuality, item.ProviderCostPoints, item.ProviderUnitCount = ledger.CorrelationQuality, ledger.ProviderCostPoints, ledger.ProviderUnitCount
	if ledger.Result != "EXACT_MATCH" {
		item.LedgerStatus = "BLOCKED"
		return verificationUpdate(item, "RUNTIME_AMBIGUOUS", ledger.Result)
	}
	if item.TerminalStatus == "succeeded" && !slices.Contains([]string{"succeeded", "success", "completed"}, ledger.ProviderStatus) {
		item.LedgerStatus = "BLOCKED"
		return verificationUpdate(item, "RUNTIME_AMBIGUOUS", "PROVIDER_USAGE_MISMATCH")
	}
	item.LedgerStatus = "PASS"
	if err := model.UpdateRuntimeVerificationItem(item); err != nil {
		return err
	}
	return engine.settle(ctx, run, item, projection, *evidence.OutputValid)
}

func (engine DFLOPVerificationEngine) settle(ctx context.Context, run *model.RuntimeVerificationRun, item *model.RuntimeVerificationItem, projection *billingexpr.TieredResult, outputValid bool) error {
	var task model.Task
	if err := model.DB.Where("task_id = ? AND user_id = ?", item.LocalTaskID, run.FundingUserID).First(&task).Error; err != nil {
		return err
	}
	if task.PrivateData.Execution == nil || task.PrivateData.Execution.RequestID != item.LocalRequestID || task.PrivateData.Execution.RuntimeVerificationItemID != item.ID {
		return errors.New("BILLING_JOURNAL_IDENTITY_MISMATCH")
	}
	var actual *int
	task.Status = model.TaskStatusFailure
	if projection != nil {
		actual = &projection.ActualQuotaAfterGroup
		task.Status = model.TaskStatusSuccess
	}
	// Persist terminal facts before the wallet finalizer; its existing journal
	// state machine handles partial database failures and duplicate finalizers.
	task.PrivateData.PluginState = []byte(item.PluginStateJSON)
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	if _, err := task.UpdateWithStatus(model.TaskStatusSubmitted); err != nil {
		return err
	}
	handled, settled := settleDFLOPTaskReservation(ctx, &task, actual, "DFLOP runtime verification exact ledger settlement")
	if !handled || !settled {
		item.BillingStatus = "BLOCKED"
		return verificationUpdate(item, "RUNTIME_AMBIGUOUS", "BILLING_JOURNAL_FINALIZATION_FAILED")
	}
	// Replay uses the same real durable journal, including after restart.
	_, replayOK := settleDFLOPTaskReservation(ctx, &task, actual, "DFLOP runtime verification duplicate terminal replay")
	journal, err := model.FindBillingReservationLog(item.LocalRequestID, run.FundingUserID)
	if err != nil || journal == nil {
		return errors.New("BILLING_JOURNAL_UNAVAILABLE")
	}
	if actual != nil {
		receipt, receiptErr := model.FindRuntimeVerificationConsumeReceipt(run, item, *actual)
		if receiptErr != nil || !receipt {
			item.BillingStatus = "BLOCKED"
			return verificationUpdate(item, "RUNTIME_AMBIGUOUS", "BILLING_CONSUME_RECEIPT_MISSING")
		}
	}
	var evidence model.RuntimeVerificationEvidence
	_ = common.UnmarshalJsonStr(item.EvidenceJSON, &evidence)
	evidence.JournalState = strings.TrimPrefix(journal.Content, "BILLING_RESERVATION_")
	evidence.OutputValid = &outputValid
	evidence.ReplayIdempotent = &replayOK
	evidence.OpenReasons = nil
	item.BillingStatus = "PASS"
	walletDelta := 0
	if actual != nil {
		walletDelta = -*actual
	}
	item.WalletDelta = &walletDelta
	encoded, _ := common.Marshal(evidence)
	item.EvidenceJSON = string(encoded)
	if actual == nil {
		zero := 0
		item.NewapiQuota = &zero
		if item.ReasonCode != "DFLOP_DEPLOYMENT_POOL_UNAVAILABLE" {
			item.Result, item.ReasonCode = "RUNTIME_FAILED", "PROVIDER_TERMINAL_FAILURE"
		}
	}
	if err = model.UpdateRuntimeVerificationItem(item); err != nil {
		return err
	}
	if err = model.SettleRuntimeVerificationProviderCost(run.ID, item.ID, item.ProviderCostPoints); err != nil {
		return err
	}
	if actual != nil {
		return model.FinalizeRuntimeVerificationItem(run.ID, item.ID)
	}
	return nil
}

func verificationProviderFromSnapshot(frozen model.RuntimeVerificationProviderSnapshot) (dflop.Item, error) {
	provider := dflop.Item{ModelID: frozen.Model, EndpointType: frozen.EndpointType, BillingFeatures: frozen.Features, PriceSemantics: dflop.PriceSemantics{SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE"}, Prices: map[string]dflop.Price{}}
	for _, override := range frozen.ContractOverrides {
		reference := "https://model.dflop.top/en/docs/reference/media-apis"
		if override.Model == "minimax-h3" {
			reference = "https://model.dflop.top/models/minimax-h3"
		}
		if override.SourceReferenceHash != verificationHash([]byte(reference)) {
			return provider, errors.New("FROZEN_PROVENANCE_INVALID")
		}
		provider.ContractOverrides = append(provider.ContractOverrides, dflop.ContractOverride{Provider: override.Provider, Model: override.Model, Feature: override.Feature, Source: override.Source, SourceURL: reference, ObservedAt: override.ObservedAt, Version: override.Version, CatalogConflict: override.CatalogConflict, Value: override.Value, AutoApplyAllowed: override.AutoApplyAllowed})
	}
	for rate, value := range frozen.Rates {
		kind := frozen.RateProvenance[rate]
		if kind == "" {
			kind = "AUTHENTICATED_EFFECTIVE_PRICE"
		}
		price := dflop.Price{EffectiveCredits: value, SourcePriceKind: kind, ContractOverrides: provider.ContractOverrides}
		if kind == dflop.DocumentedContractOverride {
			// The only supported frozen override is the verified per-second Lite leg.
			// Restore its contract attributes, then revalidate exact provenance/rates.
			price.Unit = dflop.UnitSecond
			price.PromotionState = "VERIFIED"
		}
		provider.Prices[rate] = price
	}
	provider.Raw, _ = common.Marshal(map[string]any{"free_input_images": frozen.FreeInputImages})
	for key := range provider.Prices {
		if _, err := verificationRate(provider, key); err != nil {
			return provider, err
		}
	}
	return provider, nil
}
