package model

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrRuntimeVerificationConflict = errors.New("runtime verification state conflict")
	ErrRuntimeVerificationBudget   = errors.New("runtime verification budget or request limit exceeded")
	ErrRuntimeVerificationEvidence = errors.New("unsafe or incomplete runtime verification evidence")
)

// Costs are exact provider points, stored as portable decimal strings.
// PaidRequests counts durable submit intents and never decreases after release.
type RuntimeVerificationRun struct {
	ID                        int64   `json:"id" gorm:"primaryKey"`
	ChannelID                 int     `json:"channel_id" gorm:"index"`
	FundingUserID             int     `json:"funding_user_id"`
	Source                    string  `json:"source" gorm:"type:varchar(32)"`
	CatalogHash               string  `json:"catalog_hash" gorm:"type:varchar(64)"`
	CredentialFingerprint     string  `json:"credential_fingerprint" gorm:"type:varchar(64)"`
	StartedAt                 int64   `json:"started_at" gorm:"index"`
	CompletedAt               int64   `json:"completed_at"`
	Mode                      string  `json:"mode" gorm:"type:varchar(32)"`
	AuthorizationManifestHash string  `json:"authorization_manifest_hash" gorm:"type:varchar(64)"`
	AuthorizationIdentityHash *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	TotalBudgetLimit          string  `json:"total_budget_limit" gorm:"type:varchar(128)"`
	ActualProviderCost        string  `json:"actual_provider_cost" gorm:"type:varchar(128)"`
	ReservedProviderCost      string  `json:"reserved_provider_cost" gorm:"type:varchar(128)"`
	PaidRequests              int     `json:"paid_requests"`
	MaxRequests               int     `json:"max_requests"`
	Status                    string  `json:"status" gorm:"type:varchar(64)"`
}

type RuntimeVerificationItem struct {
	ID                    int64                    `json:"id" gorm:"primaryKey"`
	RunID                 int64                    `json:"run_id" gorm:"index;uniqueIndex:runtime_verification_identity,priority:1"`
	Model                 string                   `json:"model" gorm:"type:varchar(128);uniqueIndex:runtime_verification_identity,priority:2"`
	Protocol              string                   `json:"protocol" gorm:"type:varchar(32);uniqueIndex:runtime_verification_identity,priority:3"`
	Operation             string                   `json:"operation" gorm:"type:varchar(32)"`
	Mode                  string                   `json:"mode" gorm:"type:varchar(16);uniqueIndex:runtime_verification_identity,priority:4"`
	FixtureID             string                   `json:"fixture_id" gorm:"type:varchar(128)"`
	Endpoint              string                   `json:"endpoint" gorm:"type:varchar(255)"`
	CatalogHash           string                   `json:"catalog_hash" gorm:"type:varchar(64)"`
	PricingSnapshotHash   string                   `json:"pricing_snapshot_hash" gorm:"type:varchar(64)"`
	BillingExprHash       string                   `json:"billing_expr_hash" gorm:"type:varchar(64)"`
	BillingSnapshotJSON   string                   `json:"billing_snapshot_json,omitempty" gorm:"type:text"`
	FrozenProviderJSON    string                   `json:"frozen_provider_json,omitempty" gorm:"type:text"`
	PluginStateJSON       string                   `json:"plugin_state_json,omitempty" gorm:"type:text"`
	ConfigStatus          string                   `json:"config_status" gorm:"type:varchar(16)"`
	ConnectivityStatus    string                   `json:"connectivity_status" gorm:"type:varchar(16)"`
	RequestStatus         string                   `json:"request_status" gorm:"type:varchar(16)"`
	GenerationStatus      string                   `json:"generation_status" gorm:"type:varchar(16)"`
	ParserStatus          string                   `json:"parser_status" gorm:"type:varchar(16)"`
	BillingStatus         string                   `json:"billing_status" gorm:"type:varchar(16)"`
	LedgerStatus          string                   `json:"ledger_status" gorm:"type:varchar(16)"`
	RequestID             string                   `json:"request_id" gorm:"type:varchar(128)"`
	LocalRequestID        string                   `json:"local_request_id" gorm:"type:varchar(128)"`
	LocalTaskID           string                   `json:"local_task_id" gorm:"type:varchar(128)"`
	TaskID                string                   `json:"task_id" gorm:"type:varchar(128)"`
	TraceID               string                   `json:"trace_id" gorm:"type:varchar(128)"`
	IdempotencyKeyHash    string                   `json:"idempotency_key_hash" gorm:"type:varchar(64)"`
	RequestBodyHash       string                   `json:"request_body_hash" gorm:"type:varchar(64)"`
	TerminalStatus        string                   `json:"terminal_status" gorm:"type:varchar(32)"`
	NormalizedUsageJSON   string                   `json:"normalized_usage_json" gorm:"type:text"`
	ProviderUsageJSON     string                   `json:"provider_usage_json" gorm:"type:text"`
	ProviderUnitCount     string                   `json:"provider_unit_count" gorm:"type:varchar(128)"`
	ProviderCostPoints    string                   `json:"provider_cost_points" gorm:"type:varchar(128)"`
	NewapiRawCost         string                   `json:"newapi_raw_cost" gorm:"type:varchar(128)"`
	BillingSource         string                   `json:"billing_source" gorm:"type:varchar(64)"`
	EvidenceJSON          string                   `json:"evidence_json" gorm:"type:text"`
	CorrelationQuality    string                   `json:"correlation_quality" gorm:"type:varchar(32)"`
	Result                string                   `json:"result" gorm:"type:varchar(64)"`
	ReasonCode            string                   `json:"reason_code" gorm:"type:varchar(128)"`
	NewapiQuota           *int                     `json:"newapi_quota,omitempty"`
	WalletDelta           *int                     `json:"wallet_delta,omitempty"`
	CreatedAt             int64                    `json:"created_at"`
	UpdatedAt             int64                    `json:"updated_at"`
	VerifiedAt            int64                    `json:"verified_at"`
	ReservedProviderCost  string                   `json:"reserved_provider_cost" gorm:"type:varchar(128)"`
	ProviderCostSettledAt int64                    `json:"provider_cost_settled_at"`
	HistoricalEvidence    *RuntimeVerificationItem `json:"historical_evidence,omitempty" gorm:"-"`
}

// RuntimeVerificationEvidence holds facts about outputs, never their payloads
// or locations. Only the server engine may provide these facts.
type RuntimeVerificationEvidence struct {
	HistoricalRuntimeState string            `json:"historical_runtime_state,omitempty"`
	HistoricalReasonCode   string            `json:"historical_reason_code,omitempty"`
	HistoricalRunID        int64             `json:"historical_run_id,omitempty"`
	CurrentCanaryReadiness string            `json:"current_canary_readiness,omitempty"`
	ConfigHash             string            `json:"config_hash,omitempty"`
	WarningCodes           []string          `json:"warning_codes,omitempty"`
	OutcomeClass           string            `json:"outcome_class,omitempty"`
	OutputValid            *bool             `json:"output_valid,omitempty"`
	ReplayIdempotent       *bool             `json:"replay_idempotent,omitempty"`
	JournalState           string            `json:"journal_state,omitempty"`
	BillingSource          string            `json:"billing_source,omitempty"`
	ContractPlan           string            `json:"contract_plan,omitempty"`
	FixtureHash            string            `json:"fixture_hash,omitempty"`
	OutputHash             string            `json:"output_hash,omitempty"`
	DocsHash               string            `json:"docs_hash,omitempty"`
	CatalogSchemaVersion   string            `json:"catalog_schema_version,omitempty"`
	Currency               string            `json:"currency,omitempty"`
	Callable               *bool             `json:"callable,omitempty"`
	OutputCount            *int              `json:"output_count,omitempty"`
	ProviderStatusCode     *int              `json:"provider_status_code,omitempty"`
	PollAttempts           int               `json:"poll_attempts,omitempty"`
	PollTraceIDs           []string          `json:"poll_trace_ids,omitempty"`
	LedgerID               string            `json:"ledger_id,omitempty"`
	LedgerModel            string            `json:"ledger_model,omitempty"`
	LedgerStatus           string            `json:"ledger_status,omitempty"`
	LedgerTraceID          string            `json:"ledger_trace_id,omitempty"`
	CatalogTraceID         string            `json:"catalog_trace_id,omitempty"`
	CurrencyTraceID        string            `json:"currency_trace_id,omitempty"`
	LedgerFacts            map[string]string `json:"ledger_facts,omitempty"`
	RequiredFacts          []string          `json:"required_facts,omitempty"`
	MissingFacts           []string          `json:"missing_facts,omitempty"`
	BlockingReasons        []string          `json:"blocking_reasons,omitempty"`
	OpenReasons            map[string]string `json:"open_reasons,omitempty"`
	Layers                 map[string]string `json:"layers,omitempty"`
}

// RuntimeVerificationProviderSnapshot freezes provider point tariffs and the
// display conversion alongside the production BillingSnapshot.
type RuntimeVerificationContractOverride struct {
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Feature             string `json:"feature"`
	Source              string `json:"source"`
	SourceReferenceHash string `json:"source_reference_hash"`
	ObservedAt          string `json:"observed_at"`
	Version             string `json:"version"`
	CatalogConflict     string `json:"catalog_conflict"`
	Value               string `json:"value"`
	AutoApplyAllowed    bool   `json:"auto_apply_allowed"`
}

type RuntimeVerificationProviderSnapshot struct {
	Model             string                                `json:"model"`
	EndpointType      string                                `json:"endpoint_type"`
	Features          []string                              `json:"features"`
	Rates             map[string]string                     `json:"rates"`
	RateProvenance    map[string]string                     `json:"rate_provenance,omitempty"`
	ContractOverrides []RuntimeVerificationContractOverride `json:"contract_overrides,omitempty"`
	FreeInputImages   *int                                  `json:"free_input_images,omitempty"`
	PointsPerCNY      string                                `json:"points_per_cny"`
	CNYToUSD          string                                `json:"cny_to_usd"`
	Markup            string                                `json:"markup"`
	ConfigHash        string                                `json:"config_hash"`
	PluginHash        string                                `json:"plugin_hash"`
	FixtureHash       string                                `json:"fixture_hash"`
}

func CreateRuntimeVerificationRun(run *RuntimeVerificationRun, items []RuntimeVerificationItem) error {
	if run == nil || run.ID != 0 || run.ChannelID <= 0 || run.FundingUserID < 0 || run.MaxRequests < 0 || run.MaxRequests > 10000 || run.PaidRequests != 0 || run.CompletedAt != 0 {
		return ErrRuntimeVerificationEvidence
	}
	if !runtimeVerificationIdentifier(run.Source, 32, false) || !runtimeVerificationIdentifier(run.Mode, 32, false) || !runtimeVerificationHash(run.CatalogHash, false) || !runtimeVerificationHash(run.CredentialFingerprint, false) || !runtimeVerificationHash(run.AuthorizationManifestHash, true) {
		return ErrRuntimeVerificationEvidence
	}
	if run.AuthorizationIdentityHash != nil && *run.AuthorizationIdentityHash != run.AuthorizationManifestHash {
		return ErrRuntimeVerificationEvidence
	}
	run.AuthorizationIdentityHash = nil
	if run.AuthorizationManifestHash != "" {
		authorizationHash := run.AuthorizationManifestHash
		run.AuthorizationIdentityHash = &authorizationHash
	}
	budget, err := runtimeVerificationDecimal(run.TotalBudgetLimit, true)
	if err != nil {
		return err
	}
	run.TotalBudgetLimit, run.ActualProviderCost, run.ReservedProviderCost = budget.String(), "0", "0"
	if run.StartedAt == 0 {
		run.StartedAt = time.Now().Unix()
	}
	if run.Status == "" {
		run.Status = "PREPARED"
	}
	if !runtimeVerificationIdentifier(run.Status, 64, false) {
		return ErrRuntimeVerificationEvidence
	}
	seen := make(map[string]bool, len(items))
	for i := range items {
		item := &items[i]
		identity := item.Model + "\x00" + item.Protocol + "\x00" + item.Mode
		if item.ID != 0 || item.RunID != 0 || seen[identity] || item.RequestBodyHash != "" || item.IdempotencyKeyHash != "" || item.ProviderCostSettledAt != 0 || item.VerifiedAt != 0 || item.Result == "RUNTIME_VERIFIED" {
			return ErrRuntimeVerificationEvidence
		}
		seen[identity] = true
		if item.CatalogHash == "" {
			item.CatalogHash = run.CatalogHash
		}
		if item.CatalogHash != run.CatalogHash {
			return ErrRuntimeVerificationEvidence
		}
		item.CreatedAt, item.UpdatedAt = run.StartedAt, run.StartedAt
		item.ReservedProviderCost = "0"
		if item.Result == "" {
			item.Result = "CONFIG_READY_RUNTIME_UNTESTED"
		}
		if err := sanitizeRuntimeVerificationItem(item); err != nil {
			return err
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if run.AuthorizationManifestHash != "" {
			var existing int64
			if err := tx.Model(&RuntimeVerificationRun{}).Where("authorization_manifest_hash = ?", run.AuthorizationManifestHash).Count(&existing).Error; err != nil {
				return err
			}
			if existing != 0 {
				return ErrRuntimeVerificationConflict
			}
		}
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].RunID = run.ID
		}
		if len(items) == 0 {
			return nil
		}
		return tx.CreateInBatches(items, 100).Error
	})
}

func GetRuntimeVerificationRun(id int64) (*RuntimeVerificationRun, error) {
	var run RuntimeVerificationRun
	if err := DB.First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func ListRuntimeVerificationItems(runID int64) ([]RuntimeVerificationItem, error) {
	var items []RuntimeVerificationItem
	err := DB.Where("run_id = ?", runID).Order("model asc, protocol asc, mode asc").Find(&items).Error
	return items, err
}

// FindRuntimeVerificationConsumeReceipt proves one committed consume log for
// the exact durable task. Log.RequestId is generated by the logger and is not
// the original local request identity.
func FindRuntimeVerificationConsumeReceipt(run *RuntimeVerificationRun, item *RuntimeVerificationItem, actualQuota int) (bool, error) {
	if run == nil || item == nil || run.ID <= 0 || item.ID <= 0 || item.RunID != run.ID || run.FundingUserID <= 0 || run.ChannelID <= 0 || actualQuota < 0 || !runtimeVerificationIdentifier(item.Model, 128, false) || !runtimeVerificationIdentifier(item.LocalTaskID, 128, false) || !runtimeVerificationIdentifier(item.LocalRequestID, 128, false) {
		return false, ErrRuntimeVerificationEvidence
	}
	if LOG_DB == nil || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return false, errors.New("billing receipt database unavailable or unsupported")
	}
	var candidates []Log
	matches := 0
	err := LOG_DB.Select("id", "other", "upstream_request_id").Where("user_id = ? AND channel_id = ? AND model_name = ? AND quota = ? AND type = ?", run.FundingUserID, run.ChannelID, item.Model, actualQuota, LogTypeConsume).FindInBatches(&candidates, 100, func(_ *gorm.DB, _ int) error {
		for _, candidate := range candidates {
			var receipt struct {
				TaskID             string `json:"task_id"`
				BillingState       string `json:"billing_state"`
				SettlementVerified bool   `json:"settlement_verified"`
				RootInfo           struct {
					UpstreamTaskID    string `json:"upstream_task_id"`
					RequestID         string `json:"request_id"`
					UpstreamRequestID string `json:"upstream_request_id"`
				} `json:"root_info"`
			}
			if common.UnmarshalJsonStr(candidate.Other, &receipt) != nil || receipt.TaskID != item.LocalTaskID || receipt.BillingState != "SETTLED" || !receipt.SettlementVerified {
				continue
			}
			if receipt.RootInfo.UpstreamTaskID != item.TaskID || receipt.RootInfo.RequestID != "" && receipt.RootInfo.RequestID != item.LocalRequestID || receipt.RootInfo.UpstreamRequestID != "" && receipt.RootInfo.UpstreamRequestID != item.RequestID || candidate.UpstreamRequestId != "" && candidate.UpstreamRequestId != item.RequestID {
				continue
			}
			matches++
			if matches > 1 {
				return ErrRuntimeVerificationConflict
			}
		}
		return nil
	}).Error
	if err != nil {
		return false, err
	}
	return matches == 1, nil
}

func LatestRuntimeVerification(channelID int) (*RuntimeVerificationRun, []RuntimeVerificationItem, error) {
	var run RuntimeVerificationRun
	err := DB.Where("channel_id = ?", channelID).Order("started_at desc, id desc").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	items, err := ListRuntimeVerificationItems(run.ID)
	return &run, items, err
}

// LatestRuntimeVerificationEvidence keeps the latest configuration/connectivity
// state distinct from prior paid attempts, including attempts on an old catalog
// or credential. Historical evidence retains its original run and provenance.
func LatestRuntimeVerificationEvidence(channelID int) (*RuntimeVerificationRun, []RuntimeVerificationItem, error) {
	var run RuntimeVerificationRun
	err := DB.Where("channel_id = ? AND status <> ?", channelID, "SUPERSEDED").Order("started_at desc, id desc").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	runs := DB.Model(&RuntimeVerificationRun{}).Select("id").Where("channel_id = ? AND source = ? AND status <> ?", channelID, run.Source, "SUPERSEDED")
	var candidates []RuntimeVerificationItem
	if err := DB.Where("run_id IN (?)", runs).Order("run_id desc, id desc").Find(&candidates).Error; err != nil {
		return nil, nil, err
	}
	// A full catalog refresh defines the current target set. Later single-model
	// refreshes replace only their exact identity; removed modes stay in history.
	var baseline RuntimeVerificationRun
	baselineErr := DB.Where("channel_id = ? AND source = ? AND mode = ? AND status <> ?", channelID, run.Source, "ZERO_COST", "SUPERSEDED").Order("id desc").First(&baseline).Error
	if baselineErr != nil && !errors.Is(baselineErr, gorm.ErrRecordNotFound) {
		return nil, nil, baselineErr
	}
	active := map[[3]string]bool{}
	if baselineErr == nil {
		for _, item := range candidates {
			if item.RunID >= baseline.ID {
				active[[3]string{item.Model, item.Protocol, item.Mode}] = true
			}
		}
	}
	items := make([]RuntimeVerificationItem, 0, len(candidates))
	current := make(map[[3]string]int)
	for _, item := range candidates {
		identity := [3]string{item.Model, item.Protocol, item.Mode}
		if baselineErr == nil && !active[identity] {
			continue
		}
		if index, exists := current[identity]; exists {
			if items[index].HistoricalEvidence == nil {
				previous := item
				items[index].HistoricalEvidence = &previous
			}
			continue
		}
		current[identity] = len(items)
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b RuntimeVerificationItem) int {
		if order := strings.Compare(a.Model, b.Model); order != 0 {
			return order
		}
		if order := strings.Compare(a.Protocol, b.Protocol); order != 0 {
			return order
		}
		return strings.Compare(a.Mode, b.Mode)
	})
	return &run, items, nil
}

// UpdateRuntimeVerificationItem is an engine repository operation. HTTP callers
// must never bind client evidence into this struct. It cannot promote verified
// state or modify the durable budget journal and submit hashes.
func UpdateRuntimeVerificationItem(item *RuntimeVerificationItem) error {
	if item == nil || item.ID <= 0 || item.RunID <= 0 || item.Result == "RUNTIME_VERIFIED" || item.VerifiedAt != 0 {
		return ErrRuntimeVerificationEvidence
	}
	if err := sanitizeRuntimeVerificationItem(item); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var run RuntimeVerificationRun
		if err := lockForUpdate(tx).First(&run, item.RunID).Error; err != nil {
			return err
		}
		var saved RuntimeVerificationItem
		if err := lockForUpdate(tx).Where("id = ? AND run_id = ?", item.ID, item.RunID).First(&saved).Error; err != nil {
			return err
		}
		if saved.Result == "RUNTIME_VERIFIED" || saved.Model != item.Model || saved.Protocol != item.Protocol || saved.Mode != item.Mode || saved.CatalogHash != item.CatalogHash {
			return ErrRuntimeVerificationConflict
		}
		if saved.RequestBodyHash != "" && (saved.Operation != item.Operation || saved.FixtureID != item.FixtureID || saved.Endpoint != item.Endpoint || saved.PricingSnapshotHash != item.PricingSnapshotHash || saved.BillingExprHash != item.BillingExprHash || saved.BillingSnapshotJSON != item.BillingSnapshotJSON || saved.FrozenProviderJSON != item.FrozenProviderJSON) {
			return ErrRuntimeVerificationConflict
		}
		if item.RequestBodyHash != "" && item.RequestBodyHash != saved.RequestBodyHash || item.IdempotencyKeyHash != "" && item.IdempotencyKeyHash != saved.IdempotencyKeyHash {
			return ErrRuntimeVerificationConflict
		}
		if saved.ProviderCostSettledAt != 0 && item.ProviderCostPoints != "" && item.ProviderCostPoints != saved.ProviderCostPoints {
			return ErrRuntimeVerificationConflict
		}
		if saved.LocalTaskID != "" && item.LocalTaskID != "" && item.LocalTaskID != saved.LocalTaskID || saved.LocalRequestID != "" && item.LocalRequestID != "" && item.LocalRequestID != saved.LocalRequestID {
			return ErrRuntimeVerificationConflict
		}
		if item.LocalTaskID == "" {
			item.LocalTaskID = saved.LocalTaskID
		}
		if item.LocalRequestID == "" {
			item.LocalRequestID = saved.LocalRequestID
		}
		if saved.ProviderCostSettledAt != 0 {
			item.ProviderCostPoints = saved.ProviderCostPoints
		}
		if saved.RequestBodyHash != "" && saved.RequestStatus == "AMBIGUOUS" && item.RequestStatus == "NOT_TESTED" {
			item.RequestStatus, item.Result, item.ReasonCode = saved.RequestStatus, saved.Result, saved.ReasonCode
		}
		item.UpdatedAt = time.Now().Unix()
		return tx.Model(&saved).Select("*").Omit("id", "run_id", "model", "protocol", "mode", "catalog_hash", "created_at", "verified_at", "request_body_hash", "idempotency_key_hash", "reserved_provider_cost", "provider_cost_settled_at").Updates(item).Error
	})
}

// ClaimRuntimeVerificationSubmit commits intent before a paid POST. A second
// claim always fails, including after a restart or uncertain network response.
// Manifest expiry and per-model authorization are validated by the engine.
func ClaimRuntimeVerificationSubmit(runID, itemID int64, bodyHash, keyHash, maxPoints string) error {
	if !runtimeVerificationHash(bodyHash, false) || !runtimeVerificationHash(keyHash, false) {
		return ErrRuntimeVerificationEvidence
	}
	hold, err := runtimeVerificationDecimal(maxPoints, false)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var run RuntimeVerificationRun
		if err := lockForUpdate(tx).First(&run, runID).Error; err != nil {
			return err
		}
		var item RuntimeVerificationItem
		if err := lockForUpdate(tx).Where("id = ? AND run_id = ?", itemID, runID).First(&item).Error; err != nil {
			return err
		}
		if run.CompletedAt != 0 || run.AuthorizationManifestHash == "" || run.AuthorizationIdentityHash == nil || *run.AuthorizationIdentityHash != run.AuthorizationManifestHash || run.FundingUserID <= 0 || item.RequestBodyHash != "" || item.IdempotencyKeyHash != "" || item.RequestStatus == "AMBIGUOUS" || item.RequestStatus == "BLOCKED" || item.RequestStatus == "FAIL" || item.Result == "RUNTIME_AMBIGUOUS" || item.ConfigStatus != "PASS" || item.ConnectivityStatus != "PASS" || item.BillingSnapshotJSON == "" || item.FrozenProviderJSON == "" || item.PricingSnapshotHash == "" || item.BillingExprHash == "" {
			return ErrRuntimeVerificationConflict
		}
		var outstanding int64
		if err := tx.Model(&RuntimeVerificationItem{}).Where("run_id = ? AND id <> ? AND request_body_hash <> ? AND (provider_cost_settled_at = ? OR result <> ?)", runID, itemID, "", 0, "RUNTIME_VERIFIED").Count(&outstanding).Error; err != nil {
			return err
		}
		if outstanding != 0 {
			return ErrRuntimeVerificationConflict
		}
		budget, err := runtimeVerificationDecimal(run.TotalBudgetLimit, false)
		if err != nil {
			return err
		}
		actual, err := runtimeVerificationDecimal(run.ActualProviderCost, false)
		if err != nil {
			return err
		}
		reserved, err := runtimeVerificationDecimal(run.ReservedProviderCost, false)
		if err != nil {
			return err
		}
		if run.PaidRequests >= run.MaxRequests || reserved.Add(actual).Add(hold).GreaterThan(budget) {
			return ErrRuntimeVerificationBudget
		}
		updated := tx.Model(&run).Where("paid_requests = ? AND reserved_provider_cost = ? AND actual_provider_cost = ?", run.PaidRequests, run.ReservedProviderCost, run.ActualProviderCost).Updates(map[string]any{"paid_requests": run.PaidRequests + 1, "reserved_provider_cost": reserved.Add(hold).String(), "status": "RUNNING"})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrRuntimeVerificationConflict
		}
		updated = tx.Model(&item).Where("request_body_hash = ? AND idempotency_key_hash = ?", "", "").Updates(map[string]any{"request_body_hash": bodyHash, "idempotency_key_hash": keyHash, "reserved_provider_cost": hold.String(), "request_status": "AMBIGUOUS", "result": "RUNTIME_AMBIGUOUS", "reason_code": "SUBMIT_INTENT_CLAIMED", "updated_at": time.Now().Unix()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrRuntimeVerificationConflict
		}
		return nil
	})
}

// SettleRuntimeVerificationProviderCost is separate from wallet settlement.
// Exact terminal ledger evidence releases this item's provider budget hold.
// Failure, uncertainty or conflicting replay never releases a reservation.
func SettleRuntimeVerificationProviderCost(runID, itemID int64, actualPoints string) error {
	cost, err := runtimeVerificationDecimal(actualPoints, false)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var run RuntimeVerificationRun
		if err := lockForUpdate(tx).First(&run, runID).Error; err != nil {
			return err
		}
		var item RuntimeVerificationItem
		if err := lockForUpdate(tx).Where("id = ? AND run_id = ?", itemID, runID).First(&item).Error; err != nil {
			return err
		}
		if item.ProviderCostSettledAt != 0 {
			if item.ProviderCostPoints == cost.String() {
				return nil
			}
			return ErrRuntimeVerificationConflict
		}
		if item.RequestBodyHash == "" || item.LedgerStatus != "PASS" || !runtimeVerificationExactCorrelation(&item) || !runtimeVerificationTerminal(item.TerminalStatus) {
			return ErrRuntimeVerificationEvidence
		}
		actual, err := runtimeVerificationDecimal(run.ActualProviderCost, false)
		if err != nil {
			return err
		}
		reserved, err := runtimeVerificationDecimal(run.ReservedProviderCost, false)
		if err != nil {
			return err
		}
		hold, err := runtimeVerificationDecimal(item.ReservedProviderCost, false)
		if err != nil {
			return err
		}
		if reserved.LessThan(hold) {
			return ErrRuntimeVerificationConflict
		}
		if err := tx.Model(&run).Updates(map[string]any{"actual_provider_cost": actual.Add(cost).String(), "reserved_provider_cost": reserved.Sub(hold).String()}).Error; err != nil {
			return err
		}
		return tx.Model(&item).Updates(map[string]any{"provider_cost_points": cost.String(), "reserved_provider_cost": "0", "provider_cost_settled_at": time.Now().Unix(), "updated_at": time.Now().Unix()}).Error
	})
}

func FinalizeRuntimeVerificationItem(runID, itemID int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var run RuntimeVerificationRun
		if err := lockForUpdate(tx).First(&run, runID).Error; err != nil {
			return err
		}
		var item RuntimeVerificationItem
		if err := lockForUpdate(tx).Where("id = ? AND run_id = ?", itemID, runID).First(&item).Error; err != nil {
			return err
		}
		if item.Result == "RUNTIME_VERIFIED" {
			return nil
		}
		for _, state := range []string{item.ConfigStatus, item.ConnectivityStatus, item.RequestStatus, item.GenerationStatus, item.ParserStatus, item.BillingStatus, item.LedgerStatus} {
			if state != "PASS" {
				return ErrRuntimeVerificationEvidence
			}
		}
		var evidence RuntimeVerificationEvidence
		if common.UnmarshalJsonStr(item.EvidenceJSON, &evidence) != nil || evidence.OutputValid == nil || !*evidence.OutputValid || evidence.ReplayIdempotent == nil || !*evidence.ReplayIdempotent || evidence.JournalState != "SETTLED" || len(evidence.MissingFacts) != 0 || len(evidence.BlockingReasons) != 0 || len(evidence.OpenReasons) != 0 {
			return ErrRuntimeVerificationEvidence
		}
		if item.TerminalStatus != "succeeded" || !runtimeVerificationExactCorrelation(&item) || item.ProviderCostSettledAt == 0 || item.RequestBodyHash == "" || item.NormalizedUsageJSON == "" || item.NormalizedUsageJSON == "{}" || item.ProviderUsageJSON == "" || item.ProviderUsageJSON == "{}" || item.BillingSnapshotJSON == "" {
			return ErrRuntimeVerificationEvidence
		}
		now := time.Now().Unix()
		if err := tx.Model(&item).Updates(map[string]any{"result": "RUNTIME_VERIFIED", "reason_code": "", "verified_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		var remaining int64
		if err := tx.Model(&RuntimeVerificationItem{}).Where("run_id = ? AND result <> ?", runID, "RUNTIME_VERIFIED").Count(&remaining).Error; err != nil {
			return err
		}
		status, completedAt := "RUNNING", int64(0)
		if remaining == 0 {
			status, completedAt = "COMPLETED", now
		}
		return tx.Model(&run).Updates(map[string]any{"status": status, "completed_at": completedAt}).Error
	})
}

func runtimeVerificationExactCorrelation(item *RuntimeVerificationItem) bool {
	switch item.CorrelationQuality {
	case "EXACT_TRACE_ID":
		return item.TraceID != ""
	case "EXACT_REQUEST_ID":
		return item.RequestID != ""
	case "EXACT_TASK_ID":
		return item.TaskID != ""
	default:
		return false
	}
}

func runtimeVerificationTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "expired", "cancelled":
		return true
	default:
		return false
	}
}

func runtimeVerificationDecimal(raw string, emptyZero bool) (decimal.Decimal, error) {
	if raw == "" && emptyZero {
		return decimal.Zero, nil
	}
	if raw == "" || len(raw) > 128 {
		return decimal.Zero, ErrRuntimeVerificationEvidence
	}
	for _, c := range raw {
		if c != '.' && (c < '0' || c > '9') {
			return decimal.Zero, ErrRuntimeVerificationEvidence
		}
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() {
		return decimal.Zero, ErrRuntimeVerificationEvidence
	}
	return value, nil
}

func runtimeVerificationHash(value string, optional bool) bool {
	if value == "" {
		return optional
	}
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func runtimeVerificationIdentifier(value string, limit int, optional bool) bool {
	if value == "" {
		return optional
	}
	if len(value) > limit || strings.Contains(value, "://") || strings.HasPrefix(strings.ToLower(value), "sk-") {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.:/", c)) {
			return false
		}
	}
	return true
}

// Reject secret-bearing input before projecting allowlisted fields. This also
// catches secrets hidden in otherwise unknown keys that would be discarded.
func runtimeVerificationSafeJSON(raw string) (map[string]any, error) {
	if len(raw) > 1<<20 {
		return nil, ErrRuntimeVerificationEvidence
	}
	var input map[string]any
	if common.UnmarshalJsonStr(raw, &input) != nil || input == nil {
		return nil, ErrRuntimeVerificationEvidence
	}
	if err := runtimeVerificationRejectSecrets(input, 0); err != nil {
		return nil, err
	}
	return input, nil
}

func runtimeVerificationRejectSecrets(value any, depth int) error {
	if depth > 32 {
		return ErrRuntimeVerificationEvidence
	}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			lower := strings.ToLower(key)
			switch lower {
			case "authorization", "headers", "api_key", "apikey", "password", "secret", "token", "access_token", "refresh_token", "prompt", "messages", "request_body", "raw_response", "b64_json", "image", "audio", "video", "url":
				return ErrRuntimeVerificationEvidence
			}
			if strings.HasSuffix(lower, "_url") || strings.HasSuffix(lower, "_secret") || strings.HasSuffix(lower, "_password") {
				return ErrRuntimeVerificationEvidence
			}
			if err := runtimeVerificationRejectSecrets(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := runtimeVerificationRejectSecrets(child, depth+1); err != nil {
				return err
			}
		}
	case string:
		lower := strings.ToLower(v)
		if len(v) > 32768 || strings.Contains(lower, "://") || strings.Contains(lower, "bearer ") || strings.Contains(lower, "data:") || strings.Contains(lower, "authorization:") || strings.Contains(lower, "-----begin") || strings.HasPrefix(lower, "sk-") {
			return ErrRuntimeVerificationEvidence
		}
		for _, c := range v {
			if unicode.IsControl(c) {
				return ErrRuntimeVerificationEvidence
			}
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrRuntimeVerificationEvidence
		}
	}
	return nil
}

func runtimeVerificationUsageJSON(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	input, err := runtimeVerificationSafeJSON(raw)
	if err != nil {
		return "", err
	}
	facts := RuntimeBillingFacts([]byte(raw))
	for key, value := range facts {
		switch v := value.(type) {
		case float64:
			if v < 0 {
				return "", ErrRuntimeVerificationEvidence
			}
		case string:
			switch key {
			case "model", "status", "resolution", "service_tier", "quality", "size", "mode", "tier", "usage_semantic", "unit_type":
				if !runtimeVerificationIdentifier(v, 128, false) {
					return "", ErrRuntimeVerificationEvidence
				}
			default:
				if _, err := runtimeVerificationDecimal(v, false); err != nil {
					return "", err
				}
			}
		case map[string]any:
			if original, ok := input[key].(map[string]any); ok {
				v = original
			}
			body, err := common.Marshal(v)
			if err != nil {
				return "", err
			}
			safe, err := runtimeVerificationUsageJSON(string(body))
			if err != nil {
				return "", err
			}
			var nested map[string]any
			if err := common.UnmarshalJsonStr(safe, &nested); err != nil {
				return "", err
			}
			facts[key] = nested
		}
	}
	for _, key := range []string{"input_image_count", "small_image_count", "large_image_count", "images_up_to_1_5k", "images_above_1_5k", "input_images", "count", "tokens", "units", "clips", "ops", "input_mode", "character_count", "source_duration_sec", "generated_images", "asr_units", "translation_units", "output_pixel_tier", "billable_characters", "successful_output_quantity"} {
		value, exists := input[key]
		if !exists {
			continue
		}
		switch v := value.(type) {
		case float64:
			if v < 0 {
				return "", ErrRuntimeVerificationEvidence
			}
			facts[key] = v
		case string:
			if key == "input_mode" || key == "output_pixel_tier" {
				if !runtimeVerificationIdentifier(v, 64, false) {
					return "", ErrRuntimeVerificationEvidence
				}
				facts[key] = v
			} else {
				if _, err := runtimeVerificationDecimal(v, false); err != nil {
					return "", err
				}
				facts[key] = v
			}
		default:
			return "", ErrRuntimeVerificationEvidence
		}
	}
	encoded, err := common.Marshal(facts)
	return string(encoded), err
}

func sanitizeRuntimeVerificationItem(item *RuntimeVerificationItem) error {
	for _, field := range []struct {
		value    string
		limit    int
		optional bool
	}{
		{item.Model, 128, false}, {item.Protocol, 32, false}, {item.Operation, 32, true}, {item.Mode, 16, false}, {item.FixtureID, 128, true}, {item.Endpoint, 255, true}, {item.RequestID, 128, true}, {item.LocalRequestID, 128, true}, {item.LocalTaskID, 128, true}, {item.TaskID, 128, true}, {item.TraceID, 128, true}, {item.TerminalStatus, 32, true}, {item.Result, 64, true}, {item.ReasonCode, 128, true}, {item.BillingSource, 64, true},
	} {
		if !runtimeVerificationIdentifier(field.value, field.limit, field.optional) {
			return ErrRuntimeVerificationEvidence
		}
	}
	for _, hash := range []string{item.CatalogHash, item.PricingSnapshotHash, item.BillingExprHash, item.RequestBodyHash, item.IdempotencyKeyHash} {
		if !runtimeVerificationHash(hash, true) {
			return ErrRuntimeVerificationEvidence
		}
	}
	for _, state := range []*string{&item.ConfigStatus, &item.ConnectivityStatus, &item.RequestStatus, &item.GenerationStatus, &item.ParserStatus, &item.BillingStatus, &item.LedgerStatus} {
		if *state == "" {
			*state = "NOT_TESTED"
		}
		switch *state {
		case "PASS", "FAIL", "NOT_TESTED", "BLOCKED", "AMBIGUOUS":
		default:
			return ErrRuntimeVerificationEvidence
		}
	}
	if item.CorrelationQuality == "" {
		item.CorrelationQuality = "NONE"
	}
	switch item.CorrelationQuality {
	case "NONE", "EXACT_TRACE_ID", "EXACT_REQUEST_ID", "EXACT_TASK_ID":
	default:
		return ErrRuntimeVerificationEvidence
	}
	if item.NewapiQuota != nil && *item.NewapiQuota < 0 {
		return ErrRuntimeVerificationEvidence
	}
	for _, raw := range []*string{&item.ProviderUnitCount, &item.ProviderCostPoints, &item.NewapiRawCost, &item.ReservedProviderCost} {
		if *raw != "" {
			value, err := runtimeVerificationDecimal(*raw, false)
			if err != nil {
				return err
			}
			*raw = value.String()
		}
	}
	var err error
	item.NormalizedUsageJSON, err = runtimeVerificationUsageJSON(item.NormalizedUsageJSON)
	if err != nil {
		return err
	}
	item.ProviderUsageJSON, err = runtimeVerificationUsageJSON(item.ProviderUsageJSON)
	if err != nil {
		return err
	}
	if item.EvidenceJSON != "" {
		if _, err := runtimeVerificationSafeJSON(item.EvidenceJSON); err != nil {
			return err
		}
		var evidence RuntimeVerificationEvidence
		if err := common.UnmarshalJsonStr(item.EvidenceJSON, &evidence); err != nil {
			return ErrRuntimeVerificationEvidence
		}
		if evidence.HistoricalRunID < 0 {
			return ErrRuntimeVerificationEvidence
		}
		for _, value := range []string{evidence.JournalState, evidence.BillingSource, evidence.ContractPlan, evidence.CatalogSchemaVersion, evidence.Currency, evidence.LedgerID, evidence.LedgerModel, evidence.LedgerStatus, evidence.LedgerTraceID, evidence.CatalogTraceID, evidence.CurrencyTraceID, evidence.OutcomeClass, evidence.HistoricalRuntimeState, evidence.HistoricalReasonCode, evidence.CurrentCanaryReadiness} {
			if !runtimeVerificationIdentifier(value, 128, true) {
				return ErrRuntimeVerificationEvidence
			}
		}
		for _, hash := range []string{evidence.FixtureHash, evidence.OutputHash, evidence.DocsHash, evidence.ConfigHash} {
			if !runtimeVerificationHash(hash, true) {
				return ErrRuntimeVerificationEvidence
			}
		}
		for _, values := range [][]string{evidence.RequiredFacts, evidence.MissingFacts, evidence.BlockingReasons, evidence.WarningCodes} {
			for _, value := range values {
				if !runtimeVerificationIdentifier(value, 128, false) {
					return ErrRuntimeVerificationEvidence
				}
			}
		}
		if len(evidence.PollTraceIDs) > 601 {
			return ErrRuntimeVerificationEvidence
		}
		for _, traceID := range evidence.PollTraceIDs {
			if !runtimeVerificationIdentifier(traceID, 128, false) {
				return ErrRuntimeVerificationEvidence
			}
		}
		for key, raw := range evidence.LedgerFacts {
			switch key {
			case "input_tokens", "output_tokens", "cached_tokens", "unit_count", "cost", "latency_ms", "latency", "attempt_count":
			default:
				return ErrRuntimeVerificationEvidence
			}
			value, err := runtimeVerificationDecimal(raw, false)
			if err != nil {
				return err
			}
			evidence.LedgerFacts[key] = value.String()
		}
		for key, value := range evidence.OpenReasons {
			if !runtimeVerificationIdentifier(key, 32, false) || !runtimeVerificationIdentifier(value, 128, false) {
				return ErrRuntimeVerificationEvidence
			}
		}
		for key, value := range evidence.Layers {
			if !runtimeVerificationIdentifier(key, 32, false) {
				return ErrRuntimeVerificationEvidence
			}
			switch value {
			case "PASS", "FAIL", "NOT_TESTED", "BLOCKED", "AMBIGUOUS":
			default:
				return ErrRuntimeVerificationEvidence
			}
		}
		if evidence.PollAttempts < 0 || evidence.OutputCount != nil && *evidence.OutputCount < 0 || evidence.ProviderStatusCode != nil && (*evidence.ProviderStatusCode < 100 || *evidence.ProviderStatusCode > 599) {
			return ErrRuntimeVerificationEvidence
		}
		body, err := common.Marshal(evidence)
		if err != nil {
			return err
		}
		item.EvidenceJSON = string(body)
	}
	if item.PluginStateJSON != "" {
		input, err := runtimeVerificationSafeJSON(item.PluginStateJSON)
		if err != nil {
			return err
		}
		state := map[string]any{}
		for _, key := range []string{"reservedCharacters", "upstreamModel", "billingPending", "blocker", "requested_duration_sec", "reference_mode", "billing_contract", "input_image_count", "ops", "extract", "remove", "resolution", "input_mode", "_verification_task_id"} {
			if value, ok := input[key]; ok {
				switch v := value.(type) {
				case string:
					if !runtimeVerificationIdentifier(v, 128, false) {
						return ErrRuntimeVerificationEvidence
					}
				case float64:
					if v < 0 {
						return ErrRuntimeVerificationEvidence
					}
				case bool:
				default:
					return ErrRuntimeVerificationEvidence
				}
				state[key] = value
			}
		}
		if contract, ok := input["contract"].(map[string]any); ok {
			projected := map[string]any{}
			for _, key := range []string{"profile", "max_outputs", "max_refs", "refs_plus_outputs_max", "free_input_images", "large_pixel_threshold", "fixed_outputs", "model", "source"} {
				if value, ok := contract[key]; ok {
					switch v := value.(type) {
					case string:
						if !runtimeVerificationIdentifier(v, 128, false) {
							return ErrRuntimeVerificationEvidence
						}
					case float64:
						if v < 0 {
							return ErrRuntimeVerificationEvidence
						}
					default:
						return ErrRuntimeVerificationEvidence
					}
					projected[key] = value
				}
			}
			state["contract"] = projected
		}
		if reservation, exists := input["reservation"]; exists {
			if _, ok := reservation.(map[string]any); !ok {
				return ErrRuntimeVerificationEvidence
			}
			body, err := common.Marshal(reservation)
			if err != nil {
				return err
			}
			safe, err := runtimeVerificationUsageJSON(string(body))
			if err != nil {
				return err
			}
			var projected map[string]any
			if err := common.UnmarshalJsonStr(safe, &projected); err != nil {
				return err
			}
			state["reservation"] = projected
		}
		body, err := common.Marshal(state)
		if err != nil {
			return err
		}
		item.PluginStateJSON = string(body)
	}
	if item.BillingSnapshotJSON != "" {
		if _, err := runtimeVerificationSafeJSON(item.BillingSnapshotJSON); err != nil {
			return err
		}
		var snapshot billingexpr.BillingSnapshot
		if err := common.UnmarshalJsonStr(item.BillingSnapshotJSON, &snapshot); err != nil {
			return ErrRuntimeVerificationEvidence
		}
		if snapshot.ExprString == "" || snapshot.ExprHash != billingexpr.ExprHashString(snapshot.ExprString) || item.BillingExprHash != "" && item.BillingExprHash != snapshot.ExprHash {
			return ErrRuntimeVerificationEvidence
		}
		if !runtimeVerificationIdentifier(snapshot.BillingMode, 32, false) || !runtimeVerificationIdentifier(snapshot.ModelName, 128, false) || !runtimeVerificationIdentifier(strings.ReplaceAll(snapshot.EstimatedTier, "@", "_"), 128, true) {
			return ErrRuntimeVerificationEvidence
		}
		if snapshot.GroupRatio < 0 || snapshot.QuotaPerUnit <= 0 || snapshot.EstimatedQuotaAfterGroup < 0 || snapshot.EstimatedQuotaBeforeGroup < 0 || snapshot.EstimatedPromptTokens < 0 || snapshot.EstimatedCompletionTokens < 0 || snapshot.PreConsumeMultiplier < 0 || snapshot.EstimatedImageCount != nil && *snapshot.EstimatedImageCount < 0 || snapshot.EstimatedFixedPrice != nil && *snapshot.EstimatedFixedPrice < 0 {
			return ErrRuntimeVerificationEvidence
		}
		switch snapshot.EstimatedBillingUnit {
		case "", billingexpr.BillingUnitToken, billingexpr.BillingUnitRequest:
		default:
			return ErrRuntimeVerificationEvidence
		}
		if snapshot.UsageFacts != nil {
			body, err := common.Marshal(snapshot.UsageFacts)
			if err != nil {
				return err
			}
			safe, err := runtimeVerificationUsageJSON(string(body))
			if err != nil {
				return err
			}
			if err := common.UnmarshalJsonStr(safe, &snapshot.UsageFacts); err != nil {
				return err
			}
		}
		body, err := common.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("invalid frozen billing snapshot: %w", err)
		}
		item.BillingSnapshotJSON = string(body)
	}
	if item.FrozenProviderJSON != "" {
		if _, err := runtimeVerificationSafeJSON(item.FrozenProviderJSON); err != nil {
			return err
		}
		var snapshot RuntimeVerificationProviderSnapshot
		if err := common.UnmarshalJsonStr(item.FrozenProviderJSON, &snapshot); err != nil {
			return ErrRuntimeVerificationEvidence
		}
		if snapshot.Model != item.Model || !runtimeVerificationIdentifier(snapshot.EndpointType, 32, false) || snapshot.FreeInputImages != nil && *snapshot.FreeInputImages < 0 {
			return ErrRuntimeVerificationEvidence
		}
		for _, feature := range snapshot.Features {
			if !runtimeVerificationIdentifier(feature, 128, false) {
				return ErrRuntimeVerificationEvidence
			}
		}
		for key, raw := range snapshot.Rates {
			if !runtimeVerificationIdentifier(strings.ReplaceAll(key, "@", "_"), 128, false) {
				return ErrRuntimeVerificationEvidence
			}
			value, err := runtimeVerificationDecimal(raw, false)
			if err != nil {
				return err
			}
			snapshot.Rates[key] = value.String()
		}
		for _, raw := range []string{snapshot.PointsPerCNY, snapshot.CNYToUSD, snapshot.Markup} {
			value, err := runtimeVerificationDecimal(raw, false)
			if err != nil || !value.IsPositive() {
				return ErrRuntimeVerificationEvidence
			}
		}
		for _, hash := range []string{snapshot.ConfigHash, snapshot.PluginHash, snapshot.FixtureHash} {
			if !runtimeVerificationHash(hash, false) {
				return ErrRuntimeVerificationEvidence
			}
		}
		body, err := common.Marshal(snapshot)
		if err != nil {
			return err
		}
		item.FrozenProviderJSON = string(body)
	}
	return nil
}
