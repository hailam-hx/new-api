package dflop

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type canaryCatalogTransport struct{ requests []string }

func (t *canaryCatalogTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requests = append(t.requests, request.Method+" "+request.URL.Path)
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[]}`)), Request: request}, nil
}

func TestCanaryPlanAndAuthorization(t *testing.T) {
	transport := &canaryCatalogTransport{}
	client := Client{HTTP: &http.Client{Transport: transport}}
	response, err := client.FetchEffective(context.Background(), "test-key", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /v1/catalog"}, transport.requests)
	assert.Equal(t, "FRESH", response.State)

	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"voice-clone-pro","pricing":{"category":"audio","endpoint_type":"voice_clone","callable":true,"price_per_voice_clone":"1800"},"billing":{"features":["voice_clone"]},"caps":{"surfaces":["audio"]}}]}`)
	plan, err := BuildCanaryPlan(1, catalog, []byte(`{"unit":"points","points_per_cny":60}`), "etag", "0.15")
	require.NoError(t, err)
	require.Len(t, plan.Cases, 1)
	assert.Equal(t, "1800", plan.Cases[0].EstimatedMaxPoints)
	assert.Equal(t, "4.5", plan.Cases[0].EstimatedMaxUSD)
	assert.Equal(t, "BLOCKED_MISSING_FIXTURE", plan.Cases[0].Status)

	ready := plan.Cases[0]
	ready.Status = "READY_FOR_AUTHORIZATION"
	full := CanaryAuthorization{Execute: true, ConfirmPaid: true, CaseID: ready.ID, MaxCostUSD: "5"}
	for _, auth := range []CanaryAuthorization{
		{ConfirmPaid: true, CaseID: ready.ID, MaxCostUSD: "5"},
		{Execute: true, CaseID: ready.ID, MaxCostUSD: "5"},
		{Execute: true, ConfirmPaid: true, CaseID: ready.ID},
	} {
		assert.ErrorContains(t, CheckCanaryAuthorization(auth, ready, ready), "CANARY_AUTHORIZATION_REQUIRED")
	}
	under := full
	under.MaxCostUSD = "4"
	assert.ErrorContains(t, CheckCanaryAuthorization(under, ready, ready), "BLOCKED_BY_BUDGET")
	changed := ready
	changed.EstimatedMaxPoints, changed.EstimatedMaxUSD = "2400", "6"
	assert.ErrorContains(t, CheckCanaryAuthorization(full, ready, changed), "CANARY_PRICE_CHANGED")
	assert.NoError(t, CheckCanaryAuthorization(full, ready, ready))
	assert.ErrorContains(t, CheckCanaryAuthorization(full, plan.Cases[0], plan.Cases[0]), "CANARY_DESIGN_UNVERIFIED")
}

func TestCanaryCaptureRedactsAndResumeDoesNotResubmit(t *testing.T) {
	secret := "secret-key-value"
	raw := []byte(`{"Authorization":"Bearer secret-key-value","x-api-key":"secret-key-value","nested":{"cookie":"private","url":"https://media.example/private/a?signature=abc","path":"/Users/jake/private.wav"},"usage":{"characters":6},"x-gateway-trace":"trace-123"}`)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path, err := WriteCanaryCapture(dir, "invocation-1", CanaryCapture{CaseID: "tts-async", Response: raw, GatewayTrace: "trace-123"}, secret)
	require.NoError(t, err)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, leaked := range []string{secret, "signature=", "/Users/jake", "private.wav", `"cookie":"private"`} {
		assert.NotContains(t, string(stored), leaked)
	}
	assert.Contains(t, string(stored), `"characters":6`)
	assert.Contains(t, string(stored), "trace-123")
	_, err = WriteCanaryCapture(dir, "invocation-1", CanaryCapture{Response: raw}, secret)
	assert.ErrorIs(t, err, os.ErrExist)
	assert.Equal(t, "POLL_EXISTING_TASK", (CanaryInvocation{TaskID: "task-1"}).NextAction())
	assert.Equal(t, "SUBMIT_ONLY_AFTER_AUTHORIZATION", (CanaryInvocation{}).NextAction())
	invocation := CanaryInvocation{ID: "i1", CaseID: "tts-async", TaskID: "task-1", ChannelID: 1}
	invocationPath, err := WriteCanaryInvocation(dir, invocation)
	require.NoError(t, err)
	storedInvocation, err := ReadCanaryInvocation(invocationPath)
	require.NoError(t, err)
	assert.Equal(t, "POLL_EXISTING_TASK", storedInvocation.NextAction())
	_, err = WriteCanaryInvocation(dir, invocation)
	assert.ErrorIs(t, err, os.ErrExist)
}

func TestCanaryOfflineFactsAreOnlyFixtureEvidence(t *testing.T) {
	tests := []struct {
		id, body, want string
	}{
		{"tts-async", `{"task_id":"t1","status":"succeeded","characters":6,"duration_sec":1.2}`, "RUNTIME_FACT_VERIFIED"},
		{"tts-async", `{"task_id":"t1","status":"succeeded"}`, "RUNTIME_FACT_MISSING"},
		{"tts-async", `{"task_id":"t1","status":"failed"}`, "EXECUTED_FAILURE"},
		{"grok-tool-chat", `{"usage":{"prompt_tokens":10,"completion_tokens":2,"num_server_side_tools_used":1}}`, "RUNTIME_FACT_VERIFIED"},
		{"grok-tool-chat", `{"usage":{"prompt_tokens":10,"completion_tokens":2}}`, "RUNTIME_FACT_MISSING"},
		{"suno-generation", `{"task_id":"t1","status":"succeeded","tracks":[{"id":"a"},{"id":"b"}]}`, "SEMANTICS_CONFLICT"},
		{"voice-clone", `{"voice_id":"v1","status":"ready"}`, "RUNTIME_FACT_VERIFIED"},
		{"voice-clone", `{"status":"failed"}`, "EXECUTED_FAILURE"},
		{"output-count-midjourney", `{"status":"succeeded","data":[{"url":"https://example/a"},{"revised_prompt":"x"}]}`, "RUNTIME_FACT_VERIFIED"},
		{"output-count-midjourney", `{"status":"succeeded","data":[{"url":"https://example/a"},{"b64_json":"encoded"}]}`, "RUNTIME_FACT_VERIFIED"},
		{"seedance-example", `{"status":"succeeded","usage":{"completion_tokens":100},"duration_sec":5}`, "RUNTIME_FACT_VERIFIED"},
		{"seedance-example", `{"status":"succeeded","usage":{"completion_tokens":100}}`, "RUNTIME_FACT_MISSING"},
	}
	for _, tc := range tests {
		t.Run(tc.id+tc.want+tc.body, func(t *testing.T) {
			got, err := VerifyCanaryFixture(tc.id, []byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Status)
			assert.NotEmpty(t, got.SchemaHash)
		})
	}
	verified, err := VerifyCanaryFixture("tts-async", []byte(`{"task_id":"t1","status":"succeeded","characters":6}`))
	require.NoError(t, err)
	canary := CanaryCase{ID: "tts-async", UnitPricePoints: "0.132066"}
	require.NoError(t, CompareCanaryProviderPoints(canary, &verified, "0.792396"))
	assert.Equal(t, "PROVIDER_POINTS_MATCH", verified.CostStatus)
	assert.Equal(t, "0.792396", verified.ExpectedPoints)
	require.NoError(t, CompareCanaryProviderPoints(canary, &verified, "0.8"))
	assert.Equal(t, "UNEXPLAINED_PROVIDER_CHARGE", verified.CostStatus)
	image, err := VerifyCanaryFixture("output-count-midjourney", []byte(`{"status":"succeeded","data":[{"url":"https://example/a"},{"b64_json":"encoded"}]}`))
	require.NoError(t, err)
	assert.Equal(t, 1, image.ObservedFacts["usable_image_outputs"])
}
