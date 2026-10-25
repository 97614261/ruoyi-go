package repository

import (
	"context"
	"database/sql"
	"fmt"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
)

func erpStockQuery(tx *gorm.DB, f model.ERPQuery) (*gorm.DB, string, error) {
	q := erpWarehouseScope(TenantTable(tx, "erp_stock"), "erp_stock.warehouse_id").Joins("JOIN erp_sku s ON s.sku_id=erp_stock.sku_id").Joins("JOIN erp_warehouse w ON w.warehouse_id=erp_stock.warehouse_id")
	if f.WarehouseID > 0 {
		q = q.Where("erp_stock.warehouse_id=?", f.WarehouseID)
	}
	if f.SkuID > 0 {
		q = q.Where("erp_stock.sku_id=?", f.SkuID)
	}
	columns := "erp_stock.*,s.sku_code,s.product_name,s.unit,w.warehouse_name,erp_stock.on_hand-erp_stock.reserved AS available"
	installed, e := warehouseOperationsInstalled(tx)
	if e != nil {
		return nil, "", e
	}
	if installed {
		q = q.Joins("LEFT JOIN erp_stock_alert a ON a.warehouse_id=erp_stock.warehouse_id AND a.sku_id=erp_stock.sku_id")
		columns += ",a.threshold_quantity,COALESCE(a.version,0) AS alert_version,(a.threshold_quantity IS NOT NULL AND erp_stock.on_hand-erp_stock.reserved<=a.threshold_quantity) AS low_stock"
	} else {
		columns += ",NULL AS threshold_quantity,0 AS alert_version,0 AS low_stock"
		if f.StockState == "LOW" {
			return nil, "", errs.New("预警需要安装仓库增强007")
		}
	}
	switch f.StockState {
	case "HAS_STOCK":
		q = q.Where("erp_stock.on_hand>0")
	case "HAS_RESERVED":
		q = q.Where("erp_stock.reserved>0")
	case "IN_STOCK":
		q = q.Where("erp_stock.on_hand-erp_stock.reserved>0")
	case "ZERO":
		q = q.Where("erp_stock.on_hand=0")
	case "RESERVED_OUT":
		q = q.Where("erp_stock.on_hand>0 AND erp_stock.on_hand=erp_stock.reserved")
	case "LOW":
		q = q.Where("a.threshold_quantity IS NOT NULL AND erp_stock.on_hand-erp_stock.reserved<=a.threshold_quantity")
	}
	return q, columns, nil
}
func erpLedgerQuery(tx *gorm.DB, f model.ERPQuery) (*gorm.DB, string, error) {
	q := erpManagedDocumentScope(erpWarehouseScope(tx.Model(&model.ERPLedger{}), "erp_stock_ledger.warehouse_id"), "erp_stock_ledger.document_id").Joins("JOIN erp_document d ON d.document_id=erp_stock_ledger.document_id").Joins("JOIN erp_document_line l ON l.document_id=erp_stock_ledger.document_id AND l.sku_id=erp_stock_ledger.sku_id")
	if f.WarehouseID > 0 {
		q = q.Where("erp_stock_ledger.warehouse_id=?", f.WarehouseID)
	}
	if f.SkuID > 0 {
		q = q.Where("erp_stock_ledger.sku_id=?", f.SkuID)
	}
	if f.DocumentID > 0 {
		q = q.Where("erp_stock_ledger.document_id=?", f.DocumentID)
	}
	if f.DocumentNo != "" {
		q = q.Where("erp_stock_ledger.document_no=?", f.DocumentNo)
	}
	if f.Event != "" {
		q = q.Where("erp_stock_ledger.event=?", f.Event)
	}
	if f.BeginTime != "" {
		q = q.Where("erp_stock_ledger.create_time>=?", f.BeginTime)
	}
	if f.EndTime != "" {
		q = q.Where("erp_stock_ledger.create_time<=?", f.EndTime)
	}
	columns := "erp_stock_ledger.*,d.warehouse_name,d.kind AS document_kind,l.sku_code,l.product_name,l.unit"
	installed, e := warehouseOperationsInstalled(tx)
	if e != nil {
		return nil, "", e
	}
	if installed {
		q = q.Joins("LEFT JOIN erp_transfer t ON (t.out_document_id=d.document_id OR t.in_document_id=d.document_id)")
		columns += ",t.transfer_id"
	} else {
		columns += ",NULL AS transfer_id"
	}
	return q, columns, nil
}
func ExportERPStocks(ctx context.Context, f model.ERPQuery, limit int) ([]model.ERPStockView, error) {
	q, c, e := erpStockQuery(DB(ctx), f)
	if e != nil {
		return nil, e
	}
	out := []model.ERPStockView{}
	e = q.Select(c).Order("erp_stock.stock_id").Limit(limit).Scan(&out).Error
	return out, e
}
func ExportERPLedger(ctx context.Context, f model.ERPQuery, limit int) ([]model.ERPLedger, error) {
	q, c, e := erpLedgerQuery(DB(ctx), f)
	if e != nil {
		return nil, e
	}
	out := []model.ERPLedger{}
	e = q.Select(c).Order("erp_stock_ledger.ledger_id").Limit(limit).Scan(&out).Error
	return out, e
}

// Only aggregate numbers are projected: warehouse-level reconciliation must
// not accidentally reveal individual orders hidden by the actor's shop scope.
func ReconcileERPStocks(ctx context.Context, f model.ERPQuery, pg page.Query) ([]model.ERPStockReconciliation, int64, error) {
	out := []model.ERPStockReconciliation{}
	var n int64
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		q := erpWarehouseScope(TenantTable(tx, "erp_stock"), "erp_stock.warehouse_id")
		if f.WarehouseID > 0 {
			q = q.Where("warehouse_id=?", f.WarehouseID)
		}
		if f.SkuID > 0 {
			q = q.Where("sku_id=?", f.SkuID)
		}
		if e := q.Count(&n).Error; e != nil {
			return e
		}
		var balances []model.ERPStock
		if e := q.Order("stock_id").Offset(pg.Offset()).Limit(pg.PageSize).Find(&balances).Error; e != nil {
			return e
		}
		stockCount := n
		if len(balances) > 0 {
			// One bounded OR group for at most 100 pairs. GROUP BY cannot leak another
			// warehouse; tenant is explicit inside this grouped query as well.
			l := tx.Model(&model.ERPLedger{}).Where("tenant_id=?", tenantctx.ID(ctx))
			pairs := ""
			args := []any{}
			for i, b := range balances {
				if i > 0 {
					pairs += " OR "
				}
				pairs += "(warehouse_id=? AND sku_id=?)"
				args = append(args, b.WarehouseID, b.SkuID)
			}
			var sums []struct {
				WarehouseID int64
				SkuID       int64
				H           int64
				R           int64
			}
			if e := l.Where("("+pairs+")", args...).Select("warehouse_id,sku_id,SUM(on_hand_delta) AS h,SUM(reserved_delta) AS r").Group("warehouse_id,sku_id").Scan(&sums).Error; e != nil {
				return e
			}
			totals := map[string][2]int64{}
			for _, s := range sums {
				totals[fmt.Sprint(s.WarehouseID, ":", s.SkuID)] = [2]int64{s.H, s.R}
			}
			for _, b := range balances {
				s := totals[fmt.Sprint(b.WarehouseID, ":", b.SkuID)]
				hd, rd := b.OnHand-s[0], b.Reserved-s[1]
				out = append(out, model.ERPStockReconciliation{WarehouseID: b.WarehouseID, SkuID: b.SkuID, OnHand: b.OnHand, Reserved: b.Reserved, LedgerOnHand: s[0], LedgerReserved: s[1], OnHandDifference: hd, ReservedDifference: rd, Consistent: hd == 0 && rd == 0 && b.Reserved >= 0 && b.OnHand >= b.Reserved})
			}
		}
		// FK protects warehouse/SKU/document identities, not the existence of a
		// balance row. Also report ledger pairs whose balance is missing, even
		// when their net sum is zero. Never repair them by inventing a balance.
		missing := func() *gorm.DB {
			q := erpWarehouseScope(TenantTable(tx, "erp_stock_ledger"), "erp_stock_ledger.warehouse_id").Where("NOT EXISTS(SELECT 1 FROM erp_stock ms WHERE ms.tenant_id=? AND ms.warehouse_id=erp_stock_ledger.warehouse_id AND ms.sku_id=erp_stock_ledger.sku_id)", tenantctx.ID(ctx))
			if f.WarehouseID > 0 {
				q = q.Where("erp_stock_ledger.warehouse_id=?", f.WarehouseID)
			}
			if f.SkuID > 0 {
				q = q.Where("erp_stock_ledger.sku_id=?", f.SkuID)
			}
			return q
		}
		var orphanCount int64
		if e := missing().Select("COUNT(DISTINCT erp_stock_ledger.warehouse_id,erp_stock_ledger.sku_id)").Scan(&orphanCount).Error; e != nil {
			return e
		}
		n += orphanCount
		if len(out) < pg.PageSize && orphanCount > 0 {
			offset := int64(pg.Offset()) - stockCount
			if offset < 0 {
				offset = 0
			}
			var orphan []model.ERPStockReconciliation
			if e := missing().Select("erp_stock_ledger.warehouse_id,erp_stock_ledger.sku_id,SUM(on_hand_delta) AS ledger_on_hand,SUM(reserved_delta) AS ledger_reserved,-SUM(on_hand_delta) AS on_hand_difference,-SUM(reserved_delta) AS reserved_difference,1 AS balance_missing,0 AS consistent").Group("erp_stock_ledger.warehouse_id,erp_stock_ledger.sku_id").Order("erp_stock_ledger.warehouse_id,erp_stock_ledger.sku_id").Offset(int(offset)).Limit(pg.PageSize - len(out)).Scan(&orphan).Error; e != nil {
				return e
			}
			out = append(out, orphan...)
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return out, n, e
}
func SelectERPReservations(ctx context.Context, f model.ERPQuery, pg page.Query) ([]model.ERPDocument, int64, error) {
	if f.WarehouseID < 1 || f.SkuID < 1 {
		return nil, 0, errs.New("请选择仓库和SKU")
	}
	q := erpManagedDocumentScope(erpWarehouseScope(DB(ctx).Model(&model.ERPDocument{}), "erp_document.warehouse_id"), "erp_document.document_id").Where("kind='SALE' AND status='RESERVED' AND warehouse_id=?", f.WarehouseID).Where("EXISTS (SELECT 1 FROM erp_document_line rl WHERE rl.tenant_id=? AND rl.document_id=erp_document.document_id AND rl.sku_id=?)", tenantctx.ID(ctx), f.SkuID)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	out := []model.ERPDocument{}
	e := q.Select("document_id,document_no,warehouse_id,warehouse_name,status,version").Order("document_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&out).Error
	return out, n, e
}
