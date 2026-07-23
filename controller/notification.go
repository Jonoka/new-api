package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type notificationBotRequest struct {
	Name    string  `json:"name"`
	Token   *string `json:"token"`
	Enabled *bool   `json:"enabled"`
}

type notificationTargetRequest struct {
	Id            int    `json:"id"`
	ChatId        string `json:"chat_id"`
	MentionUserId string `json:"mention_user_id"`
	MentionName   string `json:"mention_name"`
	Enabled       *bool  `json:"enabled"`
}

type notificationTaskRequest struct {
	Name      string                      `json:"name"`
	EventType string                      `json:"event_type"`
	BotId     int                         `json:"bot_id"`
	Template  string                      `json:"template"`
	Enabled   *bool                       `json:"enabled"`
	Targets   []notificationTargetRequest `json:"targets"`
}

type notificationTaskResponse struct {
	model.NotificationTask
	BotName         string                     `json:"bot_name"`
	EventName       string                     `json:"event_name"`
	Targets         []model.NotificationTarget `json:"targets"`
	LastTriggeredAt *int64                     `json:"last_triggered_at,omitempty"`
}

type notificationHistoryResponse struct {
	model.NotificationDelivery
	EventType string         `json:"event_type"`
	EventKey  string         `json:"event_key"`
	Payload   map[string]any `json:"payload"`
	TaskName  string         `json:"task_name"`
	ChatId    string         `json:"chat_id"`
	SourceId  string         `json:"source_id"`
}

var invoicePendingTemplateSample = map[string]any{
	"invoice_id":   "9223372036854775807",
	"source_type":  strings.Repeat("s", 32),
	"source_id":    strings.Repeat("o", 255),
	"user_id":      "9223372036854775807",
	"title":        strings.Repeat("t", 255),
	"total_amount": strings.Repeat("9", 32),
	"create_time":  "9223372036854775807",
}

var invoicePendingTemplateTargetSample = model.NotificationTarget{
	MentionUserId: "9223372036854775807",
	MentionName:   strings.Repeat("m", 128),
}

func ListNotificationEventTypes(c *gin.Context) {
	common.ApiSuccess(c, notificationEventDefinitions(true))
}

func ListNotificationBots(c *gin.Context) {
	bots, err := model.NotificationBotViews()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, bots)
}

func CreateNotificationBot(c *gin.Context) {
	var req notificationBotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "通知机器人参数无效")
		return
	}
	if req.Token == nil {
		common.ApiErrorMsg(c, "请填写 Telegram Bot Token")
		return
	}
	if err := validateNotificationString(strings.TrimSpace(req.Name), "通知机器人名称", 128); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	bot := &model.NotificationBot{
		Name:    strings.TrimSpace(req.Name),
		Type:    model.NotificationEndpointTypeTelegram,
		Token:   strings.TrimSpace(*req.Token),
		Enabled: enabled,
	}
	if err := model.CreateNotificationBot(bot); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, model.NotificationBotView{NotificationBot: *bot, TokenConfigured: true})
}

func UpdateNotificationBot(c *gin.Context) {
	id, err := notificationParamId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var req notificationBotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "通知机器人参数无效")
		return
	}
	var existing model.NotificationBot
	if err := model.DB.First(&existing, id).Error; err != nil {
		common.ApiErrorMsg(c, "通知机器人不存在")
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		if err := validateNotificationString(strings.TrimSpace(req.Name), "通知机器人名称", 128); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		existing.Name = strings.TrimSpace(req.Name)
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	var token *string
	if req.Token != nil && strings.TrimSpace(*req.Token) != "" {
		trimmed := strings.TrimSpace(*req.Token)
		token = &trimmed
	}
	updates := map[string]any{
		"name": existing.Name, "enabled": existing.Enabled, "updated_at": time.Now().Unix(),
	}
	if token != nil {
		updates["token"] = *token
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if lockErr := model.LockNotificationSequenceTx(tx); lockErr != nil {
			return lockErr
		}
		if updateErr := tx.Model(&model.NotificationBot{}).Where("id = ?", existing.Id).Updates(updates).Error; updateErr != nil {
			return updateErr
		}
		if !existing.Enabled {
			return cancelPendingNotificationDeliveriesForBotTx(tx, existing.Id)
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !existing.Enabled {
		if err := model.PruneNotificationHistoryForBot(existing.Id); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, model.NotificationBotView{NotificationBot: existing, TokenConfigured: true})
}

func DisableNotificationBot(c *gin.Context) {
	id, err := notificationParamId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if lockErr := model.LockNotificationSequenceTx(tx); lockErr != nil {
			return lockErr
		}
		var bot model.NotificationBot
		if loadErr := tx.Select("id").First(&bot, id).Error; loadErr != nil {
			return loadErr
		}
		result := tx.Model(&model.NotificationBot{}).Where("id = ?", id).Updates(map[string]any{
			"enabled": false, "updated_at": time.Now().Unix(),
		})
		if result.Error != nil {
			return result.Error
		}
		return cancelPendingNotificationDeliveriesForBotTx(tx, id)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiErrorMsg(c, "通知机器人不存在")
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.PruneNotificationHistoryForBot(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TestNotificationBot(c *gin.Context) {
	id, err := notificationParamId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var req struct {
		ChatId  string `json:"chat_id"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ChatId) == "" {
		common.ApiErrorMsg(c, "请填写用于测试的 Chat ID")
		return
	}
	if err := validateNotificationString(strings.TrimSpace(req.ChatId), "Chat ID", 128); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var bot model.NotificationBot
	if err := model.DB.First(&bot, id).Error; err != nil {
		common.ApiErrorMsg(c, "通知机器人不存在")
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = "Telegram 通知机器人测试成功"
	}
	if err := service.SendTelegramNotification(bot.Token, req.ChatId, message); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ListNotificationTasks(c *gin.Context) {
	var tasks []model.NotificationTask
	if err := model.DB.Order("id desc").Find(&tasks).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	lastTriggeredAt, err := model.NotificationTaskLastTriggeredAt()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]notificationTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		var targets []model.NotificationTarget
		if err := model.DB.Where("task_id = ? AND enabled = ?", task.Id, true).Order("id asc").Find(&targets).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		var bot model.NotificationBot
		botName := ""
		if err := model.DB.Select("name").First(&bot, task.BotId).Error; err == nil {
			botName = bot.Name
		}
		eventName := task.EventType
		if definition, exists := findNotificationEventDefinition(task.EventType); exists {
			eventName = definition.Label
		}
		var triggeredAt *int64
		if timestamp, exists := lastTriggeredAt[task.Id]; exists {
			triggeredAt = &timestamp
		}
		responses = append(responses, notificationTaskResponse{
			NotificationTask: task,
			BotName:          botName,
			EventName:        eventName,
			Targets:          targets,
			LastTriggeredAt:  triggeredAt,
		})
	}
	common.ApiSuccess(c, responses)
}

func CreateNotificationTask(c *gin.Context) {
	var req notificationTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "通知任务参数无效")
		return
	}
	if err := validateNotificationTaskRequest(&req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	task := model.NotificationTask{
		Name:      strings.TrimSpace(req.Name),
		EventType: req.EventType,
		BotId:     req.BotId,
		Template:  req.Template,
		Enabled:   req.Enabled == nil || *req.Enabled,
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if lockErr := model.LockNotificationSequenceTx(tx); lockErr != nil {
			return lockErr
		}
		var bot model.NotificationBot
		if loadErr := tx.Select("id").First(&bot, req.BotId).Error; loadErr != nil {
			return errors.New("Telegram 机器人不存在")
		}
		var latest model.NotificationEvent
		if loadErr := tx.Order("id desc").First(&latest).Error; loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return loadErr
		}
		now := time.Now().Unix()
		task.ActiveAfterEventId = latest.Id
		task.CreatedAt = now
		task.UpdatedAt = now
		if createErr := tx.Create(&task).Error; createErr != nil {
			return createErr
		}
		return replaceNotificationTargetsTx(tx, task.Id, req.Targets)
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task.Id)
}

func UpdateNotificationTask(c *gin.Context) {
	id, err := notificationParamId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var req notificationTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "通知任务参数无效")
		return
	}
	if err := validateNotificationTaskRequest(&req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var task model.NotificationTask
	if err := model.DB.First(&task, id).Error; err != nil {
		common.ApiErrorMsg(c, "通知任务不存在")
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if lockErr := model.LockNotificationSequenceTx(tx); lockErr != nil {
			return lockErr
		}
		if loadErr := tx.First(&task, id).Error; loadErr != nil {
			return loadErr
		}
		var bot model.NotificationBot
		if loadErr := tx.Select("id").First(&bot, req.BotId).Error; loadErr != nil {
			return errors.New("Telegram 机器人不存在")
		}
		enabled := task.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		eventTypeChanged := task.EventType != req.EventType
		updates := map[string]any{
			"name": strings.TrimSpace(req.Name), "event_type": req.EventType,
			"bot_id": req.BotId, "template": req.Template, "enabled": enabled, "updated_at": time.Now().Unix(),
		}
		if (!task.Enabled && enabled) || eventTypeChanged {
			var latest model.NotificationEvent
			if loadErr := tx.Order("id desc").First(&latest).Error; loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
				return loadErr
			}
			updates["active_after_event_id"] = latest.Id
		}
		if updateErr := tx.Model(&model.NotificationTask{}).Where("id = ?", id).Updates(updates).Error; updateErr != nil {
			return updateErr
		}
		if !enabled || eventTypeChanged {
			reason := "notification task event type changed"
			if !enabled {
				reason = "notification task disabled"
			}
			if updateErr := model.CancelNotificationTaskDeliveriesTx(tx, id, reason); updateErr != nil {
				return updateErr
			}
		}
		return replaceNotificationTargetsTx(tx, id, req.Targets)
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.PruneNotificationHistoryForTask(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DisableNotificationTask(c *gin.Context) {
	id, err := notificationParamId(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if lockErr := model.LockNotificationSequenceTx(tx); lockErr != nil {
			return lockErr
		}
		var current model.NotificationTask
		if loadErr := tx.Select("id").First(&current, id).Error; loadErr != nil {
			return loadErr
		}
		result := tx.Model(&model.NotificationTask{}).Where("id = ?", id).Updates(map[string]any{
			"enabled": false, "updated_at": time.Now().Unix(),
		})
		if result.Error != nil {
			return result.Error
		}
		return model.CancelNotificationTaskDeliveriesTx(tx, id, "notification task disabled")
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.PruneNotificationHistoryForTask(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ListNotificationDeliveries(c *gin.Context) {
	taskId, _ := strconv.Atoi(c.Query("task_id"))
	items, err := model.ListNotificationHistory(taskId, 5)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]notificationHistoryResponse, 0, len(items))
	for _, item := range items {
		response := notificationHistoryResponse{NotificationDelivery: item, Payload: map[string]any{}}
		var event model.NotificationEvent
		if err := model.DB.First(&event, item.EventId).Error; err == nil {
			response.EventType = event.EventType
			response.EventKey = event.EventKey
			_ = common.UnmarshalJsonStr(event.Payload, &response.Payload)
			if sourceId, exists := response.Payload["source_id"]; exists {
				response.SourceId = fmt.Sprint(sourceId)
			}
		}
		var task model.NotificationTask
		if err := model.DB.Select("name").First(&task, item.TaskId).Error; err == nil {
			response.TaskName = task.Name
		}
		var target model.NotificationTarget
		if err := model.DB.Select("chat_id").First(&target, item.TargetId).Error; err == nil {
			response.ChatId = target.ChatId
		}
		responses = append(responses, response)
	}
	common.ApiSuccess(c, responses)
}

func validateNotificationTaskRequest(req *notificationTaskRequest) error {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return errors.New("请填写通知任务名称")
	}
	if err := validateNotificationString(strings.TrimSpace(req.Name), "通知任务名称", 128); err != nil {
		return err
	}
	req.EventType = strings.TrimSpace(req.EventType)
	definition, exists := findNotificationEventDefinition(req.EventType)
	if !exists {
		return errors.New("暂不支持该通知事件")
	}
	if req.BotId <= 0 {
		return errors.New("请选择 Telegram 机器人")
	}
	var botCount int64
	if err := model.DB.Model(&model.NotificationBot{}).Where("id = ?", req.BotId).Count(&botCount).Error; err != nil {
		return err
	}
	if botCount != 1 {
		return errors.New("Telegram 机器人不存在")
	}
	req.Template = strings.TrimSpace(req.Template)
	if req.Template == "" {
		req.Template = definition.DefaultTemplate
	}
	renderedTemplate, err := service.RenderNotificationTemplate(req.Template, definition.SamplePayload, &invoicePendingTemplateTargetSample)
	if err != nil {
		return fmt.Errorf("消息模板无效: %w", err)
	}
	if utf8.RuneCountInString(renderedTemplate) > 4096 {
		return errors.New("消息模板渲染后不能超过 Telegram 的 4096 字符限制")
	}
	if len(req.Targets) == 0 {
		return errors.New("请至少添加一个接收目标")
	}
	seen := make(map[string]struct{}, len(req.Targets))
	for index := range req.Targets {
		target := &req.Targets[index]
		target.ChatId = strings.TrimSpace(target.ChatId)
		target.MentionUserId = strings.TrimSpace(target.MentionUserId)
		target.MentionName = strings.TrimSpace(target.MentionName)
		if target.ChatId == "" {
			return fmt.Errorf("第 %d 个接收目标缺少 Chat ID", index+1)
		}
		if err := validateNotificationString(target.ChatId, fmt.Sprintf("第 %d 个 Chat ID", index+1), 128); err != nil {
			return err
		}
		if err := validateNotificationString(target.MentionUserId, fmt.Sprintf("第 %d 个提及用户 ID", index+1), 128); err != nil {
			return err
		}
		if err := validateNotificationString(target.MentionName, fmt.Sprintf("第 %d 个提及名称", index+1), 128); err != nil {
			return err
		}
		if target.MentionUserId != "" {
			mentionUserId, err := strconv.ParseInt(target.MentionUserId, 10, 64)
			if err != nil || mentionUserId <= 0 {
				return fmt.Errorf("第 %d 个提及用户 ID 必须是正整数", index+1)
			}
		}
		key := target.ChatId + "\x00" + target.MentionUserId
		if _, exists := seen[key]; exists {
			return fmt.Errorf("第 %d 个接收目标重复", index+1)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func replaceNotificationTargetsTx(tx *gorm.DB, taskId int, targets []notificationTargetRequest) error {
	now := time.Now().Unix()
	if err := tx.Model(&model.NotificationTarget{}).Where("task_id = ?", taskId).Updates(map[string]any{
		"enabled": false, "updated_at": now,
	}).Error; err != nil {
		return err
	}
	for _, item := range targets {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		updates := map[string]any{
			"chat_id": item.ChatId, "mention_user_id": item.MentionUserId,
			"mention_name": item.MentionName, "enabled": enabled, "updated_at": now,
		}
		if item.Id > 0 {
			var existing model.NotificationTarget
			if err := tx.Select("id").Where("id = ? AND task_id = ?", item.Id, taskId).First(&existing).Error; err != nil {
				return errors.New("通知接收目标不存在")
			}
			result := tx.Model(&model.NotificationTarget{}).Where("id = ? AND task_id = ?", item.Id, taskId).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			continue
		}
		var existing model.NotificationTarget
		findErr := tx.Where("task_id = ? AND chat_id = ? AND mention_user_id = ?", taskId, item.ChatId, item.MentionUserId).First(&existing).Error
		if findErr == nil {
			if err := tx.Model(&model.NotificationTarget{}).Where("id = ?", existing.Id).Updates(updates).Error; err != nil {
				return err
			}
			continue
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		target := model.NotificationTarget{
			TaskId: taskId, ChatId: item.ChatId, MentionUserId: item.MentionUserId,
			MentionName: item.MentionName, Enabled: enabled, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&target).Error; err != nil {
			return err
		}
	}
	var disabledTargetIds []int
	if err := tx.Model(&model.NotificationTarget{}).Where("task_id = ? AND enabled = ?", taskId, false).Pluck("id", &disabledTargetIds).Error; err != nil {
		return err
	}
	if len(disabledTargetIds) > 0 {
		if err := tx.Model(&model.NotificationDelivery{}).
			Where("target_id IN ? AND status IN ?", disabledTargetIds, []string{model.NotificationDeliveryPending, model.NotificationDeliveryRetrying, model.NotificationDeliveryClaimed}).
			Updates(map[string]any{"status": model.NotificationDeliveryCanceled, "last_error": "notification target removed", "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func notificationParamId(c *gin.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, errors.New("参数 ID 无效")
	}
	return id, nil
}

func cancelPendingNotificationDeliveriesForBotTx(tx *gorm.DB, botId int) error {
	var taskIds []int
	if err := tx.Model(&model.NotificationTask{}).Where("bot_id = ?", botId).Pluck("id", &taskIds).Error; err != nil {
		return err
	}
	if len(taskIds) == 0 {
		return nil
	}
	return tx.Model(&model.NotificationDelivery{}).
		Where("task_id IN ? AND status IN ?", taskIds, []string{model.NotificationDeliveryPending, model.NotificationDeliveryRetrying, model.NotificationDeliveryClaimed}).
		Updates(map[string]any{"status": model.NotificationDeliveryCanceled, "last_error": "notification bot disabled", "updated_at": time.Now().Unix()}).Error
}

func validateNotificationString(value, label string, maxRunes int) error {
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s 最多允许 %d 个字符", label, maxRunes)
	}
	return nil
}
