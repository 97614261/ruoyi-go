package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func erpShopItemScope(q *gorm.DB) *gorm.DB { return erpShopColumnScope(q, "erp_shop_item.shop_id") }
func shopItemViewQuery(tx *gorm.DB) *gorm.DB {
	return erpShopItemScope(TenantTable(tx, "erp_shop_item")).
		Joins("JOIN erp_shop s ON s.shop_id=erp_shop_item.shop_id").
		Joins("LEFT JOIN erp_shop_sku_mapping m ON m.shop_item_id=erp_shop_item.item_id AND m.shop_id=erp_shop_item.shop_id").
		Joins("LEFT JOIN erp_sku k ON k.sku_id=m.sku_id")
}

const shopItemViewColumns = "erp_shop_item.*, s.shop_code, s.shop_name, s.status AS shop_status, m.mapping_id, m.version AS mapping_version, m.status AS mapping_status, m.sku_id, k.sku_code, k.product_name, k.unit, k.status AS sku_status"

func SelectERPShopItemPage(ctx context.Context, q model.ERPShopItemQuery, pg page.Query) ([]model.ERPShopItemView, int64, error) {
	tx := shopItemViewQuery(DB(ctx))
	if q.ShopID > 0 {
		tx = tx.Where("erp_shop_item.shop_id=?", q.ShopID)
	}
	if q.ShopItemCode != "" {
		tx = tx.Where("erp_shop_item.shop_item_code=?", q.ShopItemCode)
	}
	if q.Title != "" {
		tx = tx.Where("erp_shop_item.title LIKE ?", "%"+q.Title+"%")
	}
	if q.Status != "" {
		tx = tx.Where("erp_shop_item.status=?", q.Status)
	}
	if q.MappingState == "MAPPED" {
		tx = tx.Where("m.status='0'")
	} else if q.MappingState == "UNMAPPED" {
		tx = tx.Where("(m.mapping_id IS NULL OR m.status<>'0')")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPShopItemView{}
	if total == 0 {
		return rows, 0, nil
	}
	err := tx.Select(shopItemViewColumns).Order(pg.Stable("erp_shop_item.item_id", "erp_shop_item.item_id")).Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
func SelectERPShopItem(ctx context.Context, id int64) (*model.ERPShopItemView, error) {
	var result model.ERPShopItemView
	err := shopItemViewQuery(DB(ctx)).Select(shopItemViewColumns).Where("erp_shop_item.item_id=?", id).Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.New("商品不存在或无店铺权限")
	}
	return &result, err
}
func lockShopForItem(tx *gorm.DB, id int64, active bool) (model.ErpShop, error) {
	var shop model.ErpShop
	err := erpShopScope(tx.Model(&model.ErpShop{})).Clauses(clause.Locking{Strength: "UPDATE"}).Where("shop_id=?", id).Take(&shop).Error
	if err != nil {
		return shop, erpReferenceError(err)
	}
	if active && shop.Status != "0" {
		return shop, errs.New("店铺已停用")
	}
	return shop, nil
}

// Parent shop lock serializes item/mapping writes and future order-review
// writers. Keep actor -> shop -> item -> mapping -> SKU; never lock in reverse.
func lockShopItem(tx *gorm.DB, id int64, active bool) (model.ErpShopItem, error) {
	var locator model.ErpShopItem
	if err := erpShopItemScope(tx.Model(&model.ErpShopItem{})).Select("item_id,shop_id").Where("item_id=?", id).Take(&locator).Error; err != nil {
		return locator, erpReferenceError(err)
	}
	if _, err := lockShopForItem(tx, locator.ShopID, active); err != nil {
		return locator, err
	}
	var item model.ErpShopItem
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("item_id=? AND shop_id=?", id, locator.ShopID).Take(&item).Error
	return item, erpReferenceError(err)
}
func checkShopItemVersion(item model.ErpShopItem, expected int64) error {
	if expected < 1 || expected != item.Version {
		return errs.New("商品或配对已变更，请刷新后重试")
	}
	if item.Version == math.MaxInt64 {
		return errs.New("商品版本已达上限")
	}
	return nil
}
func InsertERPShopItem(ctx context.Context, target *model.ErpShopItem) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		if _, err := lockShopForItem(tx, target.ShopID, true); err != nil {
			return err
		}
		return tx.Create(target).Error
	}))
}
func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func UpdateERPShopItem(ctx context.Context, target *model.ErpShopItem) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		old, err := lockShopItem(tx, target.ItemID, true)
		if err != nil {
			return err
		}
		if old.ShopID != target.ShopID {
			return errs.New("商品不能转移到其他店铺")
		}
		if err = checkShopItemVersion(old, target.Version); err != nil {
			return err
		}
		identityChanged := old.ShopItemCode != target.ShopItemCode || !sameOptionalString(old.ExternalProductID, target.ExternalProductID) || !sameOptionalString(old.ExternalVariantID, target.ExternalVariantID)
		if identityChanged {
			var mappings []int
			if err = tx.Model(&model.ERPShopSkuMapping{}).Select("1").Where("shop_item_id=?", old.ItemID).Limit(1).Scan(&mappings).Error; err != nil {
				return err
			}
			if len(mappings) > 0 {
				return errs.New("商品有配对历史，店铺SKU编码和外部标识不能修改")
			}
			var lines []int
			if err = TenantTable(tx, "erp_order_line").Select("1").Where("shop_id=? AND shop_item_id=?", old.ShopID, old.ItemID).Limit(1).Scan(&lines).Error; err != nil {
				return err
			}
			if len(lines) > 0 {
				return errs.New("商品已被订单引用，店铺SKU编码和外部标识不能修改")
			}
		}
		result := tx.Model(&model.ErpShopItem{}).Where("item_id=? AND version=?", old.ItemID, old.Version).Updates(map[string]any{"shop_item_code": target.ShopItemCode, "external_product_id": target.ExternalProductID, "external_variant_id": target.ExternalVariantID, "title": target.Title, "status": target.Status, "version": old.Version + 1, "update_by": target.UpdateBy, "update_time": target.UpdateTime, "remark": target.Remark})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errs.New("商品已变更，请刷新后重试")
		}
		return nil
	}))
}
func requireMappingUnreferencedByOpenOrder(tx *gorm.DB, item model.ErpShopItem, mapping model.ERPShopSkuMapping) error {
	if mapping.MappingID == 0 {
		return nil
	}
	var hits []int
	// Explicit table/tenant guards also apply to this simple JOIN. No row loop.
	err := TenantTable(tx, "erp_order_line").Joins("JOIN erp_order o ON o.order_id=erp_order_line.order_id AND o.shop_id=erp_order_line.shop_id").Select("1").Where("erp_order_line.shop_id=? AND erp_order_line.shop_item_id=? AND erp_order_line.mapping_id=?", item.ShopID, item.ItemID, mapping.MappingID).Where("o.status NOT IN ?", []string{"SHIPPED", "CANCELLED"}).Limit(1).Scan(&hits).Error
	if err != nil {
		return err
	}
	if len(hits) > 0 {
		return errs.New("配对被未结束订单引用，请先完成或取消相关订单")
	}
	return nil
}
func ChangeERPShopItemMapping(ctx context.Context, id, version, skuID int64, remark, operator string, remove bool) (*model.ERPMappingResult, error) {
	var result *model.ERPMappingResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		item, err := lockShopItem(tx, id, !remove)
		if err != nil {
			return err
		}
		if err = checkShopItemVersion(item, version); err != nil {
			return err
		}
		if !remove && item.Status != "0" {
			return errs.New("商品已停用，不能建立配对")
		}
		var mapping model.ERPShopSkuMapping
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shop_item_id=? AND shop_id=?", item.ItemID, item.ShopID).Take(&mapping).Error
		exists := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if remove && (!exists || mapping.Status != "0") {
			return errs.New("商品当前没有有效配对")
		}
		if mapping.Version == math.MaxInt64 {
			return errs.New("配对版本已达上限")
		}
		if err = requireMappingUnreferencedByOpenOrder(tx, item, mapping); err != nil {
			return err
		}
		previousSKU, previousVersion := mapping.SkuID, mapping.Version
		status := "0"
		if remove {
			status = "1"
			skuID = mapping.SkuID
		} else {
			var sku model.ErpSku
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sku_id=?", skuID).Take(&sku).Error; err != nil {
				return erpReferenceError(err)
			}
			if sku.Status != "0" {
				return errs.New("内部SKU已停用，不能建立配对")
			}
		}
		now := types.Now()
		if !exists {
			mapping = model.ERPShopSkuMapping{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: now, Remark: remark}, ShopID: item.ShopID, ShopItemID: item.ItemID, SkuID: skuID, Status: status, Version: 1}
			if err = tx.Create(&mapping).Error; err != nil {
				return err
			}
		} else {
			updates := map[string]any{"sku_id": skuID, "status": status, "version": mapping.Version + 1, "update_by": operator, "update_time": now}
			if !remove {
				updates["remark"] = remark
			}
			if err = tx.Model(&model.ERPShopSkuMapping{}).Where("mapping_id=? AND version=?", mapping.MappingID, mapping.Version).Updates(updates).Error; err != nil {
				return err
			}
			mapping.Version++
			mapping.Status = status
		}
		changed := tx.Model(&model.ErpShopItem{}).Where("item_id=? AND version=?", item.ItemID, item.Version).Updates(map[string]any{"version": item.Version + 1, "update_by": operator, "update_time": now})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return errs.New("商品或配对已变更，请刷新后重试")
		}
		action := "set"
		if remove {
			action = "cancel"
		}
		payload, err := json.Marshal(map[string]any{"itemId": item.ItemID, "shopId": item.ShopID, "mappingId": mapping.MappingID, "action": action, "previousSkuId": previousSKU, "skuId": skuID, "previousMappingVersion": previousVersion, "mappingVersion": mapping.Version, "previousVersion": item.Version, "version": item.Version + 1})
		if err != nil {
			return err
		}
		method := "PUT"
		if remove {
			method = "DELETE"
		}
		if err = tx.Create(&model.SysOperLog{Title: "ERP商品配对", BusinessType: model.BusinessTypeUpdate, Method: "erp.shopItem.mapping", RequestMethod: method, OperName: operator, OperParam: string(payload), JSONResult: `{"code":200}`, OperTime: now}).Error; err != nil {
			return err
		}
		result = &model.ERPMappingResult{ItemID: item.ItemID, Version: item.Version + 1, MappingID: mapping.MappingID, MappingVersion: mapping.Version, MappingStatus: status}
		return nil
	}))
	return result, err
}
