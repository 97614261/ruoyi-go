package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/decimalx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
	"strings"
)

func CreateSalesFinanceReceivable(ctx context.Context, in model.SalesFinReceivableCreate, operator string) (*model.FinResult, error) {
	if in.OrderID < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) {
		return nil, errs.New("已发货订单或请求标识不合法")
	}
	return repository.CreateSalesFinReceivable(ctx, in, purchaseHash(ctx, "RECEIVABLE_CREATE", 0, in), operator)
}
func prepareSalesFinAdjust(in *model.SalesFinAdjust) error {
	if in.Version < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || in.ReturnID < 1 || !orderText(in.Reason, 500, true) {
		return errs.New("退货、版本、原因或请求标识不合法")
	}
	var e error
	in.Amount, e = decimalx.Normalize(in.Amount)
	if e != nil {
		return errs.New("请明确填写非负精确抵减金额，零额须人工确认")
	}
	return nil
}
func AdjustSalesFinanceReceivable(ctx context.Context, id int64, in model.SalesFinAdjust, operator string) (*model.FinResult, error) {
	if id < 1 {
		return nil, errs.New("应收编号不合法")
	}
	if e := prepareSalesFinAdjust(&in); e != nil {
		return nil, e
	}
	return repository.AdjustSalesFinReceivable(ctx, id, in, purchaseHash(ctx, "RECEIVABLE_ADJUST", id, in), operator)
}
func prepareSalesFinCollection(in *model.SalesFinCollectionWrite) error {
	if in.AccountID < 1 || in.ShopID < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || !orderText(in.Remark, 500, false) || len(in.Allocations) < 1 || len(in.Allocations) > 100 {
		return errs.New("收款参数、请求标识或核销行数不合法")
	}
	in.Currency = strings.ToUpper(in.Currency)
	if !orderCurrencyPattern.MatchString(in.Currency) {
		return errs.New("币种须为三位英文字母")
	}
	var e error
	in.Amount, e = decimalx.Normalize(in.Amount)
	if e != nil || in.Amount == "0.0000" {
		return errs.New("收款金额须为正的精确十进制字符串")
	}
	in.Allocations = append([]model.SalesFinAllocationInput(nil), in.Allocations...)
	sort.Slice(in.Allocations, func(i, j int) bool { return in.Allocations[i].ReceivableID < in.Allocations[j].ReceivableID })
	total := "0.0000"
	for i := range in.Allocations {
		l := &in.Allocations[i]
		if l.ReceivableID < 1 || l.Version < 1 || i > 0 && l.ReceivableID == in.Allocations[i-1].ReceivableID {
			return errs.New("核销应收不得重复，须携带各单当前版本")
		}
		l.Amount, e = decimalx.Normalize(l.Amount)
		if e != nil || l.Amount == "0.0000" {
			return errs.New("每行核销金额须为正的精确十进制字符串")
		}
		total, e = decimalx.Add(total, l.Amount)
		if e != nil {
			return errs.New("核销合计超出金额上限")
		}
	}
	if total != in.Amount {
		return errs.New("收款金额必须等于本次全部核销行合计")
	}
	return nil
}
func CreateSalesFinanceCollection(ctx context.Context, in model.SalesFinCollectionWrite, operator string) (*model.FinResult, error) {
	if e := prepareSalesFinCollection(&in); e != nil {
		return nil, e
	}
	return repository.CreateSalesFinCollection(ctx, in, purchaseHash(ctx, "COLLECTION_CREATE", 0, in), operator)
}
func ReverseSalesFinanceCollection(ctx context.Context, id int64, in model.FinReverse, operator string) (*model.FinResult, error) {
	if id < 1 || in.Version < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || !orderText(in.Reason, 500, true) {
		return nil, errs.New("收款编号、版本、请求标识或冲销原因不合法")
	}
	return repository.ReverseSalesFinCollection(ctx, id, in, purchaseHash(ctx, "COLLECTION_REVERSE", id, in), operator)
}
func validSalesFinQuery(q *model.SalesFinQuery) error {
	if q.ShopID < 0 || q.WarehouseID < 0 || q.AccountID < 0 || q.OrderID < 0 || !orderText(q.Code, 64, false) {
		return errs.New("财务筛选参数不合法")
	}
	if q.Currency != "" {
		q.Currency = strings.ToUpper(q.Currency)
		if !orderCurrencyPattern.MatchString(q.Currency) {
			return errs.New("筛选币种不合法")
		}
	}
	return nil
}
func ListSalesFinanceSources(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinSource, int64, error) {
	if e := validSalesFinQuery(&q); e != nil {
		return nil, 0, e
	}
	return repository.SelectSalesFinSources(ctx, q, pg)
}
func GetSalesFinanceSource(ctx context.Context, id int64) (*model.SalesFinSource, error) {
	return repository.SelectSalesFinSource(ctx, id)
}
func ListSalesFinanceReceivables(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinReceivable, int64, error) {
	if e := validSalesFinQuery(&q); e != nil {
		return nil, 0, e
	}
	return repository.SelectSalesFinReceivables(ctx, q, pg)
}
func GetSalesFinanceReceivable(ctx context.Context, id int64) (*model.SalesFinReceivable, error) {
	return repository.SelectSalesFinReceivable(ctx, id)
}
func ListSalesFinanceReturns(ctx context.Context, id int64, pending bool, pg page.Query) ([]model.SalesFinCredit, int64, error) {
	return repository.SelectSalesFinReturns(ctx, id, pending, pg)
}
func ListSalesFinanceCollections(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinCollection, int64, error) {
	if e := validSalesFinQuery(&q); e != nil {
		return nil, 0, e
	}
	if q.Status != "" && q.Status != "POSTED" && q.Status != "REVERSED" {
		return nil, 0, errs.New("收款状态不合法")
	}
	return repository.SelectSalesFinCollections(ctx, q, pg)
}
func GetSalesFinanceCollection(ctx context.Context, id int64) (*model.SalesFinCollection, error) {
	return repository.SelectSalesFinCollection(ctx, id)
}
func ListSalesFinanceStatements(ctx context.Context, q model.SalesFinQuery, pg page.Query) ([]model.SalesFinStatement, int64, error) {
	if e := validSalesFinQuery(&q); e != nil {
		return nil, 0, e
	}
	return repository.SelectSalesFinStatements(ctx, q, pg)
}
func ExportSalesFinanceReceivables(ctx context.Context, q model.SalesFinQuery) ([]model.SalesFinReceivable, error) {
	if e := validSalesFinQuery(&q); e != nil {
		return nil, e
	}
	rows, e := repository.ExportSalesFinReceivables(ctx, q, MaxExportRows+1)
	if e == nil {
		e = checkExportSize(len(rows))
	}
	return rows, e
}
func ExportSalesFinanceStatements(ctx context.Context, q model.SalesFinQuery) ([]model.SalesFinStatement, error) {
	rows, _, e := ListSalesFinanceStatements(ctx, q, page.Query{PageNum: 1, PageSize: MaxExportRows + 1})
	if e == nil {
		e = checkExportSize(len(rows))
	}
	return rows, e
}
