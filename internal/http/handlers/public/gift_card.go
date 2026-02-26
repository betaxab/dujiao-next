package public

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemGiftCardRequest 兑换礼品卡请求
type RedeemGiftCardRequest struct {
	Code           string                `json:"code" binding:"required"`
	CaptchaPayload CaptchaPayloadRequest `json:"captcha_payload"`
}

// RedeemGiftCard 用户兑换礼品卡
func (h *Handler) RedeemGiftCard(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	var req RedeemGiftCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if h.CaptchaService != nil {
		if captchaErr := h.CaptchaService.Verify(constants.CaptchaSceneGiftCardRedeem, req.CaptchaPayload.ToServicePayload(), c.ClientIP()); captchaErr != nil {
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

	card, account, txn, err := h.GiftCardService.RedeemGiftCard(service.GiftCardRedeemInput{
		UserID: uid,
		Code:   strings.TrimSpace(req.Code),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGiftCardInvalid):
			respondError(c, response.CodeBadRequest, "error.gift_card_invalid", nil)
		case errors.Is(err, service.ErrGiftCardNotFound):
			respondError(c, response.CodeNotFound, "error.gift_card_not_found", nil)
		case errors.Is(err, service.ErrGiftCardExpired):
			respondError(c, response.CodeBadRequest, "error.gift_card_expired", nil)
		case errors.Is(err, service.ErrGiftCardDisabled):
			respondError(c, response.CodeBadRequest, "error.gift_card_disabled", nil)
		case errors.Is(err, service.ErrGiftCardRedeemed):
			respondError(c, response.CodeBadRequest, "error.gift_card_redeemed", nil)
		default:
			respondError(c, response.CodeInternal, "error.gift_card_redeem_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"gift_card":    card,
		"wallet":       account,
		"transaction":  txn,
		"wallet_delta": card.Amount,
	})
}
