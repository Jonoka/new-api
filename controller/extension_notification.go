package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	maxExtensionNotificationEventBodyBytes     = 16 << 10
	maxExtensionNotificationEventKeyRunes      = 128
	maxExtensionNotificationPayloadFields      = 32
	maxExtensionNotificationPayloadStringRunes = 1024
)

type notificationEventTypeDefinition struct {
	Value           string         `json:"value"`
	Label           string         `json:"label"`
	Description     string         `json:"description,omitempty"`
	DefaultTemplate string         `json:"default_template"`
	Variables       []string       `json:"variables"`
	Owner           string         `json:"owner"`
	Available       bool           `json:"available"`
	SamplePayload   map[string]any `json:"-"`
}

type extensionNotificationEventRequest struct {
	EventType string         `json:"event_type"`
	EventKey  string         `json:"event_key"`
	Payload   map[string]any `json:"payload"`
}

func notificationEventDefinitions(includeDisabled bool) []notificationEventTypeDefinition {
	definitions := []notificationEventTypeDefinition{{
		Value:           model.NotificationEventTypeInvoicePending,
		Label:           "新待开票订单",
		DefaultTemplate: model.NotificationTaskDefaultTemplate,
		Variables: []string{
			"mention", "invoice_id", "source_type", "source_id", "user_id", "title", "total_amount", "create_time",
		},
		Owner:         "core",
		Available:     true,
		SamplePayload: invoicePendingTemplateSample,
	}}
	for _, registered := range extension.DefaultManager.NotificationEvents(includeDisabled) {
		variables := []string{"mention", "module_id", "event_type", "event_key"}
		sample := map[string]any{
			"module_id":  registered.ModuleID,
			"event_type": registered.EventType,
			"event_key":  strings.Repeat("k", maxExtensionNotificationEventKeyRunes),
		}
		for _, variable := range registered.Event.Variables {
			variables = append(variables, variable.Name)
			sample[variable.Name] = notificationVariableSample(variable)
		}
		definitions = append(definitions, notificationEventTypeDefinition{
			Value:           registered.EventType,
			Label:           registered.Event.Label,
			Description:     registered.Event.Description,
			DefaultTemplate: registered.Event.DefaultTemplate,
			Variables:       variables,
			Owner:           registered.ModuleID,
			Available:       registered.Enabled,
			SamplePayload:   sample,
		})
	}
	return definitions
}

func findNotificationEventDefinition(eventType string) (notificationEventTypeDefinition, bool) {
	eventType = strings.TrimSpace(eventType)
	for _, definition := range notificationEventDefinitions(true) {
		if definition.Value == eventType {
			return definition, true
		}
	}
	return notificationEventTypeDefinition{}, false
}

func notificationEventNames() map[string]string {
	result := make(map[string]string)
	for _, definition := range notificationEventDefinitions(true) {
		result[definition.Value] = definition.Label
	}
	return result
}

func notificationVariableSample(variable extension.NotificationVariable) any {
	switch variable.Type {
	case "number":
		return "-9223372036854775808.123456"
	case "boolean":
		return true
	default:
		return strings.Repeat("x", maxExtensionNotificationPayloadStringRunes)
	}
}

// PublishExtensionNotificationEvent 接收模块事件，Bot、目标和模板仍由通知中心统一管理。
func PublishExtensionNotificationEvent(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "仅 Root 可以发布扩展通知事件",
		})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxExtensionNotificationEventBodyBytes)
	var req extensionNotificationEventRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		if _, tooLarge := err.(*http.MaxBytesError); tooLarge {
			common.ApiErrorMsg(c, "通知事件请求体不能超过 16 KiB")
			return
		}
		common.ApiErrorMsg(c, "通知事件参数无效")
		return
	}
	registered, err := extension.DefaultManager.ResolveNotificationEvent(c.Param("id"), req.EventType, c.GetInt("role"))
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	req.EventKey = strings.TrimSpace(req.EventKey)
	if err := validateExtensionNotificationEventKey(req.EventKey); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	payload, err := validateExtensionNotificationPayload(registered, req.Payload)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	payload["module_id"] = registered.ModuleID
	payload["event_type"] = registered.EventType
	payload["event_key"] = req.EventKey

	var result model.NotificationEnqueueResult
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var enqueueErr error
		result, enqueueErr = model.EnqueueNotificationEventTxWithResult(tx, registered.EventType, req.EventKey, payload)
		return enqueueErr
	})
	if err != nil {
		if errors.Is(err, model.ErrNotificationStorageUnavailable) {
			common.ApiErrorMsg(c, "通知中心存储尚未就绪")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"status":         result.Status,
		"event_type":     registered.EventType,
		"event_key":      req.EventKey,
		"delivery_count": result.DeliveryCount,
	})
}

func validateExtensionNotificationEventKey(eventKey string) error {
	if eventKey == "" {
		return fmt.Errorf("event_key 不能为空")
	}
	if !utf8.ValidString(eventKey) || utf8.RuneCountInString(eventKey) > maxExtensionNotificationEventKeyRunes || len(eventKey) > 255 {
		return fmt.Errorf("event_key 最多允许 %d 个字符", maxExtensionNotificationEventKeyRunes)
	}
	for _, value := range eventKey {
		if unicode.IsControl(value) {
			return fmt.Errorf("event_key 不能包含控制字符")
		}
	}
	return nil
}

func validateExtensionNotificationPayload(registered extension.RegisteredNotificationEvent, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	if len(payload) > maxExtensionNotificationPayloadFields {
		return nil, fmt.Errorf("payload 最多允许 %d 个字段", maxExtensionNotificationPayloadFields)
	}
	variables := make(map[string]extension.NotificationVariable, len(registered.Event.Variables))
	for _, variable := range registered.Event.Variables {
		variables[variable.Name] = variable
	}
	for name := range payload {
		if _, exists := variables[name]; !exists {
			return nil, fmt.Errorf("payload 字段 %s 未在 manifest 中声明", name)
		}
	}

	result := make(map[string]any, len(variables)+3)
	for _, variable := range registered.Event.Variables {
		value, exists := payload[variable.Name]
		if !exists || value == nil {
			if variable.Required {
				return nil, fmt.Errorf("payload 缺少必填字段 %s", variable.Name)
			}
			result[variable.Name] = emptyNotificationVariableValue(variable.Type)
			continue
		}
		if err := validateExtensionNotificationVariableValue(variable, value); err != nil {
			return nil, err
		}
		result[variable.Name] = value
	}
	return result, nil
}

func emptyNotificationVariableValue(variableType string) any {
	switch variableType {
	case "number":
		return 0
	case "boolean":
		return false
	default:
		return ""
	}
}

func validateExtensionNotificationVariableValue(variable extension.NotificationVariable, value any) error {
	switch variable.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("payload 字段 %s 必须是字符串", variable.Name)
		}
		if !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxExtensionNotificationPayloadStringRunes {
			return fmt.Errorf("payload 字段 %s 最多允许 %d 个字符", variable.Name, maxExtensionNotificationPayloadStringRunes)
		}
		for _, character := range text {
			if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '	' {
				return fmt.Errorf("payload 字段 %s 包含不支持的控制字符", variable.Name)
			}
		}
	case "number":
		number, ok := value.(float64)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("payload 字段 %s 必须是数字", variable.Name)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("payload 字段 %s 必须是布尔值", variable.Name)
		}
	default:
		return fmt.Errorf("payload 字段 %s 的类型声明无效", variable.Name)
	}
	return nil
}
