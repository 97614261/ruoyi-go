package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
)

func normalizePurchaseReceive(in *model.ERPPurchaseReceiveInput) error {
	if e := validERPAction(in.ERPActionToken); e != nil {
		return e
	}
	if len(in.Lines) < 1 || len(in.Lines) > 100 {
		return errs.New("本次收货须为1—100行")
	}
	in.Lines = append([]model.ERPPurchaseReceiveLineInput(nil), in.Lines...)
	seen := map[int64]bool{}
	for _, l := range in.Lines {
		if l.PurchaseLineID < 1 || l.Quantity < 1 || l.Quantity > 1000000000 || seen[l.PurchaseLineID] {
			return errs.New("采购行须唯一，收货数量须为1—十亿整数")
		}
		seen[l.PurchaseLineID] = true
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].PurchaseLineID < in.Lines[j].PurchaseLineID })
	return nil
}
func ReceiveERPPurchase(ctx context.Context, id int64, in model.ERPPurchaseReceiveInput, operator string) (*model.ERPPurchaseReceiveResult, error) {
	if id < 1 {
		return nil, errs.New("采购编号不合法")
	}
	if e := normalizePurchaseReceive(&in); e != nil {
		return nil, e
	}
	return repository.ReceiveERPPurchase(ctx, id, in, purchaseHash(ctx, "RECEIVE", id, in), operator)
}
func ListERPPurchaseReceivables(ctx context.Context, q model.ERPPurchaseQuery, pg page.Query) ([]model.ERPPurchaseSummary, int64, error) {
	switch q.Status {
	case "", "CONFIRMED", "PARTIAL_RECEIVED":
	default:
		return nil, 0, errs.New("可收货列表仅支持已确认或部分收货状态")
	}
	return repository.SelectERPPurchaseReceivables(ctx, q, pg)
}
func GetERPPurchaseReceivable(ctx context.Context, id int64) (*model.ERPPurchaseReceivable, error) {
	if id < 1 {
		return nil, errs.New("采购编号不合法")
	}
	return repository.SelectERPPurchaseReceivable(ctx, id)
}
func ListERPPurchaseReceipts(ctx context.Context, q model.ERPPurchaseReceiptQuery, pg page.Query) ([]model.ERPPurchaseReceipt, int64, error) {
	return repository.SelectERPPurchaseReceipts(ctx, q, pg)
}
func GetERPPurchaseReceipt(ctx context.Context, id int64) (*model.ERPPurchaseReceipt, error) {
	if id < 1 {
		return nil, errs.New("收货编号不合法")
	}
	return repository.SelectERPPurchaseReceipt(ctx, id)
}
