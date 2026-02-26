package public

import handlershared "github.com/dujiao-next/internal/http/handlers/shared"

// CaptchaPayloadRequest 验证码请求载荷
// 前端提交时根据 provider 传入对应字段
// image: captcha_id + captcha_code
// turnstile: turnstile_token
// 未启用场景允许空载荷
// 由 service 层根据配置判定是否必填
type CaptchaPayloadRequest = handlershared.CaptchaPayloadRequest
