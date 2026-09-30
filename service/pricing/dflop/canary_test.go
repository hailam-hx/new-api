package dflop

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

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
	ready.Status = "READY_FOR_PAID_AUTHORIZATION"
	ready.RequestReadiness, ready.FixtureReadiness, ready.ExecutorReadiness = "VALIDATED", "AVAILABLE", "READY"
	full := CanaryAuthorization{Execute: true, InvocationID: "invocation-1", CaseID: ready.ID, ChannelID: 1, CatalogHash: plan.CatalogHash, MaxCostPoints: "1800", MaxCostUSD: "5"}
	full.Confirmation = CanaryConfirmation(full.InvocationID, full.CaseID, full.ChannelID, full.CatalogHash, full.MaxCostPoints)
	for _, auth := range []CanaryAuthorization{
		{CaseID: ready.ID, MaxCostUSD: "5"},
		{Execute: true, CaseID: ready.ID, MaxCostUSD: "5"},
		{Execute: true, CaseID: ready.ID, ChannelID: 1, CatalogHash: plan.CatalogHash, MaxCostPoints: "1800", MaxCostUSD: "5"},
	} {
		assert.ErrorContains(t, CheckCanaryAuthorization(auth, ready, ready), "CANARY_AUTHORIZATION_REQUIRED")
	}
	under := full
	under.MaxCostUSD = "4"
	assert.ErrorContains(t, CheckCanaryAuthorization(under, ready, ready), "BLOCKED_BY_BUDGET")
	changed := ready
	changed.EstimatedMaxPoints, changed.EstimatedMaxUSD = "2400", "6"
	assert.ErrorContains(t, CheckCanaryAuthorization(full, ready, changed), "CANARY_CATALOG_CHANGED")
	assert.NoError(t, CheckCanaryAuthorization(full, ready, ready))
	assert.ErrorContains(t, CheckCanaryAuthorization(full, plan.Cases[0], plan.Cases[0]), "CANARY_DESIGN_UNVERIFIED")
}

func TestCanaryBalanceAuditUsesOnlyFreeGET(t *testing.T) {
	requests := make([]string, 0, 1)
	responseBody := `{"remaining_usd":1.25,"used_usd":2,"total_usd":3.25}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		assert.Equal(t, "Bearer fake-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	balance, err := (CanaryTransport{HTTP: server.Client(), BaseURL: server.URL, Key: "fake-secret"}).FetchCanaryBalance(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /v1/key/balance"}, requests)
	assert.Equal(t, "1.25", balance.RemainingUSD)
	assert.Equal(t, "2", balance.UsedUSD)
	assert.Equal(t, "3.25", balance.TotalUSD)
	assert.False(t, balance.FetchedAt.IsZero())
	responseBody = `{"remaining_usd":-1,"used_usd":2,"total_usd":3.25}`
	_, err = (CanaryTransport{HTTP: server.Client(), BaseURL: server.URL, Key: "fake-secret"}).FetchCanaryBalance(context.Background())
	assert.ErrorContains(t, err, "CANARY_BALANCE_INVALID")
	assert.Equal(t, []string{"GET /v1/key/balance", "GET /v1/key/balance"}, requests)
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
		{"tts-async", `{"id":"t1","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://example/a","duration_sec":1.2}`, "RUNTIME_FACT_VERIFIED"},
		{"tts-async", `{"task_id":"t1","status":"succeeded"}`, "RUNTIME_FACT_MISSING"},
		{"tts-async", `{"task_id":"t1","status":"failed"}`, "EXECUTED_FAILURE"},
		{"grok-tool-chat", `{"usage":{"prompt_tokens":10,"completion_tokens":2,"num_server_side_tools_used":1}}`, "RUNTIME_FACT_VERIFIED"},
		{"grok-tool-chat", `{"usage":{"prompt_tokens":10,"completion_tokens":2}}`, "RUNTIME_FACT_MISSING"},
		{"grok-tool-chat", `{"tools":[{"type":"function"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`, "RUNTIME_FACT_MISSING"},
		{"suno-generation", `{"id":"t1","model":"suno-v3.5","status":"succeeded","tracks":[{"clip_id":"a","audio_url":"https://example/a"},{"clip_id":"b","audio_url":"https://example/b"}]}`, "RUNTIME_FACT_VERIFIED"},
		{"suno-generation", `{"id":"t1","model":"suno-v3.5","status":"succeeded","tracks":[]}`, "RUNTIME_FACT_MISSING"},
		{"suno-generation", `{"id":"t1","model":"suno-v3.5","status":"expired"}`, "EXECUTED_FAILURE"},
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
	verified, err := VerifyCanaryFixture("tts-async", []byte(`{"id":"t1","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://example/a"}`))
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

func TestBoundedCanaryCasesAndReplay(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"voice-tts-pro","pricing":{"category":"audio","endpoint_type":"tts","callable":true,"price_per_tts_char":"0.132066"},"billing":{"features":["tts_char"]},"caps":{"surfaces":["audio"]}},{"id":"suno-v3.5","pricing":{"category":"audio","endpoint_type":"music","callable":true,"price_per_music_generation":"20.4"},"billing":{"features":["music"]},"caps":{"surfaces":["audio"]}},{"id":"voice-clone-pro","pricing":{"category":"audio","endpoint_type":"voice_clone","callable":true,"price_per_voice_clone":"1800"},"billing":{"features":["voice_clone"]},"caps":{"surfaces":["audio"]}}]}`)
	plan, err := BuildCanaryPlan(1, catalog, []byte(`{"unit":"points","points_per_cny":60}`), "etag", "0.15")
	require.NoError(t, err)
	require.Len(t, plan.Cases, 3)
	assert.Equal(t, "0.792396", plan.Cases[0].EstimatedMaxPoints)
	assert.Equal(t, "READY_FOR_PAID_AUTHORIZATION", plan.Cases[0].Status)
	underBudget := CanaryAuthorization{Execute: true, InvocationID: "small-budget", CaseID: "tts-async", ChannelID: 1, CatalogHash: plan.CatalogHash, MaxCostPoints: "0.7", MaxCostUSD: "1"}
	underBudget.Confirmation = CanaryConfirmation(underBudget.InvocationID, underBudget.CaseID, underBudget.ChannelID, underBudget.CatalogHash, underBudget.MaxCostPoints)
	assert.ErrorContains(t, CheckCanaryAuthorization(underBudget, plan.Cases[0], plan.Cases[0]), "BLOCKED_BY_BUDGET")
	assert.Equal(t, "20.4", plan.Cases[1].EstimatedMaxPoints)
	assert.Equal(t, "READY_FOR_PAID_AUTHORIZATION", plan.Cases[1].Status)
	assert.Equal(t, "BOUNDED", plan.Cases[2].CostReadiness)
	assert.Equal(t, "BLOCKED_MISSING_FIXTURE", plan.Cases[2].Status)
	request, err := BuildCanaryRequest(plan.Cases[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"voice-tts-pro","input":"Hello.","async":true}`, request.Body)
	assert.NotEmpty(t, request.RequestHash)
	suno, err := BuildCanaryRequest(plan.Cases[1])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"suno-v3.5","prompt":"Short calm instrumental melody"}`, suno.Body)
	for _, count := range []int{1, 2} {
		body := `{"id":"task","model":"suno-v3.5","status":"succeeded","tracks":[{"clip_id":"a","audio_url":"https://example/a"}]}`
		if count == 2 {
			body = `{"id":"task","model":"suno-v3.5","status":"succeeded","tracks":[{"clip_id":"a","audio_url":"https://example/a"},{"clip_id":"b","audio_url":"https://example/b"}]}`
		}
		verified, err := VerifyCanaryFixture("suno-generation", []byte(body))
		require.NoError(t, err)
		require.NoError(t, CompareCanaryProviderPoints(plan.Cases[1], &verified, ""))
		assert.Equal(t, "20.4", verified.ExpectedPoints)
	}
	tooMany, err := VerifyCanaryFixture("tts-async", []byte(`{"id":"task","model":"voice-tts-pro","status":"succeeded","characters":7,"audio_url":"https://example/a"}`))
	require.NoError(t, err)
	assert.Equal(t, "CANARY_COST_MODEL_INVALID", tooMany.Status)
	changed := strings.Replace(string(catalog), "0.132066", "0.2", 1)
	changedPlan, err := BuildCanaryPlan(1, []byte(changed), []byte(`{"unit":"points","points_per_cny":60}`), "etag2", "0.15")
	require.NoError(t, err)
	assert.Equal(t, "1.2", changedPlan.Cases[0].EstimatedMaxPoints)
	ambiguous := strings.Replace(string(catalog), `"features":["music"]`, `"features":["music","per_image"]`, 1)
	ambiguousPlan, err := BuildCanaryPlan(1, []byte(ambiguous), []byte(`{"unit":"points","points_per_cny":60}`), "etag", "0.15")
	require.NoError(t, err)
	assert.NotEqual(t, "READY_FOR_PAID_AUTHORIZATION", ambiguousPlan.Cases[1].Status)
}

func TestCanaryTransportPreflightSubmitAndResume(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"voice-tts-pro","pricing":{"category":"audio","endpoint_type":"tts","callable":true,"price_per_tts_char":"0.132066"},"billing":{"features":["tts_char"]},"caps":{"surfaces":["audio"]}}]}`)
	currency := []byte(`{"unit":"points","points_per_cny":60}`)
	plan, err := BuildCanaryPlan(1, catalog, currency, "etag", "0.15")
	require.NoError(t, err)
	var posts int
	var receivedKey string
	balanceUSD := "10"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer fake-secret", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/catalog":
			w.Header().Set("ETag", "etag")
			_, _ = w.Write(catalog)
		case "/api/v1/config/currency":
			_, _ = w.Write(currency)
		case "/v1/key/balance":
			_, _ = w.Write([]byte(`{"remaining_usd":"` + balanceUSD + `","used_usd":"0","total_usd":"10"}`))
		case "/v1/audio/speech":
			posts++
			receivedKey = r.Header.Get("Idempotency-Key")
			body, _ := io.ReadAll(r.Body)
			assert.JSONEq(t, `{"model":"voice-tts-pro","input":"Hello.","async":true}`, string(body))
			w.Header().Set("x-gateway-trace", "submit-trace")
			_, _ = w.Write([]byte(`{"id":"task-1","status":"pending"}`))
		case "/v1/audio/speech/task-1":
			w.Header().Set("x-gateway-trace", "terminal-trace")
			_, _ = w.Write([]byte(`{"id":"task-1","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://example/audio"}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	auth := CanaryAuthorization{Execute: true, InvocationID: "invocation-2", CaseID: "tts-async", ChannelID: 1, CatalogHash: plan.CatalogHash, MaxCostPoints: "0.792396", MaxCostUSD: "0.01"}
	auth.Confirmation = CanaryConfirmation(auth.InvocationID, auth.CaseID, auth.ChannelID, auth.CatalogHash, auth.MaxCostPoints)
	transport := CanaryTransport{HTTP: server.Client(), BaseURL: server.URL, Key: "fake-secret"}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path, record, err := transport.PrepareCanaryExecution(context.Background(), dir, plan, plan.Cases[0], auth)
	require.NoError(t, err)
	assert.Equal(t, "PREPARED", record.State)
	assert.Equal(t, 0, posts)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), "fake-secret")
	record, err = transport.SubmitCanaryExecution(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.Equal(t, record.IdempotencyKey, receivedKey)
	assert.Equal(t, "submit-trace", record.SubmitTrace)
	record, err = transport.SubmitCanaryExecution(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	body, trace, err := transport.PollCanaryExecution(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "terminal-trace", trace)
	captured, err := os.ReadFile(dir + "/captures/" + record.InvocationID + ".json")
	require.NoError(t, err)
	assert.Contains(t, string(captured), `"origin":"LIVE_DFLOP_CANARY"`)
	assert.Contains(t, string(captured), `"terminal_trace":"terminal-trace"`)
	assert.NotContains(t, string(captured), "https://example/audio")
	verified, err := VerifyCanaryFixture("tts-async", body)
	require.NoError(t, err)
	assert.Equal(t, "RUNTIME_FACT_VERIFIED", verified.Status)
	record.TaskID = ""
	record.State = "PREPARED"
	record.RequestBody = `{"model":"voice-tts-pro","input":"Changed.","async":true}`
	encoded, err := common.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0600))
	wrongCredential := transport
	wrongCredential.Key = "different-key"
	_, err = wrongCredential.SubmitCanaryExecution(context.Background(), path)
	assert.ErrorContains(t, err, "CANARY_CHANNEL_CHANGED")
	_, err = transport.SubmitCanaryExecution(context.Background(), path)
	assert.ErrorContains(t, err, "CANARY_REQUEST_CHANGED")
	assert.Equal(t, 1, posts)
	balanceUSD = "0"
	_, _, err = transport.PrepareCanaryExecution(context.Background(), dir, plan, plan.Cases[0], auth)
	assert.ErrorContains(t, err, "CANARY_INSUFFICIENT_BALANCE")
	assert.Equal(t, 1, posts)
	balanceUSD = "10"
	catalog = []byte(strings.Replace(string(catalog), "0.132066", "0.2", 1))
	_, _, err = transport.PrepareCanaryExecution(context.Background(), dir, plan, plan.Cases[0], auth)
	assert.ErrorContains(t, err, "CANARY_CATALOG_CHANGED")
	assert.Equal(t, 1, posts)
	record.CreatedAt = time.Now().Add(-8 * 24 * time.Hour)
	record.RequestBody = `{"model":"voice-tts-pro","input":"Hello.","async":true}`
	encoded, err = common.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0600))
	_, err = transport.SubmitCanaryExecution(context.Background(), path)
	assert.ErrorContains(t, err, "CANARY_RETRY_UNSAFE")
}

func TestCanaryLostSubmitResponseReusesKeyAndBody(t *testing.T) {
	catalog := []byte(`{"schema_version":"1.0","currency":"points","aliases":{},"models":[{"id":"voice-tts-pro","pricing":{"category":"audio","endpoint_type":"tts","callable":true,"price_per_tts_char":"0.132066"},"billing":{"features":["tts_char"]},"caps":{"surfaces":["audio"]}}]}`)
	plan, err := BuildCanaryPlan(1, catalog, []byte(`{"unit":"points","points_per_cny":60}`), "etag", "0.15")
	require.NoError(t, err)
	var firstKey, firstBody string
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/catalog":
			_, _ = w.Write(catalog)
		case "/api/v1/config/currency":
			_, _ = w.Write([]byte(`{"unit":"points","points_per_cny":60}`))
		case "/v1/key/balance":
			_, _ = w.Write([]byte(`{"remaining_usd":"1"}`))
		case "/v1/audio/speech":
			posts++
			body, _ := io.ReadAll(r.Body)
			if posts == 1 {
				firstKey, firstBody = r.Header.Get("Idempotency-Key"), string(body)
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
			assert.Equal(t, firstKey, r.Header.Get("Idempotency-Key"))
			assert.Equal(t, firstBody, string(body))
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	auth := CanaryAuthorization{Execute: true, InvocationID: "invocation-3", CaseID: "tts-async", ChannelID: 1, CatalogHash: plan.CatalogHash, MaxCostPoints: "0.792396", MaxCostUSD: "0.01"}
	auth.Confirmation = CanaryConfirmation(auth.InvocationID, auth.CaseID, auth.ChannelID, auth.CatalogHash, auth.MaxCostPoints)
	transport := CanaryTransport{HTTP: server.Client(), BaseURL: server.URL, Key: "fake-secret"}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	path, _, err := transport.PrepareCanaryExecution(context.Background(), dir, plan, plan.Cases[0], auth)
	require.NoError(t, err)
	_, err = transport.SubmitCanaryExecution(context.Background(), path)
	assert.ErrorContains(t, err, "CANARY_HTTP_504")
	record, err := transport.SubmitCanaryExecution(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "task-1", record.TaskID)
	assert.Equal(t, 2, posts)
	assert.NotEmpty(t, firstKey)
}
