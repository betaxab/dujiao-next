package public

import (
	"errors"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// GetImageCaptcha 获取图片验证码挑战
func (h *Handler) GetImageCaptcha(c *gin.Context) {
	if h.CaptchaService == nil {
		respondError(c, response.CodeInternal, "error.captcha_unavailable", service.ErrCaptchaConfigInvalid)
		return
	}

	challenge, err := h.CaptchaService.GenerateImageChallenge()
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCaptchaConfigInvalid):
			respondError(c, response.CodeBadRequest, "error.captcha_unavailable", nil)
		default:
			respondError(c, response.CodeInternal, "error.captcha_generate_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"captcha_id":   challenge.CaptchaID,
		"image_base64": challenge.ImageBase64,
	})
}
