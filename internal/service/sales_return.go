package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
	"strings"
)

func normalizeSalesReturn(in *model.ERPSalesReturnInput) error {
	if e := validERPAction(in.ERPActionToken); e != nil {
		return e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !orderText(in.Reason, 500, true) {
		return errs.New("退货原因必填、最多500字且不得含控制字符")
	}
	if len(in.Lines) < 1 || len(in.Lines) > 100 {
		return errs.New("本次退货须为1—100行")
	}
	in.Lines = append([]model.ERPSalesReturnLineInput(nil), in.Lines...)
	seen := map[int64]bool{}
	for _, l := range in.Lines {
		if l.PackageLineID < 1 || l.Quantity < 1 || l.Quantity > 1000000000 || seen[l.PackageLineID] {
			return errs.New("原包裹行须唯一，退货数量须为1—十亿整数")
		}
		seen[l.PackageLineID] = true
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].PackageLineID < in.Lines[j].PackageLineID })
	return nil
}
func ReturnERPSalesPackage(ctx context.Context, id int64, in model.ERPSalesReturnInput, operator string) (*model.ERPSalesReturnResult, error) {
	if id < 1 {
		return nil, errs.New("包裹编号不合法")
	}
	if e := normalizeSalesReturn(&in); e != nil {
		return nil, e
	}
	return repository.ReturnERPSalesPackage(ctx, id, in, purchaseHash(ctx, "SALES_RETURN", id, in), operator)
}
func ListERPSalesReturnables(ctx context.Context, q model.ERPSalesReturnQuery, pg page.Query) ([]model.ERPSalesReturnable, int64, error) {
	return repository.SelectERPSalesReturnables(ctx, q, pg)
}
func GetERPSalesReturnable(ctx context.Context, id int64) (*model.ERPSalesReturnable, error) {
	if id < 1 {
		return nil, errs.New("包裹编号不合法")
	}
	return repository.SelectERPSalesReturnable(ctx, id)
}
func ListERPSalesReturns(ctx context.Context, q model.ERPSalesReturnQuery, pg page.Query) ([]model.ERPSalesReturn, int64, error) {
	return repository.SelectERPSalesReturns(ctx, q, pg)
}
func GetERPSalesReturn(ctx context.Context, id int64) (*model.ERPSalesReturn, error) {
	if id < 1 {
		return nil, errs.New("退货编号不合法")
	}
	return repository.SelectERPSalesReturn(ctx, id)
}
