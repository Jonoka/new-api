package extension

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	CapabilityNotificationEventsPublish = "notification.events.publish"
	ExtensionNotificationEventPrefix    = "extension."

	maxNotificationEventsPerModule    = 32
	maxNotificationVariablesPerEvent  = 32
	maxNotificationEventTypeLength    = 64
	maxNotificationEventLabelLength   = 128
	maxNotificationEventDescription   = 512
	maxNotificationDefaultTemplateLen = 4096
)

var (
	notificationModuleIDPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	notificationEventIDPattern          = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	notificationVariableNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	notificationTemplateVariablePattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)
)

var reservedNotificationVariables = map[string]struct{}{
	"mention":    {},
	"module_id":  {},
	"event_type": {},
	"event_key":  {},
}

type NotificationContribution struct {
	Events []NotificationEventContribution `json:"events,omitempty"`
}

type NotificationEventContribution struct {
	ID              string                 `json:"id"`
	Label           string                 `json:"label"`
	Description     string                 `json:"description,omitempty"`
	DefaultTemplate string                 `json:"default_template"`
	Variables       []NotificationVariable `json:"variables,omitempty"`
}

type NotificationVariable struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// RegisteredNotificationEvent 是通知中心可消费的完整模块事件定义。
type RegisteredNotificationEvent struct {
	ModuleID  string
	EventType string
	Enabled   bool
	Event     NotificationEventContribution
}

func (m *Manifest) validateNotificationContribution() error {
	capabilities := make(map[string]struct{}, len(m.Permissions.Capabilities))
	for index, capability := range m.Permissions.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return errors.New("permissions.capabilities cannot contain an empty value")
		}
		if capability != CapabilityNotificationEventsPublish {
			return fmt.Errorf("unsupported permission capability: %s", capability)
		}
		if _, exists := capabilities[capability]; exists {
			return fmt.Errorf("duplicate permission capability: %s", capability)
		}
		capabilities[capability] = struct{}{}
		m.Permissions.Capabilities[index] = capability
	}

	_, canPublish := capabilities[CapabilityNotificationEventsPublish]
	if len(m.Notifications.Events) == 0 {
		if canPublish {
			return errors.New("notification.events.publish requires notifications.events")
		}
		return nil
	}
	if !canPublish {
		return errors.New("notifications.events requires notification.events.publish capability")
	}
	if !notificationModuleIDPattern.MatchString(m.ID) {
		return errors.New("modules that publish notifications require a lowercase id using letters, numbers, - or _")
	}
	if len(m.Notifications.Events) > maxNotificationEventsPerModule {
		return fmt.Errorf("notifications.events supports at most %d entries", maxNotificationEventsPerModule)
	}

	seenEvents := make(map[string]struct{}, len(m.Notifications.Events))
	for eventIndex := range m.Notifications.Events {
		event := &m.Notifications.Events[eventIndex]
		event.ID = strings.TrimSpace(event.ID)
		event.Label = strings.TrimSpace(event.Label)
		event.Description = strings.TrimSpace(event.Description)
		event.DefaultTemplate = strings.TrimSpace(event.DefaultTemplate)
		if !notificationEventIDPattern.MatchString(event.ID) {
			return fmt.Errorf("notifications.events[%d].id is invalid", eventIndex)
		}
		if _, exists := seenEvents[event.ID]; exists {
			return fmt.Errorf("notifications.events[%d].id is duplicated", eventIndex)
		}
		seenEvents[event.ID] = struct{}{}
		if len(ExtensionNotificationEventPrefix+m.ID+"."+event.ID) > maxNotificationEventTypeLength {
			return fmt.Errorf("notifications.events[%d] full event type exceeds %d characters", eventIndex, maxNotificationEventTypeLength)
		}
		if event.Label == "" || utf8.RuneCountInString(event.Label) > maxNotificationEventLabelLength {
			return fmt.Errorf("notifications.events[%d].label is required and must not exceed %d characters", eventIndex, maxNotificationEventLabelLength)
		}
		if utf8.RuneCountInString(event.Description) > maxNotificationEventDescription {
			return fmt.Errorf("notifications.events[%d].description must not exceed %d characters", eventIndex, maxNotificationEventDescription)
		}
		if event.DefaultTemplate == "" || utf8.RuneCountInString(event.DefaultTemplate) > maxNotificationDefaultTemplateLen {
			return fmt.Errorf("notifications.events[%d].default_template is required and must not exceed %d characters", eventIndex, maxNotificationDefaultTemplateLen)
		}
		if err := validateNotificationVariables(eventIndex, event); err != nil {
			return err
		}
	}
	return nil
}

func validateNotificationVariables(eventIndex int, event *NotificationEventContribution) error {
	if len(event.Variables) > maxNotificationVariablesPerEvent {
		return fmt.Errorf("notifications.events[%d].variables supports at most %d entries", eventIndex, maxNotificationVariablesPerEvent)
	}
	allowed := make(map[string]struct{}, len(event.Variables)+len(reservedNotificationVariables))
	for name := range reservedNotificationVariables {
		allowed[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(event.Variables))
	for variableIndex := range event.Variables {
		variable := &event.Variables[variableIndex]
		variable.Name = strings.TrimSpace(variable.Name)
		variable.Label = strings.TrimSpace(variable.Label)
		variable.Type = strings.ToLower(strings.TrimSpace(variable.Type))
		if variable.Type == "" {
			variable.Type = "string"
		}
		if !notificationVariableNamePattern.MatchString(variable.Name) {
			return fmt.Errorf("notifications.events[%d].variables[%d].name is invalid", eventIndex, variableIndex)
		}
		if _, reserved := reservedNotificationVariables[variable.Name]; reserved {
			return fmt.Errorf("notifications.events[%d].variables[%d].name is reserved", eventIndex, variableIndex)
		}
		if _, exists := seen[variable.Name]; exists {
			return fmt.Errorf("notifications.events[%d].variables[%d].name is duplicated", eventIndex, variableIndex)
		}
		switch variable.Type {
		case "string", "number", "boolean":
		default:
			return fmt.Errorf("notifications.events[%d].variables[%d].type must be string, number or boolean", eventIndex, variableIndex)
		}
		seen[variable.Name] = struct{}{}
		allowed[variable.Name] = struct{}{}
	}
	for _, match := range notificationTemplateVariablePattern.FindAllStringSubmatch(event.DefaultTemplate, -1) {
		if len(match) != 2 {
			continue
		}
		if _, exists := allowed[match[1]]; !exists {
			return fmt.Errorf("notifications.events[%d].default_template uses undeclared variable %s", eventIndex, match[1])
		}
	}
	return nil
}

func hasNotificationPublishCapability(module Module) bool {
	for _, capability := range module.Permissions.Capabilities {
		if capability == CapabilityNotificationEventsPublish {
			return true
		}
	}
	return false
}

func registeredNotificationEvent(module Module, event NotificationEventContribution) RegisteredNotificationEvent {
	return RegisteredNotificationEvent{
		ModuleID:  module.ID,
		EventType: ExtensionNotificationEventPrefix + module.ID + "." + event.ID,
		Enabled:   module.Enabled,
		Event:     event,
	}
}

// NotificationEvents 返回已安装模块声明的事件。includeDisabled 用于管理端保留停用模块的任务配置。
func (m *Manager) NotificationEvents(includeDisabled bool) []RegisteredNotificationEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]RegisteredNotificationEvent, 0)
	for _, module := range m.modules {
		if module.Error != "" || !hasNotificationPublishCapability(module) {
			continue
		}
		if !includeDisabled && !module.Enabled {
			continue
		}
		for _, event := range module.Notifications.Events {
			result = append(result, registeredNotificationEvent(module, event))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].EventType < result[j].EventType
	})
	return result
}

// ResolveNotificationEvent 校验模块状态、调用角色、能力声明和事件白名单。
func (m *Manager) ResolveNotificationEvent(moduleID, eventID string, role int) (RegisteredNotificationEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	module, exists := m.modules[strings.TrimSpace(moduleID)]
	if !exists {
		return RegisteredNotificationEvent{}, errors.New("扩展模块不存在")
	}
	if module.Error != "" {
		return RegisteredNotificationEvent{}, errors.New("扩展模块 manifest 无效")
	}
	if !module.Enabled {
		return RegisteredNotificationEvent{}, errors.New("扩展模块未启用")
	}
	if !roleAllowed(role, module.Permissions.Roles) {
		return RegisteredNotificationEvent{}, errors.New("当前账号无权调用该扩展模块")
	}
	if !hasNotificationPublishCapability(module) {
		return RegisteredNotificationEvent{}, errors.New("扩展模块未声明通知事件发布能力")
	}
	eventID = strings.TrimSpace(eventID)
	for _, event := range module.Notifications.Events {
		if event.ID == eventID {
			return registeredNotificationEvent(module, event), nil
		}
	}
	return RegisteredNotificationEvent{}, errors.New("扩展模块未声明该通知事件")
}
