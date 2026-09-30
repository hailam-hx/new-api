package service

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/shopspring/decimal"
)

type CanaryReplayResult struct {
	CaseID         string         `json:"case_id"`
	Status         string         `json:"status"`
	ExpectedPoints string         `json:"expected_points,omitempty"`
	Quota          int            `json:"quota"`
	MatchedTier    string         `json:"matched_tier,omitempty"`
	LogOther       map[string]any `json:"consume_log_other,omitempty"`
	Note           string         `json:"note"`
}

// ReplayCanaryFixture uses the same billing evaluator and log projections as
// live task/text settlement. The expression is a transient source-cost probe,
// not a saved customer selling-price expression or a promotion decision.
func ReplayCanaryFixture(c dflop.CanaryCase, raw []byte, pointsPerCNY, cnyToUSD string) (CanaryReplayResult, error) {
	verified, err := dflop.VerifyCanaryFixture(c.ID, raw)
	if err != nil {
		return CanaryReplayResult{}, err
	}
	output := CanaryReplayResult{CaseID: c.ID, Status: verified.Status, Note: "Synthetic/offline path only; provider identity and production route binding remain unverified"}
	if verified.Status != "RUNTIME_FACT_VERIFIED" {
		return output, nil
	}
	points, err := decimal.NewFromString(pointsPerCNY)
	rate, rateErr := decimal.NewFromString(cnyToUSD)
	if err != nil || rateErr != nil || !points.IsPositive() || !rate.IsPositive() {
		return output, errors.New("CANARY_CURRENCY_INVALID")
	}
	if err := dflop.CompareCanaryProviderPoints(c, &verified, ""); err != nil && !strings.HasPrefix(c.ID, "grok-tool-") {
		return output, err
	}
	output.ExpectedPoints = verified.ExpectedPoints
	other := model.NewLogOther()
	if strings.HasPrefix(c.ID, "grok-tool-") {
		var payload struct {
			Usage    dto.Usage `json:"usage"`
			Response struct {
				Usage dto.Usage `json:"usage"`
			} `json:"response"`
		}
		var capture dflop.CanaryCapture
		if common.Unmarshal(raw, &capture) == nil && capture.CaseID != "" && len(capture.Response) > 0 {
			raw = capture.Response
		}
		if err := common.Unmarshal(raw, &payload); err != nil {
			return output, err
		}
		usage := payload.Usage
		if c.ID == "grok-tool-responses" {
			usage = payload.Response.Usage
		}
		var billingUsage *dto.BillingUsage
		if c.ID == "grok-tool-responses" {
			billingUsage = dto.NewOpenAIResponsesBillingUsage(&usage)
		} else {
			billingUsage = dto.NewOpenAIChatBillingUsage(&usage)
		}
		canonical, ok := billingUsage.CanonicalUsage()
		if !ok || canonical.NumServerSideToolsUsed == nil {
			return output, errors.New("CANARY_USAGE_MISSING")
		}
		params := BuildTieredTokenParams(canonical, false, map[string]bool{"st": true, "cr": true})
		inputPrice, inputErr := decimal.NewFromString(c.PriceComponentsPoints["input_per_1m"])
		outputPrice, outputErr := decimal.NewFromString(c.PriceComponentsPoints["output_per_1m"])
		toolPrice, toolErr := decimal.NewFromString(c.PriceComponentsPoints["price_per_server_tool_call"])
		cachePrice := decimal.Zero
		if rawCache, ok := c.PriceComponentsPoints["cached_input_per_1m"]; ok {
			cachePrice, err = decimal.NewFromString(rawCache)
			if err != nil {
				return output, errors.New("CANARY_PRICE_INVALID")
			}
		}
		if inputErr != nil || outputErr != nil || toolErr != nil {
			return output, errors.New("CANARY_PRICE_INVALID")
		}
		if canonical.PromptTokensDetails.CachedTokens < 0 || canonical.PromptTokensDetails.CachedTokens > canonical.PromptTokens {
			return output, errors.New("CANARY_USAGE_INVALID")
		}
		inputUSD := inputPrice.DivRound(points, 24).Mul(rate)
		cacheUSD := cachePrice.DivRound(points, 24).Mul(rate)
		outputUSD := outputPrice.DivRound(points, 24).Mul(rate)
		toolUSDScaled := toolPrice.DivRound(points, 24).Mul(rate).Mul(decimal.NewFromInt(1000000))
		expr := `tier("canary", p * ` + inputUSD.String() + ` + cr * ` + cacheUSD.String() + ` + c * ` + outputUSD.String() + ` + st * ` + toolUSDScaled.String() + `)`
		snap := &billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1}
		settled, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			return output, err
		}
		other.SetPublic("billing_mode", "tiered_expr")
		other.SetPublic("expr_b64", base64.StdEncoding.EncodeToString([]byte(expr)))
		other.SetPublic("matched_tier", settled.MatchedTier)
		AppendServerToolLogInfo(other, canonical)
		output.Quota, output.MatchedTier, output.LogOther = settled.ActualQuotaAfterGroup, settled.MatchedTier, other.Snapshot()
		toolCount := decimal.NewFromInt(int64(*canonical.NumServerSideToolsUsed))
		inputCount := decimal.NewFromInt(int64(canonical.PromptTokens - canonical.PromptTokensDetails.CachedTokens))
		cacheCount := decimal.NewFromInt(int64(canonical.PromptTokensDetails.CachedTokens))
		outputCount := decimal.NewFromInt(int64(canonical.CompletionTokens))
		output.ExpectedPoints = inputPrice.Mul(inputCount).DivRound(decimal.NewFromInt(1000000), 24).Add(cachePrice.Mul(cacheCount).DivRound(decimal.NewFromInt(1000000), 24)).Add(outputPrice.Mul(outputCount).DivRound(decimal.NewFromInt(1000000), 24)).Add(toolPrice.Mul(toolCount)).String()
		return output, nil
	}
	var factName string
	var quantity float64
	switch c.ID {
	case "tts-async":
		factName = "characters"
		quantity = float64(verified.ObservedFacts["characters"].(int64))
	case "suno-generation":
		factName = "generations"
		quantity = 1
	default:
		return output, errors.New("CANARY_REPLAY_UNSUPPORTED")
	}
	unitPrice, err := decimal.NewFromString(c.UnitPricePoints)
	if err != nil {
		return output, err
	}
	unitUSD := unitPrice.DivRound(points, 24).Mul(rate)
	expr := `tier("canary", u("` + factName + `") * ` + unitUSD.String() + `)`
	snap := &billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: map[string]any{factName: quantity}}
	settled, facts, err := EvaluateTaskCompletionUsage(snap, map[string]any{factName: quantity})
	if err != nil {
		return output, err
	}
	snap.UsageFacts, snap.EstimatedTier = facts, settled.MatchedTier
	AppendTaskExpressionLogInfo(other, snap)
	output.Quota, output.MatchedTier, output.LogOther = settled.ActualQuotaAfterGroup, settled.MatchedTier, other.Snapshot()
	return output, nil
}
