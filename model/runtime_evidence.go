package model

import (
	"github.com/QuantumNous/new-api/common"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// RuntimeEvidence is credential-free provenance. Missing provider facts remain
// missing; request selectors are distinct from authoritative terminal facts.
type RuntimeEvidence struct {
	ClientModel       string         `json:"client_model"`
	ChannelID         int            `json:"channel_id"`
	ChannelType       int            `json:"channel_type"`
	PluginKey         string         `json:"plugin_key,omitempty"`
	UpstreamModel     string         `json:"upstream_model"`
	UpstreamEndpoint  string         `json:"upstream_endpoint,omitempty"`
	Protocol          string         `json:"protocol,omitempty"`
	Endpoint          string         `json:"endpoint,omitempty"`
	LocalRequestID    string         `json:"local_request_id,omitempty"`
	RequestID         string         `json:"request_id,omitempty"`
	TraceID           string         `json:"x_gateway_trace,omitempty"`
	TerminalRequestID string         `json:"terminal_request_id,omitempty"`
	TerminalTraceID   string         `json:"terminal_x_gateway_trace,omitempty"`
	TaskID            string         `json:"task_id,omitempty"`
	SubmittedAt       int64          `json:"submitted_at"`
	Requested         map[string]any `json:"requested,omitempty"`
	Terminal          map[string]any `json:"terminal,omitempty"`
	HostStatus        string         `json:"host_status,omitempty"`
	ValidatedUsage    map[string]any `json:"validated_usage,omitempty"`
}

// RuntimeBillingFacts copies only known scalar billing fields. Unknown nested
// keys, prompts, headers and media data are never retained in this projection.
func RuntimeBillingFacts(data []byte) map[string]any {
	var input map[string]any
	if len(data) > 1<<20 || common.Unmarshal(data, &input) != nil {
		return nil
	}
	output := map[string]any{}
	for _, key := range []string{"model", "status", "resolution", "service_tier", "quality", "size", "mode", "tier", "unit_type", "seconds", "generation_count", "music_count", "video_tokens", "image_tokens", "output_image_count", "upscale", "duration_sec", "input_video_duration_sec", "characters", "unit_count", "completion_tokens", "total_tokens", "input_tokens", "output_tokens", "prompt_tokens", "cached_tokens", "cache_creation_input_tokens", "cached_creation_tokens", "claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens", "ephemeral_5m_input_tokens", "ephemeral_1h_input_tokens", "usage_semantic", "audio_tokens", "reasoning_tokens", "text_tokens", "cache_read_input_tokens", "cache_creation_5m_input_tokens", "cache_creation_1h_input_tokens", "server_tool_calls", "image_count", "fast_mode", "input_video_present"} {
		value, ok := input[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case float64, bool:
			output[key] = v
		case string:
			if len(v) > 128 || strings.Contains(v, "://") || strings.HasPrefix(v, "Bearer ") {
				continue
			}
			switch key {
			case "model", "status", "resolution", "service_tier", "quality", "size", "mode", "tier", "usage_semantic", "unit_type":
				output[key] = v
			default:
				if number, err := strconv.ParseFloat(v, 64); err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
					output[key] = v
				}
			}

		}
	}
	for _, key := range []string{"usage", "cache_creation", "input_tokens_details", "output_tokens_details", "prompt_tokens_details", "completion_tokens_details"} {
		if child, ok := input[key].(map[string]any); ok {
			body, err := common.Marshal(child)
			if err == nil {
				output[key] = RuntimeBillingFacts(body)
			}
		}
	}
	for _, key := range []string{"input_video", "input_video_url", "reference_video", "reference_video_url"} {
		if value, ok := input[key]; ok {
			switch v := value.(type) {
			case string:
				output["input_video_present"] = v != ""
			case bool:
				output["input_video_present"] = v
			}
		}
	}
	return output
}

func CaptureTaskRuntime(task *Task, headers http.Header, body []byte) {
	if task == nil || task.PrivateData.Execution == nil || task.PrivateData.Execution.Passive == nil {
		return
	}
	capture := task.PrivateData.Execution.Passive
	if id := headers.Get("x-request-id"); id != "" {
		capture.TerminalRequestID = id
	}
	if id := headers.Get("x-gateway-trace"); id != "" {
		capture.TerminalTraceID = id
	}
	capture.Terminal = RuntimeBillingFacts(body)
	if capture.Terminal == nil {
		capture.Terminal = map[string]any{}
	}
}

func CaptureTaskCompletionFacts(task *Task, status string, usage map[string]any) {
	if task == nil || task.PrivateData.Execution == nil || task.PrivateData.Execution.Passive == nil {
		return
	}
	capture := task.PrivateData.Execution.Passive
	capture.HostStatus = status
	body, err := common.Marshal(usage)
	if err == nil {
		capture.ValidatedUsage = RuntimeBillingFacts(body)
	}
}
