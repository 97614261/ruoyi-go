package repository

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
)

func lockFinPayables(tx *gorm.DB, ids []int64, supplier int64, currency string) ([]model.FinPayable, error) {
	rows := []model.FinPayable{}
	e := financePayableScope(tx.Model(&model.FinPayable{})).Clauses(clause.Locking{Strength: "UPDATE"}).Where("payable_id IN ? AND supplier_id=? AND currency=?", ids, supplier, currency).Order("payable_id").Find(&rows).Error
	if e != nil {
		return nil, e
	}
	if len(rows) != len(ids) {
		return nil, errs.New("应付不存在、供应商/币种不一致或缺少仓库范围")
	}
	return rows, nil
}
func CreateFinPayment(ctx context.Context, in model.FinPaymentWrite, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		supplier, e := lockPurchaseSupplier(tx, in.SupplierID)
		if e != nil {
			return e
		}
		var account model.FinAccount
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=?", in.AccountID).Take(&account).Error; e != nil {
			return erpReferenceError(e)
		}
		ids := make([]int64, len(in.Allocations))
		byID := map[int64]model.FinAllocationInput{}
		for i, l := range in.Allocations {
			ids[i] = l.PayableID
			byID[l.PayableID] = l
		}
		payables, e := lockFinPayables(tx, ids, in.SupplierID, in.Currency)
		if e != nil {
			return e
		}
		replay, e := financeReplay(tx, "PAYMENT_CREATE", 0, in.RequestKey, hash, "payment")
		if e != nil {
			return e
		}
		if replay != nil {
			var p model.FinPayment
			if e = financePaymentScope(tx.Model(&p)).Where("payment_id=?", replay.ID).Take(&p).Error; e != nil {
				return erpReferenceError(e)
			}
			out = replay
			return nil
		}
		if account.Status != "0" || account.Currency != in.Currency {
			return errs.New("付款账户已停用或币种不一致")
		}
		receipts := make([]int64, len(payables))
		for i, p := range payables {
			receipts[i] = p.ReceiptID
		}
		var pending int64
		if e = pendingFinReturns(tx, receipts).Count(&pending).Error; e != nil {
			return e
		}
		if pending != 0 {
			return errs.New("存在未登记的采购退货，请先核对并登记应付抵减")
		}
		for i := range payables {
			p := &payables[i]
			l := byID[p.PayableID]
			if p.Version != l.Version {
				return errs.New("应付版本已变更，请刷新后核对付款")
			}
			if e = financePayableTotals(p); e != nil {
				return e
			}
			cmp, e := decimalx.Compare(l.Amount, p.DueAmount)
			if e != nil {
				return e
			}
			if cmp > 0 {
				return errs.New("核销金额超过该应付当前待付金额")
			}
			p.PaidAmount, e = decimalx.Add(p.PaidAmount, l.Amount)
			if e != nil {
				return e
			}
			if e = finSavePayable(tx, p, operator); e != nil {
				return e
			}
		}
		head := model.FinPayment{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, PaymentNo: "PAY-" + uuid.NewString(), AccountID: account.AccountID, AccountName: account.AccountName, SupplierID: supplier.SupplierId, SupplierName: supplier.SupplierName, Currency: in.Currency, Amount: in.Amount, Status: "POSTED", Version: 1}
		if e = tx.Create(&head).Error; e != nil {
			return e
		}
		lines := make([]model.FinAllocation, len(in.Allocations))
		for i, l := range in.Allocations {
			lines[i] = model.FinAllocation{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, PaymentID: head.PaymentID, PayableID: l.PayableID, Amount: l.Amount}
		}
		if e = tx.Create(&lines).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: head.PaymentID, Version: 1, Kind: "payment", Status: "POSTED"}
		return financeRecord(tx, "PAYMENT_CREATE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
func ReverseFinPayment(ctx context.Context, id int64, in model.FinReverse, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var loc model.FinPayment
		if e := financePaymentScope(tx.Model(&loc)).Where("payment_id=?", id).Take(&loc).Error; e != nil {
			return erpReferenceError(e)
		}
		if _, e := lockPurchaseSupplier(tx, loc.SupplierID); e != nil {
			return e
		}
		var account model.FinAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=?", loc.AccountID).Take(&account).Error; e != nil {
			return erpReferenceError(e)
		}
		var p model.FinPayment
		if e := financePaymentScope(tx.Model(&p)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("payment_id=? AND account_id=? AND supplier_id=?", id, loc.AccountID, loc.SupplierID).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		var lines []model.FinAllocation
		if e := tx.Where("payment_id=?", id).Order("payable_id").Limit(101).Find(&lines).Error; e != nil {
			return e
		}
		if len(lines) < 1 || len(lines) > 100 {
			return errs.New("付款核销记录异常")
		}
		ids := make([]int64, len(lines))
		byID := map[int64]string{}
		total := "0.0000"
		for i, l := range lines {
			ids[i] = l.PayableID
			byID[l.PayableID] = l.Amount
			var e error
			total, e = decimalx.Add(total, l.Amount)
			if e != nil {
				return e
			}
		}
		if cmp, e := decimalx.Compare(total, p.Amount); e != nil || cmp != 0 {
			return errs.New("付款与核销合计不一致")
		}
		payables, e := lockFinPayables(tx, ids, p.SupplierID, p.Currency)
		if e != nil {
			return e
		}
		replay, e := financeReplay(tx, "PAYMENT_REVERSE", id, in.RequestKey, hash, "payment")
		if e != nil {
			return e
		}
		if replay != nil {
			out = replay
			return nil
		}
		if p.Status != "POSTED" || p.Version != in.Version {
			return errs.New("付款状态或版本已变更，请刷新核对")
		}
		if e = financeVersion(p.Version); e != nil {
			return e
		}
		for i := range payables {
			h := &payables[i]
			h.PaidAmount, e = decimalx.Subtract(h.PaidAmount, byID[h.PayableID])
			if e != nil {
				return errs.New("应付已付金额记录异常，不能自动冲销")
			}
			if e = finSavePayable(tx, h, operator); e != nil {
				return e
			}
		}
		p.Version++
		p.Status = "REVERSED"
		if e = tx.Model(&p).Updates(map[string]any{"status": p.Status, "version": p.Version, "reverse_reason": in.Reason, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: id, Version: p.Version, Kind: "payment", Status: p.Status}
		return financeRecord(tx, "PAYMENT_REVERSE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
