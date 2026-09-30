package dflop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// HistoricalTransport has no submission operation. Read rejects other methods
// and paths before constructing a request; redirects never carry credentials.
type HistoricalTransport struct {
	HTTP    *http.Client
	BaseURL string
	Key     string
}

func (t HistoricalTransport) Read(ctx context.Context, method, path string, query url.Values) ([]byte, int, error) {
	if method != http.MethodGet {
		return nil, 0, errors.New("EVIDENCE_GET_ONLY")
	}
	allowed := false
	for _, base := range []string{"/v1/logs", "/v1/images/generations", "/v1/videos/generations", "/v1/music/generations", "/v1/audio/speech", "/v1/audio/voices"} {
		if path == base {
			allowed = true
			break
		}
		if id, ok := strings.CutPrefix(path, base+"/"); ok && id != "" && !strings.ContainsAny(id, "/\\.%?#") {
			allowed = true
			for _, ch := range id {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
					allowed = false
				}
			}
			break
		}
	}
	if !allowed {
		return nil, 0, errors.New("EVIDENCE_PATH_NOT_ALLOWED")
	}
	base, err := url.Parse(strings.TrimSuffix(t.BaseURL, "/"))
	if err != nil || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Path != "" || base.Host == "" || (base.Scheme != "https" && !(t.HTTP != nil && base.Scheme == "http")) {
		return nil, 0, errors.New("EVIDENCE_SOURCE_INVALID")
	}
	base.Path, base.RawQuery = path, query.Encode()
	client := http.Client{Timeout: 20 * time.Second}
	if t.HTTP != nil {
		client = *t.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for attempt := range 3 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
		if err != nil {
			return nil, 0, errors.New("EVIDENCE_REQUEST_INVALID")
		}
		req.Header.Set("Authorization", "Bearer "+t.Key)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "new-api-dflop-evidence-recover/1")
		res, err := client.Do(req)
		if err != nil {
			return nil, 0, errors.New("EVIDENCE_GET_FAILED")
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 16*1024*1024+1))
		_ = res.Body.Close()
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			seconds, parseErr := strconv.Atoi(res.Header.Get("Retry-After"))
			if parseErr != nil || seconds < 1 || seconds > 60 {
				return nil, res.StatusCode, errors.New("EVIDENCE_RATE_LIMITED")
			}
			timer := time.NewTimer(time.Duration(seconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, res.StatusCode, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if res.StatusCode != http.StatusOK {
			return nil, res.StatusCode, fmt.Errorf("EVIDENCE_HTTP_%d", res.StatusCode)
		}
		if readErr != nil || len(body) > 16*1024*1024 {
			return nil, res.StatusCode, errors.New("EVIDENCE_BODY_INVALID")
		}
		if !strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "application/json") {
			return nil, res.StatusCode, errors.New("EVIDENCE_NOT_JSON")
		}
		return body, res.StatusCode, nil
	}
	return nil, 0, errors.New("EVIDENCE_GET_FAILED")
}

type HistoricalOptions struct {
	ChannelID   int                                                                    `json:"channel_id"`
	Scope       string                                                                 `json:"scope"`
	Period      string                                                                 `json:"period"`
	From        string                                                                 `json:"from,omitempty"`
	To          string                                                                 `json:"to,omitempty"`
	Model       string                                                                 `json:"model_filter,omitempty"`
	CatalogHash string                                                                 `json:"catalog_hash"`
	Replay      func(context.Context, Item, json.RawMessage) (HistoricalReplay, error) `json:"-"`
}

type HistoricalReplay struct {
	Parser             string `json:"parser,omitempty"`
	QuantityVerified   bool   `json:"quantity_verified"`
	SettlementReplayed bool   `json:"settlement_replayed"`
	BindingVerified    bool   `json:"binding_verified"`
	SettlementVerified bool   `json:"settlement_verified"`
	Result             any    `json:"result,omitempty"`
	Note               string `json:"note,omitempty"`
}

type HistoricalRequest struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	HTTPStatus int    `json:"http_status"`
	Pages      int    `json:"pages"`
	Records    int    `json:"records"`
	Complete   bool   `json:"complete"`
	Error      string `json:"error,omitempty"`
}

type HistoricalCapture struct {
	EvidenceID       string            `json:"evidence_id"`
	EvidenceClass    string            `json:"evidence_class"`
	SourcePath       string            `json:"source_path"`
	Model            string            `json:"model"`
	CapturedAt       time.Time         `json:"captured_at"`
	CatalogHash      string            `json:"catalog_hash"`
	CaptureHash      string            `json:"capture_hash"`
	RedactionVersion string            `json:"redaction_version"`
	GatewayTrace     string            `json:"x-gateway-trace,omitempty"`
	Response         json.RawMessage   `json:"response"`
	Replay           *HistoricalReplay `json:"production_replay,omitempty"`
}

type HistoricalComparison struct {
	EvidenceID     string `json:"evidence_id"`
	ExpectedPoints string `json:"expected_points"`
	SettledPoints  string `json:"settled_points"`
	Result         string `json:"result"`
	Basis          string `json:"basis"`
}

type HistoricalModelReport struct {
	CatalogPrices        map[string]Price       `json:"current_catalog_prices"`
	BillingFeatures      []string               `json:"billing_features"`
	Reconciliations      []HistoricalComparison `json:"reconciliations"`
	Model                string                 `json:"model"`
	Family               string                 `json:"family"`
	ExistingReason       string                 `json:"existing_reason"`
	HistoricalCallsFound int                    `json:"historical_calls_found"`
	SuccessfulCallsFound int                    `json:"successful_calls_found"`
	EvidenceIDs          []string               `json:"evidence_ids"`
	RuntimeFields        map[string]any         `json:"runtime_fields"`
	SettledCostAvailable bool                   `json:"settled_cost_available"`
	PriceVerified        bool                   `json:"price_verified"`
	SemanticsVerified    bool                   `json:"semantics_verified"`
	BindingVerified      bool                   `json:"binding_verified"`
	RuntimeUsageVerified bool                   `json:"runtime_usage_verified"`
	QuantityVerified     bool                   `json:"quantity_verified"`
	SettlementReplayed   bool                   `json:"settlement_replayed"`
	SettlementVerified   bool                   `json:"settlement_verified"`
	ReconciliationResult string                 `json:"reconciliation_result"`
	ProposedReason       string                 `json:"proposed_reason"`
	CanUnlock            bool                   `json:"can_unlock"`
	Notes                []string               `json:"notes"`
}

type ProviderEscalation struct {
	Issue            string   `json:"issue"`
	RequiredContract string   `json:"required_dflop_field_or_contract"`
	Models           []string `json:"models"`
}

type HistoricalReport struct {
	SourceSnapshot         map[string]any          `json:"fresh_source_snapshot"`
	EndpointMatrix         []map[string]any        `json:"gpt_endpoint_billing_matrix"`
	LocalCorrelationStatus string                  `json:"local_correlation_status"`
	CoverageBefore         map[string]int          `json:"coverage_before"`
	CoverageAfter          map[string]int          `json:"coverage_after"`
	Mode                   string                  `json:"mode"`
	Options                HistoricalOptions       `json:"options"`
	CapturedAt             time.Time               `json:"captured_at"`
	PaidRequestsExecuted   bool                    `json:"paid_requests_executed"`
	StatusFilter           string                  `json:"status_filter"`
	Requests               []HistoricalRequest     `json:"read_only_requests"`
	Models                 []HistoricalModelReport `json:"models"`
	Captures               []HistoricalCapture     `json:"captures"`
	Escalations            []ProviderEscalation    `json:"provider_escalations"`
}

// ReconcileHistoricalPoints permits rounding only at an explicitly established
// provider precision. Recovery uses -1 (exact), because a short cost string
// alone does not establish the provider's rounding contract.
func ReconcileHistoricalPoints(expected, settled string, decimalPlaces int32) string {
	want, err := decimal.NewFromString(expected)
	got, gotErr := decimal.NewFromString(settled)
	if err != nil || gotErr != nil || want.IsNegative() || got.IsNegative() {
		return "INSUFFICIENT_EVIDENCE"
	}
	if want.Equal(got) {
		return "MATCH"
	}
	if decimalPlaces >= 0 && decimalPlaces <= 24 && want.Round(decimalPlaces).Equal(got) {
		return "ROUNDING_MATCH"
	}
	return "MISMATCH"
}

func RecoverHistoricalEvidence(ctx context.Context, transport HistoricalTransport, options HistoricalOptions, items []Item) (HistoricalReport, error) {
	if options.Scope == "" {
		options.Scope = "key"
	}
	if options.Period == "" {
		options.Period = "30d"
	}
	if options.Scope != "key" && options.Scope != "account" {
		return HistoricalReport{}, errors.New("scope must be key or account")
	}
	if !slices.Contains([]string{"7d", "30d", "this_month", "all", "today"}, options.Period) || (options.From == "") != (options.To == "") {
		return HistoricalReport{}, errors.New("invalid evidence time window")
	}
	for _, value := range []string{options.From, options.To} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return HistoricalReport{}, errors.New("from/to must be dates or RFC3339 timestamps")
			}
		}
	}
	report := HistoricalReport{Mode: "EVIDENCE_RECOVER", Options: options, CapturedAt: time.Now().UTC(), StatusFilter: "logs=success in selected time window; task lists=succeeded across provider-retained history (may extend beyond log window); voice list includes pending/ready/failed; media list visibility is provider-controlled"}
	byModel := make(map[string]int, len(items))
	itemByModel := make(map[string]Item, len(items))
	for _, item := range items {
		if !item.Callable || options.Model != "" && item.ModelID != options.Model {
			continue
		}
		reason := item.ReasonCode
		if reason == "NO_PLUGIN_USAGE_PROFILE" {
			reason = missingTaskBindingReason(item)
		}
		byModel[item.ModelID] = len(report.Models)
		itemByModel[item.ModelID] = item
		family := item.Category
		switch {
		case strings.Contains(item.ModelID, "midjourney"):
			family = "MIDJOURNEY"
		case item.ModelID == "voice-tts-pro":
			family = "TTS"
		case strings.HasPrefix(item.ModelID, "suno-"):
			family = "SUNO"
		case item.ModelID == "voice-clone-pro":
			family = "VOICE_CLONE"
		case strings.Contains(item.ModelID, "seedance"):
			family = "SEEDANCE"
		case strings.HasPrefix(item.ModelID, "gpt-"):
			family = "GPT"
		case strings.Contains(item.ModelID, "grok"):
			family = "GROK"
		case strings.Contains(item.ModelID, "claude"):
			family = "CLAUDE"
		}
		report.Models = append(report.Models, HistoricalModelReport{CatalogPrices: item.Prices, BillingFeatures: item.BillingFeatures, Model: item.ModelID, Family: family, ExistingReason: reason, ProposedReason: reason, EvidenceIDs: []string{}, RuntimeFields: map[string]any{}, ReconciliationResult: "INSUFFICIENT_EVIDENCE", Notes: []string{}, PriceVerified: len(item.Prices) > 0, SemanticsVerified: strings.Contains(item.ModelID, "claude") || strings.Contains(item.ModelID, "seedance") || item.ModelID == "voice-tts-pro" || strings.HasPrefix(item.ModelID, "suno-") || strings.Contains(item.ModelID, "midjourney") || strings.HasPrefix(item.ModelID, "dh-") || item.ModelID == "clip-compose"})
	}
	logsQuery := url.Values{"scope": {options.Scope}, "period": {options.Period}, "status": {"success"}, "include": {"attempts"}, "limit": {"200"}, "order": {"desc"}}
	if options.From != "" {
		logsQuery.Set("from", options.From)
		logsQuery.Set("to", options.To)
	}
	if options.Model != "" {
		logsQuery.Set("model", options.Model)
	}
	logs, request := readHistoricalList(ctx, transport, "/v1/logs", logsQuery, true, options.Scope)
	report.Requests = append(report.Requests, request)
	// Logical calls are joined by provider task ID. A list record and its ledger
	// are two pieces of evidence for one call, not two successful generations.
	seenCalls := map[string]bool{}
	ledgerByTask := map[string]map[string]any{}
	taskModels := map[string]string{}
	taskPaths := map[string]string{}
	for _, row := range logs {
		modelName, _ := row["model"].(string)
		index, known := byModel[modelName]
		if !known {
			continue
		}
		id := historicalID(row["id"])
		if id == "" {
			continue
		}
		modelReport := &report.Models[index]
		taskID, _ := row["task_id"].(string)
		callKey := "log:" + id
		if taskID != "" {
			callKey = "task:" + taskID
			ledgerByTask[taskID] = row
		}
		if !seenCalls[callKey] {
			modelReport.HistoricalCallsFound++
			if row["status"] == "success" {
				modelReport.SuccessfulCallsFound++
			}
			seenCalls[callKey] = true
		}
		if err := captureHistoricalRecord(&report, modelReport, transport.Key, "log:"+id, "CAPTURED_REAL_DFLOP_LEDGER", "/v1/logs", row, nil); err != nil {
			return report, err
		}
		ObserveHistoricalBilling(modelReport, itemByModel[modelName], row, nil)
		if modelReport.ExistingReason == "" {
			continue
		}
		detailPath := "/v1/logs/" + id
		body, status, err := transport.Read(ctx, http.MethodGet, detailPath, nil)
		detailRequest := HistoricalRequest{Method: http.MethodGet, Path: detailPath, HTTPStatus: status, Pages: 1, Complete: err == nil}
		if err != nil {
			detailRequest.Error = err.Error()
		} else {
			var detail map[string]any
			if common.Unmarshal(body, &detail) == nil {
				detailRequest.Records = 1
				if detail["request_id"] == nil {
					detail["request_id"] = row["request_id"]
				}
				var replay *HistoricalReplay
				if options.Replay != nil {
					result, replayErr := options.Replay(ctx, itemByModel[modelName], body)
					if replayErr == nil {
						replay = &result
						modelReport.SettlementReplayed = modelReport.SettlementReplayed || result.SettlementReplayed
						modelReport.QuantityVerified = modelReport.QuantityVerified || result.QuantityVerified
					}
				}
				if err := captureHistoricalRecord(&report, modelReport, transport.Key, "technical:"+id, "CAPTURED_REAL_DFLOP_RESPONSE", detailPath, detail, replay); err != nil {
					return report, err
				}
			}
		}
		report.Requests = append(report.Requests, detailRequest)
		if taskID != "" {
			if path := historicalTaskPath(row); path != "" {
				taskModels[taskID] = modelName
				taskPaths[taskID] = path
			}
		}
	}
	for _, path := range []string{"/v1/images/generations", "/v1/videos/generations", "/v1/music/generations", "/v1/audio/speech", "/v1/audio/voices"} {
		query := url.Values{"limit": {"100"}}
		if path != "/v1/audio/voices" {
			query.Set("status", "succeeded")
		}
		if path == "/v1/images/generations" && options.From != "" {
			query.Set("from", options.From)
			query.Set("to", options.To)
		}
		rows, request := readHistoricalList(ctx, transport, path, query, false, options.Scope)
		report.Requests = append(report.Requests, request)
		for _, row := range rows {
			modelName, _ := row["model"].(string)
			if path == "/v1/audio/voices" && modelName == "" {
				if ledger := ledgerByTask[historicalID(row["id"])]; ledger != nil {
					modelName, _ = ledger["model"].(string)
				}
			}
			index, known := byModel[modelName]
			if !known {
				if path == "/v1/audio/voices" {
					orphan := HistoricalModelReport{Model: "UNKNOWN_MODEL", RuntimeFields: map[string]any{}}
					if err := captureHistoricalRecord(&report, &orphan, transport.Key, "unassigned-voice:"+historicalID(row["id"]), "CAPTURED_REAL_DFLOP_TASK", path, row, nil); err != nil {
						return report, err
					}
				}
				continue
			}
			id := historicalID(row["id"])
			if id == "" {
				continue
			}
			modelReport := &report.Models[index]
			if !seenCalls["task:"+id] {
				modelReport.HistoricalCallsFound++
				if row["status"] == "succeeded" || row["status"] == "ready" {
					modelReport.SuccessfulCallsFound++
				}
				seenCalls["task:"+id] = true
			}
			taskModels[id] = modelName
			taskPaths[id] = path
			if err := captureHistoricalRecord(&report, modelReport, transport.Key, "task-list:"+id, "CAPTURED_REAL_DFLOP_TASK", path, row, nil); err != nil {
				return report, err
			}
			ObserveHistoricalBilling(modelReport, itemByModel[modelName], row, ledgerByTask[id])
		}
	}
	ids := make([]string, 0, len(taskPaths))
	for id := range taskPaths {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		modelName := taskModels[id]
		modelReport := &report.Models[byModel[modelName]]
		path := taskPaths[id] + "/" + id
		body, status, err := transport.Read(ctx, http.MethodGet, path, nil)
		request := HistoricalRequest{Method: http.MethodGet, Path: path, HTTPStatus: status, Pages: 1, Complete: err == nil}
		if err != nil {
			request.Error = err.Error()
			report.Requests = append(report.Requests, request)
			continue
		}
		var row map[string]any
		if err := common.Unmarshal(body, &row); err != nil {
			request.Error = "EVIDENCE_JSON_INVALID"
			report.Requests = append(report.Requests, request)
			continue
		}
		request.Records = 1
		report.Requests = append(report.Requests, request)
		if returnedModel, ok := row["model"].(string); ok && returnedModel != "" && returnedModel != modelName {
			modelReport.Notes = append(modelReport.Notes, "TERMINAL_MODEL_MISMATCH")
			continue
		}
		var replay *HistoricalReplay
		if options.Replay != nil {
			result, replayErr := options.Replay(ctx, itemByModel[modelName], body)
			if replayErr != nil {
				result.Note = "production replay unavailable: " + replayErr.Error()
			}
			replay = &result
			modelReport.QuantityVerified = modelReport.QuantityVerified || result.QuantityVerified
			modelReport.SettlementReplayed = modelReport.SettlementReplayed || result.SettlementReplayed
			modelReport.BindingVerified = modelReport.BindingVerified || result.BindingVerified
			modelReport.SettlementVerified = modelReport.SettlementVerified || result.SettlementVerified
		}
		if err := captureHistoricalRecord(&report, modelReport, transport.Key, "task-poll:"+id, "CAPTURED_REAL_DFLOP_TASK", path, row, replay); err != nil {
			return report, err
		}
		ObserveHistoricalBilling(modelReport, itemByModel[modelName], row, ledgerByTask[id])
	}
	for i := range report.Models {
		row := &report.Models[i]
		if row.HistoricalCallsFound == 0 {
			row.Notes = append(row.Notes, "NO_HISTORICAL_EVIDENCE")
		}
		if row.HistoricalCallsFound > 0 {
			row.Notes = append(row.Notes, "Current authenticated catalog is a comparison snapshot; the historical call's price snapshot is not established")
		}
		row.CanUnlock = row.PriceVerified && row.SemanticsVerified && row.BindingVerified && row.QuantityVerified && row.SettlementVerified && (row.ReconciliationResult == "MATCH" || row.ReconciliationResult == "ROUNDING_MATCH")
		if row.CanUnlock {
			row.ProposedReason = SupportedAuto
		}
		if row.ExistingReason == "DFLOP_CACHE_CONTRACT_RUNTIME_UNVERIFIED" || row.ExistingReason == "MISSING_CACHE_WRITE_1H_PRICE" {
			report.Escalations = append(report.Escalations, ProviderEscalation{Issue: "Claude native cache TTL acceptance", RequiredContract: "Clarify whether cache_control ttl=1h is accepted on native /v1/messages; if accepted, expose its effective price in /v1/catalog", Models: []string{row.Model}})
		}
		if slices.Contains(row.Notes, "INPUT_VIDEO_MODE_UNVERIFIED") {
			report.Escalations = append(report.Escalations, ProviderEscalation{Issue: "Seedance effective tariff selection", RequiredContract: "Expose authoritative input-video mode/quantity and selected token tier in historical terminal usage/logs; provide trace IDs to correlate the exact New API execution", Models: []string{row.Model}})
		}
		if row.ExistingReason == "MISSING_FAST_MODE_MAPPING" {
			report.Escalations = append(report.Escalations, ProviderEscalation{Issue: "GPT fast-mode contract", RequiredContract: "Clarify fast_mode/per_image applicability by endpoint/protocol; expose authoritative selected tier and quantity in terminal usage/logs", Models: []string{row.Model}})
		}
		if strings.Contains(row.ExistingReason, "UNKNOWN") || row.ExistingReason == "UNSUPPORTED_PRICE_SHAPE" {
			report.Escalations = append(report.Escalations, ProviderEscalation{Issue: "Unmapped billing feature/component", RequiredContract: "Machine-readable component unit, selector, effective price and authoritative terminal quantity", Models: []string{row.Model}})
		}
	}
	return report, nil
}

func readHistoricalList(ctx context.Context, transport HistoricalTransport, path string, query url.Values, logs bool, scope string) ([]map[string]any, HistoricalRequest) {
	request := HistoricalRequest{Method: http.MethodGet, Path: path}
	rows := []map[string]any{}
	seenCursors := map[string]bool{}
	for {
		body, status, err := transport.Read(ctx, http.MethodGet, path, query)
		request.Pages++
		request.HTTPStatus = status
		if err != nil {
			request.Error = err.Error()
			if logs && scope == "account" && status == http.StatusForbidden {
				request.Error = "ACCOUNT_SCOPE_PERMISSION_REQUIRED"
			}
			break
		}
		var page struct {
			Currency   string           `json:"currency"`
			Scope      string           `json:"scope"`
			Data       []map[string]any `json:"data"`
			Voices     []map[string]any `json:"voices"`
			HasMore    *bool            `json:"has_more"`
			NextCursor *string          `json:"next_cursor"`
		}
		if err := common.Unmarshal(body, &page); err != nil {
			request.Error = "EVIDENCE_JSON_INVALID"
			break
		}
		if logs && (page.Currency != "points" || page.Scope != scope) {
			request.Error = "EVIDENCE_LEDGER_SCOPE_OR_CURRENCY_INVALID"
			break
		}
		if path == "/v1/audio/voices" {
			page.Data = page.Voices
		}
		rows = append(rows, page.Data...)
		request.Records = len(rows)
		cursor := ""
		if page.NextCursor != nil {
			cursor = *page.NextCursor
		}
		if cursor == "" {
			if logs && page.HasMore == nil {
				request.Error = "EVIDENCE_PAGINATION_CONTRACT_MISSING"
				break
			}
			if page.HasMore != nil && *page.HasMore {
				request.Error = "EVIDENCE_CURSOR_MISSING"
				break
			}
			if !logs && page.HasMore == nil && len(page.Data) >= 100 {
				request.Error = "HISTORY_WINDOW_TRUNCATION_UNVERIFIED"
				break
			}
			request.Complete = true
			break
		}
		if seenCursors[cursor] {
			request.Error = "EVIDENCE_CURSOR_REPEATED"
			break
		}
		seenCursors[cursor] = true
		query.Set("cursor", cursor)
	}
	return rows, request
}

func historicalID(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if v > 0 && v <= 9007199254740991 && v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

func historicalTaskPath(row map[string]any) string {
	switch row["unit_type"] {
	case "image":
		return "/v1/images/generations"
	case "video", "avatar":
		return "/v1/videos/generations"
	case "music":
		return "/v1/music/generations"
	case "audio":
		return "/v1/audio/speech"
	case "voice":
		return "/v1/audio/voices"
	}
	return ""
}

func captureHistoricalRecord(report *HistoricalReport, modelReport *HistoricalModelReport, key, id, class, path string, row map[string]any, replay *HistoricalReplay) error {
	encoded, err := common.Marshal(row)
	if err != nil {
		return err
	}
	redacted, err := RedactCanaryJSON(encoded, key)
	if err != nil {
		return err
	}
	trace, _ := row["request_id"].(string)
	report.Captures = append(report.Captures, HistoricalCapture{EvidenceID: id, EvidenceClass: class, SourcePath: path, Model: modelReport.Model, CapturedAt: time.Now().UTC(), CatalogHash: report.Options.CatalogHash, CaptureHash: fmt.Sprintf("%x", sha256.Sum256(redacted)), RedactionVersion: "2", GatewayTrace: trace, Response: redacted, Replay: replay})
	modelReport.EvidenceIDs = append(modelReport.EvidenceIDs, id)
	var safe map[string]any
	if err := common.Unmarshal(redacted, &safe); err != nil {
		return err
	}
	facts := map[string]any{}
	for _, field := range []string{"model", "kind", "status", "unit_type", "unit_count", "cost", "task_id", "request_id", "submitted_at", "characters", "duration_sec", "resolution", "service_tier", "usage", "input_video_duration_sec", "width", "height", "attempts_count", "request", "upstream", "gateway"} {
		if value, present := safe[field]; present {
			facts[field] = value
		}
	}
	modelReport.RuntimeFields[id] = facts
	return nil
}

func ObserveHistoricalBilling(report *HistoricalModelReport, item Item, row, ledger map[string]any) {
	status, _ := row["status"].(string)
	if status != "success" && status != "succeeded" && status != "ready" {
		return
	}
	cost, _ := row["cost"].(string)
	if cost == "" && ledger != nil {
		cost, _ = ledger["cost"].(string)
	}
	if cost != "" {
		report.SettledCostAvailable = true
	}
	// Token tier probes use reported completion tokens, never an estimated
	// width/height/FPS formula. Composite contracts require both terminal legs.
	if slices.Contains(item.BillingFeatures, "video_token") {
		usage, _ := row["usage"].(map[string]any)
		resolution, _ := row["resolution"].(string)
		tier := ""
		if rawDuration, ok := row["input_video_duration_sec"]; ok {
			encoded, _ := common.Marshal(rawDuration)
			duration, err := decimal.NewFromString(strings.Trim(string(encoded), "\""))
			if err == nil && !duration.IsNegative() {
				tier = "default"
				if duration.IsPositive() {
					tier = "with_video_input"
				}
			}
		}
		price, found := item.Prices["video_token_tier:"+tier+"@"+resolution]
		if !found && slices.Contains(item.BillingFeatures, "video_token_formula_seedance_2_5") && (resolution == "480p" || resolution == "720p") {
			price, found = item.Prices["video_token_tier:"+tier]
		}

		tokensRaw, _ := common.Marshal(usage["completion_tokens"])
		tokens, tokensErr := decimal.NewFromString(string(tokensRaw))
		if tokensErr == nil && !tokens.IsNegative() && tokens.Equal(tokens.Truncate(0)) {
			report.QuantityVerified = true
		}
		if slices.Contains(item.BillingFeatures, "video_second") && !slices.Contains(item.BillingFeatures, "video_two_stage") {
			report.SemanticsVerified = true
			// The documented standard contract is token-only. Input mode is a
			// separate selector; upstream service_tier is not evidence of that mode.
			if row["input_video_duration_sec"] == nil {
				if !slices.Contains(report.Notes, "INPUT_VIDEO_MODE_UNVERIFIED") {
					report.Notes = append(report.Notes, "INPUT_VIDEO_MODE_UNVERIFIED")
				}
				return
			}
		}

		unit, unitErr := decimal.NewFromString(price.Credits)
		if !found || tier == "" || resolution == "" || tokensErr != nil || unitErr != nil || tokens.IsNegative() || !tokens.Equal(tokens.Truncate(0)) {
			return
		}
		if slices.Contains(item.BillingFeatures, "video_input_seconds") && row["input_video_duration_sec"] == nil {
			return
		}
		expected := tokens.Mul(unit).DivRound(decimal.NewFromInt(1000000), 24)
		if slices.Contains(item.BillingFeatures, "video_two_stage") {
			secondsRaw, _ := common.Marshal(row["duration_sec"])
			seconds, secondsErr := decimal.NewFromString(strings.Trim(string(secondsRaw), "\""))
			secondPrice, found := item.Prices["video_second_stage:"+resolution]
			perSecond, secondErr := decimal.NewFromString(secondPrice.Credits)
			if !found || secondsErr != nil || secondErr != nil || !seconds.IsPositive() {
				return
			}
			expected = expected.Add(seconds.Mul(perSecond))
		}
		report.QuantityVerified = true
		recordHistoricalComparison(report, expected.String(), cost, "current catalog token tier + all required delivered-second legs; historical effective price unverified")
		return
	}
	var quantity any
	var price Price
	compatible := false
	switch {
	case slices.Equal(item.BillingFeatures, []string{"per_image"}):
		quantity = row["unit_count"]
		price, compatible = item.Prices["price_per_image"]
	case slices.Equal(item.BillingFeatures, []string{"tts_char"}):
		quantity = row["characters"]
		price, compatible = item.Prices["price_per_tts_char"]
	case slices.Equal(item.BillingFeatures, []string{"music"}):
		quantity = row["unit_count"]
		price, compatible = item.Prices["price_per_music_generation"]
	case slices.Equal(item.BillingFeatures, []string{"voice_clone"}):
		quantity = row["unit_count"]
		price, compatible = item.Prices["price_per_voice_clone"]
	case slices.Equal(item.BillingFeatures, []string{"video_second"}):
		quantity = row["duration_sec"]
		price, compatible = item.Prices["price_per_video_second"]
	}
	if quantity == nil && ledger != nil && !slices.Contains(item.BillingFeatures, "tts_char") {
		quantity = ledger["unit_count"]
	}
	encoded, err := common.Marshal(quantity)
	if err != nil || quantity == nil || !compatible {
		return
	}
	count, countErr := decimal.NewFromString(strings.Trim(string(encoded), "\""))
	unit, unitErr := decimal.NewFromString(price.Credits)
	if countErr != nil || unitErr != nil || !count.IsPositive() || unit.IsNegative() {
		return
	}
	if slices.Contains(item.BillingFeatures, "per_image") && (!count.Equal(count.Truncate(0)) || count.GreaterThan(decimal.NewFromInt(128))) {
		return
	}
	if (slices.Contains(item.BillingFeatures, "music") || slices.Contains(item.BillingFeatures, "voice_clone")) && !count.Equal(decimal.NewFromInt(1)) {
		return
	}
	report.QuantityVerified = true
	report.SemanticsVerified = true
	recordHistoricalComparison(report, unit.Mul(count).String(), cost, "current catalog unit price × authoritative terminal/ledger quantity; historical effective price unverified")
}

func recordHistoricalComparison(report *HistoricalModelReport, expected, cost, basis string) {
	result := ReconcileHistoricalPoints(expected, cost, -1)
	id := ""
	if len(report.EvidenceIDs) > 0 {
		id = report.EvidenceIDs[len(report.EvidenceIDs)-1]
	}
	report.Reconciliations = append(report.Reconciliations, HistoricalComparison{EvidenceID: id, ExpectedPoints: expected, SettledPoints: cost, Result: result, Basis: basis})
	if report.ReconciliationResult == "MISMATCH" {
		return
	}
	if result != "INSUFFICIENT_EVIDENCE" {
		report.ReconciliationResult = result
	}
}
