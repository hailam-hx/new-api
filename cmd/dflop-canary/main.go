// dflop-canary prepares and audits local canary evidence. Paid execution
// remains disconnected from the CLI.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type auditProposal struct {
	Status                string   `json:"status"`
	InvocationID          string   `json:"invocation_id"`
	CaseID                string   `json:"case_id"`
	ChannelID             int      `json:"channel_id"`
	CredentialFingerprint string   `json:"credential_fingerprint"`
	CatalogHash           string   `json:"catalog_hash"`
	Model                 string   `json:"model"`
	BillingFeatures       []string `json:"billing_features"`
	RequestHash           string   `json:"request_hash"`
	MaxCostPoints         string   `json:"max_cost_points"`
	MaxCostUSD            string   `json:"max_cost_usd"`
}

type auditReport struct {
	Mode                          string              `json:"mode"`
	Proposal                      auditProposal       `json:"proposal"`
	CatalogETag                   string              `json:"catalog_etag"`
	SchemaVersion                 string              `json:"schema_version"`
	PointsPerCNY                  string              `json:"points_per_cny"`
	CNYToUSD                      string              `json:"cny_to_usd"`
	Balance                       dflop.CanaryBalance `json:"balance"`
	PreviousCatalogHash           string              `json:"previous_catalog_hash,omitempty"`
	PreviousCredentialFingerprint string              `json:"previous_credential_fingerprint,omitempty"`
	RecoveryDirectoryWritable     bool                `json:"recovery_directory_writable"`
	RedactionPassed               bool                `json:"redaction_passed"`
	Blockers                      []string            `json:"blockers"`
}

func run(args []string) error {
	mode := "plan"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		mode, args = strings.ToLower(args[0]), args[1:]
	}
	flags := flag.NewFlagSet("dflop-canary", flag.ContinueOnError)
	channelID := flags.Int("channel-id", 0, "existing DFLOP channel ID")
	manualRate := flags.String("cny-to-usd", "", "current CNY-to-USD reporting rate when pricing sync has no configured rate")
	dbPath := flags.String("sqlite-db", "one-api.db", "local SQLite database path")
	output := flags.String("output", filepath.Join(os.TempDir(), "new-api-dflop-canary", "dflop-canary-plan.json"), "plan output path")
	auditOutput := flags.String("audit-output", filepath.Join(os.TempDir(), "new-api-dflop-canary", "dflop-canary-audit.json"), "non-secret audit proposal path")
	caseID := flags.String("case", "", "single canary case")
	maxUSD := flags.String("max-cost-usd", "", "single invocation USD ceiling")
	maxPoints := flags.String("max-cost-points", "", "single invocation provider points ceiling")
	execute := flags.Bool("execute", false, "explicit paid execution gate")
	confirm := flags.String("confirm-paid-canary", "", "invocation-bound paid canary confirmation")
	dryRun := flags.Bool("dry-run", false, "render a redacted request without submitting")
	input := flags.String("input", "", "offline redacted response JSON")
	planPath := flags.String("plan", "", "authenticated canary plan ledger for offline cost comparison")
	invocation := flags.String("invocation-id", "", "single invocation ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if mode == "verify" {
		if *input == "" || *caseID == "" {
			return errors.New("verify requires --input and --case")
		}
		body, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		result, err := dflop.VerifyCanaryFixture(*caseID, body)
		if err != nil {
			return err
		}
		if *planPath != "" {
			planBody, err := os.ReadFile(*planPath)
			if err != nil {
				return err
			}
			var plan dflop.CanaryPlan
			if err := common.Unmarshal(planBody, &plan); err != nil {
				return err
			}
			var reported struct {
				Points json.Number `json:"points"`
			}
			var envelope dflop.CanaryCapture
			if common.Unmarshal(body, &envelope) == nil && envelope.CaseID != "" && len(envelope.Response) > 0 {
				body = envelope.Response
			}
			_ = common.Unmarshal(body, &reported)
			for _, plannedCase := range plan.Cases {
				if plannedCase.ID == *caseID {
					if err := dflop.CompareCanaryProviderPoints(plannedCase, &result, reported.Points.String()); err != nil {
						result.CostStatus = err.Error()
					}
					replay, err := service.ReplayCanaryFixture(plannedCase, body, plan.PointsPerCNY, plan.CNYToUSD)
					if err == nil {
						result.ProductionReplay = replay
					} else {
						result.Note += "; replay: " + err.Error()
					}
					break
				}
			}
		}
		encoded, err := common.Marshal(result)
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	if mode == "capture" {
		if *input == "" || *invocation == "" || *caseID == "" || *channelID <= 0 {
			return errors.New("capture requires --input, --invocation-id, --case and --channel-id")
		}
		body, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		capture := dflop.CanaryCapture{CaseID: *caseID, ChannelID: *channelID, Response: body}
		var envelope dflop.CanaryCapture
		if common.Unmarshal(body, &envelope) == nil && envelope.CaseID != "" && len(envelope.Response) > 0 {
			if envelope.CaseID != *caseID || (envelope.ChannelID != 0 && envelope.ChannelID != *channelID) {
				return errors.New("capture metadata does not match selected case/channel")
			}
			capture = envelope
		}
		db, err := openCanaryDB(*dbPath)
		if err != nil {
			return err
		}
		model.DB = db
		_, credential, err := dflop.LoadSourceChannel(*channelID)
		if err != nil {
			return err
		}
		path, err := dflop.WriteCanaryCapture(filepath.Join(os.TempDir(), "new-api-dflop-canary", "captures"), *invocation, capture, credential)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}
	if mode != "plan" && mode != "audit" && mode != "execute" {
		return errors.New("mode must be plan, audit, execute, capture, or verify")
	}
	if *channelID <= 0 {
		return errors.New("select an existing channel with --channel-id; implicit selection is disabled")
	}
	db, err := openCanaryDB(*dbPath)
	if err != nil {
		return err
	}
	model.DB = db
	var option model.Option
	result := db.Where(map[string]any{"key": model.DFLOPConfigOption}).Limit(1).Find(&option)
	if result.Error != nil {
		return fmt.Errorf("DFLOP pricing configuration unavailable: %w", result.Error)
	}
	config := model.DefaultDFLOPConfig()
	if result.RowsAffected > 0 {
		if err := common.UnmarshalJsonStr(option.Value, &config); err != nil {
			return err
		}
	}
	if *manualRate != "" {
		config.CNYToUSD = *manualRate
	}
	if err := config.ValidateForPreview(); err != nil {
		return err
	}
	source, key, err := dflop.LoadSourceChannel(*channelID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := dflop.Client{}
	catalog, err := client.FetchEffective(ctx, key, "")
	if err != nil {
		return err
	}
	currency, err := client.FetchCurrency(ctx)
	if err != nil {
		return err
	}
	plan, err := dflop.BuildCanaryPlan(*channelID, catalog.Body, currency, catalog.ETag, config.CNYToUSD)
	if err != nil {
		return err
	}
	if mode == "audit" {
		var selected *dflop.CanaryCase
		for i := range plan.Cases {
			if plan.Cases[i].ID == "tts-async" {
				selected = &plan.Cases[i]
				break
			}
		}
		if selected == nil {
			return errors.New("CANARY_TTS_NOT_CALLABLE")
		}
		request, err := dflop.BuildCanaryRequest(*selected)
		if err != nil {
			return err
		}
		balance, err := (dflop.CanaryTransport{BaseURL: source.BaseURL, Key: key}).FetchCanaryBalance(ctx)
		if err != nil {
			return err
		}
		id, err := dflop.NewCanaryInvocationID()
		if err != nil {
			return err
		}
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
		audit := auditReport{
			Mode: "AUDIT",
			Proposal: auditProposal{
				Status:                "PROPOSAL_NOT_AUTHORIZATION",
				InvocationID:          id,
				CaseID:                selected.ID,
				ChannelID:             *channelID,
				CredentialFingerprint: fingerprint,
				CatalogHash:           plan.CatalogHash,
				Model:                 selected.Model,
				BillingFeatures:       selected.BillingFeatures,
				RequestHash:           request.RequestHash,
				MaxCostPoints:         selected.EstimatedMaxPoints,
				MaxCostUSD:            selected.EstimatedMaxUSD,
			},
			CatalogETag:   plan.ETag,
			SchemaVersion: plan.SchemaVersion,
			PointsPerCNY:  plan.PointsPerCNY,
			CNYToUSD:      plan.CNYToUSD,
			Balance:       balance,
			Blockers:      []string{"EXPLICIT_OPERATOR_AUTHORIZATION_MISSING", "CANARY_EXECUTION_NOT_ENABLED", "PRODUCTION_TTS_BINDING_NOT_INTEGRATED"},
		}
		if prior, err := os.ReadFile(*auditOutput); err == nil {
			var previous auditReport
			if common.Unmarshal(prior, &previous) == nil {
				audit.PreviousCatalogHash = previous.Proposal.CatalogHash
				audit.PreviousCredentialFingerprint = previous.Proposal.CredentialFingerprint
				if previous.Proposal.CredentialFingerprint != "" && previous.Proposal.CredentialFingerprint != fingerprint {
					audit.Blockers = append(audit.Blockers, "CREDENTIAL_CHANGED_SINCE_PRIOR_AUDIT")
				}
			}
		}
		if audit.PreviousCredentialFingerprint == "" {
			audit.Blockers = append(audit.Blockers, "CREDENTIAL_BASELINE_UNAVAILABLE")
		}
		if audit.PreviousCatalogHash != "" && audit.PreviousCatalogHash != plan.CatalogHash {
			audit.Blockers = append(audit.Blockers, "CATALOG_CHANGED_SINCE_PRIOR_AUDIT")
		}
		remaining, _ := decimal.NewFromString(balance.RemainingUSD)
		needed, _ := decimal.NewFromString(selected.EstimatedMaxUSD)
		if remaining.LessThan(needed) {
			audit.Blockers = append(audit.Blockers, "CANARY_INSUFFICIENT_BALANCE")
		}
		recoveryDir := filepath.Join(os.TempDir(), "new-api-dflop-canary", "invocations")
		if err := os.MkdirAll(recoveryDir, 0700); err == nil {
			if info, err := os.Stat(recoveryDir); err == nil && info.IsDir() && info.Mode().Perm()&0077 == 0 {
				if probe, err := os.CreateTemp(recoveryDir, ".audit-"); err == nil {
					closeErr := probe.Close()
					removeErr := os.Remove(probe.Name())
					audit.RecoveryDirectoryWritable = closeErr == nil && removeErr == nil
				}
			}
		}
		if !audit.RecoveryDirectoryWritable {
			audit.Blockers = append(audit.Blockers, "CANARY_RECOVERY_DIRECTORY_UNWRITABLE")
		}
		probe, err := dflop.RedactCanaryJSON([]byte(`{"authorization":"Bearer audit-secret","audio_url":"https://media.example/secret?token=value"}`), "audit-secret")
		audit.RedactionPassed = err == nil && !strings.Contains(string(probe), "audit-secret") && !strings.Contains(string(probe), "media.example")
		if !audit.RedactionPassed {
			audit.Blockers = append(audit.Blockers, "CANARY_REDACTION_FAILED")
		}
		encoded, err := common.Marshal(audit)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*auditOutput), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(*auditOutput, encoded, 0600); err != nil {
			return err
		}
		fmt.Printf("AUDIT TTS %s; catalog %s; max %s points / %s USD; balance sufficient %t; recovery writable %t; redaction %t; proposal %s; NO PAID POST\n", selected.Model, plan.CatalogHash, selected.EstimatedMaxPoints, selected.EstimatedMaxUSD, !remaining.LessThan(needed), audit.RecoveryDirectoryWritable, audit.RedactionPassed, *auditOutput)
		return nil
	}
	encoded, err := common.Marshal(plan)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(*output, encoded, 0600); err != nil {
		return err
	}
	fmt.Printf("PLAN %d/%d directly supported before exact task bindings; %d cases; catalog %s; ledger %s\n", plan.DirectSupported, plan.CallableModels, len(plan.Cases), plan.CatalogHash, *output)
	for _, c := range plan.Cases {
		if c.Priority <= 6 {
			fmt.Printf("%s | %s | %s | %s points | %s USD | request=%s fixture=%s executor=%s | %s\n", c.ID, c.Model, c.PriceUnit, c.EstimatedMaxPoints, c.EstimatedMaxUSD, c.RequestReadiness, c.FixtureReadiness, c.ExecutorReadiness, c.Status)
		}
	}
	if *dryRun {
		if *invocation == "" {
			generated, err := dflop.NewCanaryInvocationID()
			if err != nil {
				return err
			}
			*invocation = generated
		}
		for _, c := range plan.Cases {
			if c.ID == *caseID {
				request, err := dflop.BuildCanaryRequest(c)
				if err != nil {
					return err
				}
				encoded, err := common.Marshal(request)
				if err != nil {
					return err
				}
				fmt.Println(string(encoded))
				fmt.Printf("invocation_id=%s\n", *invocation)
				return nil
			}
		}
		return errors.New("CANARY_REQUEST_UNVERIFIED")
	}
	if mode == "execute" {
		var selected *dflop.CanaryCase
		for i := range plan.Cases {
			if plan.Cases[i].ID == *caseID {
				selected = &plan.Cases[i]
				break
			}
		}
		if selected == nil {
			return errors.New("CANARY_AUTHORIZATION_REQUIRED: select a valid --case")
		}
		if err := dflop.CheckCanaryAuthorization(dflop.CanaryAuthorization{Execute: *execute, InvocationID: *invocation, CaseID: *caseID, ChannelID: *channelID, CatalogHash: plan.CatalogHash, MaxCostPoints: *maxPoints, MaxCostUSD: *maxUSD, Confirmation: *confirm}, *selected, *selected); err != nil {
			return err
		}
		return errors.New("CANARY_EXECUTION_NOT_ENABLED")
	}
	return nil
}

func openCanaryDB(dbPath string) (*gorm.DB, error) {
	// Read existing rows without InitDB, migrations, or scheduler startup.
	dsn := os.Getenv("SQL_DSN")
	switch {
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		return gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
	case dsn != "" && !strings.HasPrefix(dsn, "local"):
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	default:
		absoluteDB, err := filepath.Abs(dbPath)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(absoluteDB); err != nil {
			return nil, err
		}
		return gorm.Open(sqlite.Open("file:"+absoluteDB+"?mode=ro&_pragma=busy_timeout(30000)"), &gorm.Config{})
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
