package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
	"time"
)

func validateLogisticsWrite(id int64, in *model.LogisticsWrite, method bool) error {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Remark = strings.TrimSpace(in.Remark)
	if id < 0 || !orderRequestKeyPattern.MatchString(in.RequestKey) || id == 0 && in.Version != 0 || id > 0 && in.Version < 1 || !carrierPattern.MatchString(in.Code) || !orderText(in.Name, 120, true) || !orderText(in.Remark, 500, false) || (in.Status != "0" && in.Status != "1") || method && in.CarrierID < 1 || !method && in.CarrierID != 0 {
		return errs.New("物流编码、名称、状态、所属公司或版本不合法")
	}
	return nil
}
func WriteLogisticsMaster(ctx context.Context, id int64, in model.LogisticsWrite, method bool, operator string) (*model.LogisticsResult, error) {
	if e := validateLogisticsWrite(id, &in, method); e != nil {
		return nil, e
	}
	return repository.WriteLogisticsMaster(ctx, id, in, method, orderHash([]any{"LOGISTICS_MASTER", method, id, in}), operator)
}
func validateLogisticsQuery(in model.LogisticsQuery) error {
	if in.Code != "" && !carrierPattern.MatchString(in.Code) || !orderText(in.Name, 120, false) || (in.Status != "" && in.Status != "0" && in.Status != "1") || in.CarrierID < 0 {
		return errs.New("物流查询条件不合法")
	}
	return nil
}
func ListLogisticsMasters(ctx context.Context, in model.LogisticsQuery, pg page.Query, method bool) ([]model.LogisticsMasterView, int64, error) {
	if e := validateLogisticsQuery(in); e != nil {
		return nil, 0, e
	}
	return repository.SelectLogisticsMasters(ctx, in, pg, method)
}
func GetLogisticsMaster(ctx context.Context, id int64, method bool) (*model.LogisticsMasterView, error) {
	if id < 1 {
		return nil, errs.New("物流档案编号不正确")
	}
	return repository.SelectLogisticsMaster(ctx, id, method)
}
func ListLogisticsOptions(ctx context.Context, in model.LogisticsQuery, pg page.Query) ([]model.LogisticsOption, int64, error) {
	if e := validateLogisticsQuery(in); e != nil {
		return nil, 0, e
	}
	return repository.SelectLogisticsOptions(ctx, in, pg)
}
func SavePackageShipping(ctx context.Context, id int64, in model.PackageShippingInput, operator string) (*model.ERPFulfillmentResult, error) {
	if e := validERPAction(in.ERPActionToken); e != nil {
		return nil, e
	}
	if id < 1 || in.ShippingMethodID < 1 || !orderText(in.TrackingNo, 128, true) {
		return nil, errs.New("包裹、配送方式或运单号不合法")
	}
	return repository.SaveERPPackageShipping(ctx, id, in, orderHash([]any{"CATALOG_LOGISTICS", id, in}), operator)
}
func GetPackageShipping(ctx context.Context, id int64) (*model.PackageShippingView, error) {
	if id < 1 {
		return nil, errs.New("包裹编号不正确")
	}
	return repository.SelectPackageShippingView(ctx, id)
}
func CheckLogisticsTracking(ctx context.Context, in model.TrackingCheckInput) (*model.TrackingCheck, error) {
	if in.PackageID < 1 || in.ShippingMethodID < 1 || !orderText(in.TrackingNo, 128, true) {
		return nil, errs.New("包裹、配送方式或运单号不合法")
	}
	return repository.CheckLogisticsTracking(ctx, in)
}
func validateShipmentQuery(in model.ShipmentQuery) error {
	if in.ShopID < 0 || in.WarehouseID < 0 || in.ShippingMethodID < 0 || in.CarrierCode != "" && !carrierPattern.MatchString(in.CarrierCode) || !orderText(in.TrackingNo, 128, false) || (in.Mode != "" && in.Mode != "MANUAL" && in.Mode != "CATALOG") {
		return errs.New("发货查询条件不合法")
	}
	var from, to time.Time
	var e error
	if in.ShippedFrom != "" {
		from, e = time.ParseInLocation("2006-01-02 15:04:05", in.ShippedFrom, types.Location)
		if e != nil {
			return errs.New("发货起始时间不合法")
		}
	}
	if in.ShippedTo != "" {
		to, e = time.ParseInLocation("2006-01-02 15:04:05", in.ShippedTo, types.Location)
		if e != nil {
			return errs.New("发货结束时间不合法")
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return errs.New("发货时间范围不正确")
	}
	return nil
}
func ListLogisticsShipments(ctx context.Context, in model.ShipmentQuery, pg page.Query) ([]model.ShipmentView, int64, error) {
	if e := validateShipmentQuery(in); e != nil {
		return nil, 0, e
	}
	return repository.SelectLogisticsShipments(ctx, in, pg)
}
func ExportLogisticsShipments(ctx context.Context, in model.ShipmentQuery) ([]model.ShipmentView, error) {
	if e := validateShipmentQuery(in); e != nil {
		return nil, e
	}
	rows, e := repository.SelectLogisticsShipmentExport(ctx, in, MaxExportRows+1)
	if e != nil {
		return nil, e
	}
	return rows, checkExportSize(len(rows))
}
