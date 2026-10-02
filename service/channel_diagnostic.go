package service

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/service/pricing/dflop"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// TaskChannelDiagnostic records configuration evidence, never upstream health.
// This service does not execute plugin hooks, acquire credentials, or submit tasks.
type TaskChannelDiagnostic struct {
	CatalogPricingState    string                `json:"catalog_pricing_state,omitempty"`
	ActivePricingState     string                `json:"active_pricing_state,omitempty"`
	QuantityState          string                `json:"quantity_state,omitempty"`
	FormulaState           string                `json:"formula_state,omitempty"`
	UsageProfile           string                `json:"usage_profile,omitempty"`
	RoutingStatus          string                `json:"routing_status,omitempty"`
	RuntimeAttempted       bool                  `json:"runtime_attempted"`
	OverallStatus          string                `json:"overall_status"`
	ResolvedUpstreamModel  string                `json:"resolved_upstream_model"`
	Endpoint               string                `json:"endpoint,omitempty"`
	Protocol               string                `json:"protocol,omitempty"`
	PricingState           string                `json:"pricing_state"`
	UsageSchemaState       string                `json:"usage_schema_state"`
	CredentialState        string                `json:"credential_state"`
	ConnectivityAvailable  bool                  `json:"connectivity_available"`
	ConnectivityKeyIndices []int                 `json:"connectivity_key_indices,omitempty"`
	ConnectivityStatus     string                `json:"connectivity_status,omitempty"`
	ConnectivityLatencyMS  int64                 `json:"connectivity_latency_ms"`
	CredentialIdentity     string                `json:"credential_identity,omitempty"`
	ModelAccess            string                `json:"model_access,omitempty"`
	CatalogModel           string                `json:"catalog_model,omitempty"`
	Kind                   string                `json:"kind"`
	Mode                   string                `json:"mode"`
	Status                 string                `json:"status"`
	Outcome                string                `json:"outcome"`
	Plugin                 string                `json:"plugin,omitempty"`
	PluginVersion          string                `json:"plugin_version,omitempty"`
	Generation             uint64                `json:"generation"`
	Model                  string                `json:"model"`
	MappedModel            string                `json:"mapped_model"`
	Operation              string                `json:"operation,omitempty"`
	ConnectivityTested     bool                  `json:"connectivity_tested"`
	LiveGenerationTested   bool                  `json:"live_generation_tested"`
	DiagnosticDurationMS   int64                 `json:"diagnostic_duration_ms"`
	Checks                 []TaskDiagnosticCheck `json:"checks"`
}

type TaskDiagnosticCheck struct {
	Check         string `json:"check"`
	Status        string `json:"status"`
	ReasonCode    string `json:"reason_code,omitempty"`
	Message       string `json:"message,omitempty"`
	CheckType     string `json:"check_type,omitempty"`
	Reason        string `json:"reason,omitempty"`
	BillingSource string `json:"billing_source,omitempty"`
	Evidence      string `json:"evidence,omitempty"`
}

// Fail records a concrete validation failure. Not-tested checks never use it.
func (d *TaskChannelDiagnostic) Fail(check, status, reason string) {
	d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: check, Status: "fail", Reason: reason})
	// Keep the first concrete blocker, especially the existing pricing reason.
	if d.Outcome != "fail" {
		d.Status = status
		d.Outcome = "fail"
	}
}

// EvaluateTaskChannelConfiguration consumes production selection and pricing
// results. API v1 has no complete request-schema/template contract, so absence of
// input is deliberately not a validation error or evidence of constructibility.
func EvaluateTaskChannelConfiguration(d *TaskChannelDiagnostic, plugin *jsplugin.LoadedPlugin, plan billing_setting.TaskBillingPlan, legacyPriceConfigured bool, credentialPresent bool) {
	d.Plugin = plugin.Meta.Key
	d.PluginVersion = plugin.Meta.Version
	d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "plugin_load", Status: "pass"}, TaskDiagnosticCheck{Check: "ownership", Status: "pass"})
	schema, _ := plugin.Meta.UsageForModels(d.MappedModel, d.Model)
	if plan.ExpressionFound {
		if !billing_setting.TaskExprCompatible(plan.Expression, schema) {
			d.Fail("usage_schema", "usage_schema_invalid", "PLUGIN_EXPR_INVALID")
		} else {
			d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "usage_schema", Status: "pass"})
		}
	} else {
		d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "usage_schema", Status: "not_tested", Reason: "expression_not_configured"})
	}
	if plan.Resolved || (!IsDFLOPTaskPlatform(constant.TaskPlatform(plugin.Meta.Key)) && legacyPriceConfigured && !plan.ExpressionFound) {
		source := plan.BillingSource
		if source == "" {
			source = "legacy"
		}
		d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "pricing", Status: "pass", BillingSource: source})
	} else {
		reason := plan.Reason
		if reason == "" {
			reason = "MODEL_PRICE_NOT_CONFIGURED"
		}
		d.Fail("pricing", "pricing_not_ready", reason)
		d.Checks[len(d.Checks)-1].Message = plan.Message
		d.Checks[len(d.Checks)-1].BillingSource = plan.BillingSource
	}
	if !credentialPresent {
		d.Fail("credentials", "credentials_missing_or_invalid", "credential_missing")
	} else {
		// Presence alone proves neither vendor structure nor authentication. Avoid
		// interpreting/rotating multi-keys or OAuth credentials in diagnostics.
		d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "credentials", Status: "not_tested", Reason: "credential_structure_not_verified"})
	}
	d.Checks = append(d.Checks,
		TaskDiagnosticCheck{Check: "request_constructibility", Status: "not_tested", Reason: "live_canary_requires_input"},
		TaskDiagnosticCheck{Check: "connectivity", Status: "not_tested", Reason: "connectivity_unavailable"},
		TaskDiagnosticCheck{Check: "live_generation", Status: "not_tested", Reason: "live_canary_required"})
	if d.Outcome != "fail" {
		d.Outcome = "partial"
		d.Status = "preflight_partial"
	}
}

// CompleteDetails exposes stable diagnostic codes while retaining legacy reason
// values for existing clients. Unknown checks are explicitly unverified.
func (d *TaskChannelDiagnostic) CompleteDetails() {
	d.OverallStatus = d.Outcome
	d.ResolvedUpstreamModel = d.MappedModel
	d.PricingState, d.UsageSchemaState, d.CredentialState = "not_tested", "not_tested", "not_tested"
	for i := range d.Checks {
		check := &d.Checks[i]
		check.CheckType = check.Check
		switch check.Check {
		case "ownership", "mapping":
			check.CheckType = "model_binding"
		case "usage_schema":
			check.CheckType = "billing_usage_schema"
		case "credentials":
			check.CheckType = "credential_structure"
		case "live_generation":
			check.CheckType = "runtime_evidence"
		}
		if check.Reason == "PROVIDER_SCOPE_MISMATCH" {
			check.CheckType = "model_binding"
		}
		switch check.CheckType {
		case "pricing":
			d.PricingState = check.Status
			d.ActivePricingState = check.Status
		case "billing_usage_schema":
			d.UsageSchemaState = check.Status
		case "credential_structure":
			d.CredentialState = check.Status
		}
		if check.ReasonCode == "" {
			switch check.Reason {
			case "PLUGIN_PRICE_NOT_FOUND", "MODEL_PRICE_NOT_CONFIGURED", "GENERIC_PRICE_NOT_FOUND":
				check.ReasonCode = "MISSING_PRICING"
			case "model_not_owned_by_plugin", "model_mapping_invalid", "PROVIDER_SCOPE_MISMATCH":
				check.ReasonCode = "MODEL_BINDING_MISMATCH"
			case "operation_not_supported":
				check.ReasonCode = "PROTOCOL_MISMATCH"
			case "credential_structure_not_verified", "expression_not_configured":
				check.ReasonCode = "INSUFFICIENT_EVIDENCE"
			default:
				check.ReasonCode = strings.ToUpper(check.Reason)
			}
		}
		if check.Message == "" {
			check.Message = check.Reason
		}
	}
}

// ChannelFailureEvidence separates observed provider failures from assumptions.
// A timeout without exact ledger evidence remains ambiguous. Classification has
// no transport or retry side effects; weak matches never authorize settlement.
type ChannelFailureEvidence struct {
	CatalogChecked    bool     `json:"catalog_checked"`
	CatalogFound      bool     `json:"catalog_found"`
	Callable          bool     `json:"callable"`
	ExpectedProtocols []string `json:"expected_protocols"`
	Protocol          string   `json:"protocol"`
	HTTPStatus        int      `json:"http_status"`
	ProviderCode      string   `json:"provider_code"`
	LedgerStatus      string   `json:"ledger_status"`
	Correlation       string   `json:"correlation"`
}

func ClassifyChannelFailure(e ChannelFailureEvidence) string {
	if e.CatalogChecked && !e.CatalogFound {
		return "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG"
	}
	if e.CatalogChecked && !e.Callable {
		return "SOURCE_MODEL_NOT_CALLABLE"
	}
	if e.Protocol != "" && len(e.ExpectedProtocols) > 0 && !slices.Contains(e.ExpectedProtocols, e.Protocol) {
		return "PROTOCOL_MISMATCH"
	}
	switch e.ProviderCode {
	case "no_channel_available":
		return "DFLOP_NO_HEALTHY_CHANNEL"
	case "deployment_unavailable", "deployment_pool_unavailable":
		return "DFLOP_DEPLOYMENT_POOL_UNAVAILABLE"
	case "upstream_unreachable":
		return "DFLOP_UPSTREAM_UNREACHABLE"
	case "upstream_timeout":
		return "DFLOP_UPSTREAM_TIMEOUT"
	}
	if (e.Correlation == "EXACT_REQUEST_ID" || e.Correlation == "EXACT_TASK_ID" || e.Correlation == "EXACT_TRACE_ID") && e.LedgerStatus == "timeout" {
		return "DFLOP_UPSTREAM_TIMEOUT"
	}
	return "INSUFFICIENT_EVIDENCE"
}

// AttachAuthenticatedCatalog consumes an explicitly fetched catalog, never
// submits a request and never updates pricing or model visibility.
func (d *TaskChannelDiagnostic) AttachAuthenticatedCatalog(items []dflop.Item, meta dflop.EffectiveMetadata, hash string) {
	candidate := d.MappedModel
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ModelID] = true
	}
	seen := map[string]bool{}
	for meta.Aliases[candidate] != "" && !ids[candidate] && !seen[candidate] {
		seen[candidate] = true
		candidate = meta.Aliases[candidate]
	}
	d.CatalogModel = candidate
	var item *dflop.Item
	for i := range items {
		if items[i].ModelID == candidate {
			item = &items[i]
			break
		}
	}
	if item == nil {
		d.Fail("provider_contract", "source_model_stale", ClassifyChannelFailure(ChannelFailureEvidence{CatalogChecked: true}))
		d.Status = "source_model_stale"
		d.Checks[len(d.Checks)-1].Evidence = hash
		d.CompleteDetails()
		return
	}
	if !item.Callable {
		d.Fail("provider_contract", "source_model_not_callable", ClassifyChannelFailure(ChannelFailureEvidence{CatalogChecked: true, CatalogFound: true}))
	} else if item.ReasonCode != "" {
		d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "provider_contract", Status: "not_tested", Reason: item.ReasonCode, ReasonCode: item.ReasonCode, Message: item.Reason})
		if d.Outcome != "fail" {
			d.Outcome = "partial"
			d.Status = "provider_contract_blocked"
		}
	} else {
		d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "provider_contract", Status: "pass", Evidence: hash})
	}
	d.Checks[len(d.Checks)-1].Evidence = hash
	if len(item.Prices) > 0 {
		d.CatalogPricingState = "known"
	}
	if item.TaskExpression != "" || item.Expression != "" {
		d.FormulaState = "known"
	}
	if d.CatalogPricingState == "known" {
		for i := range d.Checks {
			check := &d.Checks[i]
			if check.Check == "pricing" && check.Status == "fail" && slices.Contains([]string{"PLUGIN_PRICE_NOT_FOUND", "GENERIC_PRICE_NOT_FOUND", "MODEL_PRICE_NOT_CONFIGURED", "PRICING_NOT_APPLIED"}, check.Reason) {
				check.Reason = "PRICING_NOT_APPLIED"
				check.ReasonCode = "PRICING_NOT_APPLIED"
				check.Message = "Authenticated prices are known; the executing DFLOP plugin has no applied quantity expression"
			}
		}
	}
	if item.ModelID == "doubao-seedream-5-0-pro-260628" {
		d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "provider_documentation", Status: "not_tested", Reason: "PUBLIC_DOCUMENTATION_THRESHOLD_CONFLICT", Message: "Public media documentation and model/image guides disagree on the pixel threshold (2.61 MP versus 2.36 MP); only the authenticated catalog threshold is accepted", Evidence: "https://model.dflop.top/en/docs/reference/media-apis ; https://model.dflop.top/models/doubao-seedream-5-0-pro-260628"})
	}
	switch item.ReasonCode {
	case "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS", "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION", "PROVIDER_CONTRACT_IMAGE_THRESHOLD_CONFLICT":
		d.QuantityState = "blocked"
		for i := range d.Checks {
			check := &d.Checks[i]
			if check.Check == "pricing" && check.Status == "fail" {
				check.Reason = item.ReasonCode
				check.ReasonCode = item.ReasonCode
				check.Message = item.Reason
				// Preserve active-price status independently of known source rates.
			}
		}
		d.Status = "provider_contract_blocked"
	}
	binding, err := dflop.CatalogEndpoint(*item)
	if d.Endpoint != "" {
		if selected, selectedErr := dflop.CatalogEndpointForPath(*item, d.Endpoint); selectedErr == nil {
			binding = selected
			d.Protocol = selected.Protocol
		}
	}
	if err == nil && (d.Protocol == "") {
		d.Protocol = binding.Protocol
		d.Endpoint = binding.Path
	}
	if item.EndpointType == "images_generations" {
		d.RoutingStatus = "ROUTING_FIXED_NOT_RUNTIME_REVERIFIED"
	}
	switch item.TaskPlugin {
	case "dflop-image":
		d.UsageProfile = "IMAGE_PER_OUTPUT"
		if slices.Contains(item.BillingFeatures, "input_images") {
			d.UsageProfile = "IMAGE_PER_OUTPUT_PLUS_REFERENCE"
		}
		if slices.Contains(item.BillingFeatures, "image_size_bands") {
			d.UsageProfile = "IMAGE_PIXEL_TIER"
		}
	case "dflop-media":
		if item.EndpointType == "videos_generations" {
			d.UsageProfile = "VIDEO_OUTPUT_SECONDS_BY_RESOLUTION"
		}
		if slices.Contains(item.BillingFeatures, "video_input_seconds") {
			d.UsageProfile = "VIDEO_INPUT_PLUS_OUTPUT_SECONDS_BY_RESOLUTION"
		}
		if item.ReasonCode == "MISSING_AUTHORITATIVE_SOURCE_VIDEO_DURATION" {
			d.UsageProfile = "SUBTITLE_SOURCE_SECONDS_BY_OPERATION"
		}
	}
	protocols := item.Protocols
	if err == nil && item.EndpointType != "" && item.EndpointType != "chat_completions" {
		protocols = []string{binding.Protocol}
	}

	if d.Protocol != "" && len(protocols) > 0 && !slices.Contains(protocols, d.Protocol) {
		d.Fail("protocol_binding", "protocol_mismatch", ClassifyChannelFailure(ChannelFailureEvidence{Protocol: d.Protocol, ExpectedProtocols: protocols}))
		d.Checks[len(d.Checks)-1].Evidence = hash
	}
	d.CompleteDetails()
}
