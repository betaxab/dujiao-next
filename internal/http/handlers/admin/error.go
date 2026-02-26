package admin

import (
	handlershared "github.com/dujiao-next/internal/http/handlers/shared"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func requestLog(c *gin.Context) *zap.SugaredLogger {
	return handlershared.RequestLog(c)
}

func respondError(c *gin.Context, code int, key string, err error) {
	handlershared.RespondError(c, code, key, err)
}

func respondErrorWithMsg(c *gin.Context, code int, msg string, err error) {
	handlershared.RespondErrorWithMsg(c, code, msg, err)
}
