package admin

import (
	"errors"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// GetCaptchaSettings 获取验证码配置（脱敏）
func (h *Handler) GetCaptchaSettings(c *gin.Context) {
	setting, err := h.SettingService.GetCaptchaSetting(h.Config.Captcha)
	if err != nil {
		respondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	response.Success(c, service.MaskCaptchaSettingForAdmin(setting))
}

// UpdateCaptchaSettings 更新验证码配置
func (h *Handler) UpdateCaptchaSettings(c *gin.Context) {
	var req service.CaptchaSettingPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	setting, err := h.SettingService.PatchCaptchaSetting(h.Config.Captcha, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCaptchaConfigInvalid):
			respondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
		default:
			respondError(c, response.CodeInternal, "error.settings_save_failed", err)
		}
		return
	}

	h.Config.Captcha = service.CaptchaSettingToConfig(setting)
	if h.CaptchaService != nil {
		h.CaptchaService.SetDefaultConfig(h.Config.Captcha)
		h.CaptchaService.InvalidateCache()
	}
	_ = cache.Del(c.Request.Context(), publicConfigCacheKey)

	response.Success(c, service.MaskCaptchaSettingForAdmin(setting))
}
