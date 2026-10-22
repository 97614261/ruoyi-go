package repository

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
	"strings"
)

func SelectERPOrderWorkbench(ctx context.Context, q model.ERPOrderQuery) (*model.ERPOrderWorkbench, error) {
	// Deliberately ignore the selected status: these counts are the status tabs
	// for the same shop/source filters, not a second view of only the active tab.
	q.Status = ""
	var groups []struct {
		Status string
		Total  int64
	}
	if err := orderListQuery(DB(ctx), q).Select("status,COUNT(*) AS total").Group("status").Scan(&groups).Error; err != nil {
		return nil, err
	}
	out := &model.ERPOrderWorkbench{}
	for _, g := range groups {
		out.Total += g.Total
		switch g.Status {
		case "PENDING_REVIEW":
			out.PendingReview = g.Total
		case "READY_TO_SHIP":
			out.ReadyToShip = g.Total
		case "SHIPPED":
			out.Shipped = g.Total
		case "CANCELLED":
			out.Cancelled = g.Total
		}
	}
	return out, nil
}

func CheckERPOrderReadiness(ctx context.Context, id, warehouse int64) (*model.ERPOrderReadiness, error) {
	out := &model.ERPOrderReadiness{OrderID: id, WarehouseID: warehouse, Issues: []model.ERPReviewIssue{}, Stocks: []model.ERPReviewStock{}}
	issue := func(code, message string) {
		out.Issues = append(out.Issues, model.ERPReviewIssue{Code: code, Message: message})
	}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var o model.ERPOrder
		if err := orderScope(tx.Model(&o)).Select("order_id,shop_id,status,version,warehouse_id,receiver_name,receiver_country,receiver_address").Where("order_id=?", id).Take(&o).Error; err != nil {
			return erpReferenceError(err)
		}
		if err := requireERPWarehouse(tx, warehouse); err != nil {
			return err
		}
		// A different authorized selection must not reveal state/version of an
		// already-reviewed order assigned to an unauthorized warehouse.
		if o.WarehouseID != nil && *o.WarehouseID != warehouse {
			if err := requireERPWarehouse(tx, *o.WarehouseID); err != nil {
				return err
			}
		}
		out.Version, out.Status = o.Version, o.Status
		var shop model.ErpShop
		if err := tx.Select("shop_id,status").Where("shop_id=?", o.ShopID).Take(&shop).Error; err != nil {
			return erpReferenceError(err)
		}
		var wh model.ErpWarehouse
		if err := tx.Select("warehouse_id,status").Where("warehouse_id=?", warehouse).Take(&wh).Error; err != nil {
			return erpReferenceError(err)
		}
		if o.Status != "PENDING_REVIEW" {
			issue("ORDER_STATUS", "订单已不在待审核状态，请刷新订单")
			return nil
		}
		if shop.Status != "0" {
			issue("SHOP_DISABLED", "店铺已停用")
		}
		if wh.Status != "0" {
			issue("WAREHOUSE_DISABLED", "仓库已停用")
		}
		if o.Version < 1 || o.Version == math.MaxInt64 {
			issue("VERSION", "订单版本异常或已到上限")
		}
		if strings.TrimSpace(o.ReceiverName) == "" || strings.TrimSpace(o.ReceiverAddress) == "" || len(o.ReceiverCountry) != 2 {
			issue("RECIPIENT", "请先完善订单收件信息")
		}
		// Reuse the actual review resolver, including aggregate limits. Only its
		// explicitly public, cause-free validation errors become diagnostic issues.
		lines, err := orderLinesForReview(tx, &o)
		if err != nil {
			if b := errs.As(err); b != nil && b.Cause == nil {
				issue("LINES", b.Msg)
				return nil
			}
			return err
		}
		ids := make([]int64, 0, len(lines))
		for _, l := range lines {
			ids = append(ids, l.SkuID)
		}
		var skus []model.ErpSku
		if err = tx.Where("sku_id IN ?", ids).Limit(100).Find(&skus).Error; err != nil {
			return err
		}
		byID := map[int64]model.ErpSku{}
		for _, s := range skus {
			byID[s.SkuId] = s
		}
		var stocks []model.ERPStock
		if err = tx.Where("warehouse_id=? AND sku_id IN ?", warehouse, ids).Limit(100).Find(&stocks).Error; err != nil {
			return err
		}
		available := map[int64]int64{}
		for _, s := range stocks {
			available[s.SkuID] = s.OnHand - s.Reserved
		}
		for _, l := range lines {
			s, ok := byID[l.SkuID]
			if !ok || s.Status != "0" {
				issue("SKU_DISABLED", "存在已停用或不可用的内部SKU")
			}
			row := model.ERPReviewStock{SkuID: l.SkuID, SkuCode: s.SkuCode, ProductName: s.ProductName, Unit: s.Unit, Required: l.Quantity, Available: available[l.SkuID]}
			if row.Required > row.Available {
				row.Shortage = row.Required - row.Available
				issue("STOCK_SHORTAGE", "所选仓库可用库存不足，请查看缺口明细")
			}
			out.Stocks = append(out.Stocks, row)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	out.CheckedAt = types.Now()
	out.Ready = len(out.Issues) == 0
	return out, err
}
