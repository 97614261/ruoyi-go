package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"sort"
)

const purchaseSummaryColumns = "purchase_id,purchase_no,supplier_id,warehouse_id,supplier_name,warehouse_name,currency,status,version,create_time"

func SelectERPPurchaseReceivables(ctx context.Context, q model.ERPPurchaseQuery, pg page.Query) ([]model.ERPPurchaseSummary, int64, error) {
	tx := purchaseQuery(DB(ctx), q).Where("status IN ?", []string{"CONFIRMED", "PARTIAL_RECEIVED"})
	var n int64
	if e := tx.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ERPPurchaseSummary{}
	e := tx.Select(purchaseSummaryColumns).Order("purchase_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	if e == nil {
		e = purchaseProgress(DB(ctx), rows)
	}
	return rows, n, e
}
func SelectERPPurchaseReceivable(ctx context.Context, id int64) (*model.ERPPurchaseReceivable, error) {
	var out model.ERPPurchaseReceivable
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := purchaseScope(tx.Model(&model.ERPPurchase{})).Select(purchaseSummaryColumns).Where("purchase_id=? AND status IN ?", id, []string{"CONFIRMED", "PARTIAL_RECEIVED"}).Scan(&out.ERPPurchaseSummary).Error; e != nil {
			return e
		}
		if out.PurchaseID == 0 {
			return errs.New("采购单不存在、无仓库权限或当前不可收货")
		}
		out.Lines = []model.ERPPurchaseReceivableLine{}
		if e := tx.Model(&model.ERPPurchaseLine{}).Select("line_id,sku_id,sku_code,product_name,specification,unit,ordered_quantity,received_quantity").Where("purchase_id=?", id).Order("line_no").Limit(101).Scan(&out.Lines).Error; e != nil {
			return e
		}
		if len(out.Lines) < 1 || len(out.Lines) > 100 {
			return errs.New("采购明细记录异常")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &out, e
}
func SelectERPPurchaseReceipts(ctx context.Context, in model.ERPPurchaseReceiptQuery, pg page.Query) ([]model.ERPPurchaseReceipt, int64, error) {
	q := erpWarehouseScope(DB(ctx).Model(&model.ERPPurchaseReceipt{}), "erp_purchase_receipt.warehouse_id")
	if in.PurchaseID > 0 {
		q = q.Where("purchase_id=?", in.PurchaseID)
	}
	if in.WarehouseID > 0 {
		q = q.Where("warehouse_id=?", in.WarehouseID)
	}
	if in.ReceiptNo != "" {
		q = q.Where("receipt_no=?", in.ReceiptNo)
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ERPPurchaseReceipt{}
	e := q.Select("receipt_id,receipt_no,purchase_id,warehouse_id,stock_document_id,actor_user_id,posted_at").Order("receipt_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, n, e
}
func SelectERPPurchaseReceipt(ctx context.Context, id int64) (*model.ERPPurchaseReceipt, error) {
	var r model.ERPPurchaseReceipt
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := erpWarehouseScope(tx.Model(&r), "erp_purchase_receipt.warehouse_id").Where("receipt_id=?", id).Take(&r).Error; e != nil {
			return erpReferenceError(e)
		}
		r.Lines = []model.ERPPurchaseReceiptLine{}
		if e := tx.Where("receipt_id=? AND purchase_id=?", id, r.PurchaseID).Order("line_id").Limit(101).Find(&r.Lines).Error; e != nil {
			return e
		}
		if len(r.Lines) < 1 || len(r.Lines) > 100 {
			return errs.New("收货明细记录异常")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &r, e
}
func replayPurchaseReceipt(tx *gorm.DB, p model.ERPPurchase, key, hash string) (*model.ERPPurchaseReceiveResult, error) {
	var ev model.ERPPurchaseEvent
	e := tx.Where("purchase_id=? AND action='RECEIVE' AND request_key=?", p.PurchaseID, key).Take(&ev).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if ev.RequestHash != hash || ev.ActorUserID != erpctx.UserID(tx.Statement.Context) {
		return nil, errs.New("收货请求标识已用于不同内容或操作者")
	}
	var r model.ERPPurchaseReceiveResult
	if e = json.Unmarshal([]byte(ev.ResultJSON), &r); e != nil || ev.ReceiptID == nil || r.ReceiptID != *ev.ReceiptID || r.PurchaseID != p.PurchaseID || r.Version != ev.ToVersion || r.Status != ev.ToStatus || r.StockDocumentID < 1 {
		return nil, errs.New("收货原结果记录异常")
	}
	r.Existing = true
	return &r, nil
}
func ReceiveERPPurchase(ctx context.Context, id int64, in model.ERPPurchaseReceiveInput, hash, operator string) (*model.ERPPurchaseReceiveResult, error) {
	var out *model.ERPPurchaseReceiveResult
	e := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		p, _, e := lockPurchase(tx, id)
		if e != nil {
			return e
		}
		if out, e = replayPurchaseReceipt(tx, p, in.RequestKey, hash); e != nil || out != nil {
			return e
		}
		if p.Version != in.Version || p.Version == math.MaxInt64 || (p.Status != "CONFIRMED" && p.Status != "PARTIAL_RECEIVED") {
			return errs.New("采购版本已变化或当前状态不可收货")
		}
		var lines []model.ERPPurchaseLine
		if e = tx.Where("purchase_id=?", id).Order("sku_id").Limit(101).Find(&lines).Error; e != nil {
			return e
		}
		if len(lines) < 1 || len(lines) > 100 {
			return errs.New("采购明细记录异常")
		}
		byID := map[int64]int{}
		for i, l := range lines {
			if l.OrderedQuantity < 1 || l.OrderedQuantity > 1000000000 || l.ReceivedQuantity < 0 || l.ReceivedQuantity > l.OrderedQuantity {
				return errs.New("采购数量记录异常")
			}
			byID[l.LineID] = i
		}
		received := map[int64]int64{}
		for _, l := range in.Lines {
			i, ok := byID[l.PurchaseLineID]
			if !ok || l.Quantity < 1 || l.Quantity > lines[i].OrderedQuantity-lines[i].ReceivedQuantity || received[l.PurchaseLineID] != 0 {
				return errs.New("收货行不属于本单、重复或数量超过剩余量")
			}
			received[l.PurchaseLineID] = l.Quantity
		}
		if len(received) < 1 || len(received) > 100 {
			return errs.New("本次须有1—100行正数收货")
		}
		selected := make([]model.ERPPurchaseLine, 0, len(received))
		for _, l := range lines {
			if received[l.LineID] > 0 {
				selected = append(selected, l)
			}
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].SkuID < selected[j].SkuID })
		ids := make([]int64, len(selected))
		for i, l := range selected {
			ids[i] = l.SkuID
		}
		// Lock warehouse/SKUs BEFORE inserting new FK children, avoiding shared-FK
		// lock upgrades by different purchases. The stock core reuses these locks.
		_, skus, e := lockERPReferences(tx, p.WarehouseID, ids, true)
		if e != nil {
			return e
		}
		for _, l := range selected {
			if l.Unit != skus[l.SkuID].Unit {
				return errs.New("采购单位与商品记录不一致")
			}
		}
		doc := model.ERPDocument{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, DocumentNo: "PI" + uuid.NewString(), Kind: "IN", WarehouseID: p.WarehouseID, WarehouseName: p.WarehouseName, Status: "DRAFT", Version: 1, RequestKey: "P4:" + uuid.NewString(), RequestHash: hash}
		if e = tx.Omit("Lines").Create(&doc).Error; e != nil {
			return e
		}
		stockLines := make([]model.ERPDocumentLine, 0, len(selected))
		for _, l := range selected {
			stockLines = append(stockLines, model.ERPDocumentLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, DocumentID: doc.DocumentID, SkuID: l.SkuID, SkuCode: l.SkuCode, ProductName: l.ProductName, Specification: l.Specification, Unit: l.Unit, Quantity: received[l.LineID]})
		}
		if e = tx.Create(&stockLines).Error; e != nil {
			return e
		}
		r := model.ERPPurchaseReceipt{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ReceiptNo: "R" + uuid.NewString(), PurchaseID: id, WarehouseID: p.WarehouseID, StockDocumentID: doc.DocumentID, ActorUserID: erpctx.UserID(ctx), RequestKey: in.RequestKey, RequestHash: hash, PostedAt: types.Now()}
		if e = tx.Omit("Lines").Create(&r).Error; e != nil {
			return e
		}
		receiptLines := make([]model.ERPPurchaseReceiptLine, 0, len(selected))
		for _, l := range selected {
			receiptLines = append(receiptLines, model.ERPPurchaseReceiptLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ReceiptID: r.ReceiptID, PurchaseID: id, PurchaseLineID: l.LineID, SkuID: l.SkuID, Quantity: received[l.LineID], SkuCode: l.SkuCode, ProductName: l.ProductName, Specification: l.Specification, Unit: l.Unit})
		}
		if e = tx.Create(&receiptLines).Error; e != nil {
			return e
		}
		if e = transitionERPDocumentLocked(tx, &doc, "confirm", operator); e != nil {
			return e
		}
		if doc.Status != "POSTED" {
			return errs.New("采购库存过账未完成")
		}
		complete := true
		for _, l := range lines {
			qty := received[l.LineID]
			if qty > 0 {
				res := tx.Model(&model.ERPPurchaseLine{}).Where("purchase_id=? AND line_id=? AND received_quantity=?", id, l.LineID, l.ReceivedQuantity).Updates(map[string]any{"received_quantity": l.ReceivedQuantity + qty, "update_by": operator, "update_time": types.Now()})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected != 1 {
					return errs.New("采购已收数量发生变化")
				}
			}
			complete = complete && (l.ReceivedQuantity+qty == l.OrderedQuantity)
		}
		next := "PARTIAL_RECEIVED"
		if complete {
			next = "COMPLETED"
		}
		res := tx.Model(&model.ERPPurchase{}).Where("purchase_id=? AND version=?", id, p.Version).Updates(map[string]any{"status": next, "version": p.Version + 1, "update_by": operator, "update_time": types.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New("采购进度发生变化")
		}
		out = &model.ERPPurchaseReceiveResult{PurchaseID: id, ReceiptID: r.ReceiptID, StockDocumentID: doc.DocumentID, Status: next, Version: p.Version + 1}
		raw, _ := json.Marshal(out)
		event := model.ERPPurchaseEvent{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, PurchaseID: id, ReceiptID: &r.ReceiptID, ActorUserID: erpctx.UserID(ctx), Action: "RECEIVE", RequestKey: in.RequestKey, RequestHash: hash, FromStatus: p.Status, ToStatus: next, FromVersion: p.Version, ToVersion: p.Version + 1, ResultJSON: string(raw)}
		if e = tx.Create(&event).Error; e != nil {
			return e
		}
		audit, _ := json.Marshal(map[string]any{"purchaseId": id, "receiptId": r.ReceiptID, "stockDocumentId": doc.DocumentID, "fromVersion": p.Version, "version": p.Version + 1})
		return tx.Create(&model.SysOperLog{Title: "ERP采购收货", BusinessType: model.BusinessTypeInsert, Method: "erp.purchase.RECEIVE", RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
	}))
	return out, e
}

// Only fixed internal metadata probes may use System; never carry it forward
// into the customer business query or its nested tenant predicates.
func purchasingTableExists(tx *gorm.DB, table string) (bool, error) {
	if table != "erp_purchase_receipt" && table != "erp_purchase_order_line" {
		return false, errs.New("非法采购元数据探测")
	}
	var n int64
	e := tx.WithContext(tenantctx.System(tx.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name=?", table).Count(&n).Error
	return n > 0, e
}
func requireERPPurchaseVoucherIndependent(tx *gorm.DB, id int64) error {
	installed, e := purchasingTableExists(tx, "erp_purchase_receipt")
	if e != nil || !installed {
		return e
	}
	var n int64
	e = tx.Model(&model.ERPPurchaseReceipt{}).Where(`stock_document_id=? OR stock_document_id=(SELECT pd.source_document_id FROM erp_document pd WHERE pd.tenant_id=? AND pd.document_id=?)`, id, tenantctx.ID(tx.Statement.Context), id).Count(&n).Error
	if e != nil {
		return e
	}
	if n != 0 {
		return errs.New("该库存凭据由采购收货托管，不支持直接修改、撤销或采购退货")
	}
	return nil
}
