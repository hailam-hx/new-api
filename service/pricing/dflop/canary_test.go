package dflop

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalEvidenceRecoveryIsGETOnlyAndFollowsEveryCursor(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer historical-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/logs":
			assert.Equal(t, "key", r.URL.Query().Get("scope"))
			if r.URL.Query().Get("cursor") == "" {
				_, _ = io.WriteString(w, `{"currency":"points","scope":"key","data":[{"id":1,"model":"tvod-midjourney-v7","status":"success","unit_type":"image","unit_count":4,"cost":"8.4","task_id":"image-1"}],"has_more":true,"next_cursor":"opaque/+="}`)
			} else {
				assert.Equal(t, "opaque/+=", r.URL.Query().Get("cursor"))
				_, _ = io.WriteString(w, `{"currency":"points","scope":"key","data":[],"has_more":false}`)
			}
		case "/v1/logs/1":
			_, _ = io.WriteString(w, `{"id":1,"kind":"image","request":[{"k":"prompt","v":"private customer prompt"},{"k":"resolution","v":"1024x1024"}],"upstream":[{"k":"api_key","v":"historical-secret"}]}`)
		case "/v1/images/generations":
			_, _ = io.WriteString(w, `{"data":[{"id":"image-1","model":"tvod-midjourney-v7","status":"succeeded","unit_count":4,"cost":"8.4"}],"next_cursor":null}`)
		case "/v1/images/generations/image-1":
			_, _ = io.WriteString(w, `{"id":"image-1","data":[{"url":"https://private.example/image?secret=123"}]}`)
		default:
			_, _ = io.WriteString(w, `{"data":[]}`)
		}
	}))
	defer server.Close()
	transport := HistoricalTransport{HTTP: server.Client(), BaseURL: server.URL, Key: "historical-secret"}
	_, _, err := transport.Read(context.Background(), http.MethodPost, "/v1/audio/speech", nil)
	require.ErrorContains(t, err, "GET_ONLY")
	assert.Empty(t, requests)
	_, _, err = transport.Read(context.Background(), http.MethodGet, "/v1/logs/../chat/completions", url.Values{})
	require.Error(t, err)
	assert.Empty(t, requests)
	report, err := RecoverHistoricalEvidence(context.Background(), transport, HistoricalOptions{ChannelID: 1, Scope: "key", Period: "30d", CatalogHash: "catalog-hash"}, []Item{{ModelID: "tvod-midjourney-v7", Callable: true, ReasonCode: "UNVERIFIED_OUTPUT_COUNT", BillingFeatures: []string{"per_image"}, Prices: map[string]Price{"price_per_image": {Credits: "2.1"}}}})
	require.NoError(t, err)
	require.Len(t, report.Models, 1)
	assert.Equal(t, 1, report.Models[0].SuccessfulCallsFound)
	assert.Equal(t, "MATCH", report.Models[0].ReconciliationResult)
	assert.True(t, report.Models[0].QuantityVerified)
	assert.False(t, report.Models[0].CanUnlock, "a matching ledger does not establish a New API production binding")
	encoded, err := common.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "historical-secret")
	assert.NotContains(t, string(encoded), "private customer prompt")
	assert.NotContains(t, string(encoded), "private.example")
	assert.Contains(t, string(encoded), "CAPTURED_REAL_DFLOP_LEDGER")
	assert.Contains(t, string(encoded), "CAPTURED_REAL_DFLOP_TASK")
	assert.Equal(t, 2, report.Requests[0].Pages)
}

func TestHistoricalEvidenceReconciliationDoesNotUsePercentageTolerance(t *testing.T) {
	for _, tc := range []struct{ expected, settled, want string }{
		{"0.792396", "0.792396", "MATCH"},
		{"0.792396", "0.7924", "ROUNDING_MATCH"},
		{"0.792396", "0.80", "MISMATCH"},
		{"0.792396", "", "INSUFFICIENT_EVIDENCE"},
	} {
		assert.Equal(t, tc.want, ReconcileHistoricalPoints(tc.expected, tc.settled, 4))
	}
}

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

func TestHistoricalEvidenceDoesNotAddAlternativeVideoTariffs(t *testing.T) {
	report := HistoricalModelReport{RuntimeFields: map[string]any{}, ReconciliationResult: "INSUFFICIENT_EVIDENCE"}
	item := Item{BillingFeatures: []string{"video_second", "video_token", "video_token_formula_seedance_2_0"}, Prices: map[string]Price{"video_token_tier:default@480p": {Credits: "552"}, "video_tier:480p": {Credits: "5.544288"}}}
	ObserveHistoricalBilling(&report, item, map[string]any{"status": "succeeded", "resolution": "480p", "service_tier": "default", "duration_sec": 4, "usage": map[string]any{"completion_tokens": 40594}, "cost": "22.17715200"}, nil)
	assert.True(t, report.QuantityVerified)
	assert.Equal(t, "INSUFFICIENT_EVIDENCE", report.ReconciliationResult)
	assert.Empty(t, report.Reconciliations, "presence of two prices does not establish additive billing")
	assert.True(t, report.SemanticsVerified)
}

func TestDFLOPContractAlignmentGuardsAndExactCorrelation(t *testing.T) {
	base := "https://api.dflop.top"
	for _, body := range []string{`{}`, `{"system":[{"cache_control":{"type":"ephemeral"}}]}`, `{"system":[{"cache_control":{"type":"ephemeral","ttl":"5m"}}]}`} {
		require.NoError(t, DFLOPCacheContract(base, "claude-example", []byte(body), nil))
	}
	require.ErrorContains(t, DFLOPCacheContract(base, "claude-example", []byte(`{"messages":[{"content":[{"cache_control":{"ttl":"1h"}}]}]}`), nil), "UNSUPPORTED_DFLOP_CACHE_TTL_1H")
	require.ErrorContains(t, DFLOPCacheContract(base, "claude-example", nil, &dto.Usage{ClaudeCacheCreation1hTokens: 1}), "UNSUPPORTED_DFLOP_CACHE_TTL_1H")
	require.NoError(t, DFLOPCacheContract("https://api.anthropic.com", "claude-example", nil, &dto.Usage{ClaudeCacheCreation1hTokens: 1}))
	for _, usage := range []*dto.Usage{nil, {}, {ClaudeCacheCreation5mTokens: 26}, {PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 26}}} {
		require.NoError(t, DFLOPCacheContract(base, "claude-example", nil, usage))
	}
	records := []LocalExecution{{TaskRecordID: 1, UpstreamTaskID: "real-task", RequestID: "real-request", PluginKey: "dflop-tts", ClientModel: "client", UpstreamModel: "voice-tts-pro", ChannelID: 1}}
	exact := CorrelateRuntimeBinding("real-task", "", "voice-tts-pro", 1, records)
	assert.Equal(t, "EXACT_TASK_ID", exact.Confidence)
	assert.True(t, exact.BindingVerified)
	assert.False(t, exact.SettledCostReconciled, "missing key-scope cost cannot remove exact binding")
	assert.Equal(t, "EXACT_REQUEST_ID", CorrelateRuntimeBinding("", "real-request", "voice-tts-pro", 1, records).Confidence)
	records[0].TraceID = "real-trace"
	assert.Equal(t, "EXACT_TRACE_ID", CorrelateRuntimeBinding("", "", "voice-tts-pro", 1, records, "real-trace").Confidence)
	records[0].UpstreamTaskID = ""
	records[0].TaskID = "real-task"
	assert.Equal(t, "EXACT_TASK_ID", CorrelateRuntimeBinding("real-task", "", "voice-tts-pro", 1, records).Confidence)
	assert.Equal(t, "UNVERIFIED", CorrelateRuntimeBinding("similar-task", "", "voice-tts-pro", 1, records).Confidence)
	assert.Equal(t, "UNVERIFIED", CorrelateRuntimeBinding("real-task", "", "voice-tts-pro", 2, records).Confidence)
}

func TestHistoricalSeedanceTokenAndLiteSecondLeg(t *testing.T) {
	for _, tc := range []struct {
		lite     bool
		expected string
	}{{false, "0.1"}, {true, "10.1"}} {
		report := HistoricalModelReport{ReconciliationResult: "INSUFFICIENT_EVIDENCE"}
		features := []string{"video_token", "video_second", "video_token_formula_seedance_2_0"}
		if tc.lite {
			features = append(features, "video_two_stage")
		}
		item := Item{BillingFeatures: features, Prices: map[string]Price{"video_token_tier:default@720p": {Credits: "1000"}, "video_second_stage:720p": {Credits: "2.5"}, "video_tier:720p": {Credits: "999"}}}
		ObserveHistoricalBilling(&report, item, map[string]any{"status": "succeeded", "resolution": "720p", "input_video_duration_sec": 0, "duration_sec": 4, "usage": map[string]any{"completion_tokens": 100}, "cost": tc.expected}, nil)
		require.Len(t, report.Reconciliations, 1)
		assert.Equal(t, tc.expected, report.Reconciliations[0].ExpectedPoints)
		assert.Equal(t, "MATCH", report.ReconciliationResult)
	}
}

func TestGPTEndpointSpecificBillingClassification(t *testing.T) {
	matrix := EndpointBillingMatrix([]Item{
		{ModelID: "gpt-5-example", Category: "text", Protocols: []string{"openai_chat", "openai_responses"}, BillingFeatures: []string{"token", "per_image", "fast_mode"}},
		{ModelID: "gpt-image-example", Category: "image", BillingFeatures: []string{"image_token"}},
	})
	require.Len(t, matrix, 4)
	assert.Equal(t, "/v1/chat/completions", matrix[0]["endpoint"])
	assert.Equal(t, "/v1/responses", matrix[1]["endpoint"])
	assert.Equal(t, "chat_fast", matrix[2]["profile"])
	assert.Equal(t, "MISSING_FAST_SELECTOR", matrix[2]["reason_code"])
	assert.Equal(t, []string{"token"}, matrix[0]["applicable_features"])
	for _, entry := range matrix[:3] {
		assert.Equal(t, false, entry["PRICE_VERIFIED"], "missing authenticated prices cannot verify a profile")
		assert.Equal(t, false, entry["global_additive_billing_verified"])
	}
	assert.Equal(t, "/v1/images/generations", matrix[3]["endpoint"])
	assert.Equal(t, []string{"image_token"}, matrix[3]["applicable_features"])
}

func TestHistoricalEvidenceUsesOriginalIdentityAndAccountScope(t *testing.T) {
	for _, terminal := range []string{`{"id":"other","model":"voice-tts-pro"}`, `{"model":"voice-tts-pro"}`, `{"id":"real","task_id":"other","model":"voice-tts-pro"}`} {
		t.Run(terminal, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/logs":
					assert.Equal(t, "account", r.URL.Query().Get("scope"))
					assert.Equal(t, "all", r.URL.Query().Get("period"))
					_, _ = io.WriteString(w, `{"currency":"points","scope":"account","data":[{"id":1,"model":"voice-tts-pro","request_id":"original-request","status":"success"}],"has_more":false}`)
				case "/v1/logs/1":
					assert.Equal(t, "account", r.URL.Query().Get("scope"))
					w.WriteHeader(http.StatusForbidden)
				case "/v1/audio/speech":
					_, _ = io.WriteString(w, `{"data":[{"id":"real","model":"voice-tts-pro","status":"succeeded"}]}`)
				case "/v1/audio/speech/real":
					_, _ = io.WriteString(w, terminal)
				default:
					_, _ = io.WriteString(w, `{"data":[]}`)
				}
			}))
			defer server.Close()
			report, err := RecoverHistoricalEvidence(context.Background(), HistoricalTransport{HTTP: server.Client(), BaseURL: server.URL, Key: "secret"}, HistoricalOptions{Scope: "account", Period: "all"}, []Item{{ModelID: "voice-tts-pro", Callable: true, ReasonCode: "UNVERIFIED"}})
			require.NoError(t, err)
			var detailError string
			for _, request := range report.Requests {
				if request.Path == "/v1/logs/1" {
					detailError = request.Error
				}
			}
			assert.Equal(t, "ACCOUNT_SCOPE_PERMISSION_REQUIRED", detailError)
			for _, capture := range report.Captures {
				assert.NotEqual(t, "task-poll:real", capture.EvidenceID, "unbound or conflicting terminal response cannot become exact task evidence")
				if capture.EvidenceID == "log:1" {
					assert.Empty(t, capture.GatewayTrace, "request ID is not a gateway trace")
				}
			}
			assert.False(t, report.Models[0].CanUnlock)
		})
	}
	report := HistoricalReport{}
	model := HistoricalModelReport{RuntimeFields: map[string]any{}}
	require.NoError(t, captureHistoricalRecord(&report, &model, "secret", "log:2", "ledger", "/v1/logs", map[string]any{"request_id": "request", "trace_id": "original-trace"}, nil))
	require.Len(t, report.Captures, 1)
	assert.Equal(t, "original-trace", report.Captures[0].GatewayTrace)
}

func TestHistoricalTechnicalDetailRejectsContradictoryIdentity(t *testing.T) {
	for _, detail := range []string{`{"id":2,"request_id":"request-1"}`, `{"id":1,"request_id":"other-request"}`, `{"id":1,"task_id":"other-task"}`} {
		t.Run(detail, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/logs":
					_, _ = io.WriteString(w, `{"currency":"points","scope":"key","data":[{"id":1,"model":"claude-test","request_id":"request-1","task_id":"task-1"}],"has_more":false}`)
				case "/v1/logs/1":
					_, _ = io.WriteString(w, detail)
				default:
					_, _ = io.WriteString(w, `{"data":[]}`)
				}
			}))
			defer server.Close()
			report, err := RecoverHistoricalEvidence(context.Background(), HistoricalTransport{HTTP: server.Client(), BaseURL: server.URL}, HistoricalOptions{}, []Item{{ModelID: "claude-test", Callable: true, ReasonCode: "UNVERIFIED"}})
			require.NoError(t, err)
			require.Len(t, report.Captures, 1)
			assert.Equal(t, "log:1", report.Captures[0].EvidenceID)
			assert.Contains(t, report.Models[0].Notes, "TECHNICAL_LOG_IDENTITY_MISMATCH")
		})
	}
}

func TestAccountScopeDenialIsNotAbsenceOfEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/logs" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	report, err := RecoverHistoricalEvidence(context.Background(), HistoricalTransport{HTTP: server.Client(), BaseURL: server.URL}, HistoricalOptions{Scope: "account", Period: "all"}, []Item{{ModelID: "claude-test", Callable: true}})
	require.NoError(t, err)
	require.Len(t, report.Models, 1)
	assert.Contains(t, report.Models[0].Notes, "ACCOUNT_SCOPE_PERMISSION_REQUIRED")
	assert.NotContains(t, report.Models[0].Notes, "NO_HISTORICAL_EVIDENCE")
}

func TestClaudeCanaryTargetScopedPricingAuthorization(t *testing.T) {
	input := ClaudeCanaryPricingInputs{Model: "claude-sonnet-5", BillingMode: "tiered_expr", BillingExpr: `tier("dflop", p * 1.5 + c * 7.5 + cr * 0.15 + cc * 1.875)`, QuotaPerUnit: "500000", GroupRatio: "1", EvaluatorHash: strings.Repeat("a", 64)}
	fingerprint, canonical, err := ClaudeCanaryPricingFingerprint(input)
	require.NoError(t, err)
	assert.NotContains(t, string(canonical), "global_pricing_version")
	binding := ClaudeCanaryPricingBinding{Execute: true, InvocationID: "new-invocation", CatalogHash: strings.Repeat("b", 64), ConfigHash: strings.Repeat("c", 64), SourceChannelID: 1, CredentialFingerprint: "afd454eb19c56131", TargetFingerprint: fingerprint, MaxPoints: "14.20382208", GlobalPricingVersion: "old-global"}
	invalidated := map[string]string{"old-invocation": "INVALIDATED_PRICING_VERSION_DRIFT", "new-invocation": "AUTHORIZED"}
	require.NoError(t, CheckClaudeCanaryPricingBinding(binding, binding, input, invalidated))
	require.ErrorContains(t, CheckClaudeCanaryPricingBinding(binding, binding, input, nil), "STATE_UNKNOWN")
	t.Run("unrelated pricing changes global version without changing target settlement", func(t *testing.T) {
		versions := map[string]string{"claude-sonnet-5": model.ModelPricingVersion(model.PricingValues{"billing_setting.billing_mode": input.BillingMode, "billing_setting.billing_expr": input.BillingExpr}), "unrelated": model.ModelPricingVersion(model.PricingValues{"ModelRatio": 1})}
		oldVersion := model.ModelPricingVersion(model.PricingValues{"versions": versions})
		snap := &billingexpr.BillingSnapshot{ExprString: input.BillingExpr, ExprHash: billingexpr.ExprHashString(input.BillingExpr), ExprVersion: 1, QuotaPerUnit: 500000, GroupRatio: 1}
		params := billingexpr.TokenParams{P: 100, C: 3, CR: 1000, CC: 2000, Len: 3100}
		before, err := billingexpr.ComputeTieredQuota(snap, params)
		require.NoError(t, err)
		versions["unrelated"] = model.ModelPricingVersion(model.PricingValues{"ModelRatio": 2})
		fresh := binding
		fresh.GlobalPricingVersion = model.ModelPricingVersion(model.PricingValues{"versions": versions})
		assert.NotEqual(t, oldVersion, fresh.GlobalPricingVersion)
		fp, _, err := ClaudeCanaryPricingFingerprint(input)
		require.NoError(t, err)
		assert.Equal(t, fingerprint, fp)
		after, err := billingexpr.ComputeTieredQuota(snap, params)
		require.NoError(t, err)
		assert.Equal(t, before, after)
		require.NoError(t, CheckClaudeCanaryPricingBinding(binding, fresh, input, invalidated))
	})
	for _, field := range []string{"target price", "BillingExpr", "quota conversion", "group ratio", "evaluator code", "catalog", "config", "channel", "credential", "hard max", "old invocation"} {
		t.Run(field, func(t *testing.T) {
			fresh := binding
			changed := input
			switch field {
			case "target price":
				changed.BillingExpr = strings.Replace(input.BillingExpr, "p * 1.5", "p * 2", 1)
			case "BillingExpr":
				changed.BillingExpr = strings.Replace(input.BillingExpr, "cc * 1.875", "cc * 3", 1)
			case "quota conversion":
				changed.QuotaPerUnit = "1000000"
			case "group ratio":
				changed.GroupRatio = "2"
			case "evaluator code":
				changed.EvaluatorHash = strings.Repeat("d", 64)
			case "catalog":
				fresh.CatalogHash = strings.Repeat("d", 64)
			case "config":
				fresh.ConfigHash = strings.Repeat("d", 64)
			case "channel":
				fresh.SourceChannelID = 2
			case "credential":
				fresh.CredentialFingerprint = "different"
			case "hard max":
				fresh.MaxPoints = "15"
			case "old invocation":
				fresh.InvocationID = "old-invocation"
				freshCopy := fresh
				require.ErrorContains(t, CheckClaudeCanaryPricingBinding(freshCopy, fresh, changed, invalidated), "INVALIDATED")
				return
			}
			require.Error(t, CheckClaudeCanaryPricingBinding(binding, fresh, changed, invalidated))
			if changed != input {
				fp, _, err := ClaudeCanaryPricingFingerprint(changed)
				require.NoError(t, err)
				assert.NotEqual(t, fingerprint, fp)
			}
		})
	}
	t.Run("equivalent decimal formatting", func(t *testing.T) {
		equivalent := input
		equivalent.GroupRatio = "1.0000"
		equivalent.QuotaPerUnit = "5e5"
		equivalent.BillingExpr = `v1:tier("dflop", p * 1.5000 + c * 7.50 + cr * 0.1500 + cc * 1.8750)`
		fp, body, err := ClaudeCanaryPricingFingerprint(equivalent)
		require.NoError(t, err)
		assert.Equal(t, fingerprint, fp)
		assert.Equal(t, canonical, body)
		fresh := binding
		fresh.MaxPoints = "14.2038220800"
		require.NoError(t, CheckClaudeCanaryPricingBinding(binding, fresh, equivalent, invalidated))
	})
	t.Run("unknown evaluator inputs fail closed", func(t *testing.T) {
		changed := input
		changed.BillingExpr = `tier("dflop", p * 1.5 + c * 7.5 + cr * 0.15 + cc * 1.875) * param("fast")`
		_, _, err := ClaudeCanaryPricingFingerprint(changed)
		require.Error(t, err)
	})
	t.Run("proposal is not authorization", func(t *testing.T) {
		proposal := binding
		proposal.Execute = false
		require.ErrorContains(t, CheckClaudeCanaryPricingBinding(proposal, binding, input, invalidated), "AUTHORIZATION_REQUIRED")
	})
}

func TestClaudeCacheIdentityPreservesPrefixSemantics(t *testing.T) {
	base := `{"model":"claude-sonnet-5","max_tokens":64,"stream":false,"thinking":{"type":"disabled"},"tools":[{"name":"one"},{"name":"two"}],"system":[{"type":"text","text":"café\n ","cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"role":"user","content":"A"}]}`
	headers := http.Header{"Anthropic-Version": {"2023-06-01"}}
	original, err := ClaudeCacheIdentity([]byte(base), headers)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, old, replacement string
		same                   bool
	}{
		{"identical", "A", "A", true},
		{"suffix", `"content":"A"`, `"content":"B"`, true},
		{"system", "café", "cafe", false},
		{"whitespace", `café\n `, `café\n  `, false},
		{"unicode", "café", `cafe\u0301`, false},
		{"tool order", `{"name":"one"},{"name":"two"}`, `{"name":"two"},{"name":"one"}`, false},
		{"thinking", `"disabled"`, `"adaptive"`, false},
		{"effort", `"thinking":`, `"output_config":{"effort":"high"},"thinking":`, false},
		{"breakpoint placement", `"system":[`, `"system":[{"type":"text","text":"earlier"},`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClaudeCacheIdentity([]byte(strings.Replace(base, tc.old, tc.replacement, 1)), headers)
			require.NoError(t, err)
			assert.Equal(t, tc.same, original.Hash == got.Hash)
		})
	}
	// Moving the marker between otherwise unchanged blocks changes the prefix.
	early := `{"model":"claude-sonnet-5","system":[{"type":"text","text":"first","cache_control":{"type":"ephemeral"}},{"type":"text","text":"second"}],"messages":[{"role":"user","content":"suffix"}]}`
	late := strings.Replace(strings.Replace(early, `,"cache_control":{"type":"ephemeral"}`, "", 1), `"text":"second"`, `"text":"second","cache_control":{"type":"ephemeral"}`, 1)
	first, err := ClaudeCacheIdentity([]byte(early), headers)
	require.NoError(t, err)
	last, err := ClaudeCacheIdentity([]byte(late), headers)
	require.NoError(t, err)
	assert.NotEqual(t, first.Hash, last.Hash)
	assert.Equal(t, "first", first.TextBytes["system[0].text"])
	assert.Equal(t, "second", last.TextBytes["system[1].text"])
	// A message breakpoint includes ordered conversation content and message role.
	messagePrefix := `{"model":"claude-sonnet-5","messages":[{"role":"user","content":[{"type":"text","text":"cached","cache_control":{"type":"ephemeral"}},{"type":"text","text":"suffix"}]}]}`
	cached, err := ClaudeCacheIdentity([]byte(messagePrefix), headers)
	require.NoError(t, err)
	suffix, err := ClaudeCacheIdentity([]byte(strings.Replace(messagePrefix, `"suffix"`, `"changed suffix"`, 1)), headers)
	require.NoError(t, err)
	assert.Equal(t, cached.Hash, suffix.Hash)
	role, err := ClaudeCacheIdentity([]byte(strings.Replace(messagePrefix, `"user"`, `"assistant"`, 1)), headers)
	require.NoError(t, err)
	assert.NotEqual(t, cached.Hash, role.Hash)
	image, err := ClaudeCacheIdentity([]byte(strings.Replace(messagePrefix, `{"type":"text","text":"suffix"}`, `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"synthetic"}}`, 1)), headers)
	require.NoError(t, err)
	assert.NotEqual(t, cached.Hash, image.Hash)
	numeric, err := ClaudeCacheIdentity([]byte(strings.Replace(base, `"thinking":`, `"unknown_control":9007199254740992,"thinking":`, 1)), headers)
	require.NoError(t, err)
	nextNumeric, err := ClaudeCacheIdentity([]byte(strings.Replace(base, `"thinking":`, `"unknown_control":9007199254740993,"thinking":`, 1)), headers)
	require.NoError(t, err)
	assert.NotEqual(t, numeric.Hash, nextNumeric.Hash)
	changedHeaders := headers.Clone()
	changedHeaders.Set("anthropic-beta", "inline-tools-2026-09-15")
	changed, err := ClaudeCacheIdentity([]byte(base), changedHeaders)
	require.NoError(t, err)
	assert.NotEqual(t, original.Hash, changed.Hash)
	_, err = ClaudeCacheIdentity([]byte(strings.Replace(base, `,"cache_control":{"type":"ephemeral","ttl":"5m"}`, "", 1)), headers)
	require.ErrorContains(t, err, "INSUFFICIENT_CAPTURE")
}

func TestEndpointBillingProfilesSeparateChatFromFastImagesAndTools(t *testing.T) {
	item := Item{ModelID: "gpt-6-sol", Category: "text", Callable: true, Protocols: []string{"openai_chat", "openai_responses"}, BillingFeatures: []string{"token", "per_image", "fast_mode"}, PriceSemantics: PriceSemantics{SourcePriceKind: "AUTHENTICATED_EFFECTIVE_PRICE", EffectiveState: "VERIFIED"}, Raw: []byte(`{"long_context_threshold_tokens":272000}`), Prices: map[string]Price{"input_per_1m": {SellingUSD: "2"}, "output_per_1m": {SellingUSD: "10"}, "cached_input_per_1m": {SellingUSD: "0.2"}, "input_per_1m_long": {SellingUSD: "4"}, "output_per_1m_long": {SellingUSD: "15"}, "cached_input_per_1m_long": {SellingUSD: "0.4"}, "price_per_image": {SellingUSD: "0.05"}}}
	profiles := BuildEndpointBillingProfiles(item)
	require.NotEmpty(t, profiles)
	standard := profiles[0]
	assert.Equal(t, "chat_standard", standard.Profile)
	assert.Equal(t, SupportedAuto, standard.Status)
	assert.True(t, standard.PriceVerified && standard.SemanticsVerified && standard.BindingVerified && standard.RuntimeUsageVerified && standard.SettlementVerified)
	assert.NotContains(t, standard.Expression, "image_count")
	require.Len(t, profiles, 4)
	assert.Equal(t, SupportedAuto, profiles[1].Status)
	assert.Equal(t, standard.Expression, profiles[1].Expression)
	assert.Equal(t, "MISSING_FAST_SELECTOR", profiles[2].ReasonCode)
	assert.Equal(t, "DEDICATED_IMAGE_BINDING_MISSING", profiles[3].ReasonCode)
	require.ErrorContains(t, DFLOPEndpointRequestContract("https://api.dflop.top", `tier("dflop_chat_standard", p * 2 + c * 10)`, "/v1/responses", billingexpr.RequestInput{Body: []byte(`{}`)}), "DFLOP_RESPONSES_PROFILE_UNVERIFIED")
	unknown := item
	unknown.BillingFeatures = append(slices.Clone(item.BillingFeatures), "new_billable_addon")
	unknownProfiles := BuildEndpointBillingProfiles(unknown)
	assert.Equal(t, "UNKNOWN_BILLING_FEATURE", unknownProfiles[0].ReasonCode)
	assert.Empty(t, unknownProfiles[0].Expression)
	raw, _, err := billingexpr.RunExprWithRequest(standard.Expression, billingexpr.TokenParams{P: 272000, Len: 272000, C: 10}, billingexpr.RequestInput{})
	require.NoError(t, err)
	assert.InDelta(t, 1088150, raw, 0.00001)
	for _, tc := range []struct{ endpoint, body, want string }{
		{"/v1/chat/completions", `{"messages":[]}`, ""},
		{"/v1/chat/completions", `{"tools":[{"type":"function"}]}`, ""},
		{"/v1/chat/completions", `{"stream":true,"tools":[{"type":"image_generation"}]}`, ""},
		{"/v1/responses", `{}`, ""},
		{"/v1/responses", `{"tools":[{"type":"image_generation"}]}`, ""},
		{"/v1/responses", `{"tools":[{"type":"unknown_paid_tool"}]}`, "DFLOP_SERVER_TOOL_UNBOUNDED"},
		{"/v1/images/generations", `{}`, "DFLOP_DEDICATED_IMAGE_PROFILE_UNVERIFIED"},
		{"/v1/chat/completions", `{"service_tier":"priority"}`, "DFLOP_FAST_MODE_PRICING_UNAVAILABLE"},
		{"/v1/chat/completions", `{"extra_body":{"service_tier":"priority"}}`, "DFLOP_FAST_MODE_PRICING_UNAVAILABLE"},
		{"/v1/chat/completions", `{"tools":[{"type":"web_search"}]}`, "DFLOP_SERVER_TOOL_UNBOUNDED"},
		{"/v1/chat/completions", `{"tools":{"web_search":true}}`, "DFLOP_SERVER_TOOL_UNBOUNDED"},
	} {
		t.Run(tc.body+tc.endpoint, func(t *testing.T) {
			err := DFLOPEndpointRequestContract("https://api.dflop.top", standard.Expression, tc.endpoint, billingexpr.RequestInput{Body: []byte(tc.body)})
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
	require.NoError(t, DFLOPEndpointRequestContract("https://another.example", standard.Expression, "/v1/responses", billingexpr.RequestInput{Body: []byte(`{}`)}))
	tools := 1
	require.ErrorContains(t, DFLOPEndpointUsageContract("https://api.dflop.top", standard.Expression, billingexpr.TokenParams{ServerToolCalls: &tools}), "UNEXPECTED_DFLOP_SERVER_TOOL_USAGE")
	tools = 0
	require.NoError(t, DFLOPEndpointUsageContract("https://api.dflop.top", standard.Expression, billingexpr.TokenParams{ServerToolCalls: &tools}))
	item.ModelID = "grok-4.7"
	item.BillingFeatures = []string{"token", "server_tool_call"}
	profiles = BuildEndpointBillingProfiles(item)
	require.NotEmpty(t, profiles)
	assert.Equal(t, SupportedAuto, profiles[1].Status)
	require.NoError(t, DFLOPEndpointRequestContract("https://api.dflop.top", profiles[1].Expression, "/v1/responses", billingexpr.RequestInput{Body: []byte(`{"tools":[{"type":"function"}]}`)}))
	require.NoError(t, DFLOPEndpointRequestContract("https://api.dflop.top", profiles[0].Expression, "/v1/chat/completions", billingexpr.RequestInput{Body: []byte(`{"tools":[{"type":"function"}]}`)}))
	require.ErrorContains(t, DFLOPEndpointRequestContract("https://api.dflop.top", profiles[0].Expression, "/v1/chat/completions", billingexpr.RequestInput{Body: []byte(`{"tools":[{"type":"image_generation"}]}`)}), "DFLOP_SERVER_TOOL_UNBOUNDED")
	item.PriceSemantics.SourcePriceKind = "EFFECTIVE_PRICE"
	profiles = BuildEndpointBillingProfiles(item)
	assert.NotEqual(t, SupportedAuto, profiles[0].Status)
	assert.False(t, profiles[0].PriceVerified)
}

func TestEndpointBillingResponseRequiresActualUsageAndServedStandardTier(t *testing.T) {
	expression := `tier("dflop_chat_token_only", p * 2 + c * 10)`
	for _, tc := range []struct {
		body   string
		seen   bool
		reason string
	}{
		{`{"choices":[{"delta":{"content":"a"}}]}`, false, ""},
		{`{"usage":null}`, false, ""},
		{`{"usage":{"prompt_tokens":0,"completion_tokens":0,"num_server_side_tools_used":0}}`, true, ""},
		{`{"usage":{"prompt_tokens":12,"completion_tokens":3}}`, true, ""},
		{`{"usage":{"prompt_tokens":12}}`, false, "MISSING_AUTHORITATIVE_TOKEN_USAGE"},
		{`{"usage":{"prompt_tokens":-1,"completion_tokens":3}}`, false, "INVALID_AUTHORITATIVE_TOKEN_USAGE"},
		{`{"service_tier":"priority","usage":{"prompt_tokens":12,"completion_tokens":3}}`, false, "UNEXPECTED_DFLOP_SELECTED_TIER"},
		{`{"usage":{"prompt_tokens":12,"completion_tokens":3,"num_server_side_tools_used":1}}`, false, "UNEXPECTED_DFLOP_SERVER_TOOL_USAGE"},
		{`{"object":"response","usage":{"input_tokens":1,"output_tokens":2}}`, false, "DFLOP_RESPONSES_PROFILE_UNVERIFIED"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			seen, err := DFLOPEndpointResponseContract("https://api.dflop.top", expression, []byte(tc.body))
			assert.Equal(t, tc.seen, seen)
			if tc.reason == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.reason)
			}
		})
	}
}

func TestDFLOPResponsesProfileUsesAuthoritativeUsage(t *testing.T) {
	expression := `tier("dflop_chat_standard_responses", p * 2 + c * 10 + cr * 0.2)`
	for _, tc := range []struct {
		body, reason string
		seen         bool
	}{
		{`{"object":"response","usage":{"input_tokens":0,"output_tokens":0,"input_tokens_details":{"cached_tokens":0}},"output":[{"type":"image_generation_call"}]}`, "", true},
		{`{"response":{"object":"response","usage":{"input_tokens":12,"output_tokens":5,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}}`, "", true},
		{`{"object":"response","usage":{"input_tokens":0,"input_tokens_details":{"cached_tokens":0}}}`, "MISSING_AUTHORITATIVE_TOKEN_USAGE", false},
		{`{"object":"response","usage":{"input_tokens":12,"output_tokens":5}}`, "MISSING_AUTHORITATIVE_CACHE_USAGE", false},
		{`{"object":"response","usage":{"input_tokens":12,"output_tokens":5,"input_tokens_details":{"cached_tokens":13}}}`, "INVALID_AUTHORITATIVE_CACHE_USAGE", false},
		{`{"response":{"object":"response","service_tier":"priority"}}`, "UNEXPECTED_DFLOP_SELECTED_TIER", false},
		{`{"item":{"type":"web_search_call"}}`, "UNEXPECTED_DFLOP_SERVER_TOOL_USAGE", false},
		{`{"service_tier":"priority","response":{"object":"response","usage":{"input_tokens":0,"output_tokens":0,"input_tokens_details":{"cached_tokens":0}}}}`, "UNEXPECTED_DFLOP_SELECTED_TIER", false},
		{`{"usage":{"prompt_tokens":0,"completion_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}}`, "", true},
		{`{"usage":{"prompt_tokens":12,"completion_tokens":5}}`, "MISSING_AUTHORITATIVE_CACHE_USAGE", false},
		{`{"usage":{"prompt_tokens":12,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":-1}}}`, "INVALID_AUTHORITATIVE_CACHE_USAGE", false},
		{`{"usage":{"prompt_tokens":12,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":13}}}`, "INVALID_AUTHORITATIVE_CACHE_USAGE", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			seen, err := DFLOPEndpointResponseContract("https://api.dflop.top", expression, []byte(tc.body))
			assert.Equal(t, tc.seen, seen)
			if tc.reason == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.reason)
			}
		})
	}
}
