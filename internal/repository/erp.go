package repository

import (
	"context"
	"errors"
	"fmt"
	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func erpError(err error) error {
	if err == nil {
		return nil
	}
	var my *drivermysql.MySQLError
	if errors.As(err, &my) {
		switch my.Number {
		case 1062:
			return errs.New("编码或请求标识已存在，请刷新后重试")
		case 1451:
			return errs.New("数据已被单据或库存引用，不能删除")
		case 1452:
			return errs.New("关联数据不存在或不属于当前企业")
		}
	}
	return err
}

// lockERPReferences follows warehouse -> sorted SKU order in all stock paths.
// SKU rows are fetched once, not in a per-line query loop.
func lockERPReferences(tx *gorm.DB, warehouseID int64, skuIDs []int64, active bool) (model.ErpWarehouse, map[int64]model.ErpSku, error) {
	var warehouse model.ErpWarehouse
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id=?", warehouseID).Take(&warehouse).Error
	if err != nil {
		return warehouse, nil, erpReferenceError(err)
	}
	if active && warehouse.Status != "0" {
		return warehouse, nil, errs.New("仓库已停用")
	}
	var skus []model.ErpSku
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sku_id IN ?", skuIDs).Order("sku_id").Find(&skus).Error
	if err != nil {
		return warehouse, nil, err
	}
	if len(skus) != len(skuIDs) {
		return warehouse, nil, errs.New("商品不存在或不属于当前企业")
	}
	result := make(map[int64]model.ErpSku, len(skus))
	for _, sku := range skus {
		if active && sku.Status != "0" {
			return warehouse, nil, errs.New("商品已停用")
		}
		result[sku.SkuId] = sku
	}
	return warehouse, result, nil
}
func erpReferenceError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errs.New("数据不存在或不属于当前企业")
	}
	return err
}

func InsertERPDocument(ctx context.Context, doc *model.ERPDocument, items []model.ERPLineInput) error {
	err := Transaction(ctx, func(tx *gorm.DB) error {
		ids := make([]int64, len(items))
		for i, item := range items {
			ids[i] = item.SkuID
		}
		warehouse, skus, err := lockERPReferences(tx, doc.WarehouseID, ids, true)
		if err != nil {
			return err
		}
		doc.WarehouseName = warehouse.WarehouseName
		var existing model.ERPDocument
		err = tx.Where("request_key=?", doc.RequestKey).Take(&existing).Error
		if err == nil {
			if existing.RequestHash != doc.RequestHash {
				return errs.New("同一请求标识不能用于不同单据内容")
			}
			*doc = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err = tx.Omit("Lines").Create(doc).Error; err != nil {
			return err
		}
		lines := make([]model.ERPDocumentLine, 0, len(items))
		for _, item := range items {
			sku := skus[item.SkuID]
			lines = append(lines, model.ERPDocumentLine{ERPRecord: doc.ERPRecord, DocumentID: doc.DocumentID, SkuID: item.SkuID, SkuCode: sku.SkuCode, ProductName: sku.ProductName, Specification: sku.Specification, Unit: sku.Unit, Quantity: item.Quantity})
		}
		if err = tx.Create(&lines).Error; err != nil {
			return err
		}
		doc.Lines = lines
		return nil
	})
	// Different warehouses may race on the same idempotency key. The unique key
	// wins across processes; re-read only after the losing transaction rolls back.
	var my *drivermysql.MySQLError
	if errors.As(err, &my) && my.Number == 1062 {
		var existing model.ERPDocument
		if readErr := DB(ctx).Where("request_key=?", doc.RequestKey).Take(&existing).Error; readErr == nil {
			if existing.RequestHash != doc.RequestHash {
				return errs.New("同一请求标识不能用于不同单据内容")
			}
			*doc = existing
			return nil
		}
	}
	return erpError(err)
}

func SelectERPDocument(ctx context.Context, kind string, id int64) (*model.ERPDocument, error) {
	var doc model.ERPDocument
	if err := DB(ctx).Where("document_id=? AND kind=?", id, kind).Take(&doc).Error; err != nil {
		return nil, erpReferenceError(err)
	}
	if err := DB(ctx).Where("document_id=?", id).Order("sku_id").Find(&doc.Lines).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}
func SelectERPDocuments(ctx context.Context, kind string, filter model.ERPQuery, pg page.Query) ([]model.ERPDocument, int64, error) {
	q := DB(ctx).Model(&model.ERPDocument{}).Where("kind=?", kind)
	if filter.Status != "" {
		q = q.Where("status=?", filter.Status)
	}
	if filter.DocumentNo != "" {
		q = q.Where("document_no=?", filter.DocumentNo)
	}
	if filter.WarehouseID > 0 {
		q = q.Where("warehouse_id=?", filter.WarehouseID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPDocument{}
	err := q.Order("document_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, count, err
}

// TransitionERPDocument locks the document and all balances, then writes the
// balances, immutable inventory ledger, and state in ONE database transaction.
func TransitionERPDocument(ctx context.Context, kind string, id int64, action, operator string) error {
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		var doc model.ERPDocument
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=? AND kind=?", id, kind).Take(&doc).Error; err != nil {
			return erpReferenceError(err)
		}
		next, event, err := erpNextState(kind, doc.Status, action)
		if err != nil {
			return err
		}
		if next == doc.Status {
			return nil
		}
		if err = tx.Where("document_id=?", id).Order("sku_id").Find(&doc.Lines).Error; err != nil {
			return err
		}
		if len(doc.Lines) == 0 || len(doc.Lines) > 100 {
			return errs.New("单据明细异常")
		}
		if event != "" {
			ids := make([]int64, len(doc.Lines))
			for i, line := range doc.Lines {
				ids[i] = line.SkuID
			}
			if _, _, err = lockERPReferences(tx, doc.WarehouseID, ids, action != "cancel"); err != nil {
				return err
			}
			balances := make([]model.ERPStock, len(ids))
			for i, skuID := range ids {
				balances[i] = model.ERPStock{WarehouseID: doc.WarehouseID, SkuID: skuID, ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}}
			}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&balances).Error; err != nil {
				return err
			}
			balances = nil
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id=? AND sku_id IN ?", doc.WarehouseID, ids).Order("sku_id").Find(&balances).Error; err != nil {
				return err
			}
			if len(balances) != len(doc.Lines) {
				return errs.New("库存余额缺失")
			}
			ledger := make([]model.ERPLedger, 0, len(balances))
			for i, balance := range balances {
				line := doc.Lines[i]
				if line.SkuID != balance.SkuID || line.Quantity < 1 || line.Quantity > 1000000000 {
					return errs.New("单据数量异常")
				}
				hand, reserved, err := erpApplyBalance(balance.OnHand, balance.Reserved, line.Quantity, event)
				if err != nil {
					return err
				}
				if err = tx.Model(&model.ERPStock{}).Where("stock_id=?", balance.StockID).Updates(map[string]any{"on_hand": hand, "reserved": reserved, "update_by": operator, "update_time": types.Now()}).Error; err != nil {
					return err
				}
				ledger = append(ledger, model.ERPLedger{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, DocumentID: id, DocumentNo: doc.DocumentNo, WarehouseID: doc.WarehouseID, SkuID: line.SkuID, Event: event, OnHandDelta: hand - balance.OnHand, ReservedDelta: reserved - balance.Reserved, OnHandAfter: hand, ReservedAfter: reserved})
			}
			if err = tx.Create(&ledger).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.ERPDocument{}).Where("document_id=?", id).Updates(map[string]any{"status": next, "update_by": operator, "update_time": types.Now()}).Error
	}))
}
func erpNextState(kind, status, action string) (string, string, error) {
	switch action {
	case "confirm":
		if kind == "IN" {
			if status == "POSTED" {
				return status, "", nil
			}
			if status == "DRAFT" {
				return "POSTED", "IN", nil
			}
		}
		if kind == "SALE" {
			if status == "RESERVED" || status == "SHIPPED" {
				return status, "", nil
			}
			if status == "DRAFT" {
				return "RESERVED", "RESERVE", nil
			}
		}
	case "ship":
		if kind == "SALE" {
			if status == "SHIPPED" {
				return status, "", nil
			}
			if status == "RESERVED" {
				return "SHIPPED", "OUT", nil
			}
		}
	case "cancel":
		if status == "CANCELLED" {
			return status, "", nil
		}
		if status == "DRAFT" {
			return "CANCELLED", "", nil
		}
		if kind == "SALE" && status == "RESERVED" {
			return "CANCELLED", "RELEASE", nil
		}
	}
	return "", "", errs.New("当前单据状态不允许该操作；已入库或已出库单据不能直接修改或取消")
}
func erpApplyBalance(hand, reserved, qty int64, event string) (int64, int64, error) {
	if hand < 0 || reserved < 0 || reserved > hand || hand > model.ERPMaxStock || qty < 1 || qty > 1000000000 {
		return 0, 0, errs.New("库存或单据数量异常")
	}
	switch event {
	case "IN":
		if hand > model.ERPMaxStock-qty {
			return 0, 0, errs.New("库存超过安全上限")
		}
		hand += qty
	case "RESERVE":
		if hand-reserved < qty {
			return 0, 0, errs.New("可用库存不足")
		}
		reserved += qty
	case "RELEASE":
		if reserved < qty {
			return 0, 0, errs.New("预占库存异常")
		}
		reserved -= qty
	case "OUT":
		if reserved < qty || hand < qty {
			return 0, 0, errs.New("预占库存或实物库存不足")
		}
		hand -= qty
		reserved -= qty
	default:
		return 0, 0, fmt.Errorf("unknown stock event")
	}
	return hand, reserved, nil
}

func SelectERPStocks(ctx context.Context, filter model.ERPQuery, pg page.Query) ([]model.ERPStockView, int64, error) {
	q := TenantTable(DB(ctx), "erp_stock")
	if filter.WarehouseID > 0 {
		q = q.Where("erp_stock.warehouse_id=?", filter.WarehouseID)
	}
	if filter.SkuID > 0 {
		q = q.Where("erp_stock.sku_id=?", filter.SkuID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPStockView{}
	err := q.Joins("JOIN erp_sku s ON s.sku_id=erp_stock.sku_id").Joins("JOIN erp_warehouse w ON w.warehouse_id=erp_stock.warehouse_id").Select("erp_stock.*, s.sku_code, s.product_name, s.unit, w.warehouse_name, erp_stock.on_hand-erp_stock.reserved AS available").Order("erp_stock.stock_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, count, err
}
func SelectERPLedger(ctx context.Context, filter model.ERPQuery, pg page.Query) ([]model.ERPLedger, int64, error) {
	q := DB(ctx).Model(&model.ERPLedger{})
	if filter.WarehouseID > 0 {
		q = q.Where("warehouse_id=?", filter.WarehouseID)
	}
	if filter.SkuID > 0 {
		q = q.Where("sku_id=?", filter.SkuID)
	}
	if filter.DocumentID > 0 {
		q = q.Where("document_id=?", filter.DocumentID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPLedger{}
	err := q.Order("ledger_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, count, err
}
