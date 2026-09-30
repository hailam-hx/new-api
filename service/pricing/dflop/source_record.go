package dflop

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type SourceRecord struct {
	SourceMode            string            `json:"source_mode"`
	SourceChannel         SourceChannel     `json:"source_channel"`
	CredentialFingerprint string            `json:"credential_fingerprint,omitempty"`
	CatalogSchemaVersion  string            `json:"catalog_schema_version"`
	Effective             EffectiveResponse `json:"effective"`
	Public                EffectiveResponse `json:"public"`
	EffectiveHash         string            `json:"effective_hash"`
	PublicHash            string            `json:"public_hash"`
	EffectiveCount        int               `json:"effective_count"`
	PublicCount           int               `json:"public_count"`
	PublicCallableCount   int               `json:"public_callable_count"`
	PublicOnlyCount       int               `json:"public_only_count"`
	EffectiveOnlyCount    int               `json:"effective_only_count"`
	PublicSchemaVersion   string            `json:"public_schema_version,omitempty"`
	CallableCount         int               `json:"callable_count"`
	Integrity             []string          `json:"integrity"`
	Policy                IntegrityPolicy   `json:"policy"`
	Currency              []byte            `json:"currency"`
}

type IntegrityPolicy struct {
	ManualBlockReason          string               `json:"manual_block_reason,omitempty"`
	AutoBlockReason            string               `json:"auto_block_reason,omitempty"`
	ManualConfirmationRequired bool                 `json:"manual_confirmation_required"`
	Conditions                 []IntegrityCondition `json:"conditions,omitempty"`
}

type IntegrityCondition struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
}

// EvaluateSourceIntegrity is the single apply policy for catalog conditions.
// A public registry warning cannot alter the authenticated pricing proposal.
func EvaluateSourceIntegrity(record SourceRecord) IntegrityPolicy {
	policy := IntegrityPolicy{}
	if record.SourceMode != "AUTHENTICATED_EFFECTIVE" {
		policy.ManualBlockReason = "SOURCE_CHANNEL_UNAVAILABLE"
		policy.AutoBlockReason = policy.ManualBlockReason
		policy.Conditions = []IntegrityCondition{{Code: policy.ManualBlockReason, Severity: "ALL_APPLY_BLOCK"}}
		return policy
	}
	if len(record.Integrity) == 0 {
		policy.ManualBlockReason = "INTEGRITY_STATUS_MISSING"
		policy.AutoBlockReason = policy.ManualBlockReason
		policy.Conditions = []IntegrityCondition{{Code: policy.ManualBlockReason, Severity: "ALL_APPLY_BLOCK"}}
		return policy
	}
	for _, condition := range record.Integrity {
		switch condition {
		case "HEALTHY":
			policy.Conditions = append(policy.Conditions, IntegrityCondition{Code: condition, Severity: "INFO"})
		case "PUBLIC_CATALOG_ANOMALY":
			policy.Conditions = append(policy.Conditions, IntegrityCondition{Code: condition, Severity: "AUTO_APPLY_BLOCK"})
			policy.ManualConfirmationRequired = true
			if policy.AutoBlockReason == "" {
				policy.AutoBlockReason = condition
			}
		case "UNKNOWN_BILLING_FEATURE":
			policy.Conditions = append(policy.Conditions, IntegrityCondition{Code: condition, Severity: "AUTO_APPLY_BLOCK"})
			// The affected model is blocked in its preview row. Other rows can
			// still be applied deliberately, but unattended apply stops.
			if policy.AutoBlockReason == "" {
				policy.AutoBlockReason = condition
			}
		default:
			policy.Conditions = append(policy.Conditions, IntegrityCondition{Code: condition, Severity: "ALL_APPLY_BLOCK"})
			// Unknown integrity conditions are unsafe until classified.
			if policy.ManualBlockReason == "" {
				policy.ManualBlockReason = condition
			}
			if policy.AutoBlockReason == "" {
				policy.AutoBlockReason = condition
			}
		}
	}
	if slices.Contains(record.Integrity, "MODEL_COUNT_COLLAPSE") {
		policy.ManualBlockReason = "MODEL_COUNT_COLLAPSE"
		policy.AutoBlockReason = "MODEL_COUNT_COLLAPSE"
	}
	return policy
}

func sourceDigest(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

func packSource(body []byte) (string, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(body); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes()), nil
}

func unpackSource(snapshot string) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(snapshot)
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("last-good DFLOP catalog exceeds 4 MiB")
	}
	return body, nil
}

func lastGoodSource(channelID int, fingerprint string) ([]byte, SourceRecord, error) {
	for offset := 0; ; offset += 100 {
		runs, err := model.ListPricingSyncRunsPage(100, offset)
		if err != nil {
			return nil, SourceRecord{}, err
		}
		for _, run := range runs {
			if run.SourceSnapshot == "" || run.CurrencySnapshot == "" {
				continue
			}
			var record SourceRecord
			if common.UnmarshalJsonStr(run.CurrencySnapshot, &record) != nil || record.SourceMode != "AUTHENTICATED_EFFECTIVE" || record.SourceChannel.ID != channelID || record.CredentialFingerprint != fingerprint || record.Effective.State != "FRESH" && record.Effective.State != "NOT_MODIFIED" {
				continue
			}
			if slices.Contains(record.Integrity, "EFFECTIVE_CATALOG_ANOMALY") || slices.Contains(record.Integrity, "MODEL_COUNT_COLLAPSE") || slices.Contains(record.Integrity, "SCHEMA_VERSION_CHANGED") || slices.Contains(record.Integrity, "UNKNOWN_BILLING_FEATURE") {
				continue
			}
			body, err := unpackSource(run.SourceSnapshot)
			if err != nil || sourceDigest(body) != record.EffectiveHash {
				return nil, SourceRecord{}, errors.New("last-good DFLOP catalog snapshot is corrupt")
			}
			return body, record, nil
		}
		if len(runs) < 100 {
			return nil, SourceRecord{}, nil
		}
	}
}

func collapsed(previous, current int) bool {
	return previous > 0 && current*4 < previous*3
}
