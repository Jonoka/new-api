package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	NotificationEndpointTypeTelegram    = "telegram"
	NotificationEventTypeInvoicePending = "invoice_pending"

	NotificationDeliveryPending  = "pending"
	NotificationDeliveryClaimed  = "claimed"
	NotificationDeliverySending  = "sending"
	NotificationDeliveryRetrying = "retrying"
	NotificationDeliverySuccess  = "success"
	NotificationDeliveryDead     = "dead"
	NotificationDeliveryCanceled = "canceled"

	NotificationEnqueueQueued                    = "queued"
	NotificationEnqueueDuplicate                 = "duplicate"
	NotificationEnqueueAcceptedWithoutSubscriber = "accepted_without_subscriber"

	NotificationEventReceiptTTLSeconds = 90 * 24 * 60 * 60
	notificationReceiptCleanupLimit    = 100
	notificationClaimRetryDelaySeconds = 10
	notificationSequenceLockKey        = "~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"
)

// NotificationTaskDefaultTemplate 是发票待开票通知的默认内容。
const NotificationTaskDefaultTemplate = "{{mention}} 来新的发票订单啦~\n订单：{{invoice_id}}\n金额：{{total_amount}}"

var (
	ErrNotificationStorageUnavailable = errors.New("notification storage is unavailable")
	ErrNotificationDeliveryState      = errors.New("notification delivery state changed")
)

type NotificationEnqueueResult struct {
	Status        string `json:"status"`
	DeliveryCount int    `json:"delivery_count"`
}

type NotificationBot struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"type:varchar(128);not null"`
	Type      string `json:"type" gorm:"type:varchar(32);not null;index"`
	Token     string `json:"-" gorm:"type:text;not null"`
	Enabled   bool   `json:"enabled" gorm:"not null;index"`
	CreatedAt int64  `json:"created_at" gorm:"index"`
	UpdatedAt int64  `json:"updated_at"`
}

// NotificationBotView 可安全返回给管理端，永远不包含 Token。
type NotificationBotView struct {
	NotificationBot
	TokenConfigured bool `json:"token_configured" gorm:"-"`
}

type NotificationTask struct {
	Id                 int    `json:"id" gorm:"primaryKey"`
	Name               string `json:"name" gorm:"type:varchar(128);not null"`
	EventType          string `json:"event_type" gorm:"type:varchar(64);not null;index"`
	BotId              int    `json:"bot_id" gorm:"not null;index"`
	Template           string `json:"template" gorm:"type:text;not null"`
	Enabled            bool   `json:"enabled" gorm:"not null;default:false;index"`
	ActiveAfterEventId int    `json:"-" gorm:"not null;default:0;index"`
	CreatedAt          int64  `json:"created_at" gorm:"index"`
	UpdatedAt          int64  `json:"updated_at"`
}

type NotificationTarget struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	TaskId        int    `json:"task_id" gorm:"not null;uniqueIndex:idx_notification_target,priority:1;index"`
	ChatId        string `json:"chat_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_notification_target,priority:2"`
	MentionUserId string `json:"mention_user_id,omitempty" gorm:"type:varchar(128);uniqueIndex:idx_notification_target,priority:3"`
	MentionName   string `json:"mention_name,omitempty" gorm:"type:varchar(128)"`
	Enabled       bool   `json:"enabled" gorm:"not null;index"`
	CreatedAt     int64  `json:"created_at" gorm:"index"`
	UpdatedAt     int64  `json:"updated_at"`
}

type NotificationEvent struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	EventType string `json:"event_type" gorm:"type:varchar(64);not null;uniqueIndex:idx_notification_event,priority:1;index"`
	EventKey  string `json:"event_key" gorm:"type:varchar(255);not null;uniqueIndex:idx_notification_event,priority:2"`
	Payload   string `json:"-" gorm:"type:text;not null"`
	CreatedAt int64  `json:"created_at" gorm:"index"`
}

// NotificationEventReceipt 保存不可逆事件哈希和插入所有权，按有限窗口去重。
type NotificationEventReceipt struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	DedupeKey  string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	ClaimToken string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	CreatedAt  int64  `json:"created_at" gorm:"index"`
}

type NotificationDelivery struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	EventId       int    `json:"event_id" gorm:"not null;uniqueIndex:idx_notification_delivery,priority:1;index"`
	TaskId        int    `json:"task_id" gorm:"not null;uniqueIndex:idx_notification_delivery,priority:2;index"`
	TargetId      int    `json:"target_id" gorm:"not null;uniqueIndex:idx_notification_delivery,priority:3;index"`
	Status        string `json:"status" gorm:"type:varchar(32);not null;index"`
	AttemptCount  int    `json:"attempt_count" gorm:"not null;default:0"`
	NextAttemptAt int64  `json:"next_attempt_at" gorm:"index"`
	LastError     string `json:"last_error,omitempty" gorm:"type:text"`
	SentAt        int64  `json:"sent_at,omitempty" gorm:"index"`
	CreatedAt     int64  `json:"created_at" gorm:"index"`
	UpdatedAt     int64  `json:"updated_at" gorm:"index"`
}

type NotificationDeliveryWork struct {
	Delivery NotificationDelivery
	Event    NotificationEvent
	Task     NotificationTask
	Target   NotificationTarget
	Bot      NotificationBot
}

func nowUnix() int64 { return time.Now().Unix() }

// LockNotificationSequenceTx 串行化订阅激活和事件入队。
// SQLite 由锁行写入串行化，MySQL 和 PostgreSQL 再显式锁定该行。
func LockNotificationSequenceTx(tx *gorm.DB) error {
	if tx == nil {
		return errors.New("notification transaction is nil")
	}
	lockReceipt := NotificationEventReceipt{
		DedupeKey:  notificationSequenceLockKey,
		ClaimToken: "sequence-lock",
		CreatedAt:  nowUnix(),
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lockReceipt).Error; err != nil {
		return err
	}
	query := tx.Where("dedupe_key = ?", notificationSequenceLockKey)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var stored NotificationEventReceipt
	return query.Take(&stored).Error
}

func CreateNotificationBot(bot *NotificationBot) error {
	if bot == nil || strings.TrimSpace(bot.Name) == "" || strings.TrimSpace(bot.Token) == "" {
		return errors.New("notification bot name and token are required")
	}
	if bot.Type == "" {
		bot.Type = NotificationEndpointTypeTelegram
	}
	if bot.Type != NotificationEndpointTypeTelegram {
		return fmt.Errorf("unsupported notification bot type: %s", bot.Type)
	}
	now := nowUnix()
	bot.CreatedAt, bot.UpdatedAt = now, now
	return DB.Create(bot).Error
}

// UpdateNotificationBot updates non-secret fields. A non-empty token replaces the token.
func UpdateNotificationBot(bot *NotificationBot, token *string) error {
	if bot == nil || bot.Id <= 0 || strings.TrimSpace(bot.Name) == "" {
		return errors.New("invalid notification bot")
	}
	updates := map[string]interface{}{"name": strings.TrimSpace(bot.Name), "enabled": bot.Enabled, "updated_at": nowUnix()}
	if token != nil {
		if strings.TrimSpace(*token) == "" {
			return errors.New("notification bot token cannot be empty")
		}
		updates["token"] = strings.TrimSpace(*token)
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var existing NotificationBot
		if err := tx.Select("id").First(&existing, bot.Id).Error; err != nil {
			return err
		}
		result := tx.Model(&NotificationBot{}).Where("id = ?", bot.Id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
}

func NotificationBotViews() ([]NotificationBotView, error) {
	var bots []NotificationBot
	if err := DB.Order("id asc").Find(&bots).Error; err != nil {
		return nil, err
	}
	views := make([]NotificationBotView, 0, len(bots))
	for _, bot := range bots {
		views = append(views, NotificationBotView{NotificationBot: bot, TokenConfigured: strings.TrimSpace(bot.Token) != ""})
	}
	return views, nil
}

func GetNotificationBot(id int) (*NotificationBot, error) {
	var bot NotificationBot
	if err := DB.First(&bot, id).Error; err != nil {
		return nil, err
	}
	return &bot, nil
}

func DeleteNotificationBot(id int) error {
	if id <= 0 {
		return errors.New("invalid notification bot id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&NotificationTask{}).Where("bot_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("notification bot is still referenced by tasks")
		}
		result := tx.Delete(&NotificationBot{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func CreateNotificationTask(task *NotificationTask) error {
	if task == nil || strings.TrimSpace(task.Name) == "" || strings.TrimSpace(task.EventType) == "" || task.BotId <= 0 {
		return errors.New("notification task name, event type and bot are required")
	}
	if task.Template == "" {
		task.Template = NotificationTaskDefaultTemplate
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var bot NotificationBot
		if err := tx.Select("id").First(&bot, task.BotId).Error; err != nil {
			return err
		}
		var latest NotificationEvent
		if err := tx.Order("id desc").First(&latest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		task.ActiveAfterEventId = latest.Id
		task.CreatedAt, task.UpdatedAt = nowUnix(), nowUnix()
		return tx.Create(task).Error
	})
}

func ListNotificationTasks() ([]NotificationTask, error) {
	var tasks []NotificationTask
	err := DB.Order("id asc").Find(&tasks).Error
	return tasks, err
}

func NotificationTaskLastTriggeredAt() (map[int]int64, error) {
	var rows []struct {
		TaskID          int   `gorm:"column:task_id"`
		LastTriggeredAt int64 `gorm:"column:last_triggered_at"`
	}
	if err := DB.Model(&NotificationDelivery{}).
		Select("task_id, MAX(created_at) AS last_triggered_at").
		Group("task_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[int]int64, len(rows))
	for _, row := range rows {
		result[row.TaskID] = row.LastTriggeredAt
	}
	return result, nil
}

// CancelNotificationTaskDeliveriesTx cancels work that has not started sending yet.
func CancelNotificationTaskDeliveriesTx(tx *gorm.DB, taskID int, reason string) error {
	if tx == nil || taskID <= 0 {
		return errors.New("invalid notification task cancellation")
	}
	return tx.Model(&NotificationDelivery{}).
		Where("task_id = ? AND status IN ?", taskID, []string{NotificationDeliveryPending, NotificationDeliveryRetrying, NotificationDeliveryClaimed}).
		Updates(map[string]interface{}{
			"status":     NotificationDeliveryCanceled,
			"last_error": truncateNotificationError(reason),
			"updated_at": nowUnix(),
		}).Error
}

func UpdateNotificationTask(task *NotificationTask) error {
	if task == nil || task.Id <= 0 || task.BotId <= 0 || strings.TrimSpace(task.Name) == "" || strings.TrimSpace(task.EventType) == "" {
		return errors.New("invalid notification task")
	}
	if strings.TrimSpace(task.Template) == "" {
		task.Template = NotificationTaskDefaultTemplate
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var current NotificationTask
		if err := tx.First(&current, task.Id).Error; err != nil {
			return err
		}
		var bot NotificationBot
		if err := tx.Select("id").First(&bot, task.BotId).Error; err != nil {
			return err
		}
		eventType := strings.TrimSpace(task.EventType)
		eventTypeChanged := current.EventType != eventType
		updates := map[string]interface{}{
			"name":       strings.TrimSpace(task.Name),
			"event_type": eventType,
			"bot_id":     task.BotId,
			"template":   task.Template,
			"enabled":    task.Enabled,
			"updated_at": nowUnix(),
		}
		if task.Enabled && (!current.Enabled || eventTypeChanged) {
			var latest NotificationEvent
			if err := tx.Order("id desc").First(&latest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			updates["active_after_event_id"] = latest.Id
		}
		if err := tx.Model(&NotificationTask{}).Where("id = ?", task.Id).Updates(updates).Error; err != nil {
			return err
		}
		if !task.Enabled {
			return CancelNotificationTaskDeliveriesTx(tx, task.Id, "notification task disabled")
		}
		if eventTypeChanged {
			return CancelNotificationTaskDeliveriesTx(tx, task.Id, "notification task event type changed")
		}
		return nil
	})
}

func DeleteNotificationTask(id int) error {
	if id <= 0 {
		return errors.New("invalid notification task id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&NotificationDelivery{}).Where("task_id = ? AND status IN ?", id, []string{NotificationDeliveryPending, NotificationDeliveryRetrying, NotificationDeliveryClaimed, NotificationDeliverySending}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errors.New("notification task still has active deliveries")
		}
		if err := tx.Where("task_id = ?", id).Delete(&NotificationDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", id).Delete(&NotificationTarget{}).Error; err != nil {
			return err
		}
		return tx.Delete(&NotificationTask{}, id).Error
	})
}

// SetNotificationTaskEnabled re-baselines a task when it is enabled, so old events are not replayed.
func SetNotificationTaskEnabled(id int, enabled bool) error {
	if id <= 0 {
		return errors.New("invalid notification task id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		updates := map[string]interface{}{"enabled": enabled, "updated_at": nowUnix()}
		if enabled {
			var latest NotificationEvent
			if err := tx.Order("id desc").First(&latest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			updates["active_after_event_id"] = latest.Id
		}
		if err := tx.Model(&NotificationTask{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if !enabled {
			return CancelNotificationTaskDeliveriesTx(tx, id, "notification task disabled")
		}
		return nil
	})
}

func CreateNotificationTarget(target *NotificationTarget) error {
	if target == nil || target.TaskId <= 0 || strings.TrimSpace(target.ChatId) == "" {
		return errors.New("notification target task and chat id are required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		now := nowUnix()
		target.CreatedAt, target.UpdatedAt = now, now
		return tx.Create(target).Error
	})
}

func ListNotificationTargets(taskID int) ([]NotificationTarget, error) {
	var targets []NotificationTarget
	query := DB.Order("id asc")
	if taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	err := query.Find(&targets).Error
	return targets, err
}

func UpdateNotificationTarget(target *NotificationTarget) error {
	if target == nil || target.Id <= 0 || strings.TrimSpace(target.ChatId) == "" {
		return errors.New("invalid notification target")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var existing NotificationTarget
		if err := tx.Select("id").First(&existing, target.Id).Error; err != nil {
			return err
		}
		result := tx.Model(&NotificationTarget{}).Where("id = ?", target.Id).Updates(map[string]interface{}{
			"chat_id":         strings.TrimSpace(target.ChatId),
			"mention_user_id": strings.TrimSpace(target.MentionUserId),
			"mention_name":    strings.TrimSpace(target.MentionName),
			"enabled":         target.Enabled,
			"updated_at":      nowUnix(),
		})
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
}

func DeleteNotificationTarget(id int) error {
	if id <= 0 {
		return errors.New("invalid notification target id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&NotificationDelivery{}).Where("target_id = ? AND status IN ?", id, []string{NotificationDeliveryPending, NotificationDeliveryRetrying, NotificationDeliveryClaimed, NotificationDeliverySending}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errors.New("notification target still has active deliveries")
		}
		result := tx.Delete(&NotificationTarget{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// EnqueueNotificationEventTx 在业务事务内写入幂等事件和投递记录。
// 旧业务调用在迁移表尚未创建时跳过可选通知，避免影响主事务。
func EnqueueNotificationEventTx(tx *gorm.DB, eventType, eventKey string, payload map[string]any) error {
	_, err := enqueueNotificationEventTx(tx, eventType, eventKey, payload)
	if errors.Is(err, ErrNotificationStorageUnavailable) {
		return nil
	}
	return err
}

// EnqueueNotificationEventTxWithResult 返回首次入队、重复或无订阅者状态，供宿主 API 使用。
func EnqueueNotificationEventTxWithResult(tx *gorm.DB, eventType, eventKey string, payload map[string]any) (NotificationEnqueueResult, error) {
	return enqueueNotificationEventTx(tx, eventType, eventKey, payload)
}

func cleanupExpiredNotificationReceiptsTx(tx *gorm.DB, currentDedupeKey string, now int64) error {
	cutoff := now - NotificationEventReceiptTTLSeconds
	if currentDedupeKey != "" {
		if err := tx.Where("dedupe_key = ? AND created_at <= ?", currentDedupeKey, cutoff).Delete(&NotificationEventReceipt{}).Error; err != nil {
			return err
		}
	}
	var expiredIDs []int
	if err := tx.Model(&NotificationEventReceipt{}).
		Where("dedupe_key <> ? AND created_at <= ?", notificationSequenceLockKey, cutoff).
		Order("id asc").Limit(notificationReceiptCleanupLimit).Pluck("id", &expiredIDs).Error; err != nil {
		return err
	}
	if len(expiredIDs) == 0 {
		return nil
	}
	return tx.Where("id IN ?", expiredIDs).Delete(&NotificationEventReceipt{}).Error
}

func ensureNotificationReceiptTx(tx *gorm.DB, dedupeKey string, now int64) error {
	receipt := NotificationEventReceipt{
		DedupeKey:  dedupeKey,
		ClaimToken: common.GetUUID(),
		CreatedAt:  now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt).Error
}

func claimNotificationReceiptTx(tx *gorm.DB, dedupeKey string, now int64) (bool, error) {
	claimToken := common.GetUUID()
	receipt := NotificationEventReceipt{DedupeKey: dedupeKey, ClaimToken: claimToken, CreatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt).Error; err != nil {
		return false, err
	}
	var stored NotificationEventReceipt
	if err := tx.Where("dedupe_key = ?", dedupeKey).Take(&stored).Error; err != nil {
		return false, err
	}
	return stored.ClaimToken == claimToken, nil
}

func enqueueNotificationEventTx(tx *gorm.DB, eventType, eventKey string, payload map[string]any) (NotificationEnqueueResult, error) {
	if tx == nil {
		return NotificationEnqueueResult{}, errors.New("notification transaction is nil")
	}
	if !tx.Migrator().HasTable(&NotificationEvent{}) || !tx.Migrator().HasTable(&NotificationEventReceipt{}) {
		return NotificationEnqueueResult{}, ErrNotificationStorageUnavailable
	}
	eventType, eventKey = strings.TrimSpace(eventType), strings.TrimSpace(eventKey)
	if eventType == "" || eventKey == "" {
		return NotificationEnqueueResult{}, errors.New("notification event type and key are required")
	}
	if err := LockNotificationSequenceTx(tx); err != nil {
		return NotificationEnqueueResult{}, err
	}
	now := nowUnix()
	dedupeKey := notificationEventDedupeKey(eventType, eventKey)
	if err := cleanupExpiredNotificationReceiptsTx(tx, dedupeKey, now); err != nil {
		return NotificationEnqueueResult{}, err
	}
	var existingEventCount int64
	if err := tx.Model(&NotificationEvent{}).Where("event_type = ? AND event_key = ?", eventType, eventKey).Count(&existingEventCount).Error; err != nil {
		return NotificationEnqueueResult{}, err
	}
	if existingEventCount > 0 {
		if err := ensureNotificationReceiptTx(tx, dedupeKey, now); err != nil {
			return NotificationEnqueueResult{}, err
		}
		return NotificationEnqueueResult{Status: NotificationEnqueueDuplicate}, nil
	}
	claimed, err := claimNotificationReceiptTx(tx, dedupeKey, now)
	if err != nil {
		return NotificationEnqueueResult{}, err
	}
	if !claimed {
		return NotificationEnqueueResult{Status: NotificationEnqueueDuplicate}, nil
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return NotificationEnqueueResult{}, fmt.Errorf("marshal notification event: %w", err)
	}
	event := NotificationEvent{EventType: eventType, EventKey: eventKey, Payload: string(data), CreatedAt: now}
	if err := tx.Create(&event).Error; err != nil {
		return NotificationEnqueueResult{}, err
	}
	var targets []NotificationTarget
	if err := tx.Table("notification_targets AS target").
		Select("target.*").
		Joins("JOIN notification_tasks AS task ON task.id = target.task_id").
		Joins("JOIN notification_bots AS bot ON bot.id = task.bot_id").
		Where("task.enabled = ? AND target.enabled = ? AND bot.enabled = ? AND task.event_type = ? AND task.active_after_event_id < ?", true, true, true, eventType, event.Id).
		Find(&targets).Error; err != nil {
		return NotificationEnqueueResult{}, err
	}
	if len(targets) == 0 {
		// 没有启用的接收目标时只保留哈希收据，不积累事件 payload。
		if err := tx.Delete(&event).Error; err != nil {
			return NotificationEnqueueResult{}, err
		}
		return NotificationEnqueueResult{Status: NotificationEnqueueAcceptedWithoutSubscriber}, nil
	}
	deliveryCount := 0
	for _, target := range targets {
		delivery := NotificationDelivery{EventId: event.Id, TaskId: target.TaskId, TargetId: target.Id, Status: NotificationDeliveryPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&delivery).Error; err != nil {
			return NotificationEnqueueResult{}, err
		}
		deliveryCount++
	}
	return NotificationEnqueueResult{Status: NotificationEnqueueQueued, DeliveryCount: deliveryCount}, nil
}

func notificationEventDedupeKey(eventType, eventKey string) string {
	sum := sha256.Sum256([]byte(eventType + "\x00" + eventKey))
	return hex.EncodeToString(sum[:])
}

func ClaimNotificationDeliveries(limit int, now int64, leaseSeconds int64) ([]NotificationDeliveryWork, error) {
	if limit <= 0 {
		limit = 50
	}
	if now <= 0 {
		now = nowUnix()
	}
	if leaseSeconds <= 0 {
		leaseSeconds = 120
	}
	var staleClaimedIDs []int
	if err := DB.Model(&NotificationDelivery{}).
		Where("status = ? AND updated_at <= ?", NotificationDeliveryClaimed, now-leaseSeconds).
		Limit(limit).Pluck("id", &staleClaimedIDs).Error; err != nil {
		return nil, err
	}
	if len(staleClaimedIDs) > 0 {
		releaseResult := DB.Model(&NotificationDelivery{}).
			Where("id IN ? AND status = ?", staleClaimedIDs, NotificationDeliveryClaimed).
			Updates(map[string]interface{}{
				"status":          NotificationDeliveryRetrying,
				"next_attempt_at": now,
				"last_error":      "delivery claim expired before sending; retrying safely",
				"updated_at":      now,
			})
		if releaseResult.Error != nil {
			return nil, releaseResult.Error
		}
	}

	var staleSending []NotificationDelivery
	if err := DB.Select("id", "task_id").
		Where("status = ? AND updated_at <= ?", NotificationDeliverySending, now-leaseSeconds).
		Limit(limit).Find(&staleSending).Error; err != nil {
		return nil, err
	}
	for _, delivery := range staleSending {
		err := MarkNotificationDeliveryDead(delivery.Id, "delivery result is unknown after worker interruption; automatic retry was skipped to avoid duplicate notification")
		if err != nil && !errors.Is(err, ErrNotificationDeliveryState) {
			return nil, err
		}
	}

	var candidates []NotificationDelivery
	if err := DB.Where("status IN ? AND next_attempt_at <= ?", []string{NotificationDeliveryPending, NotificationDeliveryRetrying}, now).
		Order("id asc").Limit(limit).Find(&candidates).Error; err != nil {
		return nil, err
	}
	claimed := make([]NotificationDeliveryWork, 0, len(candidates))
	var claimErrors error
	for _, candidate := range candidates {
		result := DB.Model(&NotificationDelivery{}).Where("id = ? AND status = ? AND attempt_count = ? AND updated_at = ?", candidate.Id, candidate.Status, candidate.AttemptCount, candidate.UpdatedAt).
			Updates(map[string]interface{}{"status": NotificationDeliveryClaimed, "updated_at": now})
		if result.Error != nil {
			claimErrors = errors.Join(claimErrors, result.Error)
			continue
		}
		if result.RowsAffected != 1 {
			continue
		}
		var work NotificationDeliveryWork
		loadErr := DB.First(&work.Delivery, candidate.Id).Error
		if loadErr == nil {
			loadErr = DB.First(&work.Event, work.Delivery.EventId).Error
		}
		if loadErr == nil {
			loadErr = DB.First(&work.Task, work.Delivery.TaskId).Error
		}
		if loadErr == nil {
			loadErr = DB.First(&work.Target, work.Delivery.TargetId).Error
		}
		if loadErr == nil {
			loadErr = DB.First(&work.Bot, work.Task.BotId).Error
		}
		if loadErr != nil {
			var releaseErr error
			if errors.Is(loadErr, gorm.ErrRecordNotFound) {
				releaseErr = MarkNotificationDeliveryDead(candidate.Id, "notification delivery relation is missing: "+loadErr.Error())
			} else {
				releaseErr = ReleaseNotificationDeliveryClaim(candidate.Id, now+notificationClaimRetryDelaySeconds, "load notification delivery failed: "+loadErr.Error())
			}
			claimErrors = errors.Join(claimErrors, loadErr, releaseErr)
			continue
		}
		claimed = append(claimed, work)
	}
	return claimed, claimErrors
}

func notificationDeliveryStateError(id int) error {
	return fmt.Errorf("%w: delivery %d", ErrNotificationDeliveryState, id)
}

func MarkNotificationDeliverySending(id int, startedAt int64) (int, error) {
	if startedAt <= 0 {
		startedAt = nowUnix()
	}
	attemptCount := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&NotificationDelivery{}).Where("id = ? AND status = ?", id, NotificationDeliveryClaimed).
			Updates(map[string]interface{}{
				"status":        NotificationDeliverySending,
				"attempt_count": gorm.Expr("attempt_count + ?", 1),
				"updated_at":    startedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return notificationDeliveryStateError(id)
		}
		var delivery NotificationDelivery
		if err := tx.Select("attempt_count").First(&delivery, id).Error; err != nil {
			return err
		}
		attemptCount = delivery.AttemptCount
		return nil
	})
	return attemptCount, err
}

func ReleaseNotificationDeliveryClaim(id int, nextAttemptAt int64, lastError string) error {
	if nextAttemptAt <= 0 {
		nextAttemptAt = nowUnix() + notificationClaimRetryDelaySeconds
	}
	result := DB.Model(&NotificationDelivery{}).Where("id = ? AND status = ?", id, NotificationDeliveryClaimed).
		Updates(map[string]interface{}{
			"status":          NotificationDeliveryRetrying,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      nowUnix(),
			"last_error":      truncateNotificationError(lastError),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return notificationDeliveryStateError(id)
	}
	return nil
}

func MarkNotificationDeliveryRetry(id int, nextAttemptAt int64, lastError string) error {
	if nextAttemptAt <= 0 {
		nextAttemptAt = nowUnix() + 60
	}
	result := DB.Model(&NotificationDelivery{}).Where("id = ? AND status = ?", id, NotificationDeliverySending).
		Updates(map[string]interface{}{
			"status":          NotificationDeliveryRetrying,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      nowUnix(),
			"last_error":      truncateNotificationError(lastError),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return notificationDeliveryStateError(id)
	}
	return nil
}

func markNotificationDeliveryTerminal(id int, fromStatuses []string, status string, updates map[string]interface{}) error {
	taskID := 0
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var delivery NotificationDelivery
		if err := tx.Select("task_id").First(&delivery, id).Error; err != nil {
			return err
		}
		updates["status"] = status
		result := tx.Model(&NotificationDelivery{}).Where("id = ? AND status IN ?", id, fromStatuses).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return notificationDeliveryStateError(id)
		}
		taskID = delivery.TaskId
		return nil
	}); err != nil {
		return err
	}
	return PruneNotificationHistoryForTask(taskID)
}

func MarkNotificationDeliverySuccess(id int, sentAt int64) error {
	if sentAt <= 0 {
		sentAt = nowUnix()
	}
	return markNotificationDeliveryTerminal(
		id,
		[]string{NotificationDeliverySending},
		NotificationDeliverySuccess,
		map[string]interface{}{"sent_at": sentAt, "updated_at": sentAt, "last_error": ""},
	)
}

func MarkNotificationDeliveryDead(id int, lastError string) error {
	now := nowUnix()
	return markNotificationDeliveryTerminal(
		id,
		[]string{NotificationDeliveryClaimed, NotificationDeliverySending},
		NotificationDeliveryDead,
		map[string]interface{}{"updated_at": now, "last_error": truncateNotificationError(lastError)},
	)
}

func MarkNotificationDeliveryCanceled(id int, reason string) error {
	now := nowUnix()
	return markNotificationDeliveryTerminal(
		id,
		[]string{NotificationDeliveryClaimed, NotificationDeliverySending},
		NotificationDeliveryCanceled,
		map[string]interface{}{"updated_at": now, "last_error": truncateNotificationError(reason)},
	)
}

func truncateNotificationError(value string) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "�")
	for len(value) > 2048 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

func pruneNotificationHistoryForTaskTx(tx *gorm.DB, taskID int) error {
	if tx == nil || taskID <= 0 {
		return nil
	}
	terminal := []string{NotificationDeliverySuccess, NotificationDeliveryDead, NotificationDeliveryCanceled}
	var oldEventIDs []int
	query := tx.Model(&NotificationDelivery{}).Select("event_id").Where("task_id = ?", taskID).Group("event_id").
		Having("SUM(CASE WHEN status NOT IN ? THEN 1 ELSE 0 END) = 0", terminal).Order("MAX(id) DESC").Offset(5).Pluck("event_id", &oldEventIDs)
	if query.Error != nil {
		return query.Error
	}
	for _, eventID := range oldEventIDs {
		if err := tx.Where("task_id = ? AND event_id = ? AND status IN ?", taskID, eventID, terminal).Delete(&NotificationDelivery{}).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&NotificationDelivery{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := tx.Delete(&NotificationEvent{}, eventID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// PruneNotificationHistoryForTask 只清理指定任务超过五个终态事件的历史记录。
func PruneNotificationHistoryForTask(taskID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := LockNotificationSequenceTx(tx); err != nil {
			return err
		}
		return pruneNotificationHistoryForTaskTx(tx, taskID)
	})
}

// PruneNotificationHistoryForBot 只清理指定 Bot 所属任务的历史记录。
func PruneNotificationHistoryForBot(botID int) error {
	var taskIDs []int
	if err := DB.Model(&NotificationTask{}).Where("bot_id = ?", botID).Pluck("id", &taskIDs).Error; err != nil {
		return err
	}
	for _, taskID := range taskIDs {
		if err := PruneNotificationHistoryForTask(taskID); err != nil {
			return err
		}
	}
	return nil
}

func ListNotificationHistory(taskID, limit int) ([]NotificationDelivery, error) {
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	var items []NotificationDelivery
	query := DB.Where("status IN ?", []string{NotificationDeliverySuccess, NotificationDeliveryDead, NotificationDeliveryCanceled})
	if taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	err := query.Order("id desc").Limit(limit).Find(&items).Error
	return items, err
}
