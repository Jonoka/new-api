package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPublishExtensionNotificationEventQueuesAndDeduplicates(t *testing.T) {
	originalDB := model.DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:extension-notification-controller?mode=memory&cache=shared"), &gorm.Config{})
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

	originalManager := extension.DefaultManager
	manager := extension.NewManager(t.TempDir())
	moduleDir := filepath.Join(manager.RootDir(), "orders")
	require.NoError(t, os.MkdirAll(moduleDir, 0755))
	manifest := extension.Manifest{
		ID:      "orders",
		Name:    "订单模块",
		Version: "1.0.0",
		Runtime: extension.Runtime{Type: extension.RuntimeTypeStatic},
		Permissions: extension.PermissionConfig{
			Roles:        []string{"root"},
			Capabilities: []string{extension.CapabilityNotificationEventsPublish},
		},
		Notifications: extension.NotificationContribution{Events: []extension.NotificationEventContribution{{
			ID:              "created",
			Label:           "新订单",
			DefaultTemplate: "{{mention}} {{order_id}}",
			Variables: []extension.NotificationVariable{{
				Name:     "order_id",
				Type:     "string",
				Required: true,
			}},
		}}},
	}
	manifestData, err := common.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), manifestData, 0644))
	require.NoError(t, manager.Scan())
	_, err = manager.SetEnabled("orders", true)
	require.NoError(t, err)
	extension.DefaultManager = manager
	t.Cleanup(func() {
		extension.DefaultManager = originalManager
	})

	bot := &model.NotificationBot{Name: "orders", Token: "token", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	task := &model.NotificationTask{
		Name:      "订单通知",
		EventType: "extension.orders.created",
		BotId:     bot.Id,
		Template:  "{{order_id}}",
		Enabled:   true,
	}
	require.NoError(t, model.CreateNotificationTask(task))
	target := &model.NotificationTarget{TaskId: task.Id, ChatId: "-10001", Enabled: true}
	require.NoError(t, model.CreateNotificationTarget(target))

	call := func() map[string]any {
		body, marshalErr := common.Marshal(map[string]any{
			"event_type": "created",
			"event_key":  "order:1",
			"payload":    map[string]any{"order_id": "1"},
		})
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/extensions/orders/notification-events", bytes.NewReader(body))
		ctx.Params = gin.Params{{Key: "id", Value: "orders"}}
		ctx.Set("role", common.RoleRootUser)
		PublishExtensionNotificationEvent(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.Equal(t, true, response["success"])
		data, ok := response["data"].(map[string]any)
		require.True(t, ok)
		return data
	}

	first := call()
	require.Equal(t, "queued", first["status"])
	require.EqualValues(t, 1, first["delivery_count"])
	duplicate := call()
	require.Equal(t, "duplicate", duplicate["status"])

	var deliveryCount int64
	require.NoError(t, db.Model(&model.NotificationDelivery{}).Count(&deliveryCount).Error)
	require.EqualValues(t, 1, deliveryCount)
}
