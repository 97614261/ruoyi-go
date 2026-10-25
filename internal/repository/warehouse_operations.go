package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func warehouseOperationsInstalled(tx *gorm.DB) (bool, error) {
	var n int64
	e := tx.WithContext(tenantctx.System(tx.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_transfer'").Count(&n).Error
	return n == 1, e
}
func requireWarehouseOperations(tx *gorm.DB) error {
	ok, e := warehouseOperationsInstalled(tx)
	if e != nil {
		return e
	}
	if !ok {
		return errs.New("请先备份并显式安装仓库增强007")
	}
	return nil
}
func warehouseAudit(tx *gorm.DB, action string, id, version int64, operator string) error {
	b, _ := json.Marshal(map[string]any{"id": id, "version": version, "action": action})
	return tx.Create(&model.SysOperLog{Title: "ERP仓库作业", BusinessType: model.BusinessTypeUpdate, Method: "erp." + action, RequestMethod: "POST", OperName: operator, OperParam: string(b), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}

func SetERPStockAlert(ctx context.Context, in model.ERPStockAlert, operator string) (*model.ERPStockAlert, error) {
	var out model.ERPStockAlert
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		if e := requireWarehouseOperations(tx); e != nil {
			return e
		}
		if e := requireERPWarehouse(tx, in.WarehouseID); e != nil {
			return e
		}
		if _, _, e := lockERPReferences(tx, in.WarehouseID, []int64{in.SkuID}, false); e != nil {
			return e
		}
		e := tx.Where("warehouse_id=? AND sku_id=?", in.WarehouseID, in.SkuID).Take(&out).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if in.Version != 0 {
				return errs.New("预警配置已变更，请刷新")
			}
			out = model.ERPStockAlert{WarehouseID: in.WarehouseID, SkuID: in.SkuID, ThresholdQuantity: in.ThresholdQuantity, Version: 1, ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}}
			if e = tx.Create(&out).Error; e != nil {
				return e
			}
		} else if e != nil {
			return e
		} else {
			if out.Version != in.Version || out.Version == math.MaxInt64 {
				return errs.New("预警配置已变更，请刷新")
			}
			out.ThresholdQuantity = in.ThresholdQuantity
			out.Version++
			if e = tx.Model(&model.ERPStockAlert{}).Where("alert_id=?", out.AlertID).Updates(map[string]any{"threshold_quantity": out.ThresholdQuantity, "version": out.Version, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
				return e
			}
		}
		return warehouseAudit(tx, "stock.ALERT", out.AlertID, out.Version, operator)
	})
	return &out, erpError(e)
}

func GetERPStockAlert(ctx context.Context, w, s int64) (*model.ERPStockAlert, error) {
	tx := DB(ctx)
	if e := requireERPWarehouse(tx, w); e != nil {
		return nil, e
	}
	if e := requireWarehouseOperations(tx); e != nil {
		return nil, e
	}
	var sku model.ErpSku
	if e := tx.Where("sku_id=?", s).Take(&sku).Error; e != nil {
		return nil, erpReferenceError(e)
	}
	out := model.ERPStockAlert{WarehouseID: w, SkuID: s}
	if e := tx.Where("warehouse_id=? AND sku_id=?", w, s).Take(&out).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	return &out, nil
}
