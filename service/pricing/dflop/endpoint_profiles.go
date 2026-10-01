package dflop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
)

// EndpointBillingProfile describes one route contract. Verified mechanics refer
// to the documented schema and production billing path, not captured live calls.
// The scope is encoded in the expression's tier names so ordinary pricing
// ownership, version, drift detection and rollback also cover this contract.
type EndpointBillingProfile struct {
	Model                string   `json:"model"`
	CanonicalModel       string   `json:"canonical_model"`
	Profile              string   `json:"profile"`
	Endpoint             string   `json:"endpoint"`
	Protocol             string   `json:"protocol"`
	Adapter              string   `json:"adapter"`
	Expression           string   `json:"expression,omitempty"`
	Status               string   `json:"status"`
	ReasonCode           string   `json:"reason_code,omitempty"`
	ApplicableFeatures   []string `json:"applicable_features"`
	RequiredFacts        []string `json:"required_facts"`
	PriceVerified        bool     `json:"price_verified"`
	SemanticsVerified    bool     `json:"semantics_verified"`
	BindingVerified      bool     `json:"binding_verified"`
	RuntimeUsageVerified bool     `json:"runtime_usage_verified"`
	SettlementVerified   bool     `json:"settlement_verified"`
	EvidenceBasis        string   `json:"evidence_basis"`
}

func BuildEndpointBillingProfiles(item Item) []EndpointBillingProfile {
	if item.Category != "text" || !(strings.HasPrefix(item.ModelID, "gpt-") || strings.HasPrefix(item.ModelID, "grok-") || item.ModelID == "codex-auto-review") {
		return nil
	}
	scope := "dflop_chat_standard"
	if strings.HasPrefix(item.ModelID, "grok-") {
		scope = "dflop_chat_token_only"
	}
	if slices.Contains(item.Protocols, "openai_responses") {
		scope += "_responses"
	}
	standard := EndpointBillingProfile{Model: item.ModelID, CanonicalModel: item.CanonicalID, Profile: "chat_standard", Endpoint: "/v1/chat/completions", Protocol: "openai_chat", Adapter: "openai", Status: UnsupportedMapping, ApplicableFeatures: []string{"token"}, RequiredFacts: []string{"prompt_tokens", "completion_tokens", "cached_tokens"}, EvidenceBasis: "OFFICIAL_CONTRACT_AND_PRODUCTION_SCHEMA"}
	standard.PriceVerified = item.PriceSemantics.SourcePriceKind == "AUTHENTICATED_EFFECTIVE_PRICE" && item.PriceSemantics.EffectiveState == "VERIFIED"
	standard.SemanticsVerified = slices.Contains(item.BillingFeatures, "token")
	for _, feature := range item.BillingFeatures {
		if !slices.Contains([]string{"token", "token_long_context", "per_image", "fast_mode", "server_tool_call"}, feature) {
			standard.SemanticsVerified = false
			standard.ReasonCode = "UNKNOWN_BILLING_FEATURE"
		}
	}
	standard.BindingVerified = item.Callable && slices.Contains(item.Protocols, "openai_chat")
	parts := make([]string, 0, 3)
	for _, component := range []struct{ key, variable string }{{"input_per_1m", "p"}, {"output_per_1m", "c"}, {"cached_input_per_1m", "cr"}} {
		price, exists := item.Prices[component.key]
		if !exists {
			if component.variable == "cr" {
				continue
			}
			standard.PriceVerified = false
			break
		}
		amount, err := decimal.NewFromString(price.SellingUSD)
		if err != nil || amount.IsNegative() {
			standard.PriceVerified = false
			break
		}
		parts = append(parts, component.variable+" * "+amount.String())
	}
	if len(parts) >= 2 {
		standard.Expression = fmt.Sprintf("tier(%q, %s)", scope, strings.Join(parts, " + "))
	}
	hasLong := false
	for _, key := range []string{"input_per_1m_long", "output_per_1m_long", "cached_input_per_1m_long"} {
		if _, ok := item.Prices[key]; ok {
			hasLong = true
		}
	}
	if hasLong {
		var source struct {
			Threshold *int `json:"long_context_threshold_tokens"`
		}
		if common.Unmarshal(item.Raw, &source) != nil || source.Threshold == nil || *source.Threshold <= 0 {
			standard.SemanticsVerified = false
			standard.ReasonCode = "MISSING_SELECTED_TIER"
		} else {
			longParts := make([]string, 0, 3)
			for _, component := range []struct{ key, variable string }{{"input_per_1m_long", "p"}, {"output_per_1m_long", "c"}, {"cached_input_per_1m_long", "cr"}} {
				price, ok := item.Prices[component.key]
				if !ok {
					if component.variable == "cr" {
						if _, cached := item.Prices["cached_input_per_1m"]; !cached {
							continue
						}
					}
					standard.PriceVerified = false
					break
				}
				amount, err := decimal.NewFromString(price.SellingUSD)
				if err != nil || amount.IsNegative() {
					standard.PriceVerified = false
					break
				}
				longParts = append(longParts, component.variable+" * "+amount.String())
			}
			if len(longParts) >= 2 {
				standard.Expression = fmt.Sprintf("len >= %d ? tier(%q, %s) : %s", *source.Threshold, scope+"_long", strings.Join(longParts, " + "), standard.Expression)
			}
		}
	}
	if standard.Expression != "" && standard.PriceVerified && standard.SemanticsVerified && standard.BindingVerified {
		// Actual token and cached-token fields already pass through the OpenAI
		// JSON/SSE normalizer and the snapshot-based expression settlement path.
		if _, err := billingexpr.CompileFromCache(standard.Expression); err == nil {
			standard.RuntimeUsageVerified = true
			standard.SettlementVerified = true
			standard.Status = SupportedAuto
		}
	}
	if standard.Status != SupportedAuto && standard.ReasonCode == "" {
		standard.ReasonCode = "PROVIDER_CONTRACT_REQUIRED"
		standard.Expression = ""
	}
	if standard.Status != SupportedAuto {
		standard.Expression = ""
	}
	profiles := []EndpointBillingProfile{standard}
	if slices.Contains(item.Protocols, "openai_responses") {
		responses := standard
		responses.Profile, responses.Endpoint, responses.Protocol = "responses", "/v1/responses", "openai_responses"
		responses.RequiredFacts = []string{"input_tokens", "output_tokens", "input_tokens_details.cached_tokens"}
		responses.EvidenceBasis = "OFFICIAL_RESPONSES_CONTRACT_AND_PRODUCTION_SCHEMA"

		profiles = append(profiles, responses)
	}
	if slices.Contains(item.BillingFeatures, "fast_mode") {
		profiles = append(profiles, EndpointBillingProfile{Model: item.ModelID, CanonicalModel: item.CanonicalID, Profile: "chat_fast", Endpoint: standard.Endpoint, Protocol: standard.Protocol, Adapter: "openai", Status: UnsupportedMapping, ReasonCode: "MISSING_FAST_SELECTOR", ApplicableFeatures: []string{"token", "fast_mode"}, RequiredFacts: []string{"served_service_tier"}, PriceVerified: standard.PriceVerified, SemanticsVerified: true, BindingVerified: standard.BindingVerified, EvidenceBasis: "PRIORITY_REQUEST_MAY_FALL_BACK_TO_STANDARD"})
	}
	if _, ok := item.Prices["price_per_image"]; ok {
		profiles = append(profiles, EndpointBillingProfile{Model: item.ModelID, CanonicalModel: item.CanonicalID, Profile: "dedicated_image", Endpoint: "/v1/images/generations", Protocol: "openai_image", Adapter: "openai", Status: UnsupportedMapping, ReasonCode: "DEDICATED_IMAGE_BINDING_MISSING", ApplicableFeatures: []string{"per_image"}, RequiredFacts: []string{"actual_output_count", "exact_image_model_binding"}, PriceVerified: standard.PriceVerified, EvidenceBasis: "IMAGE_ENDPOINT_MODEL_BINDING_REQUIRED"})
	}
	if _, ok := item.Prices["price_per_server_tool_call"]; ok {
		profiles = append(profiles, EndpointBillingProfile{Model: item.ModelID, CanonicalModel: item.CanonicalID, Profile: "server_tools", Endpoint: standard.Endpoint, Protocol: standard.Protocol, Adapter: "openai", Status: UnsupportedMapping, ReasonCode: "SERVER_TOOL_CALLS_UNBOUNDED", ApplicableFeatures: []string{"token", "server_tools"}, RequiredFacts: []string{"num_server_side_tools_used"}, PriceVerified: standard.PriceVerified, SemanticsVerified: true, BindingVerified: standard.BindingVerified, EvidenceBasis: "OFFICIAL_TOOL_USAGE_WITHOUT_PROVIDER_ENFORCED_MAX_CALLS"})
	}
	return profiles
}

func HasEndpointBillingProfile(expression string) bool {
	return strings.Contains(expression, `tier("dflop_chat_standard`) || strings.Contains(expression, `tier("dflop_chat_token_only`)
}

func HasResponsesEndpointBillingProfile(expression string) bool {
	return strings.Contains(expression, `tier("dflop_chat_standard_responses`) || strings.Contains(expression, `tier("dflop_chat_token_only_responses`)
}

func DFLOPEndpointSourceApplies(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "api.dflop.top" && parsed.User == nil && parsed.RawPath == "" && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && (parsed.Path == "" || parsed.Path == "/")
}

func DFLOPEndpointProfileApplies(baseURL, expression string) bool {
	return DFLOPEndpointSourceApplies(baseURL) && HasEndpointBillingProfile(expression)
}

// DFLOPEndpointModelApplies scopes unavailable provider profiles even when the
// administrator has retained legacy or custom pricing without our tier marker.
func DFLOPEndpointModelApplies(baseURL, model string) bool {
	if !DFLOPEndpointSourceApplies(baseURL) {
		return false
	}
	return model == "codex-auto-review" || strings.HasPrefix(model, "gpt-") && !strings.HasPrefix(model, "gpt-image-") || strings.HasPrefix(model, "grok-") && !strings.HasPrefix(model, "grok-imagine-")
}

func DFLOPEndpointModelRequestContract(baseURL, model, expression, endpoint string, input billingexpr.RequestInput) error {
	applies := DFLOPEndpointModelApplies(baseURL, model) || DFLOPEndpointProfileApplies(baseURL, expression)
	var body map[string]any
	if common.Unmarshal(input.Body, &body) == nil {
		if outbound, ok := body["model"].(string); ok && outbound != "" {
			if applies && model != "" && outbound != model {
				return errors.New("DFLOP_BILLING_MODEL_MISMATCH")
			}
			model = outbound
		}
	}
	if !applies && !DFLOPEndpointModelApplies(baseURL, model) {
		return nil
	}
	if !HasEndpointBillingProfile(expression) {
		// The temporary marker only validates the legacy Chat request. It does not
		// alter or authorize a new endpoint's administrator pricing.
		expression = `tier("dflop_chat_token_only", 0)`
	}
	return DFLOPEndpointRequestContract(baseURL, expression, endpoint, input)
}

func DFLOPEndpointModelResponseContract(baseURL, model, expression string, body []byte) (bool, error) {
	if !DFLOPEndpointModelApplies(baseURL, model) && !DFLOPEndpointProfileApplies(baseURL, expression) {
		return false, nil
	}
	if !HasEndpointBillingProfile(expression) {
		expression = `tier("dflop_chat_token_only", 0)`
	}
	return DFLOPEndpointResponseContract(baseURL, expression, body)
}

// DFLOPEndpointRequestContract runs on the frozen request and on the final
// serialized outbound body, after channel parameter/header overrides.
func DFLOPEndpointRequestContract(baseURL, expression, endpoint string, input billingexpr.RequestInput) error {
	if !DFLOPEndpointProfileApplies(baseURL, expression) {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("DFLOP_BILLING_REQUEST_INVALID")
	}
	responses := parsed.Path == "/v1/responses"
	if responses && !HasResponsesEndpointBillingProfile(expression) {
		return errors.New("DFLOP_RESPONSES_PROFILE_UNVERIFIED")
	}
	if parsed.Path != "/v1/chat/completions" && !responses {
		return errors.New("DFLOP_DEDICATED_IMAGE_PROFILE_UNVERIFIED")
	}
	var body map[string]any
	if common.Unmarshal(input.Body, &body) != nil || body == nil {
		return errors.New("DFLOP_BILLING_REQUEST_INVALID")
	}
	queue := []map[string]any{body}
	for len(queue) > 0 {
		value := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, key := range []string{"extra_body", "parameters", "metadata"} {
			if nested, ok := value[key].(map[string]any); ok {
				queue = append(queue, nested)
			}
		}
		if tier, exists := value["service_tier"]; exists && tier != nil && tier != "" && tier != "default" {
			return errors.New("DFLOP_FAST_MODE_PRICING_UNAVAILABLE")
		}
		for _, key := range []string{"fast_mode", "fast", "speed"} {
			if mode, exists := value[key]; exists && mode != nil && mode != false && mode != "" {
				return errors.New("DFLOP_FAST_MODE_PRICING_UNAVAILABLE")
			}
		}
		for _, key := range []string{"search_parameters", "web_search_options", "server_tools"} {
			if _, exists := value[key]; exists {
				return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
			}
		}
		if choice, exists := value["tool_choice"]; exists && choice != nil {
			switch selected := choice.(type) {
			case string:
				if selected != "auto" && selected != "none" && selected != "required" {
					return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
				}
			case map[string]any:
				kind, _ := selected["type"].(string)
				if kind != "function" && !(kind == "image_generation" && strings.Contains(expression, `tier("dflop_chat_standard`) && (responses || value["stream"] == true)) {
					return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
				}
			default:
				return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
			}
		}
		if tools, exists := value["tools"]; exists && tools != nil {
			entries, ok := tools.([]any)
			if !ok {
				return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
			}
			for _, entry := range entries {
				tool, ok := entry.(map[string]any)
				if !ok {
					return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
				}
				kind, _ := tool["type"].(string)
				if kind == "function" {
					continue
				}
				if kind == "image_generation" && strings.Contains(expression, `tier("dflop_chat_standard`) && (responses || value["stream"] == true) {
					continue
				}
				return errors.New("DFLOP_SERVER_TOOL_UNBOUNDED")
			}
		}
	}
	for key, value := range input.Headers {
		lower := strings.ToLower(key)
		if (strings.Contains(lower, "tier") || strings.Contains(lower, "fast") || strings.Contains(lower, "speed")) && value != "" && value != "default" {
			return errors.New("DFLOP_FAST_MODE_PRICING_UNAVAILABLE")
		}
	}
	return nil
}

func DFLOPEndpointUsageContract(baseURL, expression string, params billingexpr.TokenParams) error {
	if !DFLOPEndpointProfileApplies(baseURL, expression) {
		return nil
	}
	if params.ServerToolCalls != nil && *params.ServerToolCalls != 0 {
		return errors.New("UNEXPECTED_DFLOP_SERVER_TOOL_USAGE")
	}
	return nil
}

// DFLOPEndpointResponseContract distinguishes absent token usage from an
// explicit zero and rejects a served tariff outside the frozen standard profile.
// SSE chunks without usage are normal; completion must have observed usage.
func DFLOPEndpointResponseContract(baseURL, expression string, body []byte) (bool, error) {
	if !DFLOPEndpointProfileApplies(baseURL, expression) {
		return false, nil
	}
	if len(body) == 0 || string(body) == "[DONE]" {
		return false, nil
	}
	var response struct {
		Response json.RawMessage `json:"response"`
		Item     *struct {
			Type string `json:"type"`
		} `json:"item"`
		Output []struct {
			Type string `json:"type"`
		} `json:"output"`
		Object      string  `json:"object"`
		ServiceTier *string `json:"service_tier"`
		Usage       *struct {
			InputTokens  *int `json:"input_tokens"`
			OutputTokens *int `json:"output_tokens"`
			InputDetails *struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			PromptDetails *struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
			ServerTools      *int `json:"num_server_side_tools_used"`
		} `json:"usage"`
	}
	if common.Unmarshal(body, &response) != nil {
		return false, errors.New("DFLOP_BILLING_RESPONSE_INVALID")
	}
	if response.Item != nil && response.Item.Type != "" && (response.Item.Type == "image_generation_call" && strings.Contains(expression, `tier("dflop_chat_token_only`) || !slices.Contains([]string{"message", "reasoning", "function_call", "image_generation_call"}, response.Item.Type)) {
		return false, errors.New("UNEXPECTED_DFLOP_SERVER_TOOL_USAGE")
	}
	for _, output := range response.Output {
		if output.Type == "image_generation_call" && strings.Contains(expression, `tier("dflop_chat_token_only`) || !slices.Contains([]string{"message", "reasoning", "function_call", "image_generation_call"}, output.Type) {
			return false, errors.New("UNEXPECTED_DFLOP_SERVER_TOOL_USAGE")
		}
	}
	if response.ServiceTier != nil && *response.ServiceTier != "" && *response.ServiceTier != "default" {
		return false, errors.New("UNEXPECTED_DFLOP_SELECTED_TIER")
	}
	if response.Usage != nil && response.Usage.ServerTools != nil && *response.Usage.ServerTools != 0 {
		return false, errors.New("UNEXPECTED_DFLOP_SERVER_TOOL_USAGE")
	}
	if len(response.Response) != 0 && string(response.Response) != "null" {
		return DFLOPEndpointResponseContract(baseURL, expression, response.Response)
	}
	if response.Object == "response" {
		if !HasResponsesEndpointBillingProfile(expression) {
			return false, errors.New("DFLOP_RESPONSES_PROFILE_UNVERIFIED")
		}
		if response.Usage != nil {
			response.Usage.PromptTokens, response.Usage.CompletionTokens = response.Usage.InputTokens, response.Usage.OutputTokens
			if billingexpr.UsedVars(expression)["cr"] && (response.Usage.InputDetails == nil || response.Usage.InputDetails.CachedTokens == nil) {
				return false, errors.New("MISSING_AUTHORITATIVE_CACHE_USAGE")
			}
			if response.Usage.InputDetails != nil && response.Usage.InputDetails.CachedTokens != nil && (*response.Usage.InputDetails.CachedTokens < 0 || response.Usage.InputTokens != nil && *response.Usage.InputDetails.CachedTokens > *response.Usage.InputTokens) {
				return false, errors.New("INVALID_AUTHORITATIVE_CACHE_USAGE")
			}
		}
	}
	if response.Usage == nil {
		return false, nil
	}
	if response.Object != "response" && billingexpr.UsedVars(expression)["cr"] {
		if response.Usage.PromptDetails == nil || response.Usage.PromptDetails.CachedTokens == nil {
			return false, errors.New("MISSING_AUTHORITATIVE_CACHE_USAGE")
		}
		cached := *response.Usage.PromptDetails.CachedTokens
		if cached < 0 || response.Usage.PromptTokens != nil && cached > *response.Usage.PromptTokens {
			return false, errors.New("INVALID_AUTHORITATIVE_CACHE_USAGE")
		}
	}
	if response.Usage.ServerTools != nil && *response.Usage.ServerTools != 0 {
		return false, errors.New("UNEXPECTED_DFLOP_SERVER_TOOL_USAGE")
	}
	if response.Usage.PromptTokens == nil || response.Usage.CompletionTokens == nil {
		return false, errors.New("MISSING_AUTHORITATIVE_TOKEN_USAGE")
	}
	if *response.Usage.PromptTokens < 0 || *response.Usage.CompletionTokens < 0 {
		return false, errors.New("INVALID_AUTHORITATIVE_TOKEN_USAGE")
	}
	return true, nil
}
