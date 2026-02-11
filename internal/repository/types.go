package repository

import "time"

// ProductListFilter 查询商品列表的过滤条件
type ProductListFilter struct {
	Page              int
	PageSize          int
	CategoryID        string
	Search            string
	ManualStockStatus string
	OnlyActive        bool
	WithCategory      bool
}

// PostListFilter 查询文章列表的过滤条件
type PostListFilter struct {
	Page          int
	PageSize      int
	Type          string
	Search        string
	OnlyPublished bool
	OrderBy       string
}

// BannerListFilter 查询 Banner 列表的过滤条件
type BannerListFilter struct {
	Page      int
	PageSize  int
	Position  string
	Search    string
	IsActive  *bool
	OrderBy   string
	OnlyValid bool
}

// OrderListFilter 查询订单列表的过滤条件
type OrderListFilter struct {
	Page        int
	PageSize    int
	UserID      uint
	Status      string
	OrderNo     string
	GuestEmail  string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

// PaymentListFilter 查询支付列表的过滤条件
type PaymentListFilter struct {
	Page         int
	PageSize     int
	UserID       uint
	OrderID      uint
	ChannelID    uint
	ProviderType string
	ChannelType  string
	Status       string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

// PaymentChannelListFilter 查询支付渠道列表的过滤条件
type PaymentChannelListFilter struct {
	Page         int
	PageSize     int
	ProviderType string
	ChannelType  string
	ActiveOnly   bool
}

// CouponUsageListFilter 查询优惠券使用记录列表的过滤条件
type CouponUsageListFilter struct {
	Page     int
	PageSize int
	UserID   uint
}

// UserListFilter 查询用户列表的过滤条件
type UserListFilter struct {
	Page          int
	PageSize      int
	Keyword       string
	Status        string
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	LastLoginFrom *time.Time
	LastLoginTo   *time.Time
}

// UserLoginLogListFilter 查询用户登录日志列表的过滤条件
type UserLoginLogListFilter struct {
	Page        int
	PageSize    int
	UserID      uint
	Email       string
	Status      string
	FailReason  string
	ClientIP    string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

// AuthzAuditLogListFilter 查询权限审计日志列表的过滤条件
type AuthzAuditLogListFilter struct {
	Page            int
	PageSize        int
	OperatorAdminID uint
	TargetAdminID   uint
	Action          string
	Role            string
	Object          string
	Method          string
	CreatedFrom     *time.Time
	CreatedTo       *time.Time
}
