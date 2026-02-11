package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"github.com/mojocn/base64Captcha"
)

// CaptchaVerifyPayload 验证码校验请求载荷
type CaptchaVerifyPayload struct {
	CaptchaID      string `json:"captcha_id"`
	CaptchaCode    string `json:"captcha_code"`
	TurnstileToken string `json:"turnstile_token"`
}

// CaptchaImageChallenge 图片验证码挑战
type CaptchaImageChallenge struct {
	CaptchaID   string `json:"captcha_id"`
	ImageBase64 string `json:"image_base64"`
}

type turnstileVerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// CaptchaService 验证码服务
// 负责统一读取配置、生成挑战与执行校验
// 按场景开关决定是否需要验证码
// 对图片验证码与 Turnstile 进行统一封装
// 外部仅需要调用 Verify(scene, payload, clientIP)
// 以及图片模式下调用 GenerateImageChallenge
//
//nolint:govet
type CaptchaService struct {
	settingService *SettingService
	defaultConfig  config.CaptchaConfig

	httpClient *http.Client
	cacheTTL   time.Duration

	mu            sync.RWMutex
	cachedSetting CaptchaSetting
	cachedAt      time.Time

	imageStore          base64Captcha.Store
	imageStoreMaxStore  int
	imageStoreExpireSec int
}

// NewCaptchaService 创建验证码服务
func NewCaptchaService(settingService *SettingService, defaultConfig config.CaptchaConfig) *CaptchaService {
	return &CaptchaService{
		settingService: settingService,
		defaultConfig:  defaultConfig,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		cacheTTL: 30 * time.Second,
	}
}

// SetDefaultConfig 更新默认配置（通常在后台保存后调用）
func (s *CaptchaService) SetDefaultConfig(defaultConfig config.CaptchaConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultConfig = defaultConfig
	s.cachedAt = time.Time{}
}

// InvalidateCache 失效本地缓存配置
func (s *CaptchaService) InvalidateCache() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedAt = time.Time{}
}

// GetPublicSetting 获取公开可下发配置
func (s *CaptchaService) GetPublicSetting() (models.JSON, error) {
	setting, err := s.getSetting()
	if err != nil {
		return nil, err
	}
	return PublicCaptchaSetting(setting), nil
}

// GenerateImageChallenge 生成图片验证码
func (s *CaptchaService) GenerateImageChallenge() (*CaptchaImageChallenge, error) {
	setting, err := s.getSetting()
	if err != nil {
		return nil, err
	}
	if setting.Provider != constants.CaptchaProviderImage {
		return nil, ErrCaptchaConfigInvalid
	}

	store := s.ensureImageStore(setting)
	driver := base64Captcha.NewDriverString(
		setting.Image.Height,
		setting.Image.Width,
		setting.Image.NoiseCount,
		setting.Image.ShowLine,
		setting.Image.Length,
		"0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
		nil,
		base64Captcha.DefaultEmbeddedFonts,
		nil,
	)
	captcha := base64Captcha.NewCaptcha(driver, store)
	id, b64s, _, genErr := captcha.Generate()
	if genErr != nil {
		return nil, genErr
	}

	return &CaptchaImageChallenge{
		CaptchaID:   strings.TrimSpace(id),
		ImageBase64: strings.TrimSpace(b64s),
	}, nil
}

// Verify 按场景校验验证码
func (s *CaptchaService) Verify(scene string, payload CaptchaVerifyPayload, clientIP string) error {
	setting, err := s.getSetting()
	if err != nil {
		return err
	}

	if !setting.IsSceneEnabled(scene) {
		return nil
	}

	switch setting.Provider {
	case constants.CaptchaProviderImage:
		captchaID := strings.TrimSpace(payload.CaptchaID)
		captchaCode := strings.TrimSpace(payload.CaptchaCode)
		if captchaID == "" || captchaCode == "" {
			return ErrCaptchaRequired
		}
		store := s.ensureImageStore(setting)
		if !store.Verify(captchaID, captchaCode, true) {
			return ErrCaptchaInvalid
		}
		return nil
	case constants.CaptchaProviderTurnstile:
		token := strings.TrimSpace(payload.TurnstileToken)
		if token == "" {
			return ErrCaptchaRequired
		}
		return s.verifyTurnstile(setting.Turnstile, token, strings.TrimSpace(clientIP))
	case constants.CaptchaProviderNone:
		return ErrCaptchaConfigInvalid
	default:
		return ErrCaptchaConfigInvalid
	}
}

func (s *CaptchaService) verifyTurnstile(cfg CaptchaTurnstileSetting, token, clientIP string) error {
	secret := strings.TrimSpace(cfg.SecretKey)
	verifyURL := strings.TrimSpace(cfg.VerifyURL)
	if secret == "" || verifyURL == "" {
		return ErrCaptchaConfigInvalid
	}

	timeout := cfg.TimeoutMS
	if timeout < 500 || timeout > 10000 {
		timeout = 2000
	}

	client := s.httpClient
	if client == nil || client.Timeout != time.Duration(timeout)*time.Millisecond {
		client = &http.Client{Timeout: time.Duration(timeout) * time.Millisecond}
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if clientIP != "" {
		form.Set("remoteip", clientIP)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaptchaVerifyFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaptchaVerifyFailed, err)
	}
	defer resp.Body.Close()

	var result turnstileVerifyResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
		return fmt.Errorf("%w: %v", ErrCaptchaVerifyFailed, decodeErr)
	}
	if !result.Success {
		return ErrCaptchaInvalid
	}
	return nil
}

func (s *CaptchaService) ensureImageStore(setting CaptchaSetting) base64Captcha.Store {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.imageStore != nil && s.imageStoreMaxStore == setting.Image.MaxStore && s.imageStoreExpireSec == setting.Image.ExpireSeconds {
		return s.imageStore
	}
	s.imageStore = base64Captcha.NewMemoryStore(setting.Image.MaxStore, time.Duration(setting.Image.ExpireSeconds)*time.Second)
	s.imageStoreMaxStore = setting.Image.MaxStore
	s.imageStoreExpireSec = setting.Image.ExpireSeconds
	return s.imageStore
}

func (s *CaptchaService) getSetting() (CaptchaSetting, error) {
	if s == nil {
		return CaptchaDefaultSetting(config.CaptchaConfig{}), nil
	}

	now := time.Now()
	s.mu.RLock()
	if !s.cachedAt.IsZero() && now.Sub(s.cachedAt) <= s.cacheTTL {
		cached := s.cachedSetting
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	fallback := s.defaultConfig
	if s.settingService == nil {
		setting := CaptchaDefaultSetting(fallback)
		s.mu.Lock()
		s.cachedSetting = setting
		s.cachedAt = now
		s.mu.Unlock()
		return setting, nil
	}

	setting, err := s.settingService.GetCaptchaSetting(fallback)
	if err != nil {
		return CaptchaSetting{}, err
	}
	setting = NormalizeCaptchaSetting(setting)

	s.mu.Lock()
	s.cachedSetting = setting
	s.cachedAt = now
	s.mu.Unlock()
	return setting, nil
}
