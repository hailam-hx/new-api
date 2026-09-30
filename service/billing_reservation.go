package service

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/pricing/dflop"
	"github.com/gin-gonic/gin"
)

// EnsureDFLOPCacheReservation is the final outbound barrier, after routing and
// overrides. It reserves through BillingSession and durably records the hold.
func EnsureDFLOPCacheReservation(c *gin.Context, info *relaycommon.RelayInfo) error {
	if info == nil || info.ChannelMeta == nil || !dflop.DFLOPCacheContractApplies(info.ChannelBaseUrl, info.UpstreamModelName) {
		return nil
	}
	if common.BatchUpdateEnabled {
		return errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED")
	}
	target := info.PriceData.QuotaToPreConsume
	if info.TieredBillingSnapshot != nil {
		target = info.TieredBillingSnapshot.EstimatedQuotaAfterGroup
	}
	if info.Billing == nil {
		info.ForcePreConsume = true
		if err := PreConsumeBilling(c, target, info); err != nil {
			return err
		}
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok {
		return errors.New("durable billing reservation unavailable")
	}
	session.mu.Lock()
	if session.quarantined || session.settled || session.refunded {
		session.mu.Unlock()
		return errors.New("billing reservation is closed")
	}
	session.trusted = false
	if wallet, ok := session.funding.(*WalletFunding); ok {
		wallet.durable = true
	}
	session.mu.Unlock()
	if err := session.Reserve(target); err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.reservationLogID != 0 {
		return session.transitionReservation("HELD", nil)
	}
	if info.RequestId == "" {
		info.RequestId = c.GetString(common.RequestIdKey)
	}
	if info.RequestId == "" {
		info.RequestId = common.NewRequestId()
		c.Set(common.RequestIdKey, info.RequestId)
	}
	other := session.reservationMetadata("HELD", nil)
	log := model.Log{UserId: info.UserId, TokenId: info.TokenId, ChannelId: info.ChannelId, ModelName: info.OriginModelName, RequestId: info.RequestId, CreatedAt: common.GetTimestamp(), Type: model.LogTypeSystem, Content: "BILLING_RESERVATION_HELD", Other: other.JSONString()}
	if err := model.CreateBillingReservationLog(&log); err != nil {
		return err
	}
	session.reservationLogID, session.reservationState = log.Id, "HELD"
	return nil
}

func (s *BillingSession) reservationMetadata(state string, facts map[string]any) *model.LogOther {
	other := model.NewLogOther()
	other.SetPublic("billing_state", state)
	other.SetPublic("settlement_verified", state == "SETTLED")
	info := s.relayInfo
	detail := map[string]any{"reserved_quota": s.preConsumedQuota, "token_reserved_quota": s.tokenConsumed, "billing_source": info.BillingSource, "subscription_id": info.SubscriptionId, "client_model": info.OriginModelName, "upstream_model": info.UpstreamModelName, "channel_id": info.ChannelId, "request_id": info.RequestId, "upstream_request_id": info.PassiveRequestID, "x_gateway_trace": info.PassiveTraceID, "usage": facts}
	if snap := info.TieredBillingSnapshot; snap != nil {
		detail["expression"] = snap.ExprString
		detail["expression_hash"] = snap.ExprHash
		detail["group_ratio"] = snap.GroupRatio
	}
	other.SetAdmin("billing_anomaly", detail)
	return other
}

func (s *BillingSession) transitionReservation(state string, facts map[string]any) error {
	if s.reservationLogID == 0 {
		return nil
	}
	if err := model.TransitionBillingReservationLog(s.reservationLogID, s.reservationState, state, s.reservationMetadata(state, facts)); err != nil {
		return err
	}
	s.reservationState = state
	return nil
}

func (s *BillingSession) Quarantine(facts map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reservationState == "QUARANTINED" {
		return nil
	}
	s.quarantined = true
	if s.reservationLogID == 0 {
		return errors.New("billing quarantine has no durable reservation")
	}
	return s.transitionReservation("QUARANTINED", facts)
}

func (s *BillingSession) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settled || s.refunded || s.quarantined
}
