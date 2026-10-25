package repository

import (
	"context"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

func salesSalesFinReceivableQuery(tx *gorm.DB, q model.SalesFinQuery) *gorm.DB {
	out := salesFinanceReceivableScope(tx.Model(&model.SalesFinReceivable{}))
	if q.ShopID > 0 {
		out = out.Where("shop_id=?", q.ShopID)
	}
	if q.WarehouseID > 0 {
		out = out.Where("warehouse_id=?", q.WarehouseID)
	}
	if q.OrderID > 0 {
		out = out.Where("order_id=?", q.OrderID)
	}
	if q.Currency != "" {
		out = out.Where("currency=?", q.Currency)
	}
	if q.Code != "" {
		out = out.Where("receivable_no=?", q.Code)
	}
	return out
}
func SelectSalesFinReceivables(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinReceivable, int64, error) {
	rows := []model.SalesFinReceivable{}
	var n int64
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		if e := salesSalesFinReceivableQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		if e := salesSalesFinReceivableQuery(tx, q).Order("receivable_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error; e != nil {
			return e
		}
		return fillSalesFinProgress(tx, rows)
	})
	return rows, n, e
}
func SelectSalesFinReceivable(ctx context.Context, id int64) (*model.SalesFinReceivable, error) {
	var out model.SalesFinReceivable
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		rows := []model.SalesFinReceivable{{}}
		if e := salesFinanceReceivableScope(tx.Model(&model.SalesFinReceivable{})).Where("receivable_id=?", id).Take(&rows[0]).Error; e != nil {
			return erpReferenceError(e)
		}
		if e := fillSalesFinProgress(tx, rows); e != nil {
			return e
		}
		out = rows[0]
		var e error
		out.Lines, e = receivableSalesFinLines(tx, id)
		return e
	})
	return &out, e
}

// Export has no pending-return column. Do not expand up to100001 receipt IDs
// into a second IN query (MySQL prepared statement parameter limits).
func ExportSalesFinReceivables(ctx context.Context, q model.SalesFinQuery, limit int) ([]model.SalesFinReceivable, error) {
	rows := []model.SalesFinReceivable{}
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		if e := salesSalesFinReceivableQuery(tx, q).Order("receivable_id DESC").Limit(limit).Find(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			if e := salesFinanceReceivableTotals(&rows[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return rows, e
}
func salesSalesFinCollectionQuery(tx *gorm.DB, q model.SalesFinQuery) *gorm.DB {
	out := salesFinanceCollectionScope(tx.Model(&model.SalesFinCollection{}))
	if q.ShopID > 0 {
		out = out.Where("shop_id=?", q.ShopID)
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
		out = out.Where("collection_no=?", q.Code)
	}
	return out
}
func SelectSalesFinCollections(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinCollection, int64, error) {
	rows := []model.SalesFinCollection{}
	var n int64
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		if e := salesSalesFinCollectionQuery(tx, q).Count(&n).Error; e != nil {
			return e
		}
		return salesSalesFinCollectionQuery(tx, q).Select("collection_id,collection_no,account_id,account_name,shop_id,shop_name,currency,amount,status,version,create_time").Order("collection_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	})
	return rows, n, e
}
func SelectSalesFinCollection(ctx context.Context, id int64) (*model.SalesFinCollection, error) {
	var out model.SalesFinCollection
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		if e := salesFinanceCollectionScope(tx.Model(&out)).Where("collection_id=?", id).Take(&out).Error; e != nil {
			return erpReferenceError(e)
		}
		out.Allocations = []model.SalesFinAllocation{}
		if e := tx.Where("collection_id=?", id).Order("receivable_id").Limit(101).Find(&out.Allocations).Error; e != nil {
			return e
		}
		if len(out.Allocations) < 1 || len(out.Allocations) > 100 {
			return errs.New("收款核销行记录异常")
		}
		return nil
	})
	return &out, e
}
func SelectSalesFinStatements(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinStatement, int64, error) {
	rows := []model.SalesFinStatement{}
	var n int64
	e := salesFinanceRead(ctx, func(tx *gorm.DB) error {
		if e := salesSalesFinReceivableQuery(tx, q).Group("shop_id,currency").Count(&n).Error; e != nil {
			return e
		}
		selectSQL := "shop_id,MAX(shop_name) AS shop_name,currency,SUM(gross_amount) AS gross_amount,SUM(credit_amount) AS credit_amount,SUM(received_amount) AS received_amount,SUM(GREATEST(gross_amount-credit_amount-received_amount,0)) AS due_amount,SUM(GREATEST(received_amount-gross_amount+credit_amount,0)) AS overreceived_amount"
		if e := salesSalesFinReceivableQuery(tx, q).Select(selectSQL).Group("shop_id,currency").Order("shop_id DESC,currency").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			r := &rows[i]
			for _, p := range []*string{&r.GrossAmount, &r.CreditAmount, &r.ReceivedAmount, &r.DueAmount, &r.OverreceivedAmount} {
				v, e := decimalx.Normalize(*p)
				if e != nil {
					return errs.New("范围内汇总超过金额上限，请缩小店铺或币种范围")
				}
				*p = v
			}
		}
		return nil
	})
	return rows, n, e
}
