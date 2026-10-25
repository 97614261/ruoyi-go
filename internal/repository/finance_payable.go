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

func financeRead(ctx context.Context, fn func(*gorm.DB) error) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		return fn(tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
func financeSourceQuery(tx *gorm.DB, q model.FinQuery) *gorm.DB {
	tid := tenantctx.ID(tx.Statement.Context)
	out := erpWarehouseScope(tx.Model(&model.ERPPurchaseReceipt{}), "erp_purchase_receipt.warehouse_id").Joins("JOIN erp_purchase_order p ON p.tenant_id=erp_purchase_receipt.tenant_id AND p.purchase_id=erp_purchase_receipt.purchase_id")
	out = out.Where("NOT EXISTS (SELECT 1 FROM erp_fin_payable fp WHERE fp.tenant_id=? AND fp.receipt_id=erp_purchase_receipt.receipt_id)", tid)
	if q.ReceiptID > 0 {
		out = out.Where("erp_purchase_receipt.receipt_id=?", q.ReceiptID)
	}
	if q.WarehouseID > 0 {
		out = out.Where("erp_purchase_receipt.warehouse_id=?", q.WarehouseID)
	}
	if q.SupplierID > 0 {
		out = out.Where("p.supplier_id=?", q.SupplierID)
	}
	if q.Currency != "" {
		out = out.Where("p.currency=?", q.Currency)
	}
	if q.Code != "" {
		out = out.Where("erp_purchase_receipt.receipt_no=?", q.Code)
	}
	return out
}
func SelectFinSources(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinSource, int64, error) {
	rows := []model.FinSource{}
	var n int64
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := financeSourceQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		return financeSourceQuery(tx, q).Select("erp_purchase_receipt.receipt_id,erp_purchase_receipt.receipt_no,erp_purchase_receipt.purchase_id,erp_purchase_receipt.warehouse_id,p.supplier_id,p.supplier_name,p.currency").Order("erp_purchase_receipt.receipt_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	})
	return rows, n, e
}
func financeSourceLines(tx *gorm.DB, r model.ERPPurchaseReceipt) ([]model.FinPayableLine, string, error) {
	var lines []model.ERPPurchaseReceiptLine
	if e := tx.Where("receipt_id=? AND purchase_id=?", r.ReceiptID, r.PurchaseID).Order("line_id").Limit(101).Find(&lines).Error; e != nil {
		return nil, "", e
	}
	if len(lines) < 1 || len(lines) > 100 {
		return nil, "", errs.New("原收货明细异常")
	}
	ids := make([]int64, len(lines))
	for i, l := range lines {
		ids[i] = l.PurchaseLineID
	}
	var prices []model.ERPPurchaseLine
	if e := tx.Where("purchase_id=? AND line_id IN ?", r.PurchaseID, ids).Find(&prices).Error; e != nil {
		return nil, "", e
	}
	byID := map[int64]model.ERPPurchaseLine{}
	for _, l := range prices {
		byID[l.LineID] = l
	}
	out := make([]model.FinPayableLine, 0, len(lines))
	total := "0.0000"
	for _, l := range lines {
		p, ok := byID[l.PurchaseLineID]
		if !ok || p.SkuID != l.SkuID || l.Quantity < 1 || l.Quantity > 1000000000 {
			return nil, "", errs.New("原收货与采购快照不一致")
		}
		price, e := decimalx.Normalize(p.UnitPrice)
		if e != nil {
			return nil, "", errs.New("原采购价格记录异常")
		}
		amount, e := decimalx.Multiply(price, l.Quantity)
		if e != nil {
			return nil, "", e
		}
		total, e = decimalx.Add(total, amount)
		if e != nil {
			return nil, "", e
		}
		out = append(out, model.FinPayableLine{ReceiptID: r.ReceiptID, ReceiptLineID: l.LineID, PurchaseLineID: l.PurchaseLineID, SkuCode: l.SkuCode, ProductName: l.ProductName, Unit: l.Unit, Quantity: l.Quantity, UnitPrice: price, LineAmount: amount})
	}
	return out, total, nil
}
func SelectFinSource(ctx context.Context, id int64) (*model.FinSource, error) {
	var out model.FinSource
	e := financeRead(ctx, func(tx *gorm.DB) error {
		var r model.ERPPurchaseReceipt
		if e := erpWarehouseScope(tx.Model(&r), "erp_purchase_receipt.warehouse_id").Where("receipt_id=?", id).Take(&r).Error; e != nil {
			return erpReferenceError(e)
		}
		var p model.ERPPurchase
		if e := tx.Where("purchase_id=?", r.PurchaseID).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		out = model.FinSource{ReceiptID: r.ReceiptID, ReceiptNo: r.ReceiptNo, PurchaseID: p.PurchaseID, SupplierID: p.SupplierID, SupplierName: p.SupplierName, WarehouseID: r.WarehouseID, Currency: p.Currency}
		var e error
		out.Lines, out.GrossAmount, e = financeSourceLines(tx, r)
		return e
	})
	return &out, e
}
func lockFinReceipt(tx *gorm.DB, id int64) (model.ERPPurchaseReceipt, model.ERPPurchase, error) {
	var loc model.ERPPurchaseReceipt
	if e := erpWarehouseScope(tx.Model(&loc), "erp_purchase_receipt.warehouse_id").Where("receipt_id=?", id).Take(&loc).Error; e != nil {
		return loc, model.ERPPurchase{}, erpReferenceError(e)
	}
	p, _, e := lockPurchase(tx, loc.PurchaseID)
	if e != nil {
		return loc, p, e
	}
	var r model.ERPPurchaseReceipt
	e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("receipt_id=? AND purchase_id=? AND warehouse_id=?", id, p.PurchaseID, p.WarehouseID).Take(&r).Error
	return r, p, erpReferenceError(e)
}
func CreateFinPayable(ctx context.Context, in model.FinPayableCreate, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		receipt, p, e := lockFinReceipt(tx, in.ReceiptID)
		if e != nil {
			return e
		}
		replay, e := financeReplay(tx, "PAYABLE_CREATE", 0, in.RequestKey, hash, "payable")
		if e != nil {
			return e
		}
		if replay != nil {
			var h model.FinPayable
			if e = financePayableScope(tx.Model(&h)).Where("payable_id=? AND receipt_id=?", replay.ID, in.ReceiptID).Take(&h).Error; e != nil {
				return erpReferenceError(e)
			}
			out = replay
			return nil
		}
		var n int64
		if e = tx.Model(&model.FinPayable{}).Where("receipt_id=?", receipt.ReceiptID).Count(&n).Error; e != nil {
			return e
		}
		if n != 0 {
			return errs.New("该收货批次已登记应付，请查看原应付单")
		}
		lines, total, e := financeSourceLines(tx, receipt)
		if e != nil {
			return e
		}
		h := model.FinPayable{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, PayableNo: "AP-" + uuid.NewString(), ReceiptID: receipt.ReceiptID, ReceiptNo: receipt.ReceiptNo, PurchaseID: p.PurchaseID, SupplierID: p.SupplierID, SupplierName: p.SupplierName, WarehouseID: receipt.WarehouseID, Currency: p.Currency, GrossAmount: total, CreditAmount: "0.0000", PaidAmount: "0.0000", Version: 1}
		if e = tx.Create(&h).Error; e != nil {
			return e
		}
		for i := range lines {
			lines[i].PayableID = h.PayableID
			lines[i].ERPRecord = model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}
		}
		if e = tx.Create(&lines).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: h.PayableID, Kind: "payable", Version: 1, Status: "REGISTERED"}
		return financeRecord(tx, "PAYABLE_CREATE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}

func pendingFinReturns(tx *gorm.DB, receipts []int64) *gorm.DB {
	return tx.Model(&model.ERPPurchaseReturn{}).Where("receipt_id IN ?", receipts).Where("NOT EXISTS (SELECT 1 FROM erp_fin_credit fc WHERE fc.tenant_id=? AND fc.return_id=erp_purchase_return_order.return_id)", tenantctx.ID(tx.Statement.Context))
}
func fillFinProgress(tx *gorm.DB, rows []model.FinPayable) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].ReceiptID
		if e := financePayableTotals(&rows[i]); e != nil {
			return e
		}
	}
	var counts []struct {
		ReceiptID int64
		N         int64
	}
	if e := pendingFinReturns(tx, ids).Select("receipt_id,COUNT(*) AS n").Group("receipt_id").Scan(&counts).Error; e != nil {
		return e
	}
	m := map[int64]int64{}
	for _, r := range counts {
		m[r.ReceiptID] = r.N
	}
	for i := range rows {
		rows[i].PendingReturnCount = m[rows[i].ReceiptID]
	}
	return nil
}
func payableFinLines(tx *gorm.DB, id int64) ([]model.FinPayableLine, error) {
	rows := []model.FinPayableLine{}
	e := tx.Where("payable_id=?", id).Order("line_id").Limit(101).Find(&rows).Error
	if e == nil && (len(rows) < 1 || len(rows) > 100) {
		e = errs.New("应付明细记录异常")
	}
	return rows, e
}

// All return rows in a bounded page are valued with one original-line read and
// one return-line read. No per-return/per-line lookup or current SKU pricing.
func finReturnAmounts(tx *gorm.DB, p model.FinPayable, heads []model.ERPPurchaseReturn) (map[int64]string, error) {
	amounts := map[int64]string{}
	if len(heads) == 0 {
		return amounts, nil
	}
	ids := make([]int64, len(heads))
	for i, h := range heads {
		if h.ReceiptID != p.ReceiptID || h.PurchaseID != p.PurchaseID {
			return nil, errs.New("退货来源不一致")
		}
		ids[i] = h.ReturnID
		amounts[h.ReturnID] = "0.0000"
	}
	original, e := payableFinLines(tx, p.PayableID)
	if e != nil {
		return nil, e
	}
	byLine := map[int64]model.FinPayableLine{}
	for _, l := range original {
		byLine[l.ReceiptLineID] = l
	}
	var returned []model.ERPPurchaseReturnLine
	if e = tx.Where("return_id IN ? AND receipt_id=? AND purchase_id=?", ids, p.ReceiptID, p.PurchaseID).Limit(10001).Find(&returned).Error; e != nil {
		return nil, e
	}
	if len(returned) > 10000 {
		return nil, errs.New("退货明细超限")
	}
	seen := map[int64]bool{}
	for _, l := range returned {
		basis, ok := byLine[l.ReceiptLineID]
		if !ok || basis.PurchaseLineID != l.PurchaseLineID || l.Quantity < 1 || l.Quantity > basis.Quantity {
			return nil, errs.New("退货与应付原行不一致")
		}
		amount, e := decimalx.Multiply(basis.UnitPrice, l.Quantity)
		if e != nil {
			return nil, e
		}
		amounts[l.ReturnID], e = decimalx.Add(amounts[l.ReturnID], amount)
		if e != nil {
			return nil, e
		}
		seen[l.ReturnID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			return nil, errs.New("退货明细缺失")
		}
	}
	return amounts, nil
}
func SelectFinReturns(ctx context.Context, id int64, pending bool, pg page.Query) ([]model.FinCredit, int64, error) {
	out := []model.FinCredit{}
	var n int64
	e := financeRead(ctx, func(tx *gorm.DB) error {
		var p model.FinPayable
		if e := financePayableScope(tx.Model(&p)).Where("payable_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		build := func() *gorm.DB {
			if pending {
				return pendingFinReturns(tx, []int64{p.ReceiptID})
			}
			return tx.Model(&model.ERPPurchaseReturn{}).Where("receipt_id=? AND purchase_id=?", p.ReceiptID, p.PurchaseID)
		}
		if e := build().Count(&n).Error; e != nil {
			return e
		}
		var heads []model.ERPPurchaseReturn
		if e := build().Order("return_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&heads).Error; e != nil {
			return e
		}
		amounts, e := finReturnAmounts(tx, p, heads)
		if e != nil {
			return e
		}
		ids := make([]int64, len(heads))
		for i, h := range heads {
			ids[i] = h.ReturnID
		}
		var credits []model.FinCredit
		if len(ids) > 0 {
			if e = tx.Where("payable_id=? AND return_id IN ?", id, ids).Find(&credits).Error; e != nil {
				return e
			}
		}
		registered := map[int64]model.FinCredit{}
		for _, r := range credits {
			registered[r.ReturnID] = r
		}
		for _, h := range heads {
			r, ok := registered[h.ReturnID]
			if !ok {
				r = model.FinCredit{PayableID: id, ReturnID: h.ReturnID, PurchaseID: p.PurchaseID, ReceiptID: p.ReceiptID, ReturnNo: h.ReturnNo, Amount: amounts[h.ReturnID]}
			}
			r.Registered = ok
			out = append(out, r)
		}
		return nil
	})
	return out, n, e
}
func AdjustFinPayable(ctx context.Context, id int64, in model.FinAdjust, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var loc model.FinPayable
		if e := financePayableScope(tx.Model(&loc)).Where("payable_id=?", id).Take(&loc).Error; e != nil {
			return erpReferenceError(e)
		}
		if _, _, e := lockFinReceipt(tx, loc.ReceiptID); e != nil {
			return e
		}
		var p model.FinPayable
		if e := financePayableScope(tx.Model(&p)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("payable_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		r, e := financeReplay(tx, "PAYABLE_ADJUST", id, in.RequestKey, hash, "payable")
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		if p.Version != in.Version {
			return errs.New("应付版本已变更，请刷新核对")
		}
		var heads []model.ERPPurchaseReturn
		if e = pendingFinReturns(tx, []int64{p.ReceiptID}).Where("return_id IN ? AND purchase_id=?", in.ReturnIDs, p.PurchaseID).Find(&heads).Error; e != nil {
			return e
		}
		if len(heads) != len(in.ReturnIDs) {
			return errs.New("退货不存在、不属此批次或已抵减，请刷新核对")
		}
		amounts, e := finReturnAmounts(tx, p, heads)
		if e != nil {
			return e
		}
		credits := make([]model.FinCredit, 0, len(heads))
		for _, h := range heads {
			amount := amounts[h.ReturnID]
			p.CreditAmount, e = decimalx.Add(p.CreditAmount, amount)
			if e != nil {
				return e
			}
			credits = append(credits, model.FinCredit{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, PayableID: id, ReturnID: h.ReturnID, PurchaseID: p.PurchaseID, ReceiptID: p.ReceiptID, ReturnNo: h.ReturnNo, Amount: amount})
		}
		if e = financePayableTotals(&p); e != nil {
			return e
		}
		if e = tx.Create(&credits).Error; e != nil {
			return e
		}
		if e = finSavePayable(tx, &p, operator); e != nil {
			return e
		}
		out = &model.FinResult{ID: id, Version: p.Version, Kind: "payable", Status: "ADJUSTED"}
		return financeRecord(tx, "PAYABLE_ADJUST", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
