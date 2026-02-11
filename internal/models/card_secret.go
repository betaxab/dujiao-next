package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	CardSecretStatusAvailable = "available"
	CardSecretStatusReserved  = "reserved"
	CardSecretStatusUsed      = "used"
)

// CardSecret 卡密库存表
type CardSecret struct {
	ID         uint           `gorm:"primarykey" json:"id"`             // 主键
	ProductID  uint           `gorm:"index;not null" json:"product_id"` // 商品ID
	BatchID    *uint          `gorm:"index" json:"batch_id,omitempty"`  // 批次ID
	Secret     string         `gorm:"type:text;not null" json:"secret"` // 卡密内容
	Status     string         `gorm:"index;not null" json:"status"`     // 状态（available/used）
	OrderID    *uint          `gorm:"index" json:"order_id,omitempty"`  // 关联订单ID
	ReservedAt *time.Time     `gorm:"index" json:"reserved_at"`         // 占用时间
	UsedAt     *time.Time     `gorm:"index" json:"used_at"`             // 使用时间
	CreatedAt  time.Time      `gorm:"index" json:"created_at"`          // 创建时间
	UpdatedAt  time.Time      `gorm:"index" json:"updated_at"`          // 更新时间
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`                   // 软删除时间

	Batch *CardSecretBatch `gorm:"foreignKey:BatchID" json:"batch,omitempty"` // 批次信息
}

// TableName 指定表名
func (CardSecret) TableName() string {
	return "card_secrets"
}
