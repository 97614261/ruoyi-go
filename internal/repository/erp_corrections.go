package repository

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
)

func erpApplyCount(hand, reserved, target int64) (int64, int64, error) {
	if hand < 0 || reserved < 0 || reserved > hand || hand > model.ERPMaxStock || target < reserved || target > 1000000000 {
		return 0, 0, errs.New("实盘数量不能小于已预占数量，且须在安全范围内")
	}
	return target, reserved, nil
}

// The original document row is the serialization point for all posted returns
// and reversals. Locking reads below deliberately avoid an old RR snapshot.
func validateERPSource(tx *gorm.DB, doc *model.ERPDocument, items []model.ERPLineInput) error {
	return validateERPSourceWithReturn(tx, doc, items, nil)
}

type erpReturnSourceProof interface {
	verify(*gorm.DB, *model.ERPDocument, *model.ERPDocument) error
}

func validateERPSourceWithReturn(tx *gorm.DB, doc *model.ERPDocument, items []model.ERPLineInput, proof erpReturnSourceProof) error {
	if doc.SourceDocumentID == nil {
		return nil
	}
	var source model.ERPDocument
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=?", *doc.SourceDocumentID).Take(&source).Error; err != nil {
		return erpReferenceError(err)
	}
	if err := requireERPWarehouse(tx, source.WarehouseID); err != nil {
		return err
	}
	if err := requireERPDocumentRead(tx, source.DocumentID); err != nil {
		return err
	}
	if doc.Kind == "PR" {
		if proof == nil {
			if err := requireERPPurchaseVoucherIndependent(tx, source.DocumentID); err != nil {
				return err
			}
		} else if err := proof.verify(tx, doc, &source); err != nil {
			return err
		}
	}
	if doc.Kind == "SR" {
		if proof == nil {
			if err := requireERPSalesSourceIndependent(tx, source.DocumentID); err != nil {
				return err
			}
		} else if err := proof.verify(tx, doc, &source); err != nil {
			return err
		}
	}
	if doc.WarehouseID != source.WarehouseID || !((doc.Kind == "SR" && source.Kind == "SALE" && source.Status == "SHIPPED") || (doc.Kind == "PR" && source.Kind == "IN" && source.Status == "POSTED")) {
		return errs.New("原单类型、仓库或状态不允许退货或冲销")
	}
	var original []model.ERPDocumentLine
	if err := tx.Where("document_id=?", source.DocumentID).Order("sku_id").Find(&original).Error; err != nil {
		return err
	}
	quantities := map[int64]int64{}
	for _, line := range original {
		quantities[line.SkuID] = line.Quantity
	}
	var returned []struct {
		SkuID    int64
		Quantity int64
	}
	if err := tx.Model(&model.ERPDocumentLine{}).Joins("JOIN erp_document rd ON rd.document_id=erp_document_line.document_id").Select("erp_document_line.sku_id, SUM(erp_document_line.quantity) AS quantity").Where("rd.source_document_id=? AND rd.status='POSTED'", source.DocumentID).Group("erp_document_line.sku_id").Scan(&returned).Error; err != nil {
		return err
	}
	used := map[int64]int64{}
	for _, line := range returned {
		used[line.SkuID] += line.Quantity
	}
	if doc.Reversal && (len(returned) > 0 || len(items) != len(original)) {
		return errs.New("整单冲销仅允许未退货的原单，且必须包含全部原数量")
	}
	for _, item := range items {
		qty, ok := quantities[item.SkuID]
		if !ok || item.Quantity < 1 || item.Quantity > qty-used[item.SkuID] {
			return errs.New("退货数量超过原单剩余可退数量或商品不在原单中")
		}
		if doc.Reversal && item.Quantity != qty {
			return errs.New("整单冲销必须包含全部原数量")
		}
	}
	return nil
}

func buildERPLines(tx *gorm.DB, doc *model.ERPDocument, items []model.ERPLineInput, skus map[int64]model.ErpSku) ([]model.ERPDocumentLine, error) {
	history := map[int64]model.ERPDocumentLine{}
	if doc.SourceDocumentID != nil {
		var lines []model.ERPDocumentLine
		if err := tx.Where("document_id=?", *doc.SourceDocumentID).Find(&lines).Error; err != nil {
			return nil, err
		}
		for _, line := range lines {
			history[line.SkuID] = line
		}
	}
	balances := map[int64]model.ERPStock{}
	if doc.Kind == "COUNT" {
		ids := make([]int64, len(items))
		for i, item := range items {
			ids[i] = item.SkuID
		}
		var rows []model.ERPStock
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id=? AND sku_id IN ?", doc.WarehouseID, ids).Order("sku_id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			balances[row.SkuID] = row
		}
	}
	result := make([]model.ERPDocumentLine, 0, len(items))
	for _, item := range items {
		sku := skus[item.SkuID]
		line := model.ERPDocumentLine{ERPRecord: doc.ERPRecord, DocumentID: doc.DocumentID, SkuID: item.SkuID, SkuCode: sku.SkuCode, ProductName: sku.ProductName, Specification: sku.Specification, Unit: sku.Unit, Quantity: item.Quantity}
		if old, ok := history[item.SkuID]; ok {
			line.SkuCode = old.SkuCode
			line.ProductName = old.ProductName
			line.Specification = old.Specification
			line.Unit = old.Unit
		}
		if doc.Kind == "COUNT" {
			b := balances[item.SkuID]
			line.ExpectedOnHand = b.OnHand
			line.ExpectedReserved = b.Reserved
			line.ExpectedVersion = b.Version
			if item.Quantity < b.Reserved {
				return nil, errs.New("实盘数量不能小于已预占数量")
			}
		}
		result = append(result, line)
	}
	return result, nil
}

func UpdateERPDocumentDraft(ctx context.Context, input *model.ERPDocument, items []model.ERPLineInput, operator string) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		var hint model.ERPDocument
		if err := tx.Where("document_id=? AND kind=?", input.DocumentID, input.Kind).Take(&hint).Error; err != nil {
			return erpReferenceError(err)
		}
		if err := requireERPDocumentNotManaged(tx, input.DocumentID); err != nil {
			return err
		}
		if err := requireERPWarehouse(tx, hint.WarehouseID); err != nil {
			return err
		}
		if hint.SourceDocumentID != nil {
			var source model.ERPDocument
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=?", *hint.SourceDocumentID).Take(&source).Error; err != nil {
				return erpReferenceError(err)
			}
		}
		var doc model.ERPDocument
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=? AND kind=?", input.DocumentID, input.Kind).Take(&doc).Error; err != nil {
			return erpReferenceError(err)
		}
		if doc.Status != "DRAFT" {
			return errs.New("仅草稿可以编辑，过账后请使用关联退货或冲销")
		}
		if err := requireERPWarehouse(tx, doc.WarehouseID); err != nil {
			return err
		}
		if doc.Version != input.Version {
			return errs.New("单据已被修改，请刷新后重试")
		}
		if (doc.SourceDocumentID == nil) != (input.SourceDocumentID == nil) || (doc.SourceDocumentID != nil && *doc.SourceDocumentID != *input.SourceDocumentID) || doc.Reversal != input.Reversal {
			return errs.New("原单和冲销类型不可变更，请取消后重建")
		}
		if err := requireERPWarehouse(tx, input.WarehouseID); err != nil {
			return err
		}
		if err := validateERPSource(tx, input, items); err != nil {
			return err
		}
		ids := make([]int64, len(items))
		for i, item := range items {
			ids[i] = item.SkuID
		}
		warehouse, skus, err := lockERPReferences(tx, input.WarehouseID, ids, input.SourceDocumentID == nil)
		if err != nil {
			return err
		}
		newRemark := input.Remark
		input.ERPRecord = doc.ERPRecord
		// Preserve creator, but line snapshots are the new draft content.
		input.Remark = newRemark
		lines, err := buildERPLines(tx, input, items, skus)
		if err != nil {
			return err
		}
		if err = tx.Where("document_id=?", doc.DocumentID).Delete(&model.ERPDocumentLine{}).Error; err != nil {
			return err
		}
		if err = tx.Create(&lines).Error; err != nil {
			return err
		}
		return tx.Model(&model.ERPDocument{}).Where("document_id=? AND version=? AND status='DRAFT'", doc.DocumentID, doc.Version).Updates(map[string]any{"warehouse_id": input.WarehouseID, "warehouse_name": warehouse.WarehouseName, "remark": input.Remark, "version": doc.Version + 1, "update_by": operator, "update_time": types.Now()}).Error
	}))
}
