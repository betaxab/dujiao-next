package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/repository"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestBuildOrderNotificationPayloadIncludesCustomerAndItemSummary(t *testing.T) {
	dsn := fmt.Sprintf("file:payment_notification_payload_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("auto migrate user failed: %v", err)
	}

	user := &models.User{
		Email:        "member@example.com",
		DisplayName:  "Member User",
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	repo := newMockSettingRepo()
	repo.store[constants.SettingKeyNotificationCenterConfig] = models.JSON{
		"default_locale": "en-US",
	}

	svc := &PaymentService{
		userRepo:       repository.NewUserRepository(db),
		settingService: NewSettingService(repo),
	}
	order := &models.Order{
		ID:          1001,
		OrderNo:     "DJ202603230001",
		UserID:      user.ID,
		Currency:    "usd",
		Status:      constants.OrderStatusPaid,
		TotalAmount: models.NewMoneyFromDecimal(decimal.NewFromInt(99)),
		Items: []models.OrderItem{
			{
				TitleJSON: models.JSON{
					"zh-CN": "自动发货商品",
					"en-US": "Auto Product",
				},
				SKUSnapshotJSON: models.JSON{
					"sku_code": "AUTO-001",
					"spec_values": models.JSON{
						"zh-CN": "区域: HK",
						"en-US": "Region: HK",
					},
				},
				Quantity:        1,
				FulfillmentType: constants.FulfillmentTypeAuto,
			},
			{
				TitleJSON: models.JSON{
					"zh-CN": "人工交付商品",
					"en-US": "Manual Product",
				},
				SKUSnapshotJSON: models.JSON{
					"sku_code": "MANUAL-002",
					"spec_values": models.JSON{
						"zh-CN": "周期: 30天",
						"en-US": "Cycle: 30 days",
					},
				},
				Quantity:        2,
				FulfillmentType: constants.FulfillmentTypeManual,
			},
		},
	}
	payment := &models.Payment{
		ID:           9,
		ProviderType: "epay",
		ChannelType:  "alipay",
	}

	payload := svc.buildOrderNotificationPayload(order, payment)

	if got := fmt.Sprintf("%v", payload["customer_email"]); got != "member@example.com" {
		t.Fatalf("customer_email want member@example.com got %s", got)
	}
	if got := fmt.Sprintf("%v", payload["customer_type"]); got != "registered" {
		t.Fatalf("customer_type want registered got %s", got)
	}
	if got := fmt.Sprintf("%v", payload["payment_channel"]); got != "epay/alipay" {
		t.Fatalf("payment_channel want epay/alipay got %s", got)
	}

	itemsSummary := fmt.Sprintf("%v", payload["items_summary"])
	if !strings.Contains(itemsSummary, "Auto Product / Region: HK x1 [Auto]") {
		t.Fatalf("items_summary missing auto item detail: %s", itemsSummary)
	}
	if !strings.Contains(itemsSummary, "Manual Product / Cycle: 30 days x2 [Manual]") {
		t.Fatalf("items_summary missing manual item detail: %s", itemsSummary)
	}

	fulfillmentSummary := fmt.Sprintf("%v", payload["fulfillment_items_summary"])
	if strings.Contains(fulfillmentSummary, "Auto Product") {
		t.Fatalf("fulfillment_items_summary should not include auto item: %s", fulfillmentSummary)
	}
	if !strings.Contains(fulfillmentSummary, "Manual Product / Cycle: 30 days x2 [Manual]") {
		t.Fatalf("fulfillment_items_summary missing manual item detail: %s", fulfillmentSummary)
	}

	deliverySummary := fmt.Sprintf("%v", payload["delivery_summary"])
	if deliverySummary != "Total 2 items, auto 1, manual 1, upstream 0" {
		t.Fatalf("unexpected delivery_summary: %s", deliverySummary)
	}
}

func TestBuildOrderNotificationPayloadFallsBackToChildrenItems(t *testing.T) {
	repo := newMockSettingRepo()
	repo.store[constants.SettingKeyNotificationCenterConfig] = models.JSON{
		"default_locale": "en-US",
	}

	svc := &PaymentService{
		settingService: NewSettingService(repo),
	}
	order := &models.Order{
		ID:          2001,
		OrderNo:     "DJ202603230201",
		Currency:    "usd",
		Status:      constants.OrderStatusPaid,
		TotalAmount: models.NewMoneyFromDecimal(decimal.NewFromInt(42)),
		Children: []models.Order{
			{
				ID: 2002,
				Items: []models.OrderItem{
					{
						OrderID: 2002,
						TitleJSON: models.JSON{
							"zh-CN": "上游交付商品",
							"en-US": "Upstream Product",
						},
						SKUSnapshotJSON: models.JSON{
							"sku_code": "UPSTREAM-001",
							"spec_values": models.JSON{
								"zh-CN": "节点: SG",
								"en-US": "Node: SG",
							},
						},
						Quantity:        1,
						FulfillmentType: constants.FulfillmentTypeUpstream,
					},
				},
			},
			{
				ID: 2003,
				Items: []models.OrderItem{
					{
						OrderID: 2003,
						TitleJSON: models.JSON{
							"zh-CN": "人工交付商品",
							"en-US": "Manual Product",
						},
						SKUSnapshotJSON: models.JSON{
							"sku_code": "MANUAL-003",
							"spec_values": models.JSON{
								"zh-CN": "周期: 7天",
								"en-US": "Cycle: 7 days",
							},
						},
						Quantity:        2,
						FulfillmentType: constants.FulfillmentTypeManual,
					},
				},
			},
		},
	}

	payload := svc.buildOrderNotificationPayload(order, nil)

	itemsSummary := fmt.Sprintf("%v", payload["items_summary"])
	if !strings.Contains(itemsSummary, "Upstream Product / Node: SG x1 [Upstream]") {
		t.Fatalf("items_summary missing child upstream item detail: %s", itemsSummary)
	}
	if !strings.Contains(itemsSummary, "Manual Product / Cycle: 7 days x2 [Manual]") {
		t.Fatalf("items_summary missing child manual item detail: %s", itemsSummary)
	}

	fulfillmentSummary := fmt.Sprintf("%v", payload["fulfillment_items_summary"])
	if !strings.Contains(fulfillmentSummary, "Upstream Product / Node: SG x1 [Upstream]") {
		t.Fatalf("fulfillment_items_summary missing child upstream item detail: %s", fulfillmentSummary)
	}
	if !strings.Contains(fulfillmentSummary, "Manual Product / Cycle: 7 days x2 [Manual]") {
		t.Fatalf("fulfillment_items_summary missing child manual item detail: %s", fulfillmentSummary)
	}

	deliverySummary := fmt.Sprintf("%v", payload["delivery_summary"])
	if deliverySummary != "Total 2 items, auto 0, manual 1, upstream 1" {
		t.Fatalf("unexpected delivery_summary from child items: %s", deliverySummary)
	}
	if got := len(order.Items); got != 2 {
		t.Fatalf("expected parent order items to be filled from children, got %d", got)
	}
}

func TestBuildManualFulfillmentNotificationPayloadUsesGuestEmailAndPendingItems(t *testing.T) {
	svc := &PaymentService{}
	order := &models.Order{
		ID:         88,
		OrderNo:    "DJ202603230099",
		GuestEmail: "guest@example.com",
		Status:     constants.OrderStatusPaid,
		Currency:   "CNY",
		Items: []models.OrderItem{
			{
				TitleJSON:       models.JSON{"zh-CN": "自动商品"},
				SKUSnapshotJSON: models.JSON{"sku_code": "AUTO-001"},
				Quantity:        1,
				FulfillmentType: constants.FulfillmentTypeAuto,
			},
			{
				TitleJSON:       models.JSON{"zh-CN": "待处理商品"},
				SKUSnapshotJSON: models.JSON{"sku_code": "MANUAL-001"},
				Quantity:        3,
				FulfillmentType: constants.FulfillmentTypeManual,
			},
		},
	}
	parent := &models.Order{
		ID:      66,
		OrderNo: "DJ202603230001",
	}

	payload := svc.buildManualFulfillmentNotificationPayload(order, parent)

	if got := fmt.Sprintf("%v", payload["customer_email"]); got != "guest@example.com" {
		t.Fatalf("customer_email want guest@example.com got %s", got)
	}
	if got := fmt.Sprintf("%v", payload["customer_type"]); got != "guest" {
		t.Fatalf("customer_type want guest got %s", got)
	}
	if got := fmt.Sprintf("%v", payload["parent_order_no"]); got != "DJ202603230001" {
		t.Fatalf("parent_order_no want DJ202603230001 got %s", got)
	}

	fulfillmentSummary := fmt.Sprintf("%v", payload["fulfillment_items_summary"])
	if strings.Contains(fulfillmentSummary, "自动商品") {
		t.Fatalf("fulfillment_items_summary should not include auto item: %s", fulfillmentSummary)
	}
	if !strings.Contains(fulfillmentSummary, "待处理商品 / MANUAL-001 x3 [人工交付]") {
		t.Fatalf("fulfillment_items_summary missing pending item detail: %s", fulfillmentSummary)
	}
}

func TestNotificationCenterDefaultSettingIncludesRichOrderVariables(t *testing.T) {
	setting := NotificationCenterDefaultSetting()

	orderBody := setting.Templates.OrderPaidSuccess.ZHCN.Body
	if !strings.Contains(orderBody, "{{customer_email}}") || !strings.Contains(orderBody, "{{items_summary}}") {
		t.Fatalf("order paid template should include rich variables, got: %s", orderBody)
	}

	manualBody := setting.Templates.ManualFulfillmentPending.ZHCN.Body
	if !strings.Contains(manualBody, "{{fulfillment_items_summary}}") || !strings.Contains(manualBody, "{{delivery_summary}}") {
		t.Fatalf("manual fulfillment template should include fulfillment summary variables, got: %s", manualBody)
	}

	exceptionBody := setting.Templates.ExceptionAlert.ZHCN.Body
	if !strings.Contains(exceptionBody, "{{affected_items_summary}}") {
		t.Fatalf("exception alert template should include affected_items_summary, got: %s", exceptionBody)
	}
}

func TestPatchNotificationCenterSettingPersistsInventoryAlertConfig(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo)
	interval := 600
	ignored := []uint{9, 2, 9, 0}

	setting, err := svc.PatchNotificationCenterSetting(NotificationCenterSettingPatch{
		InventoryAlertIntervalSeconds: &interval,
		IgnoredProductIDs:             &ignored,
	})
	if err != nil {
		t.Fatalf("patch notification center setting failed: %v", err)
	}

	if setting.InventoryAlertIntervalSeconds != 600 {
		t.Fatalf("expected interval 600, got %d", setting.InventoryAlertIntervalSeconds)
	}
	if len(setting.IgnoredProductIDs) != 2 || setting.IgnoredProductIDs[0] != 9 || setting.IgnoredProductIDs[1] != 2 {
		t.Fatalf("unexpected ignored_product_ids: %#v", setting.IgnoredProductIDs)
	}

	stored, err := svc.GetNotificationCenterSetting()
	if err != nil {
		t.Fatalf("get notification center setting failed: %v", err)
	}
	if stored.InventoryAlertIntervalSeconds != 600 {
		t.Fatalf("stored interval want 600 got %d", stored.InventoryAlertIntervalSeconds)
	}
	if len(stored.IgnoredProductIDs) != 2 || stored.IgnoredProductIDs[0] != 9 || stored.IgnoredProductIDs[1] != 2 {
		t.Fatalf("stored ignored_product_ids mismatch: %#v", stored.IgnoredProductIDs)
	}
}

func TestPatchNotificationCenterSettingPersistsPaymentOrderAlertConfig(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo)
	interval := 900
	checkInterval := 7200

	setting, err := svc.PatchNotificationCenterSetting(NotificationCenterSettingPatch{
		PaymentOrderAlertIntervalSeconds: &interval,
		PaymentOrderAlertCheckSeconds:    &checkInterval,
	})
	if err != nil {
		t.Fatalf("patch notification center setting failed: %v", err)
	}

	if setting.PaymentOrderAlertIntervalSeconds != interval {
		t.Fatalf("expected payment order interval %d, got %d", interval, setting.PaymentOrderAlertIntervalSeconds)
	}
	if setting.PaymentOrderAlertCheckSeconds != checkInterval {
		t.Fatalf("expected payment order check interval %d, got %d", checkInterval, setting.PaymentOrderAlertCheckSeconds)
	}

	stored, err := svc.GetNotificationCenterSetting()
	if err != nil {
		t.Fatalf("get notification center setting failed: %v", err)
	}
	if stored.PaymentOrderAlertIntervalSeconds != interval {
		t.Fatalf("stored payment order interval want %d got %d", interval, stored.PaymentOrderAlertIntervalSeconds)
	}
	if stored.PaymentOrderAlertCheckSeconds != checkInterval {
		t.Fatalf("stored payment order check interval want %d got %d", checkInterval, stored.PaymentOrderAlertCheckSeconds)
	}
}

func TestBuildPaymentOrderAlertDispatchPayloadsUseDashboardThresholds(t *testing.T) {
	setting := NotificationCenterDefaultSetting()
	setting.DefaultLocale = constants.LocaleZhCN
	setting.PaymentOrderAlertIntervalSeconds = 600

	payloads := buildPaymentOrderAlertDispatchPayloads(
		setting,
		DashboardSetting{
			Alert: DashboardAlertSetting{
				PendingPaymentOrdersThreshold: 5,
				PaymentsFailedThreshold:       3,
			},
		},
		queue.NotificationDispatchPayload{
			EventType: constants.NotificationEventExceptionAlertCheck,
			BizType:   constants.NotificationBizTypeDashboardAlert,
			Data: map[string]interface{}{
				"source": "scheduler",
			},
		},
		repository.DashboardPaymentOrderAlertCountsRow{
			PendingPaymentOrders: 6,
			PaymentsFailed:       4,
		},
	)
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payment order alert payloads, got %d", len(payloads))
	}

	byType := make(map[string]queue.NotificationDispatchPayload, len(payloads))
	for _, payload := range payloads {
		if payload.EventType != constants.NotificationEventExceptionAlert {
			t.Fatalf("event type want exception_alert got %s", payload.EventType)
		}
		byType[fmt.Sprintf("%v", payload.Data["alert_type_key"])] = payload
	}

	pendingPayload, ok := byType[constants.NotificationAlertTypePendingOrders]
	if !ok {
		t.Fatal("expected pending payment alert payload")
	}
	if pendingPayload.Data["alert_value"] != "6" || pendingPayload.Data["alert_threshold"] != "5" {
		t.Fatalf("pending alert value/threshold mismatch: %#v", pendingPayload.Data)
	}
	pendingMessage := fmt.Sprintf("%v", pendingPayload.Data["message"])
	if !strings.Contains(pendingMessage, "待支付订单 6 笔") || !strings.Contains(pendingMessage, "10 分钟") {
		t.Fatalf("unexpected pending payment message: %s", pendingMessage)
	}

	failedPayload, ok := byType[constants.NotificationAlertTypePaymentsFailed]
	if !ok {
		t.Fatal("expected payment failed alert payload")
	}
	if failedPayload.Data["alert_value"] != "4" || failedPayload.Data["alert_threshold"] != "3" {
		t.Fatalf("payment failed alert value/threshold mismatch: %#v", failedPayload.Data)
	}
	failedMessage := fmt.Sprintf("%v", failedPayload.Data["message"])
	if !strings.Contains(failedMessage, "支付失败 4 笔") || !strings.Contains(failedMessage, "10 分钟") {
		t.Fatalf("unexpected payment failed message: %s", failedMessage)
	}

	payloads = buildPaymentOrderAlertDispatchPayloads(
		setting,
		DashboardSetting{
			Alert: DashboardAlertSetting{
				PendingPaymentOrdersThreshold: 5,
				PaymentsFailedThreshold:       3,
			},
		},
		queue.NotificationDispatchPayload{EventType: constants.NotificationEventExceptionAlertCheck},
		repository.DashboardPaymentOrderAlertCountsRow{PendingPaymentOrders: 4, PaymentsFailed: 2},
	)
	if len(payloads) != 0 {
		t.Fatalf("expected no payload below dashboard thresholds, got %d", len(payloads))
	}
}

func TestBuildInventoryAlertDispatchPayloadsIncludesSummaryAndIgnoreRules(t *testing.T) {
	setting := NotificationCenterDefaultSetting()
	setting.DefaultLocale = constants.LocaleEnUS
	setting.InventoryAlertIntervalSeconds = 900
	setting.IgnoredProductIDs = []uint{2}

	dashboardSetting := DashboardSetting{
		Alert: DashboardAlertSetting{
			LowStockThreshold:           5,
			OutOfStockProductsThreshold: 1,
		},
	}

	payloads := buildInventoryAlertDispatchPayloads(
		setting,
		dashboardSetting,
		queue.NotificationDispatchPayload{
			EventType: constants.NotificationEventExceptionAlertCheck,
			BizType:   constants.NotificationBizTypeDashboardAlert,
			Data: map[string]interface{}{
				"source": "scheduler",
			},
		},
		[]repository.DashboardInventoryAlertRow{
			{
				ProductID:         1,
				SKUID:             11,
				ProductTitleJSON:  models.JSON{"en-US": "Manual Product"},
				SKUCode:           "MANUAL-A",
				SKUSpecValuesJSON: models.JSON{"en-US": "Cycle: 30 days"},
				FulfillmentType:   constants.FulfillmentTypeManual,
				AlertType:         constants.NotificationAlertTypeLowStockProducts,
				AvailableStock:    3,
			},
			{
				ProductID:         2,
				SKUID:             21,
				ProductTitleJSON:  models.JSON{"en-US": "Ignored Product"},
				SKUCode:           "IGNORE-ME",
				SKUSpecValuesJSON: models.JSON{"en-US": "Region: SG"},
				FulfillmentType:   constants.FulfillmentTypeAuto,
				AlertType:         constants.NotificationAlertTypeOutOfStockProducts,
				AvailableStock:    0,
			},
			{
				ProductID:         3,
				SKUID:             31,
				ProductTitleJSON:  models.JSON{"en-US": "Auto Product"},
				SKUCode:           "AUTO-PRO",
				SKUSpecValuesJSON: models.JSON{"en-US": "Region: HK"},
				FulfillmentType:   constants.FulfillmentTypeAuto,
				AlertType:         constants.NotificationAlertTypeOutOfStockProducts,
				AvailableStock:    0,
			},
		},
	)

	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}

	var lowStockPayload queue.NotificationDispatchPayload
	var outOfStockPayload queue.NotificationDispatchPayload
	for _, item := range payloads {
		switch fmt.Sprintf("%v", item.Data["alert_type_key"]) {
		case constants.NotificationAlertTypeLowStockProducts:
			lowStockPayload = item
		case constants.NotificationAlertTypeOutOfStockProducts:
			outOfStockPayload = item
		}
	}

	lowSummary := fmt.Sprintf("%v", lowStockPayload.Data["affected_items_summary"])
	if !strings.Contains(lowSummary, "Manual Product / Cycle: 30 days") {
		t.Fatalf("low stock summary missing manual item: %s", lowSummary)
	}
	if !strings.Contains(fmt.Sprintf("%v", lowStockPayload.Data["message"]), "15 minutes") {
		t.Fatalf("low stock message should include interval text, got: %v", lowStockPayload.Data["message"])
	}

	outSummary := fmt.Sprintf("%v", outOfStockPayload.Data["affected_items_summary"])
	if !strings.Contains(outSummary, "Auto Product / Region: HK") {
		t.Fatalf("out of stock summary missing auto item: %s", outSummary)
	}
	if strings.Contains(outSummary, "Ignored Product") {
		t.Fatalf("ignored product should not appear in summary: %s", outSummary)
	}
	if got := fmt.Sprintf("%v", outOfStockPayload.Data["alert_value"]); got != "1" {
		t.Fatalf("out of stock alert_value want 1 got %s", got)
	}
	if got := fmt.Sprintf("%v", outOfStockPayload.Data["affected_items_count"]); got != "1" {
		t.Fatalf("affected_items_count want 1 got %s", got)
	}
}

func TestBuildNotificationTestVariablesIncludesSceneSpecificSamples(t *testing.T) {
	orderVars := buildNotificationTestVariables(constants.NotificationEventOrderPaidSuccess, constants.LocaleEnUS)
	if !strings.Contains(fmt.Sprintf("%v", orderVars["items_summary"]), "Netflix Annual") {
		t.Fatalf("order test variables should include order item summary, got: %v", orderVars["items_summary"])
	}
	if got := fmt.Sprintf("%v", orderVars["payment_channel"]); got != "epay/alipay" {
		t.Fatalf("payment_channel want epay/alipay got %s", got)
	}

	alertVars := buildNotificationTestVariables(constants.NotificationEventExceptionAlert, constants.LocaleEnUS)
	if !strings.Contains(fmt.Sprintf("%v", alertVars["affected_items_summary"]), "Remaining 1") {
		t.Fatalf("exception test variables should include inventory summary, got: %v", alertVars["affected_items_summary"])
	}
	if !strings.Contains(fmt.Sprintf("%v", alertVars["message"]), "30 minutes") {
		t.Fatalf("exception test message should include interval wording, got: %v", alertVars["message"])
	}
}

func TestResolveInventoryAlertTypeKey(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			name: "use code directly",
			data: map[string]interface{}{
				"alert_type_key": constants.NotificationAlertTypeLowStockProducts,
				"alert_type":     "低库存商品",
			},
			want: constants.NotificationAlertTypeLowStockProducts,
		},
		{
			name: "fallback from zh label",
			data: map[string]interface{}{
				"alert_type": "低库存商品",
			},
			want: constants.NotificationAlertTypeLowStockProducts,
		},
		{
			name: "fallback from zh-tw label",
			data: map[string]interface{}{
				"alert_type": "低庫存商品",
			},
			want: constants.NotificationAlertTypeLowStockProducts,
		},
		{
			name: "fallback from en label",
			data: map[string]interface{}{
				"alert_type": "Out of Stock",
			},
			want: constants.NotificationAlertTypeOutOfStockProducts,
		},
		{
			name: "unknown type",
			data: map[string]interface{}{
				"alert_type": "something_else",
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveInventoryAlertTypeKey(tc.data)
			if got != tc.want {
				t.Fatalf("resolveInventoryAlertTypeKey want %q got %q", tc.want, got)
			}
		})
	}
}
