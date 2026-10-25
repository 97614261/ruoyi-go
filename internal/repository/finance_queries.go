package repository

import (
	"context"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

func finAccountQuery(tx *gorm.DB, q model.FinQuery) *gorm.DB {
	out := tx.Model(&model.FinAccount{})
	if q.AccountID > 0 {
		out = out.Where("account_id=?", q.AccountID)
	}
	if q.Code != "" {
		out = out.Where("account_code=?", q.Code)
	}
	if q.Currency != "" {
		out = out.Where("currency=?", q.Currency)
	}
	if q.Status != "" {
		out = out.Where("status=?", q.Status)
	}
	return out
}
func SelectFinAccounts(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinAccount, int64, error) {
	rows := []model.FinAccount{}
	var n int64
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := finAccountQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		return finAccountQuery(tx, q).Select("account_id,account_code,account_name,account_type,currency,status,version,create_time").Order("account_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	})
	return rows, n, e
}
func SelectFinAccount(ctx context.Context, id int64) (*model.FinAccount, error) {
	var out model.FinAccount
	e := financeRead(ctx, func(tx *gorm.DB) error { return erpReferenceError(tx.Where("account_id=?", id).Take(&out).Error) })
	return &out, e
}
func finPayableQuery(tx *gorm.DB, q model.FinQuery) *gorm.DB {
	out := financePayableScope(tx.Model(&model.FinPayable{}))
	if q.SupplierID > 0 {
		out = out.Where("supplier_id=?", q.SupplierID)
	}
	if q.WarehouseID > 0 {
		out = out.Where("warehouse_id=?", q.WarehouseID)
	}
	if q.ReceiptID > 0 {
		out = out.Where("receipt_id=?", q.ReceiptID)
	}
	if q.Currency != "" {
		out = out.Where("currency=?", q.Currency)
	}
	if q.Code != "" {
		out = out.Where("payable_no=?", q.Code)
	}
	return out
}
func SelectFinPayables(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinPayable, int64, error) {
	rows := []model.FinPayable{}
	var n int64
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := finPayableQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		if e := finPayableQuery(tx, q).Order("payable_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error; e != nil {
			return e
		}
		return fillFinProgress(tx, rows)
	})
	return rows, n, e
}
func SelectFinPayable(ctx context.Context, id int64) (*model.FinPayable, error) {
	var out model.FinPayable
	e := financeRead(ctx, func(tx *gorm.DB) error {
		rows := []model.FinPayable{{}}
		if e := financePayableScope(tx.Model(&model.FinPayable{})).Where("payable_id=?", id).Take(&rows[0]).Error; e != nil {
			return erpReferenceError(e)
		}
		if e := fillFinProgress(tx, rows); e != nil {
			return e
		}
		out = rows[0]
		var e error
		out.Lines, e = payableFinLines(tx, id)
		return e
	})
	return &out, e
}

// Export has no pending-return column. Do not expand up to100001 receipt IDs
// into a second IN query (MySQL prepared statement parameter limits).
func ExportFinPayables(ctx context.Context, q model.FinQuery, limit int) ([]model.FinPayable, error) {
	rows := []model.FinPayable{}
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := finPayableQuery(tx, q).Order("payable_id DESC").Limit(limit).Find(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			if e := financePayableTotals(&rows[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return rows, e
}
func finPaymentQuery(tx *gorm.DB, q model.FinQuery) *gorm.DB {
	out := financePaymentScope(tx.Model(&model.FinPayment{}))
	if q.SupplierID > 0 {
		out = out.Where("supplier_id=?", q.SupplierID)
	}
	if q.AccountID > 0 {
		out = out.Where("account_id=?", q.AccountID)
	}
	if q.Currency != "" {
		out = out.Where("currency=?", q.Currency)
	}
	if q.Status != "" {
		out = out.Where("status=?", q.Status)
	}
	if q.Code != "" {
		out = out.Where("payment_no=?", q.Code)
	}
	return out
}
func SelectFinPayments(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinPayment, int64, error) {
	rows := []model.FinPayment{}
	var n int64
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := finPaymentQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		return finPaymentQuery(tx, q).Select("payment_id,payment_no,account_id,account_name,supplier_id,supplier_name,currency,amount,status,version,create_time").Order("payment_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	})
	return rows, n, e
}
func SelectFinPayment(ctx context.Context, id int64) (*model.FinPayment, error) {
	var out model.FinPayment
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := financePaymentScope(tx.Model(&out)).Where("payment_id=?", id).Take(&out).Error; e != nil {
			return erpReferenceError(e)
		}
		out.Allocations = []model.FinAllocation{}
		if e := tx.Where("payment_id=?", id).Order("payable_id").Limit(101).Find(&out.Allocations).Error; e != nil {
			return e
		}
		if len(out.Allocations) < 1 || len(out.Allocations) > 100 {
			return errs.New("付款核销行记录异常")
		}
		return nil
	})
	return &out, e
}
func SelectFinStatements(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinStatement, int64, error) {
	rows := []model.FinStatement{}
	var n int64
	e := financeRead(ctx, func(tx *gorm.DB) error {
		if e := finPayableQuery(tx, q).Group("supplier_id,currency").Count(&n).Error; e != nil {
			return e
		}
		selectSQL := "supplier_id,MAX(supplier_name) AS supplier_name,currency,SUM(gross_amount) AS gross_amount,SUM(credit_amount) AS credit_amount,SUM(paid_amount) AS paid_amount,SUM(GREATEST(gross_amount-credit_amount-paid_amount,0)) AS due_amount,SUM(GREATEST(paid_amount-gross_amount+credit_amount,0)) AS overpaid_amount"
		if e := finPayableQuery(tx, q).Select(selectSQL).Group("supplier_id,currency").Order("supplier_id DESC,currency").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			r := &rows[i]
			for _, p := range []*string{&r.GrossAmount, &r.CreditAmount, &r.PaidAmount, &r.DueAmount, &r.OverpaidAmount} {
				v, e := decimalx.Normalize(*p)
				if e != nil {
					return errs.New("范围内汇总超过金额上限，请缩小供应商或币种范围")
				}
				*p = v
			}
		}
		return nil
	})
	return rows, n, e
}
