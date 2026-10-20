package repository

import (
	"context"
	"errors"
	"fmt"

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
	err := erpWarehouseScope(TenantTable(DB(ctx), "erp_warehouse"), "erp_warehouse.warehouse_id").Where("warehouse_id = ?", id).Take(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询详情失败: %w", err)
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
		return tx.Create(target).Error
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
		result := erpWarehouseScope(TenantTable(tx, "erp_warehouse"), "erp_warehouse.warehouse_id").Where("warehouse_id = ?", target.WarehouseId).Updates(updates)
		if result.Error != nil {
			return erpError(fmt.Errorf("更新数据失败: %w", result.Error))
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("更新数据失败: 数据不存在")
		}
		return nil
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
		return tx.Where("warehouse_id IN ?", ids).Delete(&model.ErpWarehouse{}).Error
	}))
}
