package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// UserAuthService 用户认证服务
type UserAuthService struct {
	cfg          *config.Config
	userRepo     repository.UserRepository
	codeRepo     repository.EmailVerifyCodeRepository
	emailService *EmailService
}

// NewUserAuthService 创建用户认证服务
func NewUserAuthService(cfg *config.Config, userRepo repository.UserRepository, codeRepo repository.EmailVerifyCodeRepository, emailService *EmailService) *UserAuthService {
	return &UserAuthService{
		cfg:          cfg,
		userRepo:     userRepo,
		codeRepo:     codeRepo,
		emailService: emailService,
	}
}

// UserJWTClaims 用户 JWT 声明
type UserJWTClaims struct {
	UserID       uint   `json:"user_id"`
	Email        string `json:"email"`
	TokenVersion uint64 `json:"token_version"`
	jwt.RegisteredClaims
}

// GenerateUserJWT 生成用户 JWT Token
func (s *UserAuthService) GenerateUserJWT(user *models.User, expireHours int) (string, time.Time, error) {
	resolvedHours := expireHours
	if resolvedHours <= 0 {
		resolvedHours = resolveUserJWTExpireHours(s.cfg.UserJWT)
	}
	expiresAt := time.Now().Add(time.Duration(resolvedHours) * time.Hour)
	claims := UserJWTClaims{
		UserID:       user.ID,
		Email:        user.Email,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.UserJWT.SecretKey))
	if err != nil {
		return "", time.Time{}, err
	}
	return tokenString, expiresAt, nil
}

// ParseUserJWT 解析用户 JWT Token
func (s *UserAuthService) ParseUserJWT(tokenString string) (*UserJWTClaims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	claims := &UserJWTClaims{}
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.UserJWT.SecretKey), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*UserJWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("无效的 token")
}

// SendVerifyCode 发送邮箱验证码
func (s *UserAuthService) SendVerifyCode(email, purpose, locale string) error {
	if s.emailService == nil {
		return ErrEmailServiceNotConfigured
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if !isVerifyPurposeSupported(purpose) {
		return ErrInvalidVerifyPurpose
	}

	if purpose == constants.VerifyPurposeRegister {
		exist, err := s.userRepo.GetByEmail(normalized)
		if err != nil {
			return err
		}
		if exist != nil {
			return ErrEmailExists
		}
	}

	if purpose == constants.VerifyPurposeReset {
		user, err := s.userRepo.GetByEmail(normalized)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNotFound
		}
		if strings.TrimSpace(user.Locale) != "" {
			locale = user.Locale
		}
	}

	return s.sendVerifyCode(normalized, strings.ToLower(purpose), locale)
}

// Register 用户注册
func (s *UserAuthService) Register(email, password, code string, agreementAccepted bool) (*models.User, string, time.Time, error) {
	if !agreementAccepted {
		return nil, "", time.Time{}, ErrAgreementRequired
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if err := validatePassword(s.cfg.Security.PasswordPolicy, password); err != nil {
		return nil, "", time.Time{}, err
	}

	exist, err := s.userRepo.GetByEmail(normalized)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if exist != nil {
		return nil, "", time.Time{}, ErrEmailExists
	}

	if _, err := s.verifyCode(normalized, constants.VerifyPurposeRegister, code); err != nil {
		return nil, "", time.Time{}, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	now := time.Now()
	nickname := resolveNicknameFromEmail(normalized)
	user := &models.User{
		Email:           normalized,
		PasswordHash:    string(hashedPassword),
		DisplayName:     nickname,
		Status:          constants.UserStatusActive,
		EmailVerifiedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.GenerateUserJWT(user, 0)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	user.LastLoginAt = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, "", time.Time{}, err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))

	return user, token, expiresAt, nil
}

// Login 用户登录
func (s *UserAuthService) Login(email, password string) (*models.User, string, time.Time, error) {
	return s.LoginWithRememberMe(email, password, false)
}

// LoginWithRememberMe 用户登录（支持记住我）
func (s *UserAuthService) LoginWithRememberMe(email, password string, rememberMe bool) (*models.User, string, time.Time, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	user, err := s.userRepo.GetByEmail(normalized)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if user == nil {
		return nil, "", time.Time{}, ErrInvalidCredentials
	}
	if strings.ToLower(user.Status) != constants.UserStatusActive {
		return nil, "", time.Time{}, ErrUserDisabled
	}
	if user.EmailVerifiedAt == nil {
		return nil, "", time.Time{}, ErrEmailNotVerified
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", time.Time{}, ErrInvalidCredentials
	}

	expireHours := resolveUserJWTExpireHours(s.cfg.UserJWT)
	if rememberMe {
		expireHours = resolveRememberMeExpireHours(s.cfg.UserJWT)
	}
	token, expiresAt, err := s.GenerateUserJWT(user, expireHours)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	now := time.Now()
	user.LastLoginAt = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, "", time.Time{}, err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))

	return user, token, expiresAt, nil
}

// ResetPassword 重置密码
func (s *UserAuthService) ResetPassword(email, code, newPassword string) error {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if err := validatePassword(s.cfg.Security.PasswordPolicy, newPassword); err != nil {
		return err
	}
	user, err := s.userRepo.GetByEmail(normalized)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	if _, err := s.verifyCode(normalized, constants.VerifyPurposeReset, code); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hashedPassword)
	now := time.Now()
	user.UpdatedAt = now
	user.TokenVersion++
	user.TokenInvalidBefore = &now
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))
	return nil
}

// ChangePassword 登录态修改密码
func (s *UserAuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	if userID == 0 {
		return ErrNotFound
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidPassword
	}

	if err := validatePassword(s.cfg.Security.PasswordPolicy, newPassword); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hashedPassword)
	now := time.Now()
	user.UpdatedAt = now
	user.TokenVersion++
	user.TokenInvalidBefore = &now
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))
	return nil
}

// UpdateProfile 更新用户资料
func (s *UserAuthService) UpdateProfile(userID uint, nickname, locale *string) (*models.User, error) {
	if userID == 0 {
		return nil, ErrNotFound
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}

	updated := false
	if nickname != nil {
		trimmed := strings.TrimSpace(*nickname)
		if trimmed != "" {
			user.DisplayName = trimmed
			updated = true
		}
	}

	if locale != nil {
		trimmed := strings.TrimSpace(*locale)
		if trimmed != "" {
			user.Locale = trimmed
			updated = true
		}
	}

	if !updated {
		return nil, ErrProfileEmpty
	}

	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	return user, nil
}

// SendChangeEmailCode 发送更换邮箱验证码
func (s *UserAuthService) SendChangeEmailCode(userID uint, kind, newEmail, locale string) error {
	if s.emailService == nil {
		return ErrEmailServiceNotConfigured
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	if strings.TrimSpace(user.Locale) != "" {
		locale = user.Locale
	}

	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "old":
		return s.sendVerifyCode(user.Email, constants.VerifyPurposeChangeEmailOld, locale)
	case "new":
		normalized, err := normalizeEmail(newEmail)
		if err != nil {
			return err
		}
		if strings.EqualFold(normalized, user.Email) {
			return ErrEmailChangeInvalid
		}
		exist, err := s.userRepo.GetByEmail(normalized)
		if err != nil {
			return err
		}
		if exist != nil {
			return ErrEmailChangeExists
		}
		return s.sendVerifyCode(normalized, constants.VerifyPurposeChangeEmailNew, locale)
	default:
		return ErrEmailChangeInvalid
	}
}

// ChangeEmail 更换邮箱（旧邮箱/新邮箱双验证）
func (s *UserAuthService) ChangeEmail(userID uint, newEmail, oldCode, newCode string) (*models.User, error) {
	if userID == 0 {
		return nil, ErrNotFound
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}

	normalized, err := normalizeEmail(newEmail)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(normalized, user.Email) {
		return nil, ErrEmailChangeInvalid
	}
	exist, err := s.userRepo.GetByEmail(normalized)
	if err != nil {
		return nil, err
	}
	if exist != nil {
		return nil, ErrEmailChangeExists
	}

	if _, err := s.verifyCode(user.Email, constants.VerifyPurposeChangeEmailOld, oldCode); err != nil {
		return nil, err
	}
	if _, err := s.verifyCode(normalized, constants.VerifyPurposeChangeEmailNew, newCode); err != nil {
		return nil, err
	}

	now := time.Now()
	user.Email = normalized
	user.EmailVerifiedAt = &now
	user.UpdatedAt = now
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID 获取用户信息
func (s *UserAuthService) GetUserByID(id uint) (*models.User, error) {
	if id == 0 {
		return nil, ErrNotFound
	}
	return s.userRepo.GetByID(id)
}

func (s *UserAuthService) verifyCode(email, purpose, code string) (*models.EmailVerifyCode, error) {
	record, err := s.codeRepo.GetLatest(email, purpose)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrVerifyCodeInvalid
	}
	if record.VerifiedAt != nil {
		return nil, ErrVerifyCodeInvalid
	}

	now := time.Now()
	if record.ExpiresAt.Before(now) {
		return nil, ErrVerifyCodeExpired
	}

	maxAttempts := resolveMaxAttempts(s.cfg.Email.VerifyCode)
	if maxAttempts > 0 && record.AttemptCount >= maxAttempts {
		return nil, ErrVerifyCodeAttemptsExceeded
	}

	if strings.TrimSpace(record.Code) != strings.TrimSpace(code) {
		_ = s.codeRepo.IncrementAttempt(record.ID)
		return nil, ErrVerifyCodeInvalid
	}

	if err := s.codeRepo.MarkVerified(record.ID, now); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *UserAuthService) sendVerifyCode(email, purpose, locale string) error {
	latest, err := s.codeRepo.GetLatest(email, purpose)
	if err != nil {
		return err
	}
	now := time.Now()
	if latest != nil {
		interval := time.Duration(resolveSendIntervalSeconds(s.cfg.Email.VerifyCode)) * time.Second
		if !latest.SentAt.IsZero() && now.Sub(latest.SentAt) < interval {
			return ErrVerifyCodeTooFrequent
		}
	}

	code, err := randomNumericCode(resolveCodeLength(s.cfg.Email.VerifyCode))
	if err != nil {
		return err
	}

	record := &models.EmailVerifyCode{
		Email:     email,
		Purpose:   strings.ToLower(purpose),
		Code:      code,
		ExpiresAt: now.Add(time.Duration(resolveExpireMinutes(s.cfg.Email.VerifyCode)) * time.Minute),
		SentAt:    now,
		CreatedAt: now,
	}
	if err := s.emailService.SendVerifyCode(email, code, purpose, locale); err != nil {
		return err
	}

	if err := s.codeRepo.Create(record); err != nil {
		return err
	}

	return nil
}

func normalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return "", ErrInvalidEmail
	}
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", ErrInvalidEmail
	}
	return normalized, nil
}

// NormalizeEmail 统一邮箱格式
func NormalizeEmail(email string) (string, error) {
	return normalizeEmail(email)
}

func isVerifyPurposeSupported(purpose string) bool {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case constants.VerifyPurposeRegister, constants.VerifyPurposeReset, constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
		return true
	default:
		return false
	}
}

func resolveUserJWTExpireHours(cfg config.JWTConfig) int {
	if cfg.ExpireHours <= 0 {
		return 24
	}
	return cfg.ExpireHours
}

func resolveRememberMeExpireHours(cfg config.JWTConfig) int {
	if cfg.RememberMeExpireHours <= 0 {
		return resolveUserJWTExpireHours(cfg)
	}
	return cfg.RememberMeExpireHours
}

func resolveNicknameFromEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0])
	}
	return email
}

func resolveExpireMinutes(cfg config.VerifyCodeConfig) int {
	if cfg.ExpireMinutes <= 0 {
		return 10
	}
	return cfg.ExpireMinutes
}

func resolveSendIntervalSeconds(cfg config.VerifyCodeConfig) int {
	if cfg.SendIntervalSeconds <= 0 {
		return 60
	}
	return cfg.SendIntervalSeconds
}

func resolveMaxAttempts(cfg config.VerifyCodeConfig) int {
	if cfg.MaxAttempts <= 0 {
		return 5
	}
	return cfg.MaxAttempts
}

func resolveCodeLength(cfg config.VerifyCodeConfig) int {
	if cfg.Length < 4 || cfg.Length > 10 {
		return 6
	}
	return cfg.Length
}

func randomNumericCode(length int) (string, error) {
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteString(fmt.Sprintf("%d", n.Int64()))
	}
	return b.String(), nil
}
