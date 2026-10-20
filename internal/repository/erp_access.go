package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// Original-row locks serialize correction writers; READ COMMITTED keeps a
// bounded grouped SUM current even if the transaction read idempotency earlier.
func erpTransaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return DB(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// Scope is live SQL, not a cached list supplied by a browser/JWT. Each subquery
// carries an explicit tenant predicate because it is not a GORM model query.
func erpWarehouseScope(q *gorm.DB, column string) *gorm.DB {
	ctx := q.Statement.Context
	uid := erpctx.UserID(ctx)
	tid := tenantctx.ID(ctx)
	if uid < 1 || tid < 1 || tenantctx.Get(ctx).Platform {
		q.AddError(errs.New("缺少企业ERP身份"))
		return q
	}
	return q.Where("(EXISTS (SELECT 1 FROM sys_tenant et WHERE et.tenant_id=? AND et.owner_user_id=?) OR "+column+" IN (SELECT warehouse_id FROM erp_warehouse_access WHERE tenant_id=? AND user_id=?))", tid, uid, tid, uid)
}
func erpOwner(tx *gorm.DB) (bool, error) {
	var n int64
	err := tx.Model(&model.SysTenant{}).Where("owner_user_id=?", erpctx.UserID(tx.Statement.Context)).Count(&n).Error
	return n == 1, err
}

// Every business write locks the authenticated user before document/warehouse.
// Grant replacement locks the same user, so revocation cannot race a write.
func lockERPActor(tx *gorm.DB) error {
	uid := erpctx.UserID(tx.Statement.Context)
	if uid < 1 || tenantctx.Get(tx.Statement.Context).Platform {
		return errs.New("缺少企业ERP身份")
	}
	var user model.SysUser
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND status='0' AND del_flag='0'", uid).Take(&user).Error; err != nil {
		return erpReferenceError(err)
	}
	return nil
}
func requireERPWarehouse(tx *gorm.DB, id int64) error {
	var n int64
	if err := erpWarehouseScope(tx.Model(&model.ErpWarehouse{}), "erp_warehouse.warehouse_id").Where("warehouse_id=?", id).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return errs.New("仓库不存在或无仓库权限")
	}
	return nil
}
func GetERPWarehouseAccess(ctx context.Context, userID int64) (*model.ERPAccessView, error) {
	var result *model.ERPAccessView
	err := Transaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		owner, err := erpOwner(tx)
		if err != nil {
			return err
		}
		if !owner {
			return errs.New("仅企业所有者可管理仓库授权")
		}
		var user model.SysUser
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND del_flag='0'", userID).Take(&user).Error; err != nil {
			return erpReferenceError(err)
		}
		var p model.ERPAccessPolicy
		err = tx.Where("user_id=?", userID).Take(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p.Version = 1
		} else if err != nil {
			return err
		}
		ids := []int64{}
		if err = tx.Model(&model.ERPWarehouseAccess{}).Where("user_id=?", userID).Order("warehouse_id").Pluck("warehouse_id", &ids).Error; err != nil {
			return err
		}
		result = &model.ERPAccessView{UserID: userID, WarehouseIDs: ids, Version: p.Version}
		return nil
	})
	return result, err
}
func ReplaceERPWarehouseAccess(ctx context.Context, userID int64, input model.ERPAccessInput, operator string) error {
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		owner, err := erpOwner(tx)
		if err != nil {
			return err
		}
		if !owner {
			return errs.New("仅企业所有者可管理仓库授权")
		}
		var user model.SysUser
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND del_flag='0'", userID).Take(&user).Error; err != nil {
			return erpReferenceError(err)
		}
		if userID == erpctx.UserID(ctx) {
			return errs.New("所有者拥有全部仓库，无需给自己授权")
		}
		var p model.ERPAccessPolicy
		err = tx.Where("user_id=?", userID).Take(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p = model.ERPAccessPolicy{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, UserID: userID, Version: 1}
			if err = tx.Create(&p).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if input.Version != p.Version {
			return errs.New("仓库授权已变更，请刷新后重试")
		}
		if len(input.WarehouseIDs) > 0 {
			var warehouses []model.ErpWarehouse
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id IN ?", input.WarehouseIDs).Order("warehouse_id").Find(&warehouses).Error; err != nil {
				return err
			}
			if len(warehouses) != len(input.WarehouseIDs) {
				return errs.New("仓库不存在或不属于当前企业")
			}
		}
		if err = tx.Where("user_id=?", userID).Delete(&model.ERPWarehouseAccess{}).Error; err != nil {
			return err
		}
		grants := make([]model.ERPWarehouseAccess, 0, len(input.WarehouseIDs))
		for _, id := range input.WarehouseIDs {
			grants = append(grants, model.ERPWarehouseAccess{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, UserID: userID, WarehouseID: id})
		}
		if len(grants) > 0 {
			if err = tx.Create(&grants).Error; err != nil {
				return err
			}
		}
		if err = tx.Model(&model.ERPAccessPolicy{}).Where("policy_id=?", p.PolicyID).Updates(map[string]any{"version": p.Version + 1, "update_by": operator, "update_time": types.Now()}).Error; err != nil {
			return err
		}
		encoded, _ := json.Marshal(map[string]any{"userId": userID, "warehouseIds": input.WarehouseIDs, "previousVersion": p.Version})
		if len(encoded) > 1900 {
			return errs.New("授权审计信息过长，请减少单次仓库数量")
		}
		return tx.Create(&model.SysOperLog{Title: "ERP仓库授权", BusinessType: model.BusinessTypeGrant, Method: "erp.warehouse.access", RequestMethod: "PUT", OperName: operator, OperParam: string(encoded), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
	}))
}
