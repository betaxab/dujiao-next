package public

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/constants"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// UserSendVerifyCodeRequest 发送验证码请求
type UserSendVerifyCodeRequest struct {
	Email          string                `json:"email" binding:"required"`
	Purpose        string                `json:"purpose" binding:"required"`
	CaptchaPayload CaptchaPayloadRequest `json:"captcha_payload"`
}

// SendUserVerifyCode 发送用户邮箱验证码
func (h *Handler) SendUserVerifyCode(c *gin.Context) {
	var req UserSendVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	captchaScene := ""
	switch purpose {
	case constants.VerifyPurposeRegister:
		captchaScene = constants.CaptchaSceneRegisterSendCode
	case constants.VerifyPurposeReset:
		captchaScene = constants.CaptchaSceneResetSendCode
	}
	if captchaScene != "" && h.CaptchaService != nil {
		if captchaErr := h.CaptchaService.Verify(captchaScene, req.CaptchaPayload.toServicePayload(), c.ClientIP()); captchaErr != nil {
			switch {
			case errors.Is(captchaErr, service.ErrCaptchaRequired):
				respondError(c, response.CodeBadRequest, "error.captcha_required", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaInvalid):
				respondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaConfigInvalid):
				respondError(c, response.CodeInternal, "error.captcha_config_invalid", captchaErr)
				return
			default:
				respondError(c, response.CodeInternal, "error.captcha_verify_failed", captchaErr)
				return
			}
		}
	}

	locale := i18n.ResolveLocale(c)
	if err := h.UserAuthService.SendVerifyCode(req.Email, req.Purpose, locale); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrInvalidVerifyPurpose):
			respondError(c, response.CodeBadRequest, "error.verify_purpose_invalid", nil)
		case errors.Is(err, service.ErrEmailExists):
			respondError(c, response.CodeBadRequest, "error.email_exists", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		case errors.Is(err, service.ErrVerifyCodeTooFrequent):
			respondError(c, response.CodeTooManyRequests, "error.verify_code_too_frequent", nil)
		case errors.Is(err, service.ErrEmailRecipientRejected):
			respondError(c, response.CodeBadRequest, "error.email_recipient_not_found", nil)
		case errors.Is(err, service.ErrEmailServiceDisabled),
			errors.Is(err, service.ErrEmailServiceNotConfigured):
			respondError(c, response.CodeInternal, "error.email_service_not_configured", err)
		default:
			respondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"sent": true})
}

// UserRegisterRequest 注册请求
type UserRegisterRequest struct {
	Email             string `json:"email" binding:"required"`
	Password          string `json:"password" binding:"required"`
	Code              string `json:"code" binding:"required"`
	AgreementAccepted bool   `json:"agreement_accepted"`
}

// UserRegister 用户注册
func (h *Handler) UserRegister(c *gin.Context) {
	var req UserRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	user, token, expiresAt, err := h.UserAuthService.Register(req.Email, req.Password, req.Code, req.AgreementAccepted)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailExists):
			respondError(c, response.CodeBadRequest, "error.email_exists", nil)
		case errors.Is(err, service.ErrVerifyCodeInvalid):
			respondError(c, response.CodeBadRequest, "error.verify_code_invalid", nil)
		case errors.Is(err, service.ErrVerifyCodeExpired):
			respondError(c, response.CodeBadRequest, "error.verify_code_expired", nil)
		case errors.Is(err, service.ErrVerifyCodeAttemptsExceeded):
			respondError(c, response.CodeBadRequest, "error.verify_code_attempts_exceeded", nil)
		case errors.Is(err, service.ErrAgreementRequired):
			respondError(c, response.CodeBadRequest, "error.agreement_required", nil)
		case errors.Is(err, service.ErrWeakPassword):
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				respondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			respondError(c, response.CodeBadRequest, "error.password_weak", nil)
		default:
			respondError(c, response.CodeInternal, "error.register_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"user": gin.H{
			"id":                user.ID,
			"email":             user.Email,
			"nickname":          user.DisplayName,
			"email_verified_at": user.EmailVerifiedAt,
		},
		"token":      token,
		"expires_at": expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// UserLoginRequest 登录请求
type UserLoginRequest struct {
	Email          string                `json:"email" binding:"required"`
	Password       string                `json:"password" binding:"required"`
	RememberMe     bool                  `json:"remember_me"`
	CaptchaPayload CaptchaPayloadRequest `json:"captcha_payload"`
}

// UserLogin 用户登录
func (h *Handler) UserLogin(c *gin.Context) {
	var req UserLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonBadRequest)
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if h.CaptchaService != nil {
		if captchaErr := h.CaptchaService.Verify(constants.CaptchaSceneLogin, req.CaptchaPayload.toServicePayload(), c.ClientIP()); captchaErr != nil {
			switch {
			case errors.Is(captchaErr, service.ErrCaptchaRequired):
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaRequired)
				respondError(c, response.CodeBadRequest, "error.captcha_required", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaInvalid):
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaInvalid)
				respondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaConfigInvalid):
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaConfigInvalid)
				respondError(c, response.CodeInternal, "error.captcha_config_invalid", captchaErr)
				return
			default:
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaVerifyFailed)
				respondError(c, response.CodeInternal, "error.captcha_verify_failed", captchaErr)
				return
			}
		}
	}

	user, token, expiresAt, err := h.UserAuthService.LoginWithRememberMe(req.Email, req.Password, req.RememberMe)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonInvalidEmail)
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrInvalidCredentials):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonInvalidCredentials)
			respondError(c, response.CodeUnauthorized, "error.login_invalid", nil)
		case errors.Is(err, service.ErrEmailNotVerified):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonEmailNotVerified)
			respondError(c, response.CodeUnauthorized, "error.email_not_verified", nil)
		case errors.Is(err, service.ErrUserDisabled):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonUserDisabled)
			respondError(c, response.CodeUnauthorized, "error.user_disabled", nil)
		default:
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonInternalError)
			respondError(c, response.CodeInternal, "error.login_failed", err)
		}
		return
	}

	h.recordUserLogin(c, user.Email, user.ID, constants.LoginLogStatusSuccess, "")
	response.Success(c, gin.H{
		"user": gin.H{
			"id":                user.ID,
			"email":             user.Email,
			"nickname":          user.DisplayName,
			"email_verified_at": user.EmailVerifiedAt,
		},
		"token":      token,
		"expires_at": expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *Handler) recordUserLogin(c *gin.Context, email string, userID uint, status, failReason string) {
	if h == nil || h.UserLoginLogService == nil {
		return
	}
	requestID := ""
	if c != nil {
		if rid, ok := c.Get("request_id"); ok {
			if value, ok := rid.(string); ok {
				requestID = strings.TrimSpace(value)
			}
		}
	}
	_ = h.UserLoginLogService.Record(service.RecordUserLoginInput{
		UserID:      userID,
		Email:       email,
		Status:      status,
		FailReason:  failReason,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		LoginSource: constants.LoginLogSourceWeb,
		RequestID:   requestID,
	})
}

// UserResetPasswordRequest 重置密码请求
type UserResetPasswordRequest struct {
	Email       string `json:"email" binding:"required"`
	Code        string `json:"code" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UserForgotPassword 忘记密码重置
func (h *Handler) UserForgotPassword(c *gin.Context) {
	var req UserResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if err := h.UserAuthService.ResetPassword(req.Email, req.Code, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		case errors.Is(err, service.ErrVerifyCodeInvalid):
			respondError(c, response.CodeBadRequest, "error.verify_code_invalid", nil)
		case errors.Is(err, service.ErrVerifyCodeExpired):
			respondError(c, response.CodeBadRequest, "error.verify_code_expired", nil)
		case errors.Is(err, service.ErrVerifyCodeAttemptsExceeded):
			respondError(c, response.CodeBadRequest, "error.verify_code_attempts_exceeded", nil)
		case errors.Is(err, service.ErrWeakPassword):
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				respondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			respondError(c, response.CodeBadRequest, "error.password_weak", nil)
		default:
			respondError(c, response.CodeInternal, "error.reset_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"reset": true})
}

// GetCurrentUser 获取当前用户信息
func (h *Handler) GetCurrentUser(c *gin.Context) {
	id, ok := getUserID(c)
	if !ok {
		return
	}

	user, err := h.UserAuthService.GetUserByID(id)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	if user == nil {
		respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		return
	}

	response.Success(c, gin.H{
		"id":                user.ID,
		"email":             user.Email,
		"nickname":          user.DisplayName,
		"email_verified_at": user.EmailVerifiedAt,
		"locale":            user.Locale,
	})
}

// UserProfileUpdateRequest 更新资料请求
type UserProfileUpdateRequest struct {
	Nickname *string `json:"nickname"`
	Locale   *string `json:"locale"`
}

// UpdateUserProfile 更新用户资料
func (h *Handler) UpdateUserProfile(c *gin.Context) {
	id, ok := getUserID(c)
	if !ok {
		return
	}

	var req UserProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	user, err := h.UserAuthService.UpdateProfile(id, req.Nickname, req.Locale)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProfileEmpty):
			respondError(c, response.CodeBadRequest, "error.profile_empty", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.user_update_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"id":                user.ID,
		"email":             user.Email,
		"nickname":          user.DisplayName,
		"email_verified_at": user.EmailVerifiedAt,
		"locale":            user.Locale,
	})
}

// ChangeEmailSendCodeRequest 更换邮箱验证码请求
type ChangeEmailSendCodeRequest struct {
	Kind     string `json:"kind" binding:"required"`
	NewEmail string `json:"new_email"`
}

// SendChangeEmailCode 发送更换邮箱验证码
func (h *Handler) SendChangeEmailCode(c *gin.Context) {
	id, ok := getUserID(c)
	if !ok {
		return
	}

	var req ChangeEmailSendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	locale := i18n.ResolveLocale(c)
	if err := h.UserAuthService.SendChangeEmailCode(id, req.Kind, req.NewEmail, locale); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeInvalid):
			respondError(c, response.CodeBadRequest, "error.email_change_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeExists):
			respondError(c, response.CodeBadRequest, "error.email_change_exists", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		case errors.Is(err, service.ErrVerifyCodeTooFrequent):
			respondError(c, response.CodeTooManyRequests, "error.verify_code_too_frequent", nil)
		case errors.Is(err, service.ErrEmailRecipientRejected):
			respondError(c, response.CodeBadRequest, "error.email_recipient_not_found", nil)
		case errors.Is(err, service.ErrEmailServiceDisabled),
			errors.Is(err, service.ErrEmailServiceNotConfigured):
			respondError(c, response.CodeInternal, "error.email_service_not_configured", err)
		default:
			respondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"sent": true})
}

// ChangeEmailRequest 更换邮箱请求
type ChangeEmailRequest struct {
	NewEmail string `json:"new_email" binding:"required"`
	OldCode  string `json:"old_code" binding:"required"`
	NewCode  string `json:"new_code" binding:"required"`
}

// ChangeEmail 更换邮箱
func (h *Handler) ChangeEmail(c *gin.Context) {
	id, ok := getUserID(c)
	if !ok {
		return
	}

	var req ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	user, err := h.UserAuthService.ChangeEmail(id, req.NewEmail, req.OldCode, req.NewCode)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			respondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeInvalid):
			respondError(c, response.CodeBadRequest, "error.email_change_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeExists):
			respondError(c, response.CodeBadRequest, "error.email_change_exists", nil)
		case errors.Is(err, service.ErrVerifyCodeInvalid):
			respondError(c, response.CodeBadRequest, "error.verify_code_invalid", nil)
		case errors.Is(err, service.ErrVerifyCodeExpired):
			respondError(c, response.CodeBadRequest, "error.verify_code_expired", nil)
		case errors.Is(err, service.ErrVerifyCodeAttemptsExceeded):
			respondError(c, response.CodeBadRequest, "error.verify_code_attempts_exceeded", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.email_change_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"id":                user.ID,
		"email":             user.Email,
		"nickname":          user.DisplayName,
		"email_verified_at": user.EmailVerifiedAt,
		"locale":            user.Locale,
	})
}

// ChangeUserPasswordRequest 用户改密请求
type ChangeUserPasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangeUserPassword 用户登录态修改密码
func (h *Handler) ChangeUserPassword(c *gin.Context) {
	id, ok := getUserID(c)
	if !ok {
		return
	}

	var req ChangeUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if err := h.UserAuthService.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPassword):
			respondError(c, response.CodeBadRequest, "error.password_old_invalid", nil)
		case errors.Is(err, service.ErrWeakPassword):
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				respondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			respondError(c, response.CodeBadRequest, "error.password_weak", nil)
		case errors.Is(err, service.ErrNotFound):
			respondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			respondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"updated": true})
}
