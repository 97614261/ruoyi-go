package repository

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func salesFinanceRead(ctx context.Context, fn func(*gorm.DB) error) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := requireSalesFinance(tx); e != nil {
			return e
		}
		return fn(tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
func salesFinanceOrderScope(tx *gorm.DB) *gorm.DB {
	return erpWarehouseScope(orderScope(tx), "erp_order.warehouse_id")
}
func lockSalesFinShop(tx *gorm.DB, id int64) (model.ErpShop, error) {
	var s model.ErpShop
	e := erpShopColumnScope(tx.Model(&s), "erp_shop.shop_id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("shop_id=?", id).Take(&s).Error
	return s, erpReferenceError(e)
}
func lockSalesFinOrder(tx *gorm.DB, id int64) (model.ERPOrder, model.ErpShop, error) {
	var hint model.ERPOrder
	if e := salesFinanceOrderScope(tx.Model(&hint)).Where("order_id=?", id).Take(&hint).Error; e != nil {
		return hint, model.ErpShop{}, erpReferenceError(e)
	}
	s, e := lockSalesFinShop(tx, hint.ShopID)
	if e != nil {
		return hint, s, e
	}
	var o model.ERPOrder
	e = salesFinanceOrderScope(tx.Model(&o)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id=? AND shop_id=?", id, s.ShopId).Take(&o).Error
	return o, s, erpReferenceError(e)
}
func lockSalesFinOrigins(tx *gorm.DB, ids []int64) error {
	var refs []model.SalesFinReceivable
	if e := salesFinanceReceivableScope(tx.Model(&model.SalesFinReceivable{})).Select("order_id").Where("receivable_id IN ?", ids).Find(&refs).Error; e != nil {
		return e
	}
	if len(refs) != len(ids) {
		return errs.New("缺少应收或店铺/仓库范围")
	}
	orders := make([]int64, len(refs))
	for i, r := range refs {
		orders[i] = r.OrderID
	}
	var rows []model.ERPOrder
	if e := salesFinanceOrderScope(tx.Model(&model.ERPOrder{})).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id IN ?", orders).Order("order_id").Find(&rows).Error; e != nil {
		return e
	}
	if len(rows) != len(ids) {
		return errs.New("原订单或范围异常")
	}
	return nil
}
func salesFinanceSourceQuery(tx *gorm.DB, q model.SalesFinQuery) *gorm.DB {
	r := salesFinanceOrderScope(tx.Model(&model.ERPOrder{})).Joins("JOIN erp_shop ss ON ss.tenant_id=erp_order.tenant_id AND ss.shop_id=erp_order.shop_id").Where("erp_order.status='SHIPPED'").Where("NOT EXISTS (SELECT 1 FROM erp_sf_receivable r WHERE r.tenant_id=? AND r.order_id=erp_order.order_id)", tenantctx.ID(tx.Statement.Context))
	if q.OrderID > 0 {
		r = r.Where("erp_order.order_id=?", q.OrderID)
	}
	if q.ShopID > 0 {
		r = r.Where("erp_order.shop_id=?", q.ShopID)
	}
	if q.WarehouseID > 0 {
		r = r.Where("erp_order.warehouse_id=?", q.WarehouseID)
	}
	if q.Currency != "" {
		r = r.Where("erp_order.currency=?", q.Currency)
	}
	if q.Code != "" {
		r = r.Where("erp_order.order_no=?", q.Code)
	}
	return r
}
func SelectSalesFinSources(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinSource, int64, error) {
	rows := []model.SalesFinSource{}
	var n int64
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		if e := salesFinanceSourceQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		return salesFinanceSourceQuery(tx, q).Select("erp_order.order_id,erp_order.order_no,erp_order.shop_id,ss.shop_name,erp_order.warehouse_id,erp_order.currency").Order("erp_order.order_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	})
	return rows, n, e
}
func salesFinSource(tx *gorm.DB, o model.ERPOrder, s model.ErpShop) (model.SalesFinSource, error) {
	out := model.SalesFinSource{}
	if o.Status != "SHIPPED" || o.WarehouseID == nil || o.OriginalAmount == nil {
		return out, errs.New("仅已发货且有明确原订单总额的订单可登记；缺金额不当作零")
	}
	total, e := decimalx.Normalize(*o.OriginalAmount)
	if e != nil {
		return out, errs.New("原订单总额异常")
	}
	var p model.ERPPackage
	if e = tx.Where("order_id=? AND status='SHIPPED' AND warehouse_id=?", o.OrderID, *o.WarehouseID).Take(&p).Error; e != nil {
		return out, erpReferenceError(e)
	}
	var original []model.ERPOrderLine
	if e = tx.Where("order_id=? AND shop_id=?", o.OrderID, o.ShopID).Order("line_id").Limit(101).Find(&original).Error; e != nil {
		return out, e
	}
	if len(original) < 1 || len(original) > 100 {
		return out, errs.New("原订单明细异常")
	}
	lines := make([]model.SalesFinReceivableLine, 0, len(original))
	for _, l := range original {
		if l.SkuCode == nil || l.ProductName == nil || l.Unit == nil || l.Quantity < 1 {
			return out, errs.New("原发货快照缺失")
		}
		lines = append(lines, model.SalesFinReceivableLine{OrderID: o.OrderID, OrderLineID: l.LineID, SkuCode: *l.SkuCode, ProductName: *l.ProductName, Unit: *l.Unit, Quantity: l.Quantity})
	}
	out = model.SalesFinSource{OrderID: o.OrderID, OrderNo: o.OrderNo, PackageID: p.PackageID, ShopID: o.ShopID, ShopName: s.ShopName, WarehouseID: *o.WarehouseID, Currency: o.Currency, GrossAmount: total, Lines: lines}
	return out, nil
}
func SelectSalesFinSource(ctx context.Context, id int64) (*model.SalesFinSource, error) {
	var out model.SalesFinSource
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		var o model.ERPOrder
		if e := salesFinanceOrderScope(tx.Model(&o)).Where("order_id=?", id).Take(&o).Error; e != nil {
			return erpReferenceError(e)
		}
		var s model.ErpShop
		if e := tx.Where("shop_id=?", o.ShopID).Take(&s).Error; e != nil {
			return erpReferenceError(e)
		}
		var e error
		out, e = salesFinSource(tx, o, s)
		return e
	})
	return &out, e
}
func CreateSalesFinReceivable(ctx context.Context, in model.SalesFinReceivableCreate, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireSalesFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		o, s, e := lockSalesFinOrder(tx, in.OrderID)
		if e != nil {
			return e
		}
		r, e := salesFinanceReplay(tx, "RECEIVABLE_CREATE", 0, in.RequestKey, hash, "receivable")
		if e != nil {
			return e
		}
		if r != nil {
			var h model.SalesFinReceivable
			if e := salesFinanceReceivableScope(tx.Model(&h)).Where("receivable_id=? AND order_id=?", r.ID, o.OrderID).Take(&h).Error; e != nil {
				return erpReferenceError(e)
			}
			out = r
			return nil
		}
		var n int64
		if e = tx.Model(&model.SalesFinReceivable{}).Where("order_id=?", o.OrderID).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return errs.New("原订单已登记应收，请查看原记录")
		}
		source, e := salesFinSource(tx, o, s)
		if e != nil {
			return e
		}
		h := model.SalesFinReceivable{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ReceivableNo: "AR-" + uuid.NewString(), OrderID: o.OrderID, PackageID: source.PackageID, ShopID: o.ShopID, ShopName: s.ShopName, OrderNo: o.OrderNo, WarehouseID: source.WarehouseID, Currency: o.Currency, GrossAmount: source.GrossAmount, CreditAmount: "0.0000", ReceivedAmount: "0.0000", Version: 1}
		if e = tx.Create(&h).Error; e != nil {
			return e
		}
		for i := range source.Lines {
			source.Lines[i].ReceivableID = h.ReceivableID
			source.Lines[i].ERPRecord = model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}
		}
		if e = tx.Create(&source.Lines).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: h.ReceivableID, Version: 1, Kind: "receivable", Status: "REGISTERED"}
		return salesFinanceRecord(tx, "RECEIVABLE_CREATE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
func pendingSalesFinReturns(tx *gorm.DB, orders []int64) *gorm.DB {
	return tx.Model(&model.ERPSalesReturn{}).Where("order_id IN ?", orders).Where("NOT EXISTS (SELECT 1 FROM erp_sf_credit c WHERE c.tenant_id=? AND c.return_id=erp_sales_return_order.return_id)", tenantctx.ID(tx.Statement.Context))
}
func fillSalesFinProgress(tx *gorm.DB, rows []model.SalesFinReceivable) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].OrderID
		if e := salesFinanceReceivableTotals(&rows[i]); e != nil {
			return e
		}
	}
	var counts []struct {
		OrderID int64
		N       int64
	}
	if e := pendingSalesFinReturns(tx, ids).Select("order_id,COUNT(*) n").Group("order_id").Scan(&counts).Error; e != nil {
		return e
	}
	m := map[int64]int64{}
	for _, c := range counts {
		m[c.OrderID] = c.N
	}
	for i := range rows {
		rows[i].PendingReturnCount = m[rows[i].OrderID]
	}
	return nil
}
func receivableSalesFinLines(tx *gorm.DB, id int64) ([]model.SalesFinReceivableLine, error) {
	out := []model.SalesFinReceivableLine{}
	e := tx.Where("receivable_id=?", id).Order("line_id").Limit(101).Find(&out).Error
	if e == nil && (len(out) < 1 || len(out) > 100) {
		e = errs.New("原应收行记录异常")
	}
	return out, e
}
func SelectSalesFinReturns(ctx context.Context, id int64, pending bool, pg page.Query) ([]model.SalesFinCredit, int64, error) {
	out := []model.SalesFinCredit{}
	var n int64
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		var p model.SalesFinReceivable
		if e := salesFinanceReceivableScope(tx.Model(&p)).Where("receivable_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		query := func() *gorm.DB {
			if pending {
				return pendingSalesFinReturns(tx, []int64{p.OrderID})
			}
			return tx.Model(&model.ERPSalesReturn{}).Where("order_id=? AND package_id=?", p.OrderID, p.PackageID)
		}
		if e := query().Count(&n).Error; e != nil {
			return e
		}
		var heads []model.ERPSalesReturn
		if e := query().Order("return_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&heads).Error; e != nil {
			return e
		}
		ids := []int64{}
		for _, h := range heads {
			ids = append(ids, h.ReturnID)
		}
		var credits []model.SalesFinCredit
		if len(ids) > 0 {
			if e := tx.Where("receivable_id=? AND return_id IN ?", id, ids).Find(&credits).Error; e != nil {
				return e
			}
		}
		seen := map[int64]model.SalesFinCredit{}
		for _, c := range credits {
			seen[c.ReturnID] = c
		}
		for _, h := range heads {
			c, ok := seen[h.ReturnID]
			if !ok {
				c = model.SalesFinCredit{ReceivableID: id, ReturnID: h.ReturnID, OrderID: p.OrderID, PackageID: p.PackageID, ReturnNo: h.ReturnNo}
			}
			c.Registered = ok
			out = append(out, c)
		}
		return nil
	})
	return out, n, e
}
func AdjustSalesFinReceivable(ctx context.Context, id int64, in model.SalesFinAdjust, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireSalesFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var hint model.SalesFinReceivable
		if e := salesFinanceReceivableScope(tx.Model(&hint)).Where("receivable_id=?", id).Take(&hint).Error; e != nil {
			return erpReferenceError(e)
		}
		if _, _, e := lockSalesFinOrder(tx, hint.OrderID); e != nil {
			return e
		}
		var p model.SalesFinReceivable
		if e := salesFinanceReceivableScope(tx.Model(&p)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("receivable_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		r, e := salesFinanceReplay(tx, "RECEIVABLE_ADJUST", id, in.RequestKey, hash, "receivable")
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		if p.Version != in.Version {
			return errs.New("应收版本已变更，请刷新核对")
		}
		var h model.ERPSalesReturn
		if e = pendingSalesFinReturns(tx, []int64{p.OrderID}).Where("return_id=? AND package_id=?", in.ReturnID, p.PackageID).Take(&h).Error; e != nil {
			return erpReferenceError(e)
		}
		p.CreditAmount, e = decimalx.Add(p.CreditAmount, in.Amount)
		if e != nil {
			return e
		}
		if e = salesFinanceReceivableTotals(&p); e != nil {
			return e
		}
		c := model.SalesFinCredit{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Reason}, ReceivableID: id, OrderID: p.OrderID, PackageID: p.PackageID, ReturnID: h.ReturnID, ReturnNo: h.ReturnNo, Amount: in.Amount}
		if e = tx.Create(&c).Error; e != nil {
			return e
		}
		if e = salesFinSaveReceivable(tx, &p, operator); e != nil {
			return e
		}
		out = &model.FinResult{ID: id, Version: p.Version, Kind: "receivable", Status: "ADJUSTED"}
		return salesFinanceRecord(tx, "RECEIVABLE_ADJUST", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
