package admin

import (
	"github.com/dujiao-next/internal/http/response"

	"github.com/gin-gonic/gin"
)

func getContextUintWithKeys(c *gin.Context, key, invalidKey, typeInvalidKey string) (uint, bool) {
	value, exists := c.Get(key)
	if !exists {
		respondError(c, response.CodeUnauthorized, "error.unauthorized", nil)
		return 0, false
	}

	switch v := value.(type) {
	case uint:
		return v, true
	case int:
		if v < 0 {
			respondError(c, response.CodeBadRequest, invalidKey, nil)
			return 0, false
		}
		return uint(v), true
	case float64:
		if v < 0 {
			respondError(c, response.CodeBadRequest, invalidKey, nil)
			return 0, false
		}
		return uint(v), true
	default:
		respondError(c, response.CodeInternal, typeInvalidKey, nil)
		return 0, false
	}
}

func getAdminID(c *gin.Context) (uint, bool) {
	return getContextUintWithKeys(c, "admin_id", "error.admin_id_invalid", "error.admin_id_type_invalid")
}
