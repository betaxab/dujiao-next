package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func setupWalletServiceTest(t *testing.T) (*WalletService, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:wallet_service_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Order{},
		&models.OrderItem{},
		&models.Fulfillment{},
		&models.WalletAccount{},
		&models.WalletTransaction{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	models.DB = db
	walletRepo := repository.NewWalletRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	userRepo := repository.NewUserRepository(db)
	return NewWalletService(walletRepo, orderRepo, userRepo), db
}

func createTestUser(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	user := models.User{
		ID:           id,
		Email:        fmt.Sprintf("wallet_user_%d@example.com", id),
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
}

func createTestOrder(t *testing.T, db *gorm.DB, userID uint, orderNo string, total decimal.Decimal) *models.Order {
	t.Helper()
	now := time.Now()
	order := &models.Order{
		OrderNo:          orderNo,
		UserID:           userID,
		Status:           constants.OrderStatusPendingPayment,
		Currency:         "CNY",
		OriginalAmount:   models.NewMoneyFromDecimal(total),
		DiscountAmount:   models.NewMoneyFromDecimal(decimal.Zero),
		TotalAmount:      models.NewMoneyFromDecimal(total),
		WalletPaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		OnlinePaidAmount: models.NewMoneyFromDecimal(total),
		RefundedAmount:   models.NewMoneyFromDecimal(decimal.Zero),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	return order
}

func TestWalletServiceRecharge(t *testing.T) {
	svc, db := setupWalletServiceTest(t)
	createTestUser(t, db, 101)

	account, txn, err := svc.Recharge(WalletRechargeInput{
		UserID: 101,
		Amount: models.NewMoneyFromDecimal(decimal.NewFromInt(120)),
		Remark: "测试充值",
	})
	if err != nil {
		t.Fatalf("recharge failed: %v", err)
	}
	if !account.Balance.Decimal.Equal(decimal.NewFromInt(120)) {
		t.Fatalf("unexpected balance: %s", account.Balance.String())
	}
	if txn == nil || txn.Type != constants.WalletTxnTypeRecharge || txn.Direction != constants.WalletTxnDirectionIn {
		t.Fatalf("unexpected transaction: %+v", txn)
	}
}

func TestWalletServiceAdminAdjustInsufficient(t *testing.T) {
	svc, db := setupWalletServiceTest(t)
	createTestUser(t, db, 102)

	if _, _, err := svc.Recharge(WalletRechargeInput{
		UserID: 102,
		Amount: models.NewMoneyFromDecimal(decimal.NewFromInt(10)),
	}); err != nil {
		t.Fatalf("recharge failed: %v", err)
	}

	_, _, err := svc.AdminAdjustBalance(WalletAdjustInput{
		UserID: 102,
		Delta:  models.NewMoneyFromDecimal(decimal.NewFromInt(-20)),
		Remark: "测试扣减",
	})
	if !errors.Is(err, ErrWalletInsufficientBalance) {
		t.Fatalf("expected insufficient balance, got: %v", err)
	}
}

func TestWalletServiceApplyAndReleaseOrderBalance(t *testing.T) {
	svc, db := setupWalletServiceTest(t)
	createTestUser(t, db, 103)
	order := createTestOrder(t, db, 103, "DJTESTAPPLY001", decimal.NewFromInt(30))

	if _, _, err := svc.Recharge(WalletRechargeInput{
		UserID: 103,
		Amount: models.NewMoneyFromDecimal(decimal.NewFromInt(50)),
	}); err != nil {
		t.Fatalf("recharge failed: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		deducted, err := svc.ApplyOrderBalance(tx, order, true)
		if err != nil {
			return err
		}
		if !deducted.Equal(decimal.NewFromInt(30)) {
			t.Fatalf("expected deducted 30, got %s", deducted.String())
		}
		return nil
	}); err != nil {
		t.Fatalf("apply order balance failed: %v", err)
	}

	account, err := svc.GetAccount(103)
	if err != nil {
		t.Fatalf("get account failed: %v", err)
	}
	if !account.Balance.Decimal.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("unexpected balance after apply: %s", account.Balance.String())
	}

	var refreshed models.Order
	if err := db.First(&refreshed, order.ID).Error; err != nil {
		t.Fatalf("reload order failed: %v", err)
	}
	order.WalletPaidAmount = refreshed.WalletPaidAmount
	order.OnlinePaidAmount = refreshed.OnlinePaidAmount

	if err := db.Transaction(func(tx *gorm.DB) error {
		refunded, err := svc.ReleaseOrderBalance(tx, order, constants.WalletTxnTypeOrderRefund, "测试回退")
		if err != nil {
			return err
		}
		if !refunded.Equal(decimal.NewFromInt(30)) {
			t.Fatalf("expected refunded 30, got %s", refunded.String())
		}
		return nil
	}); err != nil {
		t.Fatalf("release order balance failed: %v", err)
	}

	account, err = svc.GetAccount(103)
	if err != nil {
		t.Fatalf("get account failed: %v", err)
	}
	if !account.Balance.Decimal.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("unexpected balance after release: %s", account.Balance.String())
	}
}

func TestWalletServiceAdminRefundToWallet(t *testing.T) {
	svc, db := setupWalletServiceTest(t)
	createTestUser(t, db, 104)
	order := createTestOrder(t, db, 104, "DJTESTREFUND001", decimal.NewFromInt(40))
	paidAt := time.Now()
	if err := db.Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"status":  constants.OrderStatusPaid,
		"paid_at": paidAt,
	}).Error; err != nil {
		t.Fatalf("update order status failed: %v", err)
	}

	updatedOrder, txn, err := svc.AdminRefundToWallet(AdminRefundToWalletInput{
		OrderID: order.ID,
		Amount:  models.NewMoneyFromDecimal(decimal.NewFromInt(15)),
		Remark:  "测试退款",
	})
	if err != nil {
		t.Fatalf("admin refund failed: %v", err)
	}
	if txn == nil || txn.Type != constants.WalletTxnTypeAdminRefund {
		t.Fatalf("unexpected refund transaction: %+v", txn)
	}
	if !updatedOrder.RefundedAmount.Decimal.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("unexpected refunded amount: %s", updatedOrder.RefundedAmount.String())
	}

	_, _, err = svc.AdminRefundToWallet(AdminRefundToWalletInput{
		OrderID: order.ID,
		Amount:  models.NewMoneyFromDecimal(decimal.NewFromInt(30)),
		Remark:  "超额退款",
	})
	if !errors.Is(err, ErrWalletRefundExceeded) {
		t.Fatalf("expected refund exceeded, got: %v", err)
	}
}

func TestWalletServiceAdminRefundToWalletRejectUnpaidOrder(t *testing.T) {
	svc, db := setupWalletServiceTest(t)
	createTestUser(t, db, 105)
	order := createTestOrder(t, db, 105, "DJTESTREFUND002", decimal.NewFromInt(40))
	if err := db.Model(&models.Order{}).Where("id = ?", order.ID).Update("status", constants.OrderStatusCanceled).Error; err != nil {
		t.Fatalf("update order status failed: %v", err)
	}

	_, _, err := svc.AdminRefundToWallet(AdminRefundToWalletInput{
		OrderID: order.ID,
		Amount:  models.NewMoneyFromDecimal(decimal.NewFromInt(15)),
		Remark:  "未支付退款",
	})
	if !errors.Is(err, ErrOrderStatusInvalid) {
		t.Fatalf("expected order status invalid, got: %v", err)
	}
}
