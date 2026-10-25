package repository

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// Called only with the actor/order/package locks already held. Carrier before
// method matches master writers; they never acquire an order/package lock.
func savePackageShipping(tx *gorm.DB, p model.ERPPackage, methodID int64, operator string) (string, error) {
	if e := requireLogistics(tx); e != nil {
		return "", e
	}
	var old model.PackageShipping
	oldErr := tx.Where("package_id=?", p.PackageID).Take(&old).Error
	if oldErr != nil && !errors.Is(oldErr, gorm.ErrRecordNotFound) {
		return "", oldErr
	}
	// Editing only the tracking text retains the saved method/name snapshot.
	// Disabling a master prevents NEW selections, not maintenance of an already
	// selected method. Switching away and back still requires an enabled master.
	if oldErr == nil && old.ShippingMethodID == methodID {
		if old.OrderID != p.OrderID || old.CarrierCode != p.CarrierCode {
			return "", errs.New("包裹物流关联异常，请核对")
		}
		return old.CarrierCode, nil
	}
	var hint model.ShippingMethod
	if e := tx.Where("shipping_method_id=?", methodID).Take(&hint).Error; e != nil {
		return "", erpReferenceError(e)
	}
	c, e := lockLogisticsCarrier(tx, hint.CarrierID, true)
	if e != nil {
		return "", e
	}
	var m model.ShippingMethod
	if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shipping_method_id=? AND carrier_id=?", methodID, c.CarrierID).Take(&m).Error; e != nil {
		return "", erpReferenceError(e)
	}
	if m.Status != "0" {
		return "", errs.New("配送方式已停用，不能新选")
	}
	if errors.Is(oldErr, gorm.ErrRecordNotFound) {
		e = tx.Create(&model.PackageShipping{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, OrderID: p.OrderID, PackageID: p.PackageID, CarrierID: c.CarrierID, ShippingMethodID: m.ShippingMethodID, CarrierCode: c.CarrierCode, CarrierName: c.CarrierName, MethodCode: m.MethodCode, MethodName: m.MethodName}).Error
	} else {
		e = tx.Model(&old).Updates(map[string]any{"carrier_id": c.CarrierID, "shipping_method_id": m.ShippingMethodID, "carrier_code": c.CarrierCode, "carrier_name": c.CarrierName, "method_code": m.MethodCode, "method_name": m.MethodName, "update_by": operator, "update_time": types.Now()}).Error
	}
	return c.CarrierCode, e
}
func clearPackageShipping(tx *gorm.DB, packageID int64) error {
	present, e := logisticsPresent(tx)
	if e != nil || !present {
		return e
	}
	return tx.Where("package_id=?", packageID).Delete(&model.PackageShipping{}).Error
}
func SaveERPPackageShipping(ctx context.Context, id int64, in model.PackageShippingInput, hash, operator string) (*model.ERPFulfillmentResult, error) {
	return actERPPackage(ctx, id, model.ERPLogisticsInput{ERPActionToken: in.ERPActionToken, TrackingNo: in.TrackingNo}, false, hash, operator, in.ShippingMethodID)
}
func SelectPackageShipping(ctx context.Context, id int64) (*model.PackageShipping, error) {
	v, e := SelectPackageShippingView(ctx, id)
	if e != nil {
		return nil, e
	}
	return v.Snapshot, nil
}

func SelectPackageShippingView(ctx context.Context, id int64) (*model.PackageShippingView, error) {
	out := &model.PackageShippingView{PackageID: id}
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ERPPackage
		if e := packageScope(tx.Model(&p)).Select("erp_package.package_id,erp_package.version").Where("erp_package.package_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		present, e := logisticsPresent(tx)
		out.PackageVersion = p.Version
		out.Available = present
		if e != nil || !present {
			return e
		}
		var s model.PackageShipping
		e = tx.Where("package_id=?", id).Take(&s).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		out.Snapshot = &s
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return out, e
}
func CheckLogisticsTracking(ctx context.Context, in model.TrackingCheckInput) (*model.TrackingCheck, error) {
	q := DB(ctx)
	if e := requireLogistics(q); e != nil {
		return nil, e
	}
	var p model.ERPPackage
	if e := packageScope(q.Model(&p)).Select("erp_package.package_id").Where("erp_package.package_id=?", in.PackageID).Take(&p).Error; e != nil {
		return nil, erpReferenceError(e)
	}
	var selected struct{ CarrierCode string }
	var saved model.PackageShipping
	err := q.Where("package_id=? AND shipping_method_id=?", in.PackageID, in.ShippingMethodID).Take(&saved).Error
	if err == nil {
		selected.CarrierCode = saved.CarrierCode
	} else {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if e := q.Model(&model.ShippingMethod{}).Joins("JOIN erp_carrier c ON c.carrier_id=erp_shipping_method.carrier_id").Where("erp_shipping_method.shipping_method_id=? AND erp_shipping_method.status='0' AND c.status='0'", in.ShippingMethodID).Select("c.carrier_code").Take(&selected).Error; e != nil {
			return nil, erpReferenceError(e)
		}
	}
	var n int64
	e := packageScope(q.Model(&model.ERPPackage{})).Where("erp_package.package_id<>? AND erp_package.carrier_code=? AND erp_package.tracking_no=?", in.PackageID, selected.CarrierCode, in.TrackingNo).Count(&n).Error
	return &model.TrackingCheck{Duplicate: n > 0}, e
}
func shipmentQuery(tx *gorm.DB, in model.ShipmentQuery) *gorm.DB {
	q := packageScope(tx.Model(&model.ERPPackage{})).Joins("LEFT JOIN erp_package_shipping ps ON ps.package_id=erp_package.package_id").Where("erp_package.status='SHIPPED'")
	if in.ShopID > 0 {
		q = q.Where("o.shop_id=?", in.ShopID)
	}
	if in.WarehouseID > 0 {
		q = q.Where("erp_package.warehouse_id=?", in.WarehouseID)
	}
	if in.ShippingMethodID > 0 {
		q = q.Where("ps.shipping_method_id=?", in.ShippingMethodID)
	}
	if in.CarrierCode != "" {
		q = q.Where("erp_package.carrier_code=?", in.CarrierCode)
	}
	if in.TrackingNo != "" {
		q = q.Where("erp_package.tracking_no=?", in.TrackingNo)
	}
	if in.Mode == "CATALOG" {
		q = q.Where("ps.shipping_id IS NOT NULL")
	}
	if in.Mode == "MANUAL" {
		q = q.Where("ps.shipping_id IS NULL")
	}
	if in.ShippedFrom != "" {
		q = q.Where("erp_package.shipped_at>=?", in.ShippedFrom)
	}
	if in.ShippedTo != "" {
		q = q.Where("erp_package.shipped_at<=?", in.ShippedTo)
	}
	return q
}

const shipmentColumns = "erp_package.package_id,erp_package.package_no,erp_package.order_id,o.shop_id,erp_package.warehouse_id,erp_package.carrier_code,erp_package.tracking_no,erp_package.shipped_at,ps.shipping_method_id,COALESCE(ps.carrier_name,'') AS carrier_name,COALESCE(ps.method_code,'') AS method_code,COALESCE(ps.method_name,'') AS method_name,CASE WHEN ps.shipping_id IS NULL THEN 'MANUAL' ELSE 'CATALOG' END AS mode"

func SelectLogisticsShipments(ctx context.Context, in model.ShipmentQuery, pg page.Query) ([]model.ShipmentView, int64, error) {
	base := DB(ctx)
	if e := requireLogistics(base); e != nil {
		return nil, 0, e
	}
	q := shipmentQuery(base, in)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ShipmentView{}
	e := q.Select(shipmentColumns).Order("erp_package.package_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, n, e
}
func SelectLogisticsShipmentExport(ctx context.Context, in model.ShipmentQuery, limit int) ([]model.ShipmentView, error) {
	base := DB(ctx)
	if e := requireLogistics(base); e != nil {
		return nil, e
	}
	rows := []model.ShipmentView{}
	e := shipmentQuery(base, in).Select(shipmentColumns).Order("erp_package.package_id DESC").Limit(limit).Scan(&rows).Error
	return rows, e
}
