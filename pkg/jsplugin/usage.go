package jsplugin

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

// ValidateUsageFacts is the production completion-fact contract. It preserves
// explicit zeros, selector values and legacy extension fields while normalizing
// numeric facts exactly as the task adaptor does before frozen settlement.
func ValidateUsageFacts(values map[string]any, schema map[string]UsageFieldSchema) (map[string]any, error) {
	if values == nil {
		return nil, nil
	}
	validated := make(map[string]any, len(values))
	for key, value := range values {
		validated[key] = value
		if field, declared := schema[key]; declared {
			number, err := ValidateUsageValue(value, field, false)
			if err != nil {
				return nil, err
			}
			if field.Type == "number" {
				validated[key] = number
			}
			continue
		}
		if limit, canonical := CanonicalUsageLimit(key); canonical {
			number, numeric := UsageNumber(value, false)
			if !numeric {
				return nil, fmt.Errorf("plugin usage value must be a number")
			}
			if err := ValidateUsageNumberLimit(number, limit); err != nil {
				return nil, err
			}
			validated[key] = number
			continue
		}
		switch key {
		case "upstreamUnits", "completionTokens", "totalTokens":
			number, numeric := UsageNumber(value, false)
			if !numeric || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
				return nil, fmt.Errorf("plugin usage value must be a finite non-negative number")
			}
			validated[key] = float64(common.QuotaFromFloat(number))
		default:
			if number, numeric := UsageNumber(value, false); numeric {
				validated[key] = number
			}
		}
	}
	return validated, nil
}

// ValidateUsageValue is shared by request, reservation and completion paths.
func ValidateUsageValue(value any, schema UsageFieldSchema, allowNumericString bool) (float64, error) {
	if len(schema.Enum) > 0 {
		text, ok := value.(string)
		if !ok {
			return 0, fmt.Errorf("plugin usage enum must be a string")
		}
		if slices.Contains(schema.Enum, text) {
			return 0, nil
		}
		return 0, fmt.Errorf("plugin usage enum is not an allowed value")
	}
	if schema.Type == "boolean" {
		if _, ok := value.(bool); !ok {
			return 0, fmt.Errorf("plugin usage value must be a boolean")
		}
		return 0, nil
	}
	number, ok := UsageNumber(value, allowNumericString)
	if !ok {
		return 0, fmt.Errorf("plugin usage value must be a number")
	}
	if schema.Unit == "character" {
		if err := ValidateUsageNumberLimit(number, common.MaxQuota); err != nil {
			return 0, err
		}
		if math.Trunc(number) != number {
			return 0, fmt.Errorf("plugin character usage must be an integer")
		}
		return number, nil
	}
	if schema.Unit == "token" || schema.Unit == "credit" {
		if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return 0, fmt.Errorf("plugin usage value must be a finite non-negative number")
		}
		if quota, clamp := common.QuotaFromFloatChecked(number); clamp != nil {
			return float64(quota), nil
		}
		return number, nil
	}
	limit := relaycommon.MaxTaskDurationSeconds
	if schema.Unit == "count" {
		limit = kitdto.MaxImageN
	}
	if err := ValidateUsageNumberLimit(number, limit); err != nil {
		return 0, err
	}
	return number, nil
}

func ValidateUsageNumberLimit(number float64, limit int) error {
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return fmt.Errorf("plugin usage value must be a finite non-negative number")
	}
	if number > float64(limit) {
		return fmt.Errorf("plugin usage value exceeds the host limit")
	}
	return nil
}

func UsageNumber(value any, allowNumericString bool) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int64:
		return float64(number), true
	case int:
		return float64(number), true
	case string:
		if !allowNumericString {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func CanonicalUsageLimit(key string) (int, bool) {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
	switch normalized {
	case "duration", "durationseconds", "second", "seconds":
		return relaycommon.MaxTaskDurationSeconds, true
	case "n", "count", "imagecount", "samplecount", "batchcount", "numimages":
		return kitdto.MaxImageN, true
	default:
		return 0, false
	}
}
