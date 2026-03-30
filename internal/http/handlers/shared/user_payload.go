package shared

import (
	"github.com/dujiao-next/internal/models"

	"github.com/gin-gonic/gin"
)

// BuildTelegramBindingResponse 构造 Telegram 绑定信息响应载荷。
func BuildTelegramBindingResponse(identity *models.UserOAuthIdentity) gin.H {
	if identity == nil {
		return gin.H{"bound": false}
	}
	return gin.H{
		"bound":            true,
		"provider":         identity.Provider,
		"provider_user_id": identity.ProviderUserID,
		"username":         identity.Username,
		"avatar_url":       identity.AvatarURL,
		"auth_at":          identity.AuthAt,
		"updated_at":       identity.UpdatedAt,
	}
}
