package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func requireFinance(tx *gorm.DB) error {
	ctx := tx.Statement.Context
	if erpctx.UserID(ctx) < 1 || tenantctx.ID(ctx) < 1 || tenantctx.Get(ctx).Platform {
		return errs.New("缺少企业财务身份")
	}
	var n int64
	if e := tx.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_fin_payable'").Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errs.New("请先备份并显式安装采购财务008")
	}
	return nil
}
func financeReplay(tx *gorm.DB, action string, id int64, key, hash, kind string) (*model.FinResult, error) {
	var ev model.FinAction
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
func financeRecord(tx *gorm.DB, action, key, hash, operator string, r *model.FinResult) error {
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	ev := model.FinAction{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ActorUserID: erpctx.UserID(tx.Statement.Context), Action: action, TargetID: r.ID, RequestKey: key, RequestHash: hash, ResultJSON: string(raw)}
	if e = tx.Create(&ev).Error; e != nil {
		return e
	}
	// No amounts, account/beneficiary text, source payload or reason in generic logs.
	audit, _ := json.Marshal(map[string]any{"id": r.ID, "version": r.Version, "kind": r.Kind, "action": action})
	return tx.Create(&model.SysOperLog{Title: "ERP采购财务", BusinessType: model.BusinessTypeUpdate, Method: "erp.finance." + action, RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}
func financeVersion(v int64) error {
	if v < 1 || v == math.MaxInt64 {
		return errs.New("财务版本异常或耗尽，请核对")
	}
	return nil
}
func financePayableTotals(p *model.FinPayable) error {
	var e error
	p.NetAmount, e = decimalx.Subtract(p.GrossAmount, p.CreditAmount)
	if e != nil {
		return errs.New("应付及抵减金额记录异常")
	}
	cmp, e := decimalx.Compare(p.PaidAmount, p.NetAmount)
	if e != nil {
		return errs.New("核销金额记录异常")
	}
	p.DueAmount, p.OverpaidAmount = "0.0000", "0.0000"
	if cmp <= 0 {
		p.DueAmount, e = decimalx.Subtract(p.NetAmount, p.PaidAmount)
	} else {
		p.OverpaidAmount, e = decimalx.Subtract(p.PaidAmount, p.NetAmount)
	}
	return e
}
func finSavePayable(tx *gorm.DB, p *model.FinPayable, operator string) error {
	if e := financeVersion(p.Version); e != nil {
		return e
	}
	p.Version++
	return tx.Model(&model.FinPayable{}).Where("payable_id=?", p.PayableID).Updates(map[string]any{"credit_amount": p.CreditAmount, "paid_amount": p.PaidAmount, "version": p.Version, "update_by": operator, "update_time": types.Now()}).Error
}
func CreateFinAccount(ctx context.Context, in model.FinAccountWrite, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		r, e := financeReplay(tx, "ACCOUNT_CREATE", 0, in.RequestKey, hash, "account")
		if e != nil {
			return e
		}
		if r != nil {
			var a model.FinAccount
			if e = tx.Where("account_id=?", r.ID).Take(&a).Error; e != nil {
				return erpReferenceError(e)
			}
			out = r
			return nil
		}
		a := model.FinAccount{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, AccountCode: in.AccountCode, AccountName: in.AccountName, AccountType: in.AccountType, Currency: in.Currency, Status: in.Status, Version: 1}
		if e = tx.Create(&a).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: a.AccountID, Version: 1, Kind: "account", Status: a.Status}
		return financeRecord(tx, "ACCOUNT_CREATE", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}
func EditFinAccount(ctx context.Context, id int64, in model.FinAccountWrite, hash, operator string) (*model.FinResult, error) {
	var out *model.FinResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireFinance(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var a model.FinAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=?", id).Take(&a).Error; e != nil {
			return erpReferenceError(e)
		}
		r, e := financeReplay(tx, "ACCOUNT_EDIT", id, in.RequestKey, hash, "account")
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		if a.Version != in.Version {
			return errs.New("账户已修改，请刷新核对")
		}
		if e = financeVersion(a.Version); e != nil {
			return e
		}
		if a.AccountCode != in.AccountCode || a.Currency != in.Currency || a.AccountType != in.AccountType {
			return errs.New("账户编码、类型和币种创建后不可修改")
		}
		a.Version++
		if e = tx.Model(&a).Updates(map[string]any{"account_name": in.AccountName, "status": in.Status, "remark": in.Remark, "version": a.Version, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		out = &model.FinResult{ID: id, Kind: "account", Version: a.Version, Status: in.Status}
		return financeRecord(tx, "ACCOUNT_EDIT", in.RequestKey, hash, operator, out)
	})
	return out, erpError(e)
}

func financePayableScope(q *gorm.DB) *gorm.DB {
	return erpWarehouseScope(q, "erp_fin_payable.warehouse_id")
}

// A multi-warehouse payment is all-or-nothing visible. Every nested SQL table
// has an explicit tenant predicate; no partial total leaks to a single side.
func financePaymentScope(q *gorm.DB) *gorm.DB {
	ctx := q.Statement.Context
	tid, uid := tenantctx.ID(ctx), erpctx.UserID(ctx)
	if tid < 1 || uid < 1 || tenantctx.Get(ctx).Platform {
		q.AddError(errs.New("缺少企业财务身份"))
		return q
	}
	return q.Where(`EXISTS (SELECT 1 FROM erp_fin_allocation fa WHERE fa.tenant_id=? AND fa.payment_id=erp_fin_payment.payment_id)
 AND NOT EXISTS (SELECT 1 FROM erp_fin_allocation fa JOIN erp_fin_payable fp ON fp.tenant_id=fa.tenant_id AND fp.payable_id=fa.payable_id
 WHERE fa.tenant_id=? AND fa.payment_id=erp_fin_payment.payment_id AND NOT
 (EXISTS (SELECT 1 FROM sys_tenant ft WHERE ft.tenant_id=? AND ft.owner_user_id=?) OR fp.warehouse_id IN
 (SELECT warehouse_id FROM erp_warehouse_access WHERE tenant_id=? AND user_id=?)))`, tid, tid, tid, uid, tid, uid)
}
