package service

import (
	"errors"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

// PublicCurrencyUnitAndRate projects USD into the configured customer display unit.
func PublicCurrencyUnitAndRate() (string, decimal.Decimal, error) {
	if math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		return "", decimal.Zero, errors.New("invalid quota conversion")
	}
	setting := operation_setting.GetGeneralSetting()
	var unit string
	switch setting.QuotaDisplayType {
	case operation_setting.QuotaDisplayTypeUSD:
		unit = "USD"
	case operation_setting.QuotaDisplayTypeCNY:
		unit = "CNY"
	case operation_setting.QuotaDisplayTypeTokens:
		return "", decimal.Zero, errors.New("internal quota display is not a public monetary unit")
	case operation_setting.QuotaDisplayTypeCustom:
		if math.IsNaN(setting.CustomCurrencyExchangeRate) || math.IsInf(setting.CustomCurrencyExchangeRate, 0) {
			return "", decimal.Zero, errors.New("invalid display currency rate")
		}
		unit = "custom"
		switch strings.ToLower(strings.TrimSpace(setting.CustomCurrencySymbol)) {
		case "积分", "điểm", "points":
			unit = "points"
		}
	default:
		return "", decimal.Zero, errors.New("invalid display currency")
	}
	rate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return "", decimal.Zero, errors.New("invalid display currency rate")
	}
	return unit, decimal.NewFromFloat(rate), nil
}
