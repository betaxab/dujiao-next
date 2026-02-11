package public

import (
	"strings"

	"github.com/dujiao-next/internal/service"
)

// CaptchaPayloadRequest 验证码请求载荷
// 前端提交时根据 provider 传入对应字段
// image: captcha_id + captcha_code
// turnstile: turnstile_token
// 未启用场景允许空载荷
// 由 service 层根据配置判定是否必填
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
