// dflop-canary is a local, read-only evidence planner. Paid transport is not
// installed in this phase; even fully specified execute flags cannot send it.
package main

import (
	"context"
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
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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
	caseID := flags.String("case", "", "single canary case")
	maxUSD := flags.String("max-cost-usd", "", "single invocation USD ceiling")
	execute := flags.Bool("execute", false, "explicit paid execution gate")
	confirm := flags.Bool("confirm-paid-canary", false, "explicit paid canary confirmation")
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
			if common.Unmarshal(body, &envelope) == nil && len(envelope.Response) > 0 {
				body = envelope.Response
			}
			_ = common.Unmarshal(body, &reported)
			for _, plannedCase := range plan.Cases {
				if plannedCase.ID == *caseID {
					if err := dflop.CompareCanaryProviderPoints(plannedCase, &result, reported.Points.String()); err != nil {
						result.CostStatus = err.Error()
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
		if common.Unmarshal(body, &envelope) == nil && len(envelope.Response) > 0 {
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
	if mode != "plan" && mode != "execute" {
		return errors.New("mode must be plan, execute, capture, or verify")
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
	_, key, err := dflop.LoadSourceChannel(*channelID)
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
		// No billable transport exists. A future phase must re-fetch and pass a
		// separately reviewed executor through CheckCanaryAuthorization.
		if err := dflop.CheckCanaryAuthorization(dflop.CanaryAuthorization{Execute: *execute, CaseID: *caseID, MaxCostUSD: *maxUSD, ConfirmPaid: *confirm}, *selected, *selected); err != nil {
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
