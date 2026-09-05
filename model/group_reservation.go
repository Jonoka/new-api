package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	GroupReservationWallet       = "wallet"
	GroupReservationSubscription = "subscription"
)

var (
	ErrGroupReservationWalletInsufficient       = errors.New("wallet quota insufficient")
	ErrGroupReservationSubscriptionInsufficient = errors.New("subscription quota insufficient")
	ErrGroupReservationTokenInsufficient        = errors.New("token quota insufficient")
)

type GroupReservationRequest struct {
	Source           string
	RequestId        string
	UserId           int
	ModelName        string
	SubscriptionId   int
	TokenId          int
	TokenKey         string
	TokenUnlimited   bool
	SkipTokenQuota   bool
	ExpectedReserved int
	TargetReserved   int
	// PostConsume is only for an accepted upstream result, never retry admission.
	PostConsume bool
	// Submission fields are populated only for ForcePreConsume/DeferTaskBilling
	// flows. They bind every resize to one durable async submission owner.
	SubmissionID           string
	SubmissionLeaseToken   string
	SubmissionOperationID  string
	SubmissionTaskRowID    int64
	SubmissionLeaseSeconds int64
	SubmissionFinalState   string
	SubmissionFinalReason  string
}

type GroupReservationResult struct {
	Reserved                         int
	SubscriptionId                   int
	SubscriptionAmountTotal          int64
	SubscriptionAmountUsedAfter      int64
	SubscriptionReservationResetTime *int64
}

// ReconcileGroupReservation atomically moves one live request reservation to
// TargetReserved. Pending batch deltas for the same wallet/token are applied in
// the same transaction and removed from their stores exactly once.
func ReconcileGroupReservation(req GroupReservationRequest) (*GroupReservationResult, error) {
	return WithReconciledGroupReservation(req, nil)
}

// WithReconciledGroupReservation couples an async ownership handoff to the
// reservation transaction. Callback failure also restores pending batch deltas.
func WithReconciledGroupReservation(req GroupReservationRequest, apply func(*gorm.DB, *GroupReservationResult) error) (*GroupReservationResult, error) {
	if req.UserId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if req.ExpectedReserved < 0 || req.TargetReserved < 0 || req.ExpectedReserved > common.MaxQuota || req.TargetReserved > common.MaxQuota {
		return nil, errors.New("reservation quota is out of range")
	}
	if req.Source != GroupReservationWallet && req.Source != GroupReservationSubscription {
		return nil, fmt.Errorf("unsupported group reservation source: %s", req.Source)
	}
	if req.SubmissionID != "" {
		req.RequestId = req.SubmissionID
		if req.SubmissionOperationID == "" {
			req.SubmissionOperationID = common.GetUUID()
		}
	}
	if req.Source == GroupReservationSubscription && strings.TrimSpace(req.RequestId) == "" {
		return nil, errors.New("requestId is empty")
	}

	lockUserBatch := req.Source == GroupReservationWallet
	lockTokenBatch := !req.SkipTokenQuota
	if lockUserBatch {
		userQuotaBatchApplyLock.Lock()
		defer userQuotaBatchApplyLock.Unlock()
		batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
		defer batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	}
	if lockTokenBatch {
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
		defer batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	}

	pendingUser := 0
	if lockUserBatch {
		pendingUser = batchUpdateStores[BatchUpdateTypeUserQuota][req.UserId]
		delete(batchUpdateStores[BatchUpdateTypeUserQuota], req.UserId)
	}
	pendingToken := 0
	if lockTokenBatch {
		pendingToken = batchUpdateStores[BatchUpdateTypeTokenQuota][req.TokenId]
		delete(batchUpdateStores[BatchUpdateTypeTokenQuota], req.TokenId)
	}

	result := &GroupReservationResult{Reserved: req.TargetReserved}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var submission *TaskSubmission
		if req.SubmissionID != "" {
			var err error
			submission, err = prepareTaskSubmissionReservationTx(tx, req)
			if err != nil {
				return err
			}
		}
		delta := req.TargetReserved - req.ExpectedReserved
		if req.Source == GroupReservationWallet {
			if err := reconcileWalletReservationTx(tx, req.UserId, pendingUser, delta, req.PostConsume); err != nil {
				return err
			}
		} else {
			// Match wallet lock order before a handoff later updates user counters.
			var user User
			if err := lockForUpdate(tx).Select("id").Where("id = ?", req.UserId).First(&user).Error; err != nil {
				return err
			}
			subResult, err := reconcileSubscriptionReservationTx(tx, req, delta)
			if err != nil {
				return err
			}
			result.SubscriptionId = subResult.SubscriptionId
			result.SubscriptionAmountTotal = subResult.SubscriptionAmountTotal
			result.SubscriptionAmountUsedAfter = subResult.SubscriptionAmountUsedAfter
			result.SubscriptionReservationResetTime = subResult.SubscriptionReservationResetTime
		}
		if err := reconcileTokenReservationTx(tx, req, pendingToken, delta); err != nil {
			return err
		}
		if submission != nil {
			if err := updateTaskSubmissionReservationTx(tx, submission, req, result, pendingUser != 0 || pendingToken != 0); err != nil {
				return err
			}
		}
		if apply != nil {
			return apply(tx, result)
		}
		return nil
	})
	if err != nil {
		restorePending := true
		if req.SubmissionID != "" {
			resolution, resolveErr := resolveTaskSubmissionReservationCommit(req)
			if resolveErr != nil {
				// An unavailable or conflicting durable journal cannot safely be
				// classified as a rollback. Do not replay pulled deltas by guess.
				restorePending = false
			} else if resolution.Committed {
				restorePending = false
				if resolution.CanReturn && apply == nil {
					if cacheErr := ReconcileTaskSubmissionCache(context.Background(), req.SubmissionID); cacheErr != nil {
						common.SysLog("failed to invalidate task submission quota cache after commit reconciliation: " + cacheErr.Error())
					}
					return &resolution.Result, nil
				}
			}
		}
		if restorePending && pendingUser != 0 {
			batchUpdateStores[BatchUpdateTypeUserQuota][req.UserId] += pendingUser
		}
		if restorePending && pendingToken != 0 {
			batchUpdateStores[BatchUpdateTypeTokenQuota][req.TokenId] += pendingToken
		}
		return nil, err
	}

	if req.Source == GroupReservationWallet && common.RedisEnabled {
		if err := invalidateUserCache(req.UserId); err != nil {
			common.SysLog("failed to invalidate user quota cache after group reservation: " + err.Error())
		}
	}
	if !req.SkipTokenQuota && common.RedisEnabled && req.TokenKey != "" {
		if err := cacheDeleteToken(req.TokenKey); err != nil {
			common.SysLog("failed to invalidate token quota cache after group reservation: " + err.Error())
		}
	}
	if req.SubmissionID != "" {
		if err := ReconcileTaskSubmissionCache(context.Background(), req.SubmissionID); err != nil {
			common.SysLog("failed to invalidate task submission quota cache: " + err.Error())
		}
	}
	return result, nil
}

func reconcileWalletReservationTx(tx *gorm.DB, userId int, pendingDelta int, reservationDelta int, postConsume bool) error {
	var user User
	if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	newQuota, err := checkedAccountingQuota(user.Quota, pendingDelta, reservationDelta)
	if err != nil {
		return err
	}
	if !postConsume && reservationDelta > 0 && newQuota < 0 {
		return ErrGroupReservationWalletInsufficient
	}
	return tx.Model(&User{}).Where("id = ?", userId).Update("quota", newQuota).Error
}

func reconcileTokenReservationTx(tx *gorm.DB, req GroupReservationRequest, pendingDelta int, reservationDelta int) error {
	if req.SkipTokenQuota {
		return nil
	}
	if req.TokenId <= 0 || strings.TrimSpace(req.TokenKey) == "" {
		return errors.New("invalid token reservation identity")
	}
	var token Token
	tokenTx := tx
	if req.SubmissionID != "" && reservationDelta <= 0 {
		tokenTx = tx.Unscoped()
	}
	if err := lockForUpdate(tokenTx).Where("id = ?", req.TokenId).First(&token).Error; err != nil {
		if reservationDelta <= 0 && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	newRemain, err := checkedAccountingQuota(token.RemainQuota, pendingDelta, reservationDelta)
	if err != nil {
		return err
	}
	if !req.PostConsume && !req.TokenUnlimited && reservationDelta > 0 && newRemain < 0 {
		return ErrGroupReservationTokenInsufficient
	}
	newUsed, err := checkedAccountingQuota(token.UsedQuota, reservationDelta, pendingDelta)
	if err != nil {
		return err
	}
	return tokenTx.Model(&Token{}).Where("id = ?", req.TokenId).Updates(map[string]interface{}{
		"remain_quota":  newRemain,
		"used_quota":    newUsed,
		"accessed_time": common.GetTimestamp(),
	}).Error
}

func reconcileSubscriptionReservationTx(tx *gorm.DB, req GroupReservationRequest, reservationDelta int) (*GroupReservationResult, error) {
	var record SubscriptionPreConsumeRecord
	query := lockForUpdate(tx).Where("request_id = ?", req.RequestId).Limit(1).Find(&record)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		if req.ExpectedReserved != 0 || req.TargetReserved <= 0 {
			return nil, errors.New("subscription reservation record is missing")
		}
		return createSubscriptionReservationTx(tx, req)
	}
	if record.Status != "consumed" {
		return nil, fmt.Errorf("subscription reservation is %s", record.Status)
	}
	if record.UserId != req.UserId || (req.SubscriptionId > 0 && record.UserSubscriptionId != req.SubscriptionId) {
		return nil, errors.New("subscription reservation identity mismatch")
	}
	if record.PreConsumed != int64(req.ExpectedReserved) {
		return nil, fmt.Errorf("subscription reservation changed, expected=%d actual=%d", req.ExpectedReserved, record.PreConsumed)
	}

	var sub UserSubscription
	if err := lockForUpdate(tx).Where("id = ?", record.UserSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
	if err != nil {
		return nil, err
	}
	now := getDBTimestampTx(tx)
	if !req.PostConsume && req.TargetReserved > 0 && (sub.Status != "active" || sub.EndTime <= now) {
		return nil, ErrGroupReservationSubscriptionInsufficient
	}
	if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
		return nil, err
	}
	reservationDetached := record.ReservationResetTime != nil && *record.ReservationResetTime != sub.LastResetTime
	if reservationDetached {
		// A quota-period reset erased the old reservation from AmountUsed. The
		// selected attempt must establish its complete target in the new period.
		reservationDelta = req.TargetReserved
	}
	if reservationDelta > 0 && sub.AmountUsed > math.MaxInt64-int64(reservationDelta) {
		return nil, errors.New("subscription reservation quota out of range")
	}
	newUsed := sub.AmountUsed + int64(reservationDelta)
	if newUsed < 0 {
		// Legacy records do not carry a reset epoch. Retain the existing clamp
		// policy without guessing whether their old-period quota was consumed.
		newUsed = 0
	}
	if sub.AmountTotal > 0 && newUsed > sub.AmountTotal {
		return nil, fmt.Errorf("%w, remaining=%d need=%d", ErrGroupReservationSubscriptionInsufficient, sub.AmountTotal-sub.AmountUsed, reservationDelta)
	}
	sub.AmountUsed = newUsed
	if err := tx.Save(&sub).Error; err != nil {
		return nil, err
	}
	record.PreConsumed = int64(req.TargetReserved)
	record.ReservationResetTime = common.GetPointer(sub.LastResetTime)
	if req.PostConsume && req.TargetReserved == 0 {
		record.Status = "refunded"
	}
	if err := tx.Save(&record).Error; err != nil {
		return nil, err
	}
	return &GroupReservationResult{
		Reserved:                         req.TargetReserved,
		SubscriptionId:                   sub.Id,
		SubscriptionAmountTotal:          sub.AmountTotal,
		SubscriptionAmountUsedAfter:      sub.AmountUsed,
		SubscriptionReservationResetTime: common.GetPointer(sub.LastResetTime),
	}, nil
}

func createSubscriptionReservationTx(tx *gorm.DB, req GroupReservationRequest) (*GroupReservationResult, error) {
	now := getDBTimestampTx(tx)
	var subs []UserSubscription
	if err := lockForUpdate(tx).
		Where("user_id = ? AND status = ? AND end_time > ?", req.UserId, "active", now).
		Order("end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	for _, candidate := range subs {
		sub := candidate
		plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
		if err != nil {
			return nil, err
		}
		if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
			return nil, err
		}
		if sub.AmountTotal > 0 && sub.AmountTotal-sub.AmountUsed < int64(req.TargetReserved) {
			continue
		}
		if sub.AmountUsed > math.MaxInt64-int64(req.TargetReserved) {
			return nil, errors.New("subscription reservation quota out of range")
		}
		sub.AmountUsed += int64(req.TargetReserved)
		if err := tx.Save(&sub).Error; err != nil {
			return nil, err
		}
		record := &SubscriptionPreConsumeRecord{
			RequestId:            req.RequestId,
			UserId:               req.UserId,
			UserSubscriptionId:   sub.Id,
			PreConsumed:          int64(req.TargetReserved),
			ReservationResetTime: common.GetPointer(sub.LastResetTime),
			Status:               "consumed",
		}
		if err := tx.Create(record).Error; err != nil {
			return nil, err
		}
		return &GroupReservationResult{
			Reserved:                         req.TargetReserved,
			SubscriptionId:                   sub.Id,
			SubscriptionAmountTotal:          sub.AmountTotal,
			SubscriptionAmountUsedAfter:      sub.AmountUsed,
			SubscriptionReservationResetTime: common.GetPointer(sub.LastResetTime),
		}, nil
	}
	return nil, fmt.Errorf("%w, need=%d", ErrGroupReservationSubscriptionInsufficient, req.TargetReserved)
}
