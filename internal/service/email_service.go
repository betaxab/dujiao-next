package service

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/models"
)

// EmailService 邮件发送服务
type EmailService struct {
	cfg *config.EmailConfig
}

// NewEmailService 创建邮件服务
func NewEmailService(cfg *config.EmailConfig) *EmailService {
	return &EmailService{cfg: cfg}
}

// SetConfig 更新运行时邮件配置
func (s *EmailService) SetConfig(cfg *config.EmailConfig) {
	if cfg == nil {
		return
	}
	s.cfg = cfg
}

// SendVerifyCode 发送邮箱验证码
func (s *EmailService) SendVerifyCode(toEmail, code, purpose, locale string) error {
	subject, body := buildVerifyCodeContent(code, purpose, locale)
	return s.sendTextEmail(toEmail, subject, body)
}

// OrderStatusEmailInput 订单状态邮件输入
type OrderStatusEmailInput struct {
	OrderNo         string
	Status          string
	Amount          models.Money
	Currency        string
	FulfillmentInfo string
	IsGuest         bool
}

// SendOrderStatusEmail 发送订单状态通知
func (s *EmailService) SendOrderStatusEmail(toEmail string, input OrderStatusEmailInput, locale string) error {
	subject, body := buildOrderStatusContent(input, locale)
	return s.sendTextEmail(toEmail, subject, body)
}

// SendCustomEmail 发送测试邮件或自定义邮件
func (s *EmailService) SendCustomEmail(toEmail, subject, body string) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "SMTP 配置测试邮件"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "这是一封来自 D&J Studio 的 SMTP 测试邮件，说明当前配置可正常发送。"
	}
	return s.sendTextEmail(toEmail, subject, body)
}

func (s *EmailService) sendTextEmail(toEmail, subject, body string) error {
	if s.cfg == nil || !s.cfg.Enabled {
		return ErrEmailServiceDisabled
	}
	if s.cfg.Host == "" || s.cfg.Port == 0 || s.cfg.From == "" {
		return ErrEmailServiceNotConfigured
	}
	if _, err := mail.ParseAddress(toEmail); err != nil {
		return ErrInvalidEmail
	}

	from := buildFromAddress(s.cfg.From, s.cfg.FromName)
	msg := buildEmailMessage(from, toEmail, subject, body)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	var auth smtp.Auth
	if s.cfg.Username != "" || s.cfg.Password != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	if s.cfg.UseSSL {
		return normalizeEmailSendError(sendMailWithSSL(addr, auth, s.cfg.Host, s.cfg.From, []string{toEmail}, []byte(msg)))
	}
	if s.cfg.UseTLS {
		return normalizeEmailSendError(sendMailWithStartTLS(addr, auth, s.cfg.Host, s.cfg.From, []string{toEmail}, []byte(msg)))
	}
	return normalizeEmailSendError(sendMailPlain(addr, auth, s.cfg.Host, s.cfg.From, []string{toEmail}, []byte(msg)))
}

func buildVerifyCodeContent(code, purpose, locale string) (string, string) {
	normalized := normalizeLocale(locale)
	purposeKey := strings.ToLower(strings.TrimSpace(purpose))
	switch normalized {
	case i18n.LocaleTW:
		subject := "郵箱驗證碼"
		purposeText := "郵箱驗證"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "註冊驗證碼"
			purposeText = "註冊"
		case constants.VerifyPurposeReset:
			subject = "重置密碼驗證碼"
			purposeText = "重置密碼"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "更換郵箱驗證碼"
			purposeText = "更換郵箱"
		}
		body := fmt.Sprintf("您的驗證碼是：%s\n\n該驗證碼用於 %s，請勿洩露。", code, purposeText)
		return subject, body
	case i18n.LocaleEN:
		subject := "Email Verification Code"
		purposeText := "email verification"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "Registration Code"
			purposeText = "registration"
		case constants.VerifyPurposeReset:
			subject = "Password Reset Code"
			purposeText = "password reset"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "Change Email Code"
			purposeText = "change email"
		}
		body := fmt.Sprintf("Your verification code is: %s\n\nThis code is for %s. Do not share it.", code, purposeText)
		return subject, body
	default:
		subject := "邮箱验证码"
		purposeText := "邮箱验证"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "注册验证码"
			purposeText = "注册"
		case constants.VerifyPurposeReset:
			subject = "重置密码验证码"
			purposeText = "重置密码"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "更换邮箱验证码"
			purposeText = "更换邮箱"
		}
		body := fmt.Sprintf("您的验证码是：%s\n\n该验证码用于 %s，请勿泄露。", code, purposeText)
		return subject, body
	}
}

func buildOrderStatusContent(input OrderStatusEmailInput, locale string) (string, string) {
	normalized := normalizeLocale(locale)
	statusKey := "order.status." + strings.ToLower(strings.TrimSpace(input.Status))
	statusLabel := i18n.T(normalized, statusKey)
	if statusLabel == statusKey {
		statusLabel = input.Status
	}
	amount := input.Amount.String()
	currency := strings.TrimSpace(input.Currency)
	subject := i18n.Sprintf(normalized, "email.order_status.subject", statusLabel)
	payload := strings.TrimSpace(input.FulfillmentInfo)
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case constants.OrderStatusDelivered:
		if payload != "" {
			body := i18n.Sprintf(normalized, "email.order_status.body_delivered", input.OrderNo, statusLabel, amount, currency, payload)
			return subject, appendGuestTip(normalized, input, body)
		}
		body := i18n.Sprintf(normalized, "email.order_status.body_delivered_simple", input.OrderNo, statusLabel, amount, currency)
		return subject, appendGuestTip(normalized, input, body)
	case constants.OrderStatusPaid:
		body := i18n.Sprintf(normalized, "email.order_status.body_paid", input.OrderNo, statusLabel, amount, currency)
		return subject, appendGuestTip(normalized, input, body)
	case constants.OrderStatusCanceled:
		body := i18n.Sprintf(normalized, "email.order_status.body_canceled", input.OrderNo, statusLabel, amount, currency)
		return subject, appendGuestTip(normalized, input, body)
	default:
		body := i18n.Sprintf(normalized, "email.order_status.body", input.OrderNo, statusLabel, amount, currency)
		return subject, appendGuestTip(normalized, input, body)
	}
}

func appendGuestTip(locale string, input OrderStatusEmailInput, body string) string {
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
	if strings.TrimSpace(name) == "" {
		return from
	}
	encoded := mime.QEncoding.Encode("UTF-8", name)
	return (&mail.Address{Name: encoded, Address: from}).String()
}

func buildEmailMessage(from, to, subject, body string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject)))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	return buf.String()
}

func sendMailWithSSL(addr string, auth smtp.Auth, host, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}

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
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func sendMailWithStartTLS(addr string, auth smtp.Auth, host, from string, to []string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return err
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}

	return sendSMTPData(client, from, to, msg)
}

func sendMailPlain(addr string, auth smtp.Auth, host, from string, to []string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}

	return sendSMTPData(client, from, to, msg)
}

func sendSMTPData(client *smtp.Client, from string, to []string, msg []byte) error {
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
	return client.Quit()
}

func normalizeEmailSendError(err error) error {
	if err == nil {
		return nil
	}
	if isEmailRecipientRejected(err) {
		return ErrEmailRecipientRejected
	}
	return err
}

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
