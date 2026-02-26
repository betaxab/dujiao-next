package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func setupPaymentRepositoryTest(t *testing.T) (*GormPaymentRepository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:payment_repo_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Order{},
		&models.Payment{},
		&models.WalletRechargeOrder{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewPaymentRepository(db), db
}

func TestPaymentRepositoryListAdminByUserIncludesWalletRechargePayments(t *testing.T) {
	repo, db := setupPaymentRepositoryTest(t)
	now := time.Now().UTC().Truncate(time.Second)

	user1 := models.User{
		Email:        "payment_repo_user1@example.com",
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	user2 := models.User{
		Email:        "payment_repo_user2@example.com",
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(&user1).Error; err != nil {
		t.Fatalf("create user1 failed: %v", err)
	}
	if err := db.Create(&user2).Error; err != nil {
		t.Fatalf("create user2 failed: %v", err)
	}

	order := models.Order{
		OrderNo:                 "DJPAYREPO001",
		UserID:                  user1.ID,
		Status:                  constants.OrderStatusPendingPayment,
		Currency:                "CNY",
		OriginalAmount:          models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		DiscountAmount:          models.NewMoneyFromDecimal(decimal.Zero),
		PromotionDiscountAmount: models.NewMoneyFromDecimal(decimal.Zero),
		TotalAmount:             models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		WalletPaidAmount:        models.NewMoneyFromDecimal(decimal.Zero),
		OnlinePaidAmount:        models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		RefundedAmount:          models.NewMoneyFromDecimal(decimal.Zero),
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	orderPayment := models.Payment{
		OrderID:         order.ID,
		ChannelID:       1,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.PaymentStatusSuccess,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&orderPayment).Error; err != nil {
		t.Fatalf("create order payment failed: %v", err)
	}

	rechargePaymentUser1 := models.Payment{
		OrderID:         0,
		ChannelID:       2,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeWechat,
		InteractionMode: constants.PaymentInteractionQR,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(50)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.PaymentStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&rechargePaymentUser1).Error; err != nil {
		t.Fatalf("create user1 recharge payment failed: %v", err)
	}
	if err := db.Create(&models.WalletRechargeOrder{
		RechargeNo:      "DJRUSER1001",
		UserID:          user1.ID,
		PaymentID:       rechargePaymentUser1.ID,
		ChannelID:       rechargePaymentUser1.ChannelID,
		ProviderType:    rechargePaymentUser1.ProviderType,
		ChannelType:     rechargePaymentUser1.ChannelType,
		InteractionMode: rechargePaymentUser1.InteractionMode,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(50)),
		PayableAmount:   models.NewMoneyFromDecimal(decimal.NewFromInt(50)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.WalletRechargeStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error; err != nil {
		t.Fatalf("create user1 recharge order failed: %v", err)
	}

	rechargePaymentUser2 := models.Payment{
		OrderID:         0,
		ChannelID:       3,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeWechat,
		InteractionMode: constants.PaymentInteractionQR,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(60)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.PaymentStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&rechargePaymentUser2).Error; err != nil {
		t.Fatalf("create user2 recharge payment failed: %v", err)
	}
	if err := db.Create(&models.WalletRechargeOrder{
		RechargeNo:      "DJRUSER2001",
		UserID:          user2.ID,
		PaymentID:       rechargePaymentUser2.ID,
		ChannelID:       rechargePaymentUser2.ChannelID,
		ProviderType:    rechargePaymentUser2.ProviderType,
		ChannelType:     rechargePaymentUser2.ChannelType,
		InteractionMode: rechargePaymentUser2.InteractionMode,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(60)),
		PayableAmount:   models.NewMoneyFromDecimal(decimal.NewFromInt(60)),
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Currency:        "CNY",
		Status:          constants.WalletRechargeStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error; err != nil {
		t.Fatalf("create user2 recharge order failed: %v", err)
	}

	rows, total, err := repo.ListAdmin(PaymentListFilter{
		Page:     1,
		PageSize: 50,
		UserID:   user1.ID,
	})
	if err != nil {
		t.Fatalf("list admin payments failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("total want 2 got %d", total)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len want 2 got %d", len(rows))
	}

	foundOrderPayment := false
	foundUser1Recharge := false
	for _, row := range rows {
		if row.ID == orderPayment.ID {
			foundOrderPayment = true
		}
		if row.ID == rechargePaymentUser1.ID {
			foundUser1Recharge = true
		}
		if row.ID == rechargePaymentUser2.ID {
			t.Fatalf("should not include other user's recharge payment, got payment_id=%d", row.ID)
		}
	}
	if !foundOrderPayment {
		t.Fatalf("missing normal order payment for user")
	}
	if !foundUser1Recharge {
		t.Fatalf("missing wallet recharge payment for user")
	}
}
