package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"

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
	return ensureBillingReservation(c, info)
}

func EnsureDFLOPTaskReservation(c *gin.Context, info *relaycommon.RelayInfo) error {
	if !DFLOPTaskReservationApplies(info) {
		return nil
	}
	return ensureBillingReservation(c, info)
}

func ensureBillingReservation(c *gin.Context, info *relaycommon.RelayInfo) error {
	if common.BatchUpdateEnabled {
		return errors.New("BILLING_JOURNAL_BATCH_UNSUPPORTED")
	}
	target := info.PriceData.QuotaToPreConsume
	if DFLOPTaskReservationApplies(info) {
		target = info.PriceData.Quota
	}
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

func DFLOPTaskReservationApplies(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	if info.DFLOPTaskPlugin != "dflop-media" && info.DFLOPTaskPlugin != "dflop-image" && info.DFLOPTaskPlugin != "dflop-tts" {
		return false
	}
	base, err := url.Parse(info.ChannelBaseUrl)
	return err == nil && base.Scheme == "https" && base.Host == "api.dflop.top" && base.User == nil && base.RawQuery == "" && base.Fragment == "" && (base.Path == "" || base.Path == "/")
}

// PrepareDFLOPTaskBillingIdentity binds one billed operation to the caller's
// idempotency key. The key itself never enters the journal or usage logs.
func PrepareDFLOPTaskBillingIdentity(c *gin.Context, info *relaycommon.RelayInfo, plugin string) error {
	info.DFLOPTaskPlugin = plugin
	if plugin != "dflop-media" && plugin != "dflop-image" && plugin != "dflop-tts" {
		return nil
	}
	if !DFLOPTaskReservationApplies(info) {
		return errors.New("UNVERIFIED_DFLOP_TASK_ORIGIN")
	}
	key := c.Request.Header.Get("Idempotency-Key")
	if len(key) == 0 || len(key) > 200 || strings.TrimSpace(key) != key {
		return errors.New("a valid Idempotency-Key is required")
	}
	for _, char := range key {
		if char < 32 || char > 126 {
			return errors.New("a valid Idempotency-Key is required")
		}
	}
	identity := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", info.UserId, plugin, key)))
	info.RequestId = fmt.Sprintf("%x", identity)
	c.Set(common.RequestIdKey, info.RequestId)
	info.ForcePreConsume = true
	return nil
}

// HoldDFLOPTaskReservation records uncertain provider acceptance without
// converting an estimate to final usage or automatically refunding a paid POST.
func HoldDFLOPTaskReservation(info *relaycommon.RelayInfo, reason string) error {
	if !DFLOPTaskReservationApplies(info) || !info.DFLOPTaskOutbound {
		return nil
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok || session == nil {
		return errors.New("task billing reservation unavailable")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.settled || session.refunded {
		return nil
	}
	session.quarantined = true
	if session.reservationLogID == 0 {
		return errors.New("task billing journal unavailable")
	}
	taskID := ""
	if info.TaskRelayInfo != nil {
		taskID = info.PublicTaskID
	}
	return session.transitionReservation("PENDING", map[string]any{"reason": reason, "accepted": info.DFLOPTaskAccepted, "http_status": info.DFLOPTaskHTTPStatus, "upstream_task_id": info.DFLOPTaskUpstreamTaskID, "task_id": taskID})
}

// LinkDFLOPTaskReservation preserves the accepted provider task identity in the
// reservation journal before the HTTP request can finish or its session vanish.
func LinkDFLOPTaskReservation(info *relaycommon.RelayInfo, task *model.Task) error {
	if !DFLOPTaskReservationApplies(info) {
		return nil
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok || session == nil {
		return errors.New("task billing reservation unavailable")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.transitionReservation("HELD", map[string]any{"task_record": task.ID, "task_id": task.TaskID, "upstream_task_id": task.PrivateData.UpstreamTaskID})
}
