package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

// SettingService 设置业务服务
type SettingService struct {
	repo repository.SettingRepository
}

// NewSettingService 创建设置服务
func NewSettingService(repo repository.SettingRepository) *SettingService {
	return &SettingService{repo: repo}
}

// GetConfig 获取站点配置（合并默认值）
func (s *SettingService) GetConfig(defaults map[string]interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	for k, v := range defaults {
		data[k] = v
	}

	setting, err := s.repo.GetByKey(constants.SettingKeySiteConfig)
	if err != nil {
		return nil, err
	}
	if setting == nil {
		return data, nil
	}

	for k, v := range setting.ValueJSON {
		data[k] = v
	}
	return data, nil
}

// GetByKey 获取设置
func (s *SettingService) GetByKey(key string) (models.JSON, error) {
	setting, err := s.repo.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if setting == nil {
		return nil, nil
	}
	return setting.ValueJSON, nil
}

// Update 设置值
func (s *SettingService) Update(key string, value map[string]interface{}) (models.JSON, error) {
	normalized := normalizeSettingValueByKey(key, value)

	setting, err := s.repo.Upsert(key, normalized)
	if err != nil {
		return nil, err
	}
	return setting.ValueJSON, nil
}

// GetOrderPaymentExpireMinutes 获取订单超时分钟配置
func (s *SettingService) GetOrderPaymentExpireMinutes(defaultValue int) (int, error) {
	if s == nil {
		return defaultValue, nil
	}
	value, err := s.GetByKey(constants.SettingKeyOrderConfig)
	if err != nil {
		return defaultValue, err
	}
	if value == nil {
		return defaultValue, nil
	}
	raw, ok := value[constants.SettingFieldPaymentExpireMinutes]
	if !ok {
		return defaultValue, nil
	}
	minutes, err := parseSettingInt(raw)
	if err != nil {
		return defaultValue, err
	}
	if minutes <= 0 {
		return defaultValue, nil
	}
	return minutes, nil
}

func parseSettingInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), nil
		}
		if f, err := v.Float64(); err == nil {
			return int(f), nil
		}
		return 0, fmt.Errorf("invalid json number")
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("empty string")
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported value type")
	}
}
