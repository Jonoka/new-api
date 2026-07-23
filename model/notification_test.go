package model

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupNotificationTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	dsn := fmt.Sprintf("file:notification-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, DB.AutoMigrate(
		&NotificationBot{},
		&NotificationTask{},
		&NotificationTarget{},
		&NotificationEventReceipt{},
		&NotificationEvent{},
		&NotificationDelivery{},
	))

	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB = originalDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})
}

func createNotificationFixture(t *testing.T) (*NotificationBot, *NotificationTask, *NotificationTarget) {
	t.Helper()
	bot := &NotificationBot{Name: "invoice bot", Token: "secret-token", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	task := &NotificationTask{Name: "invoice pending", EventType: NotificationEventTypeInvoicePending, BotId: bot.Id, Enabled: true}
	require.NoError(t, CreateNotificationTask(task))
	target := &NotificationTarget{TaskId: task.Id, ChatId: "-10001", MentionUserId: "42", MentionName: "管理员", Enabled: true}
	require.NoError(t, CreateNotificationTarget(target))
	return bot, task, target
}

func TestNotificationBotTokenIsNeverMarshaled(t *testing.T) {
	bot := NotificationBot{Id: 1, Name: "bot", Type: NotificationEndpointTypeTelegram, Token: "must-not-leak", Enabled: true}
	data, err := common.Marshal(bot)
	require.NoError(t, err)
	require.NotContains(t, string(data), "must-not-leak")
	require.NotContains(t, string(data), "token")
}

func TestNotificationTaskOnlyReceivesNewEvents(t *testing.T) {
	setupNotificationTestDB(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:old", map[string]any{"invoice_id": 1})
	}))
	_, task, _ := createNotificationFixture(t)

	var deliveries int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.Zero(t, deliveries)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:new", map[string]any{"invoice_id": 2})
	}))
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 1, deliveries)

	require.NoError(t, SetNotificationTaskEnabled(task.Id, false))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:disabled", map[string]any{"invoice_id": 3})
	}))
	require.NoError(t, SetNotificationTaskEnabled(task.Id, true))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:reenabled", map[string]any{"invoice_id": 4})
	}))
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 2, deliveries)
}

func TestUpdateNotificationTaskCancelsDeliveriesFromOldEventType(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, _ := createNotificationFixture(t)
	for i := 1; i <= 3; i++ {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, fmt.Sprintf("invoice:event-type-%d", i), map[string]any{"invoice_id": i})
		}))
	}

	var deliveries []NotificationDelivery
	require.NoError(t, DB.Order("id asc").Find(&deliveries).Error)
	require.Len(t, deliveries, 3)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveries[1].Id).Update("status", NotificationDeliveryRetrying).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveries[2].Id).Update("status", NotificationDeliveryClaimed).Error)

	task.EventType = "extension.test.created"
	require.NoError(t, UpdateNotificationTask(task))

	var canceled []NotificationDelivery
	require.NoError(t, DB.Order("id asc").Find(&canceled).Error)
	require.Len(t, canceled, 3)
	for _, delivery := range canceled {
		require.Equal(t, NotificationDeliveryCanceled, delivery.Status)
		require.Equal(t, "notification task event type changed", delivery.LastError)
	}

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:old-type-after-update", map[string]any{"invoice_id": 4})
	}))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, "extension.test.created", "extension:test:new-type", map[string]any{"id": 1})
	}))
	var deliveryCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.EqualValues(t, 4, deliveryCount, "旧事件类型不应再投递，新事件类型应从新的基线开始")
}

func TestNotificationTaskLastTriggeredAtUsesLatestDelivery(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, _ := createNotificationFixture(t)
	triggeredAt, err := NotificationTaskLastTriggeredAt()
	require.NoError(t, err)
	require.NotContains(t, triggeredAt, task.Id)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:last-triggered", map[string]any{"invoice_id": 5})
	}))
	triggeredAt, err = NotificationTaskLastTriggeredAt()
	require.NoError(t, err)
	require.Positive(t, triggeredAt[task.Id])
}

func TestCreateNotificationTaskRejectsMissingBotInsideSequence(t *testing.T) {
	setupNotificationTestDB(t)
	task := &NotificationTask{Name: "orphan", EventType: "event", BotId: 999, Enabled: true}
	require.ErrorIs(t, CreateNotificationTask(task), gorm.ErrRecordNotFound)
}

func TestEnqueueNotificationEventIsIdempotent(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	for range 3 {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:dedupe", map[string]any{"invoice_id": 9})
		}))
	}
	var eventCount, deliveryCount int64
	require.NoError(t, DB.Model(&NotificationEvent{}).Count(&eventCount).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.EqualValues(t, 1, eventCount)
	require.EqualValues(t, 1, deliveryCount)
}

func TestDisabledBotDoesNotRetainNotificationEvent(t *testing.T) {
	setupNotificationTestDB(t)
	bot, _, _ := createNotificationFixture(t)
	require.NoError(t, DB.Model(&NotificationBot{}).Where("id = ?", bot.Id).Update("enabled", false).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:disabled-bot", map[string]any{"invoice_id": 10})
	}))

	var eventCount, deliveryCount int64
	require.NoError(t, DB.Model(&NotificationEvent{}).Count(&eventCount).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.Zero(t, eventCount)
	require.Zero(t, deliveryCount)
}

func TestStaleSendingDeliveryBecomesDeadWithoutAutomaticRetry(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:unknown-result", map[string]any{"invoice_id": 11})
	}))
	now := time.Now().Unix()
	work, err := ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	deliveryID := work[0].Delivery.Id
	_, err = MarkNotificationDeliverySending(deliveryID, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveryID).Update("updated_at", now-180).Error)

	work, err = ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Empty(t, work)
	var delivery NotificationDelivery
	require.NoError(t, DB.First(&delivery, deliveryID).Error)
	require.Equal(t, NotificationDeliveryDead, delivery.Status)
	require.Contains(t, delivery.LastError, "duplicate notification")
}

func TestNotificationHistoryKeepsFiveTerminalEventsAndPending(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, _ := createNotificationFixture(t)
	for i := 1; i <= 7; i++ {
		key := fmt.Sprintf("invoice:%d", i)
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, key, map[string]any{"invoice_id": i})
		}))
	}
	work, err := ClaimNotificationDeliveries(20, 0, 120)
	require.NoError(t, err)
	require.Len(t, work, 7)
	for _, item := range work {
		_, err := MarkNotificationDeliverySending(item.Delivery.Id, 0)
		require.NoError(t, err)
		require.NoError(t, MarkNotificationDeliverySuccess(item.Delivery.Id, 0))
	}
	var terminalCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ?", task.Id).Count(&terminalCount).Error)
	require.EqualValues(t, 5, terminalCount)
	var receiptCount int64
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key <> ?", notificationSequenceLockKey).Count(&receiptCount).Error)
	require.EqualValues(t, 7, receiptCount)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:1", map[string]any{"invoice_id": 1})
	}))
	work, err = ClaimNotificationDeliveries(20, 0, 120)
	require.NoError(t, err)
	require.Empty(t, work, "历史 payload 被裁剪后仍必须通过哈希凭据保持幂等")

	for i := 8; i <= 9; i++ {
		key := fmt.Sprintf("invoice:%d", i)
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, key, map[string]any{"invoice_id": i})
		}))
	}
	work, err = ClaimNotificationDeliveries(20, 0, 120)
	require.NoError(t, err)
	require.Len(t, work, 2)
	_, err = MarkNotificationDeliverySending(work[0].Delivery.Id, 0)
	require.NoError(t, err)
	_, err = MarkNotificationDeliverySending(work[1].Delivery.Id, 0)
	require.NoError(t, err)
	require.NoError(t, MarkNotificationDeliveryRetry(work[0].Delivery.Id, 9999999999, "temporary"))
	require.NoError(t, MarkNotificationDeliverySuccess(work[1].Delivery.Id, 0))

	var retryCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ? AND status = ?", task.Id, NotificationDeliveryRetrying).Count(&retryCount).Error)
	require.EqualValues(t, 1, retryCount)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ? AND status IN ?", task.Id, []string{NotificationDeliverySuccess, NotificationDeliveryDead, NotificationDeliveryCanceled}).Count(&terminalCount).Error)
	require.EqualValues(t, 5, terminalCount)
}
func TestEnqueueNotificationEventReturnsStatus(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)

	var first NotificationEnqueueResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, "invoice:status", map[string]any{"invoice_id": 1})
		return err
	}))
	require.Equal(t, NotificationEnqueueQueued, first.Status)
	require.Equal(t, 1, first.DeliveryCount)

	var duplicate NotificationEnqueueResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		duplicate, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, "invoice:status", map[string]any{"invoice_id": 1})
		return err
	}))
	require.Equal(t, NotificationEnqueueDuplicate, duplicate.Status)
	require.Zero(t, duplicate.DeliveryCount)

	var withoutSubscriber NotificationEnqueueResult
	require.NoError(t, DB.Model(&NotificationTask{}).Where("event_type = ?", NotificationEventTypeInvoicePending).Update("enabled", false).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		withoutSubscriber, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, "invoice:no-subscriber", map[string]any{"invoice_id": 2})
		return err
	}))
	require.Equal(t, NotificationEnqueueAcceptedWithoutSubscriber, withoutSubscriber.Status)
	require.Zero(t, withoutSubscriber.DeliveryCount)
}

func TestNotificationReceiptClaimUsesStoredToken(t *testing.T) {
	setupNotificationTestDB(t)
	now := time.Now().Unix()
	existingKey := notificationEventDedupeKey("event", "existing")
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: existingKey, ClaimToken: "first-writer", CreatedAt: now,
	}).Error)

	var claimed bool
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		claimed, err = claimNotificationReceiptTx(tx, existingKey, now)
		return err
	}))
	require.False(t, claimed, "重复判断必须以回读的 claim token 为准，不能依赖 RowsAffected")

	newKey := notificationEventDedupeKey("event", "new")
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		claimed, err = claimNotificationReceiptTx(tx, newKey, now)
		return err
	}))
	require.True(t, claimed)
}

func TestExpiredNotificationReceiptsAreCleanedWithoutSubscriber(t *testing.T) {
	setupNotificationTestDB(t)
	now := time.Now().Unix()
	eventType := "extension.test.created"
	eventKey := "without-subscriber"
	currentExpiredKey := notificationEventDedupeKey(eventType, eventKey)
	otherExpiredKey := notificationEventDedupeKey("event", "expired")
	freshKey := notificationEventDedupeKey("event", "fresh")
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: currentExpiredKey, ClaimToken: "expired-current", CreatedAt: now - NotificationEventReceiptTTLSeconds - 1,
	}).Error)
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: otherExpiredKey, ClaimToken: "expired-other", CreatedAt: now - NotificationEventReceiptTTLSeconds - 1,
	}).Error)
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: freshKey, ClaimToken: "fresh", CreatedAt: now,
	}).Error)

	var result NotificationEnqueueResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = EnqueueNotificationEventTxWithResult(tx, eventType, eventKey, map[string]any{"id": "1"})
		return err
	}))
	require.Equal(t, NotificationEnqueueAcceptedWithoutSubscriber, result.Status)

	var currentCount, expiredCount, freshCount, lockCount int64
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", currentExpiredKey).Count(&currentCount).Error)
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", otherExpiredKey).Count(&expiredCount).Error)
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", freshKey).Count(&freshCount).Error)
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", notificationSequenceLockKey).Count(&lockCount).Error)
	require.EqualValues(t, 1, currentCount, "超过 TTL 后相同事件键应作为新事件重新写入收据")
	require.Zero(t, expiredCount)
	require.EqualValues(t, 1, freshCount)
	require.EqualValues(t, 1, lockCount)
}

func TestClaimedDeliveryIsSafelyReclaimedBeforeSending(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:reclaim", map[string]any{"invoice_id": 12})
	}))
	now := time.Now().Unix()
	work, err := ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	deliveryID := work[0].Delivery.Id
	require.Equal(t, NotificationDeliveryClaimed, work[0].Delivery.Status)
	require.Zero(t, work[0].Delivery.AttemptCount)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveryID).Update("updated_at", now-180).Error)

	work, err = ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	require.Equal(t, deliveryID, work[0].Delivery.Id)
	require.Equal(t, NotificationDeliveryClaimed, work[0].Delivery.Status)
	require.Zero(t, work[0].Delivery.AttemptCount)
}

func TestClaimMissingRelationDoesNotLeaveSendingDelivery(t *testing.T) {
	setupNotificationTestDB(t)
	now := time.Now().Unix()
	delivery := NotificationDelivery{
		EventId: 999, TaskId: 999, TargetId: 999, Status: NotificationDeliveryPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(&delivery).Error)

	work, err := ClaimNotificationDeliveries(10, now, 120)
	require.Error(t, err)
	require.Empty(t, work)
	require.NoError(t, DB.First(&delivery, delivery.Id).Error)
	require.Equal(t, NotificationDeliveryDead, delivery.Status)
	require.NotEqual(t, NotificationDeliverySending, delivery.Status)
	require.NotEqual(t, NotificationDeliveryClaimed, delivery.Status)
}

func TestNotificationDeliveryRequiresSendingTransition(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:transition", map[string]any{"invoice_id": 13})
	}))
	work, err := ClaimNotificationDeliveries(10, 0, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	deliveryID := work[0].Delivery.Id
	require.ErrorIs(t, MarkNotificationDeliverySuccess(deliveryID, 0), ErrNotificationDeliveryState)
	attempt, err := MarkNotificationDeliverySending(deliveryID, 0)
	require.NoError(t, err)
	require.Equal(t, 1, attempt)
	_, err = MarkNotificationDeliverySending(deliveryID, 0)
	require.ErrorIs(t, err, ErrNotificationDeliveryState)
	require.NoError(t, MarkNotificationDeliverySuccess(deliveryID, 0))
	require.ErrorIs(t, MarkNotificationDeliverySuccess(deliveryID, 0), ErrNotificationDeliveryState)
}

func TestTruncateNotificationErrorPreservesUTF8(t *testing.T) {
	value := strings.Repeat("a", 2047) + "通知"
	truncated := truncateNotificationError(value)
	require.LessOrEqual(t, len(truncated), 2048)
	require.True(t, utf8.ValidString(truncated))
}

func TestPruneNotificationHistoryOnlyTouchesRequestedTask(t *testing.T) {
	setupNotificationTestDB(t)
	bot := &NotificationBot{Name: "prune bot", Token: "token", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	taskOne := &NotificationTask{Name: "one", EventType: "event", BotId: bot.Id, Enabled: true}
	taskTwo := &NotificationTask{Name: "two", EventType: "event", BotId: bot.Id, Enabled: true}
	require.NoError(t, CreateNotificationTask(taskOne))
	require.NoError(t, CreateNotificationTask(taskTwo))
	now := time.Now().Unix()
	for index := 0; index < 7; index++ {
		event := NotificationEvent{EventType: "event", EventKey: fmt.Sprintf("event:%d", index), Payload: `{}`, CreatedAt: now + int64(index)}
		require.NoError(t, DB.Create(&event).Error)
		require.NoError(t, DB.Create(&NotificationDelivery{
			EventId: event.Id, TaskId: taskOne.Id, TargetId: index + 1, Status: NotificationDeliverySuccess,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
		require.NoError(t, DB.Create(&NotificationDelivery{
			EventId: event.Id, TaskId: taskTwo.Id, TargetId: index + 100, Status: NotificationDeliverySuccess,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	require.NoError(t, PruneNotificationHistoryForTask(taskOne.Id))
	var taskOneCount, taskTwoCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ?", taskOne.Id).Count(&taskOneCount).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ?", taskTwo.Id).Count(&taskTwoCount).Error)
	require.EqualValues(t, 5, taskOneCount)
	require.EqualValues(t, 7, taskTwoCount)
}

func TestNotificationSequenceLockSerializesSQLiteTransactions(t *testing.T) {
	setupNotificationTestDB(t)
	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- DB.Transaction(func(tx *gorm.DB) error {
			if err := LockNotificationSequenceTx(tx); err != nil {
				return err
			}
			close(firstLocked)
			<-releaseFirst
			return nil
		})
	}()
	<-firstLocked

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- DB.Transaction(func(tx *gorm.DB) error {
			return LockNotificationSequenceTx(tx)
		})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("第二个事务不应越过序列锁: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)

	var lockRows int64
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", notificationSequenceLockKey).Count(&lockRows).Error)
	require.EqualValues(t, 1, lockRows)
}
