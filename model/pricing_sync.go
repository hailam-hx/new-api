package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DFLOPConfigOption = "dflop_pricing_sync.config"

type DFLOPConfig struct {
	Enabled                  bool   `json:"enabled"`
	SourceChannelID          int    `json:"source_channel_id"`
	AutoSyncEnabled          bool   `json:"auto_sync_enabled"`
	AutoApplyEnabled         bool   `json:"auto_apply_enabled"`
	SyncIntervalHours        int    `json:"sync_interval_hours"`
	CNYToUSD                 string `json:"cny_to_usd"`
	MarkupMultiplier         string `json:"markup_multiplier"`
	SyncText                 bool   `json:"sync_text"`
	SyncImage                bool   `json:"sync_image"`
	SyncVideo                bool   `json:"sync_video"`
	SyncAudio                bool   `json:"sync_audio"`
	SyncOther                bool   `json:"sync_other"`
	IncludeNewCallableModels bool   `json:"include_new_callable_models"`
	MaxAutoIncreasePercent   string `json:"max_auto_increase_percent"`
	MaxAutoDecreasePercent   string `json:"max_auto_decrease_percent"`
	AllowAutoApplyNewModels  bool   `json:"allow_auto_apply_new_models"`
}

func DefaultDFLOPConfig() DFLOPConfig {
	return DFLOPConfig{SyncIntervalHours: 6, CNYToUSD: "0", MarkupMultiplier: "1", SyncText: true, SyncImage: true, SyncVideo: true, SyncAudio: true, SyncOther: true, IncludeNewCallableModels: true, MaxAutoIncreasePercent: "20", MaxAutoDecreasePercent: "50"}
}

func (c DFLOPConfig) Validate() error {
	if c.SyncIntervalHours < 1 || c.SyncIntervalHours > 720 {
		return errors.New("sync interval must be between 1 and 720 hours")
	}
	for name, raw := range map[string]string{"markup": c.MarkupMultiplier, "increase threshold": c.MaxAutoIncreasePercent, "decrease threshold": c.MaxAutoDecreasePercent, "CNY to USD": c.CNYToUSD} {
		value, err := decimal.NewFromString(raw)
		if err != nil || value.IsNegative() || (name == "markup" && value.IsZero()) {
			return fmt.Errorf("invalid %s", name)
		}
	}
	if c.AutoApplyEnabled && (!c.Enabled || !c.AutoSyncEnabled) {
		return errors.New("automatic apply requires enabled automatic sync")
	}
	if c.SourceChannelID < 0 || (c.AutoApplyEnabled && c.SourceChannelID == 0) {
		return errors.New("automatic apply requires a DFLOP pricing source channel")
	}
	if (c.AutoApplyEnabled || c.AutoSyncEnabled) && !c.Enabled {
		return errors.New("automatic sync requires enabled pricing sync")
	}
	return nil
}

func (c DFLOPConfig) ValidateForPreview() error {
	if err := c.Validate(); err != nil {
		return err
	}
	rate, _ := decimal.NewFromString(c.CNYToUSD)
	if !rate.GreaterThan(decimal.Zero) {
		return errors.New("set a positive CNY to USD rate before previewing")
	}
	return nil
}

func (c DFLOPConfig) Hash() string {
	encoded, _ := common.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func GetDFLOPConfig() (DFLOPConfig, error) {
	config := DefaultDFLOPConfig()
	var option Option
	result := DB.Where(commonKeyCol+" = ?", DFLOPConfigOption).Limit(1).Find(&option)
	if result.Error != nil {
		return config, result.Error
	}
	if result.RowsAffected == 0 {
		return config, nil
	}
	if err := common.UnmarshalJsonStr(option.Value, &config); err != nil {
		return config, err
	}
	if config.SourceChannelID == 0 {
		// Legacy settings never selected a key-specific price source.
		config.AutoApplyEnabled = false
	}
	return config, config.Validate()
}

func SaveDFLOPConfig(config DFLOPConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	encoded, err := common.Marshal(config)
	if err != nil {
		return err
	}
	return UpdateOptionsBulk(map[string]string{DFLOPConfigOption: string(encoded)})
}

type PricingSyncRun struct {
	ID                   string `json:"id" gorm:"type:varchar(64);primaryKey"`
	Provider             string `json:"provider" gorm:"type:varchar(32);index"`
	Trigger              string `json:"trigger" gorm:"type:varchar(32)"`
	Mode                 string `json:"mode" gorm:"type:varchar(32)"`
	Status               string `json:"status" gorm:"type:varchar(32);index"`
	SourceHash           string `json:"source_hash" gorm:"type:varchar(64)"`
	ConfigHash           string `json:"config_hash" gorm:"type:varchar(64)"`
	PricingVersionBefore string `json:"pricing_version_before" gorm:"type:varchar(64)"`
	PricingVersionAfter  string `json:"pricing_version_after" gorm:"type:varchar(64)"`
	SourceSnapshot       string `json:"-" gorm:"type:text"`
	CurrencySnapshot     string `json:"currency_snapshot" gorm:"type:text"`
	ConfigSnapshot       string `json:"config_snapshot" gorm:"type:text"`
	BeforeState          string `json:"-" gorm:"type:text"`
	AfterState           string `json:"-" gorm:"type:text"`
	BeforeManaged        string `json:"-" gorm:"type:text"`
	ErrorMessage         string `json:"error_message" gorm:"type:text"`
	ActorID              int    `json:"actor_id"`
	AppliedBy            int    `json:"applied_by"`
	RollbackOfRunID      string `json:"rollback_of_run_id" gorm:"type:varchar(64);index"`
	TotalCount           int    `json:"total_count"`
	ChangedCount         int    `json:"changed_count"`
	BlockedCount         int    `json:"blocked_count"`
	StartedAt            int64  `json:"started_at" gorm:"index"`
	FinishedAt           int64  `json:"finished_at"`
}

type PricingSyncItem struct {
	ID              uint64 `json:"id" gorm:"primaryKey"`
	RunID           string `json:"run_id" gorm:"type:varchar(64);index"`
	ModelID         string `json:"model_id" gorm:"type:varchar(191);index"`
	Category        string `json:"category" gorm:"type:varchar(32)"`
	EndpointType    string `json:"endpoint_type" gorm:"type:varchar(64)"`
	Status          string `json:"status" gorm:"type:varchar(32);index"`
	Action          string `json:"action" gorm:"type:varchar(16)"`
	Reason          string `json:"reason" gorm:"type:text"`
	ExpectedVersion string `json:"expected_version" gorm:"type:varchar(64)"`
	CurrentPricing  string `json:"current_pricing" gorm:"type:text"`
	ProposedPricing string `json:"proposed_pricing" gorm:"type:text"`
	Prices          string `json:"prices" gorm:"type:text"`
	Expression      string `json:"expression" gorm:"type:text"`
	DeltaPercent    string `json:"delta_percent" gorm:"type:varchar(64)"`
	PricingShape    string `json:"pricing_shape" gorm:"type:varchar(255)"`
	PricingScope    string `json:"pricing_scope" gorm:"type:varchar(32)"`
	PluginKey       string `json:"plugin_key" gorm:"type:varchar(64)"`
	ReasonCode      string `json:"reason_code" gorm:"type:varchar(64)"`
	RequiredFacts   string `json:"required_facts" gorm:"type:text"`
	AvailableFacts  string `json:"available_facts" gorm:"type:text"`
	MissingFacts    string `json:"missing_facts" gorm:"type:text"`
}

type PricingSyncManaged struct {
	ModelID         string `json:"model_id" gorm:"type:varchar(191);primaryKey"`
	Provider        string `json:"provider" gorm:"type:varchar(32);index"`
	SourceHash      string `json:"source_hash" gorm:"type:varchar(64)"`
	LastRunID       string `json:"last_run_id" gorm:"type:varchar(64)"`
	LastAppliedHash string `json:"last_applied_hash" gorm:"type:varchar(64)"`
	Prices          string `json:"prices" gorm:"type:text"`
	PricingShape    string `json:"pricing_shape" gorm:"type:varchar(255)"`
	AppliedAt       int64  `json:"applied_at"`
}

func (PricingSyncManaged) TableName() string { return "pricing_sync_managed_models" }

func CreatePricingSyncPreview(run *PricingSyncRun, items []PricingSyncItem) error {
	if run == nil || run.ID == "" || len(run.ID) > 64 || run.Status != "preview" || run.PricingVersionBefore == "" {
		return errors.New("invalid pricing preview")
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.RunID != run.ID || item.ModelID == "" || len(item.ModelID) > 191 || seen[item.ModelID] {
			return errors.New("invalid pricing preview item")
		}
		seen[item.ModelID] = true
	}
	run.Mode = "preview"
	if run.StartedAt == 0 {
		run.StartedAt = time.Now().Unix()
	}
	run.TotalCount = len(items)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			return tx.CreateInBatches(items, 100).Error
		}
		return nil
	})
}

func CreatePricingSyncFailure(run *PricingSyncRun) error {
	if run == nil || run.ID == "" || len(run.ID) > 64 || run.Status != "failed" {
		return errors.New("invalid pricing sync failure")
	}
	return DB.Create(run).Error
}

func RecordPricingSyncApplyFailure(id string, applyErr error) error {
	if id == "" || applyErr == nil {
		return nil
	}
	return DB.Model(&PricingSyncRun{}).Where("id = ? AND status = ?", id, "preview").Update("error_message", applyErr.Error()).Error
}

func GetPricingSyncPreview(id string) (*PricingSyncRun, []PricingSyncItem, error) {
	var run PricingSyncRun
	if err := DB.Where("id = ?", id).First(&run).Error; err != nil {
		return nil, nil, err
	}
	var items []PricingSyncItem
	if err := DB.Where("run_id = ?", id).Order("model_id asc").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	return &run, items, nil
}

func ListPricingSyncRuns(limit int) ([]PricingSyncRun, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return ListPricingSyncRunsPage(limit, 0)
}

func ListPricingSyncRunsPage(limit, offset int) ([]PricingSyncRun, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, errors.New("invalid pricing sync history page")
	}
	var runs []PricingSyncRun
	err := DB.Where("provider = ?", "dflop").Order("started_at desc, id desc").Limit(limit).Offset(offset).Find(&runs).Error
	return runs, err
}

func GetPricingSyncManaged(names []string) (map[string]PricingSyncManaged, error) {
	result := make(map[string]PricingSyncManaged)
	if len(names) == 0 {
		return result, nil
	}
	var rows []PricingSyncManaged
	if err := DB.Where("model_id IN ?", names).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ModelID] = row
	}
	return result, nil
}

func ApplyPricingSync(id, configHash, sourceHash string, selected []string, adopt bool, actorID int) (*PricingSyncRun, error) {
	return ApplyPricingSyncWithContext(context.Background(), id, configHash, sourceHash, selected, adopt, actorID)
}

func ApplyPricingSyncWithContext(ctx context.Context, id, configHash, sourceHash string, selected []string, adopt bool, actorID int) (*PricingSyncRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, errors.New("select at least one changed model")
	}
	chosen := make(map[string]bool, len(selected))
	for _, name := range selected {
		if name == "" || chosen[name] {
			return nil, errors.New("invalid model selection")
		}
		chosen[name] = true
	}
	var applied PricingSyncRun
	err := mutateModelPricingOptionsWithContext(ctx, func(tx *gorm.DB, values map[string]map[string]any) error {
		config := DefaultDFLOPConfig()
		var option Option
		configRow := lockForUpdate(tx).Where(commonKeyCol+" = ?", DFLOPConfigOption).Limit(1).Find(&option)
		if configRow.Error != nil {
			return configRow.Error
		}
		if configRow.RowsAffected > 0 {
			if err := common.UnmarshalJsonStr(option.Value, &config); err != nil {
				return err
			}
		}
		if !config.Enabled || config.Hash() != configHash {
			return ErrModelPricingConflict
		}
		var run PricingSyncRun
		if err := lockForUpdate(tx).Where("id = ?", id).First(&run).Error; err != nil {
			return err
		}
		if run.Status != "preview" || run.ConfigHash != configHash || run.SourceHash != sourceHash {
			return ErrModelPricingConflict
		}
		if time.Now().Unix()-run.StartedAt > 600 {
			return ErrModelPricingConflict
		}
		var items []PricingSyncItem
		if err := tx.Where("run_id = ?", id).Find(&items).Error; err != nil {
			return err
		}
		versions := make(map[string]string, len(items))
		for _, item := range items {
			versions[item.ModelID] = ModelPricingVersion(modelPricingValues(values, item.ModelID))
		}
		if ModelPricingVersion(PricingValues{"versions": versions}) != run.PricingVersionBefore {
			return ErrModelPricingConflict
		}
		before := make(map[string]PricingValues)
		after := make(map[string]PricingValues)
		managedBefore := make(map[string]*PricingSyncManaged)
		for _, item := range items {
			if !chosen[item.ModelID] {
				continue
			}
			if item.Action != "ADD" && item.Action != "UPDATE" {
				return fmt.Errorf("model %s has no applicable change", item.ModelID)
			}
			if item.Status != "SUPPORTED_AUTO" && item.Status != "SUPPORTED_MANUAL" && !(adopt && (item.Status == "MANUAL_OVERRIDE" || item.Status == "MANUAL_DRIFT")) {
				return fmt.Errorf("model %s is blocked: %s", item.ModelID, item.Reason)
			}
			previous := modelPricingValues(values, item.ModelID)
			if ModelPricingVersion(previous) != item.ExpectedVersion {
				return fmt.Errorf("%w: %s", ErrModelPricingConflict, item.ModelID)
			}
			var owned PricingSyncManaged
			ownerRow := lockForUpdate(tx).Where("model_id = ?", item.ModelID).Limit(1).Find(&owned)
			if ownerRow.Error != nil {
				return ownerRow.Error
			}
			if ownerRow.RowsAffected > 0 {
				managedBefore[item.ModelID] = &owned
				if owned.LastAppliedHash != ModelPricingVersion(previous) && !adopt {
					return fmt.Errorf("%w: manual drift on %s", ErrModelPricingConflict, item.ModelID)
				}
			}
			if ownerRow.RowsAffected == 0 && len(previous) > 0 && !adopt {
				return fmt.Errorf("%w: manual pricing on %s", ErrModelPricingConflict, item.ModelID)
			}
			var proposed PricingValues
			if err := common.UnmarshalJsonStr(item.ProposedPricing, &proposed); err != nil {
				return err
			}
			if err := validateModelPricing(item.ModelID, proposed, previous); err != nil {
				return err
			}
			before[item.ModelID] = previous
			after[item.ModelID] = proposed
			replaceModelPricing(values, item.ModelID, proposed)
			owned = PricingSyncManaged{ModelID: item.ModelID, Provider: "dflop", SourceHash: run.SourceHash, LastRunID: id, LastAppliedHash: ModelPricingVersion(proposed), Prices: item.Prices, PricingShape: item.PricingShape, AppliedAt: time.Now().Unix()}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&owned).Error; err != nil {
				return err
			}
		}
		if len(before) != len(chosen) {
			return errors.New("selected model was not found in preview")
		}
		beforeJSON, err := common.Marshal(before)
		if err != nil {
			return err
		}
		afterJSON, err := common.Marshal(after)
		if err != nil {
			return err
		}
		managedJSON, err := common.Marshal(managedBefore)
		if err != nil {
			return err
		}
		run.BeforeState, run.AfterState, run.BeforeManaged = string(beforeJSON), string(afterJSON), string(managedJSON)
		run.Mode, run.Status, run.ChangedCount, run.FinishedAt, run.AppliedBy = "apply", "applied", len(before), time.Now().Unix(), actorID
		run.PricingVersionAfter = ModelPricingVersion(PricingValues{"models": after})
		if err := tx.Save(&run).Error; err != nil {
			return err
		}
		applied = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &applied, nil
}

func RollbackPricingSync(id string, actorID int) (*PricingSyncRun, error) {
	var rollback PricingSyncRun
	err := mutateModelPricingOptions(func(tx *gorm.DB, values map[string]map[string]any) error {
		var original PricingSyncRun
		if err := lockForUpdate(tx).Where("id = ?", id).First(&original).Error; err != nil {
			return err
		}
		if original.Status != "applied" {
			return ErrModelPricingConflict
		}
		var before, after map[string]PricingValues
		if err := common.UnmarshalJsonStr(original.BeforeState, &before); err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(original.AfterState, &after); err != nil {
			return err
		}
		var managedBefore map[string]*PricingSyncManaged
		if err := common.UnmarshalJsonStr(original.BeforeManaged, &managedBefore); err != nil {
			return err
		}
		for name, expected := range after {
			if ModelPricingVersion(modelPricingValues(values, name)) != ModelPricingVersion(expected) {
				return fmt.Errorf("%w: %s", ErrModelPricingConflict, name)
			}
		}
		for name, previous := range before {
			replaceModelPricing(values, name, previous)
			if owned := managedBefore[name]; owned != nil {
				if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(owned).Error; err != nil {
					return err
				}
			} else if err := tx.Where("model_id = ?", name).Delete(&PricingSyncManaged{}).Error; err != nil {
				return err
			}
		}
		random, err := common.GenerateRandomCharsKey(32)
		if err != nil {
			return err
		}
		rollback = PricingSyncRun{ID: "rollback-" + random, Provider: "dflop", Trigger: "manual", Mode: "rollback", Status: "rolled_back", RollbackOfRunID: id, ActorID: actorID, StartedAt: time.Now().Unix(), FinishedAt: time.Now().Unix(), ChangedCount: len(before), BeforeState: original.AfterState, AfterState: original.BeforeState}
		if err := tx.Create(&rollback).Error; err != nil {
			return err
		}
		return tx.Model(&original).Update("status", "rolled_back").Error
	})
	if err != nil {
		return nil, err
	}
	return &rollback, nil
}
