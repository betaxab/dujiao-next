package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
)

const (
	affiliateCommissionRateMin       = 0
	affiliateCommissionRateMax       = 100
	affiliateConfirmDaysMin          = 0
	affiliateConfirmDaysMax          = 3650
	affiliateMinWithdrawAmountMin    = 0
	affiliateWithdrawChannelsMaxSize = 20
	affiliateWithdrawChannelMaxRune  = 50
)

// AffiliateSetting 推广返利配置
type AffiliateSetting struct {
	Enabled           bool     `json:"enabled"`
	CommissionRate    float64  `json:"commission_rate"`
	ConfirmDays       int      `json:"confirm_days"`
	MinWithdrawAmount float64  `json:"min_withdraw_amount"`
	WithdrawChannels  []string `json:"withdraw_channels"`
}

// AffiliateDefaultSetting 默认推广返利配置
func AffiliateDefaultSetting() AffiliateSetting {
	return NormalizeAffiliateSetting(AffiliateSetting{
		Enabled:           false,
		CommissionRate:    0,
		ConfirmDays:       0,
		MinWithdrawAmount: 0,
		WithdrawChannels:  []string{},
	})
}

// NormalizeAffiliateSetting 归一化推广返利配置
func NormalizeAffiliateSetting(setting AffiliateSetting) AffiliateSetting {
	setting.CommissionRate = roundAffiliateDecimal(setting.CommissionRate)
	if setting.CommissionRate < affiliateCommissionRateMin {
		setting.CommissionRate = affiliateCommissionRateMin
	}
	if setting.CommissionRate > affiliateCommissionRateMax {
		setting.CommissionRate = affiliateCommissionRateMax
	}

	if setting.ConfirmDays < affiliateConfirmDaysMin {
		setting.ConfirmDays = affiliateConfirmDaysMin
	}
	if setting.ConfirmDays > affiliateConfirmDaysMax {
		setting.ConfirmDays = affiliateConfirmDaysMax
	}

	setting.MinWithdrawAmount = roundAffiliateDecimal(setting.MinWithdrawAmount)
	if setting.MinWithdrawAmount < affiliateMinWithdrawAmountMin {
		setting.MinWithdrawAmount = affiliateMinWithdrawAmountMin
	}

	setting.WithdrawChannels = normalizeAffiliateWithdrawChannels(setting.WithdrawChannels)
	return setting
}

// ValidateAffiliateSetting 校验推广返利配置
func ValidateAffiliateSetting(setting AffiliateSetting) error {
	normalized := NormalizeAffiliateSetting(setting)
	if normalized.CommissionRate < affiliateCommissionRateMin || normalized.CommissionRate > affiliateCommissionRateMax {
		return fmt.Errorf("%w: 返利比例必须在 0-100 之间", ErrAffiliateConfigInvalid)
	}
	if normalized.ConfirmDays < affiliateConfirmDaysMin || normalized.ConfirmDays > affiliateConfirmDaysMax {
		return fmt.Errorf("%w: 佣金确认天数必须在 0-3650 之间", ErrAffiliateConfigInvalid)
	}
	if normalized.MinWithdrawAmount < affiliateMinWithdrawAmountMin {
		return fmt.Errorf("%w: 最低提现金额不能小于 0", ErrAffiliateConfigInvalid)
	}
	return nil
}

// AffiliateSettingToMap 将推广返利配置转换为 settings 存储结构
func AffiliateSettingToMap(setting AffiliateSetting) map[string]interface{} {
	normalized := NormalizeAffiliateSetting(setting)
	return map[string]interface{}{
		"enabled":             normalized.Enabled,
		"commission_rate":     normalized.CommissionRate,
		"confirm_days":        normalized.ConfirmDays,
		"min_withdraw_amount": normalized.MinWithdrawAmount,
		"withdraw_channels":   cloneStringSlice(normalized.WithdrawChannels),
	}
}

func affiliateSettingFromJSON(raw models.JSON, fallback AffiliateSetting) AffiliateSetting {
	result := fallback

	if enabledRaw, ok := raw["enabled"]; ok {
		result.Enabled = parseSettingBool(enabledRaw)
	}
	if rateRaw, ok := raw["commission_rate"]; ok {
		if parsed, err := parseSettingFloat(rateRaw); err == nil {
			result.CommissionRate = parsed
		}
	}
	if confirmDaysRaw, ok := raw["confirm_days"]; ok {
		if parsed, err := parseSettingInt(confirmDaysRaw); err == nil {
			result.ConfirmDays = parsed
		}
	}
	if minWithdrawRaw, ok := raw["min_withdraw_amount"]; ok {
		if parsed, err := parseSettingFloat(minWithdrawRaw); err == nil {
			result.MinWithdrawAmount = parsed
		}
	}
	if channelsRaw, ok := raw["withdraw_channels"]; ok {
		result.WithdrawChannels = normalizeSettingStringList(channelsRaw)
	}

	return NormalizeAffiliateSetting(result)
}

func normalizeAffiliateSettingMap(value map[string]interface{}) models.JSON {
	setting := affiliateSettingFromJSON(models.JSON(value), AffiliateDefaultSetting())
	return models.JSON(AffiliateSettingToMap(setting))
}

// GetAffiliateSetting 获取推广返利设置（优先 settings，空时回退默认）
func (s *SettingService) GetAffiliateSetting() (AffiliateSetting, error) {
	fallback := AffiliateDefaultSetting()
	if s == nil {
		return fallback, nil
	}

	value, err := s.GetByKey(constants.SettingKeyAffiliateConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return affiliateSettingFromJSON(value, fallback), nil
}

// UpdateAffiliateSetting 更新推广返利设置
func (s *SettingService) UpdateAffiliateSetting(setting AffiliateSetting) (AffiliateSetting, error) {
	normalized := NormalizeAffiliateSetting(setting)
	if err := ValidateAffiliateSetting(normalized); err != nil {
		return AffiliateDefaultSetting(), err
	}
	if _, err := s.Update(constants.SettingKeyAffiliateConfig, AffiliateSettingToMap(normalized)); err != nil {
		return AffiliateDefaultSetting(), err
	}
	return normalized, nil
}

func parseSettingFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("empty string")
		}
		return strconv.ParseFloat(trimmed, 64)
	default:
		return 0, fmt.Errorf("unsupported value type")
	}
}

func roundAffiliateDecimal(value float64) float64 {
	return math.Round(value*100) / 100
}

func normalizeAffiliateWithdrawChannels(channels []string) []string {
	if len(channels) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(channels))
	seen := make(map[string]struct{}, len(channels))
	for _, raw := range channels {
		value := normalizeSettingTextWithRuneLimit(raw, affiliateWithdrawChannelMaxRune)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) >= affiliateWithdrawChannelsMaxSize {
			break
		}
	}
	return result
}

func normalizeSettingStringList(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		items := make([]string, 0, len(value))
		for _, item := range value {
			items = append(items, normalizeSettingText(item))
		}
		return items
	default:
		return nil
	}
}
