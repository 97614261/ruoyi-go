package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func purchaseScope(q *gorm.DB) *gorm.DB {
	return erpWarehouseScope(q, "erp_purchase_order.warehouse_id")
}
func purchaseQuery(tx *gorm.DB, q model.ERPPurchaseQuery) *gorm.DB {
	tx = purchaseScope(tx.Model(&model.ERPPurchase{}))
	if q.PurchaseNo != "" {
		tx = tx.Where("purchase_no=?", q.PurchaseNo)
	}
	if q.SupplierID > 0 {
		tx = tx.Where("supplier_id=?", q.SupplierID)
	}
	if q.WarehouseID > 0 {
		tx = tx.Where("warehouse_id=?", q.WarehouseID)
	}
	if q.Status != "" {
		tx = tx.Where("status=?", q.Status)
	}
	if q.AwaitingReceipt {
		tx = purchaseAwaiting(tx)
	}
	return tx
}
func SelectERPPurchasePage(ctx context.Context, q model.ERPPurchaseQuery, pg page.Query) ([]model.ERPPurchaseSummary, int64, error) {
	tx := purchaseQuery(DB(ctx), q)
	var total int64
	if e := tx.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ERPPurchaseSummary{}
	e := tx.Select("purchase_id,purchase_no,supplier_id,warehouse_id,supplier_name,warehouse_name,currency,status,version,create_time").Order(pg.Stable("purchase_id DESC", "purchase_id")).Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	if e == nil {
		e = purchaseProgress(DB(ctx), rows)
	}
	return rows, total, e
}

// One bounded grouped read for a page, never a lookup per purchase.
func purchaseProgress(tx *gorm.DB, rows []model.ERPPurchaseSummary) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.PurchaseID
	}
	var sums []struct {
		PurchaseID       int64
		OrderedQuantity  int64
		ReceivedQuantity int64
	}
	if e := tx.Model(&model.ERPPurchaseLine{}).Select("purchase_id,SUM(ordered_quantity) AS ordered_quantity,SUM(received_quantity) AS received_quantity").Where("purchase_id IN ?", ids).Group("purchase_id").Scan(&sums).Error; e != nil {
		return e
	}
	byID := map[int64]int{}
	for i, r := range rows {
		byID[r.PurchaseID] = i
	}
	for _, s := range sums {
		if i, ok := byID[s.PurchaseID]; ok {
			rows[i].OrderedQuantity = s.OrderedQuantity
			rows[i].ReceivedQuantity = s.ReceivedQuantity
		}
	}
	return nil
}
func SelectERPPurchase(ctx context.Context, id int64) (*model.ERPPurchase, error) {
	var p model.ERPPurchase
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := purchaseScope(tx.Model(&p)).Where("purchase_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		p.Lines = []model.ERPPurchaseLine{}
		if e := tx.Where("purchase_id=?", id).Order("line_no").Limit(101).Find(&p.Lines).Error; e != nil {
			return e
		}
		if len(p.Lines) < 1 || len(p.Lines) > 100 {
			return errs.New("采购明细记录异常")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &p, e
}
func SelectERPPurchaseEvents(ctx context.Context, id int64, pg page.Query) ([]model.ERPPurchaseEvent, int64, error) {
	rows := []model.ERPPurchaseEvent{}
	var total int64
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ERPPurchase
		if e := purchaseScope(tx.Model(&p)).Select("purchase_id").Where("purchase_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		q := tx.Model(&model.ERPPurchaseEvent{}).Where("purchase_id=?", id)
		if e := q.Count(&total).Error; e != nil {
			return e
		}
		return q.Select("event_id,purchase_id,receipt_id,actor_user_id,action,from_status,to_status,from_version,to_version,create_time").Order("event_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return rows, total, e
}
func lockPurchaseSupplier(tx *gorm.DB, id int64) (model.ErpSupplier, error) {
	var s model.ErpSupplier
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("supplier_id=?", id).Take(&s).Error
	return s, erpReferenceError(e)
}

// Actor -> supplier -> purchase -> warehouse -> sorted SKU. The initial locator
// is scoped and nonlocking; immutable supplier/warehouse are rechecked on lock.
func lockPurchase(tx *gorm.DB, id int64) (model.ERPPurchase, model.ErpSupplier, error) {
	var loc model.ERPPurchase
	if e := purchaseScope(tx.Model(&loc)).Select("purchase_id,supplier_id,warehouse_id").Where("purchase_id=?", id).Take(&loc).Error; e != nil {
		return loc, model.ErpSupplier{}, erpReferenceError(e)
	}
	if e := requireERPWarehouse(tx, loc.WarehouseID); e != nil {
		return loc, model.ErpSupplier{}, e
	}
	s, e := lockPurchaseSupplier(tx, loc.SupplierID)
	if e != nil {
		return loc, s, e
	}
	var p model.ERPPurchase
	e = purchaseScope(tx.Model(&p)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("purchase_id=? AND supplier_id=? AND warehouse_id=?", id, loc.SupplierID, loc.WarehouseID).Take(&p).Error
	return p, s, erpReferenceError(e)
}
func replayPurchase(tx *gorm.DB, p model.ERPPurchase, action, key, hash string) (*model.ERPPurchaseResult, error) {
	var ev model.ERPPurchaseEvent
	e := tx.Where("purchase_id=? AND action=? AND request_key=?", p.PurchaseID, action, key).Take(&ev).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if ev.RequestHash != hash || ev.ActorUserID != erpctx.UserID(tx.Statement.Context) {
		return nil, errs.New("同一动作请求标识已用于不同内容或操作者")
	}
	var r model.ERPPurchaseResult
	if e = json.Unmarshal([]byte(ev.ResultJSON), &r); e != nil || r.PurchaseID != p.PurchaseID || r.Version != ev.ToVersion || r.Status != ev.ToStatus {
		return nil, errs.New("采购动作结果记录异常")
	}
	r.Existing = true
	return &r, nil
}
func recordPurchase(tx *gorm.DB, old, p model.ERPPurchase, action, key, hash, operator string) (*model.ERPPurchaseResult, error) {
	r := &model.ERPPurchaseResult{PurchaseID: p.PurchaseID, Version: p.Version, Status: p.Status}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, e
	}
	ev := model.ERPPurchaseEvent{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, PurchaseID: p.PurchaseID, ActorUserID: erpctx.UserID(tx.Statement.Context), Action: action, RequestKey: key, RequestHash: hash, FromStatus: old.Status, ToStatus: p.Status, FromVersion: old.Version, ToVersion: p.Version, ResultJSON: string(raw)}
	if e = tx.Create(&ev).Error; e != nil {
		return nil, e
	}
	audit, _ := json.Marshal(map[string]any{"purchaseId": p.PurchaseID, "action": action, "fromVersion": old.Version, "version": p.Version})
	business, method := model.BusinessTypeUpdate, "PUT"
	if action == "CREATE" {
		business, method = model.BusinessTypeInsert, "POST"
	}
	if e = tx.Create(&model.SysOperLog{Title: "ERP采购单", BusinessType: business, Method: "erp.purchase." + action, RequestMethod: method, OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error; e != nil {
		return nil, e
	}
	return r, nil
}
func purchaseDraftLines(tx *gorm.DB, p *model.ERPPurchase, s model.ErpSupplier, in model.ERPPurchaseWrite, operator string) error {
	if s.Status != "0" {
		return errs.New("供应商已停用")
	}
	ids := make([]int64, len(in.Lines))
	for i, l := range in.Lines {
		ids[i] = l.SkuID
	}
	wh, skus, e := lockERPReferences(tx, in.WarehouseID, ids, true)
	if e != nil {
		return e
	}
	p.SupplierCode = s.SupplierCode
	p.SupplierName = s.SupplierName
	p.WarehouseName = wh.WarehouseName
	p.OrderedAmount = in.OrderedAmount
	p.Lines = make([]model.ERPPurchaseLine, 0, len(in.Lines))
	for i, l := range in.Lines {
		sku := skus[l.SkuID]
		p.Lines = append(p.Lines, model.ERPPurchaseLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, PurchaseID: p.PurchaseID, LineNo: int64(i + 1), SkuID: l.SkuID, SkuCode: sku.SkuCode, ProductName: sku.ProductName, Specification: sku.Specification, Unit: sku.Unit, OrderedQuantity: l.Quantity, UnitPrice: l.UnitPrice, LineAmount: l.LineAmount})
	}
	return nil
}
func CreateERPPurchase(ctx context.Context, in model.ERPPurchaseWrite, hash, operator string) (*model.ERPPurchaseResult, error) {
	var out *model.ERPPurchaseResult
	e := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		if e := requireERPWarehouse(tx, in.WarehouseID); e != nil {
			return e
		}
		supplier, e := lockPurchaseSupplier(tx, in.SupplierID)
		if e != nil {
			return e
		}
		var old model.ERPPurchase
		e = tx.Where("create_request_key=?", in.RequestKey).Take(&old).Error
		if e == nil {
			if e = requireERPWarehouse(tx, old.WarehouseID); e != nil {
				return e
			}
			if old.CreateRequestHash != hash {
				return errs.New("创建请求标识已用于不同内容")
			}
			out, e = replayPurchase(tx, old, "CREATE", in.RequestKey, hash)
			if e == nil && out == nil {
				return errs.New("缺少原创建结果")
			}
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		p := model.ERPPurchase{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, PurchaseNo: "P" + uuid.NewString(), SupplierID: in.SupplierID, WarehouseID: in.WarehouseID, Currency: in.Currency, Status: "DRAFT", Version: 1, CreateRequestKey: in.RequestKey, CreateRequestHash: hash}
		if e = purchaseDraftLines(tx, &p, supplier, in, operator); e != nil {
			return e
		}
		if e = tx.Omit("Lines").Create(&p).Error; e != nil {
			return e
		}
		for i := range p.Lines {
			p.Lines[i].PurchaseID = p.PurchaseID
		}
		if e = tx.Create(&p.Lines).Error; e != nil {
			return e
		}
		out, e = recordPurchase(tx, model.ERPPurchase{}, p, "CREATE", in.RequestKey, hash, operator)
		return e
	}))
	return out, e
}
func EditERPPurchase(ctx context.Context, id int64, in model.ERPPurchaseEdit, hash, operator string) (*model.ERPPurchaseResult, error) {
	var out *model.ERPPurchaseResult
	e := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		p, s, e := lockPurchase(tx, id)
		if e != nil {
			return e
		}
		if out, e = replayPurchase(tx, p, "EDIT", in.RequestKey, hash); e != nil || out != nil {
			return e
		}
		if p.Status != "DRAFT" || p.Version != in.Version || p.Version == math.MaxInt64 {
			return errs.New("仅可编辑当前版本草稿")
		}
		if p.SupplierID != in.SupplierID || p.WarehouseID != in.WarehouseID {
			return errs.New("供应商和到货仓库不可修改，请取消后新建")
		}
		old := p
		if e = purchaseDraftLines(tx, &p, s, in.ERPPurchaseWrite, operator); e != nil {
			return e
		}
		p.Version++
		p.Currency = in.Currency
		u := map[string]any{"supplier_code": p.SupplierCode, "supplier_name": p.SupplierName, "warehouse_name": p.WarehouseName, "currency": p.Currency, "ordered_amount": p.OrderedAmount, "remark": in.Remark, "version": p.Version, "update_by": operator, "update_time": types.Now()}
		r := tx.Model(&model.ERPPurchase{}).Where("purchase_id=? AND version=?", id, old.Version).Updates(u)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errs.New("采购单已变化")
		}
		if e = tx.Where("purchase_id=?", id).Delete(&model.ERPPurchaseLine{}).Error; e != nil {
			return e
		}
		if e = tx.Create(&p.Lines).Error; e != nil {
			return e
		}
		out, e = recordPurchase(tx, old, p, "EDIT", in.RequestKey, hash, operator)
		return e
	}))
	return out, e
}
func confirmPurchaseLines(tx *gorm.DB, p *model.ERPPurchase, s model.ErpSupplier, operator string) error {
	if s.Status != "0" {
		return errs.New("供应商已停用")
	}
	if e := tx.Where("purchase_id=?", p.PurchaseID).Order("sku_id").Limit(101).Find(&p.Lines).Error; e != nil {
		return e
	}
	if len(p.Lines) < 1 || len(p.Lines) > 100 {
		return errs.New("采购明细记录异常")
	}
	ids := make([]int64, len(p.Lines))
	for i, l := range p.Lines {
		if l.OrderedQuantity < 1 || l.OrderedQuantity > 1000000000 || l.ReceivedQuantity != 0 {
			return errs.New("采购明细数量异常")
		}
		ids[i] = l.SkuID
	}
	wh, skus, e := lockERPReferences(tx, p.WarehouseID, ids, true)
	if e != nil {
		return e
	}
	total := "0.0000"
	// All lookups were batched. At most100 bounded row updates, not N+1 reads.
	for i := range p.Lines {
		l := &p.Lines[i]
		sku := skus[l.SkuID]
		amount, e := decimalx.Multiply(l.UnitPrice, l.OrderedQuantity)
		if e != nil {
			return errs.New("采购金额记录异常")
		}
		total, e = decimalx.Add(total, amount)
		if e != nil {
			return errs.New("采购金额记录异常")
		}
		if amount != l.LineAmount || l.Unit != sku.Unit {
			return errs.New("采购金额或计量单位记录异常")
		}
		if e = tx.Model(&model.ERPPurchaseLine{}).Where("line_id=? AND purchase_id=?", l.LineID, p.PurchaseID).Updates(map[string]any{"sku_code": sku.SkuCode, "product_name": sku.ProductName, "specification": sku.Specification, "unit": sku.Unit, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
			return e
		}
	}
	if total != p.OrderedAmount {
		return errs.New("采购总额记录异常")
	}
	p.SupplierCode = s.SupplierCode
	p.SupplierName = s.SupplierName
	p.WarehouseName = wh.WarehouseName
	return nil
}
func ActERPPurchase(ctx context.Context, id int64, action string, in model.ERPPurchaseAction, hash, operator string) (*model.ERPPurchaseResult, error) {
	var out *model.ERPPurchaseResult
	e := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		p, s, e := lockPurchase(tx, id)
		if e != nil {
			return e
		}
		if out, e = replayPurchase(tx, p, action, in.RequestKey, hash); e != nil || out != nil {
			return e
		}
		if p.Version != in.Version || p.Version == math.MaxInt64 {
			return errs.New("采购版本已变化或到达上限")
		}
		old := p
		switch action {
		case "CONFIRM":
			if p.Status != "DRAFT" {
				return errs.New("只有草稿可确认")
			}
			if e = confirmPurchaseLines(tx, &p, s, operator); e != nil {
				return e
			}
			p.Status = "CONFIRMED"
			p.ConfirmedAt = types.Now()
		case "CANCEL":
			if p.Status != "DRAFT" && p.Status != "CONFIRMED" {
				return errs.New("只有未收货采购单可取消")
			}
			var n int64
			if e = tx.Model(&model.ERPPurchaseLine{}).Where("purchase_id=? AND received_quantity<>0", id).Count(&n).Error; e != nil {
				return e
			}
			if n != 0 {
				return errs.New("采购单已有收货，不能取消")
			}
			p.Status = "CANCELLED"
			p.CancelledAt = types.Now()
			p.CloseReason = in.Reason
		case "CLOSE":
			if p.Status != "CONFIRMED" && p.Status != "PARTIAL_RECEIVED" {
				return errs.New("仅可关闭已确认且未收齐的采购单")
			}
			var remainder int64
			if e = tx.Model(&model.ERPPurchaseLine{}).Where("purchase_id=? AND received_quantity<ordered_quantity", id).Count(&remainder).Error; e != nil {
				return e
			}
			if remainder < 1 {
				return errs.New("采购单没有可关闭余量")
			}
			p.Status = "CLOSED"
			p.ClosedAt = types.Now()
			p.CloseReason = in.Reason
		default:
			return errs.New("采购动作不合法")
		}
		p.Version++
		r := tx.Model(&model.ERPPurchase{}).Where("purchase_id=? AND version=?", id, old.Version).Updates(map[string]any{"supplier_code": p.SupplierCode, "supplier_name": p.SupplierName, "warehouse_name": p.WarehouseName, "status": p.Status, "version": p.Version, "confirmed_at": p.ConfirmedAt, "closed_at": p.ClosedAt, "cancelled_at": p.CancelledAt, "close_reason": p.CloseReason, "update_by": operator, "update_time": types.Now()})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errs.New("采购单已变化")
		}
		out, e = recordPurchase(tx, old, p, action, in.RequestKey, hash, operator)
		return e
	}))
	return out, e
}

// Optional004 must not break the older core-only ERP. Metadata errors fail
// closed, unlike a HasTable boolean that would silently ignore a DB error.
func purchaseSKUReferenceCount(tx *gorm.DB, ids any) (int64, error) {
	// Only this fixed metadata query uses System; the following business count
	// keeps the original customer context and tenant guard.
	installed, e := purchasingTableExists(tx, "erp_purchase_order_line")
	if e != nil {
		return 0, e
	}
	if !installed {
		return 0, nil
	}
	var n int64
	e = tx.Model(&model.ERPPurchaseLine{}).Where("sku_id IN ?", ids).Count(&n).Error
	return n, e
}
