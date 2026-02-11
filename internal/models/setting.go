package models

// Setting 系统设置表（键值对存储）
type Setting struct {
	Key       string `gorm:"primarykey" json:"key"`  // 配置键
	ValueJSON JSON   `gorm:"type:json" json:"value"` // 配置值
}

// TableName 指定表名
func (Setting) TableName() string {
	return "settings"
}
