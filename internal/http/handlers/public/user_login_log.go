package public

import (
	"strconv"

	"github.com/dujiao-next/internal/http/response"

	"github.com/gin-gonic/gin"
)

// GetMyLoginLogs 获取当前用户登录日志
func (h *Handler) GetMyLoginLogs(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	logs, total, err := h.UserLoginLogService.ListByUser(uid, page, pageSize)
	if err != nil {
		respondError(c, response.CodeInternal, "error.user_login_log_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, logs, pagination)
}
