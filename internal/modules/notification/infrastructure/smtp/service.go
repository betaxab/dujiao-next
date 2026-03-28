package smtp

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/dujiao-next/internal/telegramidentity"
)

// writeStandardHeaders 写入 RFC 5322 要求的通用邮件头（Date、Message-ID、From、To、Subject、MIME-Version）。
func writeStandardHeaders(buf *bytes.Buffer, from, to, subject string, replyTo ...string) {
	fmt.Fprintf(buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(buf, "Message-ID: %s\r\n", generateMessageID(from))
	fmt.Fprintf(buf, "From: %s\r\n", from)
	fmt.Fprintf(buf, "To: %s\r\n", to)
	if len(replyTo) > 0 {
		if normalized := normalizeReplyToHeader(replyTo[0]); normalized != "" {
			fmt.Fprintf(buf, "Reply-To: %s\r\n", normalized)
		}
	}
	fmt.Fprintf(buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
}

// generateMessageID 生成 RFC 5322 兼容的 Message-ID，域名取自 From 地址，失败则回退到 localhost。
func generateMessageID(from string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	domain := "localhost"
	if addr, err := mail.ParseAddress(from); err == nil {
		if i := strings.LastIndex(addr.Address, "@"); i >= 0 && i < len(addr.Address)-1 {
			domain = addr.Address[i+1:]
		}
	}
	return fmt.Sprintf("<%s@%s>", hex.EncodeToString(b[:]), domain)
}

// Service 邮件发送服务
type Service struct {
	cfg *config.EmailConfig
}

// New 创建邮件服务
func New(cfg *config.EmailConfig) *Service {
	return &Service{cfg: cfg}
}

// SetConfig 更新运行时邮件配置
func (s *Service) SetConfig(cfg *config.EmailConfig) {
	if cfg == nil {
		return
	}
	s.cfg = cfg
}

var verifyCodeEmailHeaders = []emailHeader{
	{Name: "Importance", Value: "high"},
	{Name: "Priority", Value: "urgent"},
	{Name: "X-Priority", Value: "1"},
	{Name: "Auto-Submitted", Value: "auto-generated"},
	{Name: "X-Auto-Response-Suppress", Value: "All"},
}

type emailHeader struct {
	Name  string
	Value string
}

// SendVerifyCode 发送邮箱验证码
func (s *Service) SendVerifyCode(toEmail, code, purpose, locale string, brand mailbrand.Brand) error {
	input := notificationcontract.VerifyCodeEmailInput{
		Code:     code,
		Purpose:  purpose,
		SiteName: brand.SiteName,
		SiteURL:  brand.SiteURL,
	}
	subject, plainBody, htmlBody := s.buildVerifyCodeEmailParts(input, locale)
	return s.sendAlternativeEmail(toEmail, subject, plainBody, htmlBody, verifyCodeEmailHeaders, brand)
}

// SendOrderStatusEmail 发送订单状态通知
func (s *Service) SendOrderStatusEmail(toEmail string, input notificationcontract.OrderStatusEmailInput, locale string) error {
	subject, body := s.buildOrderStatusEmail(input, locale)
	if input.AttachmentName != "" && input.AttachmentContent != "" {
		return s.sendEmailWithAttachment(toEmail, subject, body, input.AttachmentName, input.AttachmentContent, input.MailBrand)
	}
	return s.sendHTMLEmail(toEmail, subject, body, input.MailBrand)
}

// SendOrderStatusEmailWithTemplate 使用可配置模板发送订单状态通知
func (s *Service) SendOrderStatusEmailWithTemplate(toEmail string, input notificationcontract.OrderStatusEmailInput, locale string, tmplSetting *settingsmessaging.OrderEmailTemplateSetting) error {
	if tmplSetting == nil {
		return s.SendOrderStatusEmail(toEmail, input, locale)
	}
	subject, body := buildOrderStatusContentFromTemplate(input, locale, *tmplSetting)
	if input.AttachmentName != "" && input.AttachmentContent != "" {
		return s.sendEmailWithAttachment(toEmail, subject, body, input.AttachmentName, input.AttachmentContent, input.MailBrand)
	}
	bodyHTML := s.buildOrderStatusTemplateEmail(input, locale, subject, body)
	return s.sendHTMLEmail(toEmail, subject, bodyHTML, input.MailBrand)
}

func buildOrderStatusContentFromTemplate(input notificationcontract.OrderStatusEmailInput, locale string, tmplSetting settingsmessaging.OrderEmailTemplateSetting) (string, string) {
	normalized := normalizeLocale(locale)

	// 根据订单状态选择场景模板
	var sceneTmpl settingsmessaging.OrderEmailSceneTemplate
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case constants.OrderStatusPaid:
		sceneTmpl = tmplSetting.Templates.Paid
	case constants.OrderStatusDelivered, constants.OrderStatusCompleted:
		if strings.TrimSpace(input.FulfillmentInfo) != "" {
			sceneTmpl = tmplSetting.Templates.DeliveredWithContent
		} else {
			sceneTmpl = tmplSetting.Templates.Delivered
		}
	case constants.OrderStatusRefunded:
		sceneTmpl = tmplSetting.Templates.Refunded
	case constants.OrderStatusPartiallyRefunded:
		sceneTmpl = tmplSetting.Templates.PartiallyRefunded
	default:
		sceneTmpl = tmplSetting.Templates.Default
	}

	localeTmpl := settingsmessaging.ResolveOrderEmailLocaleTemplate(sceneTmpl, normalized)

	// 翻译状态标签
	statusKey := "order.status." + status
	statusLabel := i18n.T(normalized, statusKey)
	if statusLabel == statusKey {
		statusLabel = input.Status
	}

	variables := map[string]interface{}{
		"order_no":         input.OrderNo,
		"status":           statusLabel,
		"amount":           input.Amount.String(),
		"refund_amount":    "",
		"refund_reason":    "",
		"currency":         strings.TrimSpace(input.Currency),
		"site_name":        strings.TrimSpace(input.SiteName),
		"site_url":         strings.TrimSpace(input.SiteURL),
		"fulfillment_info": strings.TrimSpace(input.FulfillmentInfo),
		"instructions":     strings.TrimSpace(input.Instructions),
	}
	if status == constants.OrderStatusRefunded || status == constants.OrderStatusPartiallyRefunded {
		variables["refund_amount"] = input.RefundAmount.String()
		variables["refund_reason"] = strings.TrimSpace(input.RefundReason)
	}

	subject := renderTemplate(localeTmpl.Subject, variables)
	body := renderTemplate(localeTmpl.Body, variables)

	// 存量兼容：历史自定义模板未引用 {{instructions}} 时自动在末尾追加使用说明，避免因占位符缺失导致说明丢失。
	// 只在交付含内容场景生效（此时 input.Instructions 才会被填充）。
	if strings.TrimSpace(input.Instructions) != "" && !strings.Contains(localeTmpl.Body, "{{instructions}}") {
		body = strings.TrimRight(body, "\n") + "\n\n" + strings.TrimSpace(input.Instructions)
	}

	// 交付内容以附件形式发送时追加提示
	if input.AttachmentName != "" {
		tip := strings.TrimSpace(settingsmessaging.ResolveOrderEmailFulfillmentAttachmentTip(tmplSetting.FulfillmentAttachmentTip, normalized))
		if tip != "" {
			body = body + "\n\n" + tip
		}
	}

	// 游客订单追加提示
	if input.IsGuest {
		tip := strings.TrimSpace(settingsmessaging.ResolveOrderEmailGuestTip(tmplSetting.GuestTip, normalized))
		if tip != "" {
			body = body + "\n\n" + tip
		}
	}

	return subject, body
}

// SendCustomEmail 发送测试邮件或自定义邮件
func (s *Service) SendCustomEmail(toEmail, subject, body string) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "SMTP 配置测试邮件"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "这是一封来自 Dujiao-Next 的 SMTP 测试邮件，说明当前配置可正常发送。"
	}
	return s.sendTextEmail(toEmail, subject, body)
}

func (s *Service) sendTextEmail(toEmail, subject, body string) error {
	bodyHTML := renderEmbeddedEmail("generic", genericEmailData{
		Paragraphs: splitEmailParagraphs(body),
	}, emailLayoutData{
		Lang:  emailLangCode(""),
		Title: subject,
	})
	return s.sendHTMLEmail(toEmail, subject, bodyHTML)
}

func (s *Service) sendHTMLEmail(toEmail, subject, bodyHTML string, brands ...mailbrand.Brand) error {
	return s.sendHTMLEmailWithHeaders(toEmail, subject, bodyHTML, nil, brands...)
}

func (s *Service) sendHTMLEmailWithHeaders(toEmail, subject, bodyHTML string, headers []emailHeader, brands ...mailbrand.Brand) error {
	if telegramidentity.IsPlaceholderEmail(toEmail) {
		return nil
	}
	brand := firstMailBrand(brands)
	from, addr, err := s.prepareSMTPEnvelope(toEmail, brand.FromName)
	if err != nil {
		return err
	}
	msg := buildEmailMessageWithHeaders(from, toEmail, subject, bodyHTML, headers, brand.ReplyTo)
	return s.sendSMTPMessage(addr, toEmail, []byte(msg))
}

func (s *Service) sendAlternativeEmail(toEmail, subject, plainBody, htmlBody string, headers []emailHeader, brands ...mailbrand.Brand) error {
	if telegramidentity.IsPlaceholderEmail(toEmail) {
		return nil
	}
	brand := firstMailBrand(brands)
	from, addr, err := s.prepareSMTPEnvelope(toEmail, brand.FromName)
	if err != nil {
		return err
	}
	msg := buildAlternativeEmailMessage(from, toEmail, subject, plainBody, htmlBody, headers, brand.ReplyTo)
	return s.sendSMTPMessage(addr, toEmail, []byte(msg))
}

func (s *Service) sendEmailWithAttachment(toEmail, subject, body, attachName, attachContent string, brands ...mailbrand.Brand) error {
	if telegramidentity.IsPlaceholderEmail(toEmail) {
		return nil
	}
	brand := firstMailBrand(brands)
	from, addr, err := s.prepareSMTPEnvelope(toEmail, brand.FromName)
	if err != nil {
		return err
	}
	msg := buildEmailMessageWithAttachment(from, toEmail, subject, body, attachName, attachContent, brand.ReplyTo)
	return s.sendSMTPMessage(addr, toEmail, []byte(msg))
}

// prepareSMTPEnvelope 校验配置与收件人，并返回发件地址与 SMTP 服务器地址。
func (s *Service) prepareSMTPEnvelope(toEmail string, fromNameOverrides ...string) (string, string, error) {
	if s.cfg == nil || !s.cfg.Enabled {
		return "", "", notificationcontract.ErrEmailServiceDisabled
	}
	if s.cfg.Host == "" || s.cfg.Port == 0 || s.cfg.From == "" {
		return "", "", notificationcontract.ErrEmailNotConfigured
	}
	if _, err := mail.ParseAddress(toEmail); err != nil {
		return "", "", notificationcontract.ErrInvalidEmail
	}
	fromName := ""
	if len(fromNameOverrides) > 0 {
		fromName = strings.TrimSpace(fromNameOverrides[0])
	}
	if fromName == "" {
		fromName = s.cfg.FromName
	}
	from := buildFromAddress(s.cfg.From, fromName)
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	return from, addr, nil
}

// sendSMTPMessage 根据配置选择 SSL/STARTTLS/明文通道发送邮件。
func (s *Service) sendSMTPMessage(addr, toEmail string, msg []byte) error {
	recipients := []string{toEmail}
	if s.cfg.UseSSL {
		return normalizeEmailSendError(sendMailWithSSL(addr, s.cfg.Host, s.cfg.From, recipients, msg, s.cfg.Username, s.cfg.Password))
	}
	if s.cfg.UseTLS {
		return normalizeEmailSendError(sendMailWithStartTLS(addr, s.cfg.Host, s.cfg.From, recipients, msg, s.cfg.Username, s.cfg.Password))
	}
	return normalizeEmailSendError(sendMailPlain(addr, s.cfg.Host, s.cfg.From, recipients, msg, s.cfg.Username, s.cfg.Password))
}

func buildEmailMessageWithAttachment(from, to, subject, body, attachName, attachContent string, replyTo ...string) string {
	boundary := "----=_DujiaoNextBoundary_" + fmt.Sprintf("%d", len(body)+len(attachContent))

	var buf bytes.Buffer
	writeStandardHeaders(&buf, from, to, subject, replyTo...)
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
	buf.WriteString("\r\n")

	// 正文部分
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(base64.StdEncoding.EncodeToString([]byte(body)))
	buf.WriteString("\r\n")

	// 附件部分
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "Content-Disposition: attachment; filename=\"%s\"\r\n", mime.QEncoding.Encode("UTF-8", attachName))
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(base64.StdEncoding.EncodeToString([]byte(attachContent)))
	buf.WriteString("\r\n")

	// 结束边界
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	return buf.String()
}

func (s *Service) buildVerifyCodeEmail(input notificationcontract.VerifyCodeEmailInput, locale string) (string, string) {
	subject, _, htmlBody := s.buildVerifyCodeEmailParts(input, locale)
	return subject, htmlBody
}

// buildVerifyCodeContent 保留纯内容构建入口，供测试及不经过 SMTP 发送流程的调用方使用。
func buildVerifyCodeContent(code, purpose, locale string, brands ...mailbrand.Brand) (string, string) {
	brand := firstMailBrand(brands)
	return (&Service{}).buildVerifyCodeEmail(notificationcontract.VerifyCodeEmailInput{
		Code:     code,
		Purpose:  purpose,
		SiteName: brand.SiteName,
		SiteURL:  brand.SiteURL,
	}, locale)
}

func (s *Service) buildVerifyCodeEmailParts(input notificationcontract.VerifyCodeEmailInput, locale string) (string, string, string) {
	normalized := normalizeLocale(locale)
	siteName := strings.TrimSpace(input.SiteName)
	siteURL := strings.TrimRight(strings.TrimSpace(input.SiteURL), "/")
	expireMinutes := 10
	if s != nil && s.cfg != nil && s.cfg.VerifyCode.ExpireMinutes > 0 {
		expireMinutes = s.cfg.VerifyCode.ExpireMinutes
	}

	var (
		subject      string
		greeting     string
		thanks       string
		description  string
		expirePrefix string
		expireStrong string
		expireSuffix string
		ignoreText   string
		supportText  string
		siteLabel    string
	)

	purposeKey := strings.ToLower(strings.TrimSpace(input.Purpose))
	switch normalized {
	case i18n.LocaleZH:
		subject = "邮箱验证码"
		greeting = "您好，"
		thanks = fmt.Sprintf("感谢您注册 %s！", siteName)
		description = "您的一次性验证码为："
		expirePrefix = "此验证码将于 "
		expireStrong = fmt.Sprintf("%d 分钟", expireMinutes)
		expireSuffix = " 后失效。请使用此验证码完成您的注册流程。"
		ignoreText = "如果该项请求不是您发出的，请忽略本邮件或者联系网站支持。"
		supportText = "本邮件为系统发送，请勿直接回复本邮件！如果您有任何问题或者需要帮助，请登录我们的官方网站，联系我们的技术支持团队。"
		siteLabel = "官方网站："
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "注册验证码"
		case constants.VerifyPurposeReset:
			subject = "重置密码验证码"
			thanks = fmt.Sprintf("您正在 %s 申请重置密码。", siteName)
			expireSuffix = " 后失效。请使用此验证码完成密码重置流程。"
		case constants.VerifyPurposeTelegramBind:
			subject = "Telegram 绑定验证码"
			thanks = fmt.Sprintf("您正在 %s 绑定 Telegram。", siteName)
			expireSuffix = " 后失效。请使用此验证码完成 Telegram 绑定流程。"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "更换邮箱验证码"
			thanks = fmt.Sprintf("您正在 %s 验证更换邮箱请求。", siteName)
			expireSuffix = " 后失效。请使用此验证码完成更换邮箱流程。"
		}
	case i18n.LocaleTW:
		subject = "郵箱驗證碼"
		greeting = "您好，"
		thanks = fmt.Sprintf("感謝您註冊 %s！", siteName)
		description = "您的一次性驗證碼為："
		expirePrefix = "此驗證碼將於 "
		expireStrong = fmt.Sprintf("%d 分鐘", expireMinutes)
		expireSuffix = " 後失效。請使用此驗證碼完成您的流程。"
		ignoreText = "如果該請求不是您發出的，請忽略本郵件或聯絡網站支援。"
		supportText = "本郵件由系統自動發送，請勿直接回覆本郵件！如果您有任何問題或需要協助，請登入我們的官方網站聯絡技術支援團隊。"
		siteLabel = "官方網站："
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "註冊驗證碼"
			expireSuffix = " 後失效。請使用此驗證碼完成您的註冊流程。"
		case constants.VerifyPurposeReset:
			subject = "重置密碼驗證碼"
			thanks = fmt.Sprintf("您正在 %s 申請重置密碼。", siteName)
			expireSuffix = " 後失效。請使用此驗證碼完成密碼重置流程。"
		case constants.VerifyPurposeTelegramBind:
			subject = "Telegram 綁定驗證碼"
			thanks = fmt.Sprintf("您正在 %s 綁定 Telegram。", siteName)
			expireSuffix = " 後失效。請使用此驗證碼完成 Telegram 綁定流程。"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "更換郵箱驗證碼"
			thanks = fmt.Sprintf("您正在 %s 驗證更換郵箱請求。", siteName)
			expireSuffix = " 後失效。請使用此驗證碼完成更換郵箱流程。"
		}
	case i18n.LocaleEN:
		subject = "Email Verification Code"
		greeting = "Hello,"
		thanks = fmt.Sprintf("Thank you for registering with %s.", siteName)
		description = "Your one-time verification code is:"
		expirePrefix = "This code will expire in "
		expireStrong = fmt.Sprintf("%d minutes", expireMinutes)
		expireSuffix = ". Please use it to complete your registration."
		ignoreText = "If you did not request this, please ignore this email or contact support."
		supportText = "This is an automated email. Please do not reply directly. If you need help, please visit our official website and contact the support team."
		siteLabel = "Official website:"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "Registration Code"
		case constants.VerifyPurposeReset:
			subject = "Password Reset Code"
			thanks = fmt.Sprintf("You requested a password reset for %s.", siteName)
			expireSuffix = ". Please use it to complete your password reset."
		case constants.VerifyPurposeTelegramBind:
			subject = "Telegram Binding Code"
			thanks = fmt.Sprintf("You are binding Telegram to %s.", siteName)
			expireSuffix = ". Please use it to complete Telegram binding."
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "Change Email Code"
			thanks = fmt.Sprintf("You requested an email change for %s.", siteName)
			expireSuffix = ". Please use it to complete the email change."
		}
	}

	if siteName != "" {
		subject = fmt.Sprintf("[%s] %s", siteName, subject)
	}

	data := verifyCodeEmailData{
		emailInfoData: emailInfoData{
			SiteName:    siteName,
			SiteURL:     siteURL,
			SupportText: supportText,
			SiteLabel:   siteLabel,
			ShowInfo:    true,
		},
		Greeting:     greeting,
		Thanks:       thanks,
		Description:  description,
		Code:         strings.TrimSpace(input.Code),
		ExpirePrefix: expirePrefix,
		ExpireStrong: expireStrong,
		ExpireSuffix: expireSuffix,
		IgnoreText:   ignoreText,
	}
	htmlBody := renderEmbeddedEmail("verify_code", data, emailLayoutData{
		Lang:     emailLangCode(normalized),
		Title:    subject,
		SiteName: siteName,
	})
	plainBody := buildVerifyCodePlainText(data)
	return subject, plainBody, htmlBody
}

func buildVerifyCodePlainText(data verifyCodeEmailData) string {
	lines := []string{
		data.Greeting,
		data.Thanks,
		data.Description,
		data.Code,
		data.ExpirePrefix + data.ExpireStrong + data.ExpireSuffix,
		data.IgnoreText,
		data.SupportText,
	}
	if data.SiteURL != "" {
		lines = append(lines, strings.TrimSpace(data.SiteLabel+" "+data.SiteURL))
	}
	return strings.Join(lines, "\n\n")
}

func (s *Service) buildOrderStatusEmail(input notificationcontract.OrderStatusEmailInput, locale string) (string, string) {
	normalized := normalizeLocale(locale)
	siteName := strings.TrimSpace(input.SiteName)
	siteURL := strings.TrimRight(strings.TrimSpace(input.SiteURL), "/")
	subject, plainBody := buildOrderStatusContent(input, locale)
	statusKey := "order.status." + strings.ToLower(strings.TrimSpace(input.Status))
	statusLabel := i18n.T(normalized, statusKey)
	if statusLabel == statusKey {
		statusLabel = input.Status
	}

	var (
		greeting        string
		intro           string
		orderNoLabel    string
		statusLabelText string
		amountLabel     string
		deliveryLabel   string
		guestTipLabel   string
	)

	switch normalized {
	case i18n.LocaleZH:
		greeting = "您好，"
		intro = fmt.Sprintf("您在 %s 的订单状态已更新。", siteName)
		orderNoLabel = "订单号"
		statusLabelText = "订单状态"
		amountLabel = "订单金额"
		deliveryLabel = "交付内容"
		guestTipLabel = "游客提示"
	case i18n.LocaleTW:
		greeting = "您好，"
		intro = fmt.Sprintf("您在 %s 的訂單狀態已更新。", siteName)
		orderNoLabel = "訂單號"
		statusLabelText = "訂單狀態"
		amountLabel = "訂單金額"
		deliveryLabel = "交付內容"
		guestTipLabel = "遊客提示"
	case i18n.LocaleEN:
		greeting = "Hello,"
		intro = fmt.Sprintf("Your order status at %s has been updated.", siteName)
		orderNoLabel = "Order No."
		statusLabelText = "Status"
		amountLabel = "Amount"
		deliveryLabel = "Delivery"
		guestTipLabel = "Guest tip"
	}

	var guestTip string
	if input.IsGuest {
		tipKey := "email.order_status.guest_tip"
		tip := i18n.T(normalized, tipKey)
		if tip != tipKey {
			guestTip = tip
		}
	}

	infoText := resolveOrderStatusEmailInfoText(normalized)
	body := renderEmbeddedEmail("order_status", orderStatusEmailData{
		emailInfoData: emailInfoData{
			SiteName:    siteName,
			SiteURL:     siteURL,
			SupportText: infoText.SupportText,
			SiteLabel:   infoText.SiteLabel,
			ShowInfo:    siteURL != "",
		},
		ShowSummary:     true,
		Greeting:        greeting,
		Intro:           intro,
		OrderNoLabel:    orderNoLabel,
		OrderNo:         strings.TrimSpace(input.OrderNo),
		StatusLabelText: statusLabelText,
		StatusLabel:     statusLabel,
		AmountLabel:     amountLabel,
		Amount:          strings.TrimSpace(input.Amount.String() + " " + input.Currency),
		Paragraphs:      splitEmailParagraphs(plainBody),
		DeliveryLabel:   deliveryLabel,
		FulfillmentInfo: strings.TrimSpace(input.FulfillmentInfo),
		GuestTipLabel:   guestTipLabel,
		GuestTip:        guestTip,
	}, emailLayoutData{
		Lang:     emailLangCode(normalized),
		Title:    subject,
		SiteName: siteName,
	})
	return subject, body
}

func (s *Service) buildOrderStatusTemplateEmail(input notificationcontract.OrderStatusEmailInput, locale, subject, plainBody string) string {
	normalized := normalizeLocale(locale)
	siteName := strings.TrimSpace(input.SiteName)
	siteURL := strings.TrimRight(strings.TrimSpace(input.SiteURL), "/")
	infoText := resolveOrderStatusEmailInfoText(normalized)
	return renderEmbeddedEmail("order_status", orderStatusEmailData{
		emailInfoData: emailInfoData{
			SiteName:    siteName,
			SiteURL:     siteURL,
			SupportText: infoText.SupportText,
			SiteLabel:   infoText.SiteLabel,
			ShowInfo:    siteURL != "",
		},
		Paragraphs: splitEmailParagraphs(plainBody),
	}, emailLayoutData{
		Lang:     emailLangCode(normalized),
		Title:    subject,
		SiteName: siteName,
	})
}

type orderStatusEmailInfoText struct {
	SupportText string
	SiteLabel   string
}

func resolveOrderStatusEmailInfoText(locale string) orderStatusEmailInfoText {
	var text orderStatusEmailInfoText
	switch normalizeLocale(locale) {
	case i18n.LocaleZH:
		text.SupportText = "如果您有任何问题或者需要帮助，请登录我们的官方网站，联系我们的技术支持团队，请勿直接回复本邮件。"
		text.SiteLabel = "官方网站："
	case i18n.LocaleTW:
		text.SupportText = "如果您有任何問題或需要協助，請登入我們的官方網站聯絡技術支援團隊。請勿直接回復本郵件。"
		text.SiteLabel = "官方網站："
	case i18n.LocaleEN:
		text.SupportText = "If you need help, please visit our official website and contact the support team. Please do not reply to this email directly."
		text.SiteLabel = "Official website:"
	}
	return text
}

func buildOrderStatusContent(input notificationcontract.OrderStatusEmailInput, locale string) (string, string) {
	normalized := normalizeLocale(locale)
	statusKey := "order.status." + strings.ToLower(strings.TrimSpace(input.Status))
	statusLabel := i18n.T(normalized, statusKey)
	if statusLabel == statusKey {
		statusLabel = input.Status
	}
	amount := input.Amount.String()
	refundAmount := input.RefundAmount.String()
	refundReason := strings.TrimSpace(input.RefundReason)
	currency := strings.TrimSpace(input.Currency)
	siteName := strings.TrimSpace(input.SiteName)
	siteURL := strings.TrimSpace(input.SiteURL)
	subject := i18n.Sprintf(normalized, "email.order_status.subject", statusLabel)
	payload := strings.TrimSpace(input.FulfillmentInfo)
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case constants.OrderStatusDelivered, constants.OrderStatusCompleted:
		if payload != "" {
			body := i18n.Sprintf(normalized, "email.order_status.body_delivered", input.OrderNo, statusLabel, amount, currency, payload, siteName, siteURL)
			return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
		}
		body := i18n.Sprintf(normalized, "email.order_status.body_delivered_simple", input.OrderNo, statusLabel, amount, currency, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	case constants.OrderStatusPaid:
		body := i18n.Sprintf(normalized, "email.order_status.body_paid", input.OrderNo, statusLabel, amount, currency, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	case constants.OrderStatusRefunded:
		body := i18n.Sprintf(normalized, "email.order_status.body_refunded", input.OrderNo, statusLabel, refundAmount, currency, refundReason, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	case constants.OrderStatusPartiallyRefunded:
		body := i18n.Sprintf(normalized, "email.order_status.body_partially_refunded", input.OrderNo, statusLabel, refundAmount, currency, refundReason, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	default:
		body := i18n.Sprintf(normalized, "email.order_status.body", input.OrderNo, statusLabel, amount, currency, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	}
}

func appendFulfillmentAttachmentTip(locale string, input notificationcontract.OrderStatusEmailInput, body string) string {
	if input.AttachmentName == "" {
		return body
	}
	tipKey := "email.order_status.fulfillment_attachment_tip"
	tip := i18n.T(locale, tipKey)
	if tip == tipKey {
		return body
	}
	return body + "\n\n" + tip
}

func appendGuestTip(locale string, input notificationcontract.OrderStatusEmailInput, body string) string {
	if !input.IsGuest {
		return body
	}
	tipKey := "email.order_status.guest_tip"
	tip := i18n.T(locale, tipKey)
	if tip == tipKey {
		return body
	}
	return body + "\n\n" + tip
}

func normalizeLocale(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	switch {
	case strings.HasPrefix(l, "zh-tw"), strings.HasPrefix(l, "zh-hk"), strings.HasPrefix(l, "zh-mo"):
		return i18n.LocaleTW
	case strings.HasPrefix(l, "en"):
		return i18n.LocaleEN
	default:
		return i18n.LocaleZH
	}
}

func buildFromAddress(from, name string) string {
	name = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(name))
	if name == "" {
		return from
	}
	// mail.Address.String performs the required RFC 2047 encoding itself.
	// Pre-encoding here would make clients display the encoded-word literally.
	return (&mail.Address{Name: name, Address: from}).String()
}

func normalizeReplyToHeader(raw string) string {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return addr.Address
}

func firstMailBrand(brands []mailbrand.Brand) mailbrand.Brand {
	if len(brands) == 0 {
		return mailbrand.Brand{}
	}
	return brands[0]
}

func buildEmailMessage(from, to, subject, body string, replyTo ...string) string {
	return buildEmailMessageWithHeaders(from, to, subject, body, nil, replyTo...)
}

func buildEmailMessageWithHeaders(from, to, subject, body string, headers []emailHeader, replyTo ...string) string {
	var buf bytes.Buffer
	writeStandardHeaders(&buf, from, to, subject, replyTo...)
	for _, header := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", header.Name, header.Value)
	}
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	return buf.String()
}

func buildAlternativeEmailMessage(from, to, subject, plainBody, htmlBody string, headers []emailHeader, replyTo ...string) string {
	boundary := generateMIMEBoundary()

	var buf bytes.Buffer
	writeStandardHeaders(&buf, from, to, subject, replyTo...)
	for _, header := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", header.Name, header.Value)
	}
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary)
	buf.WriteString("\r\n")

	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")
	writeMIMEBase64(&buf, []byte(plainBody))

	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")
	writeMIMEBase64(&buf, []byte(htmlBody))

	fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	return buf.String()
}

func writeMIMEBase64(buf *bytes.Buffer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLength = 76
	for len(encoded) > lineLength {
		buf.WriteString(encoded[:lineLength])
		buf.WriteString("\r\n")
		encoded = encoded[lineLength:]
	}
	buf.WriteString(encoded)
	buf.WriteString("\r\n")
}

func generateMIMEBoundary() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "----=_DujiaoNextAlternative_" + hex.EncodeToString(b[:])
}

func sendMailWithSSL(addr, host, from string, to []string, msg []byte, username, password string) (err error) {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil && !isSMTPAlreadyClosedError(closeErr) {
			logger.Debugw("smtp_tls_conn_close_failed", "host", host, "addr", addr, "error", closeErr)
		}
		return err
	}
	defer closeSMTPClientOnError(client, &err, host, addr)

	if err := authenticateSMTPClient(client, host, username, password); err != nil {
		return err
	}

	err = sendSMTPData(client, host, addr, from, to, msg)
	return err
}

func sendMailWithStartTLS(addr, host, from string, to []string, msg []byte, username, password string) (err error) {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer closeSMTPClientOnError(client, &err, host, addr)

	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return err
	}

	if err := authenticateSMTPClient(client, host, username, password); err != nil {
		return err
	}

	err = sendSMTPData(client, host, addr, from, to, msg)
	return err
}

func sendMailPlain(addr, host, from string, to []string, msg []byte, username, password string) (err error) {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer closeSMTPClientOnError(client, &err, host, addr)

	if err := authenticateSMTPClient(client, host, username, password); err != nil {
		return err
	}

	err = sendSMTPData(client, host, addr, from, to, msg)
	return err
}

const (
	smtpAuthMechanismPlain = "PLAIN"
	smtpAuthMechanismLogin = "LOGIN"
	// 还有 XOAUTH2 等机制，当前实现暂不处理。
)

// authenticateSMTPClient 根据服务端 AUTH 能力选择并执行认证。
// Service 认证策略：优先 LOGIN，回退 PLAIN。
// 对 smtp.office365.com 的 SMTP Basic/LOGIN 场景，通常需要开启 MFA 并使用应用密码。
func authenticateSMTPClient(client *smtp.Client, host, username, password string) error {
	if client == nil {
		return nil
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" && password == "" {
		return nil
	}

	ok, advertised := client.Extension("AUTH")
	if !ok {
		return nil
	}

	switch pickSMTPAuthMechanism(advertised) {
	case smtpAuthMechanismLogin:
		return client.Auth(newLoginAuth(username, password, host))
	case smtpAuthMechanismPlain:
		return client.Auth(smtp.PlainAuth("", username, password, host))
	default:
		return fmt.Errorf("smtp auth mechanism not supported (server AUTH=%q)", advertised)
	}
}

// pickSMTPAuthMechanism 根据服务端 AUTH 能力选择机制，优先 LOGIN，再回退 PLAIN。
func pickSMTPAuthMechanism(advertised string) string {
	if hasSMTPAuthMechanism(advertised, smtpAuthMechanismLogin) {
		return smtpAuthMechanismLogin
	}
	if hasSMTPAuthMechanism(advertised, smtpAuthMechanismPlain) {
		return smtpAuthMechanismPlain
	}
	return ""
}

// hasSMTPAuthMechanism 判断服务端 AUTH 扩展是否包含指定机制。
func hasSMTPAuthMechanism(advertised, mechanism string) bool {
	if strings.TrimSpace(mechanism) == "" {
		return false
	}
	tokens := strings.Fields(strings.ToUpper(strings.TrimSpace(advertised)))
	needle := strings.ToUpper(strings.TrimSpace(mechanism))
	for _, token := range tokens {
		if token == needle {
			return true
		}
	}
	return false
}

type loginAuth struct {
	username string
	password string
	host     string
	userSent bool
}

// newLoginAuth 构造 AUTH LOGIN 认证器。
func newLoginAuth(username, password, host string) smtp.Auth {
	return &loginAuth{username: username, password: password, host: host}
}

// Start 校验连接安全性并声明 LOGIN 机制。
func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server == nil {
		return "", nil, fmt.Errorf("smtp server info is required")
	}
	if server.Name != a.host {
		return "", nil, fmt.Errorf("wrong host name")
	}
	if !server.TLS {
		return "", nil, fmt.Errorf("unencrypted connection")
	}
	a.userSent = false
	return smtpAuthMechanismLogin, nil, nil
}

// Next 按服务端 challenge 顺序回送用户名与密码。
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	challenge := strings.ToLower(strings.TrimSpace(string(fromServer)))
	if strings.Contains(challenge, "password") {
		return []byte(a.password), nil
	}
	if strings.Contains(challenge, "username") || strings.Contains(challenge, "user name") {
		a.userSent = true
		return []byte(a.username), nil
	}
	if !a.userSent {
		a.userSent = true
		return []byte(a.username), nil
	}
	return []byte(a.password), nil
}

type smtpSessionCloser interface {
	Quit() error
	Close() error
}

// sendSMTPData 发送 SMTP Envelope 与邮件正文。
func sendSMTPData(client *smtp.Client, host, addr, from string, to []string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return quitSMTPClient(client, host, addr)
}

// quitSMTPClient 优先执行 SMTP QUIT；仅在 QUIT 失败时补偿 Close 回收连接。
func quitSMTPClient(client smtpSessionCloser, host, addr string) error {
	if client == nil {
		return nil
	}
	if err := client.Quit(); err != nil {
		if closeErr := client.Close(); closeErr != nil && !isSMTPAlreadyClosedError(closeErr) {
			logger.Debugw("smtp_close_after_quit_failed", "host", host, "addr", addr, "error", closeErr)
		}
		return err
	}
	return nil
}

// closeSMTPClientOnError 仅在发送流程异常时兜底 Close，避免成功路径重复关闭噪音。
func closeSMTPClientOnError(client *smtp.Client, sendErr *error, host, addr string) {
	if client == nil || sendErr == nil || *sendErr == nil {
		return
	}
	if err := client.Close(); err != nil && !isSMTPAlreadyClosedError(err) {
		logger.Debugw("smtp_close_failed", "host", host, "addr", addr, "error", err)
	}
}

// isSMTPAlreadyClosedError 识别连接已关闭类错误，避免重复记录无效噪音。
func isSMTPAlreadyClosedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "closed connection") ||
		strings.Contains(message, "connection is closed")
}

// normalizeEmailSendError 将可识别的收件人拒绝错误归一化为业务错误码。
func normalizeEmailSendError(err error) error {
	if err == nil {
		return nil
	}
	if isEmailRecipientRejected(err) {
		return notificationcontract.ErrEmailRecipientRejected
	}
	return err
}

// isEmailRecipientRejected 识别常见 SMTP 收件人不存在/被拒绝错误。
func isEmailRecipientRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	directKeywords := []string{
		"no such recipient",
		"no such user",
		"recipient not found",
		"recipient address rejected",
		"invalid recipient",
		"user unknown",
		"unknown user",
		"unknown mailbox",
		"mailbox unavailable",
	}
	for _, keyword := range directKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	if strings.Contains(message, "550") {
		hints := []string{"recipient", "user", "mailbox", "address", "rcpt"}
		for _, hint := range hints {
			if strings.Contains(message, hint) {
				return true
			}
		}
	}
	return false
}
