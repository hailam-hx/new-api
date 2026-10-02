package service

import (
	"context"
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/shopspring/decimal"
)

const DFLOPVerificationPlanVersion = "canary-plan-v3"

type DFLOPVerificationPlannedTarget struct {
	PricingSnapshotKind    string                        `json:"pricing_snapshot_kind"`
	ContractAudit          VerificationPlanContractAudit `json:"contract_audit"`
	FixtureHash            string                        `json:"fixture_hash"`
	StaticIntentHash       string                        `json:"static_intent_hash"`
	FixturePublicURLHashes []string                      `json:"fixture_public_url_hashes"`
	FixturePublicURLs      []string                      `json:"fixture_public_urls"`
	RequestBody            common.RawMessage             `json:"request_body,omitempty"`
	ConfigHash             string                        `json:"config_hash"`
	PluginHash             string                        `json:"plugin_hash"`
	Model                  string                        `json:"model"`
	Wave                   int                           `json:"wave"`
	Fixture                VerificationFixture           `json:"fixture"`
	PricingSnapshotHash    string                        `json:"pricing_snapshot_hash"`
	BillingExprHash        string                        `json:"billing_expr_hash"`
	RequestBodyHash        string                        `json:"request_body_hash,omitempty"`
	MaximumProviderPoints  *string                       `json:"maximum_provider_points"`
	Blocker                string                        `json:"blocker,omitempty"`
}

type DFLOPVerificationPlan struct {
	Version                       string                           `json:"plan_version"`
	Supersedes                    string                           `json:"supersedes"`
	Currency                      string                           `json:"currency"`
	WaveMaximumProviderPoints     map[int]string                   `json:"wave_maximum_provider_points"`
	InformationalUSD              string                           `json:"informational_usd"`
	PointsPerCNY                  string                           `json:"points_per_cny"`
	ConfiguredCNYToUSD            string                           `json:"configured_cny_to_usd"`
	UnknownMaxima                 int                              `json:"unknown_maxima"`
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
	ModeAuditTotal                int                              `json:"mode_audit_total"`
	ModeAuditConflicts            int                              `json:"mode_audit_conflicts"`
	SupersededPlanStatus          string                           `json:"superseded_plan_status"`
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
	plan = DFLOPVerificationPlan{Version: DFLOPVerificationPlanVersion, Supersedes: "canary-plan-v2", SupersededPlanStatus: "SUPERSEDED", Currency: "points", WaveMaximumProviderPoints: map[int]string{}, PointsPerCNY: source.pointsPerCNY, ConfiguredCNYToUSD: source.config.CNYToUSD, RunID: run.ID, CatalogHash: run.CatalogHash, SourceChannel: run.ChannelID, CredentialFingerprint: run.CredentialFingerprint, Concurrency: 1, ExpiryRecommendation: "30 minutes after explicit approval", Authorization: DFLOPVerificationAuthorization{Approved: false, ChannelID: run.ChannelID, FundingUserID: run.FundingUserID, CatalogHash: run.CatalogHash, CredentialFingerprint: run.CredentialFingerprint, MaxCostPerRequest: map[string]string{}, PlanVersion: DFLOPVerificationPlanVersion, Concurrency: 1}}
	options := engine.FixtureOptions
	options.SourceCatalogHash = source.hash
	fixtures := DFLOPVerificationFixturesWithOptions(source.items, options)
	var mapping map[string]string
	if source.channel.GetModelMapping() != "" {
		if err := common.UnmarshalJsonStr(source.channel.GetModelMapping(), &mapping); err != nil {
			return plan, err
		}
	}
	bindingJSON, _ := common.Marshal(map[string]any{"pricing_config": source.config, "channel_type": source.channel.Type, "channel_setting": source.channel.GetSetting(), "model_mapping": mapping})
	total := decimal.Zero
	for _, fixture := range fixtures {
		fixture.SourceCatalogHash = run.CatalogHash
		fixtureJSON, _ := common.Marshal(fixture)
		target := DFLOPVerificationPlannedTarget{Model: fixture.Model, Wave: 2, Fixture: fixture, FixtureHash: verificationHash(fixtureJSON), FixturePublicURLs: []string{}}
		for _, media := range fixture.Media {
			if media.PublicURL != "" {
				target.FixturePublicURLs = append(target.FixturePublicURLs, media.PublicURL)
			}
		}
		intentJSON, _ := common.Marshal(fixture.Request)
		target.StaticIntentHash = verificationHash(intentJSON)
		target.FixturePublicURLHashes = []string{}
		for _, url := range target.FixturePublicURLs {
			target.FixturePublicURLHashes = append(target.FixturePublicURLHashes, verificationHash([]byte(url)))
		}
		target.ConfigHash = verificationHash(bindingJSON)
		if plugin, ok := jsplugin.DefaultRegistry.Generation().Get(fixture.Plugin); ok {
			metaJSON, _ := common.Marshal(map[string]any{"meta": plugin.Meta, "source_hash": plugin.Engine.SourceHash()})
			target.PluginHash = verificationHash(metaJSON)
			billingPlan := billing_setting.ResolveTaskBillingPlan(fixture.Plugin, fixture.Model, fixture.Model, plugin, true)
			if billingPlan.Resolved {
				target.BillingExprHash = billingexpr.ExprHashString(billingPlan.Expression)
			}
		}
		plan.ModeAuditTotal++
		provider := slices.IndexFunc(source.items, func(item dflop.Item) bool { return item.ModelID == fixture.Model })
		if provider >= 0 {
			pricingJSON, _ := common.Marshal(source.items[provider])
			target.PricingSnapshotHash = verificationHash(pricingJSON)
			target.PricingSnapshotKind = "AUTHENTICATED_CATALOG_INTENT"
			target.ContractAudit = AuditDFLOPVerificationPlanContract(fixture, source.items[provider])
		} else {
			target.ContractAudit.Blocker = "SOURCE_MODEL_NOT_IN_AUTHENTICATED_CATALOG"
		}
		if target.ContractAudit.Blocker != "" {
			plan.ModeAuditConflicts++
			target.Blocker = target.ContractAudit.Blocker
			plan.Targets = append(plan.Targets, target)
			continue
		}
		index := slices.IndexFunc(items, func(item model.RuntimeVerificationItem) bool {
			return item.Model == fixture.Model && item.Protocol == fixture.Protocol && item.Mode == fixture.Mode
		})
		if index < 0 {
			target.Blocker = "OFFLINE_ACCEPTANCE_FAILED"
			plan.Targets = append(plan.Targets, target)
			continue
		}
		item := items[index]
		if item.PricingSnapshotHash != "" {
			target.PricingSnapshotHash, target.BillingExprHash = item.PricingSnapshotHash, item.BillingExprHash
			target.PricingSnapshotKind = "FROZEN_PROVIDER_SNAPSHOT"
		}
		plugin, ok := jsplugin.DefaultRegistry.Generation().Get(fixture.Plugin)
		if !ok {
			target.Blocker = "PLUGIN_BINDING_MISMATCH"
		} else {
			request, requestErr := ValidateDFLOPVerificationFixture(ctx, fixture, plugin)
			if requestErr != nil {
				target.Blocker = verificationReason(requestErr)
			} else {
				target.RequestBodyHash = request.BodyHash
				target.RequestBody = request.Body
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
						var frozen model.RuntimeVerificationProviderSnapshot
						_ = common.UnmarshalJsonStr(item.FrozenProviderJSON, &frozen)
						target.ConfigHash, target.PluginHash = frozen.ConfigHash, frozen.PluginHash
						if frozen.FixtureHash != target.FixtureHash {
							target.Blocker = "FROZEN_FIXTURE_DRIFT"
							plan.Targets = append(plan.Targets, target)
							continue
						}
						maximum := points.String()
						target.MaximumProviderPoints = &maximum
						total = total.Add(points)
						plan.Authorization.Models = append(plan.Authorization.Models, item.Model)
						plan.Authorization.MaxCostPerRequest[item.Model] = maximum
						plan.Authorization.Targets = append(plan.Authorization.Targets, DFLOPVerificationTarget{StaticIntentHash: target.StaticIntentHash, FixturePublicURLHashes: target.FixturePublicURLHashes, Model: item.Model, Protocol: item.Protocol, Mode: item.Mode, FixtureID: item.FixtureID, PricingSnapshotHash: item.PricingSnapshotHash, BillingExprHash: item.BillingExprHash, RequestBodyHash: request.BodyHash, FixtureHash: target.FixtureHash, FixturePublicURLs: target.FixturePublicURLs, Endpoint: fixture.Endpoint, Operation: fixture.Operation, MaximumProviderPoints: maximum, ConfigHash: target.ConfigHash, PluginHash: target.PluginHash})
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
	for _, target := range plan.Targets {
		if target.MaximumProviderPoints == nil {
			plan.UnknownMaxima++
			continue
		}
		value, _ := decimal.NewFromString(*target.MaximumProviderPoints)
		old, _ := decimal.NewFromString(plan.WaveMaximumProviderPoints[target.Wave])
		plan.WaveMaximumProviderPoints[target.Wave] = old.Add(value).String()
	}
	conversion, _ := decimal.NewFromString(source.config.CNYToUSD)
	pointsPerCNY, _ := decimal.NewFromString(source.pointsPerCNY)
	plan.InformationalUSD = total.Mul(conversion).DivRound(pointsPerCNY, 12).String()
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
