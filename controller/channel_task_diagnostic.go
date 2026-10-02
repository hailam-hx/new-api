package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

// taskChannelDiagnostic pins the same immutable registry and metadata selector
// as production, without entering adaptor validation or the relay/billing path.
func taskChannelDiagnostic(channel *model.Channel, generation *jsplugin.RoutingGeneration, requested, operation string) *service.TaskChannelDiagnostic {
	start := time.Now()
	var catalogBody []byte
	var sourceBinding *dflop.EndpointBinding
	var contractErr error
	d := &service.TaskChannelDiagnostic{Kind: "task_plugin", Mode: "preflight", Status: "preflight_partial", Outcome: "partial", Model: strings.TrimSpace(requested), Operation: strings.TrimSpace(operation), Checks: []service.TaskDiagnosticCheck{}}
	defer func() {
		service.TaskConnectivityCapability(d, channel)
		if len(catalogBody) > 0 {
			cfg, err := model.GetDFLOPConfig()
			if err == nil {
				items, hash, meta, err := dflop.BuildEffective(catalogBody, []byte(`{"unit":"points","points_per_cny":60}`), cfg.CNYToUSD, cfg.MarkupMultiplier)
				if err == nil {
					d.AttachAuthenticatedCatalog(items, meta, hash)
				}
			}
		}
		if contractErr != nil && dflop.ApprovedCatalogOrigin(channel.GetBaseURL()) {
			d.Checks = append(d.Checks, service.TaskDiagnosticCheck{Check: "provider_contract", Status: "not_tested", Reason: "SOURCE_CATALOG_UNAVAILABLE", Message: contractErr.Error()})
		}
		d.CompleteDetails()
		d.DiagnosticDurationMS = time.Since(start).Milliseconds()
	}()
	if d.Model == "" {
		if channel.TestModel != nil {
			d.Model = strings.TrimSpace(*channel.TestModel)
		}
		if d.Model == "" && len(channel.GetModels()) > 0 {
			d.Model = channel.GetModels()[0]
		}
	}
	d.MappedModel = d.Model
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_mapping", channel.GetModelMapping())
	info := &relaycommon.RelayInfo{OriginModelName: d.Model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: d.Model, ChannelBaseUrl: channel.GetBaseURL()}}
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		d.Fail("mapping", "model_mapping_invalid", "model_mapping_invalid")
		return d
	}
	d.MappedModel = info.UpstreamModelName
	if dflop.ApprovedCatalogOrigin(channel.GetBaseURL()) {
		catalogBody, contractErr = dflop.AuthenticatedSourceCatalog(channel.Id, channel.Key)
		if contractErr == nil {
			item, err := dflop.AuthenticatedCatalogModel(catalogBody, d.MappedModel)
			if err == nil {
				binding, err := dflop.CatalogEndpoint(item)
				if err == nil {
					sourceBinding = &binding
				}
			}
		}
	}
	if d.Operation == "" {
		if endpoint := normalizeChannelTestEndpoint(channel, "", d.MappedModel); endpoint != "" {
			d.Operation = endpoint
		}
	}
	d.Checks = append(d.Checks, service.TaskDiagnosticCheck{Check: "mapping", Status: "pass"})
	if generation == nil {
		d.Fail("plugin_load", "plugin_load_failed", "plugin_registry_unavailable")
		return d
	}
	d.Generation = generation.Number
	if channel.Type == constant.ChannelTypeTaskPlugin {
		key := channel.GetSetting().TaskPluginKey
		d.Plugin = key
		if _, ok := generation.Get(key); !ok {
			d.Fail("plugin_load", "plugin_load_failed", "plugin_not_loaded")
			return d
		}
	}
	lookup := d.Model
	if canonical, ok := generation.CanonicalModel(lookup); ok {
		lookup = canonical
	} else if canonical, ok := generation.CanonicalModel(d.MappedModel); ok {
		lookup = canonical
	}
	var candidates []jsplugin.ProtocolBinding
	path := ""
	if d.Operation != "" {
		endpoint, ok := common.GetDefaultEndpointInfo(constant.EndpointType(d.Operation))
		if !ok && sourceBinding != nil && d.Operation == string(constant.EndpointTypeOpenAIVideo) && sourceBinding.Protocol == "openai_video" {
			endpoint, ok = common.EndpointInfo{Path: sourceBinding.Path, Method: http.MethodPost}, true
		}
		if !ok {
			d.Fail("ownership", "operation_not_supported", "operation_not_supported")
			return d
		}
		path = endpoint.Path
		if sourceBinding != nil {
			item, err := dflop.AuthenticatedCatalogModel(catalogBody, d.MappedModel)
			if err == nil && sourceBinding.Protocol == "openai_video" && d.Operation == string(constant.EndpointTypeOpenAIVideo) {
				path = sourceBinding.Path
			}
			if err == nil {
				_, err = dflop.CatalogEndpointForPath(item, path)
			}
			if err != nil {
				d.Fail("protocol_binding", "protocol_mismatch", "PROTOCOL_MISMATCH")
				return d
			}
		}
		d.Endpoint = path
		d.Protocol = d.Operation
		if d.Operation == string(constant.EndpointTypeImageGeneration) {
			d.Protocol = "openai_image"
		}
		candidates = generation.LookupEndpointCandidates(http.MethodPost, path, lookup)
	} else {
		for _, plugin := range generation.PluginsByModel(lookup) {
			candidates = append(candidates, jsplugin.ProtocolBinding{Plugin: plugin, Model: lookup})
		}
	}
	if len(candidates) == 0 {
		if channel.Type == constant.ChannelTypeTaskPlugin {
			if path != "" {
				d.Fail("ownership", "operation_not_supported", "operation_not_supported")
			} else {
				d.Fail("ownership", "model_not_owned_by_plugin", "model_not_owned_by_plugin")
			}
			return d
		}
		if d.Endpoint == "" {
			d.Endpoint = "/v1/chat/completions"
			d.Protocol = "openai_chat"
		}
		// Unclaimed models retain the ordinary GET contract. This endpoint itself
		// does not test them, even if the caller is a Task Plugin diagnostic client.
		d.Kind = "ordinary"
		d.Status = "ordinary_channel_test_required"
		d.Outcome = "untested"
		if d.Model != "" && !helper.HasRequestModelBillingConfig(d.Model) {
			d.Fail("pricing", "pricing_not_ready", "GENERIC_PRICE_NOT_FOUND")
		}
		return d
	}
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Model: lookup, Candidates: candidates})
	selected, bound := middleware.PinnedEndpointCandidateForChannel(c, channel, candidates[0].Plugin.Meta.Key)
	d.Plugin = candidates[0].Plugin.Meta.Key
	d.PluginVersion = candidates[0].Plugin.Meta.Version
	if !bound {
		// Preserve the existing provider-scope diagnostic, including the RCA's
		// blocked models, by reusing the unchanged configuration pricing resolver.
		if plan := resolveChannelTestTaskPricing(channel, generation, d.Model, d.MappedModel, path); plan != nil {
			d.Fail("pricing", "pricing_not_ready", plan.Reason)
			d.Checks[len(d.Checks)-1].Message = plan.Message
			d.Checks[len(d.Checks)-1].BillingSource = plan.BillingSource
			return d
		}
		if channel.Type == constant.ChannelTypeTaskPlugin {
			d.Fail("ownership", "model_not_owned_by_plugin", "model_not_owned_by_plugin")
			return d
		}
		d.Kind = "ordinary"
		d.Status = "ordinary_channel_test_required"
		d.Outcome = "untested"
		return d
	}
	if d.Operation == "" {
		d.Operation = "auto"
	}
	if selected.Protocol != "" {
		d.Operation = selected.Protocol + "." + selected.Operation.Name
		d.Protocol = selected.Protocol
		d.Endpoint = selected.Operation.Path
	}
	plan := billing_setting.ResolveTaskBillingPlan(selected.Plugin.Meta.Key, d.Model, d.MappedModel, selected.Plugin, service.IsDFLOPTaskPlatform(constant.TaskPlatform(selected.Plugin.Meta.Key)))
	if existing := resolveChannelTestTaskPricing(channel, generation, d.Model, d.MappedModel, path); existing != nil {
		plan = *existing
	}
	if selected.Protocol == "" {
		for _, endpoint := range []string{"/v1/images/generations", "/v1/videos", "/v1/audio/speech", "/v1/responses"} {
			for _, binding := range generation.LookupEndpointCandidates(http.MethodPost, endpoint, lookup) {
				if binding.Plugin.Meta.Key == selected.Plugin.Meta.Key {
					d.Endpoint = endpoint
					d.Protocol = binding.Protocol
					break
				}
			}
			if d.Protocol != "" {
				break
			}
		}
	}
	if d.Protocol == "" {
		for _, route := range selected.Plugin.Meta.Routes {
			if route.Method == http.MethodPost && (route.Type == jsplugin.RouteTypeSubmit || route.Type == jsplugin.RouteTypeDynamic) && (len(route.Models) == 0 || slices.Contains(route.Models, lookup)) {
				d.Endpoint = route.Path
				d.Protocol = "native"
				d.Operation = "native.submit"
				break
			}
		}
	}
	d.Checks = append(d.Checks, service.TaskDiagnosticCheck{Check: "protocol_binding", Status: "not_tested", Reason: "INSUFFICIENT_EVIDENCE"})
	if d.Protocol != "" {
		d.Checks[len(d.Checks)-1] = service.TaskDiagnosticCheck{Check: "protocol_binding", Status: "pass", Evidence: d.Endpoint}
	}
	service.EvaluateTaskChannelConfiguration(d, selected.Plugin, plan, helper.HasRequestModelBillingConfig(d.Model), strings.TrimSpace(channel.Key) != "")
	return d
}

// TestTaskChannel accepts opt-in diagnostics, never payload/URL/method overrides
// and never falls through to the synchronous tester. Only explicit connectivity
// enters the reviewed DFLOP catalog transport.
func TestTaskChannel(c *gin.Context) {
	var fields map[string]json.RawMessage
	body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 8193))
	if readErr != nil || len(body) > 8192 || common.Unmarshal(body, &fields) != nil {
		common.ApiError(c, errors.New("invalid preflight request"))
		return
	}
	for key := range fields {
		if key != "model" && key != "mode" && key != "operation" && key != "input" && key != "key_index" {
			common.ApiError(c, errors.New("unsupported preflight field"))
			return
		}
	}
	var request struct {
		Model     string          `json:"model"`
		Mode      string          `json:"mode"`
		Operation string          `json:"operation"`
		Input     json.RawMessage `json:"input"`
		KeyIndex  *int            `json:"key_index"`
	}
	raw, err := common.Marshal(fields)
	if err != nil || common.Unmarshal(raw, &request) != nil || (request.Mode != "preflight" && request.Mode != "connectivity") || (len(request.Input) > 0 && string(request.Input) != "null") || len(request.Model) > 256 || len(request.Operation) > 128 {
		common.ApiError(c, errors.New("only preflight or connectivity with null input is supported"))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("invalid channel id"))
		return
	}
	channel, err := model.CacheGetChannel(id)
	if err != nil {
		channel, err = model.GetChannelById(id, true)
	}
	if err != nil {
		common.ApiError(c, errors.New("channel not found"))
		return
	}
	diagnostic := taskChannelDiagnostic(channel, jsplugin.DefaultRegistry.Generation(), request.Model, request.Operation)
	if request.Mode == "connectivity" {
		service.ProbeTaskConnectivity(c.Request.Context(), diagnostic, channel, request.KeyIndex, dflop.Client{})
	}
	diagnostic.CompleteDetails()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": diagnostic})
}

// channelFailureDiagnostic describes an already completed attempt. It never
// executes or retries the relay, nor claims a provider timeout from HTTP alone.
func channelFailureDiagnostic(channel *model.Channel, result testResult, requested, endpoint string) *service.TaskChannelDiagnostic {
	if !dflop.ApprovedCatalogOrigin(channel.GetBaseURL()) {
		return nil
	}
	d := taskChannelDiagnostic(channel, jsplugin.DefaultRegistry.Generation(), requested, endpoint)
	if result.billingPlan != nil || result.newAPIError == nil {
		return d
	}
	if !result.outboundAttempted {
		code := string(result.newAPIError.GetErrorCode())
		check := "request_validation"
		reason := strings.ToUpper(code)
		switch result.newAPIError.GetErrorCode() {
		case types.ErrorCodeModelPriceError:
			check = "pricing"
			reason = "MISSING_PRICING"
		case types.ErrorCodeChannelModelMappedError:
			check = "model_binding"
			reason = "MODEL_BINDING_MISMATCH"
		case types.ErrorCode("PROTOCOL_MISMATCH"):
			check = "protocol_binding"
			reason = "PROTOCOL_MISMATCH"
		}
		d.Fail(check, "preflight_validation_failed", reason)
		d.Checks[len(d.Checks)-1].Message = result.newAPIError.Error()
		d.CompleteDetails()
		return d
	}
	d.Mode = "runtime"
	d.RuntimeAttempted = true
	d.Outcome = "partial"
	d.Status = "runtime_failure_unclassified"
	d.Checks = nil
	message := result.newAPIError.Error()
	status := result.upstreamStatus
	if status == 0 {
		status = result.newAPIError.StatusCode
	}
	if result.upstreamModel != "" {
		d.MappedModel = result.upstreamModel
	}
	code := string(result.newAPIError.GetErrorCode())
	if status == 503 && strings.Contains(message, "no healthy channel for model") {
		code = "no_channel_available"
	}
	if status == 503 && strings.Contains(message, "NO_HEALTHY_DEPLOYMENT") {
		code = "deployment_pool_unavailable"
	}
	reason := service.ClassifyChannelFailure(service.ChannelFailureEvidence{HTTPStatus: status, ProviderCode: code})
	if code == "PROTOCOL_MISMATCH" {
		reason = code
		d.RuntimeAttempted = false
	}
	check := service.TaskDiagnosticCheck{Check: "runtime_evidence", Status: "not_tested", ReasonCode: reason, Reason: reason, Message: message}
	if reason != "INSUFFICIENT_EVIDENCE" {
		check.Status = "fail"
		d.Outcome = "fail"
		d.Status = reason
	}
	if result.context != nil && result.context.Request != nil {
		d.Endpoint = result.context.Request.URL.Path
		check.Evidence = d.Endpoint
	}
	d.Checks = append(d.Checks, check)
	d.CompleteDetails()
	return d
}
