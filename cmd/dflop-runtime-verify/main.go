// dflop-runtime-verify prepares zero-cost evidence by default. The paid mode
// requires a newly approved, exact manifest and a matching confirmation hash.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
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
	mode := flags.String("mode", "prepare", "prepare (GET/offline only), execute, or resume (GET only)")
	database := flags.String("sqlite-db", "one-api.db", "existing SQLite database, SQL_DSN may select another engine")
	channel := flags.Int("channel-id", 1, "selected authenticated DFLOP channel")
	user := flags.Int("funding-user-id", 0, "admin user for pricing group; paid mode requires manifest binding")
	output := flags.String("output", "", "non-secret plan JSON output path")
	planPath := flags.String("plan", "", "frozen fixture plan JSON")
	manifestPath := flags.String("manifest", "", "explicitly approved authorization JSON")
	confirm := flags.String("confirm-manifest-hash", "", "SHA256 of exact approved manifest file")
	execute := flags.Bool("execute", false, "explicit paid-mode opt in; never enabled by prepare or resume")
	runID := flags.Int64("run-id", 0, "existing authorized run for GET-only recovery")
	itemID := flags.Int64("item-id", 0, "single claimed intent for GET-only recovery")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !slices.Contains([]string{"prepare", "execute", "resume"}, *mode) {
		return errors.New("INVALID_COMMAND")
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
	engine := service.DFLOPVerificationEngine{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	switch *mode {
	case "prepare":
		if *user <= 0 || *output == "" {
			return errors.New("PREPARE_REQUIRES_USER_AND_OUTPUT")
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
		if plan.CatalogHash != manifest.CatalogHash || plan.CredentialFingerprint != manifest.CredentialFingerprint {
			return errors.New("DRIFT_REVIEW_REQUIRED")
		}
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
