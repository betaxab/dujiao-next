package public

import (
	handlershared "github.com/dujiao-next/internal/http/handlers/shared"

	"github.com/gin-gonic/gin"
)

func getContextUintWithKeys(c *gin.Context, key, invalidKey, typeInvalidKey string) (uint, bool) {
	return handlershared.GetContextUintWithKeys(c, key, invalidKey, typeInvalidKey)
}

func getUserID(c *gin.Context) (uint, bool) {
	return getContextUintWithKeys(c, "user_id", "error.user_id_invalid", "error.user_id_type_invalid")
}
