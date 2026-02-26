package shared

import (
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLog 提供携带 request_id 的日志实例。
func RequestLog(c *gin.Context) *zap.SugaredLogger {
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

// RespondError 返回国际化错误响应，并在有原始错误时记录日志。
func RespondError(c *gin.Context, code int, key string, err error) {
	locale := i18n.ResolveLocale(c)
	msg := i18n.T(locale, key)
	appErr := response.WrapError(code, msg, err)
	if err != nil {
		RequestLog(c).Errorw("handler_error",
			"code", appErr.Code,
			"message", appErr.Message,
			"error", err,
		)
	}
	response.Error(c, appErr.Code, appErr.Message)
}

// RespondErrorWithMsg 返回自定义消息错误响应，并在有原始错误时记录日志。
func RespondErrorWithMsg(c *gin.Context, code int, msg string, err error) {
	appErr := response.WrapError(code, msg, err)
	if err != nil {
		RequestLog(c).Errorw("handler_error",
			"code", appErr.Code,
			"message", appErr.Message,
			"error", err,
		)
	}
	response.Error(c, appErr.Code, appErr.Message)
}
