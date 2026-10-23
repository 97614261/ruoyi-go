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

func normalizePurchaseReturn(in *model.ERPPurchaseReturnInput) error {
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
	in.Lines = append([]model.ERPPurchaseReturnLineInput(nil), in.Lines...)
	seen := map[int64]bool{}
	for _, l := range in.Lines {
		if l.ReceiptLineID < 1 || l.Quantity < 1 || l.Quantity > 1000000000 || seen[l.ReceiptLineID] {
			return errs.New("原收货行须唯一，退货数量须为1—十亿整数")
		}
		seen[l.ReceiptLineID] = true
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].ReceiptLineID < in.Lines[j].ReceiptLineID })
	return nil
}
func ReturnERPPurchaseReceipt(ctx context.Context, id int64, in model.ERPPurchaseReturnInput, operator string) (*model.ERPPurchaseReturnResult, error) {
	if id < 1 {
		return nil, errs.New("收货编号不合法")
	}
	if e := normalizePurchaseReturn(&in); e != nil {
		return nil, e
	}
	return repository.ReturnERPPurchaseReceipt(ctx, id, in, purchaseHash(ctx, "PURCHASE_RETURN", id, in), operator)
}
func ListERPPurchaseReturnables(ctx context.Context, q model.ERPPurchaseReceiptQuery, pg page.Query) ([]model.ERPPurchaseReturnable, int64, error) {
	return repository.SelectERPPurchaseReturnables(ctx, q, pg)
}
func GetERPPurchaseReturnable(ctx context.Context, id int64) (*model.ERPPurchaseReturnable, error) {
	if id < 1 {
		return nil, errs.New("收货编号不合法")
	}
	return repository.SelectERPPurchaseReturnable(ctx, id)
}
func ListERPPurchaseReturns(ctx context.Context, q model.ERPPurchaseReturnQuery, pg page.Query) ([]model.ERPPurchaseReturn, int64, error) {
	return repository.SelectERPPurchaseReturns(ctx, q, pg)
}
func GetERPPurchaseReturn(ctx context.Context, id int64) (*model.ERPPurchaseReturn, error) {
	if id < 1 {
		return nil, errs.New("退货编号不合法")
	}
	return repository.SelectERPPurchaseReturn(ctx, id)
}
