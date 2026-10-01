package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
)

// runProviderContractAudit reads provider evidence only. It does not call
// Preview, Apply, adoption, canary execution, or database migration code.
func runProviderContractAudit(ctx context.Context, client dflop.Client, channelID int, key, baselinePath, output string) error {
	var previous []byte
	if baselinePath != "" {
		var err error
		previous, err = os.ReadFile(baselinePath)
		if err != nil {
			return err
		}
	}
	catalog, err := client.FetchEffective(ctx, key, "")
	if err != nil {
		return err
	}
	public, err := client.FetchPublic(ctx)
	if err != nil {
		return err
	}
	audit, err := dflop.EvaluateProviderContracts(catalog.Body, previous)
	if err != nil {
		return err
	}
	// Redact the source objects before embedding them, so report field names
	// and non-secret metadata cannot be mistaken for credential-bearing fields.
	safeCatalog, err := dflop.RedactCanaryJSON(catalog.Body, key)
	if err != nil {
		return err
	}
	safePublic, err := dflop.RedactCanaryJSON(public.Body, key)
	if err != nil {
		return err
	}
	safeAudit, err := common.Marshal(audit)
	if err != nil {
		return err
	}
	safeAudit, err = dflop.RedactCanaryJSON(safeAudit, key)
	if err != nil {
		return err
	}
	var source struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := common.Unmarshal(catalog.Body, &source); err != nil {
		return err
	}
	music := map[string]dflop.MusicContractWatch{}
	for _, raw := range source.Models {
		var entry struct {
			ID string `json:"id"`
		}
		if err := common.Unmarshal(raw, &entry); err != nil {
			return err
		}
		if strings.HasPrefix(entry.ID, "suno-") {
			music[entry.ID] = dflop.CheckMusicGenerationContract(raw, dflop.MusicGenerationContract{BillingUnit: "generation", SongsPerGeneration: 2, Source: "https://model.dflop.top/en/docs/reference/media-apis"})
		}
	}
	// The explicit safety flags describe this command, independently of any
	// baseline status. A catalog candidate is evidence, never an apply decision.
	var report map[string]any
	if err := common.Unmarshal(safeAudit, &report); err != nil {
		return err
	}
	report["mode"] = "PROVIDER_CONTRACT_GET_ONLY"
	report["source_channel"] = channelID
	report["fetched_at"] = time.Unix(catalog.FetchedAt, 0).UTC().Format(time.RFC3339)
	report["catalog_http_status"] = catalog.HTTPStatus
	report["public_http_status"] = public.HTTPStatus
	report["catalog_snapshot"] = json.RawMessage(safeCatalog)
	report["public_catalog_snapshot"] = json.RawMessage(safePublic)
	report["music_contract_watch"] = music
	report["fresh_preview_required"] = true
	report["auto_promotes"] = false
	report["pricing_applied"] = false
	report["auto_apply"] = false
	report["paid_requests_executed"] = false
	report["next_steps"] = []string{"detect", "parse", "review compatibility evidence", "run tests", "fresh Pricing Preview on DB copy", "admin review", "explicit manual apply if appropriate"}
	body, err := common.Marshal(report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	// Atomic replacement keeps a failed audit from destroying the prior report.
	file, err := os.CreateTemp(filepath.Dir(output), ".provider-contract-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), output); err != nil {
		return err
	}
	fmt.Printf("PROVIDER_CONTRACT_GET_ONLY catalog=%s schema=%s callable=%d; fresh Preview required; NO AUTO PROMOTION; %s\n", audit.CatalogHash, audit.SchemaVersion, audit.CallableCount, output)
	for _, blocker := range audit.Blockers {
		fmt.Printf("%s | %s\n", blocker.ID, blocker.Status)
	}
	return nil
}
