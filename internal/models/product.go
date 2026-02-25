package models

import (
	"time"

	"gorm.io/gorm"
)

// Product 商品表
type Product struct {
	ID                   uint           `gorm:"primarykey" json:"id"`                                               // 主键
	CategoryID           uint           `gorm:"not null;index" json:"category_id"`                                  // 分类ID
	Slug                 string         `gorm:"uniqueIndex;not null" json:"slug"`                                   // 唯一标识
	SeoMetaJSON          JSON           `gorm:"type:json" json:"seo_meta"`                                          // SEO 元数据
	TitleJSON            JSON           `gorm:"type:json;not null" json:"title"`                                    // 多语言标题
	DescriptionJSON      JSON           `gorm:"type:json" json:"description"`                                       // 多语言描述
	ContentJSON          JSON           `gorm:"type:json" json:"content"`                                           // 多语言详情（Markdown）
	PriceAmount          Money          `gorm:"type:decimal(20,2);not null;default:0" json:"price_amount"`          // 价格金额
	Images               StringArray    `gorm:"type:json" json:"images"`                                            // 图片数组
	Tags                 StringArray    `gorm:"type:json" json:"tags"`                                              // 标签数组
	PurchaseType         string         `gorm:"type:varchar(20);not null;default:'member'" json:"purchase_type"`    // 购买身份（guest/member）
	FulfillmentType      string         `gorm:"type:varchar(20);not null;default:'manual'" json:"fulfillment_type"` // 交付类型（auto/manual）
	ManualFormSchemaJSON JSON           `gorm:"type:json" json:"manual_form_schema"`                                // 人工交付表单 schema
	ManualStockTotal     int            `gorm:"not null;default:0" json:"manual_stock_total"`                       // 手动库存总量（0 表示不启用手动库存控制）
	ManualStockLocked    int            `gorm:"not null;default:0" json:"manual_stock_locked"`                      // 手动库存占用量（待支付）
	ManualStockSold      int            `gorm:"not null;default:0" json:"manual_stock_sold"`                        // 手动库存已售量（支付成功后累加）
	AutoStockAvailable   int64          `gorm:"-" json:"auto_stock_available"`                                      // 自动发货库存可用量（仅结构，不写入数据库）
	AutoStockTotal       int64          `gorm:"-" json:"auto_stock_total"`                                          // 自动发货库存总量（仅结构，不写入数据库）
	AutoStockLocked      int64          `gorm:"-" json:"auto_stock_locked"`                                         // 自动发货库存占用量（仅结构，不写入数据库）
	AutoStockSold        int64          `gorm:"-" json:"auto_stock_sold"`                                           // 自动发货库存已售量（仅结构，不写入数据库）
	IsActive             bool           `gorm:"default:true;index" json:"is_active"`                                // 是否上架
	SortOrder            int            `gorm:"default:0;index" json:"sort_order"`                                  // 排序权重
	CreatedAt            time.Time      `gorm:"index" json:"created_at"`                                            // 创建时间
	UpdatedAt            time.Time      `json:"updated_at"`                                                         // 更新时间
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`                                                     // 软删除时间

	// 关联
	Category Category     `gorm:"foreignKey:CategoryID" json:"category,omitempty"` // 分类信息
	SKUs     []ProductSKU `gorm:"foreignKey:ProductID" json:"skus,omitempty"`      // SKU 列表
}

// TableName 指定表名
func (Product) TableName() string {
	return "products"
}
