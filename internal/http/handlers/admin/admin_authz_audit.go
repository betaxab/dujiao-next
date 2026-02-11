package admin

import (
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/repository"

	"github.com/gin-gonic/gin"
)

// ListAuthzAuditLogs 获取权限审计日志列表
func (h *Handler) ListAuthzAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizePagination(page, pageSize)

	operatorAdminIDRaw := strings.TrimSpace(c.Query("operator_admin_id"))
	targetAdminIDRaw := strings.TrimSpace(c.Query("target_admin_id"))
	action := strings.TrimSpace(c.Query("action"))
	role := strings.TrimSpace(c.Query("role"))
	object := strings.TrimSpace(c.Query("object"))
	method := strings.TrimSpace(c.Query("method"))
	createdFromRaw := strings.TrimSpace(c.Query("created_from"))
	createdToRaw := strings.TrimSpace(c.Query("created_to"))

	var operatorAdminID uint
	if operatorAdminIDRaw != "" {
		raw, err := strconv.ParseUint(operatorAdminIDRaw, 10, 64)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		operatorAdminID = uint(raw)
	}

	var targetAdminID uint
	if targetAdminIDRaw != "" {
		raw, err := strconv.ParseUint(targetAdminIDRaw, 10, 64)
		if err != nil {
			respondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		targetAdminID = uint(raw)
	}

	createdFrom, err := parseTimeNullable(createdFromRaw)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	createdTo, err := parseTimeNullable(createdToRaw)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	items, total, err := h.AuthzAuditService.ListForAdmin(repository.AuthzAuditLogListFilter{
		Page:            page,
		PageSize:        pageSize,
		OperatorAdminID: operatorAdminID,
		TargetAdminID:   targetAdminID,
		Action:          action,
		Role:            role,
		Object:          object,
		Method:          method,
		CreatedFrom:     createdFrom,
		CreatedTo:       createdTo,
	})
	if err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}

	pagination := response.Pagination{
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		TotalPage: (total + int64(pageSize) - 1) / int64(pageSize),
	}
	response.SuccessWithPage(c, items, pagination)
}
