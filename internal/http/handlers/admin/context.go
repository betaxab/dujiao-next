package admin

import (
	handlershared "github.com/dujiao-next/internal/http/handlers/shared"

	"github.com/gin-gonic/gin"
)

func getContextUintWithKeys(c *gin.Context, key, invalidKey, typeInvalidKey string) (uint, bool) {
	return handlershared.GetContextUintWithKeys(c, key, invalidKey, typeInvalidKey)
}

func getAdminID(c *gin.Context) (uint, bool) {
	return getContextUintWithKeys(c, "admin_id", "error.admin_id_invalid", "error.admin_id_type_invalid")
}
