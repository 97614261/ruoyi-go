package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"sort"
	"strings"
)

func lockFulfillmentOrder(tx *gorm.DB, id int64, active bool) (model.ERPOrder, error) {
	var loc model.ERPOrder
	if err := orderScope(tx.Model(&loc)).Select("order_id,shop_id").Where("order_id=?", id).Take(&loc).Error; err != nil {
		return loc, erpReferenceError(err)
	}
	if _, err := lockShopForItem(tx, loc.ShopID, active); err != nil {
		return loc, err
	}
	var o model.ERPOrder
	err := orderScope(tx.Model(&o)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id=?", id).Take(&o).Error
	return o, erpReferenceError(err)
}
func lockOrderPackage(tx *gorm.DB, o model.ERPOrder) (model.ERPPackage, error) {
	var p model.ERPPackage
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id=?", o.OrderID).Take(&p).Error
	if err != nil {
		return p, erpReferenceError(err)
	}
	if o.WarehouseID == nil || p.WarehouseID != *o.WarehouseID {
		return p, errs.New("订单/包裹仓库关联异常")
	}
	return p, nil
}
func checkFulfillmentVersion(actual, expected int64) error {
	if expected < 1 || actual != expected || actual == math.MaxInt64 {
		return errs.New("版本已变化或到达上限，请刷新后重试")
	}
	return nil
}
func replayFulfillment(tx *gorm.DB, o model.ERPOrder, action, key, hash string) (*model.ERPFulfillmentResult, error) {
	var e model.ERPOrderEvent
	err := tx.Where("order_id=? AND action=? AND request_key=?", o.OrderID, action, key).Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if e.RequestHash != hash {
		return nil, errs.New("同一动作请求标识不能用于不同内容")
	}
	var r model.ERPFulfillmentResult
	if err = json.Unmarshal([]byte(e.Remark), &r); err != nil || r.OrderID != o.OrderID {
		return nil, errs.New("动作重放记录异常")
	}
	r.Existing = true
	return &r, nil
}
func finishFulfillment(tx *gorm.DB, old model.ERPOrder, o *model.ERPOrder, p *model.ERPPackage, action, key, hash, operator string, packingFingerprint ...string) (*model.ERPFulfillmentResult, error) {
	if old.Version == math.MaxInt64 {
		return nil, errs.New("订单版本已到上限")
	}
	o.Version = old.Version + 1
	updates := map[string]any{"status": o.Status, "version": o.Version, "warehouse_id": o.WarehouseID, "stock_document_id": o.StockDocumentID, "reviewed_at": o.ReviewedAt, "shipped_at": o.ShippedAt, "cancelled_at": o.CancelledAt, "cancel_reason": o.CancelReason, "update_by": operator, "update_time": types.Now()}
	r := tx.Model(&model.ERPOrder{}).Where("order_id=? AND version=?", o.OrderID, old.Version).Updates(updates)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, errs.New("订单已变化，请刷新")
	}
	result := &model.ERPFulfillmentResult{OrderID: o.OrderID, Version: o.Version, Status: o.Status}
	if p != nil {
		result.PackageID = &p.PackageID
		result.PackageVersion = &p.Version
	}
	payload, err := json.Marshal(result)
	if len(packingFingerprint) > 0 {
		payload, err = json.Marshal(packingProof{ERPFulfillmentResult: *result, Fingerprint: packingFingerprint[0]})
		if len(payload) > 500 {
			return nil, errs.New("复核记录超过存储上限")
		}
	}
	if err != nil {
		return nil, err
	}
	event := model.ERPOrderEvent{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: string(payload)}, OrderID: o.OrderID, ActorUserID: erpctx.UserID(tx.Statement.Context), Action: action, RequestKey: key, RequestHash: hash, FromStatus: old.Status, ToStatus: o.Status, FromVersion: old.Version, ToVersion: o.Version}
	if err = tx.Create(&event).Error; err != nil {
		return nil, err
	}
	audit, _ := json.Marshal(map[string]any{"orderId": o.OrderID, "action": action, "fromVersion": old.Version, "version": o.Version, "packageId": result.PackageID, "packageVersion": result.PackageVersion})
	if err = tx.Create(&model.SysOperLog{Title: "ERP订单履约", BusinessType: model.BusinessTypeUpdate, Method: "erp.order." + action, RequestMethod: "PUT", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error; err != nil {
		return nil, err
	}
	return result, nil
}
func orderLinesForReview(tx *gorm.DB, o *model.ERPOrder) ([]model.ERPLineInput, error) {
	if err := tx.Where("order_id=? AND shop_id=?", o.OrderID, o.ShopID).Order("line_id").Limit(101).Find(&o.Lines).Error; err != nil {
		return nil, err
	}
	if len(o.Lines) < 1 || len(o.Lines) > 100 {
		return nil, errs.New("订单明细须为1—100行")
	}
	codes := []string{}
	for _, l := range o.Lines {
		codes = append(codes, l.ShopItemCode)
	}
	var items []model.ErpShopItem
	if err := tx.Where("shop_id=? AND shop_item_code IN ?", o.ShopID, codes).Find(&items).Error; err != nil {
		return nil, err
	}
	byCode := map[string]model.ErpShopItem{}
	itemIDs := []int64{}
	for _, i := range items {
		byCode[i.ShopItemCode] = i
		itemIDs = append(itemIDs, i.ItemID)
	}
	var mappings []model.ERPShopSkuMapping
	if len(itemIDs) > 0 {
		if err := tx.Where("shop_id=? AND shop_item_id IN ?", o.ShopID, itemIDs).Find(&mappings).Error; err != nil {
			return nil, err
		}
	}
	byItem := map[int64]model.ERPShopSkuMapping{}
	for _, m := range mappings {
		byItem[m.ShopItemID] = m
	}
	totals := map[int64]int64{}
	for i := range o.Lines {
		line := &o.Lines[i]
		item, ok := byCode[line.ShopItemCode]
		if !ok || item.Status != "0" {
			return nil, errs.New("订单有未建档或已停用的店铺商品，请先维护商品")
		}
		m, ok := byItem[item.ItemID]
		if !ok || m.Status != "0" || m.SkuID < 1 || m.Version < 1 {
			return nil, errs.New("订单有未配对商品，请先完成SKU配对")
		}
		if line.Quantity < 1 || line.Quantity > 1000000000 || totals[m.SkuID] > 1000000000-line.Quantity {
			return nil, errs.New("同一内部SKU汇总数量不得超过十亿，且须为正整数")
		}
		totals[m.SkuID] += line.Quantity
		line.ShopItemID = &item.ItemID
		line.MappingID = &m.MappingID
		line.MappingVersion = &m.Version
		line.SkuID = &m.SkuID
	}
	inputs := make([]model.ERPLineInput, 0, len(totals))
	for id, qty := range totals {
		inputs = append(inputs, model.ERPLineInput{SkuID: id, Quantity: qty})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].SkuID < inputs[j].SkuID })
	return inputs, nil
}
func ReviewERPOrder(ctx context.Context, id int64, in model.ERPReviewInput, hash, operator string) (*model.ERPFulfillmentResult, error) {
	var out *model.ERPFulfillmentResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		o, err := lockFulfillmentOrder(tx, id, true)
		if err != nil {
			return err
		}
		if err = requireERPWarehouse(tx, in.WarehouseID); err != nil {
			return err
		}
		if o.WarehouseID != nil {
			if err = requireERPWarehouse(tx, *o.WarehouseID); err != nil {
				return err
			}
		}
		replay, err := replayFulfillment(tx, o, "REVIEW", in.RequestKey, hash)
		if err != nil {
			return err
		}
		if replay != nil {
			out = replay
			return nil
		}
		if err = checkFulfillmentVersion(o.Version, in.Version); err != nil {
			return err
		}
		if o.Status != "PENDING_REVIEW" {
			return errs.New("仅待审核订单可以审核")
		}
		if strings.TrimSpace(o.ReceiverName) == "" || strings.TrimSpace(o.ReceiverAddress) == "" || len(o.ReceiverCountry) != 2 {
			return errs.New("请先完善订单收件信息")
		}
		old := o
		items, err := orderLinesForReview(tx, &o)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(items))
		for _, l := range items {
			ids = append(ids, l.SkuID)
		}
		// Item/mapping writes also own this shop. Read them in batches under that
		// parent lock, then warehouse -> sorted SKU, never item/SKU -> warehouse.
		wh, skus, err := lockERPReferences(tx, in.WarehouseID, ids, true)
		if err != nil {
			return err
		}
		doc := model.ERPDocument{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, DocumentNo: "SALE-" + uuid.NewString(), Kind: "SALE", WarehouseID: wh.WarehouseId, WarehouseName: wh.WarehouseName, Status: "DRAFT", Version: 1, RequestKey: "order_" + uuid.NewString(), RequestHash: hash}
		if err = tx.Omit("Lines").Create(&doc).Error; err != nil {
			return err
		}
		doc.Lines, err = buildERPLines(tx, &doc, items, skus)
		if err != nil {
			return err
		}
		if err = tx.Create(&doc.Lines).Error; err != nil {
			return err
		}
		if err = transitionERPDocumentLocked(tx, &doc, "confirm", operator); err != nil {
			return err
		}
		for i := range o.Lines {
			l := &o.Lines[i]
			sku := skus[*l.SkuID]
			l.SkuCode = &sku.SkuCode
			l.ProductName = &sku.ProductName
			l.Specification = &sku.Specification
			l.Unit = &sku.Unit
			l.UpdateBy = operator
			l.UpdateTime = types.Now()
		}
		if err = tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"shop_item_id", "mapping_id", "mapping_version", "sku_id", "sku_code", "product_name", "specification", "unit", "update_by", "update_time"})}).Create(&o.Lines).Error; err != nil {
			return err
		}
		o.WarehouseID = &in.WarehouseID
		o.StockDocumentID = &doc.DocumentID
		o.Status = "READY_TO_SHIP"
		o.ReviewedAt = types.Now()
		// Establish the composite FK parent before inserting its new package. New
		// rows are private to this tx; existing-package paths lock package before doc.
		if err = tx.Model(&model.ERPOrder{}).Where("order_id=?", id).Updates(map[string]any{"warehouse_id": in.WarehouseID, "stock_document_id": doc.DocumentID}).Error; err != nil {
			return err
		}
		p := model.ERPPackage{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, OrderID: id, WarehouseID: in.WarehouseID, PackageNo: "P" + uuid.NewString(), Status: "READY_TO_SHIP", Version: 1}
		if err = tx.Omit("Lines", "ShopID").Create(&p).Error; err != nil {
			return err
		}
		lines := make([]model.ERPPackageLine, 0, len(o.Lines))
		for _, l := range o.Lines {
			lines = append(lines, model.ERPPackageLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, OrderID: id, PackageID: p.PackageID, OrderLineID: l.LineID, SkuID: *l.SkuID, Quantity: l.Quantity, SkuCode: *l.SkuCode, ProductName: *l.ProductName, Unit: *l.Unit})
		}
		if err = tx.Create(&lines).Error; err != nil {
			return err
		}
		out, err = finishFulfillment(tx, old, &o, &p, "REVIEW", in.RequestKey, hash, operator)
		return err
	}))
	return out, err
}
func CancelERPOrder(ctx context.Context, id int64, in model.ERPCancelInput, hash, operator string) (*model.ERPFulfillmentResult, error) {
	var out *model.ERPFulfillmentResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		o, err := lockFulfillmentOrder(tx, id, false)
		if err != nil {
			return err
		}
		if o.WarehouseID != nil {
			if err = requireERPWarehouse(tx, *o.WarehouseID); err != nil {
				return err
			}
		}
		replay, err := replayFulfillment(tx, o, "CANCEL", in.RequestKey, hash)
		if err != nil {
			return err
		}
		if replay != nil {
			out = replay
			return nil
		}
		if err = checkFulfillmentVersion(o.Version, in.Version); err != nil {
			return err
		}
		if o.Status != "PENDING_REVIEW" && o.Status != "READY_TO_SHIP" {
			return errs.New("仅未发货、未取消订单可以取消")
		}
		old := o
		var p *model.ERPPackage
		if o.Status == "READY_TO_SHIP" {
			pack, e := lockOrderPackage(tx, o)
			if e != nil {
				return e
			}
			if pack.Status != "READY_TO_SHIP" || pack.Version == math.MaxInt64 {
				return errs.New("包裹状态或版本异常")
			}
			doc, e := lockOrderDocument(tx, o)
			if e != nil {
				return e
			}
			if e = transitionERPDocumentLocked(tx, &doc, "cancel", operator); e != nil {
				return e
			}
			if e = updateFulfillmentPackage(tx, &pack, "CANCELLED", operator, nil); e != nil {
				return e
			}
			p = &pack
		} else if o.StockDocumentID != nil || o.WarehouseID != nil {
			return errs.New("待审核订单存在异常库存关联")
		}
		o.Status = "CANCELLED"
		o.CancelledAt = types.Now()
		o.CancelReason = in.Reason
		out, err = finishFulfillment(tx, old, &o, p, "CANCEL", in.RequestKey, hash, operator)
		return err
	}))
	return out, err
}
func lockOrderDocument(tx *gorm.DB, o model.ERPOrder) (model.ERPDocument, error) {
	var d model.ERPDocument
	if o.StockDocumentID == nil || o.WarehouseID == nil {
		return d, errs.New("订单库存关联缺失")
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=?", *o.StockDocumentID).Take(&d).Error
	if err != nil {
		return d, erpReferenceError(err)
	}
	if d.Kind != "SALE" || d.WarehouseID != *o.WarehouseID || d.Status != "RESERVED" || d.SourceDocumentID != nil {
		return d, errs.New("订单库存凭据状态异常")
	}
	return d, nil
}
func updateFulfillmentPackage(tx *gorm.DB, p *model.ERPPackage, status, operator string, logistics *model.ERPLogisticsInput) error {
	if p.Version == math.MaxInt64 {
		return errs.New("包裹版本已到上限")
	}
	u := map[string]any{"status": status, "version": p.Version + 1, "update_by": operator, "update_time": types.Now()}
	if logistics != nil {
		u["carrier_code"] = logistics.CarrierCode
		u["tracking_no"] = logistics.TrackingNo
	}
	if status == "SHIPPED" {
		u["shipped_at"] = types.Now()
	}
	r := tx.Model(&model.ERPPackage{}).Where("package_id=? AND version=?", p.PackageID, p.Version).Updates(u)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errs.New("包裹已变化，请刷新")
	}
	p.Version++
	p.Status = status
	return nil
}
func ActERPPackage(ctx context.Context, id int64, in model.ERPLogisticsInput, ship bool, hash, operator string) (*model.ERPFulfillmentResult, error) {
	return actERPPackage(ctx, id, in, ship, hash, operator, 0)
}

func actERPPackage(ctx context.Context, id int64, in model.ERPLogisticsInput, ship bool, hash, operator string, shippingMethodID int64) (*model.ERPFulfillmentResult, error) {
	return actERPPackageMode(ctx, id, in, ship, hash, operator, shippingMethodID, false)
}

func actERPPackageMode(ctx context.Context, id int64, in model.ERPLogisticsInput, ship bool, hash, operator string, shippingMethodID int64, checkedShip bool) (*model.ERPFulfillmentResult, error) {
	var out *model.ERPFulfillmentResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		var loc model.ERPPackage
		if err := packageScope(tx.Model(&loc)).Select("erp_package.order_id").Where("erp_package.package_id=?", id).Take(&loc).Error; err != nil {
			return erpReferenceError(err)
		}
		o, err := lockFulfillmentOrder(tx, loc.OrderID, true)
		if err != nil {
			return err
		}
		p, err := lockOrderPackage(tx, o)
		if err != nil {
			return err
		}
		if p.PackageID != id {
			return errs.New("订单包裹关联异常")
		}
		if err = requireERPWarehouse(tx, p.WarehouseID); err != nil {
			return err
		}
		action := "LOGISTICS"
		if ship {
			action = "SHIP"
			if checkedShip {
				action = "CHECKED_SHIP"
			}
		}
		var replay *model.ERPFulfillmentResult
		if checkedShip {
			replay, err = replayPicking(tx, o, action, in.RequestKey, hash)
		} else {
			replay, err = replayFulfillment(tx, o, action, in.RequestKey, hash)
		}
		if err != nil {
			return err
		}
		if replay != nil {
			out = replay
			return nil
		}
		if err = checkFulfillmentVersion(p.Version, in.Version); err != nil {
			return err
		}
		if o.Status != "READY_TO_SHIP" || p.Status != "READY_TO_SHIP" {
			return errs.New("仅待发货订单/包裹允许此操作")
		}
		old := o
		if ship {
			if p.CarrierCode == "" || p.TrackingNo == "" {
				return errs.New("请先保存物流方式和运单号")
			}
			doc, e := lockOrderDocument(tx, o)
			if e != nil {
				return e
			}
			if checkedShip {
				if e = requirePackingCheck(tx, p); e != nil {
					return e
				}
			}
			if e = transitionERPDocumentLocked(tx, &doc, "ship", operator); e != nil {
				return e
			}
			o.Status = "SHIPPED"
			o.ShippedAt = types.Now()
			if e = updateFulfillmentPackage(tx, &p, "SHIPPED", operator, nil); e != nil {
				return e
			}
		} else {
			if shippingMethodID > 0 {
				code, e := savePackageShipping(tx, p, shippingMethodID, operator)
				if e != nil {
					return e
				}
				in.CarrierCode = code
			} else if e := clearPackageShipping(tx, p.PackageID); e != nil {
				return e
			}
			if err = updateFulfillmentPackage(tx, &p, "READY_TO_SHIP", operator, &in); err != nil {
				return err
			}
		}
		out, err = finishFulfillment(tx, old, &o, &p, action, in.RequestKey, hash, operator)
		return err
	}))
	return out, err
}
func packageScope(tx *gorm.DB) *gorm.DB {
	return erpShopColumnScope(erpWarehouseScope(tx.Joins("JOIN erp_order o ON o.order_id=erp_package.order_id"), "erp_package.warehouse_id"), "o.shop_id")
}
func SelectERPPackages(ctx context.Context, q model.ERPPackageQuery, pg page.Query) ([]model.ERPPackageSummary, int64, error) {
	tx := packageScope(DB(ctx).Model(&model.ERPPackage{}))
	if q.ShopID > 0 {
		tx = tx.Where("o.shop_id=?", q.ShopID)
	}
	if q.OrderID > 0 {
		tx = tx.Where("erp_package.order_id=?", q.OrderID)
	}
	if q.WarehouseID > 0 {
		tx = tx.Where("erp_package.warehouse_id=?", q.WarehouseID)
	}
	if q.Status != "" {
		tx = tx.Where("erp_package.status=?", q.Status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPPackageSummary{}
	err := tx.Select("erp_package.package_id,erp_package.package_no,erp_package.order_id,erp_package.warehouse_id,erp_package.status,erp_package.version,erp_package.shipped_at,o.shop_id").Order("erp_package.package_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, total, err
}
func SelectERPPackage(ctx context.Context, id int64) (*model.ERPPackage, error) {
	var p model.ERPPackage
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := packageScope(tx.Model(&model.ERPPackage{})).Select("erp_package.*,o.shop_id").Where("erp_package.package_id=?", id).Take(&p).Error; err != nil {
			return erpReferenceError(err)
		}
		p.Lines = []model.ERPPackageLine{}
		return tx.Where("package_id=? AND order_id=?", id, p.OrderID).Order("line_id").Limit(101).Find(&p.Lines).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &p, err
}
