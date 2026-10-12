package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func GetDeveloperKeyUsage(c *gin.Context) {
	token, authenticated := middleware.GetPublicAPIKey(c)
	if !authenticated || token.Id <= 0 || token.UserId <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": types.OpenAIError{Message: "Authentication required", Type: "authentication_error"}})
		return
	}
	query := c.Request.URL.Query()
	end := common.GetTimestamp() + 1
	if values, exists := query["end"]; exists {
		value, err := strconv.ParseInt(query.Get("end"), 10, 64)
		if len(values) != 1 || err != nil || value < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": types.OpenAIError{Message: "end must be a Unix timestamp in seconds", Type: "invalid_request_error", Param: "end"}})
			return
		}
		end = value
	}
	start := max(int64(0), end-30*24*60*60)
	if values, exists := query["start"]; exists {
		value, err := strconv.ParseInt(query.Get("start"), 10, 64)
		if len(values) != 1 || err != nil || value < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": types.OpenAIError{Message: "start must be a Unix timestamp in seconds", Type: "invalid_request_error", Param: "start"}})
			return
		}
		start = value
	}
	if end <= start || end-start > 31*24*60*60 || len(query["model"]) > 1 || len(query.Get("model")) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": types.OpenAIError{Message: "Usage requires a nonempty time window of at most 31 days and one exact model", Type: "invalid_request_error"}})
		return
	}
	unit, rate, err := service.PublicCurrencyUnitAndRate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: "Unable to read key usage", Type: "new_api_error"}})
		return
	}
	usage, err := model.GetDeveloperKeyUsage(c.Request.Context(), model.DeveloperUsageQuery{UserID: token.UserId, TokenID: token.Id, StartTime: start, EndTime: end, Model: query.Get("model")})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": types.OpenAIError{Message: "Unable to read key usage", Type: "new_api_error"}})
		return
	}
	divisor := decimal.NewFromFloat(common.QuotaPerUnit)
	response := dto.DeveloperKeyUsage{Object: "key.usage", Unit: unit, StartTime: start, EndTime: end,
		RecordedConsumption: usage.Consumption.Div(divisor).Mul(rate).String(),
		RecordedRefunds:     usage.Refunds.Div(divisor).Mul(rate).String(),
		RecordedNet:         usage.Consumption.Sub(usage.Refunds).Div(divisor).Mul(rate).String(),
		BillingEvents:       usage.BillingEvents, ErrorEvents: usage.ErrorEvents, Components: usage.Components, ComponentStatus: usage.ComponentStatus,
	}
	if unit == "custom" {
		response.UnitSymbol = operation_setting.GetCurrencySymbol()
	}
	c.JSON(http.StatusOK, response)
}
