package service

import (
	"strings"

	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
)

// BuildFulfillmentBatchRemarkMap 构建订单树（父/子订单）的批次说明映射。
func (s *OrderService) BuildFulfillmentBatchRemarkMap(order *models.Order) map[uint]string {
	remarksByOrderID := make(map[uint]string)
	if s == nil || order == nil {
		return remarksByOrderID
	}
	s.buildFulfillmentBatchRemarkMapRecursive(order, remarksByOrderID)
	return remarksByOrderID
}

func (s *OrderService) buildFulfillmentBatchRemarkMapRecursive(order *models.Order, remarksByOrderID map[uint]string) {
	if order == nil || order.ID == 0 {
		return
	}
	if _, exists := remarksByOrderID[order.ID]; !exists {
		remark, err := s.resolveOrderFulfillmentBatchRemark(order.ID)
		if err != nil {
			logger.Warnw("order_fulfillment_batch_remark_resolve_failed",
				"order_id", order.ID,
				"order_no", order.OrderNo,
				"error", err,
			)
		} else {
			remarksByOrderID[order.ID] = remark
		}
	}
	for idx := range order.Children {
		s.buildFulfillmentBatchRemarkMapRecursive(&order.Children[idx], remarksByOrderID)
	}
}

func (s *OrderService) resolveOrderFulfillmentBatchRemark(orderID uint) (string, error) {
	if orderID == 0 || s.cardSecretRepo == nil || s.cardSecretBatchRepo == nil {
		return "", nil
	}

	rows, err := s.cardSecretRepo.ListByOrderAndStatus(orderID, models.CardSecretStatusUsed)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}

	batchIDs := make([]uint, 0, len(rows))
	seenBatch := make(map[uint]struct{}, len(rows))
	for _, row := range rows {
		if row.BatchID == nil || *row.BatchID == 0 {
			continue
		}
		batchID := *row.BatchID
		if _, ok := seenBatch[batchID]; ok {
			continue
		}
		seenBatch[batchID] = struct{}{}
		batchIDs = append(batchIDs, batchID)
	}
	if len(batchIDs) == 0 {
		return "", nil
	}

	remarks := make([]string, 0, len(batchIDs))
	seenRemark := make(map[string]struct{}, len(batchIDs))
	for _, batchID := range batchIDs {
		batch, batchErr := s.cardSecretBatchRepo.GetByID(batchID)
		if batchErr != nil {
			return "", batchErr
		}
		if batch == nil {
			continue
		}
		remark := strings.TrimSpace(batch.Remark)
		if remark == "" {
			continue
		}
		if _, ok := seenRemark[remark]; ok {
			continue
		}
		seenRemark[remark] = struct{}{}
		remarks = append(remarks, remark)
	}
	if len(remarks) == 0 {
		return "", nil
	}
	return strings.Join(remarks, "\n"), nil
}
