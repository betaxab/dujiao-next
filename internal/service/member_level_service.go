package service

import (
	"errors"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/shopspring/decimal"
)

var (
	ErrMemberLevelNotFound      = errors.New("member_level_not_found")
	ErrMemberLevelSlugExists    = errors.New("member_level_slug_exists")
	ErrMemberLevelDeleteDefault = errors.New("member_level_cannot_delete_default")
)

// MemberLevelService 会员等级服务
type MemberLevelService struct {
	levelRepo repository.MemberLevelRepository
	priceRepo repository.MemberLevelPriceRepository
	userRepo  repository.UserRepository
}

// NewMemberLevelService 创建会员等级服务
func NewMemberLevelService(
	levelRepo repository.MemberLevelRepository,
	priceRepo repository.MemberLevelPriceRepository,
	userRepo repository.UserRepository,
) *MemberLevelService {
	return &MemberLevelService{
		levelRepo: levelRepo,
		priceRepo: priceRepo,
		userRepo:  userRepo,
	}
}

// --- 等级 CRUD ---

func (s *MemberLevelService) GetByID(id uint) (*models.MemberLevel, error) {
	return s.levelRepo.GetByID(id)
}

func (s *MemberLevelService) ListLevels(filter repository.MemberLevelListFilter) ([]models.MemberLevel, int64, error) {
	return s.levelRepo.List(filter)
}

func (s *MemberLevelService) ListActiveLevels() ([]models.MemberLevel, error) {
	return s.levelRepo.ListAllActive()
}

func (s *MemberLevelService) CreateLevel(level *models.MemberLevel) error {
	existing, err := s.levelRepo.GetBySlug(level.Slug)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrMemberLevelSlugExists
	}
	if level.IsDefault {
		if err := s.levelRepo.ClearDefault(0); err != nil {
			return err
		}
	}
	return s.levelRepo.Create(level)
}

func (s *MemberLevelService) UpdateLevel(level *models.MemberLevel) error {
	existing, err := s.levelRepo.GetBySlug(level.Slug)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != level.ID {
		return ErrMemberLevelSlugExists
	}
	if level.IsDefault {
		if err := s.levelRepo.ClearDefault(level.ID); err != nil {
			return err
		}
	}
	return s.levelRepo.Update(level)
}

func (s *MemberLevelService) DeleteLevel(id uint) error {
	level, err := s.levelRepo.GetByID(id)
	if err != nil {
		return err
	}
	if level == nil {
		return ErrMemberLevelNotFound
	}
	if level.IsDefault {
		return ErrMemberLevelDeleteDefault
	}
	return s.levelRepo.Delete(id)
}

// --- 等级价 CRUD ---

func (s *MemberLevelService) GetLevelPricesByProduct(productID uint) ([]models.MemberLevelPrice, error) {
	return s.priceRepo.ListByProduct(productID)
}

func (s *MemberLevelService) BatchUpsertLevelPrices(prices []models.MemberLevelPrice) error {
	return s.priceRepo.BatchUpsert(prices)
}

func (s *MemberLevelService) DeleteLevelPrice(id uint) error {
	return s.priceRepo.Delete(id)
}

// --- 价格解析 ---

// ResolveMemberPrice 解析会员价
// 优先级：SKU级覆盖 > 商品级覆盖 > 等级折扣率 * basePrice
// 返回会员价和会员优惠金额
func (s *MemberLevelService) ResolveMemberPrice(levelID, productID, skuID uint, basePrice decimal.Decimal) (memberPrice decimal.Decimal, memberDiscount decimal.Decimal) {
	if levelID == 0 {
		return basePrice, decimal.Zero
	}

	// 查找 SKU 级覆盖
	if skuID > 0 {
		skuPrice, err := s.priceRepo.GetByLevelAndProductAndSKU(levelID, productID, skuID)
		if err == nil && skuPrice != nil && skuPrice.PriceAmount.Decimal.GreaterThan(decimal.Zero) {
			mp := skuPrice.PriceAmount.Decimal.Round(2)
			if mp.LessThan(basePrice) {
				return mp, basePrice.Sub(mp).Round(2)
			}
			return basePrice, decimal.Zero
		}
	}

	// 查找商品级覆盖
	productPrice, err := s.priceRepo.GetByLevelAndProductAndSKU(levelID, productID, 0)
	if err == nil && productPrice != nil && productPrice.PriceAmount.Decimal.GreaterThan(decimal.Zero) {
		mp := productPrice.PriceAmount.Decimal.Round(2)
		if mp.LessThan(basePrice) {
			return mp, basePrice.Sub(mp).Round(2)
		}
		return basePrice, decimal.Zero
	}

	// 使用等级折扣率
	level, err := s.levelRepo.GetByID(levelID)
	if err != nil || level == nil {
		return basePrice, decimal.Zero
	}
	rate := level.DiscountRate.Decimal
	if rate.LessThanOrEqual(decimal.Zero) || rate.GreaterThanOrEqual(decimal.NewFromInt(100)) {
		return basePrice, decimal.Zero
	}
	mp := basePrice.Mul(rate).Div(decimal.NewFromInt(100)).Round(2)
	if mp.LessThan(basePrice) {
		return mp, basePrice.Sub(mp).Round(2)
	}
	return basePrice, decimal.Zero
}

// ResolveMemberPriceForProducts 批量解析会员价（用于商品列表）
func (s *MemberLevelService) ResolveMemberPriceForProducts(levelID uint, productIDs []uint) (map[uint][]models.MemberLevelPrice, error) {
	if levelID == 0 || len(productIDs) == 0 {
		return nil, nil
	}
	prices, err := s.priceRepo.ListByLevelAndProducts(levelID, productIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[uint][]models.MemberLevelPrice)
	for _, p := range prices {
		result[p.ProductID] = append(result[p.ProductID], p)
	}
	return result, nil
}

// --- 等级升级 ---

// CheckAndUpgrade 检查用户是否满足升级条件，只升不降
func (s *MemberLevelService) CheckAndUpgrade(userID uint) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return err
	}

	levels, err := s.levelRepo.ListAllActive()
	if err != nil {
		return err
	}
	if len(levels) == 0 {
		return nil
	}

	// levels 已按 sort_order DESC 排列，取首个满足条件且高于当前等级的
	var currentSortOrder int
	for _, l := range levels {
		if l.ID == user.MemberLevelID {
			currentSortOrder = l.SortOrder
			break
		}
	}

	for _, level := range levels {
		if level.SortOrder < currentSortOrder {
			continue
		}
		if level.ID == user.MemberLevelID {
			continue
		}
		if s.meetsThreshold(user, &level) {
			user.MemberLevelID = level.ID
			return s.userRepo.Update(user)
		}
	}
	return nil
}

// meetsThreshold 判断用户是否满足等级阈值（充值累计 OR 消费累计）
func (s *MemberLevelService) meetsThreshold(user *models.User, level *models.MemberLevel) bool {
	rechargeThreshold := level.RechargeThreshold.Decimal
	spendThreshold := level.SpendThreshold.Decimal

	if rechargeThreshold.GreaterThan(decimal.Zero) &&
		user.TotalRecharged.Decimal.GreaterThanOrEqual(rechargeThreshold) {
		return true
	}
	if spendThreshold.GreaterThan(decimal.Zero) &&
		user.TotalSpent.Decimal.GreaterThanOrEqual(spendThreshold) {
		return true
	}
	return false
}

// OnRechargeCompleted 充值到账后触发
func (s *MemberLevelService) OnRechargeCompleted(userID uint, amount decimal.Decimal) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return err
	}
	user.TotalRecharged = models.NewMoneyFromDecimal(user.TotalRecharged.Decimal.Add(amount))
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	return s.CheckAndUpgrade(userID)
}

// OnOrderPaid 订单支付成功后触发
func (s *MemberLevelService) OnOrderPaid(userID uint, amount decimal.Decimal) error {
	if userID == 0 {
		return nil
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return err
	}
	user.TotalSpent = models.NewMoneyFromDecimal(user.TotalSpent.Decimal.Add(amount))
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	return s.CheckAndUpgrade(userID)
}

// AssignDefaultLevel 为新用户分配默认等级
func (s *MemberLevelService) AssignDefaultLevel(userID uint) error {
	defaultLevel, err := s.levelRepo.GetDefault()
	if err != nil || defaultLevel == nil {
		return err
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil || user == nil {
		return err
	}
	if user.MemberLevelID == 0 {
		user.MemberLevelID = defaultLevel.ID
		return s.userRepo.Update(user)
	}
	return nil
}

// SetUserLevel 管理员手动设置用户等级
func (s *MemberLevelService) SetUserLevel(userID, levelID uint) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user_not_found")
	}
	if levelID > 0 {
		level, err := s.levelRepo.GetByID(levelID)
		if err != nil {
			return err
		}
		if level == nil {
			return ErrMemberLevelNotFound
		}
	}
	user.MemberLevelID = levelID
	return s.userRepo.Update(user)
}

// BackfillDefaultLevel 为所有未分配等级的老用户批量分配默认等级，返回影响行数
func (s *MemberLevelService) BackfillDefaultLevel() (int64, error) {
	defaultLevel, err := s.levelRepo.GetDefault()
	if err != nil {
		return 0, err
	}
	if defaultLevel == nil {
		return 0, ErrMemberLevelNotFound
	}
	return s.userRepo.AssignDefaultMemberLevel(defaultLevel.ID)
}
