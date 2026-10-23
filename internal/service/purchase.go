package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
	"strings"
)

func normalizePurchase(in *model.ERPPurchaseWrite) error {
	if in.SupplierID < 1 || in.WarehouseID < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || !orderText(in.Remark, 500, false) {
		return errs.New("供应商、仓库、请求标识或备注不合法")
	}
	in.Currency = strings.ToUpper(in.Currency)
	if !orderCurrencyPattern.MatchString(in.Currency) {
		return errs.New("币种须为3位英文字母")
	}
	if len(in.Lines) < 1 || len(in.Lines) > 100 {
		return errs.New("采购明细须为1—100行")
	}
	in.Lines = append([]model.ERPPurchaseLineInput(nil), in.Lines...)
	seen := map[int64]bool{}
	total := "0.0000"
	for i := range in.Lines {
		l := &in.Lines[i]
		if l.SkuID < 1 || l.Quantity < 1 || l.Quantity > 1000000000 || seen[l.SkuID] {
			return errs.New("SKU须唯一且数量为1—十亿的整数")
		}
		seen[l.SkuID] = true
		p, e := decimalx.Normalize(l.UnitPrice)
		if e != nil {
			return errs.New(decimalx.ErrRange.Error())
		}
		l.UnitPrice = p
		l.LineAmount, e = decimalx.Multiply(p, l.Quantity)
		if e != nil {
			return errs.New(decimalx.ErrRange.Error())
		}
		total, e = decimalx.Add(total, l.LineAmount)
		if e != nil {
			return errs.New(decimalx.ErrRange.Error())
		}
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].SkuID < in.Lines[j].SkuID })
	in.OrderedAmount = total
	return nil
}
func purchaseHash(ctx context.Context, action string, id int64, in any) string {
	return orderHash([]any{erpctx.UserID(ctx), action, id, in})
}
func CreateERPPurchase(ctx context.Context, in model.ERPPurchaseWrite, operator string) (*model.ERPPurchaseResult, error) {
	if e := normalizePurchase(&in); e != nil {
		return nil, e
	}
	return repository.CreateERPPurchase(ctx, in, purchaseHash(ctx, "CREATE", 0, in), operator)
}
func EditERPPurchase(ctx context.Context, id int64, in model.ERPPurchaseEdit, operator string) (*model.ERPPurchaseResult, error) {
	if id < 1 || in.Version < 1 {
		return nil, errs.New("采购编号或版本不合法")
	}
	if e := normalizePurchase(&in.ERPPurchaseWrite); e != nil {
		return nil, e
	}
	return repository.EditERPPurchase(ctx, id, in, purchaseHash(ctx, "EDIT", id, in), operator)
}
func ActERPPurchase(ctx context.Context, id int64, action string, in model.ERPPurchaseAction, operator string) (*model.ERPPurchaseResult, error) {
	if id < 1 {
		return nil, errs.New("采购编号不合法")
	}
	if e := validERPAction(in.ERPActionToken); e != nil {
		return nil, e
	}
	switch action {
	case "CONFIRM":
		if in.Reason != "" {
			return nil, errs.New("确认动作不接收原因")
		}
	case "CANCEL", "CLOSE":
		if !orderText(in.Reason, 500, true) {
			return nil, errs.New("取消/关闭须填写1—500字原因")
		}
	default:
		return nil, errs.New("采购动作不合法")
	}
	return repository.ActERPPurchase(ctx, id, action, in, purchaseHash(ctx, action, id, in), operator)
}
func ListERPPurchases(ctx context.Context, q model.ERPPurchaseQuery, pg page.Query) ([]model.ERPPurchaseSummary, int64, error) {
	switch q.Status {
	case "", "DRAFT", "CONFIRMED", "PARTIAL_RECEIVED", "COMPLETED", "CLOSED", "CANCELLED":
	default:
		return nil, 0, errs.New("采购状态不合法")
	}
	return repository.SelectERPPurchasePage(ctx, q, pg)
}
func GetERPPurchase(ctx context.Context, id int64) (*model.ERPPurchase, error) {
	if id < 1 {
		return nil, errs.New("采购编号不合法")
	}
	return repository.SelectERPPurchase(ctx, id)
}
func ListERPPurchaseEvents(ctx context.Context, id int64, pg page.Query) ([]model.ERPPurchaseEvent, int64, error) {
	if id < 1 {
		return nil, 0, errs.New("采购编号不合法")
	}
	return repository.SelectERPPurchaseEvents(ctx, id, pg)
}
