package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"gorm.io/gorm"

	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

func erpWarehouseFilter(db *gorm.DB, query model.ErpWarehouseQuery) *gorm.DB {
	if query.WarehouseCode != "" {
		db = db.Where("warehouse_code = ?", query.WarehouseCode)
	}
	if query.WarehouseName != "" {
		db = db.Where("warehouse_name LIKE ?", "%"+query.WarehouseName+"%")
	}
	if query.Status != "" {
		db = db.Where("status=?", query.Status)
	}
	return db
}

func SelectErpWarehousePage(ctx context.Context, query model.ErpWarehouseQuery, pg page.Query) ([]model.ErpWarehouse, int64, error) {
	db := erpWarehouseFilter(erpWarehouseScope(TenantTable(DB(ctx), "erp_warehouse"), "erp_warehouse.warehouse_id"), query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计数据失败: %w", err)
	}
	if total == 0 {
		return []model.ErpWarehouse{}, 0, nil
	}
	var list []model.ErpWarehouse
	if err := db.Order(pg.Stable("warehouse_id", "warehouse_id")).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询数据失败: %w", err)
	}
	return list, total, nil
}

func SelectErpWarehouseList(ctx context.Context, query model.ErpWarehouseQuery, limit int) ([]model.ErpWarehouse, error) {
	var list []model.ErpWarehouse
	if err := erpWarehouseFilter(erpWarehouseScope(TenantTable(DB(ctx), "erp_warehouse"), "erp_warehouse.warehouse_id"), query).Order("warehouse_id").Limit(limit).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询导出数据失败: %w", err)
	}
	return list, nil
}

func SelectErpWarehouseByID(ctx context.Context, id string) (*model.ErpWarehouse, error) {
	var target model.ErpWarehouse
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := erpWarehouseScope(TenantTable(tx, "erp_warehouse"), "erp_warehouse.warehouse_id").Where("warehouse_id=?", id).Take(&target).Error; e != nil {
			return e
		}
		target.Version = 1
		if installed, e := warehouseOperationsInstalled(tx); e != nil {
			return e
		} else if installed {
			var p model.ERPWarehouseProfile
			if e = tx.Where("warehouse_id=?", target.WarehouseId).Take(&p).Error; e == nil {
				target.Address = p.Address
				target.Manager = p.Manager
				target.Version = p.Version
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func InsertErpWarehouse(ctx context.Context, target *model.ErpWarehouse) error {
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		owner, err := erpOwner(tx)
		if err != nil {
			return err
		}
		if !owner {
			return errs.New("仅企业所有者可创建仓库")
		}
		if err := tx.Create(target).Error; err != nil {
			return err
		}
		installed, e := warehouseOperationsInstalled(tx)
		if e != nil {
			return e
		}
		if !installed {
			return nil
		}
		target.Version = 1
		p := model.ERPWarehouseProfile{WarehouseID: target.WarehouseId, Address: target.Address, Manager: target.Manager, Version: 1, ERPRecord: model.ERPRecord{CreateBy: target.CreateBy, CreateTime: target.CreateTime}}
		if e = tx.Create(&p).Error; e != nil {
			return e
		}
		return warehouseAudit(tx, "warehouse.CREATE", target.WarehouseId, 1, target.CreateBy)
	}))
}

func UpdateErpWarehouse(ctx context.Context, target *model.ErpWarehouse) error {
	updates := map[string]any{
		"warehouse_code": target.WarehouseCode,
		"warehouse_name": target.WarehouseName,
		"status":         target.Status,
		"update_by":      target.UpdateBy,
		"update_time":    target.UpdateTime,
		"remark":         target.Remark,
	}
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		owner, err := erpOwner(tx)
		if err != nil {
			return err
		}
		if !owner {
			return errs.New("仅企业所有者可修改仓库资料")
		}
		if e := requireWarehouseOperations(tx); e != nil {
			return e
		}
		var current model.ErpWarehouse
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id=?", target.WarehouseId).Take(&current).Error; e != nil {
			return erpReferenceError(e)
		}
		var p model.ERPWarehouseProfile
		e := tx.Where("warehouse_id=?", target.WarehouseId).Take(&p).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			p = model.ERPWarehouseProfile{WarehouseID: target.WarehouseId, Version: 1, ERPRecord: model.ERPRecord{CreateBy: target.UpdateBy, CreateTime: target.UpdateTime}}
			if e = tx.Create(&p).Error; e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		if p.Version != target.Version || p.Version == math.MaxInt64 {
			return errs.New("仓库资料已变更，请刷新后重试")
		}
		if e = tx.Model(&model.ERPWarehouseProfile{}).Where("profile_id=? AND version=?", p.ProfileID, p.Version).Updates(map[string]any{"address": target.Address, "manager": target.Manager, "version": p.Version + 1, "update_by": target.UpdateBy, "update_time": target.UpdateTime}).Error; e != nil {
			return e
		}
		result := erpWarehouseScope(TenantTable(tx, "erp_warehouse"), "erp_warehouse.warehouse_id").Where("warehouse_id = ?", target.WarehouseId).Updates(updates)
		if result.Error != nil {
			return erpError(fmt.Errorf("更新数据失败: %w", result.Error))
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("更新数据失败: 数据不存在")
		}
		return warehouseAudit(tx, "warehouse.EDIT", target.WarehouseId, p.Version+1, target.UpdateBy)
	}))
}

func DeleteErpWarehouseByIDs(ctx context.Context, ids []string) error {
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		owner, err := erpOwner(tx)
		if err != nil {
			return err
		}
		if !owner {
			return errs.New("仅企业所有者可删除仓库")
		}
		var rows []model.ErpWarehouse
		if err := erpWarehouseScope(tx.Model(&model.ErpWarehouse{}), "erp_warehouse.warehouse_id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id IN ?", ids).Order("warehouse_id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return errs.New("仓库不存在或不属于当前企业")
		}
		installed, e := warehouseOperationsInstalled(tx)
		if e != nil {
			return e
		}
		if installed {
			if e = tx.Where("warehouse_id IN ?", ids).Delete(&model.ERPWarehouseProfile{}).Error; e != nil {
				return e
			}
		}
		return tx.Where("warehouse_id IN ?", ids).Delete(&model.ErpWarehouse{}).Error
	}))
}
