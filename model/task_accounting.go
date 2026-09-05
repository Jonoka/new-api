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
	TaskAccountingVersion = 1

	TaskAccountingFundingWallet       = "wallet"
	TaskAccountingFundingSubscription = "subscription"
)

var (
	ErrTaskAccountingNotManaged = errors.New("task has no durable accounting ownership")
	ErrTaskAccountingConflict   = errors.New("task terminal accounting decision already exists")
)

// TaskAccounting owns the durable money lifecycle for one Task row. TaskRowID
// is the primary key so an upstream/public task id can never own money twice.
type TaskAccounting struct {
	TaskRowID      int64  `json:"task_row_id" gorm:"primaryKey;autoIncrement:false"`
	Version        int    `json:"version" gorm:"not null;default:1"`
	FundingSource  string `json:"funding_source" gorm:"type:varchar(20);not null"`
	UserID         int    `json:"user_id" gorm:"not null;index"`
	SubscriptionID int    `json:"subscription_id" gorm:"not null;default:0"`
	TokenID        int    `json:"token_id" gorm:"not null;default:0"`
	ChannelID      int    `json:"channel_id" gorm:"not null;default:0"`
	ChargedQuota   int    `json:"charged_quota" gorm:"not null"`
	InitialLogJSON string `json:"initial_log_json" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"not null"`

	DecisionID      string     `json:"decision_id" gorm:"type:varchar(64);not null;default:'';index"`
	DecisionStatus  TaskStatus `json:"decision_status" gorm:"type:varchar(20);not null;default:''"`
	DecisionQuota   int        `json:"decision_quota" gorm:"not null;default:0"`
	DecisionReason  string     `json:"decision_reason" gorm:"type:text;not null"`
	DecisionJSON    string     `json:"decision_json" gorm:"type:text;not null"`
	DecisionLogJSON string     `json:"decision_log_json" gorm:"type:text;not null"`
	DecidedAt       int64      `json:"decided_at" gorm:"not null;default:0;index"`

	MoneyApplied   bool  `json:"money_applied" gorm:"not null;default:false;index"`
	MoneyAppliedAt int64 `json:"money_applied_at" gorm:"not null;default:0"`
	CachePending   bool  `json:"cache_pending" gorm:"not null;default:false;index"`
}

type TaskAccountingLogFacts struct {
	PromptTokens      int            `json:"prompt_tokens,omitempty"`
	CompletionTokens  int            `json:"completion_tokens,omitempty"`
	UseTimeSeconds    int            `json:"use_time_seconds,omitempty"`
	IsStream          bool           `json:"is_stream,omitempty"`
	UserID            int            `json:"user_id"`
	Username          string         `json:"username,omitempty"`
	TokenID           int            `json:"token_id,omitempty"`
	TokenName         string         `json:"token_name,omitempty"`
	ChannelID         int            `json:"channel_id,omitempty"`
	ModelName         string         `json:"model_name,omitempty"`
	Group             string         `json:"group,omitempty"`
	RequestID         string         `json:"request_id,omitempty"`
	UpstreamRequestID string         `json:"upstream_request_id,omitempty"`
	IP                string         `json:"ip,omitempty"`
	LogType           int            `json:"log_type"`
	Quota             int            `json:"quota"`
	Content           string         `json:"content,omitempty"`
	CreatedAt         int64          `json:"created_at"`
	Other             map[string]any `json:"other,omitempty"`
}

type AsyncTaskHandoffRequest struct {
	Task           *Task
	ExpectedStatus TaskStatus
	ChargedQuota   int
	InitialLog     TaskAccountingLogFacts
}

type TaskTerminalDecision struct {
	TaskRowID      int64
	ExpectedStatus TaskStatus
	Status         TaskStatus
	Progress       string
	StartTime      int64
	FinishTime     int64
	FailReason     string
	ResultURL      string
	Data           []byte
	FinalQuota     int
	Reason         string
}

type taskTerminalFields struct {
	Progress   string `json:"progress"`
	StartTime  int64  `json:"start_time"`
	FinishTime int64  `json:"finish_time"`
	FailReason string `json:"fail_reason,omitempty"`
	ResultURL  string `json:"result_url,omitempty"`
	Data       []byte `json:"data,omitempty"`
}

type TaskTerminalDecisionResult struct {
	Won        bool
	Applied    bool
	Accounting TaskAccounting
	Task       Task
}

// PersistAsyncTaskHandoffTx must be called inside the same primary-DB
// transaction that reconciles the live reservation. New tasks are inserted;
// Canvas handoffs update an existing row through ExpectedStatus CAS.
func PersistAsyncTaskHandoffTx(tx *gorm.DB, req AsyncTaskHandoffRequest) (*TaskAccounting, error) {
	if tx == nil || req.Task == nil {
		return nil, errors.New("task handoff transaction and task are required")
	}
	if req.ChargedQuota < 0 || req.ChargedQuota > common.MaxQuota {
		return nil, errors.New("charged task quota is out of range")
	}
	task := req.Task
	if task.UserId <= 0 || task.UserId != req.InitialLog.UserID {
		return nil, errors.New("task handoff user identity mismatch")
	}
	if task.PrivateData.BillingSource == "" {
		task.PrivateData.BillingSource = TaskAccountingFundingWallet
	}
	if task.PrivateData.BillingSource != TaskAccountingFundingWallet && task.PrivateData.BillingSource != TaskAccountingFundingSubscription {
		return nil, fmt.Errorf("unsupported task funding source: %s", task.PrivateData.BillingSource)
	}
	if task.PrivateData.BillingSource == TaskAccountingFundingSubscription && task.PrivateData.SubscriptionId <= 0 {
		return nil, errors.New("subscription task has no subscription identity")
	}

	task.Quota = req.ChargedQuota
	now := getDBTimestampTx(tx)
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	if task.ID == 0 {
		if err := tx.Create(task).Error; err != nil {
			return nil, err
		}
	} else {
		if req.ExpectedStatus == "" {
			return nil, errors.New("existing task handoff requires expected status")
		}
		updates := taskHandoffUpdates(task)
		result := tx.Model(&Task{}).
			Where("id = ? AND status = ?", task.ID, req.ExpectedStatus).
			Where("NOT EXISTS (?)", tx.Model(&TaskAccounting{}).Select("1").Where("task_row_id = ?", task.ID)).
			Updates(updates)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrTaskAccountingConflict
		}
	}

	initialLog := req.InitialLog
	initialLog.ChannelID = task.ChannelId
	initialLog.TokenID = task.PrivateData.TokenId
	initialLog.Group = task.Group
	initialLog.Quota = req.ChargedQuota
	initialLog.LogType = LogTypeConsume
	if initialLog.CreatedAt == 0 {
		initialLog.CreatedAt = now
	}
	initialLogJSON, err := common.Marshal(initialLog)
	if err != nil {
		return nil, err
	}
	accounting := &TaskAccounting{
		TaskRowID:      task.ID,
		Version:        TaskAccountingVersion,
		FundingSource:  task.PrivateData.BillingSource,
		UserID:         task.UserId,
		SubscriptionID: task.PrivateData.SubscriptionId,
		TokenID:        task.PrivateData.TokenId,
		ChannelID:      task.ChannelId,
		ChargedQuota:   req.ChargedQuota,
		InitialLogJSON: string(initialLogJSON),
		CreatedAt:      now,
		CachePending:   true,
	}
	if err := tx.Create(accounting).Error; err != nil {
		return nil, err
	}
	if err := incrementTaskUsageTx(tx, task.UserId, task.ChannelId, req.ChargedQuota, true); err != nil {
		return nil, err
	}
	if _, err := createTaskAccountingEventTx(tx, task.ID, "initial", initialLog, true); err != nil {
		return nil, err
	}
	return accounting, nil
}

func taskHandoffUpdates(task *Task) map[string]any {
	return map[string]any{
		"updated_at": task.UpdatedAt, "task_id": task.TaskID, "platform": task.Platform,
		"user_id": task.UserId, "group": task.Group, "channel_id": task.ChannelId,
		"quota": task.Quota, "action": task.Action, "status": task.Status,
		"fail_reason": task.FailReason, "submit_time": task.SubmitTime, "start_time": task.StartTime,
		"finish_time": task.FinishTime, "progress": task.Progress, "properties": task.Properties,
		"private_data": task.PrivateData, "data": task.Data,
	}
}

func incrementTaskUsageTx(tx *gorm.DB, userID, channelID, quota int, countRequest bool) error {
	if quota == 0 && !countRequest {
		return nil
	}
	var user User
	if err := lockForUpdate(tx).Where("id = ?", userID).First(&user).Error; err != nil {
		return err
	}
	usedQuota, err := checkedAccountingQuota(user.UsedQuota, quota, 0)
	if err != nil {
		return err
	}
	if usedQuota < 0 {
		return errors.New("task accounting would make user used quota negative")
	}
	userUpdates := map[string]any{"used_quota": usedQuota}
	if countRequest {
		requestCount, err := checkedAccountingQuota(user.RequestCount, 1, 0)
		if err != nil {
			return err
		}
		userUpdates["request_count"] = requestCount
	}
	result := tx.Model(&User{}).Where("id = ?", userID).Updates(userUpdates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task accounting user row is missing")
	}
	if channelID <= 0 || quota == 0 {
		return nil
	}
	var channel Channel
	if err := lockForUpdate(tx).Where("id = ?", channelID).First(&channel).Error; err != nil {
		return err
	}
	if int64(int(channel.UsedQuota)) != channel.UsedQuota {
		return errors.New("channel used quota is out of range")
	}
	channelUsed, err := checkedTaskAccountingInt64(channel.UsedQuota, int64(quota))
	if err != nil {
		return err
	}
	if channelUsed < 0 {
		return errors.New("task accounting would make channel used quota negative")
	}
	result = tx.Model(&Channel{}).Where("id = ?", channelID).
		Update("used_quota", channelUsed)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task accounting channel row is missing")
	}
	return nil
}

// AcceptTaskTerminalDecision durably records the first terminal result while
// leaving the public task row nonterminal until its money transaction commits.
func AcceptTaskTerminalDecision(ctx context.Context, decision TaskTerminalDecision) (*TaskTerminalDecisionResult, error) {
	if decision.TaskRowID <= 0 {
		return nil, errors.New("task row id is required")
	}
	if decision.FinalQuota < 0 || decision.FinalQuota > common.MaxQuota {
		return nil, errors.New("final task quota is out of range")
	}
	if decision.Status != TaskStatusSuccess && decision.Status != TaskStatusFailure {
		return nil, errors.New("terminal task status must be SUCCESS or FAILURE")
	}
	var outcome TaskTerminalDecisionResult
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var accounting TaskAccounting
		if err := lockForUpdate(tx).Where("task_row_id = ?", decision.TaskRowID).First(&accounting).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskAccountingNotManaged
			}
			return err
		}
		var task Task
		if err := lockForUpdate(tx).Where("id = ?", decision.TaskRowID).First(&task).Error; err != nil {
			return err
		}
		if accounting.DecisionID != "" {
			outcome = TaskTerminalDecisionResult{Won: false, Applied: accounting.MoneyApplied, Accounting: accounting, Task: task}
			return nil
		}
		if task.Status != decision.ExpectedStatus || isTaskTerminal(task.Status) {
			outcome = TaskTerminalDecisionResult{Won: false, Applied: false, Accounting: accounting, Task: task}
			return nil
		}
		fields := taskTerminalFields{
			Progress: decision.Progress, StartTime: decision.StartTime, FinishTime: decision.FinishTime,
			FailReason: cleanTaskAccountingReason(decision.FailReason), ResultURL: decision.ResultURL,
			Data: append([]byte(nil), decision.Data...),
		}
		payload, err := common.Marshal(fields)
		if err != nil {
			return err
		}
		logFacts, err := buildTerminalAccountingLog(accounting, decision.Status, decision.FinalQuota, decision.Reason)
		if err != nil {
			return err
		}
		logJSON, err := common.Marshal(logFacts)
		if err != nil {
			return err
		}
		now := getDBTimestampTx(tx)
		decisionID := common.GetUUID()
		updates := map[string]any{
			"decision_id": decisionID, "decision_status": decision.Status,
			"decision_quota": decision.FinalQuota, "decision_reason": cleanTaskAccountingReason(decision.Reason),
			"decision_json": string(payload), "decision_log_json": string(logJSON), "decided_at": now,
		}
		result := tx.Model(&TaskAccounting{}).
			Where("task_row_id = ? AND decision_id = ?", decision.TaskRowID, "").Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskAccountingConflict
		}
		for key, value := range updates {
			switch key {
			case "decision_id":
				accounting.DecisionID = value.(string)
			case "decision_status":
				accounting.DecisionStatus = value.(TaskStatus)
			case "decision_quota":
				accounting.DecisionQuota = value.(int)
			case "decision_reason":
				accounting.DecisionReason = value.(string)
			case "decision_json":
				accounting.DecisionJSON = value.(string)
			case "decision_log_json":
				accounting.DecisionLogJSON = value.(string)
			case "decided_at":
				accounting.DecidedAt = value.(int64)
			}
		}
		outcome = TaskTerminalDecisionResult{Won: true, Applied: false, Accounting: accounting, Task: task}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &outcome, nil
}

func isTaskTerminal(status TaskStatus) bool {
	return status == TaskStatusSuccess || status == TaskStatusFailure
}

func cleanTaskAccountingReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 1024 {
		reason = reason[:1024]
	}
	return reason
}

func buildTerminalAccountingLog(accounting TaskAccounting, status TaskStatus, finalQuota int, reason string) (TaskAccountingLogFacts, error) {
	var facts TaskAccountingLogFacts
	if err := common.Unmarshal([]byte(accounting.InitialLogJSON), &facts); err != nil {
		return facts, err
	}
	delta := finalQuota - accounting.ChargedQuota
	facts.CreatedAt = common.GetTimestamp()
	facts.Content = cleanTaskAccountingReason(reason)
	if status == TaskStatusFailure {
		facts.Content = "async task failed"
	}
	facts.Quota = delta
	if delta < 0 {
		facts.LogType = LogTypeRefund
		facts.Quota = -delta
	} else {
		facts.LogType = LogTypeConsume
	}
	if facts.Other == nil {
		facts.Other = map[string]any{}
	}
	facts.Other["is_task"] = true
	facts.Other["pre_consumed_quota"] = accounting.ChargedQuota
	facts.Other["actual_quota"] = finalQuota
	facts.Other["reason"] = facts.Content
	return facts, nil
}

// ApplyTaskAccountingDecision applies the frozen decision in one primary-DB
// transaction. It is safe to retry after errors or process restarts.
func ApplyTaskAccountingDecision(ctx context.Context, taskRowID int64) (*TaskTerminalDecisionResult, error) {
	var outcome TaskTerminalDecisionResult
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var accounting TaskAccounting
		if err := lockForUpdate(tx).Where("task_row_id = ?", taskRowID).First(&accounting).Error; err != nil {
			return err
		}
		var task Task
		if err := lockForUpdate(tx).Where("id = ?", taskRowID).First(&task).Error; err != nil {
			return err
		}
		if accounting.DecisionID == "" {
			return errors.New("task terminal decision is missing")
		}
		if accounting.MoneyApplied {
			outcome = TaskTerminalDecisionResult{Won: false, Applied: true, Accounting: accounting, Task: task}
			return nil
		}
		delta := accounting.DecisionQuota - accounting.ChargedQuota
		if err := applyTaskFundingDeltaTx(tx, &accounting, delta); err != nil {
			return err
		}
		if err := applyTaskTokenDeltaTx(tx, accounting.TokenID, delta); err != nil {
			return err
		}
		if err := incrementTaskUsageTx(tx, accounting.UserID, accounting.ChannelID, delta, false); err != nil {
			return err
		}
		var fields taskTerminalFields
		if err := common.Unmarshal([]byte(accounting.DecisionJSON), &fields); err != nil {
			return err
		}
		privateData := task.PrivateData
		privateData.ResultURL = fields.ResultURL
		taskUpdates := map[string]any{
			"updated_at": getDBTimestampTx(tx), "status": accounting.DecisionStatus,
			"progress": fields.Progress, "start_time": fields.StartTime, "finish_time": fields.FinishTime,
			"fail_reason": fields.FailReason, "private_data": privateData, "data": fields.Data,
			"quota": accounting.DecisionQuota,
		}
		result := tx.Model(&Task{}).Where("id = ?", taskRowID).Updates(taskUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("task row disappeared during accounting")
		}
		now := getDBTimestampTx(tx)
		accounting.MoneyApplied = true
		accounting.MoneyAppliedAt = now
		accounting.CachePending = true
		if err := tx.Model(&TaskAccounting{}).Where("task_row_id = ? AND money_applied = ?", taskRowID, false).
			Updates(map[string]any{"money_applied": true, "money_applied_at": now, "cache_pending": true}).Error; err != nil {
			return err
		}
		if delta != 0 {
			var logFacts TaskAccountingLogFacts
			if err := common.Unmarshal([]byte(accounting.DecisionLogJSON), &logFacts); err != nil {
				return err
			}
			if _, err := createTaskAccountingEventTx(tx, taskRowID, "adjustment", logFacts, true); err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", taskRowID).First(&task).Error; err != nil {
			return err
		}
		outcome = TaskTerminalDecisionResult{Won: true, Applied: true, Accounting: accounting, Task: task}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &outcome, nil
}

func applyTaskFundingDeltaTx(tx *gorm.DB, accounting *TaskAccounting, delta int) error {
	if delta == 0 {
		return nil
	}
	switch accounting.FundingSource {
	case TaskAccountingFundingSubscription:
		var user User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", accounting.UserID).First(&user).Error; err != nil {
			return err
		}
		var subscription UserSubscription
		if err := lockForUpdate(tx).Where("id = ?", accounting.SubscriptionID).First(&subscription).Error; err != nil {
			return err
		}
		if _, err := checkedTaskAccountingInt64(subscription.AmountUsed, int64(delta)); err != nil {
			return err
		}
		return postConsumeUserSubscriptionDeltaTx(tx, accounting.SubscriptionID, int64(delta))
	case TaskAccountingFundingWallet:
		var user User
		if err := lockForUpdate(tx).Where("id = ?", accounting.UserID).First(&user).Error; err != nil {
			return err
		}
		newQuota, err := checkedAccountingQuota(user.Quota, 0, delta)
		if err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", accounting.UserID).Update("quota", newQuota).Error
	default:
		return fmt.Errorf("unsupported task accounting funding source: %s", accounting.FundingSource)
	}
}

func checkedTaskAccountingInt64(base, delta int64) (int64, error) {
	if (delta > 0 && base > math.MaxInt64-delta) || (delta < 0 && base < math.MinInt64-delta) {
		return 0, errors.New("task accounting integer overflow")
	}
	return base + delta, nil
}

func applyTaskTokenDeltaTx(tx *gorm.DB, tokenID, delta int) error {
	if tokenID <= 0 || delta == 0 {
		return nil
	}
	var token Token
	result := lockForUpdate(tx.Unscoped()).Where("id = ?", tokenID).Limit(1).Find(&token)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// A physically removed token cannot be restored. Funding and aggregate
		// accounting still settle; soft-deleted tokens are updated Unscoped.
		return nil
	}
	remain, err := checkedAccountingQuota(token.RemainQuota, 0, delta)
	if err != nil {
		return err
	}
	used, err := checkedAccountingQuota(token.UsedQuota, delta, 0)
	if err != nil {
		return err
	}
	if used < 0 {
		return errors.New("task accounting would make token used quota negative")
	}
	return tx.Unscoped().Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"remain_quota": remain,
		"used_quota":   used,
	}).Error
}

func ReconcileTaskAccountingCache(ctx context.Context, taskRowID int64) error {
	var accounting TaskAccounting
	if err := DB.WithContext(ctx).Where("task_row_id = ? AND cache_pending = ?", taskRowID, true).
		First(&accounting).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if err := invalidateUserCache(accounting.UserID); err != nil {
		return err
	}
	if accounting.TokenID > 0 && common.RedisEnabled {
		var token Token
		result := DB.WithContext(ctx).Unscoped().Where("id = ?", accounting.TokenID).Limit(1).Find(&token)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 && token.Key != "" {
			if err := cacheDeleteToken(token.Key); err != nil {
				return err
			}
		}
	}
	return DB.WithContext(ctx).Model(&TaskAccounting{}).
		Where("task_row_id = ? AND cache_pending = ? AND money_applied = ?", taskRowID, true, accounting.MoneyApplied).
		Update("cache_pending", false).Error
}

func RecoverTaskAccounting(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var pending []TaskAccounting
	if err := DB.WithContext(ctx).Where("decision_id <> ? AND money_applied = ?", "", false).
		Order("decided_at asc").Limit(limit).Find(&pending).Error; err != nil {
		return err
	}
	var firstErr error
	for _, accounting := range pending {
		if _, err := ApplyTaskAccountingDecision(ctx, accounting.TaskRowID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	pending = nil
	if err := DB.WithContext(ctx).Where("cache_pending = ?", true).
		Order("money_applied_at asc").Limit(limit).Find(&pending).Error; err != nil && firstErr == nil {
		firstErr = err
	}
	for _, accounting := range pending {
		if err := ReconcileTaskAccountingCache(ctx, accounting.TaskRowID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := DeliverPendingTaskAccountingLogs(ctx, limit); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func GetTaskAccounting(taskRowID int64) (*TaskAccounting, error) {
	var accounting TaskAccounting
	if err := DB.Where("task_row_id = ?", taskRowID).First(&accounting).Error; err != nil {
		return nil, err
	}
	return &accounting, nil
}
