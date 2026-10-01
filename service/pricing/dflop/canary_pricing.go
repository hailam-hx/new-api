package dflop

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// ClaudeCanaryPricingInputs describes the frozen, text-only native Messages
// settlement projection. This contract deliberately rejects other expression
// shapes rather than guessing which request or evaluator inputs they need.
// Rates come from the selected effective expression, never legacy ratio tables.
type ClaudeCanaryPricingInputs struct {
	Model         string
	BillingMode   string
	BillingExpr   string
	QuotaPerUnit  string
	GroupRatio    string
	EvaluatorHash string
}

var claudeCanaryExpression = regexp.MustCompile(`^\s*(?:v1:\s*)?tier\(\s*"dflop"\s*,\s*p\s*\*\s*([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)\s*\+\s*c\s*\*\s*([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)\s*\+\s*cr\s*\*\s*([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)\s*\+\s*cc\s*\*\s*([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)\s*\)\s*$`)

// ClaudeCanaryPricingFingerprint hashes only inputs used by the frozen v1
// evaluator. Provider catalog/config/channel credentials are separate hard
// authorization gates. Ownership, other models, and global versions are audit
// fields. common.Marshal sorts map keys; every decimal is a normalized string.
func ClaudeCanaryPricingFingerprint(input ClaudeCanaryPricingInputs) (string, []byte, error) {
	if input.Model != "claude-sonnet-5" || input.BillingMode != "tiered_expr" || len(input.EvaluatorHash) != 64 {
		return "", nil, errors.New("CANARY_PRICING_INPUTS_UNKNOWN")
	}
	parts := claudeCanaryExpression.FindStringSubmatch(input.BillingExpr)
	if len(parts) != 5 {
		return "", nil, errors.New("CANARY_EXPRESSION_INPUTS_UNKNOWN")
	}
	decimals := []string{parts[1], parts[2], parts[3], parts[4], input.QuotaPerUnit, input.GroupRatio}
	for i, value := range decimals {
		number, err := decimal.NewFromString(value)
		if err != nil || !number.IsPositive() {
			return "", nil, errors.New("CANARY_PRICING_DECIMAL_INVALID")
		}
		decimals[i] = number.String()
	}
	expression := fmt.Sprintf(`tier("dflop", p * %s + c * %s + cr * %s + cc * %s)`, decimals[0], decimals[1], decimals[2], decimals[3])
	state := map[string]any{"contract": "claude-native-text-token-v1", "model": input.Model, "billing_mode": input.BillingMode, "billing_expr": expression, "rates_usd_per_1m": map[string]string{"input": decimals[0], "output": decimals[1], "cache_read": decimals[2], "cache_creation_5m": decimals[3]}, "quota_per_unit": decimals[4], "group_ratio": decimals[5], "expr_version": 1, "task_usage_billing": false, "usage_semantic": "anthropic", "server_tool_surcharge": "0", "evaluator_hash": input.EvaluatorHash}
	body, err := common.Marshal(state)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("%x", sha256.Sum256(body)), body, nil
}

// ClaudeCanaryPricingBinding is an offline authorization policy, not an
// execution API. Existing transport guards are intentionally unchanged.
type ClaudeCanaryPricingBinding struct {
	Execute               bool
	InvocationID          string
	CatalogHash           string
	ConfigHash            string
	SourceChannelID       int
	CredentialFingerprint string
	TargetFingerprint     string
	MaxPoints             string
	GlobalPricingVersion  string
}

// CheckClaudeCanaryPricingBinding always recomputes the fresh semantic state,
// including when a global version changes. Callers must derive those inputs
// from the selected current/frozen evaluator state, not reuse an old digest.
// Invalidated invocations remain unusable even after unrelated changes.
func CheckClaudeCanaryPricingBinding(authorized, fresh ClaudeCanaryPricingBinding, inputs ClaudeCanaryPricingInputs, invocationStates map[string]string) error {
	if strings.HasPrefix(invocationStates[authorized.InvocationID], "INVALIDATED") || strings.HasPrefix(invocationStates[fresh.InvocationID], "INVALIDATED") {
		return errors.New("CANARY_INVOCATION_INVALIDATED")
	}
	if !authorized.Execute || authorized.InvocationID == "" || strings.ContainsAny(authorized.InvocationID, `/\.`) || authorized.InvocationID != fresh.InvocationID {
		return errors.New("CANARY_AUTHORIZATION_REQUIRED")
	}
	if invocationStates[authorized.InvocationID] != "AUTHORIZED" || invocationStates[fresh.InvocationID] != "AUTHORIZED" {
		return errors.New("CANARY_INVOCATION_STATE_UNKNOWN")
	}
	if authorized.SourceChannelID <= 0 || authorized.SourceChannelID != fresh.SourceChannelID || authorized.CredentialFingerprint == "" || authorized.CredentialFingerprint != fresh.CredentialFingerprint || authorized.CatalogHash == "" || authorized.CatalogHash != fresh.CatalogHash || authorized.ConfigHash == "" || authorized.ConfigHash != fresh.ConfigHash {
		return errors.New("CANARY_SOURCE_BINDING_CHANGED")
	}
	fp, _, err := ClaudeCanaryPricingFingerprint(inputs)
	if err != nil {
		return err
	}
	if fp != authorized.TargetFingerprint || fp != fresh.TargetFingerprint {
		return errors.New("CANARY_TARGET_PRICING_CHANGED_NEW_AUTHORIZATION_REQUIRED")
	}
	limit, err := decimal.NewFromString(authorized.MaxPoints)
	if err != nil || !limit.IsPositive() {
		return errors.New("CANARY_BUDGET_INVALID")
	}
	freshLimit, err := decimal.NewFromString(fresh.MaxPoints)
	if err != nil || !limit.Equal(freshLimit) {
		return errors.New("CANARY_BUDGET_CHANGED")
	}
	return nil
}
