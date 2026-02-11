package admin

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

type authzRolePayload struct {
	Role string `json:"role" binding:"required"`
}

type authzPolicyPayload struct {
	Role   string `json:"role" binding:"required"`
	Object string `json:"object" binding:"required"`
	Action string `json:"action" binding:"required"`
}

type authzSetAdminRolesPayload struct {
	Roles []string `json:"roles"`
}

// GetAuthzMe 获取当前管理员权限快照
func (h *Handler) GetAuthzMe(c *gin.Context) {
	adminID, ok := getAdminID(c)
	if !ok {
		return
	}

	roles, err := h.AuthzService.GetAdminRoles(adminID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}
	policies, err := h.AuthzService.GetAdminPolicies(adminID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}

	isSuper := false
	if value, exists := c.Get("admin_is_super"); exists {
		if flag, typeOK := value.(bool); typeOK {
			isSuper = flag
		}
	}

	response.Success(c, gin.H{
		"admin_id": adminID,
		"is_super": isSuper,
		"roles":    roles,
		"policies": policies,
	})
}

// ListAuthzRoles 获取角色列表
func (h *Handler) ListAuthzRoles(c *gin.Context) {
	roles, err := h.AuthzService.ListRoles()
	if err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}
	response.Success(c, roles)
}

// ListAuthzAdmins 获取管理员列表
func (h *Handler) ListAuthzAdmins(c *gin.Context) {
	admins, err := h.AdminRepo.List()
	if err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}

	items := make([]gin.H, 0, len(admins))
	for _, admin := range admins {
		roles, roleErr := h.AuthzService.GetAdminRoles(admin.ID)
		if roleErr != nil {
			respondError(c, response.CodeInternal, "error.config_fetch_failed", roleErr)
			return
		}
		items = append(items, gin.H{
			"id":            admin.ID,
			"username":      admin.Username,
			"is_super":      admin.IsSuper,
			"last_login_at": admin.LastLoginAt,
			"created_at":    admin.CreatedAt,
			"roles":         roles,
		})
	}

	response.Success(c, items)
}

// CreateAuthzRole 创建角色
func (h *Handler) CreateAuthzRole(c *gin.Context) {
	var req authzRolePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	role, err := h.AuthzService.EnsureRole(req.Role)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	h.recordAuthzAudit(c, service.AuthzAuditRecordInput{
		OperatorAdminID:  currentAdminID(c),
		OperatorUsername: currentUsername(c),
		Action:           "role_create",
		Role:             role,
		RequestID:        currentRequestID(c),
		Detail: models.JSON{
			"role": role,
		},
	})

	logger.Infow("admin_authz_role_created",
		"operator_admin_id", currentAdminID(c),
		"role", role,
	)

	response.Success(c, gin.H{"role": role})
}

// DeleteAuthzRole 删除角色
func (h *Handler) DeleteAuthzRole(c *gin.Context) {
	role := decodeRoleParam(c.Param("role"))
	if strings.TrimSpace(role) == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	if err := h.AuthzService.DeleteRole(role); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	h.recordAuthzAudit(c, service.AuthzAuditRecordInput{
		OperatorAdminID:  currentAdminID(c),
		OperatorUsername: currentUsername(c),
		Action:           "role_delete",
		Role:             role,
		RequestID:        currentRequestID(c),
		Detail: models.JSON{
			"role": role,
		},
	})

	logger.Infow("admin_authz_role_deleted",
		"operator_admin_id", currentAdminID(c),
		"role", role,
	)

	response.Success(c, nil)
}

// GetAuthzRolePolicies 获取角色策略
func (h *Handler) GetAuthzRolePolicies(c *gin.Context) {
	role := decodeRoleParam(c.Param("role"))
	if strings.TrimSpace(role) == "" {
		respondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	policies, err := h.AuthzService.GetRolePolicies(role)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	response.Success(c, policies)
}

// GrantAuthzPolicy 授予角色策略
func (h *Handler) GrantAuthzPolicy(c *gin.Context) {
	var req authzPolicyPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if err := h.AuthzService.GrantRolePolicy(req.Role, req.Object, req.Action); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	h.recordAuthzAudit(c, service.AuthzAuditRecordInput{
		OperatorAdminID:  currentAdminID(c),
		OperatorUsername: currentUsername(c),
		Action:           "policy_grant",
		Role:             req.Role,
		Object:           req.Object,
		Method:           req.Action,
		RequestID:        currentRequestID(c),
		Detail: models.JSON{
			"role":   req.Role,
			"object": req.Object,
			"method": strings.ToUpper(strings.TrimSpace(req.Action)),
		},
	})

	logger.Infow("admin_authz_policy_granted",
		"operator_admin_id", currentAdminID(c),
		"role", req.Role,
		"object", req.Object,
		"action", req.Action,
	)

	response.Success(c, nil)
}

// RevokeAuthzPolicy 撤销角色策略
func (h *Handler) RevokeAuthzPolicy(c *gin.Context) {
	var req authzPolicyPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if err := h.AuthzService.RevokeRolePolicy(req.Role, req.Object, req.Action); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	h.recordAuthzAudit(c, service.AuthzAuditRecordInput{
		OperatorAdminID:  currentAdminID(c),
		OperatorUsername: currentUsername(c),
		Action:           "policy_revoke",
		Role:             req.Role,
		Object:           req.Object,
		Method:           req.Action,
		RequestID:        currentRequestID(c),
		Detail: models.JSON{
			"role":   req.Role,
			"object": req.Object,
			"method": strings.ToUpper(strings.TrimSpace(req.Action)),
		},
	})

	logger.Infow("admin_authz_policy_revoked",
		"operator_admin_id", currentAdminID(c),
		"role", req.Role,
		"object", req.Object,
		"action", req.Action,
	)

	response.Success(c, nil)
}

// GetAuthzAdminRoles 获取管理员角色
func (h *Handler) GetAuthzAdminRoles(c *gin.Context) {
	adminID, ok := parseAdminIDParam(c)
	if !ok {
		return
	}
	if _, err := h.AdminRepo.GetByID(adminID); err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}

	roles, err := h.AuthzService.GetAdminRoles(adminID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}
	response.Success(c, roles)
}

// SetAuthzAdminRoles 设置管理员角色
func (h *Handler) SetAuthzAdminRoles(c *gin.Context) {
	adminID, ok := parseAdminIDParam(c)
	if !ok {
		return
	}
	admin, err := h.AdminRepo.GetByID(adminID)
	if err != nil {
		respondError(c, response.CodeInternal, "error.save_failed", err)
		return
	}
	if admin == nil {
		respondError(c, response.CodeBadRequest, "error.admin_id_invalid", nil)
		return
	}

	var req authzSetAdminRolesPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if err := h.AuthzService.SetAdminRoles(adminID, req.Roles); err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	h.recordAuthzAudit(c, service.AuthzAuditRecordInput{
		OperatorAdminID:  currentAdminID(c),
		OperatorUsername: currentUsername(c),
		TargetAdminID:    &adminID,
		TargetUsername:   admin.Username,
		Action:           "admin_roles_update",
		RequestID:        currentRequestID(c),
		Detail: models.JSON{
			"target_admin_id": adminID,
			"target_username": admin.Username,
			"roles":           req.Roles,
		},
	})

	logger.Infow("admin_authz_admin_roles_updated",
		"operator_admin_id", currentAdminID(c),
		"target_admin_id", adminID,
		"roles", req.Roles,
	)

	response.Success(c, nil)
}

func (h *Handler) recordAuthzAudit(c *gin.Context, input service.AuthzAuditRecordInput) {
	if h == nil || h.AuthzAuditService == nil {
		return
	}
	if input.OperatorAdminID == 0 || strings.TrimSpace(input.Action) == "" {
		return
	}
	if err := h.AuthzAuditService.Record(input); err != nil {
		logger.Warnw("admin_authz_audit_record_failed",
			"error", err,
			"action", input.Action,
			"operator_admin_id", input.OperatorAdminID,
		)
	}
}

func parseAdminIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		respondError(c, response.CodeBadRequest, "error.admin_id_invalid", nil)
		return 0, false
	}
	return uint(id), true
}

func decodeRoleParam(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(decoded)
}

func currentAdminID(c *gin.Context) uint {
	value, exists := c.Get("admin_id")
	if !exists {
		return 0
	}
	switch adminID := value.(type) {
	case uint:
		return adminID
	case int:
		if adminID > 0 {
			return uint(adminID)
		}
	case float64:
		if adminID > 0 {
			return uint(adminID)
		}
	}
	return 0
}

func currentUsername(c *gin.Context) string {
	value, exists := c.Get("username")
	if !exists {
		return ""
	}
	if username, ok := value.(string); ok {
		return strings.TrimSpace(username)
	}
	return ""
}

func currentRequestID(c *gin.Context) string {
	value, exists := c.Get("request_id")
	if !exists {
		return ""
	}
	if requestID, ok := value.(string); ok {
		return strings.TrimSpace(requestID)
	}
	return ""
}
