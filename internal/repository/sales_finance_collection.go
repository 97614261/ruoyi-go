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

func lockSalesFinReceivables(tx *gorm.DB, ids []int64, shop int64, currency string) ([]model.SalesFinReceivable, error) {
	if e := lockSalesFinOrigins(tx, ids); e != nil {
		return nil, e
	}
	rows := []model.SalesFinReceivable{}
	e := salesFinanceReceivableScope(tx.Model(&model.SalesFinReceivable{})).Clauses(clause.Locking{Strength: "UPDATE"}).Where("receivable_id IN ? AND shop_id=? AND currency=?", ids, shop, currency).Order("receivable_id").Find(&rows).Error
	if e != nil {
		return nil, e
	}
	if len(rows) != len(ids) {
		return nil, errs.New("应收不存在、店铺/币种不一致或缺少店铺/仓库范围")
	}
	return rows, nil
}
func CreateSalesFinCollection(ctx context.Context, in model.SalesFinCollectionWrite, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireSalesFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		shop, e := lockSalesFinShop(tx, in.ShopID)
		if e != nil {
			return e
		}
		var account model.FinAccount
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=?", in.AccountID).Take(&account).Error; e != nil {
			return erpReferenceError(e)
		}
		ids := make([]int64, len(in.Allocations))
		byID := map[int64]model.SalesFinAllocationInput{}
		for i, l := range in.Allocations {
			ids[i] = l.ReceivableID
			byID[l.ReceivableID] = l
		}
		receivables, e := lockSalesFinReceivables(tx, ids, in.ShopID, in.Currency)
		if e != nil {
			return e
		}
		replay, e := salesFinanceReplay(tx, "COLLECTION_CREATE", 0, in.RequestKey, hash, "collection")
		if e != nil {
			return e
		}
		if replay != nil {
			var p model.SalesFinCollection
			if e = salesFinanceCollectionScope(tx.Model(&p)).Where("collection_id=?", replay.ID).Take(&p).Error; e != nil {
				return erpReferenceError(e)
			}
			out = replay
			return nil
		}
		if account.Status != "0" || account.Currency != in.Currency {
			return errs.New("收款账户已停用或币种不一致")
		}
		receipts := make([]int64, len(receivables))
		for i, p := range receivables {
			receipts[i] = p.OrderID
		}
		var pending int64
		if e = pendingSalesFinReturns(tx, receipts).Count(&pending).Error; e != nil {
			return e
		}
		if pending != 0 {
			return errs.New("存在未登记的销售退货，请先核对并登记应收抵减")
		}
		for i := range receivables {
			p := &receivables[i]
			l := byID[p.ReceivableID]
			if p.Version != l.Version {
				return errs.New("应收版本已变更，请刷新后核对收款")
			}
			if e = salesFinanceReceivableTotals(p); e != nil {
				return e
			}
			cmp, e := decimalx.Compare(l.Amount, p.DueAmount)
			if e != nil {
				return e
			}
			if cmp > 0 {
				return errs.New("核销金额超过该应收当前待收金额")
			}
			p.ReceivedAmount, e = decimalx.Add(p.ReceivedAmount, l.Amount)
			if e != nil {
				return e
			}
			if e = salesFinSaveReceivable(tx, p, operator); e != nil {
				return e
			}
		}
		head := model.SalesFinCollection{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, CollectionNo: "COL-" + uuid.NewString(), AccountID: account.AccountID, AccountName: account.AccountName, ShopID: shop.ShopId, ShopName: shop.ShopName, Currency: in.Currency, Amount: in.Amount, Status: "POSTED", Version: 1}
		if e = tx.Create(&head).Error; e != nil {
			return e
		}
		lines := make([]model.SalesFinAllocation, len(in.Allocations))
		for i, l := range in.Allocations {
			lines[i] = model.SalesFinAllocation{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, CollectionID: head.CollectionID, ReceivableID: l.ReceivableID, Amount: l.Amount}
		}
		if e = tx.Create(&lines).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: head.CollectionID, Version: 1, Kind: "collection", Status: "POSTED"}
		return salesFinanceRecord(tx, "COLLECTION_CREATE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
func ReverseSalesFinCollection(ctx context.Context, id int64, in model.FinReverse, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireSalesFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var loc model.SalesFinCollection
		if e := salesFinanceCollectionScope(tx.Model(&loc)).Where("collection_id=?", id).Take(&loc).Error; e != nil {
			return erpReferenceError(e)
		}
		if _, e := lockSalesFinShop(tx, loc.ShopID); e != nil {
			return e
		}
		var account model.FinAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=?", loc.AccountID).Take(&account).Error; e != nil {
			return erpReferenceError(e)
		}
		var p model.SalesFinCollection
		if e := salesFinanceCollectionScope(tx.Model(&p)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("collection_id=? AND account_id=? AND shop_id=?", id, loc.AccountID, loc.ShopID).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		var lines []model.SalesFinAllocation
		if e := tx.Where("collection_id=?", id).Order("receivable_id").Limit(101).Find(&lines).Error; e != nil {
			return e
		}
		if len(lines) < 1 || len(lines) > 100 {
			return errs.New("收款核销记录异常")
		}
		ids := make([]int64, len(lines))
		byID := map[int64]string{}
		total := "0.0000"
		for i, l := range lines {
			ids[i] = l.ReceivableID
			byID[l.ReceivableID] = l.Amount
			var e error
			total, e = decimalx.Add(total, l.Amount)
			if e != nil {
				return e
			}
		}
		if cmp, e := decimalx.Compare(total, p.Amount); e != nil || cmp != 0 {
			return errs.New("收款与核销合计不一致")
		}
		receivables, e := lockSalesFinReceivables(tx, ids, p.ShopID, p.Currency)
		if e != nil {
			return e
		}
		replay, e := salesFinanceReplay(tx, "COLLECTION_REVERSE", id, in.RequestKey, hash, "collection")
		if e != nil {
			return e
		}
		if replay != nil {
			out = replay
			return nil
		}
		if p.Status != "POSTED" || p.Version != in.Version {
			return errs.New("收款状态或版本已变更，请刷新核对")
		}
		if e = salesFinanceVersion(p.Version); e != nil {
			return e
		}
		for i := range receivables {
			h := &receivables[i]
			h.ReceivedAmount, e = decimalx.Subtract(h.ReceivedAmount, byID[h.ReceivableID])
			if e != nil {
				return errs.New("应收已收金额记录异常，不能自动冲销")
			}
			if e = salesFinSaveReceivable(tx, h, operator); e != nil {
				return e
			}
		}
		p.Version++
		p.Status = "REVERSED"
		if e = tx.Model(&p).Updates(map[string]any{"status": p.Status, "version": p.Version, "reverse_reason": in.Reason, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: id, Version: p.Version, Kind: "collection", Status: p.Status}
		return salesFinanceRecord(tx, "COLLECTION_REVERSE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
