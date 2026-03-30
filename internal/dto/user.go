package dto

import (
	"time"

	"github.com/dujiao-next/internal/models"
)

// UserProfileResp 用户资料响应
type UserProfileResp struct {
	ID                 uint         `json:"id"`
	Email              string       `json:"email"`
	Nickname           string       `json:"nickname"`
	EmailVerifiedAt    *time.Time   `json:"email_verified_at"`
	Locale             string       `json:"locale"`
	MemberLevelID      uint         `json:"member_level_id"`
	TotalRecharged     models.Money `json:"total_recharged"`
	TotalSpent         models.Money `json:"total_spent"`
	EmailChangeMode    string       `json:"email_change_mode,omitempty"`
	PasswordChangeMode string       `json:"password_change_mode,omitempty"`
}

// NewUserProfileResp 从 models.User 构造用户资料响应
func NewUserProfileResp(user *models.User, emailMode, passwordMode string) UserProfileResp {
	if user == nil {
		return UserProfileResp{}
	}
	return UserProfileResp{
		ID:                 user.ID,
		Email:              user.Email,
		Nickname:           user.DisplayName,
		EmailVerifiedAt:    user.EmailVerifiedAt,
		Locale:             user.Locale,
		MemberLevelID:      user.MemberLevelID,
		TotalRecharged:     user.TotalRecharged,
		TotalSpent:         user.TotalSpent,
		EmailChangeMode:    emailMode,
		PasswordChangeMode: passwordMode,
	}
	// 排除：PasswordHash、PasswordSetupRequired、Status、TokenVersion、TokenInvalidBefore、
	// LastLoginAt、CreatedAt、UpdatedAt、DeletedAt
}

// UserAuthBriefResp 登录/注册返回的精简用户信息
type UserAuthBriefResp struct {
	ID              uint       `json:"id"`
	Email           string     `json:"email"`
	Nickname        string     `json:"nickname"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
}

// NewUserAuthBriefResp 从 models.User 构造登录/注册精简响应
func NewUserAuthBriefResp(user *models.User) UserAuthBriefResp {
	return UserAuthBriefResp{
		ID:              user.ID,
		Email:           user.Email,
		Nickname:        user.DisplayName,
		EmailVerifiedAt: user.EmailVerifiedAt,
	}
}
