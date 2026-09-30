package dflop

import (
	"errors"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// DFLOPCacheContract rejects a capability not priced by the verified 5m
// provider contract. It does not impose Anthropic capabilities on other hosts.
func DFLOPCacheContract(baseURL, model string, body []byte, usage *dto.Usage) error {
	if !DFLOPCacheContractApplies(baseURL, model) {
		return nil
	}
	if usage != nil && usage.ClaudeCacheCreation1hTokens > 0 {
		return errors.New("UNSUPPORTED_DFLOP_CACHE_TTL_1H")
	}
	var value any
	if len(body) == 0 {
		return nil
	}
	if err := common.Unmarshal(body, &value); err != nil {
		return errors.New("DFLOP_CACHE_REQUEST_INVALID")
	}
	queue := []any{value}
	for len(queue) > 0 {
		node := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		switch v := node.(type) {
		case map[string]any:
			if control, ok := v["cache_control"].(map[string]any); ok && control["ttl"] == "1h" {
				return errors.New("UNSUPPORTED_DFLOP_CACHE_TTL_1H")
			}
			for _, child := range v {
				queue = append(queue, child)
			}
		case []any:
			queue = append(queue, v...)
		}
	}
	return nil
}

type RuntimeBinding struct {
	TaskID                string         `json:"task_id"`
	RequestID             string         `json:"dflop_request_id"`
	Model                 string         `json:"dflop_canonical_model"`
	LocalTaskID           int64          `json:"new_api_task_record,omitempty"`
	LocalLogID            int            `json:"new_api_log_record,omitempty"`
	ChannelID             int            `json:"channel_id"`
	PluginKey             string         `json:"plugin_key,omitempty"`
	ClientModel           string         `json:"client_model,omitempty"`
	UpstreamModel         string         `json:"upstream_model,omitempty"`
	Confidence            string         `json:"binding_confidence"`
	RuntimeFields         map[string]any `json:"runtime_fields"`
	BillingFeatures       []string       `json:"catalog_billing_features"`
	BindingVerified       bool           `json:"binding_verified"`
	RuntimeUsageVerified  bool           `json:"runtime_usage_verified"`
	SettledCostReconciled bool           `json:"settled_cost_reconciled"`
	SettlementVerified    bool           `json:"settlement_verified"`
}

type LocalExecution struct {
	TaskRecordID                                                                                         int64
	LogRecordID                                                                                          int
	TaskID, UpstreamTaskID, RequestID, UpstreamRequestID, TraceID, PluginKey, ClientModel, UpstreamModel string
	ChannelID                                                                                            int
}

// CorrelateRuntimeBinding never uses model/vendor similarity. Conflicting
// records or a different source channel cannot establish an execution path.
func CorrelateRuntimeBinding(taskID, requestID, model string, channelID int, records []LocalExecution, traceIDs ...string) RuntimeBinding {
	result := RuntimeBinding{TaskID: taskID, RequestID: requestID, Model: model, ChannelID: channelID, Confidence: "UNVERIFIED"}
	for _, record := range records {
		if record.ChannelID != channelID || taskID != "" && record.UpstreamTaskID != "" && record.UpstreamTaskID != taskID && record.TaskID != taskID {
			continue
		}
		confidence := ""
		if taskID != "" && record.UpstreamTaskID == taskID {
			confidence = "EXACT_TASK_ID"
		} else if taskID != "" && record.TaskID == taskID {
			confidence = "EXACT_TASK_ID"
		} else if requestID != "" && (record.UpstreamRequestID == requestID || record.RequestID == requestID) {
			confidence = "EXACT_REQUEST_ID"
		}
		if confidence == "" && record.TraceID != "" {
			for _, trace := range traceIDs {
				if trace != "" && trace == record.TraceID {
					confidence = "EXACT_TRACE_ID"
					break
				}
			}
		}
		if confidence == "" || record.UpstreamModel != model || record.ClientModel == "" {
			continue
		}
		if result.Confidence != "UNVERIFIED" {
			if result.ClientModel != record.ClientModel || result.PluginKey != "" && record.PluginKey != "" && result.PluginKey != record.PluginKey || result.LocalTaskID != 0 && record.TaskRecordID != 0 && result.LocalTaskID != record.TaskRecordID || result.LocalLogID != 0 && record.LogRecordID != 0 && result.LocalLogID != record.LogRecordID {
				return RuntimeBinding{TaskID: taskID, RequestID: requestID, Model: model, ChannelID: channelID, Confidence: "UNVERIFIED"}
			}
		}
		if result.Confidence == "UNVERIFIED" || confidence == "EXACT_TASK_ID" || result.Confidence == "EXACT_TRACE_ID" {
			result.Confidence = confidence
		}
		if record.TaskRecordID != 0 {
			result.LocalTaskID = record.TaskRecordID
		}
		if record.LogRecordID != 0 {
			result.LocalLogID = record.LogRecordID
		}
		if record.PluginKey != "" {
			result.PluginKey = record.PluginKey
		}
		result.ClientModel, result.UpstreamModel = record.ClientModel, record.UpstreamModel
		if result.RequestID == "" {
			result.RequestID = record.UpstreamRequestID
		}
		result.BindingVerified = result.PluginKey != "" || result.LocalLogID != 0
	}
	return result
}

func DFLOPCacheContractApplies(baseURL, model string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "api.dflop.top" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && (parsed.Path == "" || parsed.Path == "/") && strings.Contains(strings.ToLower(model), "claude")
}

// EndpointBillingMatrix keeps catalog components scoped to their documented
// protocol. Unproven selectors never become a global additive expression.
func EndpointBillingMatrix(items []Item) []map[string]any {
	matrix := []map[string]any{}
	for _, item := range items {
		if !strings.HasPrefix(item.ModelID, "gpt-") {
			continue
		}
		protocols := []string{"openai_chat", "openai_responses"}
		if item.Category == "image" {
			protocols = []string{"openai_image"}
		} else if item.Category != "text" {
			continue
		}
		for _, protocol := range protocols {
			endpoint := map[string]string{"openai_chat": "/v1/chat/completions", "openai_responses": "/v1/responses", "openai_image": "/v1/images/generations"}[protocol]
			applicable := []string{"token"}
			unverified := []string{"fast_mode", "per_image"}
			fields := []string{"input_tokens", "output_tokens", "cached_tokens", "image_generation_call", "service_tier"}
			if protocol == "openai_image" {
				applicable = item.BillingFeatures
				unverified = []string{"selected_quality", "selected_size"}
				fields = []string{"images", "usage", "quality", "size"}
			}
			matrix = append(matrix, map[string]any{"model": item.ModelID, "protocol": protocol, "endpoint": endpoint, "catalog_features": item.BillingFeatures, "applicable_features": applicable, "unverified_selectors": unverified, "runtime_selector": "UNVERIFIED", "usage_fields": fields, "global_additive_billing_verified": false})
		}
	}
	return matrix
}
