package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/gin-gonic/gin"
)

func GetDFLOPPricingSyncConfig(c *gin.Context) {
	config, err := model.GetDFLOPConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func ListDFLOPPricingSourceChannels(c *gin.Context) {
	channels, err := dflop.SourceChannelCandidates()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channels)
}

func SaveDFLOPPricingSyncConfig(c *gin.Context) {
	var config model.DFLOPConfig
	if err := common.DecodeJson(c.Request.Body, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if config.SourceChannelID != 0 {
		if _, _, err := dflop.LoadSourceChannel(config.SourceChannelID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	if err := model.SaveDFLOPConfig(config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "model.pricing.dflop.config", map[string]any{"enabled": config.Enabled, "auto_apply_enabled": config.AutoApplyEnabled})
	common.ApiSuccess(c, config)
}

func PreviewDFLOPPricingSync(c *gin.Context) {
	run, items, err := (dflop.Manager{}).Preview(c.Request.Context(), c.GetInt("id"), "manual")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"run": run, "items": items})
}

func ApplyDFLOPPricingSync(c *gin.Context) {
	var request struct {
		PreviewID                string   `json:"preview_id"`
		ExpectedVersion          string   `json:"expected_version"`
		Models                   []string `json:"models"`
		Adopt                    bool     `json:"adopt"`
		AcknowledgePublicAnomaly bool     `json:"acknowledge_public_catalog_anomaly"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	run, err := (dflop.Manager{}).Apply(c.Request.Context(), request.PreviewID, request.ExpectedVersion, request.Models, request.Adopt, c.GetInt("id"), request.AcknowledgePublicAnomaly)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrModelPricingConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "model.pricing.dflop.apply", map[string]any{"run_id": run.ID, "models": request.Models, "adopt": request.Adopt, "acknowledge_public_catalog_anomaly": request.AcknowledgePublicAnomaly})
	common.ApiSuccess(c, run)
}

func ListDFLOPPricingSyncRuns(c *gin.Context) {
	runs, err := model.ListPricingSyncRuns(50)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, runs)
}

func GetDFLOPPricingSyncRun(c *gin.Context) {
	run, items, err := model.GetPricingSyncPreview(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"run": run, "items": items})
}

func RollbackDFLOPPricingSync(c *gin.Context) {
	run, err := model.RollbackPricingSync(c.Param("id"), c.GetInt("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrModelPricingConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "model.pricing.dflop.rollback", map[string]any{"run_id": run.ID, "rollback_of": run.RollbackOfRunID})
	logger.LogInfo(c, "DFLOP pricing rollback completed: run_id="+run.ID+" rollback_of="+run.RollbackOfRunID)
	common.ApiSuccess(c, run)
}
