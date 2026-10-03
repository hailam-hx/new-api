// dflop-runtime-verify prepares zero-cost evidence by default. The paid mode
// requires a newly approved, exact manifest and a matching confirmation hash.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	_ "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/service"
)

func run(args []string) error {
	common.BatchUpdateEnabled = os.Getenv("BATCH_UPDATE_ENABLED") == "true"
	flags := flag.NewFlagSet("dflop-runtime-verify", flag.ContinueOnError)
	mode := flags.String("mode", "prepare", "prepare, import-human, verify-public (HEAD/GET/Range only), approve, execute, or resume")
	humanPack := flags.String("human-pack", "", "operator-owned directory with metadata.json, portrait.png, face-video.mp4 and motion-video.mp4")
	database := flags.String("sqlite-db", "one-api.db", "existing SQLite database, SQL_DSN may select another engine")
	channel := flags.Int("channel-id", 1, "selected authenticated DFLOP channel")
	user := flags.Int("funding-user-id", 0, "admin user for pricing group; paid mode requires manifest binding")
	output := flags.String("output", "", "non-secret plan JSON output path")
	fixtureOrigin := flags.String("fixture-origin", os.Getenv("VERIFICATION_FIXTURE_PUBLIC_BASE_URL"), "deployed public HTTPS origin for immutable embedded fixtures")
	presetAvatar := flags.String("preset-avatar", "", "fresh GET-confirmed ready provider preset avatar ID")
	preparedClipFile := flags.String("prepared-clip-file", "", "reuse fresh same-source zero-cost ASR evidence without another POST")
	prepareClip := flags.Bool("prepare-clip", false, "prepare-only opt in: verified public source + current zero-price subtitle contract, then require new exact approval")
	presetVoice := flags.String("preset-voice", "", "fresh GET-confirmed provider preset voice ID")
	planPath := flags.String("plan", "", "frozen fixture plan JSON")
	manifestPath := flags.String("manifest", "", "explicitly approved authorization JSON")
	approvalKey := flags.String("approval-private-key-file", "", "operator-owned Ed25519 private key hex file; mode approve only")
	approvalActor := flags.String("approved-by", "", "explicit approving operator; mode approve only")
	approvalRef := flags.String("approval-reference", "", "explicit operator authorization reference")
	confirmPlan := flags.String("confirm-plan-hash", "", "SHA256 of exact reviewed current plan; mode approve only")
	confirm := flags.String("confirm-manifest-hash", "", "SHA256 of exact approved manifest file")
	execute := flags.Bool("execute", false, "explicit paid-mode opt in; never enabled by prepare or resume")
	runID := flags.Int64("run-id", 0, "existing authorized run for GET-only recovery")
	itemID := flags.Int64("item-id", 0, "single claimed intent for GET-only recovery")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !slices.Contains([]string{"prepare", "import-human", "verify-public", "approve", "execute", "resume"}, *mode) {
		return errors.New("INVALID_COMMAND")
	}
	if *mode == "import-human" {
		if *execute || *prepareClip || *manifestPath != "" || *humanPack == "" {
			return errors.New("OFFLINE_IMPORT_ONLY")
		}
		fixtures, err := service.ImportDFLOPHumanPack(*humanPack)
		if err != nil {
			return err
		}
		body, err := common.Marshal(fixtures)
		if err != nil {
			return err
		}
		path := *output
		if path == "" {
			path = *humanPack + "/registry.json"
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write(body)
		return err
	}
	if *mode == "verify-public" {
		if *execute || *prepareClip || *manifestPath != "" {
			return errors.New("GET_ONLY_MODE")
		}
		published, err := service.DFLOPVerificationPublishedMedia(*fixtureOrigin)
		if err != nil {
			return errors.New("OPERATOR_DEPLOY_REQUIRED: configure VERIFICATION_FIXTURE_PUBLIC_BASE_URL")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		results := map[string]any{}
		failed := false
		for _, media := range service.DFLOPVerificationMediaInventory() {
			fixture := published[media.ID]
			err := service.VerifyDFLOPPublicMedia(ctx, fixture)
			status := "PUBLIC_FIXTURE_VERIFIED"
			reason := ""
			if err != nil {
				status = "OPERATOR_DEPLOY_REQUIRED"
				reason = err.Error()
				failed = true
			}
			results[media.ID] = map[string]any{"url": fixture.PublicURL, "sha256": fixture.SHA256, "mime": fixture.MIMEType, "bytes": fixture.Bytes, "status": status, "reason": reason, "verified_at": time.Now().UTC().Format(time.RFC3339)}
		}
		data, _ := common.Marshal(results)
		if *output != "" {
			if err := os.WriteFile(*output, data, 0600); err != nil {
				return err
			}
		} else {
			fmt.Println(string(data))
		}
		if failed {
			return errors.New("OPERATOR_DEPLOY_REQUIRED")
		}
		return nil
	}
	if (*prepareClip || *preparedClipFile != "") && *mode != "prepare" {
		return errors.New("CLIP_PREPARATION_ONLY_BEFORE_NEW_APPROVAL")
	}
	if *mode != "execute" && (*execute || *manifestPath != "" || *confirm != "") {
		return errors.New("GET_ONLY_MODE")
	}
	var manifest service.DFLOPVerificationAuthorization
	var plan service.DFLOPVerificationPlan
	if *mode == "execute" {
		if os.Getenv("BATCH_UPDATE_ENABLED") == "true" {
			return errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED")
		}
		if !*execute || *manifestPath == "" || *planPath == "" || *confirm == "" {
			return errors.New("PAID_AUTHORIZATION_REQUIRED")
		}
		body, err := os.ReadFile(*manifestPath)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != *confirm {
			return errors.New("MANIFEST_CONFIRMATION_MISMATCH")
		}
		if common.Unmarshal(body, &manifest) != nil || !manifest.Approved || manifest.ApprovalReference == "" || manifest.ApprovedBy == "" || manifest.ExpiresAt <= time.Now().Unix() || manifest.FundingUserID != *user || manifest.ChannelID != *channel {
			return errors.New("PAID_AUTHORIZATION_REQUIRED")
		}
		trusted, decodeErr := hex.DecodeString(os.Getenv("DFLOP_VERIFICATION_APPROVAL_PUBLIC_KEY"))
		if decodeErr != nil {
			return errors.New("TRUSTED_APPROVAL_KEY_REQUIRED")
		}
		if err = service.VerifyDFLOPVerificationManifest(manifest, trusted, time.Now()); err != nil {
			return err
		}

	}
	if *mode != "prepare" {
		body, err := os.ReadFile(*planPath)
		if err != nil {
			return err
		}
		if err = common.Unmarshal(body, &plan); err != nil {
			return err
		}
	}
	if *mode == "approve" {
		if *planPath == "" || *output == "" || *approvalKey == "" || *approvalActor == "" || *approvalRef == "" || *confirmPlan == "" {
			return errors.New("EXPLICIT_CURRENT_PLAN_APPROVAL_REQUIRED")
		}
		body, err := os.ReadFile(*planPath)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != *confirmPlan {
			return errors.New("PLAN_CONFIRMATION_MISMATCH")
		}
		info, err := os.Stat(*approvalKey)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0077 != 0 {
			return errors.New("APPROVAL_KEY_PERMISSIONS_UNSAFE")
		}
		keyText, err := os.ReadFile(*approvalKey)
		if err != nil {
			return err
		}
		key, err := hex.DecodeString(strings.TrimSpace(string(keyText)))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return errors.New("APPROVAL_KEY_INVALID")
		}
		final, err := service.FinalizeDFLOPVerificationAuthorization(plan, *approvalActor, *approvalRef, ed25519.PrivateKey(key), time.Now())
		if err != nil {
			return err
		}
		trusted, err := hex.DecodeString(os.Getenv("DFLOP_VERIFICATION_APPROVAL_PUBLIC_KEY"))
		if err != nil {
			return errors.New("TRUSTED_APPROVAL_KEY_REQUIRED")
		}
		if err = service.VerifyDFLOPVerificationManifest(final, trusted, time.Now()); err != nil {
			return err
		}
		signed, err := common.Marshal(final)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write(signed)
		return err
	}
	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = *database
	if err := model.InitDB(); err != nil {
		return err
	}
	if err := model.InitLogDB(); err != nil {
		return err
	}
	if err := model.DB.AutoMigrate(&model.RuntimeVerificationRun{}, &model.RuntimeVerificationItem{}); err != nil {
		return err
	}
	model.InitOptionMap()
	controller.SyncTaskPluginsOnce()
	options := service.VerificationFixtureOptions{PresetAvatar: *presetAvatar, PresetVoice: *presetVoice}
	if *fixtureOrigin != "" {
		published, err := service.DFLOPVerificationPublishedMedia(*fixtureOrigin)
		if err != nil {
			return err
		}
		options.PublishedMedia = published
		options.VerifyPublicMedia = service.VerifyDFLOPPublicMedia
	}
	engine := service.DFLOPVerificationEngine{FixtureOptions: options}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	switch *mode {
	case "prepare":
		if *user <= 0 || *output == "" {
			return errors.New("PREPARE_REQUIRES_USER_AND_OUTPUT")
		}
		avatar, voice, err := engine.PreparePresetResources(ctx, *channel)
		if err != nil {
			return err
		}
		if *presetAvatar != "" && *presetAvatar != avatar.ID || *presetVoice != "" && *presetVoice != voice.ID {
			return errors.New("PRESET_GET_REVALIDATION_REQUIRED")
		}
		engine.FixtureOptions.PresetAvatar = avatar.ID
		engine.FixtureOptions.PresetVoice = voice.ID
		engine.FixtureOptions.AvatarPresetEvidence = &avatar
		engine.FixtureOptions.VoicePresetEvidence = &voice
		evidence := map[string]any{"avatar": avatar, "voice": voice, "paid_requests": 0}
		if *prepareClip {
			media, exists := engine.FixtureOptions.PublishedMedia["speaking-square-v1"]
			if !exists {
				return errors.New("OPERATOR_DEPLOY_REQUIRED")
			}
			prepared, err := engine.PrepareClip(ctx, *channel, media)
			if err != nil {
				safe, _ := common.Marshal(map[string]any{"clip_preparation": prepared, "status": err.Error(), "paid_requests": 0, "retry": false})
				if *output != "" {
					if saveErr := os.WriteFile(*output+".preparation-error.json", safe, 0600); saveErr != nil {
						return saveErr
					}
				}
				return err
			}
			engine.FixtureOptions.ClipPreparation = &prepared
			engine.FixtureOptions.ClipASRID = prepared.ASRID
			engine.FixtureOptions.ClipSourceVideoURL = prepared.SourceURL
			engine.FixtureOptions.ClipStyleID = prepared.StyleID
			engine.FixtureOptions.VerifyClipSource = engine.ClipPreparationVerifier(*channel, prepared)
			evidence["clip_preparation"] = prepared
		}
		if *preparedClipFile != "" {
			if *prepareClip {
				return errors.New("CLIP_PREPARATION_ALREADY_AVAILABLE")
			}
			body, err := os.ReadFile(*preparedClipFile)
			if err != nil {
				return err
			}
			var existing struct {
				Clip service.VerificationClipPreparation `json:"clip_preparation"`
			}
			if common.Unmarshal(body, &existing) != nil {
				return errors.New("CLIP_PREPARATION_INVALID")
			}
			prepared := existing.Clip
			media, ok := engine.FixtureOptions.PublishedMedia["speaking-square-v1"]
			if !ok || media.SHA256 != prepared.SourceSHA256 || media.PublicURL != prepared.SourceURL {
				return errors.New("CLIP_PREPARATION_SOURCE_MISMATCH")
			}
			if err := engine.ClipPreparationVerifier(*channel, prepared)(ctx, media.PublicURL, prepared.ASRID); err != nil {
				return err
			}
			engine.FixtureOptions.ClipPreparation = &prepared
			engine.FixtureOptions.ClipASRID = prepared.ASRID
			engine.FixtureOptions.ClipSourceVideoURL = prepared.SourceURL
			engine.FixtureOptions.ClipStyleID = prepared.StyleID
			engine.FixtureOptions.VerifyClipSource = engine.ClipPreparationVerifier(*channel, prepared)
			evidence["clip_preparation"] = prepared
		}
		prerequisiteJSON, _ := common.Marshal(evidence)
		if *output != "" {
			if err := os.WriteFile(*output+".prerequisites.json", prerequisiteJSON, 0600); err != nil {
				return err
			}
		}
		run, _, err := engine.Prepare(ctx, *channel, "", *user)
		if err != nil {
			return err
		}
		plan, err = engine.Plan(ctx, run.ID)
		if err != nil {
			return err
		}
		body, err := plan.JSON()
		if err != nil {
			return err
		}
		if err = os.WriteFile(*output, body, 0600); err != nil {
			return err
		}
		fmt.Printf("PREPARED run=%d fixtures=%d eligible=%d paid_requests=0\n", run.ID, plan.FixturePlans, plan.PlannedPOSTs)
	case "execute":
		if plan.Version != service.DFLOPVerificationPlanVersion || manifest.PlanVersion != plan.Version || manifest.Concurrency != 1 {
			return errors.New("SUPERSEDED_MANIFEST_REQUIRES_NEW_APPROVAL")
		}
		if plan.CatalogHash != manifest.CatalogHash || plan.CredentialFingerprint != manifest.CredentialFingerprint {
			return errors.New("DRIFT_REVIEW_REQUIRED")
		}
		restored, err := engine.RestoreApprovedPreparation(ctx, *channel, plan, manifest)
		if err != nil {
			return err
		}
		engine.FixtureOptions = restored
		run, items, err := engine.CreateAuthorizedRun(ctx, *channel, *user, manifest)
		if err != nil {
			return err
		}
		// Sequential waves stop immediately on any unresolved item. No request
		// retries or new approval run can reset the durable manifest limit.
		for wave := 1; wave <= 3; wave++ {
			for _, target := range plan.Targets {
				if target.Wave != wave || !slices.Contains(manifest.Models, target.Model) {
					continue
				}
				index := slices.IndexFunc(items, func(item model.RuntimeVerificationItem) bool {
					return item.Model == target.Model && item.Protocol == target.Fixture.Protocol && item.Mode == target.Fixture.Mode
				})
				if index < 0 || target.Blocker != "" || target.MaximumProviderPoints == nil {
					return errors.New("OFFLINE_ACCEPTANCE_FAILED")
				}
				if err = engine.Execute(ctx, run.ID, items[index].ID, manifest, target.Fixture); err != nil {
					return err
				}
				var saved model.RuntimeVerificationItem
				if err = model.DB.First(&saved, items[index].ID).Error; err != nil {
					return err
				}
				fmt.Printf("run=%d item=%d result=%s\n", run.ID, saved.ID, saved.Result)
				if saved.Result != "RUNTIME_VERIFIED" {
					return errors.New("CANARY_STOP_ON_UNRESOLVED_RESULT")
				}
			}
		}
	case "resume":
		if *runID <= 0 || *itemID <= 0 {
			return errors.New("RESUME_REQUIRES_EXACT_RUN_AND_ITEM")
		}
		var item model.RuntimeVerificationItem
		if err := model.DB.Where("id = ? AND run_id = ?", *itemID, *runID).First(&item).Error; err != nil {
			return err
		}
		index := slices.IndexFunc(plan.Targets, func(target service.DFLOPVerificationPlannedTarget) bool {
			return target.Model == item.Model && target.Fixture.ID == item.FixtureID && target.Fixture.Mode == item.Mode && target.Fixture.Protocol == item.Protocol
		})
		if index < 0 {
			return errors.New("FROZEN_FIXTURE_REQUIRED")
		}
		if err := engine.Resume(ctx, *runID, *itemID, plan.Targets[index].Fixture); err != nil {
			return err
		}
		fmt.Printf("GET_ONLY_RECOVERY run=%d item=%d\n", *runID, *itemID)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
