package service

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	funding          FundingSource
	preConsumedQuota int  // 实际预扣额度（信任用户可能为 0）
	tokenConsumed    int  // 令牌额度实际扣减量
	trusted          bool // 是否命中信任额度旁路
	fundingSettled   bool // final funding ownership/settlement has committed
	settled          bool // Settle 全部完成（资金 + 令牌）
	refunded         bool // Refund 已调用
	mu               sync.Mutex
}

// Settle adjusts funding, token quota and the subscription reservation record
// in one transaction. Post-use settlement preserves the existing arrears policy.
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return nil
	}
	if s.refunded {
		return errors.New("billing session is already refunded")
	}
	if actualQuota < 0 {
		return errors.New("actual quota cannot be negative")
	}
	delta := actualQuota - s.preConsumedQuota
	if delta != 0 || s.funding.Source() == BillingSourceSubscription {
		if apiErr := s.reconcileReservation(s.preConsumedQuota, actualQuota, true); apiErr != nil {
			return apiErr
		}
	}
	s.fundingSettled = true
	s.settled = true
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	return nil
}

// Refund releases funding and token reservation in one transaction.
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		return
	}
	if err := refundWithRetry(func() error {
		if apiErr := s.reconcileReservation(s.preConsumedQuota, 0, false); apiErr != nil {
			return apiErr
		}
		return nil
	}); err != nil {
		logger.LogError(c, "failed to release billing reservation: "+err.Error())
		return
	}
	s.syncRelayInfo()
	s.refunded = true
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	if wallet, ok := s.funding.(*WalletFunding); ok && wallet.consumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度。
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preConsumedQuota
}

// Reserve reconciles a live reservation to targetQuota. The name is retained
// for callers that add request-derived media cost after initial pricing.
func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if targetQuota < 0 {
		return errors.New("reservation quota cannot be negative")
	}
	if s.settled || s.refunded {
		return errors.New("billing session is already closed")
	}
	if s.trusted && !s.relayInfo.ForcePreConsume {
		targetQuota = 0
	}
	if targetQuota == s.preConsumedQuota && s.funding.Source() != BillingSourceSubscription {
		return nil
	}
	if apiErr := s.reconcileReservation(s.preConsumedQuota, targetQuota, false); apiErr != nil {
		return apiErr
	}
	if targetQuota > 0 {
		s.trusted = false
	}
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume executes the configured trust decision and then reserves funding
// plus token quota in one primary-database transaction.
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}
	if s.trusted {
		s.preConsumedQuota = 0
		s.tokenConsumed = 0
		s.syncRelayInfo()
		return nil
	}

	if apiErr := s.reconcileReservation(0, effectiveQuota, false); apiErr != nil {
		return apiErr
	}

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) reconcileReservation(currentQuota int, targetQuota int, postConsume bool) *types.NewAPIError {
	req := model.GroupReservationRequest{
		Source:           s.funding.Source(),
		RequestId:        s.relayInfo.RequestId,
		UserId:           s.relayInfo.UserId,
		ModelName:        s.relayInfo.OriginModelName,
		SubscriptionId:   s.relayInfo.SubscriptionId,
		TokenId:          s.relayInfo.TokenId,
		TokenKey:         s.relayInfo.TokenKey,
		TokenUnlimited:   s.relayInfo.TokenUnlimited,
		SkipTokenQuota:   s.relayInfo.IsPlayground || s.relayInfo.SkipTokenQuota,
		ExpectedReserved: currentQuota,
		TargetReserved:   targetQuota,
		PostConsume:      postConsume,
	}
	result, err := model.ReconcileGroupReservation(req)
	if err != nil {
		return groupReservationError(err)
	}

	s.preConsumedQuota = result.Reserved
	if req.SkipTokenQuota {
		s.tokenConsumed = 0
	} else {
		s.tokenConsumed = result.Reserved
	}
	switch funding := s.funding.(type) {
	case *WalletFunding:
		funding.consumed = result.Reserved
	case *SubscriptionFunding:
		funding.subscriptionId = result.SubscriptionId
		funding.preConsumed = int64(result.Reserved)
		funding.AmountTotal = result.SubscriptionAmountTotal
		funding.AmountUsedAfter = result.SubscriptionAmountUsedAfter
		if funding.subscriptionId > 0 {
			if planInfo, planErr := model.GetSubscriptionPlanInfoByUserSubscriptionId(funding.subscriptionId); planErr == nil && planInfo != nil {
				funding.PlanId = planInfo.PlanId
				funding.PlanTitle = planInfo.PlanTitle
			}
		}
	}
	return nil
}

func groupReservationError(err error) *types.NewAPIError {
	switch {
	case errors.Is(err, model.ErrGroupReservationTokenInsufficient):
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	case errors.Is(err, model.ErrGroupReservationWalletInsufficient), errors.Is(err, model.ErrGroupReservationSubscriptionInsufficient):
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	default:
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// Subscription admission must create a request-scoped reservation record.
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding:   &WalletFunding{userId: relayInfo.UserId},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
				userId:    relayInfo.UserId,
				modelName: relayInfo.OriginModelName,
				amount:    subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return tryWallet()
			}
			return nil, apiErr
		}
		return session, nil
	}
}
