package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"sort"
)

type packingProof struct {
	model.ERPFulfillmentResult
	Fingerprint string `json:"packingFingerprint"`
}

// Immutable package-line snapshots are aggregated by SKU, never mixed by unit.
// Current scan codes are read in one batch. Writes lock warehouse -> sorted
// SKU -> profiles, consistent with stock and catalogue writers.
func pickingItems(tx *gorm.DB, p model.ERPPackage, lock bool) ([]model.PickingItem, string, error) {
	lines := []model.ERPPackageLine{}
	if e := tx.Where("package_id=? AND order_id=?", p.PackageID, p.OrderID).Order("line_id").Limit(101).Find(&lines).Error; e != nil {
		return nil, "", e
	}
	if len(lines) == 0 || len(lines) > 100 {
		return nil, "", errs.New("包裹明细异常")
	}
	bySKU := map[int64]model.PickingItem{}
	ids := []int64{}
	for _, l := range lines {
		if l.SkuID < 1 || l.Quantity < 1 || l.Quantity > 1000000000 {
			return nil, "", errs.New("包裹数量异常")
		}
		v, ok := bySKU[l.SkuID]
		if !ok {
			ids = append(ids, l.SkuID)
			v = model.PickingItem{SkuID: l.SkuID, SkuCode: l.SkuCode, ProductName: l.ProductName, Unit: l.Unit}
		}
		if v.SkuCode != l.SkuCode || v.Unit != l.Unit || v.Quantity > 1000000000-l.Quantity {
			return nil, "", errs.New("包裹SKU快照或汇总数量异常")
		}
		v.Quantity += l.Quantity
		bySKU[l.SkuID] = v
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	skus := map[int64]model.ErpSku{}
	if lock {
		_, v, e := lockERPReferences(tx, p.WarehouseID, ids, true)
		if e != nil {
			return nil, "", e
		}
		skus = v
	} else {
		var rows []model.ErpSku
		if e := tx.Where("sku_id IN ?", ids).Find(&rows).Error; e != nil {
			return nil, "", e
		}
		for _, v := range rows {
			skus[v.SkuId] = v
		}
		if len(skus) != len(ids) {
			return nil, "", errs.New("包裹关联商品缺失")
		}
	}
	var n int64
	if e := tx.WithContext(tenantctx.System(tx.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name IN ?", catalogBusinessTables).Count(&n).Error; e != nil {
		return nil, "", e
	}
	if n != 0 && n != int64(len(catalogBusinessTables)) {
		return nil, "", errs.New("商品档案结构不完整，请检查安装")
	}
	profiles := map[int64]model.CatalogProfile{}
	if n > 0 {
		var rows []model.CatalogProfile
		q := tx.Where("sku_id IN ?", ids).Order("sku_id")
		if lock {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Find(&rows).Error; e != nil {
			return nil, "", e
		}
		for _, v := range rows {
			profiles[v.SkuID] = v
		}
	}
	out := []model.PickingItem{}
	codes := map[string]bool{}
	blocked := ""
	for _, id := range ids {
		v := bySKU[id]
		sku := skus[id]
		v.Specification = sku.Specification
		v.ScanCode = v.SkuCode
		if profile, ok := profiles[id]; ok && profile.Barcode != nil && *profile.Barcode != "" {
			v.ScanCode = *profile.Barcode
		}
		if sku.Status != "0" {
			blocked = "商品已停用，请联系管理员核对"
		}
		if codes[v.ScanCode] {
			blocked = "包裹内扫描编码重复，请先修正商品条码"
		}
		codes[v.ScanCode] = true
		out = append(out, v)
	}
	return out, blocked, nil
}
func pickingFingerprint(p model.ERPPackage, items []model.PickingItem) string {
	raw, _ := json.Marshal(struct {
		PackageID, OrderID, WarehouseID int64
		Items                           []model.PickingItem
	}{p.PackageID, p.OrderID, p.WarehouseID, items})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func packingEvent(tx *gorm.DB, orderID int64, action, key string, lock bool) (*model.ERPOrderEvent, error) {
	q := tx.Where("order_id=? AND action=?", orderID, action)
	if key != "" {
		q = q.Where("request_key=?", key)
	}
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var e model.ERPOrderEvent
	err := q.Order("event_id DESC").Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &e, err
}
func decodePackingProof(e *model.ERPOrderEvent, p model.ERPPackage) (packingProof, error) {
	var proof packingProof
	if json.Unmarshal([]byte(e.Remark), &proof) != nil || proof.OrderID != p.OrderID || proof.PackageID == nil || *proof.PackageID != p.PackageID || proof.PackageVersion == nil || len(proof.Fingerprint) != 64 {
		return proof, errs.New("包裹复核记录异常")
	}
	return proof, nil
}
func replayPicking(tx *gorm.DB, o model.ERPOrder, action, key, hash string) (*model.ERPFulfillmentResult, error) {
	e, err := packingEvent(tx, o.OrderID, action, key, true)
	if err != nil || e == nil {
		return nil, err
	}
	if e.ActorUserID != erpctx.UserID(tx.Statement.Context) || e.RequestHash != hash {
		return nil, errs.New("同一请求标识不能用于不同员工或内容")
	}
	var r model.ERPFulfillmentResult
	if json.Unmarshal([]byte(e.Remark), &r) != nil || r.OrderID != o.OrderID {
		return nil, errs.New("动作重放记录异常")
	}
	r.Existing = true
	return &r, nil
}
func SelectPickingSheet(ctx context.Context, id int64) (*model.PickingSheet, error) {
	out := &model.PickingSheet{}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ERPPackage
		if e := packageScope(tx.Model(&model.ERPPackage{})).Select("erp_package.*").Where("erp_package.package_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		items, blocked, e := pickingItems(tx, p, false)
		if e != nil {
			return e
		}
		var w model.ErpWarehouse
		if e = tx.Where("warehouse_id=?", p.WarehouseID).Take(&w).Error; e != nil {
			return erpReferenceError(e)
		}
		*out = model.PickingSheet{PackageID: p.PackageID, PackageNo: p.PackageNo, OrderID: p.OrderID, WarehouseID: p.WarehouseID, WarehouseName: w.WarehouseName, Version: p.Version, Status: p.Status, HasLogistics: p.CarrierCode != "" && p.TrackingNo != "", Scannable: blocked == "" && w.Status == "0" && p.Status == "READY_TO_SHIP", BlockReason: blocked, Items: items}
		event, e := packingEvent(tx, p.OrderID, "PACK_CHECK", "", false)
		if e != nil {
			return e
		}
		if event != nil {
			proof, e := decodePackingProof(event, p)
			if e != nil {
				return e
			}
			out.CheckedAt = event.CreateTime
			out.Checked = out.Scannable && *proof.PackageVersion == p.Version && proof.Fingerprint == pickingFingerprint(p, items)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
func validatePackingCounts(items []model.PickingItem, scans []model.PickingScan) error {
	if len(scans) != len(items) {
		return errs.New("复核存在漏货或错货，请重新核对")
	}
	expected := map[string]int64{}
	for _, v := range items {
		if _, ok := expected[v.ScanCode]; ok {
			return errs.New("包裹内扫描编码重复")
		}
		expected[v.ScanCode] = v.Quantity
	}
	for _, v := range scans {
		n, ok := expected[v.ScanCode]
		if !ok || v.Quantity != n {
			return errs.New("复核存在错货、多扫、漏扫或重复编码")
		}
		delete(expected, v.ScanCode)
	}
	if len(expected) > 0 {
		return errs.New("复核有未完成商品")
	}
	return nil
}
func ConfirmPacking(ctx context.Context, id int64, in model.PickingCheckInput, hash, operator string) (*model.ERPFulfillmentResult, error) {
	var out *model.ERPFulfillmentResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var loc model.ERPPackage
		if e := packageScope(tx.Model(&loc)).Select("erp_package.order_id").Where("erp_package.package_id=?", id).Take(&loc).Error; e != nil {
			return erpReferenceError(e)
		}
		o, e := lockFulfillmentOrder(tx, loc.OrderID, true)
		if e != nil {
			return e
		}
		p, e := lockOrderPackage(tx, o)
		if e != nil {
			return e
		}
		if p.PackageID != id {
			return errs.New("订单包裹关联异常")
		}
		if e = requireERPWarehouse(tx, p.WarehouseID); e != nil {
			return e
		}
		replay, e := replayPicking(tx, o, "PACK_CHECK", in.RequestKey, hash)
		if e != nil {
			return e
		}
		if replay != nil {
			out = replay
			return nil
		}
		if e = checkFulfillmentVersion(p.Version, in.Version); e != nil {
			return e
		}
		if o.Status != "READY_TO_SHIP" || p.Status != "READY_TO_SHIP" {
			return errs.New("仅待发货包裹可以复核")
		}
		items, blocked, e := pickingItems(tx, p, true)
		if e != nil {
			return e
		}
		if blocked != "" {
			return errs.New(blocked)
		}
		if e = validatePackingCounts(items, in.Scans); e != nil {
			return e
		}
		fingerprint := pickingFingerprint(p, items)
		old := o
		if e = updateFulfillmentPackage(tx, &p, p.Status, operator, nil); e != nil {
			return e
		}
		out, e = finishFulfillment(tx, old, &o, &p, "PACK_CHECK", in.RequestKey, hash, operator, fingerprint)
		return e
	}))
	return out, err
}
func requirePackingCheck(tx *gorm.DB, p model.ERPPackage) error {
	items, blocked, e := pickingItems(tx, p, true)
	if e != nil {
		return e
	}
	if blocked != "" {
		return errs.New(blocked)
	}
	event, e := packingEvent(tx, p.OrderID, "PACK_CHECK", "", true)
	if e != nil {
		return e
	}
	if event == nil {
		return errs.New("请先完成当前包裹扫码复核")
	}
	proof, e := decodePackingProof(event, p)
	if e != nil {
		return e
	}
	if *proof.PackageVersion != p.Version || proof.Fingerprint != pickingFingerprint(p, items) {
		return errs.New("包裹或商品扫描信息已变化，请重新复核")
	}
	return nil
}
func ShipCheckedPackage(ctx context.Context, id int64, in model.ERPActionToken, hash, operator string) (*model.ERPFulfillmentResult, error) {
	return actERPPackageMode(ctx, id, model.ERPLogisticsInput{ERPActionToken: in}, true, hash, operator, 0, true)
}
