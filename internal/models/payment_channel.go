package models

import (
	"time"

	"gorm.io/gorm"
)

// PaymentChannel 支付渠道配置
type PaymentChannel struct {
	ID              uint           `gorm:"primarykey" json:"id"`                                 // 主键
	Name            string         `gorm:"not null" json:"name"`                                 // 渠道名称
	ProviderType    string         `gorm:"not null" json:"provider_type"`                        // 提供方类型（official/epay）
	ChannelType     string         `gorm:"not null" json:"channel_type"`                         // 渠道类型（wechat/alipay/qqpay/paypal）
	InteractionMode string         `gorm:"not null" json:"interaction_mode"`                     // 交互方式（qr/redirect）
	FeeRate         Money          `gorm:"type:decimal(6,2);not null;default:0" json:"fee_rate"` // 手续费比例（百分比）
	ConfigJSON      JSON           `gorm:"type:json" json:"config_json"`                         // 渠道配置
	IsActive        bool           `gorm:"not null;default:true" json:"is_active"`               // 是否启用
	SortOrder       int            `gorm:"not null;default:0" json:"sort_order"`                 // 排序
	CreatedAt       time.Time      `gorm:"index" json:"created_at"`                              // 创建时间
	UpdatedAt       time.Time      `gorm:"index" json:"updated_at"`                              // 更新时间
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`                                       // 软删除时间
}

// TableName 指定表名
func (PaymentChannel) TableName() string {
	return "payment_channels"
}
