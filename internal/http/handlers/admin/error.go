package admin

import (
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func requestLog(c *gin.Context) *zap.SugaredLogger {
	if c == nil {
		return logger.S()
	}
	if requestID, ok := c.Get("request_id"); ok {
		if id, ok := requestID.(string); ok && id != "" {
			return logger.SW("request_id", id)
		}
	}
	return logger.S()
}

func respondError(c *gin.Context, code int, key string, err error) {
	locale := i18n.ResolveLocale(c)
	msg := i18n.T(locale, key)
	appErr := response.WrapError(code, msg, err)
	if err != nil {
		requestLog(c).Errorw("handler_error",
			"code", appErr.Code,
			"message", appErr.Message,
			"error", err,
		)
	}
	response.Error(c, appErr.Code, appErr.Message)
}

func respondErrorWithMsg(c *gin.Context, code int, msg string, err error) {
	appErr := response.WrapError(code, msg, err)
	if err != nil {
		requestLog(c).Errorw("handler_error",
			"code", appErr.Code,
			"message", appErr.Message,
			"error", err,
		)
	}
	response.Error(c, appErr.Code, appErr.Message)
}
