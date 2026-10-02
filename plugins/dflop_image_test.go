package plugins_test

import (
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDFLOPImageAuthoritativeSettlement(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, model string
		body        map[string]any
		want        map[string]any
		invalid     bool
	}{
		{"fixed grid ignores requested n", "tvod-midjourney-v7", map[string]any{"data": []any{map[string]any{"url": "https://example/grid"}}}, map[string]any{"image_count": int64(4)}, false},
		{"provider count has priority", "tvod-midjourney-v8.1", map[string]any{"unit_count": 2, "data": []any{map[string]any{"url": "https://example/grid"}}}, map[string]any{"image_count": int64(2)}, false},
		{"missing payload cannot prove fixed grid succeeded", "tvod-midjourney-v7", map[string]any{}, nil, true},
		{"fractional count rejected", "tvod-midjourney-v7", map[string]any{"unit_count": 1.5, "data": []any{map[string]any{"url": "https://example/grid"}}}, nil, true},
		{"reported count cannot undercount delivered payloads", "qwen-image-3.0", map[string]any{"unit_count": 1, "data": []any{map[string]any{"url": "https://example/one", "size": "1024x1024"}, map[string]any{"url": "https://example/two", "size": "1024x1024"}}}, nil, true},
		{"below threshold", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img", "size": "1024x1024"}}}, map[string]any{"image_count": int64(1), "small_image_count": int64(1), "large_image_count": int64(0), "input_image_count": int64(0)}, false},
		{"at threshold", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img", "size": "2048x1024"}}}, map[string]any{"image_count": int64(1), "small_image_count": int64(1), "large_image_count": int64(0), "input_image_count": int64(0)}, false},
		{"above threshold", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img", "size": "2049x1024"}}}, map[string]any{"image_count": int64(1), "small_image_count": int64(0), "large_image_count": int64(1), "input_image_count": int64(0)}, false},
		{"provider default output dimensions", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img", "size": "2048x2048"}}}, map[string]any{"image_count": int64(1), "small_image_count": int64(0), "large_image_count": int64(1), "input_image_count": int64(0)}, false},
		{"object output", "qwen-image-3.0-pro", map[string]any{"data": map[string]any{"url": "https://example/img", "size": "1024x1024"}}, map[string]any{"image_count": int64(1), "small_image_count": int64(1), "large_image_count": int64(0), "input_image_count": int64(0)}, false},
		{"split URL and base64 count once", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img", "size": "1024x1024"}, map[string]any{"b64_json": "encoded", "size": "1024x1024"}, map[string]any{"revised_prompt": "cat"}}}, map[string]any{"image_count": int64(1), "small_image_count": int64(1), "large_image_count": int64(0), "input_image_count": int64(0)}, false},
		{"mixed output tiers", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img", "width": 1024, "height": 1024}, map[string]any{"url": "https://example/large", "size": "2048x2048"}}}, map[string]any{"image_count": int64(2), "small_image_count": int64(1), "large_image_count": int64(1), "input_image_count": int64(0)}, false},
		{"missing size stays pending", "qwen-image-3.0-pro", map[string]any{"data": []any{map[string]any{"url": "https://example/img"}}}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := map[string]any{"model": tc.model, "upstreamModel": tc.model, "state": dflopImageSubmissionState(t, plugin, tc.model, 0)}
			value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, tc.body)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, value)
			_, err = plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, tc.body, map[string]any{"status": 200})
			require.NoError(t, err)
		})
	}
	t.Run("missing Qwen facts retain successful delivery pending billing", func(t *testing.T) {
		value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"model": "qwen-image-3.0-pro"}, map[string]any{"data": []any{map[string]any{"url": "https://example/img"}}}, map[string]any{"status": 200})
		require.NoError(t, err)
		assert.Equal(t, "SUCCESS", value.(map[string]any)["status"])
		assert.Equal(t, true, value.(map[string]any)["state"].(map[string]any)["billingPending"])
	})
}

func TestDFLOPImageBindingAndReservation(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, model := range []string{"tvod-midjourney-v7", "tvod-midjourney-v8.1", "qwen-image-3.0-pro"} {
		binding, found := registry.Generation().LookupEndpoint("POST", "/v1/images/generations", model)
		require.True(t, found)
		assert.Same(t, plugin, binding.Plugin)
	}
	for _, tc := range []struct {
		name, model         string
		req                 map[string]any
		small, large, count int64
	}{
		{"missing size reserves large", "qwen-image-3.0-pro", map[string]any{"prompt": "cat"}, 0, 1, 1},
		{"exact threshold request", "qwen-image-3.0-pro", map[string]any{"prompt": "cat", "size": "2048x1024", "n": 2}, 0, 2, 2},
		{"MJ request n is irrelevant", "tvod-midjourney-v7", map[string]any{"prompt": "cat", "n": 1}, 0, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": tc.model, "operation": "generate", "body": map[string]any{"kind": "json", "value": tc.req}})
			require.NoError(t, err)
			request := value.(map[string]any)["requestBody"].(map[string]any)
			ctx := map[string]any{"model": tc.model, "upstreamModel": tc.model, "requestBody": request, "baseUrl": "https://api.dflop.top", "apiKey": "fake", "requestHeaders": map[string]any{"Idempotency-Key": "fixed-test-key"}, "usagePurpose": "facts"}
			descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			assert.Equal(t, "https://api.dflop.top/v1/images/generations", descriptor.(map[string]any)["url"])
			assert.Equal(t, tc.model, descriptor.(map[string]any)["body"].(map[string]any)["model"])
			facts, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.count, facts.(map[string]any)["image_count"])
			if tc.model == "qwen-image-3.0-pro" {
				assert.Equal(t, tc.small, facts.(map[string]any)["small_image_count"])
				assert.Equal(t, tc.large, facts.(map[string]any)["large_image_count"])
			}
		})
	}
}

func TestDFLOPImageTerminalFailureAndGatewayBinding(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, body := range []map[string]any{{"status": "failed"}, {"status": "cancelled"}, {"status": "expired"}, {"data": []any{}}} {
		value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"model": "tvod-midjourney-v7"}, body, map[string]any{"status": 200})
		require.NoError(t, err)
		assert.Equal(t, "FAILURE", value.(map[string]any)["status"])
	}
	for _, tc := range []struct{ base, want string }{
		{"https://api.dflop.top", "https://api.dflop.top/v1/images/generations"},
		{"https://gateway.example", "https://gateway.example/dflop-image/v1/images/generations"},
	} {
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "tvod-midjourney-v7", "upstreamModel": "tvod-midjourney-v7", "baseUrl": tc.base, "apiKey": "fake", "requestHeaders": map[string]any{"Idempotency-Key": "fixed"}, "requestBody": map[string]any{"prompt": "cat", "n": 1, "async": true}, "upstream": map[string]any{"kind": "new_api"}})
		require.NoError(t, err)
		assert.Equal(t, tc.want, value.(map[string]any)["url"])
	}
}

func TestDFLOPImageCompletionBillingUsesDeliveredTiers(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": "qwen-image-3.0-pro", "state": dflopImageSubmissionState(t, plugin, "qwen-image-3.0-pro", 2)}, map[string]any{"status": "SUCCESS"}, map[string]any{"data": []any{map[string]any{"url": "https://example/small", "size": "1024x1024"}, map[string]any{"url": "https://example/large", "size": "2048x2048"}}})
	require.NoError(t, err)
	// Transient synthetic rates exercise the production evaluator, not live prices.
	expression := `tier("image", u("small_image_count") * 0.1 + u("large_image_count") * 0.2 + u("input_image_count") * 0.03)`
	result, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true}, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: value.(map[string]any)})
	require.NoError(t, err)
	assert.Equal(t, 180000, result.ActualQuotaAfterGroup)
}

func TestDFLOPQwenDefaultAndProviderUsage(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name                      string
		request, body             map[string]any
		count, small, large, refs int64
		pending                   bool
	}{
		{"omitted size cannot establish delivered dimensions", map[string]any{"n": 6, "image": []any{"ref-a", "ref-b"}}, map[string]any{"data": []any{map[string]any{"url": "https://example/image"}}}, 0, 0, 0, 0, true},
		{"terminal dimensions override default", map[string]any{"n": 6}, map[string]any{"usage": map[string]any{"output_width": 1024, "output_height": 1024, "output_image_count": 2, "input_image_count": 0}, "data": []any{map[string]any{"url": "https://example/image"}}}, 2, 2, 0, 0, false},
		{"authenticated threshold supersedes public 1536 band", map[string]any{"size": "1536x1536", "n": 1}, map[string]any{"usage": map[string]any{"output_width": 1536, "output_height": 1536, "output_image_count": 1}, "data": []any{map[string]any{"url": "https://example/image"}}}, 1, 0, 1, 0, false},
		{"explicit request dimensions cannot become final usage", map[string]any{"size": "2048x2048", "n": 2}, map[string]any{"data": []any{map[string]any{"url": "https://example/image"}}}, 0, 0, 0, 0, true},
		{"incomplete terminal dimensions cannot use default", map[string]any{"n": 1}, map[string]any{"usage": map[string]any{"output_width": 1024}, "data": []any{map[string]any{"url": "https://example/image"}}}, 0, 0, 0, 0, true},
		{"default cannot fabricate actual output count", map[string]any{"n": 2}, map[string]any{"data": []any{map[string]any{"revised_prompt": "cat"}}}, 0, 0, 0, 0, true},
		{"contradictory reported counts stay pending", map[string]any{"n": 2}, map[string]any{"unit_count": 2, "usage": map[string]any{"output_image_count": 1}, "data": []any{map[string]any{"url": "https://example/image"}}}, 0, 0, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := map[string]any{"model": "qwen-image-3.0-pro", "requestBody": tc.request}
			acknowledgement, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"statusCode": 202, "body": map[string]any{"id": "task", "model": "qwen-image-3.0-pro"}})
			require.NoError(t, err)
			ctx["state"] = acknowledgement.(map[string]any)["state"]
			terminal, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, tc.body, map[string]any{"status": 200})
			require.NoError(t, err)
			result := terminal.(map[string]any)
			assert.Equal(t, "SUCCESS", result["status"])
			assert.Equal(t, tc.pending, result["state"].(map[string]any)["billingPending"])
			ctx["state"] = result["state"]
			facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, result, tc.body)
			require.NoError(t, err)
			if tc.pending {
				assert.Empty(t, facts)
				return
			}
			assert.Equal(t, map[string]any{"image_count": tc.count, "small_image_count": tc.small, "large_image_count": tc.large, "input_image_count": tc.refs}, facts)
		})
	}
}

func TestDFLOPImageContractProfiles(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, tc := range []struct {
		model, profile string
		outputs, refs  int
		keys           []string
	}{
		{"doubao-seedream-4-0-250828", "IMAGE_PER_OUTPUT", 15, 14, []string{"image_count"}},
		{"doubao-seedream-4-5-251128", "IMAGE_PER_OUTPUT", 15, 14, []string{"image_count"}},
		{"doubao-seedream-5-0-260128", "IMAGE_PER_OUTPUT", 15, 14, []string{"image_count"}},
		{"doubao-seedream-5-0-pro-260628", "IMAGE_PIXEL_TIER", 1, 10, []string{"image_count", "small_image_count", "large_image_count", "input_image_count"}},
		{"qwen-image-3.0", "IMAGE_PER_OUTPUT_PLUS_REFERENCE", 9, 50, []string{"image_count", "input_image_count"}},
		{"qwen-image-3.0-pro", "IMAGE_PIXEL_TIER", 9, 50, []string{"image_count", "small_image_count", "large_image_count", "input_image_count"}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			binding, found := registry.Generation().LookupEndpoint("POST", "/v1/images/generations", tc.model)
			require.True(t, found)
			assert.Same(t, plugin, binding.Plugin)
			schema, _ := plugin.Meta.UsageForModel(tc.model)
			assert.Len(t, schema, len(tc.keys))
			for _, key := range tc.keys {
				assert.Contains(t, schema, key)
			}
			request := map[string]any{"prompt": "cat", "n": tc.outputs}
			decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": tc.model, "operation": "generate", "body": map[string]any{"kind": "json", "value": request}})
			require.NoError(t, err)
			ctx := map[string]any{"model": tc.model, "requestBody": decoded.(map[string]any)["requestBody"], "baseUrl": "https://api.dflop.top", "apiKey": "fake", "requestHeaders": map[string]any{"Idempotency-Key": "fixed"}}
			descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			assert.Equal(t, "https://api.dflop.top/v1/images/generations", descriptor.(map[string]any)["url"])
			ack, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"statusCode": 202, "body": map[string]any{"id": "task", "model": tc.model}})
			require.NoError(t, err)
			state := ack.(map[string]any)["state"].(map[string]any)
			assert.Equal(t, tc.profile, state["contract"].(map[string]any)["profile"])
			request["n"] = tc.outputs + 1
			_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": request}})
			require.Error(t, err)
			references := make([]any, tc.refs+1)
			for i := range references {
				references[i] = "https://example/ref"
			}
			request["n"] = 1
			request["image"] = references
			_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": request}})
			require.Error(t, err)
		})
	}
}

func TestDFLOPImageContractSettlement(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, model string
		refs        int
		data        any
		want        map[string]any
		blocker     string
	}{
		{"Seedream object", "doubao-seedream-4-0-250828", 2, map[string]any{"url": "https://example/img"}, map[string]any{"image_count": int64(1)}, ""},
		{"Seedream split counts once", "doubao-seedream-4-5-251128", 0, []any{map[string]any{"url": "https://example/img"}, map[string]any{"b64_json": "encoded"}, map[string]any{"revised_prompt": "cat"}}, map[string]any{"image_count": int64(1)}, ""},
		{"Seedream payloadless held", "doubao-seedream-5-0-260128", 0, []any{map[string]any{"revised_prompt": "cat"}}, nil, "MISSING_AUTHORITATIVE_IMAGE_USAGE"},
		{"Qwen references billed", "qwen-image-3.0", 2, map[string]any{"url": "https://example/img"}, map[string]any{"image_count": int64(1), "input_image_count": int64(2)}, ""},
		{"Seedream Pro exact authenticated threshold", "doubao-seedream-5-0-pro-260628", 2, map[string]any{"url": "https://example/img", "size": "2000x1305"}, map[string]any{"image_count": int64(1), "small_image_count": int64(1), "large_image_count": int64(0), "input_image_count": int64(2)}, ""},
		{"Seedream Pro above authenticated threshold", "doubao-seedream-5-0-pro-260628", 1, map[string]any{"url": "https://example/img", "size": "2001x1305"}, map[string]any{"image_count": int64(1), "small_image_count": int64(0), "large_image_count": int64(1), "input_image_count": int64(1)}, ""},
		{"Seedream Pro missing dimensions held", "doubao-seedream-5-0-pro-260628", 0, map[string]any{"url": "https://example/img"}, nil, "MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs := make([]any, tc.refs)
			for i := range refs {
				refs[i] = "https://example/ref"
			}
			ctx := map[string]any{"model": tc.model, "requestBody": map[string]any{"n": 1, "image": refs, "size": "1024x1024"}}
			ack, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"statusCode": 202, "body": map[string]any{"id": "task", "model": tc.model}})
			require.NoError(t, err)
			ctx["state"] = ack.(map[string]any)["state"]
			body := map[string]any{"data": tc.data}
			terminal, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, body, map[string]any{"status": 200})
			require.NoError(t, err)
			result := terminal.(map[string]any)
			assert.Equal(t, "SUCCESS", result["status"])
			ctx["state"] = result["state"]
			facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, result, body)
			require.NoError(t, err)
			if tc.blocker != "" {
				assert.Empty(t, facts)
				assert.Equal(t, tc.blocker, result["state"].(map[string]any)["blocker"])
				return
			}
			assert.Equal(t, tc.want, facts)
		})
	}
	// The common Seedream constraint is refs + outputs, independently of each cap.
	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": "doubao-seedream-4-0-250828", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "cat", "n": 15, "image": []any{"https://example/ref"}}}})
	require.Error(t, err)
}

func dflopImageSubmissionState(t *testing.T, plugin *jsplugin.LoadedPlugin, model string, references int) map[string]any {
	t.Helper()
	images := make([]any, references)
	for i := range images {
		images[i] = "https://example/ref"
	}
	value, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"model": model, "requestBody": map[string]any{"n": 1, "image": images}}, map[string]any{"statusCode": 202, "body": map[string]any{"id": "task", "model": model}})
	require.NoError(t, err)
	return value.(map[string]any)["state"].(map[string]any)
}

func TestDFLOPImageFrozenContractAndMissingFacts(t *testing.T) {
	source, err := builtinplugins.Source("dflop-image")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "dflop-image"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, model string
		state       map[string]any
		body        map[string]any
		blocker     string
	}{
		{"legacy unfrozen tier quarantined", "qwen-image-3.0-pro", map[string]any{"input_image_count": 0, "default_output_size": "2048x2048"}, map[string]any{"data": map[string]any{"url": "https://example/img", "size": "1024x1024"}}, "MISSING_AUTHORITATIVE_IMAGE_USAGE"},
		{"successful task without count quarantined", "doubao-seedream-4-0-250828", dflopImageSubmissionState(t, plugin, "doubao-seedream-4-0-250828", 0), map[string]any{"status": "completed"}, "MISSING_AUTHORITATIVE_IMAGE_USAGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"model": tc.model, "state": tc.state}, tc.body, map[string]any{"status": 200})
			require.NoError(t, err)
			terminal := result.(map[string]any)
			assert.Equal(t, "SUCCESS", terminal["status"])
			assert.Equal(t, true, terminal["state"].(map[string]any)["billingPending"])
			assert.Equal(t, tc.blocker, terminal["state"].(map[string]any)["blocker"])
		})
	}
	t.Run("request mutation cannot change frozen references", func(t *testing.T) {
		state := dflopImageSubmissionState(t, plugin, "qwen-image-3.0", 4)
		facts, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": "qwen-image-3.0", "state": state, "requestBody": map[string]any{"image": []any{}, "n": 9}}, map[string]any{"status": "SUCCESS"}, map[string]any{"data": map[string]any{"url": "https://example/img"}})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"image_count": int64(1), "input_image_count": int64(4)}, facts)
	})
	t.Run("missing authoritative threshold blocks contract", func(t *testing.T) {
		state := dflopImageSubmissionState(t, plugin, "doubao-seedream-5-0-pro-260628", 0)
		delete(state["contract"].(map[string]any), "large_pixel_threshold")
		value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"model": "doubao-seedream-5-0-pro-260628", "state": state}, map[string]any{"data": map[string]any{"url": "https://example/img", "size": "1024x1024"}}, map[string]any{"status": 200})
		require.NoError(t, err)
		assert.Equal(t, "PROVIDER_CONTRACT_IMAGE_THRESHOLD_CONFLICT", value.(map[string]any)["state"].(map[string]any)["blocker"])
	})
	t.Run("object result renders host image array", func(t *testing.T) {
		value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "render"}, map[string]any{}, map[string]any{"data": map[string]any{"data": map[string]any{"url": "https://example/img"}}})
		require.NoError(t, err)
		assert.Equal(t, []any{map[string]any{"url": "https://example/img"}}, value.(map[string]any)["data"])
	})
}
