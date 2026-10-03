package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
)

// DFLOPVerificationEngine permits an injected transport for deterministic failure
// tests. Production requests always use the selected key and approved origin.
type DFLOPVerificationEngine struct {
	HTTP           *http.Client
	FixtureOptions VerificationFixtureOptions
}

type verificationSource struct {
	channel                         *model.Channel
	items                           []dflop.Item
	key, hash, fingerprint          string
	config                          model.DFLOPConfig
	metadata                        dflop.EffectiveMetadata
	pointsPerCNY                    string
	catalogTraceID, currencyTraceID string
}

func (engine DFLOPVerificationEngine) source(ctx context.Context, channelID int) (verificationSource, error) {
	var source verificationSource
	config, err := model.GetDFLOPConfig()
	if err != nil {
		return source, err
	}
	if channelID != 1 || config.SourceChannelID != channelID {
		return source, errors.New("SOURCE_CHANNEL_MISMATCH")
	}
	_, key, err := dflop.LoadSourceChannel(channelID)
	if err != nil {
		return source, err
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return source, err
	}
	client := dflop.Client{HTTP: engine.HTTP}
	catalog, err := client.FetchConnectivityCatalog(ctx, key)
	if err != nil {
		return source, errors.New("CONNECTIVITY_CHECK_FAILED")
	}
	currencyResponse, err := client.FetchCurrencyResponse(ctx)
	if err != nil {
		return source, errors.New("CURRENCY_SOURCE_UNAVAILABLE")
	}
	currency := currencyResponse.Body
	var conversion struct {
		PointsPerCNY decimal.Decimal `json:"points_per_cny"`
		Unit         string          `json:"unit"`
	}
	if common.Unmarshal(currency, &conversion) != nil || conversion.Unit != "points" || !conversion.PointsPerCNY.IsPositive() {
		return source, errors.New("CURRENCY_SOURCE_CONFLICT")
	}
	items, hash, metadata, err := dflop.BuildEffective(catalog.Body, currency, config.CNYToUSD, config.MarkupMultiplier)
	if err != nil {
		return source, err
	}
	source = verificationSource{channel: channel, items: items, key: key, hash: hash, fingerprint: verificationHash([]byte(key)), config: config, metadata: metadata, pointsPerCNY: conversion.PointsPerCNY.String()}
	if trace := catalog.TraceID; len(trace) <= 128 && verificationSecretSafeID(trace) && !strings.Contains(trace, key) {
		source.catalogTraceID = trace
	}
	if trace := currencyResponse.TraceID; len(trace) <= 128 && verificationSecretSafeID(trace) && !strings.Contains(trace, key) {
		source.currencyTraceID = trace
	}
	return source, nil
}

// Provider token ceilings must be authoritative; a production reservation
// estimate is not a provider-guaranteed maximum token count.
func verificationMaximumFacts(fixture VerificationFixture, request ProductionVerificationRequest) (map[string]any, error) {
	facts := maps.Clone(request.ReservationFacts)
	if facts == nil {
		return nil, errors.New("BILLING_QUANTITY_MISSING")
	}
	if fixture.Plan.ID == "VIDEO_TOKEN_PLUS_OUTPUT_SECONDS" {
		// The immutable request bounds output; a fixture's provider capability
		// maximum does not enlarge the actual submitted output duration.
		seconds, err := verificationQuantity(fixture.Request, "duration", 30, true)
		bound, bounded := fixture.Bounds["duration"]
		if err != nil || !bounded || seconds.LessThan(decimal.NewFromFloat(bound.Min)) || seconds.GreaterThan(decimal.NewFromFloat(bound.Max)) {
			return nil, errors.New("BILLING_QUANTITY_INVALID: frozen output duration")
		}
		resolution, ok := facts["resolution"].(string)
		mode, modeOK := facts["input_mode"].(string)
		if !ok || !modeOK || mode != "default" && mode != "with_video_input" || fixture.Request["resolution"] != resolution {
			return nil, errors.New("BILLING_QUANTITY_MISSING: selected token tier")
		}
		inputSeconds := decimal.Zero
		if mode == "with_video_input" {
			inputSeconds = decimal.NewFromInt(15)
			if strings.HasPrefix(fixture.Model, "doubao-seedance-2.5") {
				inputSeconds = decimal.NewFromInt(30)
			}
		}
		// DFLOP reserves the largest documented frame even with a requested
		// ratio: references and provider selection can override that ratio.
		tokens, err := DFLOPSeedanceTokenEstimate(fixture.Model, resolution, "adaptive", seconds, inputSeconds)
		if err != nil {
			return nil, err
		}
		facts["completion_tokens"] = tokens.Ceil().IntPart()
		facts["duration_sec"] = seconds.IntPart()
		return facts, nil
	}
	if bound, ok := fixture.Bounds["duration"]; ok {
		seconds, err := verificationQuantity(fixture.Request, "duration", 3600, false)
		if err != nil || seconds.LessThan(decimal.NewFromFloat(bound.Min)) || seconds.GreaterThan(decimal.NewFromFloat(bound.Max)) {
			return nil, errors.New("BILLING_QUANTITY_INVALID: frozen output duration")
		}
		// The exact immutable request, rather than the global SKU capability cap,
		// defines this invocation's output ceiling.
		facts["duration_sec"] = seconds.InexactFloat64()
	}
	if fixture.Plan.ID == "VIDEO_INPUT_PLUS_OUTPUT_SECONDS" && fixture.Mode != "t2v" {
		return nil, errors.New("AUTHORITATIVE_INPUT_DURATION_REQUIRED")
	}
	if fixture.Plan.ID == "OPENAI_IMAGE_PER_OUTPUT" || fixture.Plan.ID == "OPENAI_IMAGE_OUTPUT_PLUS_REFERENCE" {
		if bound, ok := fixture.Bounds["n"]; ok {
			quantity, err := verificationQuantity(fixture.Request, "n", 128, true)
			if err != nil || quantity.LessThan(decimal.NewFromFloat(bound.Min)) || quantity.GreaterThan(decimal.NewFromFloat(bound.Max)) {
				return nil, errors.New("BILLING_QUANTITY_INVALID: frozen image count")
			}
			if fixed, present := fixture.Bounds["output_count"]; present {
				quantity = decimal.Max(quantity, decimal.NewFromFloat(fixed.Max))
			}
			facts["image_count"] = quantity.IntPart()
		}
	}
	return facts, nil
}

func verificationFrozenSnapshot(source verificationSource, fixture VerificationFixture, provider dflop.Item, plugin *jsplugin.LoadedPlugin, group string) (*billingexpr.BillingSnapshot, model.RuntimeVerificationProviderSnapshot, ProductionVerificationRequest, string, error) {
	var frozen model.RuntimeVerificationProviderSnapshot
	if source.channel == nil || !slices.Contains(source.channel.GetModels(), fixture.Model) || !source.channel.BindsTaskPluginForModel(fixture.Plugin, fixture.Model, jsplugin.DefaultRegistry.Generation()) {
		return nil, frozen, ProductionVerificationRequest{}, "", errors.New("CHANNEL_PLUGIN_BINDING_MISMATCH")
	}
	var mapping map[string]string
	if value := source.channel.GetModelMapping(); value != "" {
		if common.UnmarshalJsonStr(value, &mapping) != nil || mapping[fixture.Model] != "" && mapping[fixture.Model] != fixture.Model {
			return nil, frozen, ProductionVerificationRequest{}, "", errors.New("MODEL_MAPPING_DRIFT")
		}
	}
	plan := billing_setting.ResolveTaskBillingPlan(fixture.Plugin, fixture.Model, fixture.Model, plugin, true)
	if !plan.Resolved {
		return nil, frozen, ProductionVerificationRequest{}, "", fmt.Errorf("%s", plan.Reason)
	}
	if provider.TaskExpression == "" || plan.Expression != provider.TaskExpression {
		return nil, frozen, ProductionVerificationRequest{}, "", errors.New("PRICING_SNAPSHOT_DRIFT")
	}
	request, err := ValidateDFLOPVerificationFixture(context.Background(), fixture, plugin)
	if err != nil {
		return nil, frozen, request, "", err
	}
	facts, err := verificationMaximumFacts(fixture, request)
	if err != nil {
		return nil, frozen, request, "", err
	}
	points, err := DFLOPVerificationPoints(provider, facts)
	if err != nil {
		return nil, frozen, request, "", err
	}
	snapshot := &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ModelName: fixture.Model, ExprString: plan.Expression, ExprHash: billingexpr.ExprHashString(plan.Expression), GroupRatio: ratio_setting.GetGroupRatio(group), QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: facts}
	projection, _, err := EvaluateDFLOPTaskCompletionUsage(snapshot, facts, nil)
	if err != nil || projection.Clamp != nil || projection.ActualQuotaAfterGroup <= 0 {
		return nil, frozen, request, "", errors.New("RESERVATION_CALCULATION_FAILED")
	}
	snapshot.EstimatedQuotaBeforeGroup, snapshot.EstimatedQuotaAfterGroup, snapshot.EstimatedTier = projection.ActualQuotaBeforeGroup, projection.ActualQuotaAfterGroup, projection.MatchedTier
	metaJSON, _ := common.Marshal(map[string]any{"meta": plugin.Meta, "source_hash": plugin.Engine.SourceHash()})
	fixtureJSON, _ := common.Marshal(fixture)
	bindingJSON, _ := common.Marshal(map[string]any{"pricing_config": source.config, "channel_type": source.channel.Type, "channel_setting": source.channel.GetSetting(), "model_mapping": mapping})
	frozen = model.RuntimeVerificationProviderSnapshot{Model: provider.ModelID, EndpointType: provider.EndpointType, Features: slices.Clone(provider.BillingFeatures), Rates: map[string]string{}, RateProvenance: map[string]string{}, PointsPerCNY: source.pointsPerCNY, CNYToUSD: source.config.CNYToUSD, Markup: source.config.MarkupMultiplier, ConfigHash: verificationHash(bindingJSON), PluginHash: verificationHash(metaJSON), FixtureHash: verificationHash(fixtureJSON)}
	var raw struct {
		FreeInputImages *int `json:"free_input_images"`
	}
	_ = common.Unmarshal(provider.Raw, &raw)
	frozen.FreeInputImages = raw.FreeInputImages
	for _, override := range provider.ContractOverrides {
		frozen.ContractOverrides = append(frozen.ContractOverrides, model.RuntimeVerificationContractOverride{Provider: override.Provider, Model: override.Model, Feature: override.Feature, Source: override.Source, SourceReferenceHash: verificationHash([]byte(override.SourceURL)), ObservedAt: override.ObservedAt, Version: override.Version, CatalogConflict: override.CatalogConflict, Value: override.Value, AutoApplyAllowed: override.AutoApplyAllowed})
	}
	for key, price := range provider.Prices {
		frozen.RateProvenance[key] = price.SourcePriceKind
		if _, err := verificationRate(provider, key); err != nil {
			return nil, frozen, request, "", err
		}
		value := price.EffectiveCredits
		if value == "" {
			value = price.Credits
		}
		frozen.Rates[key] = value
	}
	return snapshot, frozen, request, points.String(), nil
}

// These original channel-test outcomes need exact upstream evidence before any
// recovery or new paid intent. Catalog visibility alone cannot resolve them.
var DFLOPAmbiguousVerificationModels = []string{
	"grok-3-mini", "grok-3-mini-fast", "grok-4.20-0309-non-reasoning", "grok-4.20-0309-reasoning", "grok-4.20-multi-agent-0309", "grok-4.3", "grok-4.5", "grok-4.6", "grok-4.7", "grok-build-0.1", "grok-composer-2.5-fast", "grok-imagine-image", "grok-imagine-image-quality", "gemini-3.7-flash",
}

func PrepareDFLOPRuntimeVerification(ctx context.Context, channelID int, modelFilter string, userID int) (*model.RuntimeVerificationRun, []model.RuntimeVerificationItem, error) {
	engine, err := NewDFLOPVerificationEngine(ctx, channelID)
	if err != nil {
		return nil, nil, err
	}
	return engine.Prepare(ctx, channelID, modelFilter, userID)
}

// NewDFLOPVerificationEngine loads only server-configured preparation resources.
func NewDFLOPVerificationEngine(ctx context.Context, channelID int) (DFLOPVerificationEngine, error) {
	engine := DFLOPVerificationEngine{}
	if origin := os.Getenv("VERIFICATION_FIXTURE_PUBLIC_BASE_URL"); origin != "" {
		media, err := DFLOPVerificationPublishedMedia(origin)
		if err != nil {
			return engine, err
		}
		engine.FixtureOptions.PublishedMedia = media
		engine.FixtureOptions.VerifyPublicMedia = VerifyDFLOPPublicMedia
	}
	avatar, voice, err := engine.PreparePresetResources(ctx, channelID)
	if err != nil {
		return engine, err
	}
	engine.FixtureOptions.PresetAvatar, engine.FixtureOptions.PresetVoice = avatar.ID, voice.ID
	engine.FixtureOptions.AvatarPresetEvidence, engine.FixtureOptions.VoicePresetEvidence = &avatar, &voice
	if file := os.Getenv("VERIFICATION_CLIP_PREPARATION_FILE"); file != "" {
		data, readErr := os.ReadFile(file)
		var saved struct {
			Prepared VerificationClipPreparation `json:"clip_preparation"`
		}
		media, published := engine.FixtureOptions.PublishedMedia["speaking-square-v1"]
		if readErr == nil && len(data) <= 2<<20 && common.Unmarshal(data, &saved) == nil && published && saved.Prepared.SourceURL == media.PublicURL && saved.Prepared.SourceSHA256 == media.SHA256 {
			p := saved.Prepared
			verifier := engine.ClipPreparationVerifier(channelID, p)
			if verifier(ctx, media.PublicURL, p.ASRID) == nil {
				engine.FixtureOptions.ClipPreparation = &p
				engine.FixtureOptions.ClipASRID = p.ASRID
				engine.FixtureOptions.ClipSourceVideoURL = p.SourceURL
				engine.FixtureOptions.ClipStyleID = p.StyleID
				engine.FixtureOptions.VerifyClipSource = verifier
			}
		}
	}
	return engine, nil
}

// Prepare performs GETs and production hook replay only. No funding operation
// or paid submission is reachable from this API, even with caller-supplied data.
func (engine DFLOPVerificationEngine) Prepare(ctx context.Context, channelID int, modelFilter string, userID int) (*model.RuntimeVerificationRun, []model.RuntimeVerificationItem, error) {
	source, err := engine.source(ctx, channelID)
	if err != nil {
		return nil, nil, err
	}
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		return nil, nil, err
	}
	options := engine.FixtureOptions
	options.SourceCatalogHash = source.hash
	fixtures := DFLOPVerificationFixturesWithOptions(source.items, options)
	if modelFilter != "" && !slices.Contains(DFLOPAmbiguousVerificationModels, modelFilter) && !slices.ContainsFunc(fixtures, func(f VerificationFixture) bool { return f.Model == modelFilter }) {
		return nil, nil, errors.New("MODEL_NOT_IN_VERIFICATION_REGISTRY")
	}
	providerItems := make(map[string]dflop.Item, len(source.items))
	for _, item := range source.items {
		providerItems[item.ModelID] = item
	}
	rows := make([]model.RuntimeVerificationItem, 0, len(fixtures))
	for _, fixture := range fixtures {
		if modelFilter != "" && fixture.Model != modelFilter {
			continue
		}
		fixture.SourceCatalogHash = source.hash
		row := model.RuntimeVerificationItem{Model: fixture.Model, Protocol: fixture.Protocol, Operation: fixture.Operation, Mode: fixture.Mode, FixtureID: fixture.ID, Endpoint: fixture.Endpoint, CatalogHash: source.hash, ConfigStatus: "PASS", ConnectivityStatus: "PASS", Result: "CONNECTIVITY_VERIFIED", ReasonCode: "LIVE_CANARY_REQUIRED"}
		evidence := model.RuntimeVerificationEvidence{DocsHash: verificationHash(DFLOPVerificationDocumentationSnapshot()), RequiredFacts: fixture.Plan.RequiredUsageFacts, ContractPlan: fixture.Plan.ID, CatalogSchemaVersion: source.metadata.SchemaVersion, Currency: source.metadata.Currency, OpenReasons: map[string]string{"request": "PAID_AUTHORIZATION_REQUIRED", "generation": "LIVE_CANARY_REQUIRED", "usage": "NOT_TESTED", "billing": "NOT_TESTED", "ledger": "NOT_TESTED"}}
		provider, visible := providerItems[fixture.Model]
		fixtureJSON, _ := common.Marshal(fixture)
		evidence.FixtureHash = verificationHash(fixtureJSON)
		if visible {
			providerJSON, _ := common.Marshal(provider)
			row.PricingSnapshotHash = verificationHash(providerJSON)
		}
		evidence.CatalogTraceID, evidence.CurrencyTraceID = source.catalogTraceID, source.currencyTraceID
		callable := visible && provider.Callable
		evidence.Callable = &callable
		if !callable {
			row.ConfigStatus, row.ConnectivityStatus, row.Result, row.ReasonCode = "BLOCKED", "BLOCKED", "PROVIDER_BLOCKED", "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG"
		}
		plugin, loaded := jsplugin.DefaultRegistry.Generation().Get(fixture.Plugin)
		if !loaded {
			row.ConfigStatus, row.Result, row.ReasonCode = "BLOCKED", "CONTRACT_BLOCKED", "PLUGIN_BINDING_MISMATCH"
		}
		if loaded && callable {
			plan := billing_setting.ResolveTaskBillingPlan(fixture.Plugin, fixture.Model, fixture.Model, plugin, true)
			row.BillingSource = plan.BillingSource
			if plan.Resolved {
				row.BillingExprHash = billingexpr.ExprHashString(plan.Expression)
			}
			if !plan.Resolved {
				row.ConfigStatus, row.Result, row.ReasonCode = "BLOCKED", "CONTRACT_BLOCKED", plan.Reason
			}
			snap, frozen, _, _, freezeErr := verificationFrozenSnapshot(source, fixture, provider, plugin, group)
			if freezeErr != nil {
				row.ReasonCode = verificationReason(freezeErr)
				row.RequestStatus = "BLOCKED"
				evidence.OpenReasons["request"] = row.ReasonCode
				if strings.Contains(row.ReasonCode, "PRICING") || strings.Contains(row.ReasonCode, "BILLING") || strings.Contains(row.ReasonCode, "TOKEN_CEILING") || strings.Contains(row.ReasonCode, "CONFLICT") {
					row.BillingStatus = "BLOCKED"
					evidence.OpenReasons["billing"] = row.ReasonCode
				}
				if strings.Contains(row.ReasonCode, "CONFLICT") || strings.Contains(row.ReasonCode, "DRIFT") {
					row.Result = "CONTRACT_BLOCKED"
				}
			} else {
				body, _ := common.Marshal(snap)
				row.RequestStatus = "PASS"
				evidence.OpenReasons["request"] = "PAID_AUTHORIZATION_REQUIRED"
				row.BillingSnapshotJSON = string(body)
				row.BillingExprHash = snap.ExprHash
				body, _ = common.Marshal(frozen)
				row.FrozenProviderJSON = string(body)
				row.PricingSnapshotHash = verificationHash(body)
				evidence.FixtureHash = frozen.FixtureHash
			}
		}
		evidenceJSON, _ := common.Marshal(evidence)
		var boundEvidence map[string]any
		_ = common.Unmarshal(evidenceJSON, &boundEvidence)
		configJSON, _ := common.Marshal(map[string]any{"pricing_config": source.config, "channel_type": source.channel.Type, "channel_setting": source.channel.GetSetting(), "model_mapping": source.channel.GetModelMapping()})
		boundEvidence["config_hash"] = verificationHash(configJSON)
		audit := AuditDFLOPVerificationPlanContract(fixture, provider)
		boundEvidence["warning_codes"] = audit.Warnings
		evidenceJSON, _ = common.Marshal(boundEvidence)
		row.EvidenceJSON = string(evidenceJSON)
		rows = append(rows, row)
	}
	previousRun, previousItems, historyErr := model.LatestRuntimeVerificationEvidence(channelID)
	if historyErr != nil {
		return nil, nil, historyErr
	}
	for _, name := range DFLOPAmbiguousVerificationModels {
		provider, visible := providerItems[name]
		if !visible || modelFilter != "" && modelFilter != name {
			continue
		}
		configJSON, _ := common.Marshal(map[string]any{"pricing_config": source.config, "channel_setting": source.channel.GetSetting(), "model_mapping": source.channel.GetModelMapping(), "catalog_hash": source.hash})
		target := DFLOPFreshCanaryTarget(provider, source.hash, verificationHash(configJSON))
		row := model.RuntimeVerificationItem{Model: name, Protocol: target.Fixture.Protocol, Operation: target.Fixture.Operation, Mode: target.Fixture.Mode, FixtureID: target.Fixture.ID, Endpoint: target.Fixture.Endpoint, CatalogHash: source.hash, PricingSnapshotHash: target.PricingSnapshotHash, BillingExprHash: target.BillingExprHash, ConfigStatus: "PASS", ConnectivityStatus: "PASS", RequestStatus: "PASS", GenerationStatus: "NOT_TESTED", Result: "READY_FOR_FRESH_CANARY", ReasonCode: "LIVE_CANARY_REQUIRED", CorrelationQuality: "NONE"}
		evidence := model.RuntimeVerificationEvidence{ConfigHash: target.ConfigHash, FixtureHash: target.FixtureHash, CatalogSchemaVersion: source.metadata.SchemaVersion, Currency: source.metadata.Currency, HistoricalRuntimeState: "HISTORICAL_RUNTIME_UNRECOVERABLE", HistoricalReasonCode: "EXACT_PROVIDER_ID_NOT_CAPTURED", CurrentCanaryReadiness: "READY_FOR_FRESH_CANARY", RequiredFacts: target.Fixture.Plan.RequiredUsageFacts, OpenReasons: map[string]string{"generation": "LIVE_CANARY_REQUIRED", "usage": "NOT_TESTED", "billing": "NOT_TESTED", "ledger": "NOT_TESTED"}}
		if previousRun != nil {
			evidence.HistoricalRunID = previousRun.ID
			for _, previous := range previousItems {
				if previous.Model != name {
					continue
				}
				var prior model.RuntimeVerificationEvidence
				if common.UnmarshalJsonStr(previous.EvidenceJSON, &prior) == nil && prior.HistoricalRunID > 0 {
					evidence.HistoricalRunID = prior.HistoricalRunID
				}
			}
		}
		if target.Blocker != "" {
			row.RequestStatus, row.Result, row.ReasonCode = "BLOCKED", "CONTRACT_BLOCKED", target.Blocker
			evidence.CurrentCanaryReadiness = "BLOCKED_BY_PROVIDER_CONTRACT"
		}
		if !provider.Callable {
			row.ConnectivityStatus = "BLOCKED"
		}
		evidenceJSON, _ := common.Marshal(evidence)
		row.EvidenceJSON = string(evidenceJSON)
		rows = append(rows, row)
	}

	run := &model.RuntimeVerificationRun{ChannelID: channelID, Source: "DFLOP_AUTHENTICATED", CatalogHash: source.hash, CredentialFingerprint: source.fingerprint, Mode: "ZERO_COST", Status: "CONNECTIVITY_VERIFIED", FundingUserID: userID}
	if modelFilter != "" {
		run.Mode = "ZERO_COST_SINGLE"
	}
	if err := model.CreateRuntimeVerificationRun(run, rows); err != nil {
		return nil, nil, err
	}
	items, err := model.ListRuntimeVerificationItems(run.ID)
	return run, items, err
}

func verificationReason(err error) string {
	reason, _, _ := strings.Cut(err.Error(), ":")
	if reason == "" || len(reason) > 128 || strings.ContainsAny(reason, " \r\n") {
		return "OFFLINE_ACCEPTANCE_FAILED"
	}
	return reason
}

// Run changes are confined to a new run. Existing zero-cost evidence cannot be
// converted into approval by an API update or a historical manifest.
func (engine DFLOPVerificationEngine) CreateAuthorizedRun(ctx context.Context, channelID, userID int, authorization DFLOPVerificationAuthorization) (*model.RuntimeVerificationRun, []model.RuntimeVerificationItem, error) {
	if !authorization.Approved || authorization.ExpiresAt <= time.Now().Unix() || authorization.ApprovedBy == "" || authorization.ApprovalReference == "" {
		return nil, nil, errors.New("PAID_AUTHORIZATION_REQUIRED")
	}
	if common.BatchUpdateEnabled {
		return nil, nil, errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED")
	}
	if !common.LogConsumeEnabled || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, nil, errors.New("BILLING_CONSUME_RECEIPT_UNAVAILABLE")
	}
	prepared, rows, err := engine.Prepare(ctx, channelID, "", userID)
	if err != nil {
		return nil, nil, err
	}
	manifest, _ := common.Marshal(authorization)
	run := &model.RuntimeVerificationRun{ChannelID: channelID, Source: prepared.Source, CatalogHash: prepared.CatalogHash, CredentialFingerprint: prepared.CredentialFingerprint, Mode: "AUTHORIZED_CANARY", AuthorizationManifestHash: verificationHash(manifest), TotalBudgetLimit: authorization.MaxTotalProviderPoints, MaxRequests: authorization.MaxRequests, FundingUserID: userID}
	if authorization.ChannelID != run.ChannelID || authorization.CatalogHash != run.CatalogHash || authorization.CredentialFingerprint != run.CredentialFingerprint || authorization.FundingUserID != userID || userID <= 0 {
		return nil, nil, errors.New("DRIFT_REVIEW_REQUIRED")
	}
	selected := make([]model.RuntimeVerificationItem, 0, len(authorization.Targets))
	for _, row := range rows {
		if !slices.Contains(authorization.Models, row.Model) || !slices.ContainsFunc(authorization.Targets, func(target DFLOPVerificationTarget) bool {
			return target.Model == row.Model && target.Protocol == row.Protocol && target.Mode == row.Mode && target.FixtureID == row.FixtureID
		}) {
			continue
		}
		row.ID, row.RunID, row.CreatedAt, row.UpdatedAt = 0, 0, 0, 0
		selected = append(selected, row)
	}
	if len(selected) == 0 {
		return nil, nil, errors.New("PAID_TARGET_NOT_AUTHORIZED")
	}
	if err = model.CreateRuntimeVerificationRun(run, selected); err != nil {
		return nil, nil, err
	}
	items, err := model.ListRuntimeVerificationItems(run.ID)
	return run, items, err
}
