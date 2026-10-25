package repository

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func requireSalesFinance(tx *gorm.DB) error {
	ctx := tx.Statement.Context
	if erpctx.UserID(ctx) < 1 || tenantctx.ID(ctx) < 1 || tenantctx.Get(ctx).Platform {
		return errs.New("缺少企业财务身份")
	}
	var n int64
	if e := tx.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_sf_receivable'").Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errs.New("请先备份并显式安装销售财务009")
	}
	return nil
}
func salesFinanceReplay(tx *gorm.DB, action string, id int64, key, hash, kind string) (*model.FinResult, error) {
	var ev model.SalesFinAction
	e := tx.Where("request_key=?", key).Take(&ev).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if ev.Action != action || ev.ActorUserID != erpctx.UserID(tx.Statement.Context) || ev.RequestHash != hash || id > 0 && ev.TargetID != id {
		return nil, errs.New("同一财务请求标识不能用于不同内容、动作或操作者")
	}
	var r model.FinResult
	if json.Unmarshal([]byte(ev.ResultJSON), &r) != nil || r.ID != ev.TargetID || r.ID < 1 || r.Version < 1 || r.Kind != kind {
		return nil, errs.New("财务动作原结果记录异常")
	}
	r.Existing = true
	return &r, nil
}
func salesFinanceRecord(tx *gorm.DB, action, key, hash, operator string, r *model.FinResult) error {
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	ev := model.SalesFinAction{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ActorUserID: erpctx.UserID(tx.Statement.Context), Action: action, TargetID: r.ID, RequestKey: key, RequestHash: hash, ResultJSON: string(raw)}
	if e = tx.Create(&ev).Error; e != nil {
		return e
	}
	// No amounts, account/beneficiary text, source payload or reason in generic logs.
	audit, _ := json.Marshal(map[string]any{"id": r.ID, "version": r.Version, "kind": r.Kind, "action": action})
	return tx.Create(&model.SysOperLog{Title: "ERP销售财务", BusinessType: model.BusinessTypeUpdate, Method: "erp.salesFinance." + action, RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}
func salesFinanceVersion(v int64) error {
	if v < 1 || v == math.MaxInt64 {
		return errs.New("财务版本异常或耗尽，请核对")
	}
	return nil
}
func salesFinanceReceivableTotals(p *model.SalesFinReceivable) error {
	var e error
	p.NetAmount, e = decimalx.Subtract(p.GrossAmount, p.CreditAmount)
	if e != nil {
		return errs.New("应收及抵减金额记录异常")
	}
	cmp, e := decimalx.Compare(p.ReceivedAmount, p.NetAmount)
	if e != nil {
		return errs.New("核销金额记录异常")
	}
	p.DueAmount, p.OverreceivedAmount = "0.0000", "0.0000"
	if cmp <= 0 {
		p.DueAmount, e = decimalx.Subtract(p.NetAmount, p.ReceivedAmount)
	} else {
		p.OverreceivedAmount, e = decimalx.Subtract(p.ReceivedAmount, p.NetAmount)
	}
	return e
}
func salesFinSaveReceivable(tx *gorm.DB, p *model.SalesFinReceivable, operator string) error {
	if e := salesFinanceVersion(p.Version); e != nil {
		return e
	}
	p.Version++
	return tx.Model(&model.SalesFinReceivable{}).Where("receivable_id=?", p.ReceivableID).Updates(map[string]any{"credit_amount": p.CreditAmount, "received_amount": p.ReceivedAmount, "version": p.Version, "update_by": operator, "update_time": types.Now()}).Error
}

func salesFinanceReceivableScope(q *gorm.DB) *gorm.DB {
	return erpShopColumnScope(erpWarehouseScope(q, "erp_sf_receivable.warehouse_id"), "erp_sf_receivable.shop_id")
}
func salesFinanceCollectionScope(q *gorm.DB) *gorm.DB {
	ctx := q.Statement.Context
	tid, uid := tenantctx.ID(ctx), erpctx.UserID(ctx)
	if tid < 1 || uid < 1 || tenantctx.Get(ctx).Platform {
		q.AddError(errs.New("缺少企业财务身份"))
		return q
	}
	return q.Where(`EXISTS (SELECT 1 FROM erp_sf_allocation a WHERE a.tenant_id=? AND a.collection_id=erp_sf_collection.collection_id)
 AND NOT EXISTS (SELECT 1 FROM erp_sf_allocation a JOIN erp_sf_receivable r ON r.tenant_id=a.tenant_id AND r.receivable_id=a.receivable_id
 WHERE a.tenant_id=? AND a.collection_id=erp_sf_collection.collection_id AND NOT
 (EXISTS (SELECT 1 FROM sys_tenant t WHERE t.tenant_id=? AND t.owner_user_id=?) OR
 (r.shop_id IN (SELECT shop_id FROM erp_shop_access WHERE tenant_id=? AND user_id=?)
 AND r.warehouse_id IN (SELECT warehouse_id FROM erp_warehouse_access WHERE tenant_id=? AND user_id=?))))`, tid, tid, tid, uid, tid, uid, tid, uid)
}
