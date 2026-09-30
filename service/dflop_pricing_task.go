package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
)

type dflopPricingTaskHandler struct{}

func (dflopPricingTaskHandler) Type() string { return model.SystemTaskTypeDFLOPPricing }

func (dflopPricingTaskHandler) Enabled() bool {
	config, err := model.GetDFLOPConfig()
	return err == nil && config.Enabled && config.AutoSyncEnabled
}

func (dflopPricingTaskHandler) Interval() time.Duration {
	config, err := model.GetDFLOPConfig()
	if err != nil || config.SyncIntervalHours < 1 {
		return 6 * time.Hour
	}
	return time.Duration(config.SyncIntervalHours) * time.Hour
}

func (dflopPricingTaskHandler) NewPayload() any { return nil }

func (dflopPricingTaskHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	finish := func(status model.SystemTaskStatus, result any, message string) {
		if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, message); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing task finish failed: %v", err))
		}
	}
	config, err := model.GetDFLOPConfig()
	if err != nil || !config.Enabled || !config.AutoSyncEnabled {
		finish(model.SystemTaskStatusSucceeded, nil, "")
		return
	}
	start := time.Now()
	run, items, err := (dflop.Manager{}).Preview(ctx, 0, "scheduler")
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing fetch/preview failed: %v", err))
		finish(model.SystemTaskStatusFailed, nil, err.Error())
		return
	}
	selected := make([]string, 0)
	if config.AutoApplyEnabled {
		var source dflop.SourceRecord
		if err := common.UnmarshalJsonStr(run.CurrencySnapshot, &source); err != nil || dflop.EvaluateSourceIntegrity(source).AutoBlockReason != "" {
			finish(model.SystemTaskStatusFailed, map[string]any{"run_id": run.ID}, "DFLOP source integrity blocks automatic apply")
			return
		}
		for _, item := range items {
			if item.Status != dflop.SupportedAuto || (item.Action != "ADD" && item.Action != "UPDATE") {
				continue
			}
			if item.Action == "ADD" && !config.AllowAutoApplyNewModels {
				continue
			}
			selected = append(selected, item.ModelID)
		}
	}
	if len(selected) > 0 {
		if _, err := (dflop.Manager{}).Apply(ctx, run.ID, run.PricingVersionBefore, selected, false, 0, false); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("DFLOP pricing auto apply blocked: run_id=%s err=%v", run.ID, err))
			finish(model.SystemTaskStatusFailed, map[string]any{"run_id": run.ID}, err.Error())
			return
		}
	}
	logger.LogInfo(ctx, fmt.Sprintf("DFLOP pricing sync completed: run_id=%s model_count=%d change_count=%d applied_count=%d duration=%s trigger=scheduler", run.ID, len(items), run.ChangedCount, len(selected), time.Since(start)))
	finish(model.SystemTaskStatusSucceeded, map[string]any{"run_id": run.ID, "applied_count": len(selected)}, "")
}

func init() { RegisterSystemTaskHandler(dflopPricingTaskHandler{}) }
