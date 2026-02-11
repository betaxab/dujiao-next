package models

import (
	"time"

	"gorm.io/gorm"
)

// CartItem 购物车项
type CartItem struct {
	ID              uint           `gorm:"primarykey" json:"id"`                                               // 主键
	UserID          uint           `gorm:"not null;uniqueIndex:idx_cart_user_product" json:"user_id"`          // 用户ID
	ProductID       uint           `gorm:"not null;uniqueIndex:idx_cart_user_product" json:"product_id"`       // 商品ID
	Quantity        int            `gorm:"not null" json:"quantity"`                                           // 数量
	FulfillmentType string         `gorm:"type:varchar(20);not null;default:'manual'" json:"fulfillment_type"` // 交付类型
	CreatedAt       time.Time      `gorm:"index" json:"created_at"`                                            // 创建时间
	UpdatedAt       time.Time      `gorm:"index" json:"updated_at"`                                            // 更新时间
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`                                                     // 软删除时间

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"` // 关联商品
}

// TableName 指定表名
func (CartItem) TableName() string {
	return "cart_items"
}
