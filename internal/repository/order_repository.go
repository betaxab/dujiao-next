package repository

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// OrderRepository 订单数据访问接口
type OrderRepository interface {
	Create(order *models.Order, items []models.OrderItem) error
	GetByID(id uint) (*models.Order, error)
	ResolveReceiverEmailByOrderID(orderID uint) (string, error)
	GetByIDAndUser(id uint, userID uint) (*models.Order, error)
	GetByOrderNoAndUser(orderNo string, userID uint) (*models.Order, error)
	GetByIDAndGuest(id uint, email, password string) (*models.Order, error)
	GetByOrderNoAndGuest(orderNo, email, password string) (*models.Order, error)
	ListChildren(parentID uint) ([]models.Order, error)
	ListByUser(filter OrderListFilter) ([]models.Order, int64, error)
	ListByGuest(email, password string, page, pageSize int) ([]models.Order, int64, error)
	ListAdmin(filter OrderListFilter) ([]models.Order, int64, error)
	UpdateStatus(id uint, status string, updates map[string]interface{}) error
	WithTx(tx *gorm.DB) *GormOrderRepository
}

// GormOrderRepository GORM 实现
type GormOrderRepository struct {
	db *gorm.DB
}

// NewOrderRepository 创建订单仓库
func NewOrderRepository(db *gorm.DB) *GormOrderRepository {
	return &GormOrderRepository{db: db}
}

// WithTx 绑定事务
func (r *GormOrderRepository) WithTx(tx *gorm.DB) *GormOrderRepository {
	if tx == nil {
		return r
	}
	return &GormOrderRepository{db: tx}
}

func (r *GormOrderRepository) withChildren(query *gorm.DB) *gorm.DB {
	return query.Preload("Children").Preload("Children.Items").Preload("Children.Fulfillment")
}

// Create 创建订单与订单项
func (r *GormOrderRepository) Create(order *models.Order, items []models.OrderItem) error {
	if err := r.db.Create(order).Error; err != nil {
		return err
	}
	for i := range items {
		items[i].OrderID = order.ID
	}
	if len(items) > 0 {
		if err := r.db.Create(&items).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetByID 根据 ID 获取订单
func (r *GormOrderRepository) GetByID(id uint) (*models.Order, error) {
	var order models.Order
	query := r.withChildren(r.db.Preload("Items").Preload("Fulfillment"))
	if err := query.First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// ResolveReceiverEmailByOrderID 根据订单 ID 解析状态通知的收件邮箱。
func (r *GormOrderRepository) ResolveReceiverEmailByOrderID(orderID uint) (string, error) {
	if orderID == 0 {
		return "", nil
	}

	var orderRow struct {
		UserID     uint
		GuestEmail string
	}
	if err := r.db.Model(&models.Order{}).
		Select("user_id", "guest_email").
		Where("id = ?", orderID).
		Take(&orderRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if orderRow.UserID == 0 {
		return strings.TrimSpace(orderRow.GuestEmail), nil
	}

	var userRow struct {
		Email string
	}
	if err := r.db.Model(&models.User{}).
		Select("email").
		Where("id = ?", orderRow.UserID).
		Take(&userRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(userRow.Email), nil
}

// GetByIDAndUser 获取用户订单详情
func (r *GormOrderRepository) GetByIDAndUser(id uint, userID uint) (*models.Order, error) {
	var order models.Order
	query := r.withChildren(r.db.Preload("Items").Preload("Fulfillment"))
	if err := query.Where("id = ? AND user_id = ? AND parent_id IS NULL", id, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}
func (r *GormOrderRepository) GetByOrderNoAndUser(orderNo string, userID uint) (*models.Order, error) {
	var order models.Order
	query := r.withChildren(r.db.Preload("Items").Preload("Fulfillment"))
	if err := query.Where("order_no = ? AND user_id = ? AND parent_id IS NULL", orderNo, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByIDAndGuest 获取游客订单详情
func (r *GormOrderRepository) GetByIDAndGuest(id uint, email, password string) (*models.Order, error) {
	var order models.Order
	query := r.withChildren(r.db.Preload("Items").Preload("Fulfillment"))
	if err := query.
		Where("id = ? AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", id, email, password).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderNoAndGuest 获取游客订单详情（按订单号）
func (r *GormOrderRepository) GetByOrderNoAndGuest(orderNo, email, password string) (*models.Order, error) {
	var order models.Order
	query := r.withChildren(r.db.Preload("Items").Preload("Fulfillment"))
	if err := query.
		Where("order_no = ? AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", orderNo, email, password).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// ListChildren 获取子订单列表
func (r *GormOrderRepository) ListChildren(parentID uint) ([]models.Order, error) {
	var orders []models.Order
	if parentID == 0 {
		return orders, nil
	}
	if err := r.db.Preload("Items").Preload("Fulfillment").
		Where("parent_id = ?", parentID).
		Order("id asc").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// ListAdmin 管理端订单列表
func (r *GormOrderRepository) ListAdmin(filter OrderListFilter) ([]models.Order, int64, error) {
	var orders []models.Order
	query := r.db.Model(&models.Order{}).Where("parent_id IS NULL")

	if filter.UserID != 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.OrderNo != "" {
		query = query.Where("order_no = ?", filter.OrderNo)
	}
	if filter.GuestEmail != "" {
		query = query.Where("guest_email = ?", filter.GuestEmail)
	}
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at <= ?", *filter.CreatedTo)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = applyPagination(query, filter.Page, filter.PageSize)

	query = r.withChildren(query.Preload("Items"))
	if err := query.Order("id desc").Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// UpdateStatus 更新订单状态
func (r *GormOrderRepository) UpdateStatus(id uint, status string, updates map[string]interface{}) error {
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["status"] = status
	return r.db.Model(&models.Order{}).Where("id = ?", id).Updates(updates).Error
}

// ListByUser 获取用户订单列表
func (r *GormOrderRepository) ListByUser(filter OrderListFilter) ([]models.Order, int64, error) {
	var orders []models.Order
	query := r.db.Model(&models.Order{}).Where("user_id = ? AND parent_id IS NULL", filter.UserID)

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.OrderNo != "" {
		query = query.Where("order_no LIKE ?", "%"+filter.OrderNo+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = applyPagination(query, filter.Page, filter.PageSize)

	query = r.withChildren(query.Preload("Items"))
	if err := query.Order("id desc").Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// ListByGuest 获取游客订单列表
func (r *GormOrderRepository) ListByGuest(email, password string, page, pageSize int) ([]models.Order, int64, error) {
	var total int64
	if err := r.db.Model(&models.Order{}).
		Where("user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", email, password).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var orders []models.Order
	query := r.withChildren(r.db.Preload("Items"))
	if err := query.
		Where("user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", email, password).
		Order("id desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}
