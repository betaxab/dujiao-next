package service

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"gorm.io/gorm"
)

// CardSecretService 卡密库存服务
type CardSecretService struct {
	secretRepo  repository.CardSecretRepository
	batchRepo   repository.CardSecretBatchRepository
	productRepo repository.ProductRepository
}

// NewCardSecretService 创建卡密库存服务
func NewCardSecretService(secretRepo repository.CardSecretRepository, batchRepo repository.CardSecretBatchRepository, productRepo repository.ProductRepository) *CardSecretService {
	return &CardSecretService{
		secretRepo:  secretRepo,
		batchRepo:   batchRepo,
		productRepo: productRepo,
	}
}

// CreateCardSecretBatchInput 批量录入卡密输入
type CreateCardSecretBatchInput struct {
	ProductID uint
	Secrets   []string
	BatchNo   string
	Note      string
	Source    string
	AdminID   uint
}

// CreateCardSecretBatch 批量录入卡密
func (s *CardSecretService) CreateCardSecretBatch(input CreateCardSecretBatchInput) (*models.CardSecretBatch, int, error) {
	if input.ProductID == 0 {
		return nil, 0, ErrCardSecretInvalid
	}
	if len(input.Secrets) == 0 {
		return nil, 0, ErrCardSecretInvalid
	}

	product, err := s.productRepo.GetByID(strings.TrimSpace(strconv.FormatUint(uint64(input.ProductID), 10)))
	if err != nil {
		return nil, 0, ErrProductFetchFailed
	}
	if product == nil {
		return nil, 0, ErrProductNotFound
	}

	normalized := normalizeSecrets(input.Secrets)
	if len(normalized) == 0 {
		return nil, 0, ErrCardSecretInvalid
	}
	if s.batchRepo == nil {
		return nil, 0, ErrCardSecretBatchCreateFailed
	}

	batchNo := strings.TrimSpace(input.BatchNo)
	if batchNo == "" {
		batchNo = generateBatchNo()
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = constants.CardSecretSourceManual
	}

	now := time.Now()
	batch := &models.CardSecretBatch{
		ProductID:  input.ProductID,
		BatchNo:    batchNo,
		Source:     source,
		TotalCount: len(normalized),
		Note:       strings.TrimSpace(input.Note),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if input.AdminID > 0 {
		batch.CreatedBy = &input.AdminID
	}

	err = models.DB.Transaction(func(tx *gorm.DB) error {
		batchRepo := s.batchRepo.WithTx(tx)
		secretRepo := s.secretRepo.WithTx(tx)
		if err := batchRepo.Create(batch); err != nil {
			return ErrCardSecretBatchCreateFailed
		}
		items := make([]models.CardSecret, 0, len(normalized))
		for _, secret := range normalized {
			items = append(items, models.CardSecret{
				ProductID: input.ProductID,
				BatchID:   &batch.ID,
				Secret:    secret,
				Status:    models.CardSecretStatusAvailable,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		if err := secretRepo.CreateBatch(items); err != nil {
			return ErrCardSecretCreateFailed
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrCardSecretBatchCreateFailed) {
			return nil, 0, ErrCardSecretBatchCreateFailed
		}
		return nil, 0, ErrCardSecretCreateFailed
	}
	return batch, batch.TotalCount, nil
}

// ImportCardSecretCSVInput 导入 CSV 输入
type ImportCardSecretCSVInput struct {
	ProductID uint
	File      *multipart.FileHeader
	BatchNo   string
	Note      string
	AdminID   uint
}

// ImportCardSecretCSV 从 CSV 导入卡密
func (s *CardSecretService) ImportCardSecretCSV(input ImportCardSecretCSVInput) (*models.CardSecretBatch, int, error) {
	if input.ProductID == 0 || input.File == nil {
		return nil, 0, ErrCardSecretInvalid
	}

	file, err := input.File.Open()
	if err != nil {
		return nil, 0, ErrCardSecretImportFailed
	}
	defer file.Close()

	secrets, err := parseCSVSecrets(file)
	if err != nil {
		return nil, 0, ErrCardSecretImportFailed
	}
	return s.CreateCardSecretBatch(CreateCardSecretBatchInput{
		ProductID: input.ProductID,
		Secrets:   secrets,
		BatchNo:   input.BatchNo,
		Note:      input.Note,
		Source:    constants.CardSecretSourceCSV,
		AdminID:   input.AdminID,
	})
}

// ListCardSecretInput 卡密列表输入
type ListCardSecretInput struct {
	ProductID uint
	Status    string
	Page      int
	PageSize  int
}

// ListCardSecrets 获取卡密列表
func (s *CardSecretService) ListCardSecrets(input ListCardSecretInput) ([]models.CardSecret, int64, error) {
	status := strings.TrimSpace(input.Status)
	var (
		items []models.CardSecret
		total int64
		err   error
	)
	if input.ProductID == 0 {
		items, total, err = s.secretRepo.ListAll(status, input.Page, input.PageSize)
	} else {
		items, total, err = s.secretRepo.ListByProduct(input.ProductID, status, input.Page, input.PageSize)
	}
	if err != nil {
		return nil, 0, ErrCardSecretFetchFailed
	}
	return items, total, nil
}

// UpdateCardSecret 更新卡密
func (s *CardSecretService) UpdateCardSecret(id uint, secret, status string) (*models.CardSecret, error) {
	if id == 0 {
		return nil, ErrCardSecretInvalid
	}
	item, err := s.secretRepo.GetByID(id)
	if err != nil {
		return nil, ErrCardSecretFetchFailed
	}
	if item == nil {
		return nil, ErrNotFound
	}
	trimmedSecret := strings.TrimSpace(secret)
	if trimmedSecret != "" {
		item.Secret = trimmedSecret
	}
	trimmedStatus := strings.TrimSpace(status)
	if trimmedStatus != "" {
		switch trimmedStatus {
		case models.CardSecretStatusAvailable, models.CardSecretStatusReserved, models.CardSecretStatusUsed:
			item.Status = trimmedStatus
		default:
			return nil, ErrCardSecretInvalid
		}
	}
	item.UpdatedAt = time.Now()
	if err := s.secretRepo.Update(item); err != nil {
		return nil, ErrCardSecretUpdateFailed
	}
	return item, nil
}

// CardSecretStats 卡密统计
type CardSecretStats struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"`
	Reserved  int64 `json:"reserved"`
	Used      int64 `json:"used"`
}

// GetStats 获取库存统计
func (s *CardSecretService) GetStats(productID uint) (*CardSecretStats, error) {
	if productID == 0 {
		return nil, ErrCardSecretInvalid
	}
	total, available, used, err := s.secretRepo.CountByProduct(productID)
	if err != nil {
		return nil, ErrCardSecretStatsFailed
	}
	reserved, err := s.secretRepo.CountReserved(productID)
	if err != nil {
		return nil, ErrCardSecretStatsFailed
	}
	return &CardSecretStats{
		Total:     total,
		Available: available,
		Reserved:  reserved,
		Used:      used,
	}, nil
}

// ListBatches 获取批次列表
func (s *CardSecretService) ListBatches(productID uint, page, pageSize int) ([]models.CardSecretBatch, int64, error) {
	if productID == 0 {
		return nil, 0, ErrCardSecretInvalid
	}
	if s.batchRepo == nil {
		return nil, 0, ErrCardSecretBatchFetchFailed
	}
	items, total, err := s.batchRepo.ListByProduct(productID, page, pageSize)
	if err != nil {
		return nil, 0, ErrCardSecretBatchFetchFailed
	}
	return items, total, nil
}

func normalizeSecrets(values []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, val := range values {
		for _, line := range strings.Split(val, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func parseCSVSecrets(reader io.Reader) ([]string, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	var (
		secrets    []string
		headerRead bool
		secretIdx  = 0
	)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) == 0 {
			continue
		}
		if !headerRead {
			headerRead = true
			skipRow := false
			for i, col := range record {
				if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(col, "\ufeff")), "secret") {
					secretIdx = i
					skipRow = true
					break
				}
			}
			if skipRow {
				continue
			}
		}
		if secretIdx >= len(record) {
			continue
		}
		secret := strings.TrimSpace(strings.TrimPrefix(record[secretIdx], "\ufeff"))
		if secret == "" {
			continue
		}
		secrets = append(secrets, secret)
	}
	return normalizeSecrets(secrets), nil
}

func generateBatchNo() string {
	now := time.Now().Format("20060102150405")
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("BATCH-%s-%04d", now, rng.Intn(10000))
}
