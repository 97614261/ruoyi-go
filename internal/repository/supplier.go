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
	"strings"
)

func supplierScope(q *gorm.DB) *gorm.DB {
	ctx := q.Statement.Context
	if tenantctx.ID(ctx) < 1 || tenantctx.Get(ctx).Platform || erpctx.UserID(ctx) < 1 {
		q.AddError(errs.New("缺少企业ERP身份"))
	}
	return q
}
func erpSupplierFilter(q *gorm.DB, in model.ErpSupplierQuery) *gorm.DB {
	if in.SupplierCode != "" {
		q = q.Where("supplier_code=?", in.SupplierCode)
	}
	if in.SupplierName != "" {
		literal := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(in.SupplierName)
		q = q.Where("supplier_name LIKE ? ESCAPE '!'", "%"+literal+"%")
	}
	if in.Status != "" {
		q = q.Where("status=?", in.Status)
	}
	return q
}
func SelectErpSupplierPage(ctx context.Context, in model.ErpSupplierQuery, pg page.Query) ([]model.ErpSupplierListRow, int64, error) {
	q := erpSupplierFilter(supplierScope(TenantTable(DB(ctx), "erp_supplier")), in)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ErpSupplierListRow{}
	err := q.Select("supplier_id,supplier_code,supplier_name,status,version,create_time,update_time").Order(pg.Stable("supplier_id DESC", "supplier_id")).Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
func SelectErpSupplierByID(ctx context.Context, id int64) (*model.ErpSupplier, error) {
	var target model.ErpSupplier
	err := supplierScope(DB(ctx).Model(&target)).Where("supplier_id=?", id).Take(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &target, err
}
func supplierSuccessAudit(tx *gorm.DB, target *model.ErpSupplier, action, operator string, from int64) error {
	raw, err := json.Marshal(map[string]any{"supplierId": target.SupplierId, "action": action, "fromVersion": from, "version": target.Version})
	if err != nil {
		return err
	}
	business, method := model.BusinessTypeUpdate, "PUT"
	if action == "create" {
		business = model.BusinessTypeInsert
		method = "POST"
	}
	return tx.Create(&model.SysOperLog{Title: "ERP供应商", BusinessType: business, Method: "erp.supplier." + action, RequestMethod: method, OperName: operator, OperParam: string(raw), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}
func InsertErpSupplier(ctx context.Context, target *model.ErpSupplier, operator string) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		if err := tx.Create(target).Error; err != nil {
			return err
		}
		return supplierSuccessAudit(tx, target, "create", operator, 0)
	}))
}
func UpdateErpSupplier(ctx context.Context, target *model.ErpSupplier, operator string) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		var old model.ErpSupplier
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("supplier_id=?", target.SupplierId).Take(&old).Error; err != nil {
			return erpReferenceError(err)
		}
		if old.Version != target.Version || old.Version == math.MaxInt64 {
			return errs.New("供应商版本已变化或到达上限，请刷新后重试")
		}
		if old.SupplierCode != target.SupplierCode {
			return errs.New("供应商编码创建后不可修改")
		}
		result := tx.Model(&model.ErpSupplier{}).Where("supplier_id=? AND version=?", old.SupplierId, old.Version).Updates(map[string]any{"supplier_name": target.SupplierName, "contact_name": target.ContactName, "phone": target.Phone, "email": target.Email, "address": target.Address, "status": target.Status, "remark": target.Remark, "version": old.Version + 1, "update_by": operator, "update_time": target.UpdateTime})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errs.New("供应商已变化，请刷新后重试")
		}
		target.Version = old.Version + 1
		return supplierSuccessAudit(tx, target, "update", operator, old.Version)
	}))
}
