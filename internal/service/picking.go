package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
)

func ListPickingPackages(ctx context.Context, q model.ERPPackageQuery, pg page.Query) ([]model.ERPPackageSummary, int64, error) {
	if q.Status == "" {
		q.Status = "READY_TO_SHIP"
	}
	return ListERPPackages(ctx, q, pg)
}
func GetPickingSheet(ctx context.Context, id int64) (*model.PickingSheet, error) {
	if id < 1 {
		return nil, errs.New("包裹编号不正确")
	}
	return repository.SelectPickingSheet(ctx, id)
}
func validatePickingInput(in *model.PickingCheckInput) error {
	if e := validERPAction(in.ERPActionToken); e != nil {
		return e
	}
	if len(in.Scans) < 1 || len(in.Scans) > 100 {
		return errs.New("复核须包含1—100个扫描编码")
	}
	seen := map[string]bool{}
	for _, v := range in.Scans {
		if !orderText(v.ScanCode, 128, true) || v.Quantity < 1 || v.Quantity > 1000000000 || seen[v.ScanCode] {
			return errs.New("扫描编码或数量不合法，不能重复或丢失前导零")
		}
		seen[v.ScanCode] = true
	}
	sort.Slice(in.Scans, func(i, j int) bool { return in.Scans[i].ScanCode < in.Scans[j].ScanCode })
	return nil
}
func ConfirmPacking(ctx context.Context, id int64, in model.PickingCheckInput, operator string) (*model.ERPFulfillmentResult, error) {
	if id < 1 {
		return nil, errs.New("包裹编号不正确")
	}
	if e := validatePickingInput(&in); e != nil {
		return nil, e
	}
	return repository.ConfirmPacking(ctx, id, in, orderHash([]any{"PACK_CHECK", id, in}), operator)
}
func ShipCheckedPackage(ctx context.Context, id int64, in model.ERPActionToken, operator string) (*model.ERPFulfillmentResult, error) {
	if id < 1 {
		return nil, errs.New("包裹编号不正确")
	}
	if e := validERPAction(in); e != nil {
		return nil, e
	}
	return repository.ShipCheckedPackage(ctx, id, in, orderHash([]any{"CHECKED_SHIP", id, in}), operator)
}
