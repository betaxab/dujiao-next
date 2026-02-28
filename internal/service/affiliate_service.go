package service

import (
	"crypto/rand"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	affiliateCodeLength        = 8
	affiliateSplitTypePrefix   = "sp"
	affiliateAttributionWindow = 30 * 24 * time.Hour
	affiliateClickDedupeWindow = 10 * time.Minute
)

// AffiliateService 推广返利业务服务
type AffiliateService struct {
	repo           repository.AffiliateRepository
	userRepo       repository.UserRepository
	orderRepo      repository.OrderRepository
	productRepo    repository.ProductRepository
	settingService *SettingService
}

// NewAffiliateService 创建推广返利服务
func NewAffiliateService(
	repo repository.AffiliateRepository,
	userRepo repository.UserRepository,
	orderRepo repository.OrderRepository,
	productRepo repository.ProductRepository,
	settingService *SettingService,
) *AffiliateService {
	return &AffiliateService{
		repo:           repo,
		userRepo:       userRepo,
		orderRepo:      orderRepo,
		productRepo:    productRepo,
		settingService: settingService,
	}
}

// AffiliateTrackClickInput 推广点击记录输入
type AffiliateTrackClickInput struct {
	AffiliateCode string
	VisitorKey    string
	LandingPath   string
	Referrer      string
	ClientIP      string
	UserAgent     string
}

// AffiliateDashboard 推广用户中心数据
type AffiliateDashboard struct {
	Opened              bool         `json:"opened"`
	AffiliateCode       string       `json:"affiliate_code"`
	PromotionPath       string       `json:"promotion_path"`
	ClickCount          int64        `json:"click_count"`
	ValidOrderCount     int64        `json:"valid_order_count"`
	ConversionRate      float64      `json:"conversion_rate"`
	PendingCommission   models.Money `json:"pending_commission"`
	AvailableCommission models.Money `json:"available_commission"`
	WithdrawnCommission models.Money `json:"withdrawn_commission"`
}

// AffiliateStats 推广统计数据
type AffiliateStats struct {
	ClickCount          int64
	ValidOrderCount     int64
	ConversionRate      float64
	PendingCommission   models.Money
	AvailableCommission models.Money
	WithdrawnCommission models.Money
}

// AffiliateWithdrawApplyInput 提现申请输入
type AffiliateWithdrawApplyInput struct {
	Amount  decimal.Decimal
	Channel string
	Account string
}

// AffiliateAdminUserItem 后台推广用户列表项
type AffiliateAdminUserItem struct {
	Profile models.AffiliateProfile `json:"profile"`
	Stats   AffiliateStats          `json:"stats"`
}

// AffiliateAdminCommissionListFilter 后台佣金列表过滤
type AffiliateAdminCommissionListFilter struct {
	Page               int
	PageSize           int
	AffiliateProfileID uint
	OrderNo            string
	Status             string
	Keyword            string
}

// AffiliateAdminWithdrawListFilter 后台提现列表过滤
type AffiliateAdminWithdrawListFilter struct {
	Page               int
	PageSize           int
	AffiliateProfileID uint
	Status             string
	Keyword            string
}

// UpdateAffiliateProfileStatus 管理端更新返利用户状态
func (s *AffiliateService) UpdateAffiliateProfileStatus(profileID uint, rawStatus string) (*models.AffiliateProfile, error) {
	if profileID == 0 || s.repo == nil {
		return nil, ErrNotFound
	}
	nextStatus := strings.TrimSpace(rawStatus)
	if nextStatus != constants.AffiliateProfileStatusActive && nextStatus != constants.AffiliateProfileStatusDisabled {
		return nil, ErrAffiliateProfileStatusInvalid
	}

	profile, err := s.repo.GetProfileByID(profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(profile.Status) == nextStatus {
		return profile, nil
	}
	if err := s.repo.UpdateProfileStatus(profileID, nextStatus, time.Now()); err != nil {
		return nil, err
	}
	return s.repo.GetProfileByID(profileID)
}

// BatchUpdateAffiliateProfileStatus 管理端批量更新返利用户状态
func (s *AffiliateService) BatchUpdateAffiliateProfileStatus(profileIDs []uint, rawStatus string) (int64, error) {
	if s.repo == nil {
		return 0, ErrNotFound
	}
	nextStatus := strings.TrimSpace(rawStatus)
	if nextStatus != constants.AffiliateProfileStatusActive && nextStatus != constants.AffiliateProfileStatusDisabled {
		return 0, ErrAffiliateProfileStatusInvalid
	}
	normalizedIDs := normalizeAffiliateProfileIDs(profileIDs)
	if len(normalizedIDs) == 0 {
		return 0, nil
	}
	return s.repo.BatchUpdateProfileStatus(normalizedIDs, nextStatus, time.Now())
}

// OpenAffiliate 为用户开通推广返利
func (s *AffiliateService) OpenAffiliate(userID uint) (*models.AffiliateProfile, error) {
	if userID == 0 {
		return nil, ErrUserDisabled
	}
	if s.repo == nil || s.userRepo == nil {
		return nil, ErrNotFound
	}
	setting, err := s.settingService.GetAffiliateSetting()
	if err != nil {
		return nil, err
	}
	if !setting.Enabled {
		return nil, ErrAffiliateDisabled
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(user.Status) == constants.UserStatusDisabled {
		return nil, ErrUserDisabled
	}

	existing, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	const maxRetry = 8
	for i := 0; i < maxRetry; i++ {
		code, genErr := generateAffiliateCode()
		if genErr != nil {
			return nil, genErr
		}
		profile := &models.AffiliateProfile{
			UserID:        userID,
			AffiliateCode: code,
			Status:        constants.AffiliateProfileStatusActive,
		}
		if err := s.repo.CreateProfile(profile); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return nil, err
		}
		created, err := s.repo.GetProfileByID(profile.ID)
		if err != nil {
			return nil, err
		}
		if created != nil {
			return created, nil
		}
		return profile, nil
	}
	return nil, ErrAffiliateCodeInvalid
}

// ResolveOrderAffiliateSnapshot 解析下单归因快照（最近30天最后一次有效点击优先）
func (s *AffiliateService) ResolveOrderAffiliateSnapshot(userID uint, rawCode, rawVisitorKey string) (*uint, string, error) {
	code := normalizeAffiliateCode(rawCode)
	visitorKey := strings.TrimSpace(rawVisitorKey)
	if s.repo == nil {
		return nil, "", nil
	}

	setting, err := s.settingService.GetAffiliateSetting()
	if err != nil {
		return nil, "", err
	}
	if !setting.Enabled {
		return nil, "", nil
	}

	if visitorKey != "" {
		profile, err := s.repo.GetLatestActiveProfileByVisitorKey(visitorKey, time.Now().Add(-affiliateAttributionWindow))
		if err != nil {
			return nil, "", err
		}
		if profile != nil {
			if userID > 0 && profile.UserID == userID {
				return nil, "", nil
			}
			profileID := profile.ID
			return &profileID, profile.AffiliateCode, nil
		}
	}

	if code == "" {
		return nil, "", nil
	}

	profile, err := s.repo.GetProfileByCode(code)
	if err != nil {
		return nil, "", err
	}
	if profile == nil || strings.TrimSpace(profile.Status) != constants.AffiliateProfileStatusActive {
		return nil, "", nil
	}
	if userID > 0 && profile.UserID == userID {
		return nil, "", nil
	}

	profileID := profile.ID
	return &profileID, profile.AffiliateCode, nil
}

// TrackClick 记录推广点击
func (s *AffiliateService) TrackClick(input AffiliateTrackClickInput) error {
	if s.repo == nil {
		return nil
	}
	code := normalizeAffiliateCode(input.AffiliateCode)
	if code == "" {
		return nil
	}
	setting, err := s.settingService.GetAffiliateSetting()
	if err != nil {
		return err
	}
	if !setting.Enabled {
		return nil
	}
	profile, err := s.repo.GetProfileByCode(code)
	if err != nil {
		return err
	}
	if profile == nil || strings.TrimSpace(profile.Status) != constants.AffiliateProfileStatusActive {
		return nil
	}
	visitorKey := strings.TrimSpace(input.VisitorKey)
	landingPath := strings.TrimSpace(input.LandingPath)
	if visitorKey != "" {
		duplicated, err := s.repo.HasRecentClick(profile.ID, visitorKey, landingPath, time.Now().Add(-affiliateClickDedupeWindow))
		if err != nil {
			return err
		}
		if duplicated {
			return nil
		}
	}

	click := &models.AffiliateClick{
		AffiliateProfileID: profile.ID,
		VisitorKey:         visitorKey,
		LandingPath:        landingPath,
		Referrer:           strings.TrimSpace(input.Referrer),
		ClientIP:           strings.TrimSpace(input.ClientIP),
		UserAgent:          strings.TrimSpace(input.UserAgent),
		CreatedAt:          time.Now(),
	}
	return s.repo.CreateClick(click)
}

// HandleOrderPaid 处理订单支付成功后的佣金生成
func (s *AffiliateService) HandleOrderPaid(orderID uint) error {
	if orderID == 0 || s.repo == nil || s.orderRepo == nil {
		return nil
	}
	setting, err := s.settingService.GetAffiliateSetting()
	if err != nil {
		return err
	}
	if !setting.Enabled || setting.CommissionRate <= 0 {
		return nil
	}

	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return err
	}
	if order == nil {
		return nil
	}
	profile, err := s.resolveAffiliateProfileForOrder(order)
	if err != nil {
		return err
	}
	if profile == nil {
		return nil
	}
	if strings.TrimSpace(profile.Status) != constants.AffiliateProfileStatusActive {
		return nil
	}
	if order.UserID > 0 && profile.UserID == order.UserID {
		return nil
	}

	commissionType := constants.AffiliateCommissionTypeOrder
	existing, err := s.repo.GetCommissionByOrderAndProfile(order.ID, profile.ID, commissionType)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	baseAmount, err := s.calculateCommissionBaseAmount(order)
	if err != nil {
		return err
	}
	if baseAmount.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	rate := decimal.NewFromFloat(setting.CommissionRate).Round(2)
	commissionAmount := baseAmount.Mul(rate).Div(decimal.NewFromInt(100)).Round(2)
	if commissionAmount.LessThanOrEqual(decimal.Zero) {
		return nil
	}

	paidAt := time.Now()
	if order.PaidAt != nil {
		paidAt = *order.PaidAt
	}
	status := constants.AffiliateCommissionStatusPendingConfirm
	var confirmAt *time.Time
	var availableAt *time.Time
	if setting.ConfirmDays <= 0 {
		status = constants.AffiliateCommissionStatusAvailable
		availableAt = &paidAt
	} else {
		t := paidAt.Add(time.Duration(setting.ConfirmDays) * 24 * time.Hour)
		confirmAt = &t
	}

	commission := &models.AffiliateCommission{
		AffiliateProfileID: profile.ID,
		OrderID:            order.ID,
		CommissionType:     commissionType,
		BaseAmount:         models.NewMoneyFromDecimal(baseAmount),
		RatePercent:        models.NewMoneyFromDecimal(rate),
		CommissionAmount:   models.NewMoneyFromDecimal(commissionAmount),
		Status:             status,
		ConfirmAt:          confirmAt,
		AvailableAt:        availableAt,
	}
	return s.repo.CreateCommission(commission)
}

// ConfirmDueCommissions 将到期佣金转可提现
func (s *AffiliateService) ConfirmDueCommissions(now time.Time) error {
	if s.repo == nil {
		return nil
	}
	_, err := s.repo.MarkPendingCommissionsAvailable(now, now)
	return err
}

// HandleOrderCanceled 处理订单取消/退款后的佣金逆向
func (s *AffiliateService) HandleOrderCanceled(orderID uint, reason string) error {
	if orderID == 0 || s.repo == nil {
		return nil
	}
	rows, err := s.repo.ListCommissionsByOrder(orderID, []string{
		constants.AffiliateCommissionStatusPendingConfirm,
		constants.AffiliateCommissionStatusAvailable,
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	now := time.Now()
	reasonText := strings.TrimSpace(reason)
	if reasonText == "" {
		reasonText = "order_canceled"
	}
	for i := range rows {
		item := rows[i]
		if item.WithdrawRequestID != nil {
			// 已进入提现流程，按业务规则不影响用户提现。
			continue
		}
		item.Status = constants.AffiliateCommissionStatusRejected
		item.InvalidReason = reasonText
		item.UpdatedAt = now
		if err := s.repo.UpdateCommission(&item); err != nil {
			return err
		}
	}
	return nil
}

// HandleOrderRefundedTx 在事务内处理订单退款后的佣金回滚
func (s *AffiliateService) HandleOrderRefundedTx(
	tx *gorm.DB,
	order *models.Order,
	refundDelta decimal.Decimal,
	refundedBefore decimal.Decimal,
	reason string,
) error {
	if tx == nil || order == nil || order.ID == 0 || s.repo == nil {
		return nil
	}
	delta := refundDelta.Round(2)
	if delta.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	totalAmount := order.TotalAmount.Decimal.Round(2)
	if totalAmount.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	before := refundedBefore.Round(2)
	if before.LessThan(decimal.Zero) {
		before = decimal.Zero
	}
	if before.GreaterThan(totalAmount) {
		before = totalAmount
	}
	remaining := totalAmount.Sub(before).Round(2)
	if remaining.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	if delta.GreaterThan(remaining) {
		delta = remaining
	}

	repoTx := s.repo.WithTx(tx)
	rows, err := repoTx.ListCommissionsByOrderForUpdate(order.ID, []string{
		constants.AffiliateCommissionStatusPendingConfirm,
		constants.AffiliateCommissionStatusAvailable,
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	now := time.Now()
	reasonText := strings.TrimSpace(reason)
	if reasonText == "" {
		reasonText = "order_refunded"
	}
	for i := range rows {
		item := rows[i]
		if item.WithdrawRequestID != nil {
			// 已进入提现流程，按业务规则不影响用户提现。
			continue
		}

		currentCommission := item.CommissionAmount.Decimal.Round(2)
		if currentCommission.LessThanOrEqual(decimal.Zero) {
			item.Status = constants.AffiliateCommissionStatusRejected
			item.InvalidReason = reasonText
			item.ConfirmAt = nil
			item.AvailableAt = nil
			item.UpdatedAt = now
			if err := repoTx.UpdateCommission(&item); err != nil {
				return err
			}
			continue
		}

		// 按“本次退款金额 / 当前剩余未退款金额”比例扣减当前佣金，避免多次退款时重复放大扣减。
		deduct := currentCommission.Mul(delta).Div(remaining).Round(2)
		nextCommission := currentCommission.Sub(deduct).Round(2)
		if nextCommission.LessThan(decimal.Zero) {
			nextCommission = decimal.Zero
		}
		currentBase := item.BaseAmount.Decimal.Round(2)
		nextBase := currentBase
		if currentBase.GreaterThan(decimal.Zero) {
			baseDeduct := currentBase.Mul(delta).Div(remaining).Round(2)
			nextBase = currentBase.Sub(baseDeduct).Round(2)
			if nextBase.LessThan(decimal.Zero) {
				nextBase = decimal.Zero
			}
		}

		item.CommissionAmount = models.NewMoneyFromDecimal(nextCommission)
		item.BaseAmount = models.NewMoneyFromDecimal(nextBase)
		item.UpdatedAt = now
		if nextCommission.LessThanOrEqual(decimal.Zero) {
			item.Status = constants.AffiliateCommissionStatusRejected
			item.InvalidReason = reasonText
			item.ConfirmAt = nil
			item.AvailableAt = nil
		}
		if err := repoTx.UpdateCommission(&item); err != nil {
			return err
		}
	}
	return nil
}

// GetUserDashboard 获取用户返利中心数据
func (s *AffiliateService) GetUserDashboard(userID uint) (AffiliateDashboard, error) {
	dashboard := AffiliateDashboard{
		Opened:              false,
		PendingCommission:   models.NewMoneyFromDecimal(decimal.Zero),
		AvailableCommission: models.NewMoneyFromDecimal(decimal.Zero),
		WithdrawnCommission: models.NewMoneyFromDecimal(decimal.Zero),
	}
	if userID == 0 || s.repo == nil {
		return dashboard, nil
	}
	profile, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return dashboard, err
	}
	if profile == nil {
		return dashboard, nil
	}

	stats, err := s.buildProfileStats(profile.ID)
	if err != nil {
		return dashboard, err
	}
	dashboard.Opened = true
	dashboard.AffiliateCode = profile.AffiliateCode
	dashboard.PromotionPath = "/?aff=" + profile.AffiliateCode
	dashboard.ClickCount = stats.ClickCount
	dashboard.ValidOrderCount = stats.ValidOrderCount
	dashboard.ConversionRate = stats.ConversionRate
	dashboard.PendingCommission = stats.PendingCommission
	dashboard.AvailableCommission = stats.AvailableCommission
	dashboard.WithdrawnCommission = stats.WithdrawnCommission
	return dashboard, nil
}

// ListUserCommissions 查询用户佣金记录
func (s *AffiliateService) ListUserCommissions(userID uint, page, pageSize int, status string) ([]models.AffiliateCommission, int64, error) {
	if userID == 0 || s.repo == nil {
		return []models.AffiliateCommission{}, 0, nil
	}
	profile, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, 0, err
	}
	if profile == nil {
		return []models.AffiliateCommission{}, 0, nil
	}
	return s.repo.ListCommissions(repository.AffiliateCommissionListFilter{
		Page:               page,
		PageSize:           pageSize,
		AffiliateProfileID: profile.ID,
		Status:             strings.TrimSpace(status),
	})
}

// ListUserWithdraws 查询用户提现记录
func (s *AffiliateService) ListUserWithdraws(userID uint, page, pageSize int, status string) ([]models.AffiliateWithdrawRequest, int64, error) {
	if userID == 0 || s.repo == nil {
		return []models.AffiliateWithdrawRequest{}, 0, nil
	}
	profile, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, 0, err
	}
	if profile == nil {
		return []models.AffiliateWithdrawRequest{}, 0, nil
	}
	return s.repo.ListWithdraws(repository.AffiliateWithdrawListFilter{
		Page:               page,
		PageSize:           pageSize,
		AffiliateProfileID: profile.ID,
		Status:             strings.TrimSpace(status),
	})
}

// ApplyWithdraw 用户提交提现申请
func (s *AffiliateService) ApplyWithdraw(userID uint, input AffiliateWithdrawApplyInput) (*models.AffiliateWithdrawRequest, error) {
	if userID == 0 || s.repo == nil {
		return nil, ErrAffiliateNotOpened
	}
	setting, err := s.settingService.GetAffiliateSetting()
	if err != nil {
		return nil, err
	}
	if !setting.Enabled {
		return nil, ErrAffiliateDisabled
	}

	amount := input.Amount.Round(2)
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrAffiliateWithdrawAmountInvalid
	}
	minAmount := decimal.NewFromFloat(setting.MinWithdrawAmount).Round(2)
	if amount.LessThan(minAmount) {
		return nil, ErrAffiliateWithdrawAmountInvalid
	}
	channel := strings.TrimSpace(input.Channel)
	account := strings.TrimSpace(input.Account)
	if channel == "" || account == "" {
		return nil, ErrAffiliateWithdrawChannelInvalid
	}
	if len(setting.WithdrawChannels) > 0 && !containsWithdrawChannel(setting.WithdrawChannels, channel) {
		return nil, ErrAffiliateWithdrawChannelInvalid
	}
	if err := s.ConfirmDueCommissions(time.Now()); err != nil {
		return nil, err
	}

	var createdID uint
	err = s.repo.Transaction(func(tx *gorm.DB) error {
		repoTx := s.repo.WithTx(tx)
		profile, err := repoTx.GetProfileByUserID(userID)
		if err != nil {
			return err
		}
		if profile == nil {
			return ErrAffiliateNotOpened
		}
		if strings.TrimSpace(profile.Status) != constants.AffiliateProfileStatusActive {
			return ErrAffiliateNotOpened
		}

		commissions, err := repoTx.ListAvailableCommissionsForUpdate(profile.ID)
		if err != nil {
			return err
		}

		remaining := amount
		selectedIDs := make([]uint, 0)
		now := time.Now()
		for _, commission := range commissions {
			if remaining.LessThanOrEqual(decimal.Zero) {
				break
			}
			rowAmount := commission.CommissionAmount.Decimal.Round(2)
			if rowAmount.LessThanOrEqual(decimal.Zero) {
				continue
			}
			if rowAmount.LessThanOrEqual(remaining) {
				selectedIDs = append(selectedIDs, commission.ID)
				remaining = remaining.Sub(rowAmount).Round(2)
				continue
			}

			// 最后一条记录金额大于申请剩余金额时，拆分记录避免超额冻结。
			boundAmount := remaining.Round(2)
			remainAmount := rowAmount.Sub(boundAmount).Round(2)
			commission.CommissionAmount = models.NewMoneyFromDecimal(boundAmount)
			commission.UpdatedAt = now
			if err := repoTx.UpdateCommission(&commission); err != nil {
				return err
			}

			remainCommission := commission
			remainCommission.ID = 0
			remainCommission.CommissionType = buildSplitCommissionType(commission.ID)
			remainCommission.CommissionAmount = models.NewMoneyFromDecimal(remainAmount)
			remainCommission.WithdrawRequestID = nil
			remainCommission.Status = constants.AffiliateCommissionStatusAvailable
			remainCommission.InvalidReason = ""
			remainCommission.CreatedAt = now
			remainCommission.UpdatedAt = now
			if err := repoTx.CreateCommission(&remainCommission); err != nil {
				return err
			}

			selectedIDs = append(selectedIDs, commission.ID)
			remaining = decimal.Zero
			break
		}
		if remaining.GreaterThan(decimal.Zero) {
			return ErrAffiliateWithdrawInsufficient
		}

		req := &models.AffiliateWithdrawRequest{
			AffiliateProfileID: profile.ID,
			Amount:             models.NewMoneyFromDecimal(amount),
			Channel:            channel,
			Account:            account,
			Status:             constants.AffiliateWithdrawStatusPendingReview,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := repoTx.CreateWithdraw(req); err != nil {
			return err
		}
		if err := repoTx.BatchUpdateCommissions(selectedIDs, map[string]interface{}{
			"withdraw_request_id": req.ID,
			"updated_at":          now,
		}); err != nil {
			return err
		}
		createdID = req.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetWithdrawByID(createdID)
}

// ReviewWithdraw 管理端审核提现申请
func (s *AffiliateService) ReviewWithdraw(adminID, withdrawID uint, action, rejectReason string) (*models.AffiliateWithdrawRequest, error) {
	if withdrawID == 0 || s.repo == nil {
		return nil, ErrNotFound
	}
	act := strings.ToLower(strings.TrimSpace(action))
	if act != constants.AffiliateWithdrawActionReject && act != constants.AffiliateWithdrawActionPay {
		return nil, ErrAffiliateWithdrawStatusInvalid
	}
	rejectReason = strings.TrimSpace(rejectReason)

	err := s.repo.Transaction(func(tx *gorm.DB) error {
		repoTx := s.repo.WithTx(tx)
		req, err := repoTx.GetWithdrawByIDForUpdate(withdrawID)
		if err != nil {
			return err
		}
		if req == nil {
			return ErrNotFound
		}
		if req.Status != constants.AffiliateWithdrawStatusPendingReview {
			return ErrAffiliateWithdrawStatusInvalid
		}

		commissions, err := repoTx.ListCommissionsByWithdrawIDForUpdate(withdrawID)
		if err != nil {
			return err
		}
		ids := make([]uint, 0, len(commissions))
		for _, commission := range commissions {
			ids = append(ids, commission.ID)
		}

		now := time.Now()
		req.ProcessedBy = &adminID
		req.ProcessedAt = &now
		req.UpdatedAt = now
		if act == constants.AffiliateWithdrawActionReject {
			req.Status = constants.AffiliateWithdrawStatusRejected
			req.RejectReason = rejectReason
			if err := repoTx.BatchUpdateCommissions(ids, map[string]interface{}{
				"withdraw_request_id": nil,
				"updated_at":          now,
			}); err != nil {
				return err
			}
		} else {
			req.Status = constants.AffiliateWithdrawStatusPaid
			req.RejectReason = ""
			if err := repoTx.BatchUpdateCommissions(ids, map[string]interface{}{
				"status":     constants.AffiliateCommissionStatusWithdrawn,
				"updated_at": now,
			}); err != nil {
				return err
			}
		}
		return repoTx.UpdateWithdraw(req)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetWithdrawByID(withdrawID)
}

// ListAdminUsers 后台查询推广用户列表
func (s *AffiliateService) ListAdminUsers(filter repository.AffiliateProfileListFilter) ([]AffiliateAdminUserItem, int64, error) {
	if s.repo == nil {
		return []AffiliateAdminUserItem{}, 0, nil
	}
	rows, total, err := s.repo.ListProfiles(filter)
	if err != nil {
		return nil, 0, err
	}
	profileIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		if row.ID == 0 {
			continue
		}
		profileIDs = append(profileIDs, row.ID)
	}
	statsMap, err := s.repo.GetProfileStatsBatch(profileIDs)
	if err != nil {
		return nil, 0, err
	}
	result := make([]AffiliateAdminUserItem, 0, len(rows))
	for _, row := range rows {
		agg := statsMap[row.ID]
		stats := AffiliateStats{
			ClickCount:          agg.ClickCount,
			ValidOrderCount:     agg.ValidOrderCount,
			ConversionRate:      calcAffiliateConversion(agg.ValidOrderCount, agg.ClickCount),
			PendingCommission:   models.NewMoneyFromDecimal(agg.PendingCommission.Round(2)),
			AvailableCommission: models.NewMoneyFromDecimal(agg.AvailableCommission.Round(2)),
			WithdrawnCommission: models.NewMoneyFromDecimal(agg.WithdrawnCommission.Round(2)),
		}
		result = append(result, AffiliateAdminUserItem{
			Profile: row,
			Stats:   stats,
		})
	}
	return result, total, nil
}

// ListAdminCommissions 后台查询佣金记录
func (s *AffiliateService) ListAdminCommissions(filter AffiliateAdminCommissionListFilter) ([]models.AffiliateCommission, int64, error) {
	if s.repo == nil {
		return []models.AffiliateCommission{}, 0, nil
	}
	return s.repo.ListCommissions(repository.AffiliateCommissionListFilter{
		Page:               filter.Page,
		PageSize:           filter.PageSize,
		AffiliateProfileID: filter.AffiliateProfileID,
		OrderNo:            strings.TrimSpace(filter.OrderNo),
		Status:             strings.TrimSpace(filter.Status),
		Keyword:            strings.TrimSpace(filter.Keyword),
	})
}

// ListAdminWithdraws 后台查询提现申请
func (s *AffiliateService) ListAdminWithdraws(filter AffiliateAdminWithdrawListFilter) ([]models.AffiliateWithdrawRequest, int64, error) {
	if s.repo == nil {
		return []models.AffiliateWithdrawRequest{}, 0, nil
	}
	return s.repo.ListWithdraws(repository.AffiliateWithdrawListFilter{
		Page:               filter.Page,
		PageSize:           filter.PageSize,
		AffiliateProfileID: filter.AffiliateProfileID,
		Status:             strings.TrimSpace(filter.Status),
		Keyword:            strings.TrimSpace(filter.Keyword),
	})
}

func (s *AffiliateService) resolveAffiliateProfileForOrder(order *models.Order) (*models.AffiliateProfile, error) {
	if order == nil || s.repo == nil {
		return nil, nil
	}
	if order.AffiliateProfileID != nil && *order.AffiliateProfileID > 0 {
		return s.repo.GetProfileByID(*order.AffiliateProfileID)
	}
	if strings.TrimSpace(order.AffiliateCode) != "" {
		return s.repo.GetProfileByCode(order.AffiliateCode)
	}
	return nil, nil
}

func (s *AffiliateService) calculateCommissionBaseAmount(order *models.Order) (decimal.Decimal, error) {
	if order == nil || s.productRepo == nil {
		return decimal.Zero, nil
	}
	productIDs := collectAffiliateProductIDs(order)
	if len(productIDs) == 0 {
		return decimal.Zero, nil
	}
	products, err := s.productRepo.ListByIDs(productIDs)
	if err != nil {
		return decimal.Zero, err
	}
	productMap := make(map[uint]models.Product, len(products))
	for _, product := range products {
		productMap[product.ID] = product
	}

	targetOrders := order.Children
	if len(targetOrders) == 0 {
		targetOrders = []models.Order{*order}
	}

	total := decimal.Zero
	for _, current := range targetOrders {
		for _, item := range current.Items {
			product, ok := productMap[item.ProductID]
			if !ok || !product.IsAffiliateEnabled {
				continue
			}
			payable := item.TotalPrice.Decimal.Sub(item.CouponDiscount.Decimal).Round(2)
			if payable.LessThan(decimal.Zero) {
				payable = decimal.Zero
			}
			total = total.Add(payable).Round(2)
		}
	}
	return total, nil
}

func collectAffiliateProductIDs(order *models.Order) []uint {
	if order == nil {
		return nil
	}
	ids := make([]uint, 0)
	seen := make(map[uint]struct{})
	appendItem := func(item models.OrderItem) {
		if item.ProductID == 0 {
			return
		}
		if _, ok := seen[item.ProductID]; ok {
			return
		}
		seen[item.ProductID] = struct{}{}
		ids = append(ids, item.ProductID)
	}
	for _, item := range order.Items {
		appendItem(item)
	}
	for _, child := range order.Children {
		for _, item := range child.Items {
			appendItem(item)
		}
	}
	return ids
}

func (s *AffiliateService) buildProfileStats(profileID uint) (AffiliateStats, error) {
	stats := AffiliateStats{
		PendingCommission:   models.NewMoneyFromDecimal(decimal.Zero),
		AvailableCommission: models.NewMoneyFromDecimal(decimal.Zero),
		WithdrawnCommission: models.NewMoneyFromDecimal(decimal.Zero),
	}
	if profileID == 0 || s.repo == nil {
		return stats, nil
	}
	clickCount, err := s.repo.CountClicksByProfile(profileID)
	if err != nil {
		return stats, err
	}
	validOrders, err := s.repo.CountValidOrdersByProfile(profileID)
	if err != nil {
		return stats, err
	}
	pendingAmount, err := s.repo.SumCommissionByProfile(profileID, []string{
		constants.AffiliateCommissionStatusPendingConfirm,
	}, false)
	if err != nil {
		return stats, err
	}
	availableAmount, err := s.repo.SumCommissionByProfile(profileID, []string{
		constants.AffiliateCommissionStatusAvailable,
	}, true)
	if err != nil {
		return stats, err
	}
	withdrawnAmount, err := s.repo.SumCommissionByProfile(profileID, []string{
		constants.AffiliateCommissionStatusWithdrawn,
	}, false)
	if err != nil {
		return stats, err
	}

	stats.ClickCount = clickCount
	stats.ValidOrderCount = validOrders
	stats.ConversionRate = calcAffiliateConversion(validOrders, clickCount)
	stats.PendingCommission = models.NewMoneyFromDecimal(pendingAmount)
	stats.AvailableCommission = models.NewMoneyFromDecimal(availableAmount)
	stats.WithdrawnCommission = models.NewMoneyFromDecimal(withdrawnAmount)
	return stats, nil
}

func calcAffiliateConversion(validOrders, clicks int64) float64 {
	if clicks <= 0 || validOrders <= 0 {
		return 0
	}
	value := (float64(validOrders) / float64(clicks)) * 100
	return math.Round(value*100) / 100
}

func containsWithdrawChannel(channels []string, channel string) bool {
	target := strings.ToLower(strings.TrimSpace(channel))
	if target == "" {
		return false
	}
	for _, item := range channels {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

func normalizeAffiliateProfileIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return []uint{}
	}
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func generateAffiliateCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var builder strings.Builder
	builder.Grow(affiliateCodeLength)
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < affiliateCodeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(alphabet[n.Int64()])
	}
	return builder.String(), nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func buildSplitCommissionType(sourceID uint) string {
	suffix := strconv.FormatInt(time.Now().UnixNano()%1000000, 10)
	base := affiliateSplitTypePrefix + strconv.FormatUint(uint64(sourceID), 36)
	result := base + suffix
	if len(result) > 20 {
		return result[:20]
	}
	return result
}
