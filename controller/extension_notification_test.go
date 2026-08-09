package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestValidateExtensionNotificationPayload(t *testing.T) {
	registered := extension.RegisteredNotificationEvent{
		ModuleID:  "orders",
		EventType: "extension.orders.created",
		Event: extension.NotificationEventContribution{
			ID:    "created",
			Label: "新订单",
			Variables: []extension.NotificationVariable{
				{Name: "order_id", Type: "string", Required: true},
				{Name: "amount", Type: "number"},
				{Name: "urgent", Type: "boolean"},
			},
		},
	}

	payload, err := validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"amount":   float64(12.5),
	})
	require.NoError(t, err)
	require.Equal(t, "order-1", payload["order_id"])
	require.Equal(t, false, payload["urgent"])

	_, err = validateExtensionNotificationPayload(registered, map[string]any{"amount": float64(1)})
	require.ErrorContains(t, err, "order_id")

	_, err = validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"unknown":  "value",
	})
	require.ErrorContains(t, err, "未在 manifest 中声明")

	_, err = validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"amount":   "12.5",
	})
	require.ErrorContains(t, err, "必须是数字")

	_, err = validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"amount":   float64(1),
		"urgent":   true,
		"extra":    "x",
	})
	require.ErrorContains(t, err, "未在 manifest 中声明")
}

func TestValidateExtensionNotificationEventKey(t *testing.T) {
	require.NoError(t, validateExtensionNotificationEventKey("order:123"))
	require.Error(t, validateExtensionNotificationEventKey(""))
	require.Error(t, validateExtensionNotificationEventKey(strings.Repeat("x", maxExtensionNotificationEventKeyRunes+1)))
	require.Error(t, validateExtensionNotificationEventKey("order\u0000id"))
}

func TestPublishExtensionNotificationEventRequiresRoot(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/extensions/orders/notification-events", strings.NewReader(`{}`))
	ctx.Set("role", common.RoleAdminUser)

	PublishExtensionNotificationEvent(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Root")
}
