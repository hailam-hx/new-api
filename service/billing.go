package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if relayInfo != nil && relayInfo.ChannelMeta != nil && dflop.DFLOPCacheContractApplies(relayInfo.ChannelBaseUrl, relayInfo.UpstreamModelName) {
		if common.BatchUpdateEnabled {
			return types.NewError(errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		if relayInfo.Billing != nil {
			return nil
		}
		if relayInfo.RequestId == "" {
			relayInfo.RequestId = c.GetString(common.RequestIdKey)
		}
		record, err := model.FindBillingReservationLog(relayInfo.RequestId, relayInfo.UserId)
		if err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		if record != nil {
			return types.NewError(errors.New("BILLING_RESERVATION_REPLAY_REQUIRES_MANUAL_RESOLUTION"), types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
	}
	taskJournal := DFLOPTaskReservationApplies(relayInfo)
	if taskJournal && relayInfo.Billing != nil {
		return nil
	}
	if taskJournal {
		if common.BatchUpdateEnabled {
			return types.NewError(errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		if relayInfo.RequestId == "" {
			return types.NewError(errors.New("task billing identity missing"), types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
		journal := model.Log{UserId: relayInfo.UserId, TokenId: relayInfo.TokenId, ChannelId: relayInfo.ChannelId, ModelName: relayInfo.OriginModelName, RequestId: relayInfo.RequestId, CreatedAt: common.GetTimestamp(), Type: model.LogTypeSystem, Content: "BILLING_RESERVATION_CLAIMED", Other: `{"billing_state":"CLAIMED","settlement_verified":false}`}
		if err := model.CreateBillingReservationLog(&journal); err != nil {
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
		relayInfo.DFLOPTaskReservationLogID = journal.Id
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	if taskJournal {
		session.mu.Lock()
		err := session.transitionReservation("HELD", nil)
		if err != nil {
			session.quarantined = true
		}
		session.mu.Unlock()
		if err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}
	return nil
}
