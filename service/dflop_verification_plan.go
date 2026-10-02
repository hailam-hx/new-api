package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/shopspring/decimal"
)

type DFLOPVerificationPlannedTarget struct {
	Model                 string              `json:"model"`
	Wave                  int                 `json:"wave"`
	Fixture               VerificationFixture `json:"fixture"`
	PricingSnapshotHash   string              `json:"pricing_snapshot_hash"`
	BillingExprHash       string              `json:"billing_expr_hash"`
	RequestBodyHash       string              `json:"request_body_hash,omitempty"`
	MaximumProviderPoints *string             `json:"maximum_provider_points"`
	Blocker               string              `json:"blocker,omitempty"`
}

type DFLOPVerificationPlan struct {
	RunID                         int64                            `json:"run_id"`
	CatalogHash                   string                           `json:"catalog_hash"`
	SourceChannel                 int                              `json:"source_channel_id"`
	CredentialFingerprint         string                           `json:"credential_fingerprint"`
	Concurrency                   int                              `json:"concurrency"`
	PaidRequestsExecuted          int                              `json:"paid_requests_executed"`
	PlannedPOSTs                  int                              `json:"planned_posts"`
	FixturePlans                  int                              `json:"fixture_plans"`
	KnownMaximumProviderPoints    string                           `json:"known_maximum_provider_points"`
	CompleteMaximumProviderPoints *string                          `json:"complete_maximum_provider_points"`
	ExpiryRecommendation          string                           `json:"expiry_recommendation"`
	Authorization                 DFLOPVerificationAuthorization   `json:"proposed_authorization"`
	Targets                       []DFLOPVerificationPlannedTarget `json:"targets"`
}

// Proposed manifests are always unapproved. Targets with unknown costs or
// unavailable fixtures cannot be included in the executable allowlist.
func (engine DFLOPVerificationEngine) Plan(ctx context.Context, runID int64) (DFLOPVerificationPlan, error) {
	var plan DFLOPVerificationPlan
	run, err := model.GetRuntimeVerificationRun(runID)
	if err != nil {
		return plan, err
	}
	source, err := engine.source(ctx, run.ChannelID)
	if err != nil {
		return plan, err
	}
	if source.hash != run.CatalogHash || source.fingerprint != run.CredentialFingerprint {
		return plan, errors.New("DRIFT_REVIEW_REQUIRED")
	}
	items, err := model.ListRuntimeVerificationItems(run.ID)
	if err != nil {
		return plan, err
	}
	plan = DFLOPVerificationPlan{RunID: run.ID, CatalogHash: run.CatalogHash, SourceChannel: run.ChannelID, CredentialFingerprint: run.CredentialFingerprint, Concurrency: 1, ExpiryRecommendation: "30 minutes after explicit approval", Authorization: DFLOPVerificationAuthorization{Approved: false, ChannelID: run.ChannelID, FundingUserID: run.FundingUserID, CatalogHash: run.CatalogHash, CredentialFingerprint: run.CredentialFingerprint, MaxCostPerRequest: map[string]string{}, ExpiresAt: time.Now().Add(30 * time.Minute).Unix()}}
	fixtures := DFLOPVerificationFixtures(source.items)
	total := decimal.Zero
	for _, fixture := range fixtures {
		fixture.SourceCatalogHash = run.CatalogHash
		target := DFLOPVerificationPlannedTarget{Model: fixture.Model, Wave: 2, Fixture: fixture}
		index := slices.IndexFunc(items, func(item model.RuntimeVerificationItem) bool {
			return item.Model == fixture.Model && item.Protocol == fixture.Protocol && item.Mode == fixture.Mode
		})
		if index < 0 {
			target.Blocker = "OFFLINE_ACCEPTANCE_FAILED"
			plan.Targets = append(plan.Targets, target)
			continue
		}
		item := items[index]
		target.PricingSnapshotHash, target.BillingExprHash = item.PricingSnapshotHash, item.BillingExprHash
		plugin, ok := jsplugin.DefaultRegistry.Generation().Get(fixture.Plugin)
		if !ok {
			target.Blocker = "PLUGIN_BINDING_MISMATCH"
		} else {
			request, requestErr := ValidateDFLOPVerificationFixture(ctx, fixture, plugin)
			if requestErr != nil {
				target.Blocker = verificationReason(requestErr)
			} else {
				target.RequestBodyHash = request.BodyHash
				provider := slices.IndexFunc(source.items, func(item dflop.Item) bool { return item.ModelID == fixture.Model })
				facts, factsErr := verificationMaximumFacts(fixture, request)
				if provider < 0 {
					target.Blocker = "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG"
				} else if factsErr != nil {
					target.Blocker = verificationReason(factsErr)
				} else {
					points, pointsErr := DFLOPVerificationPoints(source.items[provider], facts)
					if pointsErr != nil {
						target.Blocker = verificationReason(pointsErr)
					} else if item.BillingSnapshotJSON == "" || item.FrozenProviderJSON == "" {
						target.Blocker = "OFFLINE_ACCEPTANCE_FAILED"
					} else {
						maximum := points.String()
						target.MaximumProviderPoints = &maximum
						total = total.Add(points)
						plan.Authorization.Models = append(plan.Authorization.Models, item.Model)
						plan.Authorization.MaxCostPerRequest[item.Model] = maximum
						plan.Authorization.Targets = append(plan.Authorization.Targets, DFLOPVerificationTarget{Model: item.Model, Protocol: item.Protocol, Mode: item.Mode, FixtureID: item.FixtureID, PricingSnapshotHash: item.PricingSnapshotHash, BillingExprHash: item.BillingExprHash, RequestBodyHash: request.BodyHash})
					}
				}
			}
		}
		plan.Targets = append(plan.Targets, target)
	}
	// Select the cheapest eligible representative of each contract family.
	cheapest := map[string]int{}
	for i, target := range plan.Targets {
		if target.MaximumProviderPoints == nil {
			continue
		}
		previous, found := cheapest[target.Fixture.Plan.ID]
		price, _ := decimal.NewFromString(*target.MaximumProviderPoints)
		if !found {
			cheapest[target.Fixture.Plan.ID] = i
			continue
		}
		old, _ := decimal.NewFromString(*plan.Targets[previous].MaximumProviderPoints)
		if price.LessThan(old) {
			cheapest[target.Fixture.Plan.ID] = i
		}
	}
	for _, i := range cheapest {
		plan.Targets[i].Wave = 1
	}
	plan.FixturePlans = len(fixtures)
	plan.PlannedPOSTs = len(plan.Authorization.Targets)
	plan.Authorization.MaxRequests = plan.PlannedPOSTs
	plan.KnownMaximumProviderPoints = total.String()
	plan.Authorization.MaxTotalProviderPoints = total.String()
	if plan.PlannedPOSTs == len(fixtures) {
		value := total.String()
		plan.CompleteMaximumProviderPoints = &value
	}
	return plan, nil
}

// FixturePlanJSON is suitable for an explicit approval artifact, never for
// credentials or an inferred activation flag.
func (plan DFLOPVerificationPlan) JSON() ([]byte, error) { return common.Marshal(plan) }
