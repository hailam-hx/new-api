package controller

import (
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func GetDeveloperKeyBalance(c *gin.Context) {
	token, authenticated := middleware.GetPublicAPIKey(c)
	unit, rate, err := service.PublicCurrencyUnitAndRate()
	if !authenticated || err != nil || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: common.MessageWithRequestId("Unable to read key balance", c.GetString(common.RequestIdKey)), Type: "new_api_error"}})
		return
	}
	divisor := decimal.NewFromFloat(common.QuotaPerUnit)
	used := decimal.NewFromInt(int64(token.UsedQuota))
	balance := dto.DeveloperKeyBalance{Object: "key.balance", Unit: unit, Used: used.Div(divisor).Mul(rate).String(), Unlimited: token.UnlimitedQuota}
	if unit == "custom" {
		balance.UnitSymbol = operation_setting.GetCurrencySymbol()
	}
	if !token.UnlimitedQuota {
		remainingQuota := decimal.NewFromInt(int64(token.RemainQuota))
		total := remainingQuota.Add(used).Div(divisor).Mul(rate).String()
		remaining := remainingQuota.Div(divisor).Mul(rate).String()
		balance.Total, balance.Remaining = &total, &remaining
	}
	if token.ExpiredTime != -1 {
		expiry := token.ExpiredTime
		balance.ExpiresAt = &expiry
	}
	c.JSON(http.StatusOK, balance)
}
