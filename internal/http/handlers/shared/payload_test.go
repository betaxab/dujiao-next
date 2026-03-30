package shared

import (
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"

	"github.com/gin-gonic/gin"
)

func TestBuildChannelIdentityResponse(t *testing.T) {
	user := &models.User{
		ID:                    12,
		Email:                 "telegram_12@login.local",
		DisplayName:           "TG Buyer",
		Status:                "active",
		Locale:                "zh-CN",
		PasswordSetupRequired: true,
	}
	identity := &models.UserOAuthIdentity{
		Provider:       "telegram",
		ProviderUserID: "12",
		Username:       "buyer12",
		AvatarURL:      "https://example.com/avatar.png",
	}

	payload := BuildChannelIdentityResponse(true, true, user, identity)
	if payload["bound"] != true {
		t.Fatalf("bound flag mismatch: %#v", payload["bound"])
	}
	if payload["created"] != true {
		t.Fatalf("created flag mismatch: %#v", payload["created"])
	}
	identityPayload, ok := payload["identity"].(gin.H)
	if !ok {
		t.Fatalf("identity payload type mismatch: %#v", payload["identity"])
	}
	if identityPayload["provider_user_id"] != "12" {
		t.Fatalf("provider user id mismatch: %#v", identityPayload["provider_user_id"])
	}
	userPayload, ok := payload["user"].(gin.H)
	if !ok {
		t.Fatalf("user payload type mismatch: %#v", payload["user"])
	}
	if userPayload["display_name"] != "TG Buyer" {
		t.Fatalf("display name mismatch: %#v", userPayload["display_name"])
	}
}

func TestBuildTelegramBindingResponse(t *testing.T) {
	if payload := BuildTelegramBindingResponse(nil); payload["bound"] != false {
		t.Fatalf("nil identity should be unbound: %#v", payload)
	}

	authAt := time.Now().UTC()
	identity := &models.UserOAuthIdentity{
		Provider:       "telegram",
		ProviderUserID: "9988",
		Username:       "buyer9988",
		AuthAt:         &authAt,
	}
	payload := BuildTelegramBindingResponse(identity)
	if payload["bound"] != true {
		t.Fatalf("bound flag mismatch: %#v", payload["bound"])
	}
	if payload["provider_user_id"] != "9988" {
		t.Fatalf("provider user id mismatch: %#v", payload["provider_user_id"])
	}
}
