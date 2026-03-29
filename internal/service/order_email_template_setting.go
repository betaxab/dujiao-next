package service

import (
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
)

// OrderEmailLocalizedTemplate 订单邮件单语言模板
type OrderEmailLocalizedTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// OrderEmailSceneTemplate 订单邮件场景模板（多语言）
type OrderEmailSceneTemplate struct {
	ZHCN OrderEmailLocalizedTemplate `json:"zh-CN"`
	ZHTW OrderEmailLocalizedTemplate `json:"zh-TW"`
	ENUS OrderEmailLocalizedTemplate `json:"en-US"`
}

// OrderEmailGuestTip 游客提示（多语言）
type OrderEmailGuestTip struct {
	ZHCN string `json:"zh-CN"`
	ZHTW string `json:"zh-TW"`
	ENUS string `json:"en-US"`
}

// OrderEmailFulfillmentAttachmentTip 交付内容附件提示（多语言）
type OrderEmailFulfillmentAttachmentTip struct {
	ZHCN string `json:"zh-CN"`
	ZHTW string `json:"zh-TW"`
	ENUS string `json:"en-US"`
}

// OrderEmailTemplatesSetting 所有订单邮件场景模板集合
type OrderEmailTemplatesSetting struct {
	Default              OrderEmailSceneTemplate `json:"default"`
	Paid                 OrderEmailSceneTemplate `json:"paid"`
	Delivered            OrderEmailSceneTemplate `json:"delivered"`
	DeliveredWithContent OrderEmailSceneTemplate `json:"delivered_with_content"`
	Canceled             OrderEmailSceneTemplate `json:"canceled"`
}

// OrderEmailTemplateSetting 订单邮件模板配置
type OrderEmailTemplateSetting struct {
	Templates                OrderEmailTemplatesSetting         `json:"templates"`
	GuestTip                 OrderEmailGuestTip                 `json:"guest_tip"`
	FulfillmentAttachmentTip OrderEmailFulfillmentAttachmentTip `json:"fulfillment_attachment_tip"`
}

// --- Patch 结构 ---

// OrderEmailLocalizedTemplatePatch 单语言模板补丁
type OrderEmailLocalizedTemplatePatch struct {
	Subject *string `json:"subject"`
	Body    *string `json:"body"`
}

// OrderEmailSceneTemplatePatch 场景模板补丁
type OrderEmailSceneTemplatePatch struct {
	ZHCN *OrderEmailLocalizedTemplatePatch `json:"zh-CN"`
	ZHTW *OrderEmailLocalizedTemplatePatch `json:"zh-TW"`
	ENUS *OrderEmailLocalizedTemplatePatch `json:"en-US"`
}

// OrderEmailGuestTipPatch 游客提示补丁
type OrderEmailGuestTipPatch struct {
	ZHCN *string `json:"zh-CN"`
	ZHTW *string `json:"zh-TW"`
	ENUS *string `json:"en-US"`
}

// OrderEmailFulfillmentAttachmentTipPatch 交付内容附件提示补丁
type OrderEmailFulfillmentAttachmentTipPatch struct {
	ZHCN *string `json:"zh-CN"`
	ZHTW *string `json:"zh-TW"`
	ENUS *string `json:"en-US"`
}

// OrderEmailTemplatesPatch 模板集合补丁
type OrderEmailTemplatesPatch struct {
	Default              *OrderEmailSceneTemplatePatch `json:"default"`
	Paid                 *OrderEmailSceneTemplatePatch `json:"paid"`
	Delivered            *OrderEmailSceneTemplatePatch `json:"delivered"`
	DeliveredWithContent *OrderEmailSceneTemplatePatch `json:"delivered_with_content"`
	Canceled             *OrderEmailSceneTemplatePatch `json:"canceled"`
}

// OrderEmailTemplateSettingPatch 订单邮件模板配置补丁
type OrderEmailTemplateSettingPatch struct {
	Templates                *OrderEmailTemplatesPatch                `json:"templates"`
	GuestTip                 *OrderEmailGuestTipPatch                 `json:"guest_tip"`
	FulfillmentAttachmentTip *OrderEmailFulfillmentAttachmentTipPatch `json:"fulfillment_attachment_tip"`
}

// --- 默认值 ---

// OrderEmailTemplateDefaultSetting 返回默认订单邮件模板（从原 i18n 硬编码值迁移）
func OrderEmailTemplateDefaultSetting() OrderEmailTemplateSetting {
	return OrderEmailTemplateSetting{
		Templates: OrderEmailTemplatesSetting{
			Default: OrderEmailSceneTemplate{
				ZHCN: OrderEmailLocalizedTemplate{
					Subject: "订单状态更新：{{status}}",
					Body:    "订单号：{{order_no}}\n状态：{{status}}\n金额：{{amount}} {{currency}}\n\n感谢您的购买。",
				},
				ZHTW: OrderEmailLocalizedTemplate{
					Subject: "訂單狀態更新：{{status}}",
					Body:    "訂單號：{{order_no}}\n狀態：{{status}}\n金額：{{amount}} {{currency}}\n\n感謝您的購買。",
				},
				ENUS: OrderEmailLocalizedTemplate{
					Subject: "Order status updated: {{status}}",
					Body:    "Order No: {{order_no}}\nStatus: {{status}}\nAmount: {{amount}} {{currency}}\n\nThank you for your purchase.",
				},
			},
			Paid: OrderEmailSceneTemplate{
				ZHCN: OrderEmailLocalizedTemplate{
					Subject: "订单状态更新：{{status}}",
					Body:    "订单号：{{order_no}}\n状态：{{status}}\n金额：{{amount}} {{currency}}\n\n我们已收到您的付款，将尽快完成交付。",
				},
				ZHTW: OrderEmailLocalizedTemplate{
					Subject: "訂單狀態更新：{{status}}",
					Body:    "訂單號：{{order_no}}\n狀態：{{status}}\n金額：{{amount}} {{currency}}\n\n已收到付款，將盡快完成交付。",
				},
				ENUS: OrderEmailLocalizedTemplate{
					Subject: "Order status updated: {{status}}",
					Body:    "Order No: {{order_no}}\nStatus: {{status}}\nAmount: {{amount}} {{currency}}\n\nWe have received your payment and will deliver soon.",
				},
			},
			Delivered: OrderEmailSceneTemplate{
				ZHCN: OrderEmailLocalizedTemplate{
					Subject: "订单状态更新：{{status}}",
					Body:    "订单号：{{order_no}}\n状态：{{status}}\n金额：{{amount}} {{currency}}\n\n交付已完成，感谢您的购买。",
				},
				ZHTW: OrderEmailLocalizedTemplate{
					Subject: "訂單狀態更新：{{status}}",
					Body:    "訂單號：{{order_no}}\n狀態：{{status}}\n金額：{{amount}} {{currency}}\n\n交付已完成，感謝您的購買。",
				},
				ENUS: OrderEmailLocalizedTemplate{
					Subject: "Order status updated: {{status}}",
					Body:    "Order No: {{order_no}}\nStatus: {{status}}\nAmount: {{amount}} {{currency}}\n\nDelivery completed. Thank you for your purchase.",
				},
			},
			DeliveredWithContent: OrderEmailSceneTemplate{
				ZHCN: OrderEmailLocalizedTemplate{
					Subject: "订单状态更新：{{status}}",
					Body:    "订单号：{{order_no}}\n状态：{{status}}\n金额：{{amount}} {{currency}}\n\n交付内容：\n{{fulfillment_info}}\n\n感谢您的购买。",
				},
				ZHTW: OrderEmailLocalizedTemplate{
					Subject: "訂單狀態更新：{{status}}",
					Body:    "訂單號：{{order_no}}\n狀態：{{status}}\n金額：{{amount}} {{currency}}\n\n交付內容：\n{{fulfillment_info}}\n\n感謝您的購買。",
				},
				ENUS: OrderEmailLocalizedTemplate{
					Subject: "Order status updated: {{status}}",
					Body:    "Order No: {{order_no}}\nStatus: {{status}}\nAmount: {{amount}} {{currency}}\n\nDelivery content:\n{{fulfillment_info}}\n\nThank you for your purchase.",
				},
			},
			Canceled: OrderEmailSceneTemplate{
				ZHCN: OrderEmailLocalizedTemplate{
					Subject: "订单状态更新：{{status}}",
					Body:    "订单号：{{order_no}}\n状态：{{status}}\n金额：{{amount}} {{currency}}\n\n订单已取消，如有疑问请联系管理员。",
				},
				ZHTW: OrderEmailLocalizedTemplate{
					Subject: "訂單狀態更新：{{status}}",
					Body:    "訂單號：{{order_no}}\n狀態：{{status}}\n金額：{{amount}} {{currency}}\n\n訂單已取消，如有疑問請聯絡管理員。",
				},
				ENUS: OrderEmailLocalizedTemplate{
					Subject: "Order status updated: {{status}}",
					Body:    "Order No: {{order_no}}\nStatus: {{status}}\nAmount: {{amount}} {{currency}}\n\nThe order has been canceled. Please contact admin if needed.",
				},
			},
		},
		GuestTip: OrderEmailGuestTip{
			ZHCN: "游客订单可使用下单邮箱与订单密码在网站查询订单详情。",
			ZHTW: "遊客訂單可使用下單信箱與訂單密碼在網站查詢訂單詳情。",
			ENUS: "Guest orders can be queried on the site using the checkout email and order password.",
		},
		FulfillmentAttachmentTip: OrderEmailFulfillmentAttachmentTip{
			ZHCN: "交付内容较多，已作为附件发送，请查看邮件附件获取完整交付内容。",
			ZHTW: "交付內容較多，已作為附件發送，請查看郵件附件獲取完整交付內容。",
			ENUS: "The delivery content is included as an attachment. Please check the email attachment for the full content.",
		},
	}
}

// --- Normalize / Validate ---

// NormalizeOrderEmailTemplateSetting 归一化订单邮件模板配置
func NormalizeOrderEmailTemplateSetting(setting OrderEmailTemplateSetting) OrderEmailTemplateSetting {
	setting.Templates.Default = normalizeOrderEmailSceneTemplate(setting.Templates.Default)
	setting.Templates.Paid = normalizeOrderEmailSceneTemplate(setting.Templates.Paid)
	setting.Templates.Delivered = normalizeOrderEmailSceneTemplate(setting.Templates.Delivered)
	setting.Templates.DeliveredWithContent = normalizeOrderEmailSceneTemplate(setting.Templates.DeliveredWithContent)
	setting.Templates.Canceled = normalizeOrderEmailSceneTemplate(setting.Templates.Canceled)
	setting.GuestTip.ZHCN = strings.TrimSpace(setting.GuestTip.ZHCN)
	setting.GuestTip.ZHTW = strings.TrimSpace(setting.GuestTip.ZHTW)
	setting.GuestTip.ENUS = strings.TrimSpace(setting.GuestTip.ENUS)
	setting.FulfillmentAttachmentTip.ZHCN = strings.TrimSpace(setting.FulfillmentAttachmentTip.ZHCN)
	setting.FulfillmentAttachmentTip.ZHTW = strings.TrimSpace(setting.FulfillmentAttachmentTip.ZHTW)
	setting.FulfillmentAttachmentTip.ENUS = strings.TrimSpace(setting.FulfillmentAttachmentTip.ENUS)
	return setting
}

func normalizeOrderEmailSceneTemplate(t OrderEmailSceneTemplate) OrderEmailSceneTemplate {
	t.ZHCN.Subject = strings.TrimSpace(t.ZHCN.Subject)
	t.ZHCN.Body = strings.TrimSpace(t.ZHCN.Body)
	t.ZHTW.Subject = strings.TrimSpace(t.ZHTW.Subject)
	t.ZHTW.Body = strings.TrimSpace(t.ZHTW.Body)
	t.ENUS.Subject = strings.TrimSpace(t.ENUS.Subject)
	t.ENUS.Body = strings.TrimSpace(t.ENUS.Body)
	return t
}

// ValidateOrderEmailTemplateSetting 校验订单邮件模板配置
func ValidateOrderEmailTemplateSetting(setting OrderEmailTemplateSetting) error {
	scenes := []OrderEmailSceneTemplate{
		setting.Templates.Default,
		setting.Templates.Paid,
		setting.Templates.Delivered,
		setting.Templates.DeliveredWithContent,
		setting.Templates.Canceled,
	}
	for _, scene := range scenes {
		locales := []OrderEmailLocalizedTemplate{scene.ZHCN, scene.ZHTW, scene.ENUS}
		for _, lt := range locales {
			if lt.Subject == "" || lt.Body == "" {
				return ErrOrderEmailTemplateConfigInvalid
			}
		}
	}
	return nil
}

// --- ToMap / Mask ---

// OrderEmailTemplateSettingToMap 序列化为 map
func OrderEmailTemplateSettingToMap(setting OrderEmailTemplateSetting) map[string]interface{} {
	normalized := NormalizeOrderEmailTemplateSetting(setting)
	return map[string]interface{}{
		"templates": map[string]interface{}{
			"default":                orderEmailSceneTemplateToMap(normalized.Templates.Default),
			"paid":                   orderEmailSceneTemplateToMap(normalized.Templates.Paid),
			"delivered":              orderEmailSceneTemplateToMap(normalized.Templates.Delivered),
			"delivered_with_content": orderEmailSceneTemplateToMap(normalized.Templates.DeliveredWithContent),
			"canceled":               orderEmailSceneTemplateToMap(normalized.Templates.Canceled),
		},
		"guest_tip": map[string]interface{}{
			constants.LocaleZhCN: normalized.GuestTip.ZHCN,
			constants.LocaleZhTW: normalized.GuestTip.ZHTW,
			constants.LocaleEnUS: normalized.GuestTip.ENUS,
		},
		"fulfillment_attachment_tip": map[string]interface{}{
			constants.LocaleZhCN: normalized.FulfillmentAttachmentTip.ZHCN,
			constants.LocaleZhTW: normalized.FulfillmentAttachmentTip.ZHTW,
			constants.LocaleEnUS: normalized.FulfillmentAttachmentTip.ENUS,
		},
	}
}

func orderEmailSceneTemplateToMap(t OrderEmailSceneTemplate) map[string]interface{} {
	return map[string]interface{}{
		constants.LocaleZhCN: map[string]interface{}{
			"subject": t.ZHCN.Subject,
			"body":    t.ZHCN.Body,
		},
		constants.LocaleZhTW: map[string]interface{}{
			"subject": t.ZHTW.Subject,
			"body":    t.ZHTW.Body,
		},
		constants.LocaleEnUS: map[string]interface{}{
			"subject": t.ENUS.Subject,
			"body":    t.ENUS.Body,
		},
	}
}

// MaskOrderEmailTemplateSettingForAdmin 返回管理端可用配置（无敏感字段）
func MaskOrderEmailTemplateSettingForAdmin(setting OrderEmailTemplateSetting) models.JSON {
	normalized := NormalizeOrderEmailTemplateSetting(setting)
	return models.JSON(OrderEmailTemplateSettingToMap(normalized))
}

// --- Get / Patch ---

// GetOrderEmailTemplateSetting 获取订单邮件模板配置（优先 settings，空时回退默认）
func (s *SettingService) GetOrderEmailTemplateSetting() (OrderEmailTemplateSetting, error) {
	fallback := OrderEmailTemplateDefaultSetting()
	value, err := s.GetByKey(constants.SettingKeyOrderEmailTemplateConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	parsed := orderEmailTemplateSettingFromJSON(value, fallback)
	return NormalizeOrderEmailTemplateSetting(parsed), nil
}

// PatchOrderEmailTemplateSetting 基于补丁更新订单邮件模板配置
func (s *SettingService) PatchOrderEmailTemplateSetting(patch OrderEmailTemplateSettingPatch) (OrderEmailTemplateSetting, error) {
	current, err := s.GetOrderEmailTemplateSetting()
	if err != nil {
		return OrderEmailTemplateSetting{}, err
	}

	next := current
	if patch.Templates != nil {
		if patch.Templates.Default != nil {
			applyOrderEmailSceneTemplatePatch(&next.Templates.Default, patch.Templates.Default)
		}
		if patch.Templates.Paid != nil {
			applyOrderEmailSceneTemplatePatch(&next.Templates.Paid, patch.Templates.Paid)
		}
		if patch.Templates.Delivered != nil {
			applyOrderEmailSceneTemplatePatch(&next.Templates.Delivered, patch.Templates.Delivered)
		}
		if patch.Templates.DeliveredWithContent != nil {
			applyOrderEmailSceneTemplatePatch(&next.Templates.DeliveredWithContent, patch.Templates.DeliveredWithContent)
		}
		if patch.Templates.Canceled != nil {
			applyOrderEmailSceneTemplatePatch(&next.Templates.Canceled, patch.Templates.Canceled)
		}
	}
	if patch.GuestTip != nil {
		if patch.GuestTip.ZHCN != nil {
			next.GuestTip.ZHCN = strings.TrimSpace(*patch.GuestTip.ZHCN)
		}
		if patch.GuestTip.ZHTW != nil {
			next.GuestTip.ZHTW = strings.TrimSpace(*patch.GuestTip.ZHTW)
		}
		if patch.GuestTip.ENUS != nil {
			next.GuestTip.ENUS = strings.TrimSpace(*patch.GuestTip.ENUS)
		}
	}
	if patch.FulfillmentAttachmentTip != nil {
		if patch.FulfillmentAttachmentTip.ZHCN != nil {
			next.FulfillmentAttachmentTip.ZHCN = strings.TrimSpace(*patch.FulfillmentAttachmentTip.ZHCN)
		}
		if patch.FulfillmentAttachmentTip.ZHTW != nil {
			next.FulfillmentAttachmentTip.ZHTW = strings.TrimSpace(*patch.FulfillmentAttachmentTip.ZHTW)
		}
		if patch.FulfillmentAttachmentTip.ENUS != nil {
			next.FulfillmentAttachmentTip.ENUS = strings.TrimSpace(*patch.FulfillmentAttachmentTip.ENUS)
		}
	}

	normalized := NormalizeOrderEmailTemplateSetting(next)
	if err := ValidateOrderEmailTemplateSetting(normalized); err != nil {
		return OrderEmailTemplateSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyOrderEmailTemplateConfig, OrderEmailTemplateSettingToMap(normalized)); err != nil {
		return OrderEmailTemplateSetting{}, err
	}
	return normalized, nil
}

// --- Locale 解析 ---

// ResolveOrderEmailLocaleTemplate 按 locale 选择模板
func ResolveOrderEmailLocaleTemplate(t OrderEmailSceneTemplate, locale string) OrderEmailLocalizedTemplate {
	switch locale {
	case constants.LocaleZhTW:
		return t.ZHTW
	case constants.LocaleEnUS:
		return t.ENUS
	default:
		return t.ZHCN
	}
}

// ResolveOrderEmailFulfillmentAttachmentTip 按 locale 选择交付内容附件提示
func ResolveOrderEmailFulfillmentAttachmentTip(tip OrderEmailFulfillmentAttachmentTip, locale string) string {
	switch locale {
	case constants.LocaleZhTW:
		return tip.ZHTW
	case constants.LocaleEnUS:
		return tip.ENUS
	default:
		return tip.ZHCN
	}
}

// ResolveOrderEmailGuestTip 按 locale 选择游客提示
func ResolveOrderEmailGuestTip(tip OrderEmailGuestTip, locale string) string {
	switch locale {
	case constants.LocaleZhTW:
		return tip.ZHTW
	case constants.LocaleEnUS:
		return tip.ENUS
	default:
		return tip.ZHCN
	}
}

// --- JSON 解析 ---

func orderEmailTemplateSettingFromJSON(raw models.JSON, fallback OrderEmailTemplateSetting) OrderEmailTemplateSetting {
	next := fallback
	if raw == nil {
		return next
	}

	if templatesMap := toStringAnyMap(raw["templates"]); templatesMap != nil {
		if sceneMap := toStringAnyMap(templatesMap["default"]); sceneMap != nil {
			next.Templates.Default = orderEmailSceneTemplateFromMap(sceneMap, next.Templates.Default)
		}
		if sceneMap := toStringAnyMap(templatesMap["paid"]); sceneMap != nil {
			next.Templates.Paid = orderEmailSceneTemplateFromMap(sceneMap, next.Templates.Paid)
		}
		if sceneMap := toStringAnyMap(templatesMap["delivered"]); sceneMap != nil {
			next.Templates.Delivered = orderEmailSceneTemplateFromMap(sceneMap, next.Templates.Delivered)
		}
		if sceneMap := toStringAnyMap(templatesMap["delivered_with_content"]); sceneMap != nil {
			next.Templates.DeliveredWithContent = orderEmailSceneTemplateFromMap(sceneMap, next.Templates.DeliveredWithContent)
		}
		if sceneMap := toStringAnyMap(templatesMap["canceled"]); sceneMap != nil {
			next.Templates.Canceled = orderEmailSceneTemplateFromMap(sceneMap, next.Templates.Canceled)
		}
	}

	if guestTipMap := toStringAnyMap(raw["guest_tip"]); guestTipMap != nil {
		next.GuestTip.ZHCN = readString(guestTipMap, constants.LocaleZhCN, next.GuestTip.ZHCN)
		next.GuestTip.ZHTW = readString(guestTipMap, constants.LocaleZhTW, next.GuestTip.ZHTW)
		next.GuestTip.ENUS = readString(guestTipMap, constants.LocaleEnUS, next.GuestTip.ENUS)
	}

	if attachTipMap := toStringAnyMap(raw["fulfillment_attachment_tip"]); attachTipMap != nil {
		next.FulfillmentAttachmentTip.ZHCN = readString(attachTipMap, constants.LocaleZhCN, next.FulfillmentAttachmentTip.ZHCN)
		next.FulfillmentAttachmentTip.ZHTW = readString(attachTipMap, constants.LocaleZhTW, next.FulfillmentAttachmentTip.ZHTW)
		next.FulfillmentAttachmentTip.ENUS = readString(attachTipMap, constants.LocaleEnUS, next.FulfillmentAttachmentTip.ENUS)
	}

	return next
}

func orderEmailSceneTemplateFromMap(raw map[string]interface{}, fallback OrderEmailSceneTemplate) OrderEmailSceneTemplate {
	next := fallback
	if zhCNMap := toStringAnyMap(raw[constants.LocaleZhCN]); zhCNMap != nil {
		next.ZHCN.Subject = readString(zhCNMap, "subject", next.ZHCN.Subject)
		next.ZHCN.Body = readString(zhCNMap, "body", next.ZHCN.Body)
	}
	if zhTWMap := toStringAnyMap(raw[constants.LocaleZhTW]); zhTWMap != nil {
		next.ZHTW.Subject = readString(zhTWMap, "subject", next.ZHTW.Subject)
		next.ZHTW.Body = readString(zhTWMap, "body", next.ZHTW.Body)
	}
	if enUSMap := toStringAnyMap(raw[constants.LocaleEnUS]); enUSMap != nil {
		next.ENUS.Subject = readString(enUSMap, "subject", next.ENUS.Subject)
		next.ENUS.Body = readString(enUSMap, "body", next.ENUS.Body)
	}
	return next
}

// --- Patch 应用 ---

func applyOrderEmailSceneTemplatePatch(target *OrderEmailSceneTemplate, patch *OrderEmailSceneTemplatePatch) {
	if target == nil || patch == nil {
		return
	}
	if patch.ZHCN != nil {
		if patch.ZHCN.Subject != nil {
			target.ZHCN.Subject = strings.TrimSpace(*patch.ZHCN.Subject)
		}
		if patch.ZHCN.Body != nil {
			target.ZHCN.Body = strings.TrimSpace(*patch.ZHCN.Body)
		}
	}
	if patch.ZHTW != nil {
		if patch.ZHTW.Subject != nil {
			target.ZHTW.Subject = strings.TrimSpace(*patch.ZHTW.Subject)
		}
		if patch.ZHTW.Body != nil {
			target.ZHTW.Body = strings.TrimSpace(*patch.ZHTW.Body)
		}
	}
	if patch.ENUS != nil {
		if patch.ENUS.Subject != nil {
			target.ENUS.Subject = strings.TrimSpace(*patch.ENUS.Subject)
		}
		if patch.ENUS.Body != nil {
			target.ENUS.Body = strings.TrimSpace(*patch.ENUS.Body)
		}
	}
}
