package service

import (
	"fmt"
	"math"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/shopspring/decimal"
)

// DFLOPFreshCanaryTarget creates a new offline intent. No historical body,
// provider ID, timestamp or idempotency identity participates in this plan.
func DFLOPFreshCanaryTarget(provider dflop.Item, catalogHash, configHash string) DFLOPVerificationPlannedTarget {
	fixture := VerificationFixture{ID: "fresh-canary-v6:" + provider.ModelID, Version: "fresh-canary-v6", Model: provider.ModelID, Protocol: "openai_chat", Mode: "default", Operation: "create", Endpoint: "/v1/chat/completions", SourceCatalogHash: catalogHash, Request: map[string]any{"model": provider.ModelID, "messages": []map[string]string{{"role": "user", "content": "Reply with exactly: OK"}}, "stream": false, "max_tokens": 64, "tool_choice": "none"}, Bounds: map[string]VerificationBound{}, Plan: VerificationContractPlan{ID: "OPENAI_CHAT_TOKEN", Version: "6", RequiredFields: []string{"model", "messages", "max_tokens"}, RequiredOutput: "text", RequiredUsageFacts: []string{"prompt_tokens", "completion_tokens"}, Documentation: []string{"https://model.dflop.top/en/docs/reference/api-reference"}, ReservationQuantitySource: "authenticated context window upper bound plus explicit generated-token cap; maximum tariff across context tiers", TerminalQuantitySources: map[string]string{"prompt_tokens": "usage.prompt_tokens", "completion_tokens": "usage.completion_tokens"}}}
	target := DFLOPVerificationPlannedTarget{Model: provider.ModelID, Wave: 3, Fixture: fixture, ConfigHash: configHash, PricingSnapshotKind: "AUTHENTICATED_CATALOG_INTENT", BillingExprHash: billingexpr.ExprHashString(provider.Expression), PluginHash: verificationHash([]byte("NO_TASK_PLUGIN:HOST_OPENAI_RELAY")), FixturePublicURLs: []string{}, FixturePublicURLHashes: []string{}}
	pricing, _ := common.Marshal(provider)
	// These are bounded host-relay intents. The task-only executor must not
	// authorize them until host parser, journal and settlement support exists.
	target.ExecutorBlocker = "HOST_RELAY_CANARY_EXECUTOR_REQUIRED"
	target.PricingSnapshotHash = verificationHash(pricing)
	if !provider.Callable {
		target.Blocker = "SOURCE_MODEL_NOT_CALLABLE"
		return target
	}
	var raw struct {
		ContextWindow int64 `json:"context_window"`
		Caps          struct {
			Image *struct {
				Size struct {
					Default   string `json:"default_size"`
					MinPixels int    `json:"min_pixels"`
					MaxPixels int    `json:"max_pixels"`
					MaxSide   int    `json:"max_side"`
					Align     int    `json:"align"`
					Custom    bool   `json:"custom"`
				} `json:"size"`
			} `json:"image"`
		} `json:"caps"`
	}
	if common.Unmarshal(provider.Raw, &raw) != nil || provider.Expression == "" {
		target.Blocker = "PROVIDER_CONTRACT_UNRESOLVED"
		return target
	}
	maximum := decimal.Zero
	if provider.Category == "image" {
		if provider.EndpointType != "images_generations" || !slices.Equal(provider.BillingFeatures, []string{"per_image"}) || raw.Caps.Image == nil {
			target.Blocker = "PROVIDER_IMAGE_CONTRACT_UNRESOLVED"
			return target
		}
		size := raw.Caps.Image.Size.Default
		if size == "" {
			caps := raw.Caps.Image.Size
			if !caps.Custom || caps.MinPixels <= 0 || caps.MaxPixels < caps.MinPixels || caps.MaxSide <= 0 {
				target.Blocker = "PROVIDER_IMAGE_SIZE_REQUIRED"
				return target
			}
			side := int(math.Ceil(math.Sqrt(float64(caps.MinPixels))))
			if caps.Align > 0 {
				side = ((side + caps.Align - 1) / caps.Align) * caps.Align
			}
			if side > caps.MaxSide || int64(side)*int64(side) > int64(caps.MaxPixels) {
				target.Blocker = "PROVIDER_IMAGE_SIZE_REQUIRED"
				return target
			}
			size = fmt.Sprintf("%dx%d", side, side)
		}
		fixture.Protocol, fixture.Mode, fixture.Operation, fixture.Endpoint = "openai_image", "text_to_image", "generate", "/v1/images/generations"
		fixture.Request = map[string]any{"model": provider.ModelID, "prompt": "A plain blue square", "n": 1, "size": size, "response_format": "url"}
		fixture.Bounds["n"] = VerificationBound{Min: 1, Max: 1}
		fixture.Plan = DFLOPVerificationContractPlans()[0]
		var err error
		maximum, err = verificationRate(provider, "price_per_image")
		if err != nil {
			target.Blocker = verificationReason(err)
			return target
		}
	} else {
		if provider.Category != "text" || !slices.Contains(provider.Protocols, "openai_chat") || raw.ContextWindow <= 0 || raw.ContextWindow > 10000000 {
			target.Blocker = "PROVIDER_TOKEN_CEILING_REQUIRED"
			return target
		}
		for _, feature := range provider.BillingFeatures {
			if !slices.Contains([]string{"token", "token_long_context", "server_tool_call"}, feature) {
				target.Blocker = "UNKNOWN_BILLING_FEATURE"
				return target
			}
		}
		input, err := verificationRate(provider, "input_per_1m")
		if err != nil {
			target.Blocker = verificationReason(err)
			return target
		}
		output, err := verificationRate(provider, "output_per_1m")
		if err != nil {
			target.Blocker = verificationReason(err)
			return target
		}
		for _, component := range []string{"input_per_1m_long", "output_per_1m_long"} {
			if _, exists := provider.Prices[component]; !exists {
				continue
			}
			rate, err := verificationRate(provider, component)
			if err != nil {
				target.Blocker = verificationReason(err)
				return target
			}
			if component == "input_per_1m_long" {
				input = decimal.Max(input, rate)
			} else {
				output = decimal.Max(output, rate)
			}
		}
		fixture.Bounds["input_tokens"] = VerificationBound{Min: 1, Max: float64(raw.ContextWindow)}
		fixture.Bounds["output_tokens"] = VerificationBound{Min: 1, Max: 64}
		maximum = input.Mul(decimal.NewFromInt(raw.ContextWindow)).Add(output.Mul(decimal.NewFromInt(64))).Div(decimal.NewFromInt(1000000))
		body, _ := common.Marshal(fixture.Request)
		if err := dflop.DFLOPEndpointRequestContract("https://api.dflop.top", provider.Expression, fixture.Endpoint, billingexpr.RequestInput{Body: body}); err != nil {
			target.Blocker = verificationReason(err)
			return target
		}
	}
	fixture.Plan.ExactEndpointModes = []string{fixture.Mode}
	fixture.Plan.DocumentationSnapshotHash = verificationHash(DFLOPVerificationDocumentationSnapshot())
	target.Fixture = fixture
	body, _ := common.Marshal(fixture.Request)
	target.RequestBody, target.RequestBodyHash, target.StaticIntentHash = body, verificationHash(body), verificationHash(body)
	fixtureBody, _ := common.Marshal(fixture)
	target.FixtureHash = verificationHash(fixtureBody)
	value := maximum.String()
	target.MaximumProviderPoints = &value
	return target
}
