package authz

import "fmt"

// RoleSeed 预置角色定义
type RoleSeed struct {
	Role      string
	Inherits  []string
	Policies  []Policy
	Immutable bool
}

// BuiltinRoleSeeds 系统预置角色矩阵
func BuiltinRoleSeeds() []RoleSeed {
	return []RoleSeed{
		{
			Role: "readonly_auditor",
			Policies: []Policy{
				{Object: "/admin/*", Action: "GET"},
			},
			Immutable: true,
		},
		{
			Role:     "operations",
			Inherits: []string{"readonly_auditor"},
			Policies: []Policy{
				{Object: "/admin/products", Action: "*"},
				{Object: "/admin/products/:id", Action: "*"},
				{Object: "/admin/categories", Action: "*"},
				{Object: "/admin/categories/:id", Action: "*"},
				{Object: "/admin/posts", Action: "*"},
				{Object: "/admin/posts/:id", Action: "*"},
				{Object: "/admin/banners", Action: "*"},
				{Object: "/admin/banners/:id", Action: "*"},
				{Object: "/admin/coupons", Action: "*"},
				{Object: "/admin/coupons/:id", Action: "*"},
				{Object: "/admin/promotions", Action: "*"},
				{Object: "/admin/promotions/:id", Action: "*"},
				{Object: "/admin/card-secrets", Action: "*"},
				{Object: "/admin/card-secrets/:id", Action: "*"},
				{Object: "/admin/card-secrets/stats", Action: "GET"},
				{Object: "/admin/card-secrets/batches", Action: "GET"},
				{Object: "/admin/card-secrets/template", Action: "GET"},
				{Object: "/admin/upload", Action: "POST"},
			},
			Immutable: true,
		},
		{
			Role:     "support",
			Inherits: []string{"readonly_auditor"},
			Policies: []Policy{
				{Object: "/admin/orders", Action: "GET"},
				{Object: "/admin/orders/:id", Action: "GET"},
				{Object: "/admin/orders/:id", Action: "PATCH"},
				{Object: "/admin/fulfillments", Action: "POST"},
				{Object: "/admin/users", Action: "GET"},
				{Object: "/admin/users/:id", Action: "GET"},
				{Object: "/admin/user-login-logs", Action: "GET"},
				{Object: "/admin/payments", Action: "GET"},
				{Object: "/admin/payments/:id", Action: "GET"},
			},
			Immutable: true,
		},
		{
			Role:     "finance",
			Inherits: []string{"readonly_auditor"},
			Policies: []Policy{
				{Object: "/admin/payments", Action: "GET"},
				{Object: "/admin/payments/:id", Action: "GET"},
				{Object: "/admin/payments/export", Action: "GET"},
				{Object: "/admin/payment-channels", Action: "*"},
				{Object: "/admin/payment-channels/:id", Action: "*"},
				{Object: "/admin/orders", Action: "GET"},
				{Object: "/admin/orders/:id", Action: "GET"},
			},
			Immutable: true,
		},
	}
}

// BootstrapBuiltinRoles 初始化预置角色与默认策略
func (s *Service) BootstrapBuiltinRoles() error {
	if s == nil || s.enforcer == nil {
		return fmt.Errorf("authz service unavailable")
	}

	changed := false
	for _, seed := range BuiltinRoleSeeds() {
		role, err := NormalizeRole(seed.Role)
		if err != nil {
			return err
		}

		exists, err := s.enforcer.HasNamedGroupingPolicy("g", role, roleAnchor)
		if err != nil {
			return fmt.Errorf("check builtin role failed: %w", err)
		}
		if !exists {
			added, err := s.enforcer.AddNamedGroupingPolicy("g", role, roleAnchor)
			if err != nil {
				return fmt.Errorf("create builtin role failed: %w", err)
			}
			if added {
				changed = true
			}
		}

		for _, parent := range seed.Inherits {
			parentRole, err := NormalizeRole(parent)
			if err != nil {
				return err
			}
			added, err := s.enforcer.AddNamedGroupingPolicy("g", role, parentRole)
			if err != nil {
				return fmt.Errorf("link role inheritance failed: %w", err)
			}
			if added {
				changed = true
			}
		}

		for _, policy := range seed.Policies {
			action := NormalizeAction(policy.Action)
			if action == "" {
				return fmt.Errorf("builtin policy action is required")
			}
			added, err := s.enforcer.AddPolicy(role, NormalizeObject(policy.Object), action)
			if err != nil {
				return fmt.Errorf("add builtin policy failed: %w", err)
			}
			if added {
				changed = true
			}
		}
	}

	if changed {
		return s.saveAndReload()
	}
	return nil
}
