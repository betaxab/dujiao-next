package admin

import (
	"strings"

	"github.com/dujiao-next/internal/service"
)

// CaptchaPayloadRequest 管理端验证码请求载荷
type CaptchaPayloadRequest struct {
	CaptchaID      string `json:"captcha_id"`
	CaptchaCode    string `json:"captcha_code"`
	TurnstileToken string `json:"turnstile_token"`
}

func (r CaptchaPayloadRequest) toServicePayload() service.CaptchaVerifyPayload {
	return service.CaptchaVerifyPayload{
		CaptchaID:      strings.TrimSpace(r.CaptchaID),
		CaptchaCode:    strings.TrimSpace(r.CaptchaCode),
		TurnstileToken: strings.TrimSpace(r.TurnstileToken),
	}
}
