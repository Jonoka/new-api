package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskAccountingEvent struct {
	SubmissionID string `json:"submission_id,omitempty" gorm:"type:varchar(64);index;default:''"`
	EventID      string `json:"event_id" gorm:"type:varchar(64);primaryKey"`
	TaskRowID    int64  `json:"task_row_id" gorm:"not null;index"`
	Kind         string `json:"kind" gorm:"type:varchar(20);not null"`
	FactsJSON    string `json:"facts_json" gorm:"type:text;not null"`
	Ready        bool   `json:"ready" gorm:"not null;default:false;index"`
	Delivered    bool   `json:"delivered" gorm:"not null;default:false;index"`
	CreatedAt    int64  `json:"created_at" gorm:"not null"`
	DeliveredAt  int64  `json:"delivered_at" gorm:"not null;default:0"`
}

// TaskAccountingLogReceipt lives in LOG_DB. The claim token, rather than
// RowsAffected, proves which transaction owns the corresponding Log insert.
type TaskAccountingLogReceipt struct {
	EventID    string `json:"event_id" gorm:"type:varchar(64);primaryKey"`
	ClaimToken string `json:"claim_token" gorm:"type:varchar(64);not null"`
	CreatedAt  int64  `json:"created_at" gorm:"not null"`
	LogID      int    `json:"log_id" gorm:"not null;default:0"`
}

func createTaskAccountingEventTx(tx *gorm.DB, taskRowID int64, kind string, facts TaskAccountingLogFacts, ready bool) (*TaskAccountingEvent, error) {
	payload, err := common.Marshal(facts)
	if err != nil {
		return nil, err
	}
	event := &TaskAccountingEvent{
		EventID: common.GetUUID(), TaskRowID: taskRowID, Kind: kind,
		FactsJSON: string(payload), Ready: ready, CreatedAt: getDBTimestampTx(tx),
	}
	if err := tx.Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func DeliverPendingTaskAccountingLogs(ctx context.Context, limit int) error {
	if LOG_DB == nil {
		return errors.New("log database is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	var events []TaskAccountingEvent
	if err := DB.WithContext(ctx).Where("ready = ? AND delivered = ?", true, false).
		Order("created_at asc").Limit(limit).Find(&events).Error; err != nil {
		return err
	}
	var firstErr error
	for i := range events {
		if err := deliverTaskAccountingLog(ctx, &events[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func deliverTaskAccountingLog(ctx context.Context, event *TaskAccountingEvent) error {
	if event == nil || event.EventID == "" {
		return errors.New("task accounting event is invalid")
	}
	var facts TaskAccountingLogFacts
	if err := common.Unmarshal([]byte(event.FactsJSON), &facts); err != nil {
		return err
	}
	createdLog := false
	err := LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claimToken := common.GetUUID()
		receipt := TaskAccountingLogReceipt{EventID: event.EventID, ClaimToken: claimToken, CreatedAt: common.GetTimestamp()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt).Error; err != nil {
			return err
		}
		var stored TaskAccountingLogReceipt
		if err := tx.Where("event_id = ?", event.EventID).Take(&stored).Error; err != nil {
			return err
		}
		if stored.ClaimToken != claimToken {
			return nil
		}
		if facts.LogType == LogTypeConsume && !common.LogConsumeEnabled {
			return nil
		}
		log := &Log{
			PromptTokens: facts.PromptTokens, CompletionTokens: facts.CompletionTokens,
			UseTime: facts.UseTimeSeconds, IsStream: facts.IsStream,
			UserId: facts.UserID, Username: facts.Username, CreatedAt: facts.CreatedAt,
			Type: facts.LogType, Content: facts.Content, TokenName: facts.TokenName,
			ModelName: facts.ModelName, Quota: facts.Quota, ChannelId: facts.ChannelID,
			TokenId: facts.TokenID, Group: facts.Group, Ip: facts.IP,
			RequestId: facts.RequestID, UpstreamRequestId: facts.UpstreamRequestID,
			Other: common.MapToJsonStr(facts.Other),
		}
		if err := tx.Create(log).Error; err != nil {
			return err
		}
		createdLog = true
		return tx.Model(&TaskAccountingLogReceipt{}).Where("event_id = ? AND claim_token = ?", event.EventID, claimToken).
			Update("log_id", log.Id).Error
	})
	if err != nil {
		return err
	}
	if createdLog && common.DataExportEnabled && (event.Kind == "initial" || event.Kind == "synchronous_image" || event.Kind == "alpha_search" || event.Kind == "violation_fee") {
		gopool.Go(func() {
			LogQuotaData(facts.UserID, facts.Username, facts.ModelName, facts.Quota, facts.CreatedAt, facts.PromptTokens+facts.CompletionTokens)
		})
	}
	result := DB.WithContext(ctx).Model(&TaskAccountingEvent{}).Where("event_id = ? AND delivered = ?", event.EventID, false).
		Updates(map[string]any{"delivered": true, "delivered_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	return nil
}
