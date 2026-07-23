package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	notificationDispatchInterval  = 10 * time.Second
	notificationDispatchBatchSize = 50
	notificationDeliveryLease     = 2 * time.Minute
	notificationMaxAttempts       = 8
)

var (
	notificationDispatcherOnce    sync.Once
	notificationDispatcherRunning atomic.Bool
	notificationTelegramAPIBase   = "https://api.telegram.org"
	notificationTemplateVariable  = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)
)

// StartNotificationDispatcher starts the master-node notification worker.
func StartNotificationDispatcher() {
	notificationDispatcherOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("notification dispatcher started: tick=%s", notificationDispatchInterval))
			runNotificationDispatcherOnce()
			ticker := time.NewTicker(notificationDispatchInterval)
			defer ticker.Stop()
			for range ticker.C {
				runNotificationDispatcherOnce()
			}
		})
	})
}

// RunNotificationDispatcherOnce runs one bounded delivery pass. It is exported for diagnostics/tests.
func RunNotificationDispatcherOnce() {
	runNotificationDispatcherOnce()
}

func runNotificationDispatcherOnce() {
	if !notificationDispatcherRunning.CompareAndSwap(false, true) {
		return
	}
	defer notificationDispatcherRunning.Store(false)

	now := time.Now().Unix()
	workItems, err := model.ClaimNotificationDeliveries(notificationDispatchBatchSize, now, int64(notificationDeliveryLease/time.Second))
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("notification delivery claim failed: %v", err))
	}
	for _, work := range workItems {
		dispatchNotificationDelivery(work)
	}
}

func logNotificationTransitionError(deliveryID int, action string, err error) bool {
	if err == nil {
		return true
	}
	logger.LogWarn(context.Background(), fmt.Sprintf("notification delivery %d %s failed: %v", deliveryID, action, err))
	return false
}

func dispatchNotificationDelivery(work model.NotificationDeliveryWork) {
	if err := refreshNotificationDeliveryConfig(&work); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logNotificationTransitionError(work.Delivery.Id, "cancel", model.MarkNotificationDeliveryCanceled(work.Delivery.Id, "notification configuration is unavailable"))
		} else {
			logNotificationTransitionError(work.Delivery.Id, "release claim", model.ReleaseNotificationDeliveryClaim(work.Delivery.Id, time.Now().Add(notificationDispatchInterval).Unix(), err.Error()))
		}
		return
	}
	if !work.Task.Enabled || !work.Target.Enabled || !work.Bot.Enabled {
		logNotificationTransitionError(work.Delivery.Id, "cancel", model.MarkNotificationDeliveryCanceled(work.Delivery.Id, "notification task, target, or bot disabled"))
		return
	}
	if work.Bot.Type != model.NotificationEndpointTypeTelegram {
		logNotificationTransitionError(work.Delivery.Id, "mark dead", model.MarkNotificationDeliveryDead(work.Delivery.Id, "unsupported notification bot type"))
		return
	}

	var payload map[string]any
	if err := common.UnmarshalJsonStr(work.Event.Payload, &payload); err != nil {
		logNotificationTransitionError(work.Delivery.Id, "mark dead", model.MarkNotificationDeliveryDead(work.Delivery.Id, "invalid notification event payload"))
		return
	}
	content, err := RenderNotificationTemplate(work.Task.Template, payload, &work.Target)
	if err != nil {
		logNotificationTransitionError(work.Delivery.Id, "mark dead", model.MarkNotificationDeliveryDead(work.Delivery.Id, err.Error()))
		return
	}
	attemptCount, err := model.MarkNotificationDeliverySending(work.Delivery.Id, time.Now().Unix())
	if !logNotificationTransitionError(work.Delivery.Id, "start sending", err) {
		return
	}
	work.Delivery.AttemptCount = attemptCount
	if err := sendTelegramMessage(work.Bot.Token, work.Target.ChatId, content); err != nil {
		if retryErr, ok := err.(*telegramDeliveryError); ok && retryErr.retryable && work.Delivery.AttemptCount < notificationMaxAttempts {
			delay := notificationRetryDelay(work.Delivery.AttemptCount)
			if retryErr.retryAfter > 0 {
				delay = time.Duration(retryErr.retryAfter) * time.Second
			}
			logNotificationTransitionError(work.Delivery.Id, "schedule retry", model.MarkNotificationDeliveryRetry(work.Delivery.Id, time.Now().Add(delay).Unix(), err.Error()))
			return
		}
		// Telegram 不支持客户端幂等键。网络结果不确定时不自动重试，
		// 以避免消息已送达但成功状态尚未来得及落库造成重复通知。
		logNotificationTransitionError(work.Delivery.Id, "mark dead", model.MarkNotificationDeliveryDead(work.Delivery.Id, err.Error()))
		return
	}
	if err := model.MarkNotificationDeliverySuccess(work.Delivery.Id, time.Now().Unix()); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("notification delivery %d success state update failed: %v", work.Delivery.Id, err))
	}
}

func refreshNotificationDeliveryConfig(work *model.NotificationDeliveryWork) error {
	if work == nil {
		return errors.New("notification delivery work is nil")
	}
	if err := model.DB.First(&work.Task, work.Delivery.TaskId).Error; err != nil {
		return err
	}
	if err := model.DB.First(&work.Target, work.Delivery.TargetId).Error; err != nil {
		return err
	}
	return model.DB.First(&work.Bot, work.Task.BotId).Error
}

func notificationRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := time.Duration(1<<uint(attempt-1)) * time.Second
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

type telegramDeliveryError struct {
	err        error
	retryable  bool
	retryAfter int
}

func (e *telegramDeliveryError) Error() string { return e.err.Error() }
func (e *telegramDeliveryError) Unwrap() error { return e.err }

type telegramMessageRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type telegramMessageResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func sendTelegramMessage(token, chatID, content string) error {
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)
	if token == "" || chatID == "" {
		return &telegramDeliveryError{err: errors.New("telegram token and chat id are required")}
	}
	if utf8.RuneCountInString(content) > 4096 {
		return &telegramDeliveryError{err: errors.New("telegram message exceeds 4096 characters")}
	}
	payload, err := common.Marshal(telegramMessageRequest{ChatID: chatID, Text: content, ParseMode: "HTML", DisableWebPagePreview: true})
	if err != nil {
		return &telegramDeliveryError{err: fmt.Errorf("marshal telegram message: %w", err)}
	}
	endpoint := strings.TrimRight(notificationTelegramAPIBase, "/") + "/bot" + url.PathEscape(token) + "/sendMessage"
	if err := ValidateSSRFProtectedFetchURL(endpoint); err != nil {
		return &telegramDeliveryError{err: errors.New("telegram endpoint rejected by SSRF policy")}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return &telegramDeliveryError{err: errors.New("create telegram request failed")}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NewAPI-Notification/1.0")
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		return &telegramDeliveryError{err: errors.New("notification HTTP client is unavailable")}
	}
	resp, err := client.Do(req)
	if err != nil {
		return &telegramDeliveryError{err: errors.New("telegram request failed")}
	}
	defer resp.Body.Close()
	var result telegramMessageResponse
	decodeErr := common.DecodeJson(resp.Body, &result)
	if resp.StatusCode == http.StatusTooManyRequests {
		return &telegramDeliveryError{err: fmt.Errorf("telegram rate limited: %s", result.Description), retryable: true, retryAfter: result.Parameters.RetryAfter}
	}
	if err := decodeErr; err != nil {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return &telegramDeliveryError{err: fmt.Errorf("telegram returned HTTP %d", resp.StatusCode)}
		}
		return &telegramDeliveryError{err: fmt.Errorf("decode telegram response: %w", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		return &telegramDeliveryError{err: fmt.Errorf("telegram rejected message: %s", result.Description), retryable: resp.StatusCode == http.StatusTooManyRequests || result.ErrorCode == http.StatusTooManyRequests, retryAfter: result.Parameters.RetryAfter}
	}
	return nil
}

// SendTelegramNotification sends pre-rendered HTML content through Telegram.
func SendTelegramNotification(token, chatID, content string) error {
	return sendTelegramMessage(token, chatID, content)
}

// TestTelegramNotification validates mention/template rendering and sends one test message.
// The token is accepted only as an argument and is never logged or included in an error.
func TestTelegramNotification(token, chatID, mentionUserID, mentionName, template string) error {
	target := &model.NotificationTarget{MentionUserId: mentionUserID, MentionName: mentionName}
	content, err := RenderNotificationTemplate(template, map[string]any{
		"invoice_id":   "TEST",
		"total_amount": "0.00",
	}, target)
	if err != nil {
		return err
	}
	return sendTelegramMessage(token, chatID, content)
}

// RenderNotificationTemplate renders a restricted HTML template and escapes all payload values.
func RenderNotificationTemplate(template string, payload map[string]any, target *model.NotificationTarget) (string, error) {
	if strings.TrimSpace(template) == "" {
		template = model.NotificationTaskDefaultTemplate
	}
	values := make(map[string]string, len(payload)+1)
	for key, value := range payload {
		values[key] = html.EscapeString(fmt.Sprint(value))
	}
	if target != nil && strings.TrimSpace(target.MentionUserId) != "" {
		userID := strings.TrimSpace(target.MentionUserId)
		parsedUserID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil || parsedUserID <= 0 {
			return "", errors.New("mention user id must be a positive number")
		}
		name := strings.TrimSpace(target.MentionName)
		if name == "" {
			name = "用户"
		}
		values["mention"] = fmt.Sprintf(`<a href="tg://user?id=%s">%s</a>`, html.EscapeString(userID), html.EscapeString(name))
	} else {
		values["mention"] = ""
	}
	var renderErr error
	result := notificationTemplateVariable.ReplaceAllStringFunc(template, func(match string) string {
		parts := notificationTemplateVariable.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		value, ok := values[parts[1]]
		if !ok {
			renderErr = fmt.Errorf("unknown notification template variable: %s", parts[1])
			return ""
		}
		return value
	})
	if renderErr != nil {
		return "", renderErr
	}
	return result, nil
}
