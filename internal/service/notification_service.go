package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/queue"

	"github.com/hibiken/asynq"
)

var notificationTemplateVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// NotificationEnqueueInput 通知事件入队参数
type NotificationEnqueueInput struct {
	EventType string
	BizType   string
	BizID     uint
	Locale    string
	Force     bool
	Data      models.JSON
}

// NotificationTestSendInput 通知测试发送参数
type NotificationTestSendInput struct {
	Channel   string
	Target    string
	Scene     string
	Locale    string
	Variables map[string]interface{}
}

// NotificationService 通知中心服务
type NotificationService struct {
	settingService *SettingService
	emailService   *EmailService
	queueClient    *queue.Client
	dashboardSvc   *DashboardService
	telegramSender *TelegramNotifyService
}

// NewNotificationService 创建通知中心服务
func NewNotificationService(
	settingService *SettingService,
	emailService *EmailService,
	queueClient *queue.Client,
	dashboardSvc *DashboardService,
	defaultTelegramCfg config.TelegramAuthConfig,
) *NotificationService {
	return &NotificationService{
		settingService: settingService,
		emailService:   emailService,
		queueClient:    queueClient,
		dashboardSvc:   dashboardSvc,
		telegramSender: NewTelegramNotifyService(settingService, defaultTelegramCfg),
	}
}

// Enqueue 入队通知任务
func (s *NotificationService) Enqueue(input NotificationEnqueueInput) error {
	eventType := strings.ToLower(strings.TrimSpace(input.EventType))
	if !isNotificationEventSupported(eventType) {
		return ErrNotificationEventInvalid
	}
	if s == nil || s.queueClient == nil {
		return nil
	}

	payload := queue.NotificationDispatchPayload{
		EventType: eventType,
		BizType:   strings.TrimSpace(input.BizType),
		BizID:     input.BizID,
		Locale:    strings.TrimSpace(input.Locale),
		Force:     input.Force,
		Data:      notificationJSONToMap(input.Data),
	}
	return s.queueClient.EnqueueNotificationDispatch(payload, asynq.MaxRetry(5))
}

// Dispatch 处理通知分发任务
func (s *NotificationService) Dispatch(ctx context.Context, payload queue.NotificationDispatchPayload) error {
	if s == nil {
		return nil
	}
	eventType := strings.ToLower(strings.TrimSpace(payload.EventType))
	if !isNotificationEventSupported(eventType) {
		return ErrNotificationEventInvalid
	}

	setting, err := s.settingService.GetNotificationCenterSetting()
	if err != nil {
		return err
	}
	if !setting.Scenes.IsSceneEnabled(eventType) {
		return nil
	}

	if eventType == constants.NotificationEventExceptionAlertCheck {
		return s.dispatchExceptionAlertCheck(ctx, setting, payload)
	}
	return s.dispatchSingleEvent(ctx, setting, payload)
}

// SendTest 测试发送通知
func (s *NotificationService) SendTest(ctx context.Context, input NotificationTestSendInput) error {
	if s == nil {
		return ErrNotificationSendFailed
	}
	channel := strings.ToLower(strings.TrimSpace(input.Channel))
	target := strings.TrimSpace(input.Target)
	if channel == "" || target == "" {
		return ErrNotificationConfigInvalid
	}

	setting, err := s.settingService.GetNotificationCenterSetting()
	if err != nil {
		return err
	}
	scene := strings.ToLower(strings.TrimSpace(input.Scene))
	if scene == "" {
		scene = constants.NotificationEventExceptionAlert
	}
	template := setting.Templates.TemplateByEvent(scene).ResolveLocaleTemplate(resolveNotificationLocale(input.Locale, setting.DefaultLocale))
	variables := cloneNotificationVariables(input.Variables)
	if variables == nil {
		variables = map[string]interface{}{}
	}
	variables["event_type"] = scene
	variables["message"] = pickNotificationMessage(variables["message"], "test message")
	title := renderNotificationTemplate(template.Title, variables)
	body := renderNotificationTemplate(template.Body, variables)
	if strings.TrimSpace(body) == "" {
		body = title
	}
	if strings.TrimSpace(title) == "" {
		title = "Notification Test"
	}

	switch channel {
	case "email":
		if s.emailService == nil {
			return ErrNotificationSendFailed
		}
		return s.emailService.SendCustomEmail(target, title, body)
	case "telegram":
		return s.telegramSender.SendMessage(ctx, target, composeTelegramMessage(title, body))
	default:
		return ErrNotificationConfigInvalid
	}
}

func (s *NotificationService) dispatchExceptionAlertCheck(ctx context.Context, setting NotificationCenterSetting, payload queue.NotificationDispatchPayload) error {
	if s.dashboardSvc == nil || s.settingService == nil {
		return nil
	}

	dashboardSetting, err := s.settingService.GetDashboardSetting()
	if err != nil {
		return err
	}
	overview, err := s.dashboardSvc.GetOverview(ctx, DashboardQueryInput{
		Range:        "today",
		Timezone:     time.Local.String(),
		ForceRefresh: true,
	})
	if err != nil {
		return err
	}
	if overview == nil || len(overview.Alerts) == 0 {
		return nil
	}

	var firstErr error
	for _, alert := range overview.Alerts {
		data := cloneNotificationVariables(payload.Data)
		if data == nil {
			data = map[string]interface{}{}
		}
		alertType := strings.TrimSpace(alert.Type)
		data["alert_type"] = alertType
		data["alert_level"] = strings.TrimSpace(alert.Level)
		data["alert_value"] = fmt.Sprintf("%d", alert.Value)
		data["alert_threshold"] = fmt.Sprintf("%d", thresholdValueByAlertType(dashboardSetting.Alert, alertType))
		data["message"] = thresholdMessageByAlertType(alertType)

		itemPayload := payload
		itemPayload.EventType = constants.NotificationEventExceptionAlert
		itemPayload.BizType = constants.NotificationBizTypeDashboardAlert
		itemPayload.Data = data
		if err := s.dispatchSingleEvent(ctx, setting, itemPayload); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *NotificationService) dispatchSingleEvent(ctx context.Context, setting NotificationCenterSetting, payload queue.NotificationDispatchPayload) error {
	if !payload.Force {
		ok, err := acquireNotificationDedupe(ctx, setting.DedupeTTLSeconds, payload)
		if err != nil {
			logger.Warnw("notification_dedupe_failed", "event_type", payload.EventType, "error", err)
		}
		if err == nil && !ok {
			return nil
		}
	}

	template := setting.Templates.TemplateByEvent(payload.EventType).ResolveLocaleTemplate(resolveNotificationLocale(payload.Locale, setting.DefaultLocale))
	variables := buildNotificationTemplateVariables(payload)
	title := renderNotificationTemplate(template.Title, variables)
	body := renderNotificationTemplate(template.Body, variables)
	if strings.TrimSpace(body) == "" {
		body = title
	}
	if strings.TrimSpace(title) == "" {
		title = "Notification"
	}

	var firstErr error
	if setting.Channels.Email.Enabled && len(setting.Channels.Email.Recipients) > 0 && s.emailService != nil {
		for _, recipient := range setting.Channels.Email.Recipients {
			if err := s.emailService.SendCustomEmail(recipient, title, body); err != nil {
				logger.Warnw("notification_email_send_failed",
					"event_type", payload.EventType,
					"biz_type", payload.BizType,
					"biz_id", payload.BizID,
					"recipient", recipient,
					"error", err,
				)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	if setting.Channels.Telegram.Enabled && len(setting.Channels.Telegram.Recipients) > 0 && s.telegramSender != nil {
		message := composeTelegramMessage(title, body)
		for _, recipient := range setting.Channels.Telegram.Recipients {
			if err := s.telegramSender.SendMessage(ctx, recipient, message); err != nil {
				logger.Warnw("notification_telegram_send_failed",
					"event_type", payload.EventType,
					"biz_type", payload.BizType,
					"biz_id", payload.BizID,
					"recipient", recipient,
					"error", err,
				)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	if firstErr != nil {
		return fmt.Errorf("%w: %v", ErrNotificationSendFailed, firstErr)
	}
	return nil
}

func isNotificationEventSupported(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case constants.NotificationEventWalletRechargeSuccess,
		constants.NotificationEventOrderPaidSuccess,
		constants.NotificationEventManualFulfillmentPending,
		constants.NotificationEventExceptionAlert,
		constants.NotificationEventExceptionAlertCheck:
		return true
	default:
		return false
	}
}

func acquireNotificationDedupe(ctx context.Context, ttlSeconds int, payload queue.NotificationDispatchPayload) (bool, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	key := buildNotificationDedupeKey(payload)
	return cache.SetNX(ctx, key, "1", time.Duration(ttlSeconds)*time.Second)
}

func buildNotificationDedupeKey(payload queue.NotificationDispatchPayload) string {
	signature := strings.Builder{}
	signature.WriteString(strings.ToLower(strings.TrimSpace(payload.EventType)))
	signature.WriteString("|")
	signature.WriteString(strings.ToLower(strings.TrimSpace(payload.BizType)))
	signature.WriteString("|")
	signature.WriteString(fmt.Sprintf("%d", payload.BizID))
	signature.WriteString("|")

	keys := make([]string, 0, len(payload.Data))
	for key := range payload.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "occurred_at" {
			continue
		}
		signature.WriteString(key)
		signature.WriteString("=")
		signature.WriteString(strings.TrimSpace(fmt.Sprintf("%v", payload.Data[key])))
		signature.WriteString(";")
	}
	hash := sha1.Sum([]byte(signature.String()))
	return "notification:dedupe:" + hex.EncodeToString(hash[:])
}

func buildNotificationTemplateVariables(payload queue.NotificationDispatchPayload) map[string]interface{} {
	data := cloneNotificationVariables(payload.Data)
	if data == nil {
		data = map[string]interface{}{}
	}
	data["event_type"] = strings.ToLower(strings.TrimSpace(payload.EventType))
	data["biz_type"] = strings.TrimSpace(payload.BizType)
	data["biz_id"] = fmt.Sprintf("%d", payload.BizID)
	data["occurred_at"] = time.Now().Format("2006-01-02 15:04:05")
	return data
}

func renderNotificationTemplate(template string, variables map[string]interface{}) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	return notificationTemplateVarPattern.ReplaceAllStringFunc(template, func(matched string) string {
		submatch := notificationTemplateVarPattern.FindStringSubmatch(matched)
		if len(submatch) != 2 {
			return matched
		}
		key := strings.TrimSpace(submatch[1])
		value, ok := variables[key]
		if !ok {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	})
}

func resolveNotificationLocale(locale, fallback string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = strings.TrimSpace(fallback)
	}
	if _, ok := notificationSupportedLocales[locale]; ok {
		return locale
	}
	return constants.LocaleZhCN
}

func composeTelegramMessage(title, body string) string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		return body
	}
	if body == "" {
		return title
	}
	return title + "\n\n" + body
}

func notificationJSONToMap(data models.JSON) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(data))
	for key, value := range data {
		result[key] = value
	}
	return result
}

func cloneNotificationVariables(data map[string]interface{}) map[string]interface{} {
	if len(data) == 0 {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(data))
	for key, value := range data {
		result[key] = value
	}
	return result
}

func thresholdValueByAlertType(setting DashboardAlertSetting, alertType string) int64 {
	switch alertType {
	case constants.NotificationAlertTypeOutOfStockProducts:
		return setting.OutOfStockProductsThreshold
	case constants.NotificationAlertTypeLowStockProducts:
		return setting.LowStockThreshold
	case constants.NotificationAlertTypePendingOrders:
		return setting.PendingPaymentOrdersThreshold
	case constants.NotificationAlertTypePaymentsFailed:
		return setting.PaymentsFailedThreshold
	default:
		return 0
	}
}

func thresholdMessageByAlertType(alertType string) string {
	switch alertType {
	case constants.NotificationAlertTypeOutOfStockProducts:
		return "缺货商品数量达到告警阈值"
	case constants.NotificationAlertTypeLowStockProducts:
		return "低库存商品数量触发告警，请及时补货"
	case constants.NotificationAlertTypePendingOrders:
		return "待支付订单数量达到告警阈值"
	case constants.NotificationAlertTypePaymentsFailed:
		return "支付失败数量达到告警阈值"
	default:
		return "触发系统异常告警"
	}
}

func pickNotificationMessage(value interface{}, fallback string) string {
	normalized := strings.TrimSpace(fmt.Sprintf("%v", value))
	if normalized == "" || normalized == "<nil>" {
		return fallback
	}
	return normalized
}
