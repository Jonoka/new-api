package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupNotificationServiceTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	dsn := fmt.Sprintf("file:notification-service-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, db.AutoMigrate(
		&model.NotificationBot{},
		&model.NotificationTask{},
		&model.NotificationTarget{},
		&model.NotificationEventReceipt{},
		&model.NotificationEvent{},
		&model.NotificationDelivery{},
	))
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = originalDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})
}

func createNotificationServiceWork(t *testing.T, eventKey string) model.NotificationDeliveryWork {
	t.Helper()
	bot := &model.NotificationBot{Name: "service bot", Token: "test-token", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	task := &model.NotificationTask{
		Name: "service task", EventType: model.NotificationEventTypeInvoicePending,
		BotId: bot.Id, Template: "{{invoice_id}}", Enabled: true,
	}
	require.NoError(t, model.CreateNotificationTask(task))
	target := &model.NotificationTarget{TaskId: task.Id, ChatId: "-10001", Enabled: true}
	require.NoError(t, model.CreateNotificationTarget(target))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.EnqueueNotificationEventTx(tx, model.NotificationEventTypeInvoicePending, eventKey, map[string]any{"invoice_id": "1"})
	}))
	work, err := model.ClaimNotificationDeliveries(1, time.Now().Unix(), 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	return work[0]
}

func useTelegramTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	originalBase := notificationTelegramAPIBase
	originalClient := httpClient
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	notificationTelegramAPIBase = server.URL
	httpClient = server.Client()
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		server.Close()
		notificationTelegramAPIBase = originalBase
		httpClient = originalClient
		*fetchSetting = originalFetchSetting
	})
	return server
}

func TestRenderNotificationTemplateEscapesPayloadAndBuildsMention(t *testing.T) {
	target := &model.NotificationTarget{MentionUserId: "42", MentionName: `<管理员>`}
	content, err := RenderNotificationTemplate(`{{mention}} 标题：{{title}} 金额：{{total_amount}}`, map[string]any{
		"title":        `<script>alert(1)</script>`,
		"total_amount": 88.5,
	}, target)
	require.NoError(t, err)
	require.Contains(t, content, `href="tg://user?id=42"`)
	require.Contains(t, content, "&lt;管理员&gt;")
	require.Contains(t, content, "&lt;script&gt;alert(1)&lt;/script&gt;")
	require.NotContains(t, content, "<script>")
}

func TestRenderNotificationTemplateRejectsUnknownVariable(t *testing.T) {
	_, err := RenderNotificationTemplate(`{{missing}}`, nil, nil)
	require.ErrorContains(t, err, "unknown notification template variable")
}

func TestRenderNotificationTemplateRejectsNegativeMentionUserID(t *testing.T) {
	_, err := RenderNotificationTemplate(`{{mention}}`, nil, &model.NotificationTarget{MentionUserId: "-42"})
	require.ErrorContains(t, err, "positive number")
}

func TestSendTelegramNotificationUsesHTMLAndParsesSuccess(t *testing.T) {
	var body string
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body = string(data)
		require.Equal(t, "/bottest-token/sendMessage", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))

	require.NoError(t, SendTelegramNotification("test-token", "-10001", "<b>hello</b>"))
	require.Contains(t, body, `"parse_mode":"HTML"`)
	require.Contains(t, body, `"chat_id":"-10001"`)
}

func TestSendTelegramNotificationHandlesRetryAfter(t *testing.T) {
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
	}))

	err := SendTelegramNotification("test-token", "-10001", "hello")
	require.Error(t, err)
	deliveryErr, ok := err.(*telegramDeliveryError)
	require.True(t, ok)
	require.True(t, deliveryErr.retryable)
	require.Equal(t, 7, deliveryErr.retryAfter)
	require.NotContains(t, err.Error(), "test-token")
}

func TestSendTelegramNotificationRejectsOversizedMessage(t *testing.T) {
	err := SendTelegramNotification("test-token", "-10001", strings.Repeat("a", 4097))
	require.ErrorContains(t, err, "exceeds 4096")
}

func TestTelegramNetworkErrorDoesNotLeakToken(t *testing.T) {
	server := useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	err := SendTelegramNotification("highly-secret-token", "-10001", "hello")
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "highly-secret-token"))
	deliveryErr, ok := err.(*telegramDeliveryError)
	require.True(t, ok)
	require.False(t, deliveryErr.retryable)
}

func TestDispatchNotificationDeliveryTransitionsClaimedToSuccess(t *testing.T) {
	setupNotificationServiceTestDB(t)
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	work := createNotificationServiceWork(t, "invoice:dispatch-success")
	require.Equal(t, model.NotificationDeliveryClaimed, work.Delivery.Status)

	dispatchNotificationDelivery(work)

	var delivery model.NotificationDelivery
	require.NoError(t, model.DB.First(&delivery, work.Delivery.Id).Error)
	require.Equal(t, model.NotificationDeliverySuccess, delivery.Status)
	require.Equal(t, 1, delivery.AttemptCount)
}

func TestDispatchNotificationDeliveryPersistsRetryableFailure(t *testing.T) {
	setupNotificationServiceTestDB(t)
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
	}))
	work := createNotificationServiceWork(t, "invoice:dispatch-retry")

	dispatchNotificationDelivery(work)

	var delivery model.NotificationDelivery
	require.NoError(t, model.DB.First(&delivery, work.Delivery.Id).Error)
	require.Equal(t, model.NotificationDeliveryRetrying, delivery.Status)
	require.Equal(t, 1, delivery.AttemptCount)
	require.Greater(t, delivery.NextAttemptAt, time.Now().Unix())
}
