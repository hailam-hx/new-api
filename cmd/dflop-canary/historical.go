package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	_ "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/shopspring/decimal"
)

func runHistoricalRecovery(channelID int, config model.DFLOPConfig, options dflop.HistoricalOptions, output string) error {
	source, key, err := dflop.LoadSourceChannel(channelID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := dflop.Client{}
	catalog, err := client.FetchEffective(ctx, key, "")
	if err != nil {
		return err
	}
	publicCatalog, err := client.FetchPublic(ctx)
	if err != nil {
		return err
	}
	currency, err := client.FetchCurrency(ctx)
	if err != nil {
		return err
	}
	// Points-only discovery requires no exchange rate. This transient fallback
	// is never persisted and must not be used to replay USD settlement.
	actualRate := config.CNYToUSD
	if config.CNYToUSD == "" {
		config.CNYToUSD = "1"
	}
	items, hash, metadata, err := dflop.BuildEffective(catalog.Body, currency, config.CNYToUSD, "1")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.Callable {
			names = append(names, item.ModelID)
		}
	}
	snapshot, err := model.GetModelPricingSnapshot(names)
	if err != nil {
		return err
	}
	managed, err := model.GetPricingSyncManaged(names)
	if err != nil {
		return err
	}
	planned, err := dflop.Plan(items, config, snapshot.Entries, managed)
	if err != nil {
		return err
	}
	coverage := map[string]int{}
	for i := range items {
		for _, proposal := range planned {
			if proposal.ModelID != items[i].ModelID {
				continue
			}
			items[i].ReasonCode, items[i].Status = proposal.ReasonCode, proposal.Status
			if items[i].Callable {
				coverage[proposal.Status]++
			}
			break
		}
	}
	plan, err := dflop.BuildCanaryPlan(channelID, catalog.Body, currency, catalog.ETag, config.CNYToUSD)
	if err != nil {
		return err
	}
	options.CatalogHash = hash
	options.Replay = func(ctx context.Context, item dflop.Item, raw json.RawMessage) (dflop.HistoricalReplay, error) {
		return replayHistoricalTask(ctx, item, raw, plan, actualRate)
	}
	report, err := dflop.RecoverHistoricalEvidence(ctx, dflop.HistoricalTransport{BaseURL: source.BaseURL, Key: key}, options, items)
	if err != nil {
		return err
	}
	report.CoverageBefore, report.CoverageAfter = coverage, coverage
	report.SourceSnapshot = map[string]any{"catalog_hash": hash, "schema_version": metadata.SchemaVersion, "etag": catalog.ETag, "callable_count": len(names), "currency_snapshot": currency, "public_catalog_hash": fmt.Sprintf("%x", sha256.Sum256(publicCatalog.Body)), "public_catalog_sets_prices": false}
	report.EndpointMatrix = dflop.EndpointBillingMatrix(items)
	correlations, localStatus, err := correlateLocalHistory(&report, channelID)
	if err != nil {
		return err
	}
	report.LocalCorrelationStatus = localStatus
	for i := range report.Models {
		report.Models[i].RuntimeUsageVerified = report.Models[i].QuantityVerified
	}
	correlationBody, err := common.Marshal(map[string]any{"mode": "GET_ONLY_LOCAL_CORRELATION", "scope": options.Scope, "catalog_hash": hash, "local_status": localStatus, "tasks": correlations, "paid_requests_executed": false})
	if err != nil {
		return err
	}
	correlationBody, err = dflop.RedactCanaryJSON(correlationBody, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	directory, err := os.Stat(filepath.Dir(output))
	if err != nil || directory.Mode().Perm()&0077 != 0 {
		return errors.New("evidence directory must be private (0700)")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(output), "dflop-runtime-binding-correlation-2026-09-30.json"), correlationBody, 0600); err != nil {
		return err
	}
	encoded, err := common.Marshal(report)
	if err != nil {
		return err
	}
	// Reapply redaction to callback output as well as captured provider bodies.
	encoded, err = dflop.RedactCanaryJSON(encoded, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(output), ".dflop-evidence-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(file.Name(), output); err != nil {
		return err
	}
	escalationBody, err := common.Marshal(report.Escalations)
	if err != nil {
		return err
	}
	escalationPath := filepath.Join(filepath.Dir(output), "dflop-provider-escalations-2026-09-30.json")
	if err := os.WriteFile(escalationPath, escalationBody, 0600); err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}

func replayHistoricalTask(ctx context.Context, item dflop.Item, raw json.RawMessage, plan dflop.CanaryPlan, rate string) (dflop.HistoricalReplay, error) {
	result := dflop.HistoricalReplay{Note: "Historical upstream response alone does not verify New API route, durable settlement, or historical price snapshot"}
	if strings.Contains(item.ModelID, "grok") {
		for _, c := range plan.Cases {
			if c.Model != item.ModelID || !strings.HasPrefix(c.ID, "grok-tool-") {
				continue
			}
			replay, err := service.ReplayCanaryFixture(c, raw, plan.PointsPerCNY, rate)
			if err == nil && replay.Status == "RUNTIME_FACT_VERIFIED" {
				result.Parser = "canonical chat/Responses billing usage"
				result.QuantityVerified, result.SettlementReplayed, result.Result = true, true, replay
				return result, nil
			}
		}
		return result, nil
	}
	if item.PricingShape == "image:per_image" || strings.Contains(item.ModelID, "midjourney") {
		// Exercise the production OpenAI image response extractor on the original
		// terminal JSON. URLs are inspected for presence, never fetched.
		count := 1
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}, TieredBillingSnapshot: &billingexpr.BillingSnapshot{EstimatedImageCount: &count}}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/historical-replay", nil)
		response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}
		_, parseErr := openai.OpenaiImageHandler(c, info, response)
		result.Parser = "OpenaiImageHandler/UpdateImageCount"
		if parseErr != nil || info.BillingImageCount == nil {
			result.Note += "; terminal payload does not expose production image quantity"
			return result, nil
		}
		result.QuantityVerified = true
		points, pointsErr := decimal.NewFromString(plan.PointsPerCNY)
		usd, usdErr := decimal.NewFromString(rate)
		price, priceErr := decimal.NewFromString(item.Prices["price_per_image"].Credits)
		if pointsErr != nil || usdErr != nil || priceErr != nil || !points.IsPositive() || !usd.IsPositive() {
			return result, nil
		}
		expr := `tier("historical", fixed(` + price.DivRound(points, 24).Mul(usd).String() + `)) * image_count`
		snapshot := &billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1}
		settled, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, billingexpr.RequestInput{ImageCount: info.BillingImageCount})
		if err != nil {
			return result, errors.New("PRODUCTION_IMAGE_SETTLEMENT_REPLAY_FAILED")
		}
		result.SettlementReplayed = true
		result.Result = map[string]any{"image_count": *info.BillingImageCount, "quota": settled.ActualQuotaAfterGroup, "matched_tier": settled.MatchedTier}
		return result, nil
	}
	if item.ModelID != "voice-tts-pro" {
		return result, nil
	}
	plugin, ok := jsplugin.DefaultRegistry.Get("dflop-tts")
	if !ok {
		return result, errors.New("PRODUCTION_TTS_PLUGIN_UNAVAILABLE")
	}
	var payload struct {
		ID     string `json:"id"`
		Model  string `json:"model"`
		Status string `json:"status"`
	}
	if err := common.Unmarshal(raw, &payload); err != nil {
		return result, errors.New("HISTORICAL_TASK_INVALID")
	}
	if payload.ID == "" || payload.Model != item.ModelID || payload.Status != "succeeded" {
		return result, nil
	}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{})
	task := &model.Task{Properties: model.Properties{OriginModelName: item.ModelID, UpstreamModelName: item.ModelID}, PrivateData: model.TaskPrivateData{UpstreamTaskID: payload.ID}}
	parsed, err := adaptor.ParseTaskResult(task, nil, raw)
	if err != nil || parsed.Status != string(model.TaskStatusSuccess) || parsed.UsageFacts["characters"] == nil {
		return result, errors.New("PRODUCTION_TTS_USAGE_REJECTED")
	}
	result.Parser, result.QuantityVerified = "TaskAdaptor.ParseTaskResult/extractUsageOnComplete", true
	points, err := decimal.NewFromString(plan.PointsPerCNY)
	usd, rateErr := decimal.NewFromString(rate)
	price, priceErr := decimal.NewFromString(item.Prices["price_per_tts_char"].Credits)
	if err != nil || rateErr != nil || priceErr != nil || !points.IsPositive() || !usd.IsPositive() {
		result.Note += "; currency unavailable for USD replay"
		return result, nil
	}
	expr := `tier("historical", u("characters") * ` + price.DivRound(points, 24).Mul(usd).String() + `)`
	snap := &billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1, TaskUsageBilling: true}
	settled, facts, err := service.EvaluateTaskCompletionUsage(snap, parsed.UsageFacts)
	if err != nil {
		return result, errors.New("PRODUCTION_TTS_SETTLEMENT_REPLAY_FAILED")
	}
	snap.UsageFacts, snap.EstimatedTier = facts, settled.MatchedTier
	other := model.NewLogOther()
	service.AppendTaskExpressionLogInfo(other, snap)
	result.SettlementReplayed = true
	result.Result = map[string]any{"usage_facts": facts, "quota": settled.ActualQuotaAfterGroup, "consume_log_other": other.Snapshot()}
	return result, ctx.Err()
}

func correlateLocalHistory(report *dflop.HistoricalReport, channelID int) ([]dflop.RuntimeBinding, string, error) {
	records := []dflop.LocalExecution{}
	if model.DB.Migrator().HasTable(&model.Task{}) {
		for offset := 0; ; offset += 200 {
			var tasks []model.Task
			if err := model.DB.Where("channel_id = ?", channelID).Order("id").Offset(offset).Limit(200).Find(&tasks).Error; err != nil {
				return nil, "LOCAL_TASK_QUERY_FAILED", err
			}
			for _, task := range tasks {
				row := dflop.LocalExecution{TaskRecordID: task.ID, TaskID: task.TaskID, UpstreamTaskID: task.PrivateData.UpstreamTaskID, ChannelID: task.ChannelId, ClientModel: task.Properties.OriginModelName, UpstreamModel: task.Properties.UpstreamModelName}
				if task.PrivateData.Execution != nil {
					row.RequestID = task.PrivateData.Execution.RequestID
					if passive := task.PrivateData.Execution.Passive; passive != nil {
						row.UpstreamRequestID, row.TraceID = passive.RequestID, passive.TraceID
						if passive.TaskID == task.PrivateData.UpstreamTaskID && passive.ChannelID == task.ChannelId && passive.UpstreamModel == task.Properties.UpstreamModelName {
							found := false
							for _, capture := range report.Captures {
								if capture.EvidenceID == "task-poll:"+passive.TaskID {
									found = true
									break
								}
							}
							if !found && passive.Terminal != nil {
								payload := map[string]any{"host_status": passive.HostStatus, "validated_usage": passive.ValidatedUsage, "request_id": passive.RequestID, "task_id": passive.TaskID}
								for key, value := range passive.Terminal {
									payload[key] = value
								}
								body, err := common.Marshal(payload)
								if err != nil {
									return nil, "LOCAL_PASSIVE_ENCODING_FAILED", err
								}
								var replay *dflop.HistoricalReplay
								if report.Options.Replay != nil {
									for _, item := range report.Models {
										if item.Model == row.UpstreamModel {
											result, err := report.Options.Replay(context.Background(), dflop.Item{ModelID: item.Model, BillingFeatures: item.BillingFeatures, Prices: item.CatalogPrices}, task.Data)
											if err != nil {
												result.Note = err.Error()
											}
											replay = &result
											break
										}
									}
								}
								report.Captures = append(report.Captures, dflop.HistoricalCapture{EvidenceID: "task-poll:" + passive.TaskID, EvidenceClass: "CAPTURED_LOCAL_PASSIVE", Model: row.UpstreamModel, GatewayTrace: passive.TraceID, Response: body, Replay: replay, CatalogHash: report.Options.CatalogHash})
							}
						}
					}
					if task.PrivateData.Execution.TaskPlugin != nil {
						row.PluginKey = task.PrivateData.Execution.TaskPlugin.Key
					}
				}
				records = append(records, row)
			}
			if len(tasks) < 200 {
				break
			}
		}
	}
	logDB := model.DB
	status := "LOCAL_RECORDS_READ"
	if dsn := os.Getenv("LOG_SQL_DSN"); dsn != "" {
		var err error
		logDB, err = openCanaryDBWithDSN("", dsn)
		if err != nil {
			logDB = nil
			status = "SEPARATE_LOG_DB_UNAVAILABLE"
		}
	}
	if logDB != nil && logDB.Migrator().HasTable(&model.Log{}) {
		for offset := 0; ; offset += 200 {
			var logs []model.Log
			if err := logDB.Select("id, channel_id, model_name, request_id, upstream_request_id, other").Where(map[string]any{"channel_id": channelID}).Order("id").Offset(offset).Limit(200).Find(&logs).Error; err != nil {
				return nil, "LOCAL_USAGE_QUERY_FAILED", err
			}
			for _, log := range logs {
				var other struct {
					UpstreamModel string `json:"upstream_model_name"`
					TaskID        string `json:"task_id"`
					PluginKey     string `json:"task_plugin_key"`
					Root          struct {
						Passive *model.RuntimeEvidence `json:"passive_runtime_binding"`
					} `json:"root_info"`
				}
				_ = common.UnmarshalJsonStr(log.Other, &other)
				record := dflop.LocalExecution{LogRecordID: log.Id, RequestID: log.RequestId, UpstreamRequestID: log.UpstreamRequestId, TaskID: other.TaskID, PluginKey: other.PluginKey, ClientModel: log.ModelName, UpstreamModel: other.UpstreamModel, ChannelID: log.ChannelId}
				if passive := other.Root.Passive; passive != nil && passive.ChannelID == log.ChannelId && passive.ClientModel == log.ModelName {
					record.UpstreamModel, record.PluginKey, record.UpstreamTaskID = passive.UpstreamModel, passive.PluginKey, passive.TaskID
					record.UpstreamRequestID, record.TraceID = passive.RequestID, passive.TraceID
					if passive.TaskID == "" {
						payload := map[string]any{"request_id": passive.RequestID, "task_id": passive.TaskID}
						for key, value := range passive.Terminal {
							payload[key] = value
						}
						body, err := common.Marshal(payload)
						if err != nil {
							return nil, status, err
						}
						report.Captures = append(report.Captures, dflop.HistoricalCapture{EvidenceID: fmt.Sprintf("local-log:%d", log.Id), EvidenceClass: "CAPTURED_LOCAL_PASSIVE", Model: passive.UpstreamModel, GatewayTrace: passive.TraceID, Response: body})
					}
				}
				records = append(records, record)
			}
			if len(logs) < 200 {
				break
			}
		}
	}
	if len(records) == 0 && status == "LOCAL_RECORDS_READ" {
		status = "NO_LOCAL_TASK_OR_USAGE_RECORDS"
	}
	requests := map[string]string{}
	for _, capture := range report.Captures {
		var row struct {
			TaskID    string `json:"task_id"`
			RequestID string `json:"request_id"`
		}
		if common.Unmarshal(capture.Response, &row) == nil && row.TaskID != "" {
			requests[row.TaskID] = row.RequestID
		}
	}
	correlations := []dflop.RuntimeBinding{}
	for _, capture := range report.Captures {
		localLog := strings.HasPrefix(capture.EvidenceID, "local-log:")
		taskPoll := strings.HasPrefix(capture.EvidenceID, "task-poll:")
		providerLog := strings.HasPrefix(capture.EvidenceID, "log:") || strings.HasPrefix(capture.EvidenceID, "technical:")
		if !taskPoll && !localLog && !providerLog {
			continue
		}
		id := strings.TrimPrefix(capture.EvidenceID, "task-poll:")
		var row map[string]any
		if err := common.Unmarshal(capture.Response, &row); err != nil {
			return nil, status, err
		}
		requestID := requests[id]
		if localLog || providerLog {
			id, _ = row["task_id"].(string)
			requestID, _ = row["request_id"].(string)
		}
		if taskPoll {
			returnedID, _ := row["id"].(string)
			returnedTaskID, _ := row["task_id"].(string)
			if returnedID == "" && returnedTaskID == "" || returnedID != "" && returnedID != id || returnedTaskID != "" && returnedTaskID != id {
				continue
			}
		}
		binding := dflop.CorrelateRuntimeBinding(id, requestID, capture.Model, channelID, records, capture.GatewayTrace)
		if returned, ok := row["model"].(string); ok && returned != "" && returned != capture.Model {
			binding.BindingVerified = false
			binding.Confidence = "UNVERIFIED"
		}
		binding.RuntimeFields = map[string]any{}
		for _, key := range []string{"status", "characters", "duration_sec", "resolution", "service_tier", "usage", "input_video_duration_sec", "input_video_present", "server_tool_calls", "tool_outcomes", "validated_usage", "host_status"} {
			if value, ok := row[key]; ok {
				binding.RuntimeFields[key] = value
			}
		}
		if usage, ok := row["usage"].(map[string]any); ok {
			binding.RuntimeFields["completion_tokens"] = usage["completion_tokens"]
		}
		for _, item := range report.Models {
			if item.Model == capture.Model {
				binding.BillingFeatures = item.BillingFeatures
				facts := dflop.HistoricalModelReport{ReconciliationResult: "INSUFFICIENT_EVIDENCE"}
				dflop.ObserveHistoricalBilling(&facts, dflop.Item{BillingFeatures: item.BillingFeatures, Prices: item.CatalogPrices}, row, nil)
				binding.RuntimeUsageVerified = facts.QuantityVerified || (capture.Replay != nil && capture.Replay.QuantityVerified)
				binding.SettledCostReconciled = facts.ReconciliationResult == "MATCH" || facts.ReconciliationResult == "ROUNDING_MATCH"
				for i := range report.Models {
					if report.Models[i].Model == capture.Model {
						report.Models[i].BindingVerified = report.Models[i].BindingVerified || binding.BindingVerified
						report.Models[i].QuantityVerified = report.Models[i].QuantityVerified || binding.RuntimeUsageVerified
					}
				}
				break
			}
		}
		// Scope-dependent cost availability cannot downgrade an exact execution ID
		// or a validated upstream quantity. It remains a separate settlement gate.
		correlations = append(correlations, binding)
	}
	return correlations, status, nil
}
