package service

import (
	"context"
	"crypto/ed25519"
	"errors"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ValidateDFLOPAdminExecution binds the signed offline approval to the exact
// reviewed plan, administrator and channel before any provider I/O or funding.
func ValidateDFLOPAdminExecution(channelID, userID int, plan DFLOPVerificationPlan, manifestJSON, confirmedHash string, trusted ed25519.PublicKey, now time.Time) (DFLOPVerificationAuthorization, error) {
	var authorization DFLOPVerificationAuthorization
	if len(manifestJSON) == 0 || len(manifestJSON) > 1<<20 || verificationHash([]byte(manifestJSON)) != confirmedHash || common.UnmarshalJsonStr(manifestJSON, &authorization) != nil {
		return authorization, errors.New("MANIFEST_CONFIRMATION_MISMATCH")
	}
	if err := VerifyDFLOPVerificationManifest(authorization, trusted, now); err != nil {
		return authorization, err
	}
	if userID <= 0 || authorization.FundingUserID != userID || authorization.ChannelID != channelID || plan.SourceChannel != channelID || plan.Authorization.FundingUserID != userID || plan.Version != DFLOPVerificationPlanVersion || plan.CatalogHash != authorization.CatalogHash || plan.CredentialFingerprint != authorization.CredentialFingerprint {
		return authorization, errors.New("APPROVAL_SCOPE_MISMATCH")
	}
	if len(authorization.Targets) == 0 || len(authorization.Targets) > 100 || authorization.MaxRequests != len(authorization.Targets) {
		return authorization, errors.New("PAID_REQUEST_LIMIT")
	}
	seen := map[string]bool{}
	for _, target := range authorization.Targets {
		identity := target.Model + "/" + target.Protocol + "/" + target.Mode + "/" + target.FixtureID
		if seen[identity] {
			return authorization, errors.New("DUPLICATE_APPROVED_TARGET")
		}
		seen[identity] = true
		index := slices.IndexFunc(plan.Targets, func(candidate DFLOPVerificationPlannedTarget) bool {
			return candidate.Model == target.Model && candidate.Fixture.Protocol == target.Protocol && candidate.Fixture.Mode == target.Mode && candidate.Fixture.ID == target.FixtureID
		})
		if index < 0 {
			return authorization, errors.New("APPROVED_TARGET_MISSING_FROM_PLAN")
		}
		candidate := plan.Targets[index]
		if candidate.Blocker != "" {
			return authorization, errors.New(candidate.Blocker)
		}
		if candidate.ExecutorBlocker != "" {
			return authorization, errors.New(candidate.ExecutorBlocker)
		}
		if candidate.MaximumProviderPoints == nil || *candidate.MaximumProviderPoints != target.MaximumProviderPoints || candidate.RequestBodyHash != target.RequestBodyHash || candidate.FixtureHash != target.FixtureHash || candidate.ConfigHash != target.ConfigHash || candidate.PluginHash != target.PluginHash || candidate.PricingSnapshotHash != target.PricingSnapshotHash || candidate.BillingExprHash != target.BillingExprHash || candidate.Fixture.Endpoint != target.Endpoint || candidate.Fixture.Operation != target.Operation {
			return authorization, errors.New("PAID_TARGET_BINDING_MISMATCH")
		}
	}
	return authorization, nil
}

// ExecuteAdminPlan uses the existing durable manifest/intent claim and billing
// engine. An error stops the batch; a lost HTTP response never permits replay.
func (engine DFLOPVerificationEngine) ExecuteAdminPlan(ctx context.Context, channelID, userID int, plan DFLOPVerificationPlan, authorization DFLOPVerificationAuthorization) (*model.RuntimeVerificationRun, error) {
	options, err := engine.RestoreApprovedPreparation(ctx, channelID, plan, authorization)
	if err != nil {
		return nil, err
	}
	engine.FixtureOptions = options
	run, items, err := engine.CreateAuthorizedRun(ctx, channelID, userID, authorization)
	if err != nil {
		return nil, err
	}
	for _, approved := range authorization.Targets {
		planned := slices.IndexFunc(plan.Targets, func(target DFLOPVerificationPlannedTarget) bool {
			return target.Model == approved.Model && target.Fixture.ID == approved.FixtureID && target.Fixture.Mode == approved.Mode && target.Fixture.Protocol == approved.Protocol
		})
		item := slices.IndexFunc(items, func(item model.RuntimeVerificationItem) bool {
			return item.Model == approved.Model && item.FixtureID == approved.FixtureID && item.Mode == approved.Mode && item.Protocol == approved.Protocol
		})
		if planned < 0 || item < 0 {
			return run, errors.New("PAID_TARGET_NOT_AUTHORIZED")
		}
		if err = engine.Execute(ctx, run.ID, items[item].ID, authorization, plan.Targets[planned].Fixture); err != nil {
			return run, err
		}
		saved, err := model.ListRuntimeVerificationItems(run.ID)
		if err != nil {
			return run, err
		}
		index := slices.IndexFunc(saved, func(row model.RuntimeVerificationItem) bool { return row.ID == items[item].ID })
		if index < 0 || saved[index].Result != "RUNTIME_VERIFIED" {
			return run, errors.New("CANARY_STOP_ON_UNRESOLVED_RESULT")
		}
	}
	return run, nil
}

// ResumeAdminItem accepts only an already claimed intent owned by the funding
// administrator on this channel. The engine rechecks frozen fixture hashes and
// performs only GETs; arbitrary task IDs cannot become verification evidence.
func (engine DFLOPVerificationEngine) ResumeAdminItem(ctx context.Context, channelID, userID int, runID, itemID int64, fixture VerificationFixture) (*model.RuntimeVerificationRun, error) {
	run, err := model.GetRuntimeVerificationRun(runID)
	if err != nil {
		return nil, err
	}
	if userID <= 0 || run.ChannelID != channelID || run.FundingUserID != userID || run.AuthorizationManifestHash == "" {
		return nil, errors.New("VERIFICATION_RUN_SCOPE_MISMATCH")
	}
	items, err := model.ListRuntimeVerificationItems(run.ID)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(items, func(item model.RuntimeVerificationItem) bool { return item.ID == itemID })
	if index < 0 || items[index].RequestBodyHash == "" {
		return nil, errors.New("CLAIMED_VERIFICATION_INTENT_REQUIRED")
	}
	if engine.FixtureOptions.VerifyPublicMedia == nil {
		engine.FixtureOptions.VerifyPublicMedia = VerifyDFLOPPublicMedia
	}
	if engine.FixtureOptions.VerifyClipSource == nil && fixture.ClipPreparation != nil {
		engine.FixtureOptions.VerifyClipSource = engine.ClipPreparationVerifier(channelID, *fixture.ClipPreparation)
	}
	return run, engine.Resume(ctx, run.ID, itemID, fixture)
}
