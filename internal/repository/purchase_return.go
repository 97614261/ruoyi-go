package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// Not an HTTP input or context flag. Its persisted links must prove this exact
// newly created PR belongs to this receipt before allowing a managed IN source.
type purchaseReturnSource struct{ returnID, receiptID int64 }

func (p *purchaseReturnSource) verify(tx *gorm.DB, doc, source *model.ERPDocument) error {
	if p == nil || p.returnID < 1 || p.receiptID < 1 || doc.Kind != "PR" || doc.Status != "DRAFT" || doc.Reversal || doc.SourceDocumentID == nil || *doc.SourceDocumentID != source.DocumentID {
		return errs.New("采购退货来源证明不匹配")
	}
	var head model.ERPPurchaseReturn
	if e := tx.Where("return_id=? AND receipt_id=? AND stock_document_id=? AND warehouse_id=?", p.returnID, p.receiptID, doc.DocumentID, doc.WarehouseID).Take(&head).Error; e != nil {
		return erpReferenceError(e)
	}
	var n int64
	if e := tx.Model(&model.ERPPurchaseReceipt{}).Where("receipt_id=? AND purchase_id=? AND stock_document_id=? AND warehouse_id=?", head.ReceiptID, head.PurchaseID, source.DocumentID, doc.WarehouseID).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errs.New("采购退货与原收货关联异常")
	}
	return nil
}

func returnableReceiptQuery(tx *gorm.DB, in model.ERPPurchaseReceiptQuery) *gorm.DB {
	q := erpWarehouseScope(tx.Model(&model.ERPPurchaseReceipt{}), "erp_purchase_receipt.warehouse_id")
	if in.PurchaseID > 0 {
		q = q.Where("purchase_id=?", in.PurchaseID)
	}
	if in.WarehouseID > 0 {
		q = q.Where("warehouse_id=?", in.WarehouseID)
	}
	if in.ReceiptNo != "" {
		q = q.Where("receipt_no=?", in.ReceiptNo)
	}
	return q
}
func returnableSummary(r model.ERPPurchaseReceipt) model.ERPPurchaseReturnable {
	return model.ERPPurchaseReturnable{ReceiptID: r.ReceiptID, ReceiptNo: r.ReceiptNo, PurchaseID: r.PurchaseID, WarehouseID: r.WarehouseID, StockDocumentID: r.StockDocumentID, PostedAt: r.PostedAt, Version: 1}
}

// Three bounded-page aggregates, not a query for every receipt/line. Explicit
// snapshots in callers keep returned quantities and return versions consistent.
func purchaseReturnProgress(tx *gorm.DB, rows []model.ERPPurchaseReturnable) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ReceiptID
	}
	type total struct {
		ReceiptID int64
		Quantity  int64
	}
	var gross, returned []total
	if e := tx.Model(&model.ERPPurchaseReceiptLine{}).Select("receipt_id,SUM(quantity) AS quantity").Where("receipt_id IN ?", ids).Group("receipt_id").Scan(&gross).Error; e != nil {
		return e
	}
	if e := tx.Model(&model.ERPPurchaseReturnLine{}).Select("receipt_id,SUM(quantity) AS quantity").Where("receipt_id IN ?", ids).Group("receipt_id").Scan(&returned).Error; e != nil {
		return e
	}
	type revision struct {
		ReceiptID int64
		Revisions int64
	}
	var versions []revision
	if e := tx.Model(&model.ERPPurchaseReturn{}).Select("receipt_id,COUNT(*) AS revisions").Where("receipt_id IN ?", ids).Group("receipt_id").Scan(&versions).Error; e != nil {
		return e
	}
	g, u, v := map[int64]int64{}, map[int64]int64{}, map[int64]int64{}
	for _, r := range gross {
		g[r.ReceiptID] = r.Quantity
	}
	for _, r := range returned {
		u[r.ReceiptID] = r.Quantity
	}
	for _, r := range versions {
		v[r.ReceiptID] = r.Revisions
	}
	for i := range rows {
		r := &rows[i]
		if g[r.ReceiptID] < 1 || u[r.ReceiptID] < 0 || u[r.ReceiptID] > g[r.ReceiptID] || v[r.ReceiptID] == math.MaxInt64 {
			return errs.New("原收货或退货数量记录异常")
		}
		r.ReceivedQuantity = g[r.ReceiptID]
		r.ReturnedQuantity = u[r.ReceiptID]
		r.NetReceivedQuantity = r.ReceivedQuantity - r.ReturnedQuantity
		r.Version = v[r.ReceiptID] + 1
	}
	return nil
}
func SelectERPPurchaseReturnables(ctx context.Context, in model.ERPPurchaseReceiptQuery, pg page.Query) ([]model.ERPPurchaseReturnable, int64, error) {
	rows := []model.ERPPurchaseReturnable{}
	var n int64
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		q := returnableReceiptQuery(tx, in)
		if e := q.Count(&n).Error; e != nil {
			return e
		}
		var receipts []model.ERPPurchaseReceipt
		if e := q.Select("receipt_id,receipt_no,purchase_id,warehouse_id,stock_document_id,posted_at").Order("receipt_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&receipts).Error; e != nil {
			return e
		}
		for _, r := range receipts {
			rows = append(rows, returnableSummary(r))
		}
		return purchaseReturnProgress(tx, rows)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return rows, n, e
}

func purchaseReturnLines(tx *gorm.DB, receipt model.ERPPurchaseReceipt) ([]model.ERPPurchaseReturnableLine, error) {
	var lines []model.ERPPurchaseReceiptLine
	if e := tx.Where("receipt_id=? AND purchase_id=?", receipt.ReceiptID, receipt.PurchaseID).Order("sku_id").Limit(101).Find(&lines).Error; e != nil {
		return nil, e
	}
	if len(lines) < 1 || len(lines) > 100 {
		return nil, errs.New("收货明细记录异常")
	}
	var used []struct {
		ReceiptLineID int64
		Quantity      int64
	}
	if e := tx.Model(&model.ERPPurchaseReturnLine{}).Select("receipt_line_id,SUM(quantity) AS quantity").Where("receipt_id=?", receipt.ReceiptID).Group("receipt_line_id").Scan(&used).Error; e != nil {
		return nil, e
	}
	quantities := map[int64]int64{}
	for _, r := range used {
		quantities[r.ReceiptLineID] = r.Quantity
	}
	out := make([]model.ERPPurchaseReturnableLine, 0, len(lines))
	for _, l := range lines {
		u := quantities[l.LineID]
		if l.Quantity < 1 || l.Quantity > 1000000000 || u < 0 || u > l.Quantity {
			return nil, errs.New("收货或累计退货数量异常")
		}
		out = append(out, model.ERPPurchaseReturnableLine{ERPPurchaseReceiptLine: l, ReturnedQuantity: u, NetReceivedQuantity: l.Quantity - u})
		delete(quantities, l.LineID)
	}
	if len(quantities) != 0 {
		return nil, errs.New("退货行与原批次记录不一致")
	}
	return out, nil
}
func SelectERPPurchaseReturnable(ctx context.Context, id int64) (*model.ERPPurchaseReturnable, error) {
	var out model.ERPPurchaseReturnable
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var r model.ERPPurchaseReceipt
		if e := returnableReceiptQuery(tx, model.ERPPurchaseReceiptQuery{}).Where("receipt_id=?", id).Take(&r).Error; e != nil {
			return erpReferenceError(e)
		}
		rows := []model.ERPPurchaseReturnable{returnableSummary(r)}
		if e := purchaseReturnProgress(tx, rows); e != nil {
			return e
		}
		out = rows[0]
		var e error
		out.Lines, e = purchaseReturnLines(tx, r)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &out, e
}

func SelectERPPurchaseReturns(ctx context.Context, in model.ERPPurchaseReturnQuery, pg page.Query) ([]model.ERPPurchaseReturn, int64, error) {
	q := erpWarehouseScope(DB(ctx).Model(&model.ERPPurchaseReturn{}), "erp_purchase_return_order.warehouse_id")
	if in.ReceiptID > 0 {
		q = q.Where("receipt_id=?", in.ReceiptID)
	}
	if in.PurchaseID > 0 {
		q = q.Where("purchase_id=?", in.PurchaseID)
	}
	if in.WarehouseID > 0 {
		q = q.Where("warehouse_id=?", in.WarehouseID)
	}
	if in.ReturnNo != "" {
		q = q.Where("return_no=?", in.ReturnNo)
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ERPPurchaseReturn{}
	e := q.Select("return_id,return_no,purchase_id,receipt_id,warehouse_id,stock_document_id,actor_user_id,from_version,to_version,posted_at").Order("return_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, n, e
}
func SelectERPPurchaseReturn(ctx context.Context, id int64) (*model.ERPPurchaseReturn, error) {
	var out model.ERPPurchaseReturn
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := erpWarehouseScope(tx.Model(&out), "erp_purchase_return_order.warehouse_id").Where("return_id=?", id).Take(&out).Error; e != nil {
			return erpReferenceError(e)
		}
		out.Lines = []model.ERPPurchaseReturnLine{}
		if e := tx.Where("return_id=? AND receipt_id=? AND purchase_id=?", id, out.ReceiptID, out.PurchaseID).Order("sku_id").Limit(101).Find(&out.Lines).Error; e != nil {
			return e
		}
		if len(out.Lines) < 1 || len(out.Lines) > 100 {
			return errs.New("退货明细记录异常")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &out, e
}

func replayPurchaseReturn(tx *gorm.DB, receipt model.ERPPurchaseReceipt, key, hash string) (*model.ERPPurchaseReturnResult, error) {
	var head model.ERPPurchaseReturn
	e := tx.Where("receipt_id=? AND request_key=?", receipt.ReceiptID, key).Take(&head).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if head.RequestHash != hash || head.ActorUserID != erpctx.UserID(tx.Statement.Context) {
		return nil, errs.New("退货请求标识已用于不同内容或操作者")
	}
	var out model.ERPPurchaseReturnResult
	if e = json.Unmarshal([]byte(head.ResultJSON), &out); e != nil || out.ReturnID != head.ReturnID || out.ReceiptID != receipt.ReceiptID || out.PurchaseID != receipt.PurchaseID || out.StockDocumentID != head.StockDocumentID || out.Version != head.ToVersion {
		return nil, errs.New("退货原结果记录异常")
	}
	out.Existing = true
	return &out, nil
}

func ReturnERPPurchaseReceipt(ctx context.Context, id int64, in model.ERPPurchaseReturnInput, hash, operator string) (*model.ERPPurchaseReturnResult, error) {
	var out *model.ERPPurchaseReturnResult
	e := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var hint model.ERPPurchaseReceipt
		if e := returnableReceiptQuery(tx, model.ERPPurchaseReceiptQuery{}).Where("receipt_id=?", id).Take(&hint).Error; e != nil {
			return erpReferenceError(e)
		}
		p, _, e := lockPurchase(tx, hint.PurchaseID)
		if e != nil {
			return e
		}
		var r model.ERPPurchaseReceipt
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("receipt_id=? AND purchase_id=?", id, p.PurchaseID).Take(&r).Error; e != nil {
			return erpReferenceError(e)
		}
		if r.WarehouseID != p.WarehouseID {
			return errs.New("原收货仓库记录异常")
		}
		if e = requireERPWarehouse(tx, r.WarehouseID); e != nil {
			return e
		}
		if out, e = replayPurchaseReturn(tx, r, in.RequestKey, hash); e != nil || out != nil {
			return e
		}
		var revisions int64
		if e = tx.Model(&model.ERPPurchaseReturn{}).Where("receipt_id=?", id).Count(&revisions).Error; e != nil {
			return e
		}
		if revisions >= math.MaxInt64-1 || in.Version != revisions+1 {
			return errs.New("原批次退货版本已变化，请刷新后重新核对")
		}
		lines, e := purchaseReturnLines(tx, r)
		if e != nil {
			return e
		}
		byID := map[int64]model.ERPPurchaseReturnableLine{}
		for _, l := range lines {
			byID[l.LineID] = l
		}
		selected := make([]model.ERPPurchaseReturnableLine, 0, len(in.Lines))
		quantities := map[int64]int64{}
		if len(in.Lines) < 1 || len(in.Lines) > 100 {
			return errs.New("本次须有1—100行正数退货")
		}
		for _, input := range in.Lines {
			l, ok := byID[input.ReceiptLineID]
			if !ok || input.Quantity < 1 || input.Quantity > 1000000000 || input.Quantity > l.NetReceivedQuantity || quantities[input.ReceiptLineID] != 0 {
				return errs.New("退货行不属于本收货批次、重复或超过本批可退数量")
			}
			quantities[input.ReceiptLineID] = input.Quantity
			selected = append(selected, l)
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].SkuID < selected[j].SkuID })
		var source model.ERPDocument
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=?", r.StockDocumentID).Take(&source).Error; e != nil {
			return erpReferenceError(e)
		}
		if source.Kind != "IN" || source.Status != "POSTED" || source.WarehouseID != r.WarehouseID {
			return errs.New("原收货库存凭据不可退货")
		}
		ids := make([]int64, len(selected))
		for i, l := range selected {
			ids[i] = l.SkuID
		}
		if _, _, e = lockERPReferences(tx, r.WarehouseID, ids, false); e != nil {
			return e
		}
		record := model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}
		doc := model.ERPDocument{ERPRecord: record, DocumentNo: "PR5" + uuid.NewString(), Kind: "PR", WarehouseID: r.WarehouseID, WarehouseName: p.WarehouseName, SourceDocumentID: &r.StockDocumentID, Status: "DRAFT", Version: 1, RequestKey: "P5:" + uuid.NewString(), RequestHash: hash}
		if e = tx.Omit("Lines").Create(&doc).Error; e != nil {
			return e
		}
		stockLines := make([]model.ERPDocumentLine, 0, len(selected))
		for _, l := range selected {
			stockLines = append(stockLines, model.ERPDocumentLine{ERPRecord: record, DocumentID: doc.DocumentID, SkuID: l.SkuID, SkuCode: l.SkuCode, ProductName: l.ProductName, Specification: l.Specification, Unit: l.Unit, Quantity: quantities[l.LineID]})
		}
		if e = tx.Create(&stockLines).Error; e != nil {
			return e
		}
		head := model.ERPPurchaseReturn{ERPRecord: record, ReturnNo: "RT" + uuid.NewString(), PurchaseID: r.PurchaseID, ReceiptID: id, WarehouseID: r.WarehouseID, StockDocumentID: doc.DocumentID, ActorUserID: erpctx.UserID(ctx), RequestKey: in.RequestKey, RequestHash: hash, FromVersion: in.Version, ToVersion: in.Version + 1, Reason: in.Reason, PostedAt: types.Now()}
		if e = tx.Omit("Lines").Create(&head).Error; e != nil {
			return e
		}
		returnLines := make([]model.ERPPurchaseReturnLine, 0, len(selected))
		for _, l := range selected {
			returnLines = append(returnLines, model.ERPPurchaseReturnLine{ERPRecord: record, ReturnID: head.ReturnID, ReceiptID: id, ReceiptLineID: l.LineID, PurchaseID: r.PurchaseID, PurchaseLineID: l.PurchaseLineID, SkuID: l.SkuID, Quantity: quantities[l.LineID], SkuCode: l.SkuCode, ProductName: l.ProductName, Specification: l.Specification, Unit: l.Unit})
		}
		if e = tx.Create(&returnLines).Error; e != nil {
			return e
		}
		if e = transitionERPDocumentWithReturnLocked(tx, &doc, "confirm", operator, &purchaseReturnSource{returnID: head.ReturnID, receiptID: id}); e != nil {
			return e
		}
		if doc.Status != "POSTED" {
			return errs.New("采购退货库存过账未完成")
		}
		out = &model.ERPPurchaseReturnResult{ReturnID: head.ReturnID, ReceiptID: id, PurchaseID: r.PurchaseID, StockDocumentID: doc.DocumentID, Version: head.ToVersion}
		raw, e := json.Marshal(out)
		if e != nil {
			return e
		}
		result := tx.Model(&model.ERPPurchaseReturn{}).Where("return_id=? AND result_json=''", head.ReturnID).Update("result_json", string(raw))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errs.New("退货原结果未登记")
		}
		audit, _ := json.Marshal(map[string]any{"returnId": head.ReturnID, "receiptId": id, "purchaseId": r.PurchaseID, "stockDocumentId": doc.DocumentID, "fromVersion": head.FromVersion, "version": head.ToVersion})
		return tx.Create(&model.SysOperLog{Title: "ERP采购退货", BusinessType: model.BusinessTypeInsert, Method: "erp.purchase.RETURN", RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
	}))
	if e != nil {
		return nil, e
	}
	return out, nil
}
