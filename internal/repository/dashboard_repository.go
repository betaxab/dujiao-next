package repository

import (
	"fmt"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// DashboardRepository 仪表盘聚合查询接口
// 说明：仅聚合统计数据，不承载业务规则。
type DashboardRepository interface {
	GetOverview(startAt, endAt time.Time) (DashboardOverviewRow, error)
	GetOrderTrends(startAt, endAt time.Time) ([]DashboardOrderTrendRow, error)
	GetPaymentTrends(startAt, endAt time.Time) ([]DashboardPaymentTrendRow, error)
	GetStockStats(lowStockThreshold int64) (DashboardStockStatsRow, error)
	GetTopProducts(startAt, endAt time.Time, limit int) ([]DashboardProductRankingRow, error)
	GetTopChannels(startAt, endAt time.Time, limit int) ([]DashboardChannelRankingRow, error)
}

// DashboardOverviewRow 仪表盘总览原始统计结果
type DashboardOverviewRow struct {
	OrdersTotal          int64
	PaidOrders           int64
	CompletedOrders      int64
	PendingPaymentOrders int64
	ProcessingOrders     int64
	GMVPaid              float64
	PaymentsTotal        int64
	PaymentsSuccess      int64
	PaymentsFailed       int64
	NewUsers             int64
	ActiveProducts       int64
	Currency             string
}

// DashboardOrderTrendRow 订单趋势统计
type DashboardOrderTrendRow struct {
	Day         string
	OrdersTotal int64
	OrdersPaid  int64
}

// DashboardPaymentTrendRow 支付趋势统计
type DashboardPaymentTrendRow struct {
	Day             string
	PaymentsSuccess int64
	PaymentsFailed  int64
	GMVPaid         float64
}

// DashboardStockStatsRow 库存统计
type DashboardStockStatsRow struct {
	OutOfStockProducts   int64
	LowStockProducts     int64
	AutoAvailableSecrets int64
	ManualAvailableUnits int64
}

// DashboardProductRankingRow 商品排行原始行
type DashboardProductRankingRow struct {
	ProductID  uint
	Title      string
	PaidOrders int64
	Quantity   int64
	PaidAmount float64
}

// DashboardChannelRankingRow 渠道排行原始行
type DashboardChannelRankingRow struct {
	ChannelID     uint
	ChannelName   string
	ProviderType  string
	ChannelType   string
	SuccessCount  int64
	FailedCount   int64
	SuccessAmount float64
}

// GormDashboardRepository GORM 仪表盘聚合实现
type GormDashboardRepository struct {
	db *gorm.DB
}

// NewDashboardRepository 创建仪表盘仓库
func NewDashboardRepository(db *gorm.DB) *GormDashboardRepository {
	return &GormDashboardRepository{db: db}
}

func paidOrderStatuses() []string {
	return []string{
		constants.OrderStatusPaid,
		constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered,
		constants.OrderStatusDelivered,
		constants.OrderStatusCompleted,
	}
}

func onlinePaymentBase(db *gorm.DB, startAt, endAt time.Time) *gorm.DB {
	return db.Model(&models.Payment{}).
		Where("created_at >= ? AND created_at < ? AND provider_type <> ?", startAt, endAt, constants.PaymentProviderWallet)
}

// GetOverview 获取总览统计
func (r *GormDashboardRepository) GetOverview(startAt, endAt time.Time) (DashboardOverviewRow, error) {
	result := DashboardOverviewRow{}

	orderBase := func() *gorm.DB {
		return r.db.Model(&models.Order{}).
			Where("parent_id IS NULL AND created_at >= ? AND created_at < ?", startAt, endAt)
	}

	if err := orderBase().Count(&result.OrdersTotal).Error; err != nil {
		return result, err
	}

	paidStatuses := paidOrderStatuses()
	if err := orderBase().Where("status IN ?", paidStatuses).Count(&result.PaidOrders).Error; err != nil {
		return result, err
	}
	if err := orderBase().Where("status = ?", constants.OrderStatusCompleted).Count(&result.CompletedOrders).Error; err != nil {
		return result, err
	}
	if err := orderBase().Where("status = ?", constants.OrderStatusPendingPayment).Count(&result.PendingPaymentOrders).Error; err != nil {
		return result, err
	}
	processingStatuses := []string{
		constants.OrderStatusPaid,
		constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered,
		constants.OrderStatusDelivered,
	}
	if err := orderBase().Where("status IN ?", processingStatuses).Count(&result.ProcessingOrders).Error; err != nil {
		return result, err
	}

	if err := r.db.Model(&models.Order{}).
		Where("parent_id IS NULL AND paid_at IS NOT NULL AND paid_at >= ? AND paid_at < ? AND status IN ?", startAt, endAt, paidStatuses).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&result.GMVPaid).Error; err != nil {
		return result, err
	}

	paymentBase := func() *gorm.DB {
		return onlinePaymentBase(r.db, startAt, endAt)
	}
	if err := paymentBase().Count(&result.PaymentsTotal).Error; err != nil {
		return result, err
	}
	if err := paymentBase().Where("status = ?", constants.PaymentStatusSuccess).Count(&result.PaymentsSuccess).Error; err != nil {
		return result, err
	}
	if err := paymentBase().Where("status = ?", constants.PaymentStatusFailed).Count(&result.PaymentsFailed).Error; err != nil {
		return result, err
	}

	if err := r.db.Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", startAt, endAt).
		Count(&result.NewUsers).Error; err != nil {
		return result, err
	}

	if err := r.db.Model(&models.Product{}).
		Where("is_active = ?", true).
		Count(&result.ActiveProducts).Error; err != nil {
		return result, err
	}

	_ = r.db.Model(&models.Order{}).
		Where("parent_id IS NULL AND created_at >= ? AND created_at < ? AND currency <> ''", startAt, endAt).
		Order("id DESC").
		Limit(1).
		Pluck("currency", &result.Currency).Error

	return result, nil
}

// GetOrderTrends 获取订单趋势
func (r *GormDashboardRepository) GetOrderTrends(startAt, endAt time.Time) ([]DashboardOrderTrendRow, error) {
	type totalRow struct {
		Day   string
		Total int64
	}
	type paidRow struct {
		Day  string
		Paid int64
	}

	var totals []totalRow
	dayExpr := "CAST(date(created_at) AS TEXT)"
	if err := r.db.Model(&models.Order{}).
		Select(fmt.Sprintf("%s as day, COUNT(*) as total", dayExpr)).
		Where("parent_id IS NULL AND created_at >= ? AND created_at < ?", startAt, endAt).
		Group(dayExpr).
		Order("day asc").
		Scan(&totals).Error; err != nil {
		return nil, err
	}

	var paids []paidRow
	if err := r.db.Model(&models.Order{}).
		Select(fmt.Sprintf("%s as day, COUNT(*) as paid", dayExpr)).
		Where("parent_id IS NULL AND created_at >= ? AND created_at < ? AND status IN ?", startAt, endAt, paidOrderStatuses()).
		Group(dayExpr).
		Order("day asc").
		Scan(&paids).Error; err != nil {
		return nil, err
	}

	paidMap := make(map[string]int64, len(paids))
	for _, item := range paids {
		paidMap[item.Day] = item.Paid
	}

	result := make([]DashboardOrderTrendRow, 0, len(totals))
	for _, item := range totals {
		result = append(result, DashboardOrderTrendRow{
			Day:         item.Day,
			OrdersTotal: item.Total,
			OrdersPaid:  paidMap[item.Day],
		})
	}
	return result, nil
}

// GetPaymentTrends 获取支付趋势
func (r *GormDashboardRepository) GetPaymentTrends(startAt, endAt time.Time) ([]DashboardPaymentTrendRow, error) {
	type countRow struct {
		Day   string
		Total int64
	}
	type amountRow struct {
		Day   string
		Total float64
	}

	dayExpr := "CAST(date(created_at) AS TEXT)"

	var successRows []countRow
	if err := onlinePaymentBase(r.db, startAt, endAt).
		Select(fmt.Sprintf("%s as day, COUNT(*) as total", dayExpr)).
		Where("status = ?", constants.PaymentStatusSuccess).
		Group(dayExpr).
		Order("day asc").
		Scan(&successRows).Error; err != nil {
		return nil, err
	}

	var failedRows []countRow
	if err := onlinePaymentBase(r.db, startAt, endAt).
		Select(fmt.Sprintf("%s as day, COUNT(*) as total", dayExpr)).
		Where("status = ?", constants.PaymentStatusFailed).
		Group(dayExpr).
		Order("day asc").
		Scan(&failedRows).Error; err != nil {
		return nil, err
	}

	var amountRows []amountRow
	if err := onlinePaymentBase(r.db, startAt, endAt).
		Select(fmt.Sprintf("%s as day, COALESCE(SUM(amount), 0) as total", dayExpr)).
		Where("status = ?", constants.PaymentStatusSuccess).
		Group(dayExpr).
		Order("day asc").
		Scan(&amountRows).Error; err != nil {
		return nil, err
	}

	successMap := make(map[string]int64, len(successRows))
	for _, item := range successRows {
		successMap[item.Day] = item.Total
	}
	failedMap := make(map[string]int64, len(failedRows))
	for _, item := range failedRows {
		failedMap[item.Day] = item.Total
	}
	amountMap := make(map[string]float64, len(amountRows))
	for _, item := range amountRows {
		amountMap[item.Day] = item.Total
	}

	seen := make(map[string]struct{}, len(successRows)+len(failedRows)+len(amountRows))
	result := make([]DashboardPaymentTrendRow, 0)
	push := func(day string) {
		if day == "" {
			return
		}
		if _, ok := seen[day]; ok {
			return
		}
		seen[day] = struct{}{}
		result = append(result, DashboardPaymentTrendRow{
			Day:             day,
			PaymentsSuccess: successMap[day],
			PaymentsFailed:  failedMap[day],
			GMVPaid:         amountMap[day],
		})
	}
	for _, item := range successRows {
		push(item.Day)
	}
	for _, item := range failedRows {
		push(item.Day)
	}
	for _, item := range amountRows {
		push(item.Day)
	}

	return result, nil
}

// GetStockStats 获取库存总览统计
func (r *GormDashboardRepository) GetStockStats(lowStockThreshold int64) (DashboardStockStatsRow, error) {
	result := DashboardStockStatsRow{}

	type stockProductRow struct {
		ID                uint
		FulfillmentType   string
		ManualStockTotal  int64
		ManualStockLocked int64
		ManualStockSold   int64
	}
	var products []stockProductRow
	if err := r.db.Model(&models.Product{}).
		Select("id, fulfillment_type, manual_stock_total, manual_stock_locked, manual_stock_sold").
		Where("is_active = ?", true).
		Scan(&products).Error; err != nil {
		return result, err
	}

	autoProductIDs := make([]uint, 0)
	for _, product := range products {
		if product.FulfillmentType == constants.FulfillmentTypeAuto {
			autoProductIDs = append(autoProductIDs, product.ID)
			continue
		}
		if product.FulfillmentType != constants.FulfillmentTypeManual {
			continue
		}
		if product.ManualStockTotal <= 0 {
			continue
		}
		available := product.ManualStockTotal - product.ManualStockLocked - product.ManualStockSold
		if available < 0 {
			available = 0
		}
		result.ManualAvailableUnits += available
		if available <= 0 {
			result.OutOfStockProducts += 1
		} else if available <= lowStockThreshold {
			result.LowStockProducts += 1
		}
	}

	if len(autoProductIDs) == 0 {
		return result, nil
	}

	type countRow struct {
		ProductID uint
		Total     int64
	}
	var rows []countRow
	if err := r.db.Model(&models.CardSecret{}).
		Select("product_id, COUNT(*) as total").
		Where("product_id IN ? AND status = ?", autoProductIDs, models.CardSecretStatusAvailable).
		Group("product_id").
		Scan(&rows).Error; err != nil {
		return result, err
	}

	availableMap := make(map[uint]int64, len(rows))
	for _, item := range rows {
		availableMap[item.ProductID] = item.Total
		result.AutoAvailableSecrets += item.Total
	}

	for _, productID := range autoProductIDs {
		available := availableMap[productID]
		if available <= 0 {
			result.OutOfStockProducts += 1
		} else if available <= lowStockThreshold {
			result.LowStockProducts += 1
		}
	}

	return result, nil
}

// GetTopProducts 获取商品排行榜
func (r *GormDashboardRepository) GetTopProducts(startAt, endAt time.Time, limit int) ([]DashboardProductRankingRow, error) {
	if limit <= 0 {
		limit = 5
	}
	rows := make([]DashboardProductRankingRow, 0)
	titleExpr := localizedJSONCoalesceExpr(r.db, "order_items.title_json")
	if err := r.db.Model(&models.OrderItem{}).
		Select(fmt.Sprintf(`
			order_items.product_id as product_id,
			%s as title,
			COUNT(DISTINCT order_items.order_id) as paid_orders,
			COALESCE(SUM(order_items.quantity), 0) as quantity,
			COALESCE(SUM(order_items.total_price - order_items.coupon_discount - order_items.promotion_discount), 0) as paid_amount
		`, titleExpr)).
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.created_at >= ? AND orders.created_at < ? AND orders.status IN ?", startAt, endAt, paidOrderStatuses()).
		Group("order_items.product_id, title").
		Order("paid_amount DESC, quantity DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetTopChannels 获取支付渠道排行榜
func (r *GormDashboardRepository) GetTopChannels(startAt, endAt time.Time, limit int) ([]DashboardChannelRankingRow, error) {
	if limit <= 0 {
		limit = 5
	}
	rows := make([]DashboardChannelRankingRow, 0)
	if err := r.db.Model(&models.Payment{}).
		Select(`
			payments.channel_id as channel_id,
			COALESCE(payment_channels.name, '') as channel_name,
			payments.provider_type as provider_type,
			payments.channel_type as channel_type,
			SUM(CASE WHEN payments.status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN payments.status = 'failed' THEN 1 ELSE 0 END) as failed_count,
			COALESCE(SUM(CASE WHEN payments.status = 'success' THEN payments.amount ELSE 0 END), 0) as success_amount
		`).
		Joins("LEFT JOIN payment_channels ON payment_channels.id = payments.channel_id").
		Where("payments.created_at >= ? AND payments.created_at < ? AND payments.provider_type <> ?", startAt, endAt, constants.PaymentProviderWallet).
		Group("payments.channel_id, payment_channels.name, payments.provider_type, payments.channel_type").
		Order("success_amount DESC, success_count DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
