package models

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/shopspring/decimal"
)

// Money 统一金额类型（保留 2 位小数）
type Money struct {
	decimal.Decimal
}

// NewMoneyFromDecimal 从 decimal 创建金额
func NewMoneyFromDecimal(amount decimal.Decimal) Money {
	return Money{Decimal: amount.Round(2)}
}

// MarshalJSON 统一输出 2 位小数的字符串
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Decimal.Round(2).StringFixed(2))
}

// UnmarshalJSON 解析金额（字符串或数字）
func (m *Money) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		d, err := decimal.NewFromString(s)
		if err != nil {
			return err
		}
		m.Decimal = d.Round(2)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	m.Decimal = decimal.NewFromFloat(f).Round(2)
	return nil
}

// Value 用于数据库写入
func (m Money) Value() (driver.Value, error) {
	return m.Decimal.Round(2).Value()
}

// Scan 用于数据库读取
func (m *Money) Scan(value interface{}) error {
	if err := m.Decimal.Scan(value); err != nil {
		return err
	}
	m.Decimal = m.Decimal.Round(2)
	return nil
}

// String 返回 2 位小数格式
func (m Money) String() string {
	return m.Decimal.Round(2).StringFixed(2)
}
