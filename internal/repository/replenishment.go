package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"strconv"
)

func requireReplenishment(tx *gorm.DB) error {
	if e := requireWarehouseOperations(tx); e != nil {
		return e
	}
	var n int64
	if e := tx.WithContext(tenantctx.System(tx.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name IN ('erp_purchase_order','erp_purchase_order_line')").Count(&n).Error; e != nil {
		return e
	}
	if n != 2 {
		return errs.New("补货建议需要显式安装采购004和仓库增强007")
	}
	return nil
}
func replenishmentQuery(tx *gorm.DB, w int64) *gorm.DB {
	return erpWarehouseScope(TenantTable(tx, "erp_stock_alert"), "erp_stock_alert.warehouse_id").
		Joins("JOIN erp_sku s ON s.sku_id=erp_stock_alert.sku_id AND s.status='0'").
		Joins("JOIN erp_warehouse w ON w.warehouse_id=erp_stock_alert.warehouse_id AND w.status='0'").
		Joins("LEFT JOIN erp_stock b ON b.warehouse_id=erp_stock_alert.warehouse_id AND b.sku_id=erp_stock_alert.sku_id").
		Where("erp_stock_alert.warehouse_id=? AND erp_stock_alert.threshold_quantity IS NOT NULL", w)
}

const replenishmentColumns = "erp_stock_alert.alert_id,erp_stock_alert.warehouse_id,erp_stock_alert.sku_id,erp_stock_alert.threshold_quantity,erp_stock_alert.version AS alert_version,s.sku_code,s.product_name,s.unit,w.warehouse_name,COALESCE(b.on_hand,0) AS on_hand,COALESCE(b.reserved,0) AS reserved"

// All aggregations are restricted to one authorized warehouse and <=100 SKUs.
// No price, supplier contact, other warehouse, or individual purchase is exposed.
func fillReplenishments(tx *gorm.DB, w int64, rows []model.ERPReplenishment) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.SkuID
	}
	var sums []struct {
		SkuID            int64
		DraftQuantity    string
		IncomingQuantity string
	}
	e := tx.Model(&model.ERPPurchaseLine{}).Joins("JOIN erp_purchase_order p ON p.purchase_id=erp_purchase_order_line.purchase_id").
		Where("p.warehouse_id=? AND erp_purchase_order_line.sku_id IN ? AND p.status IN ('DRAFT','CONFIRMED','PARTIAL_RECEIVED')", w, ids).
		Select("erp_purchase_order_line.sku_id,SUM(CASE WHEN p.status='DRAFT' THEN ordered_quantity ELSE 0 END) AS draft_quantity,SUM(CASE WHEN p.status IN ('CONFIRMED','PARTIAL_RECEIVED') THEN GREATEST(ordered_quantity-received_quantity,0) ELSE 0 END) AS incoming_quantity").
		Group("erp_purchase_order_line.sku_id").Scan(&sums).Error
	if e != nil {
		return e
	}
	byID := map[int64][2]int64{}
	for _, s := range sums {
		d, e1 := strconv.ParseInt(s.DraftQuantity, 10, 64)
		n, e2 := strconv.ParseInt(s.IncomingQuantity, 10, 64)
		if e1 != nil || e2 != nil || d < 0 || n < 0 || d > 9007199254740991 || n > 9007199254740991 {
			return errs.New("采购待补数量超出支持范围，请核对原单")
		}
		byID[s.SkuID] = [2]int64{d, n}
	}
	for i := range rows {
		r := &rows[i]
		s := byID[r.SkuID]
		r.DraftQuantity = s[0]
		r.IncomingQuantity = s[1]
		if e = finishReplenishment(tenantctx.ID(tx.Statement.Context), r); e != nil {
			return e
		}
	}
	return nil
}
func finishReplenishment(tid int64, r *model.ERPReplenishment) error {
	if r.OnHand < 0 || r.Reserved < 0 || r.Reserved > r.OnHand || r.OnHand > 1000000000000 || r.ThresholdQuantity < 0 || r.ThresholdQuantity > 1000000000000 || r.DraftQuantity < 0 || r.IncomingQuantity < 0 || r.DraftQuantity > 9007199254740991 || r.IncomingQuantity > 9007199254740991 {
		return errs.New("库存或预警数量异常，请先核对")
	}
	r.Available = r.OnHand - r.Reserved
	// Alerts trigger at <=threshold: the minimum target that clears the alert is +1.
	r.TargetQuantity = r.ThresholdQuantity + 1
	r.NeededQuantity = r.TargetQuantity - r.Available - r.DraftQuantity - r.IncomingQuantity
	if r.NeededQuantity < 0 {
		r.NeededQuantity = 0
	}
	r.SuggestedQuantity = r.NeededQuantity
	if r.SuggestedQuantity > 1000000000 {
		r.SuggestedQuantity = 1000000000
	}
	raw, e := json.Marshal([]any{tid, r.WarehouseID, r.SkuID, r.Unit, r.ThresholdQuantity, r.AlertVersion, r.OnHand, r.Reserved, r.DraftQuantity, r.IncomingQuantity})
	if e != nil {
		return e
	}
	r.Basis = fmt.Sprintf("%x", sha256.Sum256(raw))
	return nil
}
func SelectERPReplenishments(ctx context.Context, q model.ERPReplenishmentQuery, pg page.Query) ([]model.ERPReplenishment, int64, error) {
	rows := []model.ERPReplenishment{}
	var total int64
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := requireERPWarehouse(tx, q.WarehouseID); e != nil {
			return e
		}
		if e := requireReplenishment(tx); e != nil {
			return e
		}
		query := func() *gorm.DB {
			v := replenishmentQuery(tx, q.WarehouseID)
			if q.SkuID > 0 {
				v = v.Where("erp_stock_alert.sku_id=?", q.SkuID)
			}
			return v
		}
		if e := query().Count(&total).Error; e != nil {
			return e
		}
		if e := query().Select(replenishmentColumns).Order("erp_stock_alert.alert_id").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error; e != nil {
			return e
		}
		return fillReplenishments(tx, q.WarehouseID, rows)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return rows, total, e
}
func CreateERPReplenishmentPurchase(ctx context.Context, in model.ERPReplenishmentWrite, hash, operator string) (*model.ERPPurchaseResult, error) {
	return createERPPurchase(ctx, in.ERPPurchaseWrite, hash, operator, func(tx *gorm.DB) error {
		if e := requireReplenishment(tx); e != nil {
			return e
		}
		ids := make([]int64, len(in.Basis))
		wanted := map[int64]string{}
		for i, b := range in.Basis {
			ids[i] = b.SkuID
			wanted[b.SkuID] = b.Basis
		}
		rows := []model.ERPReplenishment{}
		if e := replenishmentQuery(tx, in.WarehouseID).Where("erp_stock_alert.sku_id IN ?", ids).Select(replenishmentColumns).Order("erp_stock_alert.alert_id").Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) != len(ids) {
			return errs.New("补货建议已变化，请关闭后刷新重新核对")
		}
		if e := fillReplenishments(tx, in.WarehouseID, rows); e != nil {
			return e
		}
		for _, r := range rows {
			if r.NeededQuantity < 1 || wanted[r.SkuID] != r.Basis {
				return errs.New("补货建议已变化，请关闭后刷新重新核对")
			}
		}
		return nil
	})
}
