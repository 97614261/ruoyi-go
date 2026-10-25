package repository

import (
	"context"
	"encoding/json"
	"errors"
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

func requireLogistics(tx *gorm.DB) error {
	ctx := tx.Statement.Context
	if erpctx.UserID(ctx) < 1 || tenantctx.ID(ctx) < 1 || tenantctx.Get(ctx).Platform {
		return errs.New("缺少企业物流身份")
	}
	var n int64
	if e := tx.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name IN ?", logisticsBusinessTables).Count(&n).Error; e != nil {
		return e
	}
	if n != 4 {
		return errs.New("请先备份并显式安装物流基础011")
	}
	return nil
}
func logisticsPresent(tx *gorm.DB) (bool, error) {
	var n int64
	e := tx.WithContext(tenantctx.System(tx.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_package_shipping'").Count(&n).Error
	return n == 1, e
}
func logisticsReplay(tx *gorm.DB, key, hash, kind string) (*model.LogisticsResult, error) {
	var a model.LogisticsAction
	e := tx.Where("request_key=?", key).Take(&a).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if a.RequestHash != hash || a.ActorID != erpctx.UserID(tx.Statement.Context) || a.ActionKind != kind {
		return nil, errs.New("同一请求标识不能用于不同内容、动作或员工")
	}
	var r model.LogisticsResult
	if json.Unmarshal([]byte(a.ResultJSON), &r) != nil || r.ID < 1 || r.Version < 1 {
		return nil, errs.New("物流动作原结果异常")
	}
	r.Existing = true
	return &r, nil
}
func logisticsRecord(tx *gorm.DB, key, hash, kind, operator string, r *model.LogisticsResult) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if e = tx.Create(&model.LogisticsAction{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, RequestKey: key, RequestHash: hash, ActorID: erpctx.UserID(tx.Statement.Context), ActionKind: kind, ResultJSON: string(b)}).Error; e != nil {
		return e
	}
	audit, _ := json.Marshal(map[string]any{"id": r.ID, "version": r.Version, "kind": r.Kind})
	return tx.Create(&model.SysOperLog{Title: "ERP物流设置", BusinessType: model.BusinessTypeUpdate, Method: "erp.logistics." + kind, RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}
func lockLogisticsCarrier(tx *gorm.DB, id int64, active bool) (model.LogisticsCarrier, error) {
	var c model.LogisticsCarrier
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("carrier_id=?", id).Take(&c).Error; e != nil {
		return c, erpReferenceError(e)
	}
	if active && c.Status != "0" {
		return c, errs.New("物流公司已停用，不能新选")
	}
	return c, nil
}
func WriteLogisticsMaster(ctx context.Context, id int64, in model.LogisticsWrite, method bool, hash, operator string) (*model.LogisticsResult, error) {
	var out *model.LogisticsResult
	kind := "CARRIER"
	if method {
		kind = "METHOD"
	}
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireLogistics(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		r, e := logisticsReplay(tx, in.RequestKey, hash, kind)
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		if !method {
			c := model.LogisticsCarrier{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, CarrierCode: in.Code, CarrierName: in.Name, Status: in.Status, Version: 1}
			if id == 0 {
				e = tx.Create(&c).Error
			} else {
				c, e = lockLogisticsCarrier(tx, id, false)
				if e != nil {
					return e
				}
				if c.CarrierCode != in.Code {
					return errs.New("物流公司编码建立后不可修改")
				}
				if c.Version != in.Version || c.Version == math.MaxInt64 || c.Version < 1 {
					return errs.New("物流公司已变化，请刷新核对")
				}
				e = tx.Model(&c).Where("version=?", in.Version).Updates(map[string]any{"carrier_name": in.Name, "status": in.Status, "version": in.Version + 1, "remark": in.Remark, "update_by": operator, "update_time": types.Now()}).Error
				c.Version++
			}
			if e != nil {
				return e
			}
			out = &model.LogisticsResult{ID: c.CarrierID, Version: c.Version, Kind: kind}
		} else {
			if _, e := lockLogisticsCarrier(tx, in.CarrierID, id == 0); e != nil {
				return e
			}
			m := model.ShippingMethod{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, CarrierID: in.CarrierID, MethodCode: in.Code, MethodName: in.Name, Status: in.Status, Version: 1}
			if id == 0 {
				e = tx.Create(&m).Error
			} else {
				if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shipping_method_id=?", id).Take(&m).Error; e != nil {
					return erpReferenceError(e)
				}
				if m.CarrierID != in.CarrierID || m.MethodCode != in.Code {
					return errs.New("配送方式编码和所属物流公司建立后不可修改")
				}
				if m.Version != in.Version || m.Version == math.MaxInt64 || m.Version < 1 {
					return errs.New("配送方式已变化，请刷新核对")
				}
				e = tx.Model(&m).Where("version=?", in.Version).Updates(map[string]any{"method_name": in.Name, "status": in.Status, "version": in.Version + 1, "remark": in.Remark, "update_by": operator, "update_time": types.Now()}).Error
				m.Version++
			}
			if e != nil {
				return e
			}
			out = &model.LogisticsResult{ID: m.ShippingMethodID, Version: m.Version, Kind: kind}
		}
		return logisticsRecord(tx, in.RequestKey, hash, kind, operator, out)
	})
	return out, erpError(e)
}
func logisticsMasterQuery(tx *gorm.DB, in model.LogisticsQuery, method bool) (*gorm.DB, string, string) {
	q := tx.Model(&model.LogisticsCarrier{})
	prefix := "erp_carrier"
	code := "carrier_code"
	name := "carrier_name"
	id := "carrier_id"
	cols := "erp_carrier.carrier_id AS id,erp_carrier.carrier_code AS code,erp_carrier.carrier_name AS name,0 AS carrier_id,'' AS carrier_code,'' AS carrier_name,erp_carrier.status,erp_carrier.version,erp_carrier.remark,erp_carrier.create_time"
	if method {
		q = tx.Model(&model.ShippingMethod{}).Joins("JOIN erp_carrier c ON c.carrier_id=erp_shipping_method.carrier_id")
		prefix = "erp_shipping_method"
		code = "method_code"
		name = "method_name"
		id = "shipping_method_id"
		cols = "erp_shipping_method.shipping_method_id AS id,erp_shipping_method.method_code AS code,erp_shipping_method.method_name AS name,erp_shipping_method.carrier_id,c.carrier_code,c.carrier_name,erp_shipping_method.status,erp_shipping_method.version,erp_shipping_method.remark,erp_shipping_method.create_time"
		if in.CarrierID > 0 {
			q = q.Where("erp_shipping_method.carrier_id=?", in.CarrierID)
		}
	}
	if in.Code != "" {
		q = q.Where(prefix+"."+code+"=?", in.Code)
	}
	if in.Name != "" {
		q = q.Where(prefix+"."+name+" LIKE ?", "%"+in.Name+"%")
	}
	if in.Status != "" {
		q = q.Where(prefix+".status=?", in.Status)
	}
	return q, cols, prefix + "." + id
}
func SelectLogisticsMasters(ctx context.Context, in model.LogisticsQuery, pg page.Query, method bool) ([]model.LogisticsMasterView, int64, error) {
	base := DB(ctx)
	if e := requireLogistics(base); e != nil {
		return nil, 0, e
	}
	q, cols, order := logisticsMasterQuery(base, in, method)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.LogisticsMasterView{}
	e := q.Select(cols).Order(order + " DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, n, e
}
func SelectLogisticsMaster(ctx context.Context, id int64, method bool) (*model.LogisticsMasterView, error) {
	base := DB(ctx)
	if e := requireLogistics(base); e != nil {
		return nil, e
	}
	q, cols, key := logisticsMasterQuery(base, model.LogisticsQuery{}, method)
	var out model.LogisticsMasterView
	e := q.Select(cols).Where(key+"=?", id).Take(&out).Error
	return &out, erpReferenceError(e)
}
func SelectLogisticsOptions(ctx context.Context, in model.LogisticsQuery, pg page.Query) ([]model.LogisticsOption, int64, error) {
	base := DB(ctx)
	if e := requireLogistics(base); e != nil {
		return nil, 0, e
	}
	in.Status = "0"
	q, _, key := logisticsMasterQuery(base, in, true)
	q = q.Where("c.status='0'")
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.LogisticsOption{}
	e := q.Select("erp_shipping_method.shipping_method_id AS id,erp_shipping_method.method_code AS code,erp_shipping_method.method_name AS name,erp_shipping_method.carrier_id,c.carrier_code,c.carrier_name").Order(key + " DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, n, e
}
