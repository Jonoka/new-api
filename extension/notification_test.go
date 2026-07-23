package extension

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestManifestNotificationContributionAndResolution(t *testing.T) {
	manifest := Manifest{
		ID:      "orders",
		Name:    "订单模块",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		Permissions: PermissionConfig{
			Roles:        []string{"root"},
			Capabilities: []string{CapabilityNotificationEventsPublish},
		},
		Notifications: NotificationContribution{Events: []NotificationEventContribution{{
			ID:              "created",
			Label:           "新订单",
			DefaultTemplate: "{{mention}} 订单 {{order_id}}",
			Variables: []NotificationVariable{{
				Name:     "order_id",
				Type:     "string",
				Required: true,
			}},
		}}},
	}
	require.NoError(t, manifest.Validate())
	require.Equal(t, "string", manifest.Notifications.Events[0].Variables[0].Type)

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "orders", manifest)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("orders", true)
	require.NoError(t, err)

	events := manager.NotificationEvents(false)
	require.Len(t, events, 1)
	require.Equal(t, "extension.orders.created", events[0].EventType)
	require.True(t, events[0].Enabled)

	resolved, err := manager.ResolveNotificationEvent("orders", "created", common.RoleRootUser)
	require.NoError(t, err)
	require.Equal(t, events[0].EventType, resolved.EventType)

	_, err = manager.ResolveNotificationEvent("orders", "created", common.RoleAdminUser)
	require.Error(t, err)

	_, err = manager.SetEnabled("orders", false)
	require.NoError(t, err)
	require.Empty(t, manager.NotificationEvents(false))
	require.Len(t, manager.NotificationEvents(true), 1)
}

func TestManifestRejectsInvalidNotificationContribution(t *testing.T) {
	base := Manifest{
		ID:      "orders",
		Name:    "订单模块",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic},
		Permissions: PermissionConfig{
			Capabilities: []string{CapabilityNotificationEventsPublish},
		},
		Notifications: NotificationContribution{Events: []NotificationEventContribution{{
			ID:              "created",
			Label:           "新订单",
			DefaultTemplate: "{{order_id}}",
			Variables: []NotificationVariable{{
				Name: "order_id",
			}},
		}}},
	}
	require.NoError(t, base.Validate())

	tests := []struct {
		name  string
		apply func(*Manifest)
	}{
		{
			name: "missing capability",
			apply: func(manifest *Manifest) {
				manifest.Permissions.Capabilities = nil
			},
		},
		{
			name: "reserved variable",
			apply: func(manifest *Manifest) {
				manifest.Notifications.Events[0].Variables = []NotificationVariable{{Name: "mention"}}
				manifest.Notifications.Events[0].DefaultTemplate = "{{mention}}"
			},
		},
		{
			name: "unknown template variable",
			apply: func(manifest *Manifest) {
				manifest.Notifications.Events[0].DefaultTemplate = "{{missing}}"
			},
		},
		{
			name: "duplicate event",
			apply: func(manifest *Manifest) {
				manifest.Notifications.Events = append(manifest.Notifications.Events, manifest.Notifications.Events[0])
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.Permissions.Capabilities = append([]string(nil), base.Permissions.Capabilities...)
			candidate.Notifications.Events = append([]NotificationEventContribution(nil), base.Notifications.Events...)
			candidate.Notifications.Events[0].Variables = append([]NotificationVariable(nil), base.Notifications.Events[0].Variables...)
			test.apply(&candidate)
			require.Error(t, candidate.Validate())
		})
	}
}
