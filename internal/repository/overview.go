package repository

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// Each section gets its own read-only snapshot, including live resource grants.
// Metadata errors are errors, never fabricated module absence or business zero.
func overviewRead(ctx context.Context, tables []string, fn func(*gorm.DB) error) (bool, error) {
	if tenantctx.ID(ctx) < 1 || erpctx.UserID(ctx) < 1 || tenantctx.Get(ctx).Platform {
		return false, errs.New("缺少企业经营概览身份")
	}
	available := false
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if e := tx.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name IN ?", tables).Count(&n).Error; e != nil {
			return e
		}
		if n == 0 {
			return nil
		}
		if n != int64(len(tables)) {
			return errs.New("经营概览依赖模块结构不完整，请检查安装")
		}
		available = true
		return fn(tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return available, e
}
func SelectOverviewOrders(ctx context.Context) (*model.OverviewOrders, error) {
	out := &model.OverviewOrders{}
	a, e := overviewRead(ctx, []string{"erp_order"}, func(tx *gorm.DB) error {
		out.Counts = &model.ERPOrderWorkbench{}
		return orderListQuery(tx, model.ERPOrderQuery{}).Select("COUNT(*) AS total,COALESCE(SUM(status='PENDING_REVIEW'),0) AS pending_review,COALESCE(SUM(status='READY_TO_SHIP'),0) AS ready_to_ship,COALESCE(SUM(status='SHIPPED'),0) AS shipped,COALESCE(SUM(status='CANCELLED'),0) AS cancelled").Scan(out.Counts).Error
	})
	out.Available = a
	out.CheckedAt = types.Now()
	return out, e
}
func SelectOverviewShipments(ctx context.Context, today, seven, through string) (*model.OverviewShipments, error) {
	out := &model.OverviewShipments{TodayFrom: today, SevenDaysFrom: seven, Through: through}
	// 011 may be wholly absent while the older order/package module is present.
	// Its new table marks optional logistics availability; startup independently
	// rejects partial 011 or missing prerequisites, and query errors still fail.
	a, e := overviewRead(ctx, []string{"erp_package_shipping"}, func(tx *gorm.DB) error {
		out.Counts = &model.OverviewShipmentCounts{}
		// Whole seconds are the existing timestamp precision. through is the
		// exclusive next-day boundary, equivalent to inclusive 23:59:59 in UI.
		q := shipmentQuery(tx, model.ShipmentQuery{}).Where("erp_package.shipped_at>=? AND erp_package.shipped_at<?", seven, through)
		return q.Select("COUNT(*) AS seven_days,COALESCE(SUM(erp_package.shipped_at>=?),0) AS today", today).Scan(out.Counts).Error
	})
	out.Available = a
	out.CheckedAt = types.Now()
	return out, e
}
func SelectOverviewStocks(ctx context.Context) (*model.OverviewStocks, error) {
	out := &model.OverviewStocks{}
	a, e := overviewRead(ctx, []string{"erp_stock", "erp_sku", "erp_warehouse"}, func(tx *gorm.DB) error {
		q, _, e := erpStockQuery(tx, model.ERPQuery{})
		if e != nil {
			return e
		}
		alerts, e := warehouseOperationsInstalled(tx)
		if e != nil {
			return e
		}
		columns := "COALESCE(SUM(erp_stock.on_hand>0),0) AS with_stock,COALESCE(SUM(erp_stock.reserved>0),0) AS with_reserved"
		if alerts {
			columns += ",COALESCE(SUM(a.threshold_quantity IS NOT NULL AND erp_stock.on_hand-erp_stock.reserved<=a.threshold_quantity),0) AS low"
		}
		out.Counts = &model.OverviewStockCounts{}
		return q.Select(columns).Scan(out.Counts).Error
	})
	out.Available = a
	out.CheckedAt = types.Now()
	return out, e
}

// Gross receipts are intentional: returning received goods never reopens a PO.
func purchaseAwaiting(q *gorm.DB) *gorm.DB {
	tid := tenantctx.ID(q.Statement.Context)
	return q.Where("erp_purchase_order.status IN ('CONFIRMED','PARTIAL_RECEIVED')").Where("EXISTS (SELECT 1 FROM erp_purchase_order_line ol WHERE ol.tenant_id=? AND ol.purchase_id=erp_purchase_order.purchase_id AND ol.received_quantity<ol.ordered_quantity)", tid)
}
func SelectOverviewPurchases(ctx context.Context) (*model.OverviewPurchases, error) {
	out := &model.OverviewPurchases{}
	a, e := overviewRead(ctx, []string{"erp_purchase_order", "erp_purchase_order_line"}, func(tx *gorm.DB) error {
		var n int64
		if e := purchaseAwaiting(purchaseQuery(tx, model.ERPPurchaseQuery{})).Count(&n).Error; e != nil {
			return e
		}
		out.AwaitingReceipt = &n
		return nil
	})
	out.Available = a
	out.CheckedAt = types.Now()
	return out, e
}
func overviewMoneyQuery(tx *gorm.DB, currency string, sales bool) *gorm.DB {
	var q *gorm.DB
	if sales {
		q = salesFinanceReceivableScope(tx.Model(&model.SalesFinReceivable{}))
	} else {
		q = financePayableScope(tx.Model(&model.FinPayable{}))
	}
	if currency != "" {
		q = q.Where("currency=?", currency)
	}
	return q
}
func SelectOverviewMoney(ctx context.Context, currency string, pg page.Query, sales bool) (*model.OverviewMoneyPage, error) {
	out := &model.OverviewMoneyPage{Rows: []model.OverviewMoney{}}
	table, paid := "erp_fin_payable", "paid_amount"
	if sales {
		table, paid = "erp_sf_receivable", "received_amount"
	}
	a, e := overviewRead(ctx, []string{table}, func(tx *gorm.DB) error {
		if e := overviewMoneyQuery(tx, currency, sales).Group("currency").Count(&out.Total).Error; e != nil {
			return e
		}
		columns := "currency,COUNT(*) AS documents,SUM(GREATEST(gross_amount-credit_amount-" + paid + ",0)) AS due_amount,SUM(GREATEST(" + paid + "-gross_amount+credit_amount,0)) AS excess_amount"
		if e := overviewMoneyQuery(tx, currency, sales).Select(columns).Group("currency").Order("currency ASC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&out.Rows).Error; e != nil {
			return e
		}
		for i := range out.Rows {
			for _, s := range []*string{&out.Rows[i].DueAmount, &out.Rows[i].ExcessAmount} {
				v, e := decimalx.Normalize(*s)
				if e != nil {
					return errs.New("范围内汇总超过金额上限，请按币种缩小范围")
				}
				*s = v
			}
		}
		return nil
	})
	out.Available = a
	out.CheckedAt = types.Now()
	return out, e
}
