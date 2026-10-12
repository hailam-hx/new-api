package controller

import (
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func GetDeveloperCatalog(c *gin.Context) {
	unit, currencyRate, err := service.PublicCurrencyUnitAndRate()
	if err != nil || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: common.MessageWithRequestId("Unable to read model catalog", c.GetString(common.RequestIdKey)), Type: "new_api_error"}})
		return
	}
	groups, err := getModelListGroups(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: common.MessageWithRequestId("Unable to read model catalog", c.GetString(common.RequestIdKey)), Type: "new_api_error"}})
		return
	}
	entries, err := model.GetDeveloperCatalogEntries(groups.ownerGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: common.MessageWithRequestId("Unable to read model catalog", c.GetString(common.RequestIdKey)), Type: "new_api_error"}})
		return
	}
	groupRatios := make(map[string]decimal.Decimal, len(groups.ownerGroups))
	for _, group := range groups.ownerGroups {
		ratio := service.GetUserGroupRatio(groups.userGroup, group)
		if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: common.MessageWithRequestId("Unable to read model catalog", c.GetString(common.RequestIdKey)), Type: "new_api_error"}})
			return
		}
		groupRatios[group] = decimal.NewFromFloat(ratio)
	}
	modelLimitEnabled := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	modelLimits, _ := common.GetContextKeyType[map[string]bool](c, constant.ContextKeyTokenModelLimit)
	userSetting, _ := common.GetContextKeyType[relaydto.UserSetting](c, constant.ContextKeyUserSetting)
	acceptUnset := operation_setting.SelfUseModeEnabled || userSetting.AcceptUnsetRatioModel
	catalog := dto.DeveloperCatalog{Object: "catalog", SchemaVersion: "1.0", Currency: unit, Models: make([]dto.DeveloperCatalogModel, 0, len(entries))}
	if unit == "custom" {
		catalog.CurrencySymbol = operation_setting.GetCurrencySymbol()
	}
	for _, entry := range entries {
		if modelLimitEnabled && !middleware.TokenModelLimitAllows(modelLimits, entry.Name) {
			continue
		}
		billingName := helper.ResolveRequestBillingModelName(entry.Name)
		if item, callable := entry.PublicModel(billingName, groupRatios, currencyRate, acceptUnset); callable {
			catalog.Models = append(catalog.Models, item)
		}
	}
	c.JSON(http.StatusOK, catalog)
}
