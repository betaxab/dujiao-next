package repository

import (
	"errors"
	"time"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// CardSecretRepository 卡密库存数据访问接口
type CardSecretRepository interface {
	CreateBatch(items []models.CardSecret) error
	ListByProduct(productID, skuID uint, status string, page, pageSize int) ([]models.CardSecret, int64, error)
	ListAll(status string, page, pageSize int) ([]models.CardSecret, int64, error)
	ListByIDs(ids []uint) ([]models.CardSecret, error)
	ListByOrderAndStatus(orderID uint, status string) ([]models.CardSecret, error)
	GetByID(id uint) (*models.CardSecret, error)
	Update(secret *models.CardSecret) error
	BatchUpdateStatus(ids []uint, status string, updatedAt time.Time) (int64, error)
	BatchDeleteByIDs(ids []uint) (int64, error)
	CountByProduct(productID, skuID uint) (int64, int64, int64, error)
	CountAvailable(productID, skuID uint) (int64, error)
	CountAvailableByProductIDs(productIDs []uint) (map[uint]int64, error)
	CountReserved(productID, skuID uint) (int64, error)
	CountStockByProductIDs(productIDs []uint) ([]SKUStockCount, error)
	Reserve(ids []uint, orderID uint, reservedAt time.Time) (int64, error)
	ReleaseByOrder(orderID uint) (int64, error)
	MarkUsed(ids []uint, orderID uint, usedAt time.Time) (int64, error)
	WithTx(tx *gorm.DB) *GormCardSecretRepository
}

// SKUStockCount 卡密库存统计结果
type SKUStockCount struct {
	ProductID uint
	SKUID     uint
	Status    string
	Total     int64
}

// GormCardSecretRepository GORM 实现
type GormCardSecretRepository struct {
	db *gorm.DB
}

// NewCardSecretRepository 创建卡密仓库
func NewCardSecretRepository(db *gorm.DB) *GormCardSecretRepository {
	return &GormCardSecretRepository{db: db}
}

// WithTx 绑定事务
func (r *GormCardSecretRepository) WithTx(tx *gorm.DB) *GormCardSecretRepository {
	if tx == nil {
		return r
	}
	return &GormCardSecretRepository{db: tx}
}

// CreateBatch 批量创建卡密
func (r *GormCardSecretRepository) CreateBatch(items []models.CardSecret) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Create(&items).Error
}

// ListByProduct 按商品获取卡密列表
func (r *GormCardSecretRepository) ListByProduct(productID, skuID uint, status string, page, pageSize int) ([]models.CardSecret, int64, error) {
	if productID == 0 {
		return nil, 0, errors.New("invalid product id")
	}
	query := r.db.Model(&models.CardSecret{}).Where("product_id = ?", productID).Preload("Batch")
	if skuID > 0 {
		query = query.Where("sku_id = ?", skuID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageSize > 0 {
		offset := (page - 1) * pageSize
		query = query.Limit(pageSize).Offset(offset)
	}

	var items []models.CardSecret
	if err := query.Order("id asc").Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListAll 获取全量卡密列表
func (r *GormCardSecretRepository) ListAll(status string, page, pageSize int) ([]models.CardSecret, int64, error) {
	query := r.db.Model(&models.CardSecret{}).Preload("Batch")
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageSize > 0 {
		offset := (page - 1) * pageSize
		query = query.Limit(pageSize).Offset(offset)
	}

	var items []models.CardSecret
	if err := query.Order("id asc").Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListByIDs 按 ID 列表查询卡密
func (r *GormCardSecretRepository) ListByIDs(ids []uint) ([]models.CardSecret, error) {
	if len(ids) == 0 {
		return []models.CardSecret{}, nil
	}
	var items []models.CardSecret
	if err := r.db.Where("id IN ?", ids).Order("id asc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListByOrderAndStatus 按订单与状态获取卡密
func (r *GormCardSecretRepository) ListByOrderAndStatus(orderID uint, status string) ([]models.CardSecret, error) {
	if orderID == 0 {
		return nil, errors.New("invalid order id")
	}
	query := r.db.Model(&models.CardSecret{}).Where("order_id = ?", orderID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var items []models.CardSecret
	if err := query.Order("id asc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// GetByID 根据 ID 获取卡密
func (r *GormCardSecretRepository) GetByID(id uint) (*models.CardSecret, error) {
	var secret models.CardSecret
	if err := r.db.First(&secret, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &secret, nil
}

// Update 更新卡密
func (r *GormCardSecretRepository) Update(secret *models.CardSecret) error {
	return r.db.Save(secret).Error
}

// BatchUpdateStatus 批量更新卡密状态
func (r *GormCardSecretRepository) BatchUpdateStatus(ids []uint, status string, updatedAt time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	result := r.db.Model(&models.CardSecret{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": updatedAt,
		})
	return result.RowsAffected, result.Error
}

// BatchDeleteByIDs 批量删除卡密
func (r *GormCardSecretRepository) BatchDeleteByIDs(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.Where("id IN ?", ids).Delete(&models.CardSecret{})
	return result.RowsAffected, result.Error
}

// CountByProduct 统计库存数量（总/可用/已用）
func (r *GormCardSecretRepository) CountByProduct(productID, skuID uint) (int64, int64, int64, error) {
	if productID == 0 {
		return 0, 0, 0, errors.New("invalid product id")
	}

	buildQuery := func() *gorm.DB {
		query := r.db.Model(&models.CardSecret{}).Where("product_id = ?", productID)
		if skuID > 0 {
			query = query.Where("sku_id = ?", skuID)
		}
		return query
	}

	var total int64
	if err := buildQuery().Count(&total).Error; err != nil {
		return 0, 0, 0, err
	}

	var available int64
	if err := buildQuery().Where("status = ?", models.CardSecretStatusAvailable).
		Count(&available).Error; err != nil {
		return 0, 0, 0, err
	}

	var used int64
	if err := buildQuery().Where("status = ?", models.CardSecretStatusUsed).
		Count(&used).Error; err != nil {
		return 0, 0, 0, err
	}
	return total, available, used, nil
}

// CountAvailable 统计可用库存
func (r *GormCardSecretRepository) CountAvailable(productID, skuID uint) (int64, error) {
	if productID == 0 {
		return 0, errors.New("invalid product id")
	}
	query := r.db.Model(&models.CardSecret{}).
		Where("product_id = ? AND status = ?", productID, models.CardSecretStatusAvailable)
	if skuID > 0 {
		query = query.Where("sku_id = ?", skuID)
	}
	var count int64
	if err := query.
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountAvailableByProductIDs 批量统计可用库存
func (r *GormCardSecretRepository) CountAvailableByProductIDs(productIDs []uint) (map[uint]int64, error) {
	result := make(map[uint]int64)
	if len(productIDs) == 0 {
		return result, nil
	}

	type countRow struct {
		ProductID uint
		Total     int64
	}

	var rows []countRow
	if err := r.db.Model(&models.CardSecret{}).
		Select("product_id, COUNT(*) as total").
		Where("product_id IN ? AND status = ?", productIDs, models.CardSecretStatusAvailable).
		Group("product_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.ProductID] = row.Total
	}

	return result, nil
}

// CountStockByProductIDs 批量获取商品的 SKUs 的各状态卡密数量
func (r *GormCardSecretRepository) CountStockByProductIDs(productIDs []uint) ([]SKUStockCount, error) {
	if len(productIDs) == 0 {
		return []SKUStockCount{}, nil
	}

	var rows []SKUStockCount
	if err := r.db.Model(&models.CardSecret{}).
		Select("product_id, sku_id, status, COUNT(*) as total").
		Where("product_id IN ?", productIDs).
		Group("product_id, sku_id, status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// CountReserved 统计占用库存
func (r *GormCardSecretRepository) CountReserved(productID, skuID uint) (int64, error) {
	if productID == 0 {
		return 0, errors.New("invalid product id")
	}
	query := r.db.Model(&models.CardSecret{}).
		Where("product_id = ? AND status = ?", productID, models.CardSecretStatusReserved)
	if skuID > 0 {
		query = query.Where("sku_id = ?", skuID)
	}
	var count int64
	if err := query.
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Reserve 占用卡密库存
func (r *GormCardSecretRepository) Reserve(ids []uint, orderID uint, reservedAt time.Time) (int64, error) {
	if len(ids) == 0 || orderID == 0 {
		return 0, nil
	}
	result := r.db.Model(&models.CardSecret{}).
		Where("id IN ? AND status = ?", ids, models.CardSecretStatusAvailable).
		Updates(map[string]interface{}{
			"status":      models.CardSecretStatusReserved,
			"order_id":    orderID,
			"reserved_at": reservedAt,
			"updated_at":  reservedAt,
		})
	return result.RowsAffected, result.Error
}

// ReleaseByOrder 释放占用库存
func (r *GormCardSecretRepository) ReleaseByOrder(orderID uint) (int64, error) {
	if orderID == 0 {
		return 0, nil
	}
	now := time.Now()
	result := r.db.Model(&models.CardSecret{}).
		Where("order_id = ? AND status = ?", orderID, models.CardSecretStatusReserved).
		Updates(map[string]interface{}{
			"status":      models.CardSecretStatusAvailable,
			"order_id":    nil,
			"reserved_at": nil,
			"updated_at":  now,
		})
	return result.RowsAffected, result.Error
}

// MarkUsed 标记卡密已使用
func (r *GormCardSecretRepository) MarkUsed(ids []uint, orderID uint, usedAt time.Time) (int64, error) {
	if len(ids) == 0 || orderID == 0 {
		return 0, nil
	}
	result := r.db.Model(&models.CardSecret{}).
		Where("id IN ? AND status IN ? AND (order_id IS NULL OR order_id = ?)", ids, []string{models.CardSecretStatusAvailable, models.CardSecretStatusReserved}, orderID).
		Updates(map[string]interface{}{
			"status":      models.CardSecretStatusUsed,
			"order_id":    orderID,
			"used_at":     usedAt,
			"reserved_at": nil,
			"updated_at":  usedAt,
		})
	return result.RowsAffected, result.Error
}
