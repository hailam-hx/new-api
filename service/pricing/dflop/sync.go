package dflop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/shopspring/decimal"
)

func Plan(items []Item, config model.DFLOPConfig, entries []model.ModelPricingEntry, managed map[string]model.PricingSyncManaged) ([]model.PricingSyncItem, error) {
	current := make(map[string]model.ModelPricingEntry, len(entries))
	for _, entry := range entries {
		current[entry.ModelName] = entry
	}
	result := make([]model.PricingSyncItem, 0, len(items))
	for _, item := range items {
		if item.ReasonCode == "NO_PLUGIN_USAGE_PROFILE" {
			item.ReasonCode = missingTaskBindingReason(item)
		}
		if item.Status == SupportedAuto && item.PricingShape == "image:per_image" && mediaPricesMatch(item, "price_per_image") {
			for _, plugin := range jsplugin.DefaultRegistry.Generation().PluginsByModel(item.ModelID) {
				if plugin.Meta.Key == "doubao" && slices.Contains(plugin.Meta.Models, item.ModelID) {
					item.TaskPlugin = plugin.Meta.Key
					item.RequiredFacts = []string{"image_count"}
					item.TaskExpression = fmt.Sprintf("tier(\"image\", u(\"image_count\") * %s)", item.Prices["price_per_image"].SellingUSD)
					break
				}
			}
		}
		entry, found := current[item.ModelID]
		if !found {
			entry = model.ModelPricingEntry{ModelName: item.ModelID, Configured: model.PricingValues{}, Version: model.ModelPricingVersion(model.PricingValues{})}
		}
		proposed := maps.Clone(entry.Configured)
		if proposed == nil {
			proposed = model.PricingValues{}
		}
		status, reason, reasonCode := item.Status, item.Reason, item.ReasonCode
		pricingScope, pluginKey := "MODEL", ""
		var available, missing []string
		if item.TaskExpression != "" {
			pluginKey = item.TaskPlugin
			pricingScope = "PLUGIN_OVERRIDE"
			var schema map[string]jsplugin.UsageFieldSchema
			for _, variant := range entry.PluginVariants {
				if variant.PluginKey == pluginKey && !variant.Stale {
					schema = variant.UsageSchema
					break
				}
			}
			if schema == nil {
				for _, plugin := range jsplugin.DefaultRegistry.Generation().PluginsByModel(item.ModelID) {
					if plugin.Meta.Key == pluginKey && slices.Contains(plugin.Meta.Models, item.ModelID) {
						schema, _ = plugin.Meta.UsageForModel(item.ModelID)
						break
					}
				}
			}
			if schema == nil {
				reasonCode, reason = missingTaskBindingReason(item), "no exact task plugin binding with a usage profile"
				missing = slices.Clone(item.RequiredFacts)
			} else {
				available, missing, reasonCode = taskPricingCompatibility(item, schema)
				if reasonCode == "" {
					status, reason = SupportedAuto, ""
					variants := map[string]any{}
					if previous, ok := proposed[billing_setting.PluginBillingExprOption].(map[string]any); ok {
						maps.Copy(variants, previous)
					}
					variants[pluginKey] = item.TaskExpression
					proposed[billing_setting.PluginBillingExprOption] = variants
				} else {
					reason = "task plugin usage facts do not match DFLOP pricing requirements"
				}
			}
		} else if item.Expression != "" {
			proposed["billing_setting.billing_mode"] = "tiered_expr"
			proposed["billing_setting.billing_expr"] = item.Expression
		}
		currentJSON, err := common.Marshal(entry.Configured)
		if err != nil {
			return nil, err
		}
		proposedJSON, err := common.Marshal(proposed)
		if err != nil {
			return nil, err
		}
		pricesJSON, err := common.Marshal(item.Prices)
		if err != nil {
			return nil, err
		}
		requiredJSON, _ := common.Marshal(item.RequiredFacts)
		availableJSON, _ := common.Marshal(available)
		missingJSON, _ := common.Marshal(missing)
		expression := item.Expression
		if pricingScope == "PLUGIN_OVERRIDE" {
			expression = item.TaskExpression
		}
		row := model.PricingSyncItem{ModelID: item.ModelID, Category: item.Category, EndpointType: item.EndpointType, Status: status, Reason: reason, ReasonCode: reasonCode, ExpectedVersion: entry.Version, CurrentPricing: string(currentJSON), ProposedPricing: string(proposedJSON), Prices: string(pricesJSON), Expression: expression, PricingShape: item.PricingShape, PricingScope: pricingScope, PluginKey: pluginKey, RequiredFacts: string(requiredJSON), AvailableFacts: string(availableJSON), MissingFacts: string(missingJSON)}
		if status != SupportedAuto {
			row.Action = "SKIP"
			result = append(result, row)
			continue
		}
		if pricingScope == "MODEL" && (len(entry.PluginVariants) > 0 || len(entry.UsageSchema) > 0) {
			row.Status, row.Action, row.Reason = UnsupportedMapping, "SKIP", "task plugin pricing requires usage mapping"
			result = append(result, row)
			continue
		}
		allowed := map[string]bool{"text": config.SyncText, "image": config.SyncImage, "video": config.SyncVideo, "audio": config.SyncAudio}
		if enabled, known := allowed[item.Category]; (known && !enabled) || (!known && !config.SyncOther) {
			row.Status, row.Action, row.Reason = "SKIPPED_SCOPE", "SKIP", "category is disabled"
			result = append(result, row)
			continue
		}
		if model.ModelPricingVersion(proposed) == entry.Version {
			row.Action = "UNCHANGED"
			result = append(result, row)
			continue
		}
		if len(entry.Configured) == 0 {
			if !config.IncludeNewCallableModels {
				row.Status, row.Action, row.Reason = "SKIPPED_NEW", "SKIP", "new models are disabled"
			} else {
				row.Action = "ADD"
			}
			result = append(result, row)
			continue
		}
		row.Action = "UPDATE"
		owner, owned := managed[item.ModelID]
		if !owned {
			row.Status, row.Reason = "MANUAL_OVERRIDE", "existing pricing requires explicit adoption"
			result = append(result, row)
			continue
		}
		if owner.LastAppliedHash != entry.Version {
			row.Status, row.Reason = "MANUAL_DRIFT", "pricing was edited after the last sync"
			result = append(result, row)
			continue
		}
		if owner.PricingShape != "" && owner.PricingShape != item.PricingShape {
			row.Status, row.Action, row.Reason, row.ReasonCode = "PRICE_SHAPE_CHANGED", "BLOCK", "DFLOP pricing shape changed", "PRICE_SHAPE_CHANGED"
			result = append(result, row)
			continue
		}
		var oldPrices map[string]Price
		if err := common.UnmarshalJsonStr(owner.Prices, &oldPrices); err != nil {
			return nil, fmt.Errorf("managed model %s has invalid price provenance: %w", item.ModelID, err)
		}
		increaseLimit, _ := decimal.NewFromString(config.MaxAutoIncreasePercent)
		decreaseLimit, _ := decimal.NewFromString(config.MaxAutoDecreasePercent)
		largest := decimal.Zero
		for name, next := range item.Prices {
			old, exists := oldPrices[name]
			if !exists {
				row.Status, row.Action, row.Reason, row.ReasonCode = "PRICE_SHAPE_CHANGED", "BLOCK", "pricing components changed", "PRICE_SHAPE_CHANGED"
				break
			}
			oldValue, err := decimal.NewFromString(old.SellingUSD)
			if err != nil {
				return nil, err
			}
			nextValue, err := decimal.NewFromString(next.SellingUSD)
			if err != nil {
				return nil, err
			}
			if oldValue.IsZero() {
				if !nextValue.IsZero() {
					row.Status, row.Reason = "SUPPORTED_MANUAL", "free price became paid"
				}
				continue
			}
			delta := nextValue.Sub(oldValue).DivRound(oldValue, 12).Mul(decimal.NewFromInt(100))
			if delta.Abs().GreaterThan(largest.Abs()) {
				largest = delta
			}
			if delta.GreaterThan(increaseLimit) || delta.Neg().GreaterThan(decreaseLimit) {
				row.Status, row.Reason = "SUPPORTED_MANUAL", "price change exceeds automatic threshold"
			}
		}
		if len(oldPrices) != len(item.Prices) {
			row.Status, row.Action, row.Reason, row.ReasonCode = "PRICE_SHAPE_CHANGED", "BLOCK", "pricing components changed", "PRICE_SHAPE_CHANGED"
		}
		row.DeltaPercent = largest.Round(6).String()
		result = append(result, row)
	}
	return result, nil
}

type Manager struct {
	Source Source
	HTTP   *http.Client
}

func (m Manager) client() Client { return Client{HTTP: m.HTTP} }

func (m Manager) source() Source {
	if m.Source != nil {
		return m.Source
	}
	return Client{}
}

func (m Manager) Preview(ctx context.Context, actorID int, trigger string) (_ *model.PricingSyncRun, _ []model.PricingSyncItem, err error) {
	started := time.Now()
	random, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, nil, err
	}
	runID := "dflop-" + random
	defer func() {
		if err == nil {
			return
		}
		logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing preview failed: run_id=%s trigger=%s duration=%s err=%v", runID, trigger, time.Since(started), err))
		failure := &model.PricingSyncRun{ID: runID, Provider: "dflop", Trigger: trigger, Mode: "preview", Status: "failed", ActorID: actorID, ErrorMessage: err.Error(), StartedAt: started.Unix(), FinishedAt: time.Now().Unix()}
		if saveErr := model.CreatePricingSyncFailure(failure); saveErr != nil {
			logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing failure audit write failed: run_id=%s err=%v", runID, saveErr))
		}
	}()
	config, err := model.GetDFLOPConfig()
	if err != nil {
		return nil, nil, err
	}
	if !config.Enabled {
		return nil, nil, errors.New("DFLOP pricing sync is disabled")
	}
	if err := config.ValidateForPreview(); err != nil {
		return nil, nil, err
	}
	logger.LogInfo(ctx, fmt.Sprintf("DFLOP pricing fetch started: run_id=%s trigger=%s", runID, trigger))
	var catalog, currency []byte
	var items []Item
	var sourceHash string
	record := SourceRecord{SourceMode: "PUBLIC_DIAGNOSTIC"}
	if config.SourceChannelID == 0 {
		if m.Source == nil {
			record.Public, err = m.client().FetchPublic(ctx)
			if err != nil {
				return nil, nil, err
			}
			catalog = record.Public.Body
			currency, err = m.client().FetchCurrency(ctx)
		} else {
			catalog, currency, err = m.source().Fetch(ctx)
			record.Public = EffectiveResponse{RequestedURL: CatalogURL, State: "FRESH", FetchedAt: time.Now().Unix()}
		}
		if err != nil {
			return nil, nil, err
		}
		items, sourceHash, err = Build(catalog, currency, config.CNYToUSD, config.MarkupMultiplier)
		if err != nil {
			return nil, nil, err
		}
		record.PublicHash, record.PublicCount = sourceDigest(catalog), len(items)
		for _, item := range items {
			if item.Callable {
				record.PublicCallableCount++
			}
		}
		record.Integrity = []string{"SOURCE_CHANNEL_UNAVAILABLE"}
	} else {
		var key string
		record.SourceChannel, key, err = LoadSourceChannel(config.SourceChannelID)
		if err != nil {
			return nil, nil, err
		}
		record.CredentialFingerprint = sourceDigest([]byte(key))[:16]
		lastCatalog, previous, err := lastGoodSource(config.SourceChannelID, record.CredentialFingerprint)
		if err != nil {
			return nil, nil, err
		}
		client := m.client()
		record.Effective, err = client.FetchEffective(ctx, key, previous.Effective.ETag)
		if err != nil {
			return nil, nil, err
		}
		catalog = record.Effective.Body
		if record.Effective.State == "NOT_MODIFIED" {
			if len(lastCatalog) == 0 {
				return nil, nil, errors.New("DFLOP returned 304 without a last-good catalog")
			}
			catalog = lastCatalog
			if record.Effective.ETag == "" {
				record.Effective.ETag = previous.Effective.ETag
			}
		}
		record.EffectiveHash = sourceDigest(catalog)
		currency, err = client.FetchCurrency(ctx)
		if err != nil {
			return nil, nil, err
		}
		var metadata EffectiveMetadata
		items, sourceHash, metadata, err = BuildEffective(catalog, currency, config.CNYToUSD, config.MarkupMultiplier)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "SCHEMA_VERSION_CHANGED") {
				return nil, nil, err
			}
			record.Integrity = append(record.Integrity, "SCHEMA_VERSION_CHANGED")
			sourceHash = record.EffectiveHash
			items = nil
			err = nil
		}
		record.SourceMode, record.CatalogSchemaVersion = "AUTHENTICATED_EFFECTIVE", metadata.SchemaVersion
		record.EffectiveCount = len(items)
		if slices.Contains(record.Integrity, "SCHEMA_VERSION_CHANGED") {
			var inspection struct {
				Models []json.RawMessage `json:"models"`
			}
			if err := common.Unmarshal(catalog, &inspection); err != nil {
				return nil, nil, err
			}
			record.EffectiveCount = len(inspection.Models)
		}
		effectiveIDs := make(map[string]bool, len(items))
		for _, item := range items {
			effectiveIDs[item.ModelID] = true
			if item.Callable {
				record.CallableCount++
			}
			if item.ReasonCode == "UNKNOWN_BILLING_FEATURE" && !slices.Contains(record.Integrity, "UNKNOWN_BILLING_FEATURE") {
				record.Integrity = append(record.Integrity, "UNKNOWN_BILLING_FEATURE")
			}
		}
		if collapsed(previous.EffectiveCount, record.EffectiveCount) {
			record.Integrity = append(record.Integrity, "EFFECTIVE_CATALOG_ANOMALY", "MODEL_COUNT_COLLAPSE")
		}
		if record.EffectiveCount == 0 && !slices.Contains(record.Integrity, "EFFECTIVE_CATALOG_ANOMALY") {
			record.Integrity = append(record.Integrity, "EFFECTIVE_CATALOG_ANOMALY")
		}
		if len(lastCatalog) > 0 && !slices.Contains(record.Integrity, "SCHEMA_VERSION_CHANGED") {
			var prior struct {
				Models []struct {
					ID string `json:"id"`
				} `json:"models"`
			}
			if err := common.Unmarshal(lastCatalog, &prior); err != nil {
				return nil, nil, err
			}
			for _, old := range prior.Models {
				if old.ID != "" && !effectiveIDs[old.ID] {
					items = append(items, Item{ModelID: old.ID, CanonicalID: old.ID, Status: "NOT_AVAILABLE_TO_SOURCE_KEY", ReasonCode: "NOT_AVAILABLE_TO_SOURCE_KEY", Reason: "model is absent from this key's catalog; global removal is not inferred", Prices: map[string]Price{}})
				}
			}
		}
		public, publicErr := client.FetchPublic(ctx)
		record.Public = public
		if publicErr != nil {
			record.Integrity = append(record.Integrity, "PUBLIC_CATALOG_ANOMALY")
		} else {
			record.PublicHash = sourceDigest(public.Body)
			var parsed struct {
				SchemaVersion string `json:"schema_version"`
				Models        []struct {
					ID       string `json:"id"`
					Callable bool   `json:"callable"`
				} `json:"models"`
			}
			if common.Unmarshal(public.Body, &parsed) != nil || parsed.Models == nil {
				record.Integrity = append(record.Integrity, "PUBLIC_CATALOG_ANOMALY")
			} else {
				record.PublicCount = len(parsed.Models)
				record.PublicSchemaVersion = parsed.SchemaVersion
				publicIDs := make(map[string]bool, len(parsed.Models))
				for _, entry := range parsed.Models {
					publicIDs[entry.ID] = true
					if entry.Callable {
						record.PublicCallableCount++
					}
					if !effectiveIDs[entry.ID] {
						record.PublicOnlyCount++
					}
				}
				for id := range effectiveIDs {
					if !publicIDs[id] {
						record.EffectiveOnlyCount++
					}
				}
				if collapsed(previous.PublicCount, record.PublicCount) || collapsed(record.EffectiveCount, record.PublicCount) {
					record.Integrity = append(record.Integrity, "PUBLIC_CATALOG_ANOMALY")
				}
			}
		}
		if len(record.Integrity) == 0 {
			record.Integrity = []string{"HEALTHY"}
		}
	}
	logger.LogInfo(ctx, fmt.Sprintf("DFLOP pricing fetch completed: run_id=%s trigger=%s catalog_bytes=%d duration=%s", runID, trigger, len(catalog), time.Since(started)))
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.ModelID
	}
	snapshot, err := model.GetModelPricingSnapshot(names)
	if err != nil {
		return nil, nil, err
	}
	managed, err := model.GetPricingSyncManaged(names)
	if err != nil {
		return nil, nil, err
	}
	planned, err := Plan(items, config, snapshot.Entries, managed)
	if err != nil {
		return nil, nil, err
	}
	if record.SourceMode == "PUBLIC_DIAGNOSTIC" {
		for i := range planned {
			planned[i].Status, planned[i].Action, planned[i].ReasonCode = "PUBLIC_DIAGNOSTIC", "SKIP", "SOURCE_CHANNEL_UNAVAILABLE"
			planned[i].Reason = "select a DFLOP source channel before applying pricing"
		}
	}
	record.Policy = EvaluateSourceIntegrity(record)
	if slices.Contains(record.Integrity, "MODEL_COUNT_COLLAPSE") {
		for i := range planned {
			if planned[i].Action == "ADD" || planned[i].Action == "UPDATE" {
				planned[i].Status, planned[i].Action, planned[i].ReasonCode = UnsupportedMapping, "BLOCK", "MODEL_COUNT_COLLAPSE"
				planned[i].Reason = "authenticated catalog shrank by more than 25 percent"
			}
		}
	}
	configJSON, err := common.Marshal(config)
	if err != nil {
		return nil, nil, err
	}
	versions := make(map[string]string, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		versions[entry.ModelName] = entry.Version
	}
	version := model.ModelPricingVersion(model.PricingValues{"versions": versions})
	packed, err := packSource(catalog)
	if err != nil {
		return nil, nil, err
	}
	record.Currency = currency
	recordJSON, err := common.Marshal(record)
	if err != nil {
		return nil, nil, err
	}
	run := &model.PricingSyncRun{ID: runID, Provider: "dflop", Trigger: trigger, Status: "preview", SourceHash: sourceHash, ConfigHash: config.Hash(), ConfigSnapshot: string(configJSON), CurrencySnapshot: string(recordJSON), SourceSnapshot: packed, PricingVersionBefore: version, ActorID: actorID, StartedAt: started.Unix()}
	for i := range planned {
		planned[i].RunID = run.ID
		if planned[i].Action == "BLOCK" || planned[i].Status == UnsupportedMapping || planned[i].Status == "MANUAL_DRIFT" {
			run.BlockedCount++
		}
		if planned[i].Action == "ADD" || planned[i].Action == "UPDATE" {
			run.ChangedCount++
		}
	}
	if err := model.CreatePricingSyncPreview(run, planned); err != nil {
		return nil, nil, err
	}
	logger.LogInfo(ctx, fmt.Sprintf("DFLOP pricing preview completed: run_id=%s trigger=%s model_count=%d change_count=%d blocked_count=%d duration=%s", runID, trigger, len(planned), run.ChangedCount, run.BlockedCount, time.Since(started)))
	return run, planned, nil
}

func (m Manager) Apply(ctx context.Context, id, expectedVersion string, selected []string, adopt bool, actorID int, acknowledgePublicAnomaly bool) (_ *model.PricingSyncRun, err error) {
	defer func() {
		if err == nil {
			return
		}
		logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing apply failed: run_id=%s err=%v", id, err))
		if recordErr := model.RecordPricingSyncApplyFailure(id, err); recordErr != nil {
			logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing apply failure audit write failed: run_id=%s err=%v", id, recordErr))
		}
	}()
	run, _, err := model.GetPricingSyncPreview(id)
	if err != nil {
		return nil, err
	}
	if run.PricingVersionBefore != expectedVersion {
		return nil, model.ErrModelPricingConflict
	}
	config, err := model.GetDFLOPConfig()
	if err != nil {
		return nil, err
	}
	if !config.Enabled || config.Hash() != run.ConfigHash {
		return nil, model.ErrModelPricingConflict
	}
	var record SourceRecord
	if err := common.UnmarshalJsonStr(run.CurrencySnapshot, &record); err != nil || record.SourceMode != "AUTHENTICATED_EFFECTIVE" || record.SourceChannel.ID != config.SourceChannelID {
		return nil, errors.New("SOURCE_CHANNEL_UNAVAILABLE: authenticated preview required")
	}
	policy := EvaluateSourceIntegrity(record)
	if policy.ManualBlockReason != "" {
		return nil, fmt.Errorf("%s: DFLOP pricing source integrity blocks apply", policy.ManualBlockReason)
	}
	if actorID == 0 && policy.AutoBlockReason != "" {
		return nil, fmt.Errorf("%s: DFLOP catalog integrity blocks automatic apply", policy.AutoBlockReason)
	}
	if policy.ManualConfirmationRequired && (run.Trigger != "manual" || actorID <= 0 || !acknowledgePublicAnomaly) {
		return nil, errors.New("PUBLIC_CATALOG_ANOMALY: explicit root-admin confirmation required")
	}
	_, key, err := LoadSourceChannel(config.SourceChannelID)
	if err != nil {
		return nil, err
	}
	if record.CredentialFingerprint != sourceDigest([]byte(key))[:16] {
		return nil, model.ErrModelPricingConflict
	}
	response, err := m.client().FetchEffective(ctx, key, record.Effective.ETag)
	if err != nil {
		return nil, err
	}
	catalog := response.Body
	if response.State == "NOT_MODIFIED" {
		catalog, err = unpackSource(run.SourceSnapshot)
		if err != nil || sourceDigest(catalog) != record.EffectiveHash {
			return nil, errors.New("DFLOP preview catalog snapshot is corrupt")
		}
	}
	currency, err := m.client().FetchCurrency(ctx)
	if err != nil {
		return nil, err
	}
	_, sourceHash, _, err := BuildEffective(catalog, currency, config.CNYToUSD, config.MarkupMultiplier)
	if err != nil {
		return nil, err
	}
	if sourceHash != run.SourceHash {
		return nil, model.ErrModelPricingConflict
	}
	_, currentKey, err := LoadSourceChannel(config.SourceChannelID)
	if err != nil {
		return nil, err
	}
	if record.CredentialFingerprint != sourceDigest([]byte(currentKey))[:16] {
		return nil, model.ErrModelPricingConflict
	}
	applied, err := model.ApplyPricingSyncWithContext(ctx, id, config.Hash(), sourceHash, selected, adopt, actorID)
	if err != nil {
		return nil, err
	}
	logger.LogInfo(ctx, fmt.Sprintf("DFLOP pricing apply completed: run_id=%s trigger=%s model_count=%d", id, run.Trigger, len(selected)))
	return applied, nil
}
