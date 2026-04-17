package models

import (
	"time"

	"gorm.io/gorm"
)

// CardSecretBatch 卡密批次表
type CardSecretBatch struct {
	ID         uint           `gorm:"primarykey" json:"id"`                                 // 主键
	ProductID  uint           `gorm:"index;not null" json:"product_id"`                     // 商品ID
	SKUID      uint           `gorm:"column:sku_id;index;not null;default:0" json:"sku_id"` // SKU ID
	BatchNo    string         `gorm:"uniqueIndex;not null" json:"batch_no"`                 // 批次号
	Source     string         `gorm:"not null" json:"source"`                               // 来源（manual/csv）
	TotalCount int            `gorm:"not null" json:"total_count"`                          // 总数量
	Remark     string         `gorm:"type:text" json:"remark"`                              // 批次说明（用户可见）
	Note       string         `gorm:"type:text" json:"note"`                                // 备注（管理员可见）
	CreatedBy  *uint          `gorm:"index" json:"created_by,omitempty"`                    // 创建管理员ID
	CreatedAt  time.Time      `gorm:"index" json:"created_at"`                              // 创建时间
	UpdatedAt  time.Time      `gorm:"index" json:"updated_at"`                              // 更新时间
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`                                       // 软删除时间
}

// TableName 指定表名
func (CardSecretBatch) TableName() string {
	return "card_secret_batches"
}
