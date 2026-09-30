package dflop

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// CanaryTransport is intentionally not wired into the CLI in Phase 8. Tests
// supply a fake HTTP server; a future phase must review the live invocation path.
type CanaryTransport struct {
	HTTP    *http.Client
	BaseURL string
	Key     string
}

type CanaryBalance struct {
	RemainingUSD string    `json:"remaining_usd"`
	UsedUSD      string    `json:"used_usd"`
	TotalUSD     string    `json:"total_usd"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// FetchCanaryBalance reads only the account-wide, non-billed balance endpoint.
// The result is a preflight check, not evidence of a particular canary charge.
func (t CanaryTransport) FetchCanaryBalance(ctx context.Context) (CanaryBalance, error) {
	body, _, err := t.request(ctx, http.MethodGet, "/v1/key/balance", nil, "")
	if err != nil {
		return CanaryBalance{}, err
	}
	var response struct {
		RemainingUSD json.Number `json:"remaining_usd"`
		UsedUSD      json.Number `json:"used_usd"`
		TotalUSD     json.Number `json:"total_usd"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return CanaryBalance{}, errors.New("CANARY_BALANCE_INVALID")
	}
	for _, raw := range []json.Number{response.RemainingUSD, response.UsedUSD, response.TotalUSD} {
		value, err := decimal.NewFromString(raw.String())
		if err != nil || value.IsNegative() {
			return CanaryBalance{}, errors.New("CANARY_BALANCE_INVALID")
		}
	}
	return CanaryBalance{RemainingUSD: response.RemainingUSD.String(), UsedUSD: response.UsedUSD.String(), TotalUSD: response.TotalUSD.String(), FetchedAt: time.Now().UTC()}, nil
}

type CanaryRequest struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Model       string            `json:"model"`
	Body        string            `json:"body"`
	RequestHash string            `json:"request_hash"`
	PollPath    string            `json:"poll_path"`
	MaxPoints   string            `json:"max_points"`
	ApproxUSD   string            `json:"approx_usd"`
	Idempotency string            `json:"idempotency_key_fingerprint"`
}

func BuildCanaryRequest(c CanaryCase) (CanaryRequest, error) {
	var body []byte
	var err error
	switch c.ID {
	case "tts-async":
		body, err = common.Marshal(struct {
			Model string `json:"model"`
			Input string `json:"input"`
			Async bool   `json:"async"`
		}{c.Model, "Hello.", true})
	case "suno-generation":
		body, err = common.Marshal(struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}{c.Model, "Short calm instrumental melody"})
	default:
		return CanaryRequest{}, errors.New("CANARY_REQUEST_UNVERIFIED")
	}
	if err != nil || c.Status != "READY_FOR_PAID_AUTHORIZATION" {
		return CanaryRequest{}, errors.New("CANARY_REQUEST_UNVERIFIED")
	}
	path := c.Endpoint
	if path != "/v1/audio/speech" && path != "/v1/music/generations" {
		return CanaryRequest{}, errors.New("CANARY_REQUEST_UNVERIFIED")
	}
	hash := sha256.Sum256(append([]byte("POST "+path+"\n"), body...))
	return CanaryRequest{Method: http.MethodPost, URL: path, Headers: map[string]string{"Content-Type": "application/json", "Idempotency-Key": "<invocation-scoped>"}, Model: c.Model, Body: string(body), RequestHash: hex.EncodeToString(hash[:]), PollPath: path + "/{id}", MaxPoints: c.EstimatedMaxPoints, ApproxUSD: c.EstimatedMaxUSD, Idempotency: "generated-per-invocation"}, nil
}

type CanaryExecutionRecord struct {
	InvocationID                 string            `json:"invocation_id"`
	CaseID                       string            `json:"case_id"`
	Model                        string            `json:"model"`
	ChannelID                    int               `json:"channel_id"`
	CredentialFingerprint        string            `json:"credential_fingerprint"`
	CatalogHash                  string            `json:"catalog_hash"`
	ETag                         string            `json:"etag"`
	SchemaVersion                string            `json:"schema_version"`
	BillingFeatures              []string          `json:"billing_features"`
	PriceComponents              map[string]string `json:"price_components_points"`
	PointsPerCNY                 string            `json:"points_per_cny"`
	CNYToUSD                     string            `json:"cny_to_usd"`
	AuthorizedMaxPoints          string            `json:"authorized_max_points"`
	IdempotencyKey               string            `json:"idempotency_key"`
	RequestHash                  string            `json:"request_hash"`
	RequestBody                  string            `json:"request_body"`
	Endpoint                     string            `json:"endpoint"`
	TaskID                       string            `json:"task_id,omitempty"`
	State                        string            `json:"state"`
	SubmitTrace                  string            `json:"submit_trace,omitempty"`
	TerminalTrace                string            `json:"terminal_trace,omitempty"`
	ErrorTrace                   string            `json:"error_trace,omitempty"`
	BalanceBeforeUSD             string            `json:"balance_before_usd,omitempty"`
	BalanceAfterUSD              string            `json:"balance_after_usd,omitempty"`
	CorroboratingBalanceDeltaUSD string            `json:"corroborating_balance_delta_usd,omitempty"`
	BalanceDeltaStatus           string            `json:"balance_delta_status,omitempty"`
	CreatedAt                    time.Time         `json:"created_at"`
}

func NewCanaryInvocationID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func saveCanaryExecutionRecord(path string, record CanaryExecutionRecord) error {
	encoded, err := common.Marshal(record)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(encoded)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (t CanaryTransport) request(ctx context.Context, method, path string, body []byte, idempotency string) ([]byte, http.Header, error) {
	if t.BaseURL == "" || t.Key == "" || (!strings.HasPrefix(path, "/v1/") && path != "/api/v1/config/currency") {
		return nil, nil, errors.New("CANARY_TRANSPORT_UNCONFIGURED")
	}
	client := t.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("CANARY_REDIRECT_BLOCKED") }
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(t.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Key)
	req.Header.Set("User-Agent", "new-api-dflop-pricing-sync/1")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotency)
	}
	response, err := copyClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("CANARY_TRANSPORT_FAILED: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, response.Header, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.Header, fmt.Errorf("CANARY_HTTP_%d", response.StatusCode)
	}
	return data, response.Header, nil
}

// PrepareCanaryExecution performs free preflight GETs and durably records the
// exact body/key before a separate explicit SubmitCanaryExecution call.
func (t CanaryTransport) PrepareCanaryExecution(ctx context.Context, dir string, old CanaryPlan, selected CanaryCase, auth CanaryAuthorization) (string, CanaryExecutionRecord, error) {
	request, err := BuildCanaryRequest(selected)
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	catalog, headers, err := t.request(ctx, http.MethodGet, "/v1/catalog", nil, "")
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	currency, _, err := t.request(ctx, http.MethodGet, "/api/v1/config/currency", nil, "")
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	fresh, err := BuildCanaryPlan(old.ChannelID, catalog, currency, headers.Get("ETag"), old.CNYToUSD)
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	if old.ChannelID != auth.ChannelID || old.CatalogHash != fresh.CatalogHash || old.CatalogHash != auth.CatalogHash || old.SchemaVersion != fresh.SchemaVersion || old.PointsPerCNY != fresh.PointsPerCNY {
		return "", CanaryExecutionRecord{}, errors.New("CANARY_CATALOG_CHANGED")
	}
	var current *CanaryCase
	for i := range fresh.Cases {
		if fresh.Cases[i].ID == selected.ID {
			current = &fresh.Cases[i]
			break
		}
	}
	if current == nil {
		return "", CanaryExecutionRecord{}, errors.New("CANARY_CATALOG_CHANGED")
	}
	if err := CheckCanaryAuthorization(auth, selected, *current); err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	balanceBody, _, err := t.request(ctx, http.MethodGet, "/v1/key/balance", nil, "")
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	var balance struct {
		RemainingUSD json.Number `json:"remaining_usd"`
	}
	if err := common.Unmarshal(balanceBody, &balance); err != nil {
		return "", CanaryExecutionRecord{}, errors.New("CANARY_BALANCE_INVALID")
	}
	remaining, err := decimal.NewFromString(balance.RemainingUSD.String())
	if err != nil || remaining.IsNegative() {
		return "", CanaryExecutionRecord{}, errors.New("CANARY_BALANCE_INVALID")
	}
	maxUSD, _ := decimal.NewFromString(selected.EstimatedMaxUSD)
	if remaining.LessThan(maxUSD) {
		return "", CanaryExecutionRecord{}, errors.New("CANARY_INSUFFICIENT_BALANCE")
	}
	id := auth.InvocationID
	credentialHash := sha256.Sum256([]byte(t.Key))
	record := CanaryExecutionRecord{InvocationID: id, CaseID: selected.ID, Model: selected.Model, ChannelID: old.ChannelID, CredentialFingerprint: hex.EncodeToString(credentialHash[:]), CatalogHash: fresh.CatalogHash, ETag: fresh.ETag, SchemaVersion: fresh.SchemaVersion, BillingFeatures: selected.BillingFeatures, PriceComponents: selected.PriceComponentsPoints, PointsPerCNY: fresh.PointsPerCNY, CNYToUSD: fresh.CNYToUSD, AuthorizedMaxPoints: auth.MaxCostPoints, IdempotencyKey: id, RequestHash: request.RequestHash, RequestBody: request.Body, Endpoint: request.URL, State: "PREPARED", BalanceBeforeUSD: remaining.String(), CreatedAt: time.Now().UTC()}
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "new-api-dflop-canary", "invocations")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return "", CanaryExecutionRecord{}, errors.New("invocation directory must be private")
	}
	path := filepath.Join(dir, id+".json")
	encoded, err := common.Marshal(record)
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", CanaryExecutionRecord{}, err
	}
	_, writeErr := file.Write(encoded)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return "", CanaryExecutionRecord{}, writeErr
	}
	if closeErr != nil {
		return "", CanaryExecutionRecord{}, closeErr
	}
	return path, record, nil
}

// SubmitCanaryExecution is a separate, never-CLI-invoked Phase 8 transport.
// A lost response may be retried only with the same persisted body and key,
// within DFLOP's documented seven-day idempotency retention window.
func (t CanaryTransport) SubmitCanaryExecution(ctx context.Context, path string) (CanaryExecutionRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CanaryExecutionRecord{}, err
	}
	var record CanaryExecutionRecord
	if err := common.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if record.TaskID != "" {
		return record, nil
	}
	credentialHash := sha256.Sum256([]byte(t.Key))
	if hex.EncodeToString(credentialHash[:]) != record.CredentialFingerprint || record.IdempotencyKey != record.InvocationID {
		return record, errors.New("CANARY_CHANNEL_CHANGED")
	}
	if record.State != "PREPARED" || record.IdempotencyKey == "" || time.Since(record.CreatedAt) >= 7*24*time.Hour {
		return record, errors.New("CANARY_RETRY_UNSAFE")
	}
	hash := sha256.Sum256(append([]byte("POST "+record.Endpoint+"\n"), []byte(record.RequestBody)...))
	if hex.EncodeToString(hash[:]) != record.RequestHash {
		return record, errors.New("CANARY_REQUEST_CHANGED")
	}
	catalog, _, err := t.request(ctx, http.MethodGet, "/v1/catalog", nil, "")
	if err != nil {
		return record, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(catalog)) != record.CatalogHash {
		return record, errors.New("CANARY_CATALOG_CHANGED")
	}
	currencyBody, _, err := t.request(ctx, http.MethodGet, "/api/v1/config/currency", nil, "")
	if err != nil {
		return record, err
	}
	var currency Currency
	if common.Unmarshal(currencyBody, &currency) != nil || currency.Unit != "points" {
		return record, errors.New("CANARY_CURRENCY_CHANGED")
	}
	currentPoints, currentErr := decimal.NewFromString(string(currency.PointsPerCNY))
	storedPoints, storedErr := decimal.NewFromString(record.PointsPerCNY)
	if currentErr != nil || storedErr != nil || !currentPoints.Equal(storedPoints) {
		return record, errors.New("CANARY_CURRENCY_CHANGED")
	}
	balanceBody, _, err := t.request(ctx, http.MethodGet, "/v1/key/balance", nil, "")
	if err != nil {
		return record, err
	}
	var balance struct {
		RemainingUSD json.Number `json:"remaining_usd"`
	}
	if common.Unmarshal(balanceBody, &balance) != nil {
		return record, errors.New("CANARY_BALANCE_INVALID")
	}
	remaining, err := decimal.NewFromString(balance.RemainingUSD.String())
	if err != nil || remaining.IsNegative() {
		return record, errors.New("CANARY_BALANCE_INVALID")
	}
	maxPoints, err := decimal.NewFromString(record.AuthorizedMaxPoints)
	pointsPerCNY, pointsErr := decimal.NewFromString(record.PointsPerCNY)
	rate, rateErr := decimal.NewFromString(record.CNYToUSD)
	if err != nil || pointsErr != nil || rateErr != nil || !maxPoints.IsPositive() || !pointsPerCNY.IsPositive() || !rate.IsPositive() {
		return record, errors.New("CANARY_COST_UNBOUNDED")
	}
	if remaining.LessThan(maxPoints.DivRound(pointsPerCNY, 24).Mul(rate)) {
		return record, errors.New("CANARY_INSUFFICIENT_BALANCE")
	}
	response, headers, err := t.request(ctx, http.MethodPost, record.Endpoint, []byte(record.RequestBody), record.IdempotencyKey)
	if err != nil {
		record.ErrorTrace = headers.Get("x-gateway-trace")
		if saveErr := saveCanaryExecutionRecord(path, record); saveErr != nil {
			return record, saveErr
		}
		return record, err
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := common.Unmarshal(response, &task); err != nil || task.ID == "" || strings.ContainsAny(task.ID, "/?\\") {
		return record, errors.New("CANARY_TASK_ID_MISSING")
	}
	record.TaskID, record.State, record.SubmitTrace = task.ID, "SUBMITTED", headers.Get("x-gateway-trace")
	if err := saveCanaryExecutionRecord(path, record); err != nil {
		return record, err
	}
	return record, nil
}

func (t CanaryTransport) PollCanaryExecution(ctx context.Context, path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var record CanaryExecutionRecord
	if err := common.Unmarshal(data, &record); err != nil {
		return nil, "", err
	}
	if record.TaskID == "" {
		return nil, "", errors.New("CANARY_TASK_ID_MISSING")
	}
	body, headers, err := t.request(ctx, http.MethodGet, record.Endpoint+"/"+record.TaskID, nil, "")
	trace := headers.Get("x-gateway-trace")
	if err != nil {
		record.ErrorTrace = trace
		if saveErr := saveCanaryExecutionRecord(path, record); saveErr != nil {
			return nil, "", saveErr
		}
		return nil, trace, err
	}
	var result struct {
		Status string `json:"status"`
		Model  string `json:"model"`
	}
	if common.Unmarshal(body, &result) != nil {
		return nil, trace, errors.New("CANARY_TERMINAL_INVALID")
	}
	switch result.Status {
	case "succeeded", "failed", "expired", "cancelled", "ready":
		verification, verifyErr := VerifyCanaryFixture(record.CaseID, body)
		if verifyErr != nil {
			return nil, trace, verifyErr
		}
		if result.Model != record.Model {
			verification.Status = "CANARY_MODEL_MISMATCH"
		}
		record.State, record.TerminalTrace = "TERMINAL", trace
		if balanceBody, _, balanceErr := t.request(ctx, http.MethodGet, "/v1/key/balance", nil, ""); balanceErr == nil {
			var balance struct {
				RemainingUSD json.Number `json:"remaining_usd"`
			}
			if common.Unmarshal(balanceBody, &balance) == nil {
				record.BalanceAfterUSD = balance.RemainingUSD.String()
				before, beforeErr := decimal.NewFromString(record.BalanceBeforeUSD)
				after, afterErr := decimal.NewFromString(record.BalanceAfterUSD)
				if beforeErr == nil && afterErr == nil {
					record.CorroboratingBalanceDeltaUSD = before.Sub(after).String()
					record.BalanceDeltaStatus = "BALANCE_DELTA_NON_ISOLATED"
				}
			}
		}
		if err := saveCanaryExecutionRecord(path, record); err != nil {
			return nil, trace, err
		}
		capture := CanaryCapture{CaseID: record.CaseID, ChannelID: record.ChannelID, Model: record.Model, Endpoint: record.Endpoint, CatalogHash: record.CatalogHash, SchemaVersion: record.SchemaVersion, ETag: record.ETag, BillingFeatures: record.BillingFeatures, TerminalStatus: result.Status, VerificationResult: verification.Status, SubmitTrace: record.SubmitTrace, TerminalTrace: trace, ErrorTrace: record.ErrorTrace, Response: body, FinishedAt: time.Now().UTC()}
		_, err := writeCanaryCapture(filepath.Join(filepath.Dir(path), "captures"), record.InvocationID, capture, t.Key, true)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return nil, trace, err
		}
		return body, trace, nil
	default:
		return body, trace, nil
	}
}
