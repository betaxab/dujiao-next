package models

import (
	"time"

	"gorm.io/gorm"
)

// Order 订单表
type Order struct {
	ID                      uint           `gorm:"primarykey" json:"id"`                                                   // 主键
	OrderNo                 string         `gorm:"uniqueIndex;not null" json:"order_no"`                                   // 订单编号
	ParentID                *uint          `gorm:"index" json:"parent_id,omitempty"`                                       // 父订单ID
	UserID                  uint           `gorm:"index;not null" json:"user_id,omitempty"`                                // 用户ID（游客订单为 0）
	GuestEmail              string         `gorm:"index" json:"guest_email,omitempty"`                                     // 游客邮箱
	GuestPassword           string         `gorm:"type:varchar(200)" json:"-"`                                             // 游客订单密码
	GuestLocale             string         `gorm:"type:varchar(20)" json:"guest_locale,omitempty"`                         // 游客语言
	Status                  string         `gorm:"index;not null" json:"status"`                                           // 订单状态
	Currency                string         `gorm:"not null" json:"currency"`                                               // 币种
	OriginalAmount          Money          `gorm:"type:decimal(20,2);not null;default:0" json:"original_amount"`           // 原始金额
	DiscountAmount          Money          `gorm:"type:decimal(20,2);not null;default:0" json:"discount_amount"`           // 优惠金额
	PromotionDiscountAmount Money          `gorm:"type:decimal(20,2);not null;default:0" json:"promotion_discount_amount"` // 活动价优惠金额
	TotalAmount             Money          `gorm:"type:decimal(20,2);not null;default:0" json:"total_amount"`              // 实付金额
	CouponID                *uint          `gorm:"index" json:"coupon_id,omitempty"`                                       // 优惠券ID
	PromotionID             *uint          `gorm:"index" json:"promotion_id,omitempty"`                                    // 活动价ID（单品订单）
	ClientIP                string         `gorm:"type:varchar(64)" json:"client_ip,omitempty"`                            // 下单客户端IP
	ExpiresAt               *time.Time     `gorm:"index" json:"expires_at"`                                                // 过期时间
	PaidAt                  *time.Time     `gorm:"index" json:"paid_at"`                                                   // 支付时间
	CanceledAt              *time.Time     `gorm:"index" json:"canceled_at"`                                               // 取消时间
	CreatedAt               time.Time      `gorm:"index" json:"created_at"`                                                // 创建时间
	UpdatedAt               time.Time      `gorm:"index" json:"updated_at"`                                                // 更新时间
	DeletedAt               gorm.DeletedAt `gorm:"index" json:"-"`                                                         // 软删除时间

	Items []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"` // 订单项
	// 关联
	Fulfillment *Fulfillment `gorm:"foreignKey:OrderID" json:"fulfillment,omitempty"` // 交付记录
	Children    []Order      `gorm:"foreignKey:ParentID" json:"children,omitempty"`   // 子订单
}

// TableName 指定表名
func (Order) TableName() string {
	return "orders"
}
