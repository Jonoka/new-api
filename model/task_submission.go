package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	TaskSubmissionVersion = 1

	TaskSubmissionStateActive      = "active"
	TaskSubmissionStateTransferred = "transferred"
	TaskSubmissionStateSettled     = "settled"
	TaskSubmissionStateReleased    = "released"

	TaskSubmissionLeaseDuration = 45 * time.Second
)

var (
	ErrTaskSubmissionNotFound    = errors.New("task submission is not durable")
	ErrTaskSubmissionLeaseLost   = errors.New("task submission lease ownership was lost")
	ErrTaskSubmissionTransferred = errors.New("task submission reservation was transferred")
	ErrTaskSubmissionSettled     = errors.New("task submission reservation was settled")
	ErrTaskSubmissionReleased    = errors.New("task submission reservation was released")
	ErrTaskSubmissionConflict    = errors.New("task submission identity conflict")
)

// TaskSubmission owns a BillingSession reservation until it is released,
// synchronously settled, or transferred to TaskAccounting.
type TaskSubmission struct {
	SubmissionID      string `json:"submission_id" gorm:"type:varchar(64);primaryKey"`
	Version           int    `json:"version" gorm:"not null;default:1"`
	State             string `json:"state" gorm:"type:varchar(20);not null;index:idx_task_submission_recovery,priority:1"`
	LeaseToken        string `json:"lease_token" gorm:"type:varchar(64);not null"`
	LastOperationID   string `json:"last_operation_id" gorm:"type:varchar(64);not null;default:''"`
	LastExpectedQuota int    `json:"last_expected_quota" gorm:"not null;default:0"`
	LastTargetQuota   int    `json:"last_target_quota" gorm:"not null;default:0"`
	LastFinalState    string `json:"last_final_state" gorm:"type:varchar(20);not null;default:''"`
	// FoldedBatchOperationIDs is immutable commit evidence for legacy in-memory
	// batch values folded by reservation transactions. It never stores token keys.
	FoldedBatchOperationIDs string `json:"-" gorm:"type:text"`

	LeaseExpiresAt int64  `json:"lease_expires_at" gorm:"not null;index:idx_task_submission_recovery,priority:2"`
	UserID         int    `json:"user_id" gorm:"not null;index"`
	TaskRowID      *int64 `json:"task_row_id,omitempty" gorm:"uniqueIndex:idx_task_submission_task_row"`

	FundingSource         string `json:"funding_source" gorm:"type:varchar(20);not null;default:''"`
	SubscriptionID        int    `json:"subscription_id" gorm:"not null;default:0"`
	SubscriptionResetTime *int64 `json:"subscription_reset_time,omitempty" gorm:"type:bigint"`
	TokenID               int    `json:"token_id" gorm:"not null;default:0"`
	ModelName             string `json:"model_name" gorm:"type:varchar(191);not null;default:''"`

	ReservedQuota int  `json:"reserved_quota" gorm:"not null;default:0"`
	AcceptedQuota int  `json:"accepted_quota" gorm:"not null;default:0"`
	CachePending  bool `json:"cache_pending" gorm:"not null;default:false;index"`

	CreatedAt     int64  `json:"created_at" gorm:"not null"`
	UpdatedAt     int64  `json:"updated_at" gorm:"not null;index"`
	TransferredAt int64  `json:"transferred_at" gorm:"not null;default:0"`
	SettledAt     int64  `json:"settled_at" gorm:"not null;default:0"`
	ReleasedAt    int64  `json:"released_at" gorm:"not null;default:0"`
	ReleaseReason string `json:"release_reason" gorm:"type:text;not null"`
}

type TaskSubmissionHandoffExpectation struct {
	SubmissionID   string
	LeaseToken     string
	Task           *Task
	FundingSource  string
	ModelName      string
	UserID         int
	SubscriptionID int
	TokenID        int
	ChargedQuota   int
}

type TaskSubmissionHandoffResolution struct {
	Submission TaskSubmission
	Accounting TaskAccounting
	Task       Task
}

type taskSubmissionReservationResolution struct {
	Committed bool
	CanReturn bool
	Result    GroupReservationResult
}

func taskSubmissionLeaseSeconds(seconds int64) int64 {
	if seconds > 0 {
		return seconds
	}
	return int64(TaskSubmissionLeaseDuration / time.Second)
}

func validateTaskSubmissionIdentity(submissionID, leaseToken string, userID int) error {
	if strings.TrimSpace(submissionID) == "" || strings.TrimSpace(leaseToken) == "" {
		return errors.New("task submission identity and lease token are required")
	}
	if len(submissionID) > 64 || len(leaseToken) > 64 {
		return errors.New("task submission identity is too long")
	}
	if userID <= 0 {
		return errors.New("task submission user identity is invalid")
	}
	return nil
}

func taskSubmissionStateError(state string) error {
	switch state {
	case TaskSubmissionStateTransferred:
		return ErrTaskSubmissionTransferred
	case TaskSubmissionStateSettled:
		return ErrTaskSubmissionSettled
	case TaskSubmissionStateReleased:
		return ErrTaskSubmissionReleased
	default:
		return fmt.Errorf("%w: unexpected state %q", ErrTaskSubmissionConflict, state)
	}
}

func taskSubmissionTaskRowID(taskRowID int64) *int64 {
	if taskRowID <= 0 {
		return nil
	}
	return common.GetPointer(taskRowID)
}

// CreateQueuedTaskSubmission inserts a public Canvas/image task and its
// zero-value recovery identity atomically, before the executor is started.
func CreateQueuedTaskSubmission(task *Task, submissionID, leaseToken string) error {
	if task == nil || task.ID != 0 {
		return errors.New("new queued task is required")
	}
	if err := validateTaskSubmissionIdentity(submissionID, leaseToken, task.UserId); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		now := getDBTimestampTx(tx)
		submission := TaskSubmission{
			SubmissionID:   submissionID,
			Version:        TaskSubmissionVersion,
			State:          TaskSubmissionStateActive,
			LeaseToken:     leaseToken,
			LeaseExpiresAt: now + taskSubmissionLeaseSeconds(0),
			UserID:         task.UserId,
			TaskRowID:      taskSubmissionTaskRowID(task.ID),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return tx.Create(&submission).Error
	})
}

func prepareTaskSubmissionReservationTx(tx *gorm.DB, req GroupReservationRequest) (*TaskSubmission, error) {
	if err := validateTaskSubmissionIdentity(req.SubmissionID, req.SubmissionLeaseToken, req.UserId); err != nil {
		return nil, err
	}
	var submission TaskSubmission
	query := lockForUpdate(tx).Where("submission_id = ?", req.SubmissionID).Limit(1).Find(&submission)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		if req.ExpectedReserved != 0 {
			return nil, ErrTaskSubmissionNotFound
		}
		now := getDBTimestampTx(tx)
		submission = TaskSubmission{
			SubmissionID:   req.SubmissionID,
			Version:        TaskSubmissionVersion,
			State:          TaskSubmissionStateActive,
			LeaseToken:     req.SubmissionLeaseToken,
			LeaseExpiresAt: now + taskSubmissionLeaseSeconds(req.SubmissionLeaseSeconds),
			UserID:         req.UserId,
			TaskRowID:      taskSubmissionTaskRowID(req.SubmissionTaskRowID),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&submission).Error; err != nil {
			return nil, err
		}
		return &submission, nil
	}
	if submission.Version != TaskSubmissionVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrTaskSubmissionConflict, submission.Version)
	}
	if submission.State != TaskSubmissionStateActive {
		return nil, taskSubmissionStateError(submission.State)
	}
	if submission.LeaseToken != req.SubmissionLeaseToken {
		return nil, ErrTaskSubmissionLeaseLost
	}
	if submission.UserID != req.UserId || (!req.UseDurableExpected && submission.ReservedQuota != req.ExpectedReserved) {
		return nil, ErrTaskSubmissionConflict
	}
	if submission.TaskRowID != nil && req.SubmissionTaskRowID > 0 && *submission.TaskRowID != req.SubmissionTaskRowID {
		return nil, ErrTaskSubmissionConflict
	}
	if submission.FundingSource != "" {
		expectedTokenID := req.TokenId
		if req.SkipTokenQuota {
			expectedTokenID = 0
		}
		if submission.FundingSource != req.Source || submission.TokenID != expectedTokenID {
			return nil, ErrTaskSubmissionConflict
		}
		if req.SubscriptionId > 0 && submission.SubscriptionID != req.SubscriptionId {
			return nil, ErrTaskSubmissionConflict
		}
	}
	if submission.ModelName != "" && req.ModelName != "" && submission.ModelName != req.ModelName {
		return nil, ErrTaskSubmissionConflict
	}
	return &submission, nil
}

func updateTaskSubmissionReservationTx(tx *gorm.DB, submission *TaskSubmission, req GroupReservationRequest, result *GroupReservationResult, foldedBatchOperationID string) error {
	if submission == nil || result == nil {
		return errors.New("task submission reservation result is required")
	}
	now := getDBTimestampTx(tx)
	tokenID := req.TokenId
	if req.SkipTokenQuota {
		tokenID = 0
	}
	taskRowID := submission.TaskRowID
	if taskRowID == nil && req.SubmissionTaskRowID > 0 {
		taskRowID = taskSubmissionTaskRowID(req.SubmissionTaskRowID)
	}
	fundingSource := submission.FundingSource
	if fundingSource == "" {
		fundingSource = req.Source
	}
	modelName := submission.ModelName
	if modelName == "" {
		modelName = req.ModelName
	}
	updates := map[string]any{
		"funding_source":          fundingSource,
		"subscription_id":         result.SubscriptionId,
		"subscription_reset_time": result.SubscriptionReservationResetTime,
		"token_id":                tokenID,
		"model_name":              modelName,
		"task_row_id":             taskRowID,
		"reserved_quota":          result.Reserved,
		"lease_expires_at":        now + taskSubmissionLeaseSeconds(req.SubmissionLeaseSeconds),
		"updated_at":              now,
		"last_operation_id":       req.SubmissionOperationID,
		"last_expected_quota":     req.ExpectedReserved,
		"last_target_quota":       req.TargetReserved,
		"last_final_state":        req.SubmissionFinalState,
	}
	if foldedBatchOperationID != "" {
		operationIDs, err := appendTaskSubmissionFoldedBatchOperationID(submission.FoldedBatchOperationIDs, foldedBatchOperationID)
		if err != nil {
			return err
		}
		updates["folded_batch_operation_ids"] = operationIDs
	}
	if result.Reserved != req.ExpectedReserved || foldedBatchOperationID != "" {
		updates["cache_pending"] = true
	}
	switch req.SubmissionFinalState {
	case "":
	case TaskSubmissionStateReleased:
		updates["state"] = TaskSubmissionStateReleased
		updates["accepted_quota"] = result.Reserved
		updates["released_at"] = now
		updates["release_reason"] = cleanTaskSubmissionReason(req.SubmissionFinalReason)
		updates["lease_expires_at"] = int64(0)
	case TaskSubmissionStateSettled:
		updates["state"] = TaskSubmissionStateSettled
		updates["accepted_quota"] = result.Reserved
		updates["settled_at"] = now
		updates["lease_expires_at"] = int64(0)
	default:
		return errors.New("unsupported task submission final state")
	}
	query := tx.Model(&TaskSubmission{}).
		Where("submission_id = ? AND state = ? AND lease_token = ? AND reserved_quota = ?", submission.SubmissionID, TaskSubmissionStateActive, submission.LeaseToken, req.ExpectedReserved).
		Updates(updates)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected != 1 {
		return ErrTaskSubmissionConflict
	}
	if req.SubmissionFinalState == TaskSubmissionStateReleased && taskRowID != nil {
		return failReleasedTaskSubmissionTx(tx, *taskRowID, submission.UserID, cleanTaskSubmissionReason(req.SubmissionFinalReason), now)
	}
	return nil
}

func failReleasedTaskSubmissionTx(tx *gorm.DB, taskRowID int64, userID int, reason string, now int64) error {
	var task Task
	if err := lockForUpdate(tx).Where("id = ?", taskRowID).First(&task).Error; err != nil {
		return err
	}
	if task.UserId != userID {
		return ErrTaskSubmissionConflict
	}
	var owners int64
	if err := tx.Model(&TaskAccounting{}).Where("task_row_id = ?", taskRowID).Count(&owners).Error; err != nil {
		return err
	}
	if owners != 0 {
		return ErrTaskSubmissionConflict
	}
	if isTaskTerminal(task.Status) {
		return nil
	}
	if reason == "" {
		reason = "image request ended before task handoff"
	}
	return tx.Model(&Task{}).Where("id = ?", taskRowID).Updates(map[string]any{
		"status": TaskStatusFailure, "progress": "100%", "quota": 0,
		"fail_reason": reason, "finish_time": now, "updated_at": now,
	}).Error
}

func resolveTaskSubmissionReservationCommit(ctx context.Context, req GroupReservationRequest) (*taskSubmissionReservationResolution, error) {
	var submission TaskSubmission
	query := DB.WithContext(ctx).Where("submission_id = ?", req.SubmissionID).Limit(1).Find(&submission)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		return &taskSubmissionReservationResolution{}, nil
	}
	expectedTokenID := req.TokenId
	if req.SkipTokenQuota {
		expectedTokenID = 0
	}
	if submission.Version != TaskSubmissionVersion || submission.LeaseToken != req.SubmissionLeaseToken || submission.UserID != req.UserId {
		return nil, ErrTaskSubmissionConflict
	}
	if req.UseDurableExpected {
		req.ExpectedReserved = submission.LastExpectedQuota
	}
	if submission.TaskRowID != nil && req.SubmissionTaskRowID > 0 && *submission.TaskRowID != req.SubmissionTaskRowID {
		return nil, ErrTaskSubmissionConflict
	}
	if submission.State == TaskSubmissionStateActive && submission.FundingSource == "" &&
		submission.ReservedQuota == 0 && req.ExpectedReserved == 0 {
		return &taskSubmissionReservationResolution{}, nil
	}
	if submission.FundingSource != req.Source || submission.TokenID != expectedTokenID {
		return nil, ErrTaskSubmissionConflict
	}
	if req.SubscriptionId > 0 && submission.SubscriptionID != req.SubscriptionId {
		return nil, ErrTaskSubmissionConflict
	}
	resolution := &taskSubmissionReservationResolution{
		Result: GroupReservationResult{
			Reserved:                         req.TargetReserved,
			PreviousReserved:                 req.ExpectedReserved,
			SubscriptionId:                   submission.SubscriptionID,
			SubscriptionReservationResetTime: submission.SubscriptionResetTime,
		},
	}
	if submission.LastOperationID != req.SubmissionOperationID {
		// This transaction did not publish its operation identity. Any batch
		// deltas pulled before it began therefore remain safe to restore.
		return resolution, nil
	}
	if submission.LastExpectedQuota != req.ExpectedReserved || submission.LastTargetQuota != req.TargetReserved || submission.LastFinalState != req.SubmissionFinalState {
		return nil, ErrTaskSubmissionConflict
	}
	if submission.FundingSource == GroupReservationSubscription && submission.SubscriptionID > 0 {
		var subscription UserSubscription
		if err := DB.WithContext(ctx).Where("id = ?", submission.SubscriptionID).First(&subscription).Error; err != nil {
			return nil, err
		}
		resolution.Result.SubscriptionAmountTotal = subscription.AmountTotal
		resolution.Result.SubscriptionAmountUsedAfter = subscription.AmountUsed
	}
	switch submission.State {
	case TaskSubmissionStateActive:
		if req.SubmissionFinalState == "" && submission.ReservedQuota == req.TargetReserved {
			resolution.Committed = true
			resolution.CanReturn = true
			return resolution, nil
		}
	case TaskSubmissionStateReleased:
		if req.SubmissionFinalState == TaskSubmissionStateReleased && submission.AcceptedQuota == req.TargetReserved {
			resolution.Committed = true
			resolution.CanReturn = true
			return resolution, nil
		}
	case TaskSubmissionStateSettled:
		if req.SubmissionFinalState == TaskSubmissionStateSettled && submission.AcceptedQuota == req.TargetReserved {
			resolution.Committed = true
			resolution.CanReturn = true
			return resolution, nil
		}
	case TaskSubmissionStateTransferred:
		if submission.AcceptedQuota == req.TargetReserved {
			resolution.Committed = true
			return resolution, nil
		}
	}
	return nil, ErrTaskSubmissionConflict
}

// EnsureZeroTaskSubmissionTx creates or validates a zero-value active journal
// for free handoff paths that have no BillingSession.
func EnsureZeroTaskSubmissionTx(tx *gorm.DB, req GroupReservationRequest) error {
	if req.ExpectedReserved != 0 || req.TargetReserved != 0 {
		return errors.New("zero task submission reservation required")
	}
	submission, err := prepareTaskSubmissionReservationTx(tx, req)
	if err != nil {
		return err
	}
	return updateTaskSubmissionReservationTx(tx, submission, req, &GroupReservationResult{Reserved: 0}, "")
}

// TransferTaskSubmissionTx moves the exact active reservation to the durable
// TaskAccounting owner created in the same transaction.
func TransferTaskSubmissionTx(tx *gorm.DB, submissionID, leaseToken string, taskRowID int64, chargedQuota int) error {
	if tx == nil || taskRowID <= 0 || chargedQuota < 0 || chargedQuota > common.MaxQuota {
		return errors.New("invalid task submission transfer")
	}
	var submission TaskSubmission
	if err := lockForUpdate(tx).Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskSubmissionNotFound
		}
		return err
	}
	if submission.State != TaskSubmissionStateActive {
		return taskSubmissionStateError(submission.State)
	}
	if submission.LeaseToken != leaseToken || submission.ReservedQuota != chargedQuota {
		return ErrTaskSubmissionConflict
	}
	if submission.TaskRowID != nil && *submission.TaskRowID != taskRowID {
		return ErrTaskSubmissionConflict
	}
	var ownerCount int64
	if err := tx.Model(&TaskAccounting{}).Where("task_row_id = ?", taskRowID).Count(&ownerCount).Error; err != nil {
		return err
	}
	if ownerCount != 1 {
		return errors.New("task accounting owner is missing during submission transfer")
	}
	now := getDBTimestampTx(tx)
	query := tx.Model(&TaskSubmission{}).
		Where("submission_id = ? AND state = ? AND lease_token = ? AND reserved_quota = ?", submissionID, TaskSubmissionStateActive, leaseToken, chargedQuota).
		Updates(map[string]any{
			"state":            TaskSubmissionStateTransferred,
			"task_row_id":      taskRowID,
			"accepted_quota":   chargedQuota,
			"lease_expires_at": int64(0),
			"transferred_at":   now,
			"updated_at":       now,
		})
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected != 1 {
		return ErrTaskSubmissionConflict
	}
	return nil
}

// ResolveTaskSubmissionHandoff accepts an ambiguous transaction only when the
// journal, task and TaskAccounting owner all exactly match the intended owner.
func ResolveTaskSubmissionHandoff(ctx context.Context, expected TaskSubmissionHandoffExpectation) (*TaskSubmissionHandoffResolution, error) {
	if expected.Task == nil || expected.ChargedQuota < 0 {
		return nil, errors.New("invalid task submission handoff expectation")
	}
	var resolution TaskSubmissionHandoffResolution
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("submission_id = ?", expected.SubmissionID).First(&resolution.Submission).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskSubmissionNotFound
			}
			return err
		}
		if resolution.Submission.State != TaskSubmissionStateTransferred {
			return taskSubmissionStateError(resolution.Submission.State)
		}
		if resolution.Submission.LeaseToken != expected.LeaseToken || resolution.Submission.TaskRowID == nil ||
			resolution.Submission.UserID != expected.UserID || resolution.Submission.FundingSource != expected.FundingSource ||
			resolution.Submission.SubscriptionID != expected.SubscriptionID || resolution.Submission.TokenID != expected.TokenID ||
			resolution.Submission.ReservedQuota != expected.ChargedQuota || resolution.Submission.AcceptedQuota != expected.ChargedQuota ||
			(expected.ModelName != "" && resolution.Submission.ModelName != expected.ModelName) {
			return ErrTaskSubmissionConflict
		}
		if err := tx.Where("task_row_id = ?", *resolution.Submission.TaskRowID).First(&resolution.Accounting).Error; err != nil {
			return err
		}
		if resolution.Accounting.UserID != expected.UserID || resolution.Accounting.FundingSource != expected.FundingSource ||
			resolution.Accounting.SubscriptionID != expected.SubscriptionID || resolution.Accounting.TokenID != expected.TokenID ||
			resolution.Accounting.ChargedQuota != expected.ChargedQuota {
			return ErrTaskSubmissionConflict
		}
		if err := tx.Where("id = ?", *resolution.Submission.TaskRowID).First(&resolution.Task).Error; err != nil {
			return err
		}
		candidate := expected.Task
		if candidate.ID > 0 && candidate.ID != resolution.Task.ID {
			return ErrTaskSubmissionConflict
		}
		if candidate.UserId != resolution.Task.UserId || candidate.TaskID != resolution.Task.TaskID ||
			candidate.Platform != resolution.Task.Platform || candidate.Action != resolution.Task.Action ||
			candidate.Group != resolution.Task.Group || candidate.ChannelId != resolution.Task.ChannelId ||
			candidate.PrivateData.UpstreamTaskID != resolution.Task.PrivateData.UpstreamTaskID {
			return ErrTaskSubmissionConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &resolution, nil
}

// ExtendTaskSubmissionLease refreshes only the current active owner.
func ExtendTaskSubmissionLease(ctx context.Context, submissionID, leaseToken string, duration time.Duration) (bool, error) {
	if strings.TrimSpace(submissionID) == "" || strings.TrimSpace(leaseToken) == "" {
		return false, errors.New("task submission heartbeat identity is required")
	}
	seconds := int64(duration / time.Second)
	if seconds <= 0 {
		seconds = taskSubmissionLeaseSeconds(0)
	}
	owned := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var submission TaskSubmission
		query := lockForUpdate(tx).Where("submission_id = ?", submissionID).Limit(1).Find(&submission)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 0 || submission.State != TaskSubmissionStateActive || submission.LeaseToken != leaseToken {
			return nil
		}
		now := getDBTimestampTx(tx)
		if err := tx.Model(&TaskSubmission{}).Where("submission_id = ?", submissionID).
			Updates(map[string]any{"lease_expires_at": now + seconds, "updated_at": now}).Error; err != nil {
			return err
		}
		owned = true
		return nil
	})
	return owned, err
}

func cleanTaskSubmissionReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		return reason[:500]
	}
	return reason
}

func releaseExpiredTaskSubmissionTx(tx *gorm.DB, submissionID string) (*TaskSubmission, bool, error) {
	return releaseTaskSubmissionTx(tx, submissionID, true, "submission lease expired before durable task handoff")
}

func releaseTaskSubmissionTx(tx *gorm.DB, submissionID string, expiredOnly bool, reason string) (*TaskSubmission, bool, error) {
	var submission TaskSubmission
	if err := lockForUpdate(tx).Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
		return nil, false, err
	}
	now := getDBTimestampTx(tx)
	if submission.State != TaskSubmissionStateActive || (expiredOnly && submission.LeaseExpiresAt > now) {
		return &submission, false, nil
	}
	if submission.TaskRowID != nil {
		var ownerCount int64
		if err := tx.Model(&TaskAccounting{}).Where("task_row_id = ?", *submission.TaskRowID).Count(&ownerCount).Error; err != nil {
			return nil, false, err
		}
		if ownerCount != 0 {
			return nil, false, errors.New("expired task submission already has task accounting ownership")
		}
	}
	if submission.ReservedQuota > 0 {
		req := GroupReservationRequest{
			Source:           submission.FundingSource,
			RequestId:        submission.SubmissionID,
			UserId:           submission.UserID,
			SubscriptionId:   submission.SubscriptionID,
			TokenId:          submission.TokenID,
			SkipTokenQuota:   submission.TokenID == 0,
			ExpectedReserved: submission.ReservedQuota,
			TargetReserved:   0,
			PostConsume:      true,
		}
		if submission.FundingSource == GroupReservationWallet {
			if err := reconcileWalletReservationTx(tx, submission.UserID, 0, -submission.ReservedQuota, true); err != nil {
				return nil, false, err
			}
		} else if submission.FundingSource == GroupReservationSubscription {
			var record SubscriptionPreConsumeRecord
			if err := lockForUpdate(tx).Where("request_id = ?", submission.SubmissionID).First(&record).Error; err != nil {
				return nil, false, err
			}
			if record.UserId != submission.UserID || record.UserSubscriptionId != submission.SubscriptionID ||
				record.PreConsumed != int64(submission.ReservedQuota) || !sameTaskSubmissionResetTime(record.ReservationResetTime, submission.SubscriptionResetTime) {
				return nil, false, ErrTaskSubmissionConflict
			}
			if _, err := reconcileSubscriptionReservationTx(tx, req, -submission.ReservedQuota); err != nil {
				return nil, false, err
			}
		} else {
			return nil, false, errors.New("expired paid task submission has invalid funding source")
		}
		if submission.TokenID > 0 {
			if err := releaseTaskSubmissionTokenTx(tx, submission.TokenID, submission.ReservedQuota); err != nil {
				return nil, false, err
			}
		}
	}
	reason = cleanTaskSubmissionReason(reason)
	if submission.TaskRowID != nil {
		var task Task
		if err := lockForUpdate(tx).Where("id = ?", *submission.TaskRowID).First(&task).Error; err != nil {
			return nil, false, err
		}
		if task.UserId != submission.UserID {
			return nil, false, ErrTaskSubmissionConflict
		}
		if !isTaskTerminal(task.Status) {
			if err := tx.Model(&Task{}).Where("id = ?", task.ID).
				Updates(map[string]any{
					"status":      TaskStatusFailure,
					"progress":    "100%",
					"fail_reason": reason,
					"finish_time": now,
					"updated_at":  now,
				}).Error; err != nil {
				return nil, false, err
			}
		}
	}
	hadReservation := submission.ReservedQuota > 0
	query := tx.Model(&TaskSubmission{}).
		Where("submission_id = ? AND state = ? AND lease_token = ?", submission.SubmissionID, TaskSubmissionStateActive, submission.LeaseToken)
	if expiredOnly {
		query = query.Where("lease_expires_at <= ?", now)
	}
	query = query.Updates(map[string]any{
		"state":            TaskSubmissionStateReleased,
		"reserved_quota":   0,
		"accepted_quota":   0,
		"lease_expires_at": int64(0),
		"released_at":      now,
		"release_reason":   reason,
		"cache_pending":    submission.ReservedQuota > 0,
		"updated_at":       now,
	})
	if query.Error != nil {
		return nil, false, query.Error
	}
	if query.RowsAffected != 1 {
		return nil, false, ErrTaskSubmissionConflict
	}
	submission.State = TaskSubmissionStateReleased
	submission.ReservedQuota = 0
	submission.AcceptedQuota = 0
	submission.LeaseExpiresAt = 0
	submission.ReleasedAt = now
	submission.ReleaseReason = reason
	submission.CachePending = hadReservation
	return &submission, true, nil
}

func sameTaskSubmissionResetTime(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// FailTaskSubmission releases the durable reservation and publishes its known
// Canvas task failure together, including zero-charge failures before execution.
func FailTaskSubmission(ctx context.Context, taskRowID int64, userID int, reason string) (*Task, error) {
	var canonical Task
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var submission TaskSubmission
		if err := lockForUpdate(tx).Where("task_row_id = ?", taskRowID).First(&submission).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskSubmissionNotFound
			}
			return err
		}
		if submission.UserID != userID {
			return ErrTaskSubmissionConflict
		}
		if submission.State != TaskSubmissionStateActive && submission.State != TaskSubmissionStateReleased {
			return taskSubmissionStateError(submission.State)
		}
		if _, _, err := releaseTaskSubmissionTx(tx, submission.SubmissionID, false, reason); err != nil {
			return err
		}
		return tx.First(&canonical, taskRowID).Error
	})
	if err != nil {
		return nil, err
	}
	return &canonical, nil
}

func releaseTaskSubmissionTokenTx(tx *gorm.DB, tokenID, reservedQuota int) error {
	var token Token
	query := lockForUpdate(tx.Unscoped()).Where("id = ?", tokenID).Limit(1).Find(&token)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected == 0 {
		return nil
	}
	remain, err := checkedAccountingQuota(token.RemainQuota, 0, -reservedQuota)
	if err != nil {
		return err
	}
	used, err := checkedAccountingQuota(token.UsedQuota, -reservedQuota, 0)
	if err != nil {
		return err
	}
	if used < 0 {
		return errors.New("task submission release would make token used quota negative")
	}
	return tx.Unscoped().Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"remain_quota": remain,
		"used_quota":   used,
	}).Error
}

// RecoverExpiredTaskSubmissions releases expired active reservations exactly
// once and then retries derived cache invalidation for released rows.
func RecoverExpiredTaskSubmissions(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	firstErr := RecoverPendingTaskSubmissionBatches(ctx, limit)
	now := getDBTimestampTx(DB.WithContext(ctx))
	var ids []string
	if err := DB.WithContext(ctx).Model(&TaskSubmission{}).
		Where("state = ? AND lease_expires_at <= ?", TaskSubmissionStateActive, now).
		Order("lease_expires_at asc").Limit(limit).Pluck("submission_id", &ids).Error; err != nil {
		if firstErr == nil {
			firstErr = err
		}
		return firstErr
	}
	for _, submissionID := range ids {
		if hasPendingTaskSubmissionBatch(submissionID) {
			continue
		}
		if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, _, err := releaseExpiredTaskSubmissionTx(tx, submissionID)
			return err
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	var pending []TaskSubmission
	if err := DB.WithContext(ctx).Where("cache_pending = ?", true).
		Order("updated_at asc").Limit(limit).Find(&pending).Error; err != nil && firstErr == nil {
		firstErr = err
	}
	for _, submission := range pending {
		if err := ReconcileTaskSubmissionCache(ctx, submission.SubmissionID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := DeleteClosedOrdinaryTaskSubmissions(ctx, now-7*24*60*60, limit); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// DeleteClosedOrdinaryTaskSubmissions bounds generic receipt retention only
// after every durable owner and pending projection has been ruled out.
func DeleteClosedOrdinaryTaskSubmissions(ctx context.Context, olderThan int64, limit int) error {
	if olderThan <= 0 {
		return errors.New("closed task submission retention cutoff is required")
	}
	if limit <= 0 {
		limit = 100
	}
	query := DB.WithContext(ctx).Model(&TaskSubmission{}).
		Where("state IN ?", []string{TaskSubmissionStateSettled, TaskSubmissionStateReleased}).
		Where("task_row_id IS NULL AND cache_pending = ? AND updated_at < ?", false, olderThan).
		Where("folded_batch_operation_ids IS NULL OR folded_batch_operation_ids = ?", "")
	if DB.Migrator().HasTable(&TaskAccountingEvent{}) {
		query = query.Where("NOT EXISTS (?)", DB.WithContext(ctx).Model(&TaskAccountingEvent{}).
			Select("1").Where("task_accounting_events.submission_id = task_submissions.submission_id AND task_accounting_events.delivered = ?", false))
	}
	var ids []string
	if err := query.Order("updated_at asc").Limit(limit).Pluck("submission_id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	result := DB.WithContext(ctx).Where("submission_id IN ?", ids).
		Where("state IN ?", []string{TaskSubmissionStateSettled, TaskSubmissionStateReleased}).
		Where("task_row_id IS NULL AND cache_pending = ? AND updated_at < ?", false, olderThan).
		Where("folded_batch_operation_ids IS NULL OR folded_batch_operation_ids = ?", "")
	if DB.Migrator().HasTable(&TaskAccountingEvent{}) {
		result = result.Where("NOT EXISTS (?)", DB.WithContext(ctx).Model(&TaskAccountingEvent{}).
			Select("1").Where("task_accounting_events.submission_id = task_submissions.submission_id AND task_accounting_events.delivered = ?", false))
	}
	result = result.Delete(&TaskSubmission{})
	return result.Error
}

func ReconcileTaskSubmissionCache(ctx context.Context, submissionID string) error {
	var submission TaskSubmission
	if err := DB.WithContext(ctx).Where("submission_id = ? AND cache_pending = ?", submissionID, true).
		First(&submission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if submission.FundingSource == GroupReservationWallet {
		if err := invalidateUserCache(submission.UserID); err != nil {
			return err
		}
	}
	if submission.TokenID > 0 && common.RedisEnabled {
		var token Token
		query := DB.WithContext(ctx).Unscoped().Where("id = ?", submission.TokenID).Limit(1).Find(&token)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 && token.Key != "" {
			if err := cacheDeleteToken(token.Key); err != nil {
				return err
			}
		}
	}
	return DB.WithContext(ctx).Model(&TaskSubmission{}).
		Where("submission_id = ? AND cache_pending = ? AND state = ? AND last_operation_id = ?", submissionID, true, submission.State, submission.LastOperationID).
		Update("cache_pending", false).Error
}

func GetTaskSubmission(submissionID string) (*TaskSubmission, error) {
	var submission TaskSubmission
	if err := DB.Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
		return nil, err
	}
	return &submission, nil
}
