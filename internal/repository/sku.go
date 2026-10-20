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

func erpSkuFilter(db *gorm.DB, query model.ErpSkuQuery) *gorm.DB {
	if query.SkuCode != "" {
		db = db.Where("sku_code = ?", query.SkuCode)
	}
	if query.ProductName != "" {
		db = db.Where("product_name LIKE ?", "%"+query.ProductName+"%")
	}
	if query.Specification != "" {
		db = db.Where("specification = ?", query.Specification)
	}
	if query.Unit != "" {
		db = db.Where("unit = ?", query.Unit)
	}
	return db
}

func SelectErpSkuPage(ctx context.Context, query model.ErpSkuQuery, pg page.Query) ([]model.ErpSku, int64, error) {
	db := erpSkuFilter(TenantTable(DB(ctx), "erp_sku"), query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计数据失败: %w", err)
	}
	if total == 0 {
		return []model.ErpSku{}, 0, nil
	}
	var list []model.ErpSku
	if err := db.Order(pg.Stable("sku_id", "sku_id")).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询数据失败: %w", err)
	}
	return list, total, nil
}

func SelectErpSkuList(ctx context.Context, query model.ErpSkuQuery, limit int) ([]model.ErpSku, error) {
	var list []model.ErpSku
	if err := erpSkuFilter(TenantTable(DB(ctx), "erp_sku"), query).Order("sku_id").Limit(limit).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询导出数据失败: %w", err)
	}
	return list, nil
}

func SelectErpSkuByID(ctx context.Context, id string) (*model.ErpSku, error) {
	var target model.ErpSku
	err := TenantTable(DB(ctx), "erp_sku").Where("sku_id = ?", id).Take(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询详情失败: %w", err)
	}
	return &target, nil
}

func InsertErpSku(ctx context.Context, target *model.ErpSku) error {
	if err := TenantTable(DB(ctx), "erp_sku").Create(target).Error; err != nil {
		return erpError(fmt.Errorf("新增数据失败: %w", err))
	}
	return nil
}

func UpdateErpSku(ctx context.Context, target *model.ErpSku) error {
	updates := map[string]any{
		"sku_code":      target.SkuCode,
		"product_name":  target.ProductName,
		"specification": target.Specification,
		"unit":          target.Unit,
		"status":        target.Status,
		"update_by":     target.UpdateBy,
		"update_time":   target.UpdateTime,
		"remark":        target.Remark,
	}
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		var old model.ErpSku
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sku_id=?", target.SkuId).Take(&old).Error; err != nil {
			return erpReferenceError(err)
		}
		if old.Unit != target.Unit {
			var lines, stocks int64
			if err := tx.Model(&model.ERPDocumentLine{}).Where("sku_id=?", target.SkuId).Count(&lines).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.ERPStock{}).Where("sku_id=?", target.SkuId).Count(&stocks).Error; err != nil {
				return err
			}
			if lines > 0 || stocks > 0 {
				return errs.New("商品已被单据或库存引用，不能修改计量单位")
			}
		}
		return tx.Model(&model.ErpSku{}).Where("sku_id=?", target.SkuId).Updates(updates).Error
	}))
}

func DeleteErpSkuByIDs(ctx context.Context, ids []string) error {
	return erpError(Transaction(ctx, func(tx *gorm.DB) error {
		var rows []model.ErpSku
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sku_id IN ?", ids).Order("sku_id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return errs.New("商品不存在或不属于当前企业")
		}
		return tx.Where("sku_id IN ?", ids).Delete(&model.ErpSku{}).Error
	}))
}
