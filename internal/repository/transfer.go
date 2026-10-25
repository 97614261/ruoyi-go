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
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// Either endpoint grants visibility to this explicit transfer, not the other
// warehouse's balances/documents. Action authorization below is directional.
func transferVisible(q *gorm.DB) *gorm.DB {
	tid, uid := tenantctx.ID(q.Statement.Context), erpctx.UserID(q.Statement.Context)
	if tid < 1 || uid < 1 || tenantctx.Get(q.Statement.Context).Platform {
		q.AddError(errs.New("缺少企业ERP身份"))
		return q
	}
	return q.Where(`(EXISTS(SELECT 1 FROM sys_tenant wt WHERE wt.tenant_id=? AND wt.owner_user_id=?) OR EXISTS(SELECT 1 FROM erp_warehouse_access wa WHERE wa.tenant_id=? AND wa.user_id=? AND (wa.warehouse_id=erp_transfer.source_warehouse_id OR wa.warehouse_id=erp_transfer.target_warehouse_id)))`, tid, uid, tid, uid)
}
func SelectERPTransfers(ctx context.Context, in model.ERPTransferQuery, pg page.Query) ([]model.ERPTransfer, int64, error) {
	q := transferVisible(DB(ctx).Model(&model.ERPTransfer{}))
	if in.WarehouseID > 0 {
		q = q.Where("source_warehouse_id=? OR target_warehouse_id=?", in.WarehouseID, in.WarehouseID)
	}
	if in.TransferNo != "" {
		q = q.Where("transfer_no=?", in.TransferNo)
	}
	if in.Status != "" {
		q = q.Where("status=?", in.Status)
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ERPTransfer{}
	e := q.Order("transfer_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}
func GetERPTransfer(ctx context.Context, id int64) (*model.ERPTransfer, error) {
	var out model.ERPTransfer
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := transferVisible(tx.Model(&model.ERPTransfer{})).Where("transfer_id=?", id).Take(&out).Error; e != nil {
			return erpReferenceError(e)
		}
		return tx.Where("transfer_id=?", id).Order("sku_id").Limit(101).Find(&out.Lines).Error
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e == nil && (len(out.Lines) == 0 || len(out.Lines) > 100) {
		e = errs.New("调拨明细异常")
	}
	return &out, e
}
func requireTransferAction(tx *gorm.DB, h model.ERPTransfer, action string) error {
	switch action {
	case "RECEIVE":
		return requireERPWarehouse(tx, h.TargetWarehouseID)
	case "DISPATCH":
		return requireERPWarehouse(tx, h.SourceWarehouseID)
	default:
		if e := requireERPWarehouse(tx, h.SourceWarehouseID); e != nil {
			return e
		}
		return requireERPWarehouse(tx, h.TargetWarehouseID)
	}
}
func lockTransferReferences(tx *gorm.DB, h model.ERPTransfer, ids []int64, receive bool) (map[int64]model.ErpSku, error) {
	var ws []model.ErpWarehouse
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id IN ?", []int64{h.SourceWarehouseID, h.TargetWarehouseID}).Order("warehouse_id").Find(&ws).Error; e != nil {
		return nil, e
	}
	if len(ws) != 2 {
		return nil, errs.New("调拨仓库不存在或重复")
	}
	for _, w := range ws {
		if w.Status != "0" && (!receive || w.WarehouseId == h.TargetWarehouseID) {
			return nil, errs.New("调拨仓库已停用")
		}
	}
	var rows []model.ErpSku
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sku_id IN ?", ids).Order("sku_id").Find(&rows).Error; e != nil {
		return nil, e
	}
	if len(rows) != len(ids) {
		return nil, errs.New("调拨商品不存在或不属于当前企业")
	}
	out := map[int64]model.ErpSku{}
	for _, r := range rows {
		if r.Status != "0" {
			return nil, errs.New("调拨商品已停用")
		}
		out[r.SkuId] = r
	}
	return out, nil
}
func transferLines(h model.ERPTransfer, in []model.ERPLineInput, skus map[int64]model.ErpSku, operator string) []model.ERPTransferLine {
	out := make([]model.ERPTransferLine, 0, len(in))
	for _, l := range in {
		s := skus[l.SkuID]
		out = append(out, model.ERPTransferLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, TransferID: h.TransferID, SkuID: l.SkuID, Quantity: l.Quantity, SkuCode: s.SkuCode, ProductName: s.ProductName, Specification: s.Specification, Unit: s.Unit})
	}
	return out
}
func transferResult(raw string, id int64) (*model.ERPTransferResult, error) {
	var out model.ERPTransferResult
	if json.Unmarshal([]byte(raw), &out) != nil || out.TransferID != id || out.Version < 1 {
		return nil, errs.New("调拨原结果异常")
	}
	return &out, nil
}
func InsertERPTransfer(ctx context.Context, in model.ERPTransferInput, hash, operator string) (*model.ERPTransferResult, error) {
	var result *model.ERPTransferResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		if e := requireWarehouseOperations(tx); e != nil {
			return e
		}
		var old model.ERPTransfer
		e := tx.Where("request_key=?", in.RequestKey).Take(&old).Error
		if e == nil {
			if e = requireTransferAction(tx, old, "CREATE"); e != nil {
				return e
			}
			if old.RequestHash != hash || old.ActorUserID != erpctx.UserID(ctx) {
				return errs.New("创建标识已用于不同内容或操作者")
			}
			result, e = transferResult(old.ResultJSON, old.TransferID)
			return e
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		h := model.ERPTransfer{SourceWarehouseID: in.SourceWarehouseID, TargetWarehouseID: in.TargetWarehouseID, Status: "DRAFT", Version: 1, ActorUserID: erpctx.UserID(ctx), RequestKey: in.RequestKey, RequestHash: hash, TransferNo: "TR-" + uuid.NewString(), ResultJSON: "{}", ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}}
		if e = requireTransferAction(tx, h, "CREATE"); e != nil {
			return e
		}
		ids := make([]int64, len(in.Lines))
		for i, l := range in.Lines {
			ids[i] = l.SkuID
		}
		skus, e := lockTransferReferences(tx, h, ids, false)
		if e != nil {
			return e
		}
		var ws []model.ErpWarehouse
		if e = tx.Where("warehouse_id IN ?", []int64{h.SourceWarehouseID, h.TargetWarehouseID}).Find(&ws).Error; e != nil {
			return e
		}
		for _, w := range ws {
			if w.WarehouseId == h.SourceWarehouseID {
				h.SourceWarehouseName = w.WarehouseName
			} else {
				h.TargetWarehouseName = w.WarehouseName
			}
		}
		if e = tx.Omit("Lines").Create(&h).Error; e != nil {
			return e
		}
		lines := transferLines(h, in.Lines, skus, operator)
		if e = tx.Create(&lines).Error; e != nil {
			return e
		}
		result = &model.ERPTransferResult{TransferID: h.TransferID, Status: h.Status, Version: h.Version}
		b, _ := json.Marshal(result)
		if e = tx.Model(&model.ERPTransfer{}).Where("transfer_id=?", h.TransferID).Update("result_json", string(b)).Error; e != nil {
			return e
		}
		return warehouseAudit(tx, "transfer.CREATE", h.TransferID, h.Version, operator)
	})
	return result, erpError(e)
}

func ChangeERPTransfer(ctx context.Context, id int64, action string, in model.ERPTransferInput, hash, operator string) (*model.ERPTransferResult, error) {
	var result *model.ERPTransferResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var h model.ERPTransfer
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("transfer_id=?", id).Take(&h).Error; e != nil {
			return erpReferenceError(e)
		}
		if e := requireTransferAction(tx, h, action); e != nil {
			return e
		}
		var replay model.ERPTransferAction
		e := tx.Where("transfer_id=? AND request_key=?", id, in.RequestKey).Take(&replay).Error
		if e == nil {
			if replay.RequestHash != hash || replay.ActorUserID != erpctx.UserID(ctx) || replay.Action != action {
				return errs.New("动作标识已用于不同内容或操作者")
			}
			result, e = transferResult(replay.ResultJSON, id)
			return e
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if h.Version != in.Version || h.Version == math.MaxInt64 {
			return errs.New("调拨单已变更，请刷新后重试")
		}
		if e = tx.Where("transfer_id=?", id).Order("sku_id").Limit(101).Find(&h.Lines).Error; e != nil {
			return e
		}
		if len(h.Lines) < 1 || len(h.Lines) > 100 {
			return errs.New("调拨明细异常")
		}
		if action == "CANCEL" {
			if h.Status != "DRAFT" {
				return errs.New("仅草稿可取消，调出后需另行处理")
			}
			h.Status = "CANCELLED"
		} else {
			if (action == "EDIT" || action == "DISPATCH") && h.Status != "DRAFT" {
				return errs.New("仅草稿可编辑或调出")
			}
			if action == "RECEIVE" && h.Status != "IN_TRANSIT" {
				return errs.New("仅在途单可收货")
			}
			ids := make([]int64, 0, len(h.Lines))
			if action == "EDIT" {
				if in.SourceWarehouseID != h.SourceWarehouseID || in.TargetWarehouseID != h.TargetWarehouseID {
					return errs.New("草稿调拨仓库不可改，请取消重建")
				}
				for _, l := range in.Lines {
					ids = append(ids, l.SkuID)
				}
			} else {
				for _, l := range h.Lines {
					ids = append(ids, l.SkuID)
				}
			}
			skus, e := lockTransferReferences(tx, h, ids, action == "RECEIVE")
			if e != nil {
				return e
			}
			if action == "EDIT" {
				if e = tx.Where("transfer_id=?", id).Delete(&model.ERPTransferLine{}).Error; e != nil {
					return e
				}
				h.Lines = transferLines(h, in.Lines, skus, operator)
				if e = tx.Create(&h.Lines).Error; e != nil {
					return e
				}
				h.Remark = in.Remark
			} else {
				wh, kind := h.SourceWarehouseID, "TOUT"
				name := h.SourceWarehouseName
				if action == "RECEIVE" {
					wh, kind, name = h.TargetWarehouseID, "TIN", h.TargetWarehouseName
				}
				doc := model.ERPDocument{WarehouseID: wh, WarehouseName: name, Kind: kind, Status: "DRAFT", Version: 1, DocumentNo: kind + "-" + uuid.NewString(), RequestKey: "W7_" + uuid.NewString(), RequestHash: hash, ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}}
				if e = tx.Create(&doc).Error; e != nil {
					return e
				}
				ls := make([]model.ERPDocumentLine, 0, len(h.Lines))
				for _, l := range h.Lines {
					ls = append(ls, model.ERPDocumentLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, DocumentID: doc.DocumentID, SkuID: l.SkuID, Quantity: l.Quantity, SkuCode: l.SkuCode, ProductName: l.ProductName, Specification: l.Specification, Unit: l.Unit})
				}
				if e = tx.Create(&ls).Error; e != nil {
					return e
				}
				if e = transitionERPDocumentLocked(tx, &doc, "confirm", operator); e != nil {
					return e
				}
				if action == "DISPATCH" {
					h.Status = "IN_TRANSIT"
					h.OutDocumentID = &doc.DocumentID
				} else {
					h.Status = "COMPLETED"
					h.InDocumentID = &doc.DocumentID
				}
			}
		}
		h.Version++
		if e = tx.Model(&model.ERPTransfer{}).Where("transfer_id=?", id).Updates(map[string]any{"status": h.Status, "version": h.Version, "out_document_id": h.OutDocumentID, "in_document_id": h.InDocumentID, "remark": h.Remark, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		result = &model.ERPTransferResult{TransferID: id, Version: h.Version, Status: h.Status}
		b, _ := json.Marshal(result)
		ev := model.ERPTransferAction{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, TransferID: id, ActorUserID: erpctx.UserID(ctx), Action: action, RequestKey: in.RequestKey, RequestHash: hash, ResultJSON: string(b)}
		if e = tx.Create(&ev).Error; e != nil {
			return e
		}
		return warehouseAudit(tx, "transfer."+action, id, h.Version, operator)
	})
	return result, erpError(e)
}
