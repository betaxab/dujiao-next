package models

import (
	"time"

	"gorm.io/gorm"
)

// OrderItem 订单项表
type OrderItem struct {
	ID                           uint           `gorm:"primarykey" json:"id"`                                                   // 主键
	OrderID                      uint           `gorm:"index;not null" json:"order_id"`                                         // 订单ID
	ProductID                    uint           `gorm:"index;not null" json:"product_id"`                                       // 商品ID
	TitleJSON                    JSON           `gorm:"type:json;not null" json:"title"`                                        // 商品标题快照
	Tags                         StringArray    `gorm:"type:json" json:"tags"`                                                  // 标签快照
	UnitPrice                    Money          `gorm:"type:decimal(20,2);not null;default:0" json:"unit_price"`                // 单价
	Quantity                     int            `gorm:"not null" json:"quantity"`                                               // 数量
	TotalPrice                   Money          `gorm:"type:decimal(20,2);not null;default:0" json:"total_price"`               // 小计
	CouponDiscount               Money          `gorm:"type:decimal(20,2);not null;default:0" json:"coupon_discount_amount"`    // 优惠券分摊金额
	PromotionDiscount            Money          `gorm:"type:decimal(20,2);not null;default:0" json:"promotion_discount_amount"` // 活动价分摊金额
	PromotionID                  *uint          `gorm:"index" json:"promotion_id,omitempty"`                                    // 活动价ID
	PromotionName                string         `gorm:"-" json:"promotion_name,omitempty"`                                      // 活动价名称
	FulfillmentType              string         `gorm:"not null" json:"fulfillment_type"`                                       // 交付类型
	ManualFormSchemaSnapshotJSON JSON           `gorm:"type:json" json:"manual_form_schema_snapshot"`                           // 人工交付表单 schema 快照
	ManualFormSubmissionJSON     JSON           `gorm:"type:json" json:"manual_form_submission"`                                // 人工交付表单提交值
	CreatedAt                    time.Time      `gorm:"index" json:"created_at"`                                                // 创建时间
	UpdatedAt                    time.Time      `gorm:"index" json:"updated_at"`                                                // 更新时间
	DeletedAt                    gorm.DeletedAt `gorm:"index" json:"-"`                                                         // 软删除时间
}

// TableName 指定表名
func (OrderItem) TableName() string {
	return "order_items"
}
