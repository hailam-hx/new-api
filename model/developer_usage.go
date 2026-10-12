package model

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/shopspring/decimal"
)

const developerUsageRowLimit = 10000

type DeveloperUsageQuery struct {
	UserID, TokenID    int
	StartTime, EndTime int64
	Model              string
}

type DeveloperUsageData struct {
	Consumption, Refunds       decimal.Decimal
	BillingEvents, ErrorEvents int64
	Components                 []dto.DeveloperUsageComponent
	ComponentStatus            string
}

type developerUsageMetadata struct {
	TaskID                string                     `json:"task_id"`
	IsTask                bool                       `json:"is_task"`
	SettlementVerified    bool                       `json:"settlement_verified"`
	ActualQuota           json.RawMessage            `json:"actual_quota"`
	UsageFacts            map[string]json.RawMessage `json:"usage_facts"`
	UsageUnits            map[string]string          `json:"usage_units"`
	BillingTokens         map[string]json.RawMessage `json:"billing_tokens"`
	CacheReadTokens       json.RawMessage            `json:"cache_tokens"`
	CacheWriteTokens      json.RawMessage            `json:"cache_write_tokens"`
	CacheCreationTokens   json.RawMessage            `json:"cache_creation_tokens"`
	CacheCreation5mTokens json.RawMessage            `json:"cache_creation_tokens_5m"`
	CacheCreation1hTokens json.RawMessage            `json:"cache_creation_tokens_1h"`
	RequestPath           string                     `json:"request_path"`
	ViolationFee          bool                       `json:"violation_fee"`
	BillingState          string                     `json:"billing_state"`
	Admin                 struct {
		TaskRecordID json.RawMessage `json:"task_record_id"`
		BillingPath  string          `json:"usage_billing_path"`
	} `json:"admin_info"`
}

type developerUsageDimension struct{ subject, unit string }
type developerUsageTaskSnapshot struct {
	createdAt  int64
	logID      int
	quantities map[developerUsageDimension]decimal.Decimal
	incomplete bool
}

// GetDeveloperKeyUsage aggregates only retained events owned by this principal.
// The independent log store is not treated as an authoritative wallet ledger.
func GetDeveloperKeyUsage(ctx context.Context, query DeveloperUsageQuery) (DeveloperUsageData, error) {
	result := DeveloperUsageData{Components: make([]dto.DeveloperUsageComponent, 0), ComponentStatus: "available"}
	if query.UserID <= 0 || query.TokenID <= 0 || query.StartTime < 0 || query.EndTime <= query.StartTime || query.EndTime-query.StartTime > 31*24*60*60 {
		return result, errors.New("invalid key usage scope")
	}
	base := LOG_DB.WithContext(ctx).Model(&Log{}).Where("user_id = ? AND token_id = ? AND created_at >= ? AND created_at < ?", query.UserID, query.TokenID, query.StartTime, query.EndTime)
	if query.Model != "" {
		base = base.Where("model_name = ?", query.Model)
	}
	var totals struct {
		Consumption   decimal.Decimal
		Refunds       decimal.Decimal
		BillingEvents int64
		ErrorEvents   int64
		InvalidEvents int64
	}
	if err := base.Select(`COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS consumption,
		COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS refunds,
		COALESCE(SUM(CASE WHEN type IN (?, ?) THEN 1 ELSE 0 END), 0) AS billing_events,
		COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS error_events,
		COALESCE(SUM(CASE WHEN type IN (?, ?) AND quota < 0 THEN 1 ELSE 0 END), 0) AS invalid_events`,
		LogTypeConsume, LogTypeRefund, LogTypeConsume, LogTypeRefund, LogTypeError, LogTypeConsume, LogTypeRefund).Scan(&totals).Error; err != nil {
		return result, err
	}
	if totals.InvalidEvents > 0 {
		return result, errors.New("invalid recorded billing amount")
	}
	result.Consumption, result.Refunds = totals.Consumption, totals.Refunds
	result.BillingEvents, result.ErrorEvents = totals.BillingEvents, totals.ErrorEvents
	rows, err := base.Select("id, created_at, type, request_id, prompt_tokens, completion_tokens, other, content").Where("type IN ?", []int{LogTypeConsume, LogTypeRefund, LogTypeSystem}).Order("created_at DESC").Order("id DESC").Limit(developerUsageRowLimit + 1).Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()
	quantities := make(map[developerUsageDimension]decimal.Decimal)
	tasks := make(map[int64]developerUsageTaskSnapshot)
	incomplete, seen := false, 0
	for rows.Next() {
		seen++
		if seen > developerUsageRowLimit {
			incomplete = true
			break
		}
		var row Log
		if err := LOG_DB.ScanRows(rows, &row); err != nil {
			return result, err
		}
		var metadata developerUsageMetadata
		if len(row.Other) > 65536 || common.UnmarshalJsonStr(row.Other, &metadata) != nil {
			if row.Type != LogTypeSystem || strings.HasPrefix(row.Content, "BILLING_RESERVATION_") && row.Content != "BILLING_RESERVATION_SETTLED" && row.Content != "BILLING_RESERVATION_REFUNDED" {
				incomplete = true
			}
			continue
		}
		if row.Type == LogTypeSystem {
			switch metadata.BillingState {
			case "CLAIMED", "HELD", "PENDING", "SETTLING", "REFUNDING", "QUARANTINED":
				incomplete = true
			}
			if strings.HasPrefix(row.Content, "BILLING_RESERVATION_") && metadata.BillingState == "" && row.Content != "BILLING_RESERVATION_SETTLED" && row.Content != "BILLING_RESERVATION_REFUNDED" {
				incomplete = true
			}
			continue
		}
		if metadata.IsTask || metadata.TaskID != "" || len(metadata.Admin.TaskRecordID) > 0 {
			recordID, err := strconv.ParseInt(string(metadata.Admin.TaskRecordID), 10, 64)
			if err != nil || recordID <= 0 || row.Id <= 0 {
				incomplete = true
				continue
			}
			if latest, exists := tasks[recordID]; exists {
				if latest.createdAt == row.CreatedAt && latest.logID == row.Id {
					latest.quantities = nil
					latest.incomplete = true
					tasks[recordID] = latest
				}
				continue
			}
			snapshot := developerUsageTaskSnapshot{createdAt: row.CreatedAt, logID: row.Id, incomplete: true}
			_, actualQuotaPresent := developerUsageQuantity(metadata.ActualQuota, "credit")
			if metadata.SettlementVerified || actualQuotaPresent {
				switch metadata.BillingState {
				case "CLAIMED", "HELD", "PENDING", "SETTLING", "REFUNDING", "QUARANTINED":
				default:
					if row.Type != LogTypeRefund || actualQuotaPresent {
						snapshot.quantities = make(map[developerUsageDimension]decimal.Decimal)
						snapshot.incomplete = len(metadata.UsageFacts) == 0 || len(metadata.UsageUnits) == 0
						for subject, raw := range metadata.UsageFacts {
							unit := metadata.UsageUnits[subject]
							if unit == "" && (len(raw) > 0 && raw[0] == '"' || string(raw) == "true" || string(raw) == "false") {
								continue
							}
							if !developerUsageSubjectAllowed(subject) {
								snapshot.incomplete = true
								continue
							}
							quantity, valid := developerUsageQuantity(raw, unit)
							if !valid {
								snapshot.incomplete = true
								continue
							}
							snapshot.quantities[developerUsageDimension{subject, unit}] = quantity
						}
					}
				}
			}
			tasks[recordID] = snapshot
			continue
		}
		if metadata.ViolationFee {
			continue
		}
		if row.Type != LogTypeConsume || row.RequestId == "" || !slices.Contains([]string{"/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/messages"}, metadata.RequestPath) || !slices.Contains([]string{"billing-usage-openai", "billing-usage-anthropic", "billing-usage-gemini"}, metadata.Admin.BillingPath) || row.PromptTokens < 0 || row.CompletionTokens < 0 {
			incomplete = true
			continue
		}
		if len(metadata.BillingTokens) > 0 {
			for key, subject := range map[string]string{"p": "billable_input_tokens", "c": "billable_output_tokens", "cr": "cache_read_tokens", "cc": "cache_creation_tokens", "cc1h": "cache_creation_1h_tokens", "img": "image_input_tokens", "img_cr": "image_cache_tokens", "img_o": "image_output_tokens", "ai": "audio_input_tokens", "ao": "audio_output_tokens"} {
				raw, exists := metadata.BillingTokens[key]
				if !exists {
					continue
				}
				quantity, valid := developerUsageQuantity(raw, "token")
				if !valid {
					incomplete = true
					continue
				}
				dimension := developerUsageDimension{subject, "token"}
				quantities[dimension] = quantities[dimension].Add(quantity)
			}
			continue
		}
		cacheQuantities, cacheIncomplete := developerUsageCacheQuantities(metadata)
		incomplete = incomplete || cacheIncomplete
		for dimension, quantity := range cacheQuantities {
			quantities[dimension] = quantities[dimension].Add(quantity)
		}
		input, output := developerUsageDimension{"input_tokens", "token"}, developerUsageDimension{"output_tokens", "token"}
		quantities[input] = quantities[input].Add(decimal.NewFromInt(int64(row.PromptTokens)))
		quantities[output] = quantities[output].Add(decimal.NewFromInt(int64(row.CompletionTokens)))
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	for _, snapshot := range tasks {
		incomplete = incomplete || snapshot.incomplete
		for dimension, quantity := range snapshot.quantities {
			quantities[dimension] = quantities[dimension].Add(quantity)
		}
	}
	for dimension, quantity := range quantities {
		result.Components = append(result.Components, dto.DeveloperUsageComponent{Subject: dimension.subject, Unit: dimension.unit, Quantity: quantity.String()})
	}
	slices.SortFunc(result.Components, func(a, b dto.DeveloperUsageComponent) int {
		if order := strings.Compare(a.Subject, b.Subject); order != 0 {
			return order
		}
		return strings.Compare(a.Unit, b.Unit)
	})
	if incomplete {
		result.ComponentStatus = "partial"
		if len(result.Components) == 0 {
			result.ComponentStatus = "unknown"
		}
	}
	return result, nil
}

// developerUsageQuantity accepts only bounded numeric quantities with a proven
// host unit. It never interprets enum/boolean facts or guesses units from names.
func developerUsageQuantity(raw json.RawMessage, unit string) (decimal.Decimal, bool) {
	limit := int64(2147483647)
	switch unit {
	case "second":
		limit = 3600
	case "count":
		limit = 128
	case "token", "credit", "character":
	default:
		return decimal.Zero, false
	}
	if len(raw) == 0 || len(raw) > 64 {
		return decimal.Zero, false
	}
	value, err := decimal.NewFromString(string(raw))
	if err != nil || value.Exponent() < -18 || value.Exponent() > 10 || value.IsNegative() || value.GreaterThan(decimal.NewFromInt(limit)) {
		return decimal.Zero, false
	}
	if unit == "character" && !value.Equal(value.Truncate(0)) {
		return decimal.Zero, false
	}
	return value, true
}

// developerUsageSubjectAllowed restricts public quantity dimensions to safe
// identifiers. A frozen unit does not authorize exposing private billing fields.
func developerUsageSubjectAllowed(subject string) bool {
	if len(subject) == 0 || len(subject) > 64 {
		return false
	}
	name := strings.ToLower(subject)
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_') {
			return false
		}
	}
	if strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "_ids") || strings.HasSuffix(name, "ratio") || strings.HasSuffix(name, "cost") || strings.HasSuffix(name, "price") {
		return false
	}
	if slices.Contains([]string{"userid", "tokenid", "channelid", "apikey", "tokenkey", "access_token", "refresh_token", "billing_context", "private_data"}, name) {
		return false
	}
	for part := range strings.SplitSeq(name, "_") {
		if slices.Contains([]string{"key", "keys", "secret", "secrets", "credential", "credentials", "authorization", "password", "private", "internal", "session", "ratio", "ratios", "cost", "price"}, part) {
			return false
		}
	}
	return true
}

// developerUsageCacheQuantities preserves recorded cache facts without adding
// an aggregate to its TTL breakdown or inventing missing zero-valued categories.
func developerUsageCacheQuantities(metadata developerUsageMetadata) (map[developerUsageDimension]decimal.Decimal, bool) {
	quantities := make(map[developerUsageDimension]decimal.Decimal)
	incomplete := false
	if len(metadata.CacheReadTokens) > 0 {
		value, valid := developerUsageQuantity(metadata.CacheReadTokens, "token")
		if valid {
			quantities[developerUsageDimension{"cache_read_tokens", "token"}] = value
		} else {
			incomplete = true
		}
	}
	splits := make(map[developerUsageDimension]decimal.Decimal)
	splitTotal := decimal.Zero
	for subject, raw := range map[string]json.RawMessage{"cache_creation_5m_tokens": metadata.CacheCreation5mTokens, "cache_creation_1h_tokens": metadata.CacheCreation1hTokens} {
		if len(raw) == 0 {
			continue
		}
		value, valid := developerUsageQuantity(raw, "token")
		if !valid {
			incomplete = true
			continue
		}
		splits[developerUsageDimension{subject, "token"}] = value
		splitTotal = splitTotal.Add(value)
	}
	// The normalized host total has precedence over the legacy aggregate.
	totalRaw := metadata.CacheWriteTokens
	if len(totalRaw) == 0 {
		totalRaw = metadata.CacheCreationTokens
	}
	if len(totalRaw) > 0 {
		total, valid := developerUsageQuantity(totalRaw, "token")
		if !valid || total.LessThan(splitTotal) {
			return quantities, true
		}
		quantities[developerUsageDimension{"cache_creation_tokens", "token"}] = total
		return quantities, incomplete
	}
	for dimension, value := range splits {
		quantities[dimension] = value
	}
	return quantities, incomplete
}
