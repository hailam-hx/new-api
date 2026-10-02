package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
)

// TaskConnectivityCapability is host-reviewed and intentionally limited to the
// three DFLOP executors. A custom/proxy origin is never a trusted catalog target.
func TaskConnectivityCapability(d *TaskChannelDiagnostic, channel *model.Channel) {
	d.ConnectivityAvailable = false
	d.ConnectivityKeyIndices = nil
	if d.Kind != "task_plugin" || !dflop.ApprovedCatalogOrigin(channel.GetBaseURL()) {
		return
	}
	switch d.Plugin {
	case "dflop-image", "dflop-media", "dflop-tts":
	default:
		return
	}
	for _, check := range d.Checks {
		if check.Status == "fail" && (check.Check == "mapping" || check.Check == "ownership" || check.Check == "plugin_load") {
			return
		}
	}
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()
	if !channel.ChannelInfo.IsMultiKey {
		if strings.TrimSpace(channel.Key) != "" && !strings.ContainsAny(channel.Key, "\r\n") {
			d.ConnectivityKeyIndices = []int{0}
		}
	} else {
		for i, key := range channel.GetKeys() {
			status, set := channel.ChannelInfo.MultiKeyStatusList[i]
			if (!set || status == common.ChannelStatusEnabled) && strings.TrimSpace(key) != "" && !strings.ContainsAny(key, "\r\n") {
				d.ConnectivityKeyIndices = append(d.ConnectivityKeyIndices, i)
			}
		}
	}
	d.ConnectivityAvailable = len(d.ConnectivityKeyIndices) > 0
}

// ProbeTaskConnectivity never enters the generation, billing or key-rotation
// path. For a multi-key channel the administrator must choose one enabled index;
// the result certifies only that index, not every key or the next random choice.
func ProbeTaskConnectivity(ctx context.Context, d *TaskChannelDiagnostic, channel *model.Channel, keyIndex *int, client dflop.Client) {
	d.Mode = "connectivity"
	TaskConnectivityCapability(d, channel)
	status := "connectivity_unavailable"
	if !d.ConnectivityAvailable {
		attachConnectivityResult(d, status, "")
		return
	}
	index := 0
	if keyIndex != nil {
		index = *keyIndex
	} else if channel.ChannelInfo.IsMultiKey {
		attachConnectivityResult(d, status, "credential_selection_required")
		return
	}
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	key := channel.Key
	enabled := index == 0
	if channel.ChannelInfo.IsMultiKey {
		keys := channel.GetKeys()
		enabled = index >= 0 && index < len(keys)
		if enabled {
			key = keys[index]
			state, set := channel.ChannelInfo.MultiKeyStatusList[index]
			enabled = !set || state == common.ChannelStatusEnabled
		}
	}
	lock.Unlock()
	if !enabled || strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n") {
		attachConnectivityResult(d, status, "credential_selection_invalid")
		return
	}
	d.CredentialIdentity = fmt.Sprintf("key_index:%d", index)
	start := time.Now()
	d.ConnectivityTested = true
	response, err := client.FetchConnectivityCatalog(ctx, key)
	var networkErr net.Error
	d.ConnectivityLatencyMS = time.Since(start).Milliseconds()
	switch {
	case response.HTTPStatus == 401 || response.HTTPStatus == 403:
		status = "auth_failed"
	case response.HTTPStatus == 429:
		status = "upstream_rate_limited"
	case response.HTTPStatus >= 500:
		status = "upstream_service_error"
	case response.HTTPStatus == 0:
		status = "upstream_unreachable"
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &networkErr):
		status = "upstream_unreachable"
	case err != nil:
		status = "connectivity_response_invalid"
	default:
		status = "connectivity_pass"
		var catalog struct {
			SchemaVersion string `json:"schema_version"`
			Models        []struct {
				ID string `json:"id"`
			} `json:"models"`
			Aliases map[string]string `json:"aliases"`
		}
		// RawMessage distinguishes missing/null fields from an empty valid collection.
		var fields map[string]json.RawMessage
		if common.Unmarshal(response.Body, &fields) != nil || fields == nil || common.Unmarshal(response.Body, &catalog) != nil || catalog.SchemaVersion != "1.0" || catalog.Models == nil || catalog.Aliases == nil {
			status = "connectivity_response_invalid"
		} else {
			ids := make(map[string]bool, len(catalog.Models))
			for _, entry := range catalog.Models {
				if strings.TrimSpace(entry.ID) == "" || ids[entry.ID] {
					status = "connectivity_response_invalid"
					break
				}
				ids[entry.ID] = true
			}
			if status == "connectivity_pass" {
				// Catalog IDs and its authoritative alias map, after production model mapping.
				candidate := d.MappedModel
				seen := map[string]bool{}
				for catalog.Aliases[candidate] != "" && !ids[candidate] && !seen[candidate] {
					seen[candidate] = true
					candidate = catalog.Aliases[candidate]
				}
				d.CatalogModel = candidate
				d.ModelAccess = "not_confirmed"
				if ids[candidate] {
					d.ModelAccess = "confirmed"
				}
			}
		}
	}
	attachConnectivityResult(d, status, "")
	if status == "connectivity_pass" {
		// Conversion factors do not affect catalog presence/protocol/contract checks.
		// No computed price is adopted or persisted by this diagnostic.
		if items, _, meta, err := dflop.BuildEffective(response.Body, []byte(`{"unit":"points","points_per_cny":60}`), "1", "1"); err == nil {
			d.AttachAuthenticatedCatalog(items, meta, fmt.Sprintf("%x", sha256.Sum256(response.Body)))
		} else {
			d.Checks = append(d.Checks, TaskDiagnosticCheck{Check: "provider_contract", Status: "not_tested", Reason: "INSUFFICIENT_EVIDENCE", Message: err.Error(), Evidence: fmt.Sprintf("%x", sha256.Sum256(response.Body))})
		}
	}

}

func attachConnectivityResult(d *TaskChannelDiagnostic, status, reason string) {
	// Replace only the Phase-A connectivity placeholder. Preserve configuration,
	// credential-structure and generation checks, including the first pricing fail.
	checks := d.Checks[:0]
	for _, check := range d.Checks {
		if check.Check != "connectivity" && check.Check != "model_access" {
			checks = append(checks, check)
		}
	}
	d.Checks = checks
	d.ConnectivityStatus = status
	check := TaskDiagnosticCheck{Check: "connectivity", Status: "not_tested", Reason: status}
	if reason != "" {
		check.Reason = reason
	}
	switch status {
	case "connectivity_pass":
		check.Status = "pass"
		check.Reason = ""
		check.Evidence = "catalog"
	case "auth_failed", "upstream_unreachable", "upstream_service_error", "connectivity_response_invalid":
		check.Status = "fail"
		d.Fail("connectivity", status, status)
		if d.ConnectivityTested {
			check.Evidence = "catalog"
		}
		d.Checks[len(d.Checks)-1] = check
		return
	}
	if d.ConnectivityTested {
		check.Evidence = "catalog"
	}
	d.Checks = append(d.Checks, check)
	if status == "connectivity_pass" {
		access := TaskDiagnosticCheck{Check: "model_access", Status: "not_tested", Reason: "model_access_not_confirmed", Evidence: "catalog"}
		if d.ModelAccess == "confirmed" {
			access.Status = "pass"
			access.Reason = ""
		}
		d.Checks = append(d.Checks, access)
	}
	if d.Outcome != "fail" {
		d.Outcome = "partial"
		d.Status = status
		if d.ModelAccess == "not_confirmed" && status == "connectivity_pass" {
			d.Status = "model_access_not_confirmed"
		}
	}
}
