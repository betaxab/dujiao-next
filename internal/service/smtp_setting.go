package service

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
)

// SMTPVerifyCodeSetting SMTP 验证码相关配置
// 说明：该结构用于后台 SMTP 设置的 verify_code 子配置
// 字段范围在 NormalizeSMTPSetting 与 ValidateSMTPSetting 中统一约束
// 便于后续在设置服务中复用与序列化
// 注意：json tag 与前端字段保持一致
// 设计为值类型，避免 nil 判空分支
// 默认值由 SMTPDefaultSetting 提供
// 与 config.VerifyCodeConfig 一一对应
// 但用于 settings 存储时保持独立
// 避免直接耦合配置加载层
// 同时便于测试
// 后续若扩展可直接追加字段
// 当前仅覆盖现有验证码逻辑所需字段
// 保持最小必要集
// 并向后兼容
// 由服务层统一转换到 config.EmailConfig
// 禁止在 handler 中直接拼装
// 防止字段漂移
// 提高维护性
// 此注释遵循项目中文注释约定
// 结束
//
// (该结构仅用于配置映射)
//
//nolint:govet
type SMTPVerifyCodeSetting struct {
	ExpireMinutes       int `json:"expire_minutes"`
	SendIntervalSeconds int `json:"send_interval_seconds"`
	MaxAttempts         int `json:"max_attempts"`
	Length              int `json:"length"`
}

// SMTPSetting SMTP 配置实体
type SMTPSetting struct {
	Enabled    bool                  `json:"enabled"`
	Host       string                `json:"host"`
	Port       int                   `json:"port"`
	Username   string                `json:"username"`
	Password   string                `json:"password"`
	From       string                `json:"from"`
	FromName   string                `json:"from_name"`
	UseTLS     bool                  `json:"use_tls"`
	UseSSL     bool                  `json:"use_ssl"`
	VerifyCode SMTPVerifyCodeSetting `json:"verify_code"`
}

// SMTPVerifyCodePatch SMTP 验证码配置补丁
type SMTPVerifyCodePatch struct {
	ExpireMinutes       *int `json:"expire_minutes"`
	SendIntervalSeconds *int `json:"send_interval_seconds"`
	MaxAttempts         *int `json:"max_attempts"`
	Length              *int `json:"length"`
}

// SMTPSettingPatch SMTP 配置补丁（支持部分更新）
type SMTPSettingPatch struct {
	Enabled    *bool                `json:"enabled"`
	Host       *string              `json:"host"`
	Port       *int                 `json:"port"`
	Username   *string              `json:"username"`
	Password   *string              `json:"password"`
	From       *string              `json:"from"`
	FromName   *string              `json:"from_name"`
	UseTLS     *bool                `json:"use_tls"`
	UseSSL     *bool                `json:"use_ssl"`
	VerifyCode *SMTPVerifyCodePatch `json:"verify_code"`
}

// SMTPDefaultSetting 根据静态配置生成默认 SMTP 设置
func SMTPDefaultSetting(cfg config.EmailConfig) SMTPSetting {
	setting := SMTPSetting{
		Enabled:  cfg.Enabled,
		Host:     strings.TrimSpace(cfg.Host),
		Port:     cfg.Port,
		Username: strings.TrimSpace(cfg.Username),
		Password: strings.TrimSpace(cfg.Password),
		From:     strings.TrimSpace(cfg.From),
		FromName: strings.TrimSpace(cfg.FromName),
		UseTLS:   cfg.UseTLS,
		UseSSL:   cfg.UseSSL,
		VerifyCode: SMTPVerifyCodeSetting{
			ExpireMinutes:       cfg.VerifyCode.ExpireMinutes,
			SendIntervalSeconds: cfg.VerifyCode.SendIntervalSeconds,
			MaxAttempts:         cfg.VerifyCode.MaxAttempts,
			Length:              cfg.VerifyCode.Length,
		},
	}
	return NormalizeSMTPSetting(setting)
}

// NormalizeSMTPSetting 归一化 SMTP 配置并补齐默认值
func NormalizeSMTPSetting(setting SMTPSetting) SMTPSetting {
	setting.Host = strings.TrimSpace(setting.Host)
	setting.Username = strings.TrimSpace(setting.Username)
	setting.Password = strings.TrimSpace(setting.Password)
	setting.From = strings.TrimSpace(setting.From)
	setting.FromName = strings.TrimSpace(setting.FromName)

	if setting.Port <= 0 || setting.Port > 65535 {
		setting.Port = 587
	}

	if setting.VerifyCode.ExpireMinutes <= 0 {
		setting.VerifyCode.ExpireMinutes = 10
	}
	if setting.VerifyCode.SendIntervalSeconds <= 0 {
		setting.VerifyCode.SendIntervalSeconds = 60
	}
	if setting.VerifyCode.MaxAttempts <= 0 {
		setting.VerifyCode.MaxAttempts = 5
	}
	if setting.VerifyCode.Length < 4 || setting.VerifyCode.Length > 10 {
		setting.VerifyCode.Length = 6
	}

	return setting
}

// ValidateSMTPSetting 校验 SMTP 配置合法性
func ValidateSMTPSetting(setting SMTPSetting) error {
	if setting.Port <= 0 || setting.Port > 65535 {
		return fmt.Errorf("%w: SMTP 端口必须在 1-65535", ErrSMTPConfigInvalid)
	}
	if setting.UseTLS && setting.UseSSL {
		return fmt.Errorf("%w: TLS 与 SSL 不能同时开启", ErrSMTPConfigInvalid)
	}
	if setting.VerifyCode.Length < 4 || setting.VerifyCode.Length > 10 {
		return fmt.Errorf("%w: 验证码长度需在 4-10 之间", ErrSMTPConfigInvalid)
	}
	if setting.VerifyCode.ExpireMinutes <= 0 {
		return fmt.Errorf("%w: 验证码过期时间必须大于 0", ErrSMTPConfigInvalid)
	}
	if setting.VerifyCode.SendIntervalSeconds <= 0 {
		return fmt.Errorf("%w: 验证码发送间隔必须大于 0", ErrSMTPConfigInvalid)
	}
	if setting.VerifyCode.MaxAttempts <= 0 {
		return fmt.Errorf("%w: 验证码尝试次数必须大于 0", ErrSMTPConfigInvalid)
	}
	if !setting.Enabled {
		return nil
	}
	if strings.TrimSpace(setting.Host) == "" {
		return fmt.Errorf("%w: SMTP 主机不能为空", ErrSMTPConfigInvalid)
	}
	if strings.TrimSpace(setting.From) == "" {
		return fmt.Errorf("%w: 发件人邮箱不能为空", ErrSMTPConfigInvalid)
	}
	if _, err := mail.ParseAddress(setting.From); err != nil {
		return fmt.Errorf("%w: 发件人邮箱格式无效", ErrSMTPConfigInvalid)
	}
	return nil
}

// SMTPSettingToConfig 将 SMTP 设置转换为运行时配置
func SMTPSettingToConfig(setting SMTPSetting) config.EmailConfig {
	normalized := NormalizeSMTPSetting(setting)
	return config.EmailConfig{
		Enabled:  normalized.Enabled,
		Host:     normalized.Host,
		Port:     normalized.Port,
		Username: normalized.Username,
		Password: normalized.Password,
		From:     normalized.From,
		FromName: normalized.FromName,
		UseTLS:   normalized.UseTLS,
		UseSSL:   normalized.UseSSL,
		VerifyCode: config.VerifyCodeConfig{
			ExpireMinutes:       normalized.VerifyCode.ExpireMinutes,
			SendIntervalSeconds: normalized.VerifyCode.SendIntervalSeconds,
			MaxAttempts:         normalized.VerifyCode.MaxAttempts,
			Length:              normalized.VerifyCode.Length,
		},
	}
}

// SMTPSettingToMap 将 SMTP 设置转换为 settings 表结构
func SMTPSettingToMap(setting SMTPSetting) map[string]interface{} {
	normalized := NormalizeSMTPSetting(setting)
	return map[string]interface{}{
		"enabled":   normalized.Enabled,
		"host":      normalized.Host,
		"port":      normalized.Port,
		"username":  normalized.Username,
		"password":  normalized.Password,
		"from":      normalized.From,
		"from_name": normalized.FromName,
		"use_tls":   normalized.UseTLS,
		"use_ssl":   normalized.UseSSL,
		"verify_code": map[string]interface{}{
			"expire_minutes":        normalized.VerifyCode.ExpireMinutes,
			"send_interval_seconds": normalized.VerifyCode.SendIntervalSeconds,
			"max_attempts":          normalized.VerifyCode.MaxAttempts,
			"length":                normalized.VerifyCode.Length,
		},
	}
}

// MaskSMTPSettingForAdmin 返回脱敏后的 SMTP 设置
func MaskSMTPSettingForAdmin(setting SMTPSetting) models.JSON {
	normalized := NormalizeSMTPSetting(setting)
	return models.JSON{
		"enabled":      normalized.Enabled,
		"host":         normalized.Host,
		"port":         normalized.Port,
		"username":     normalized.Username,
		"password":     "",
		"has_password": normalized.Password != "",
		"from":         normalized.From,
		"from_name":    normalized.FromName,
		"use_tls":      normalized.UseTLS,
		"use_ssl":      normalized.UseSSL,
		"verify_code": map[string]interface{}{
			"expire_minutes":        normalized.VerifyCode.ExpireMinutes,
			"send_interval_seconds": normalized.VerifyCode.SendIntervalSeconds,
			"max_attempts":          normalized.VerifyCode.MaxAttempts,
			"length":                normalized.VerifyCode.Length,
		},
	}
}

// GetSMTPSetting 获取 SMTP 设置（优先 settings，空时回退默认配置）
func (s *SettingService) GetSMTPSetting(defaultCfg config.EmailConfig) (SMTPSetting, error) {
	fallback := SMTPDefaultSetting(defaultCfg)
	value, err := s.GetByKey(constants.SettingKeySMTPConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	parsed := smtpSettingFromJSON(value, fallback)
	return NormalizeSMTPSetting(parsed), nil
}

// PatchSMTPSetting 基于补丁更新 SMTP 设置
func (s *SettingService) PatchSMTPSetting(defaultCfg config.EmailConfig, patch SMTPSettingPatch) (SMTPSetting, error) {
	current, err := s.GetSMTPSetting(defaultCfg)
	if err != nil {
		return SMTPSetting{}, err
	}

	next := current
	if patch.Enabled != nil {
		next.Enabled = *patch.Enabled
	}
	if patch.Host != nil {
		next.Host = strings.TrimSpace(*patch.Host)
	}
	if patch.Port != nil {
		next.Port = *patch.Port
	}
	if patch.Username != nil {
		next.Username = strings.TrimSpace(*patch.Username)
	}
	if patch.Password != nil {
		password := strings.TrimSpace(*patch.Password)
		if password != "" {
			next.Password = password
		}
	}
	if patch.From != nil {
		next.From = strings.TrimSpace(*patch.From)
	}
	if patch.FromName != nil {
		next.FromName = strings.TrimSpace(*patch.FromName)
	}
	if patch.UseTLS != nil {
		next.UseTLS = *patch.UseTLS
	}
	if patch.UseSSL != nil {
		next.UseSSL = *patch.UseSSL
	}
	if patch.VerifyCode != nil {
		if patch.VerifyCode.ExpireMinutes != nil {
			next.VerifyCode.ExpireMinutes = *patch.VerifyCode.ExpireMinutes
		}
		if patch.VerifyCode.SendIntervalSeconds != nil {
			next.VerifyCode.SendIntervalSeconds = *patch.VerifyCode.SendIntervalSeconds
		}
		if patch.VerifyCode.MaxAttempts != nil {
			next.VerifyCode.MaxAttempts = *patch.VerifyCode.MaxAttempts
		}
		if patch.VerifyCode.Length != nil {
			next.VerifyCode.Length = *patch.VerifyCode.Length
		}
	}

	normalized := NormalizeSMTPSetting(next)
	if err := ValidateSMTPSetting(normalized); err != nil {
		return SMTPSetting{}, err
	}

	if _, err := s.Update(constants.SettingKeySMTPConfig, SMTPSettingToMap(normalized)); err != nil {
		return SMTPSetting{}, err
	}
	return normalized, nil
}

func smtpSettingFromJSON(raw models.JSON, fallback SMTPSetting) SMTPSetting {
	next := fallback
	if raw == nil {
		return next
	}

	next.Enabled = readBool(raw, "enabled", next.Enabled)
	next.Host = readString(raw, "host", next.Host)
	next.Port = readInt(raw, "port", next.Port)
	next.Username = readString(raw, "username", next.Username)
	next.Password = readString(raw, "password", next.Password)
	next.From = readString(raw, "from", next.From)
	next.FromName = readString(raw, "from_name", next.FromName)
	next.UseTLS = readBool(raw, "use_tls", next.UseTLS)
	next.UseSSL = readBool(raw, "use_ssl", next.UseSSL)

	verifyRaw, ok := raw["verify_code"]
	if ok {
		if verifyMap := toStringAnyMap(verifyRaw); verifyMap != nil {
			next.VerifyCode.ExpireMinutes = readInt(verifyMap, "expire_minutes", next.VerifyCode.ExpireMinutes)
			next.VerifyCode.SendIntervalSeconds = readInt(verifyMap, "send_interval_seconds", next.VerifyCode.SendIntervalSeconds)
			next.VerifyCode.MaxAttempts = readInt(verifyMap, "max_attempts", next.VerifyCode.MaxAttempts)
			next.VerifyCode.Length = readInt(verifyMap, "length", next.VerifyCode.Length)
		}
	}

	return next
}

func toStringAnyMap(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return v
	case models.JSON:
		result := make(map[string]interface{}, len(v))
		for key, item := range v {
			result[key] = item
		}
		return result
	default:
		return nil
	}
}

func readString(source map[string]interface{}, key, fallback string) string {
	value, ok := source[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return fallback
	}
}

func readBool(source map[string]interface{}, key string, fallback bool) bool {
	value, ok := source[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			return fallback
		}
	default:
		return fallback
	}
}

func readInt(source map[string]interface{}, key string, fallback int) int {
	value, ok := source[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
		if f, err := v.Float64(); err == nil {
			return int(f)
		}
		return fallback
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}
