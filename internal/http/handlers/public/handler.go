package public

import "github.com/dujiao-next/internal/provider"

// Handler 前台/公开接口处理器入口
// 说明：该处理器仅用于前台、游客、用户侧 API。
type Handler struct {
	*provider.Container
}

// New 创建前台处理器
func New(c *provider.Container) *Handler {
	return &Handler{Container: c}
}
