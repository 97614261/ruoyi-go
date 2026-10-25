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

func prepareFinAccount(in *model.FinAccountWrite, id int64) error {
	if !orderRequestKeyPattern.MatchString(in.RequestKey) || !orderText(in.AccountCode, 64, true) || !orderText(in.AccountName, 120, true) || !orderText(in.Remark, 500, false) {
		return errs.New("账户编码、名称、备注或请求标识不合法")
	}
	if in.AccountType != "BANK" && in.AccountType != "CASH" && in.AccountType != "OTHER" {
		return errs.New("账户类型须为BANK/CASH/OTHER")
	}
	in.Currency = strings.ToUpper(in.Currency)
	if !orderCurrencyPattern.MatchString(in.Currency) || in.Status != "0" && in.Status != "1" {
		return errs.New("账户币种或启停状态不合法")
	}
	if id > 0 && in.Version < 1 {
		return errs.New("请刷新取得账户版本")
	}
	if id == 0 {
		in.Version = 0
	}
	return nil
}
func CreateFinanceAccount(ctx context.Context, in model.FinAccountWrite, operator string) (*model.FinResult, error) {
	if e := prepareFinAccount(&in, 0); e != nil {
		return nil, e
	}
	return repository.CreateFinAccount(ctx, in, purchaseHash(ctx, "ACCOUNT_CREATE", 0, in), operator)
}
func EditFinanceAccount(ctx context.Context, id int64, in model.FinAccountWrite, operator string) (*model.FinResult, error) {
	if id < 1 {
		return nil, errs.New("账户编号不合法")
	}
	if e := prepareFinAccount(&in, id); e != nil {
		return nil, e
	}
	return repository.EditFinAccount(ctx, id, in, purchaseHash(ctx, "ACCOUNT_EDIT", id, in), operator)
}
func CreateFinancePayable(ctx context.Context, in model.FinPayableCreate, operator string) (*model.FinResult, error) {
	if in.ReceiptID < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) {
		return nil, errs.New("收货批次或请求标识不合法")
	}
	return repository.CreateFinPayable(ctx, in, purchaseHash(ctx, "PAYABLE_CREATE", 0, in), operator)
}
func prepareFinAdjust(in *model.FinAdjust) error {
	if in.Version < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || len(in.ReturnIDs) < 1 || len(in.ReturnIDs) > 100 {
		return errs.New("版本、请求标识及退货笔数不合法")
	}
	in.ReturnIDs = append([]int64(nil), in.ReturnIDs...)
	sort.Slice(in.ReturnIDs, func(i, j int) bool { return in.ReturnIDs[i] < in.ReturnIDs[j] })
	for i, id := range in.ReturnIDs {
		if id < 1 || i > 0 && id == in.ReturnIDs[i-1] {
			return errs.New("请选择1—100个不重复的采购退货")
		}
	}
	return nil
}
func AdjustFinancePayable(ctx context.Context, id int64, in model.FinAdjust, operator string) (*model.FinResult, error) {
	if id < 1 {
		return nil, errs.New("应付编号不合法")
	}
	if e := prepareFinAdjust(&in); e != nil {
		return nil, e
	}
	return repository.AdjustFinPayable(ctx, id, in, purchaseHash(ctx, "PAYABLE_ADJUST", id, in), operator)
}
func prepareFinPayment(in *model.FinPaymentWrite) error {
	if in.AccountID < 1 || in.SupplierID < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || !orderText(in.Remark, 500, false) || len(in.Allocations) < 1 || len(in.Allocations) > 100 {
		return errs.New("付款参数、请求标识或核销行数不合法")
	}
	in.Currency = strings.ToUpper(in.Currency)
	if !orderCurrencyPattern.MatchString(in.Currency) {
		return errs.New("币种须为三位英文字母")
	}
	var e error
	in.Amount, e = decimalx.Normalize(in.Amount)
	if e != nil || in.Amount == "0.0000" {
		return errs.New("付款金额须为正的精确十进制字符串")
	}
	in.Allocations = append([]model.FinAllocationInput(nil), in.Allocations...)
	sort.Slice(in.Allocations, func(i, j int) bool { return in.Allocations[i].PayableID < in.Allocations[j].PayableID })
	total := "0.0000"
	for i := range in.Allocations {
		l := &in.Allocations[i]
		if l.PayableID < 1 || l.Version < 1 || i > 0 && l.PayableID == in.Allocations[i-1].PayableID {
			return errs.New("核销应付不得重复，须携带各单当前版本")
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
		return errs.New("付款金额必须等于本次全部核销行合计")
	}
	return nil
}
func CreateFinancePayment(ctx context.Context, in model.FinPaymentWrite, operator string) (*model.FinResult, error) {
	if e := prepareFinPayment(&in); e != nil {
		return nil, e
	}
	return repository.CreateFinPayment(ctx, in, purchaseHash(ctx, "PAYMENT_CREATE", 0, in), operator)
}
func ReverseFinancePayment(ctx context.Context, id int64, in model.FinReverse, operator string) (*model.FinResult, error) {
	if id < 1 || in.Version < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) || !orderText(in.Reason, 500, true) {
		return nil, errs.New("付款编号、版本、请求标识或冲销原因不合法")
	}
	return repository.ReverseFinPayment(ctx, id, in, purchaseHash(ctx, "PAYMENT_REVERSE", id, in), operator)
}
func validFinQuery(q *model.FinQuery) error {
	if q.SupplierID < 0 || q.WarehouseID < 0 || q.AccountID < 0 || q.ReceiptID < 0 || !orderText(q.Code, 64, false) {
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
func ListFinanceAccounts(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinAccount, int64, error) {
	if e := validFinQuery(&q); e != nil {
		return nil, 0, e
	}
	if q.Status != "" && q.Status != "0" && q.Status != "1" {
		return nil, 0, errs.New("账户状态不合法")
	}
	return repository.SelectFinAccounts(ctx, q, pg)
}
func GetFinanceAccount(ctx context.Context, id int64) (*model.FinAccount, error) {
	return repository.SelectFinAccount(ctx, id)
}
func ListFinanceSources(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinSource, int64, error) {
	if e := validFinQuery(&q); e != nil {
		return nil, 0, e
	}
	return repository.SelectFinSources(ctx, q, pg)
}
func GetFinanceSource(ctx context.Context, id int64) (*model.FinSource, error) {
	return repository.SelectFinSource(ctx, id)
}
func ListFinancePayables(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinPayable, int64, error) {
	if e := validFinQuery(&q); e != nil {
		return nil, 0, e
	}
	return repository.SelectFinPayables(ctx, q, pg)
}
func GetFinancePayable(ctx context.Context, id int64) (*model.FinPayable, error) {
	return repository.SelectFinPayable(ctx, id)
}
func ListFinanceReturns(ctx context.Context, id int64, pending bool, pg page.Query) ([]model.FinCredit, int64, error) {
	return repository.SelectFinReturns(ctx, id, pending, pg)
}
func ListFinancePayments(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinPayment, int64, error) {
	if e := validFinQuery(&q); e != nil {
		return nil, 0, e
	}
	if q.Status != "" && q.Status != "POSTED" && q.Status != "REVERSED" {
		return nil, 0, errs.New("付款状态不合法")
	}
	return repository.SelectFinPayments(ctx, q, pg)
}
func GetFinancePayment(ctx context.Context, id int64) (*model.FinPayment, error) {
	return repository.SelectFinPayment(ctx, id)
}
func ListFinanceStatements(ctx context.Context, q model.FinQuery, pg page.Query) ([]model.FinStatement, int64, error) {
	if e := validFinQuery(&q); e != nil {
		return nil, 0, e
	}
	return repository.SelectFinStatements(ctx, q, pg)
}
func ExportFinancePayables(ctx context.Context, q model.FinQuery) ([]model.FinPayable, error) {
	if e := validFinQuery(&q); e != nil {
		return nil, e
	}
	rows, e := repository.ExportFinPayables(ctx, q, MaxExportRows+1)
	if e == nil {
		e = checkExportSize(len(rows))
	}
	return rows, e
}
func ExportFinanceStatements(ctx context.Context, q model.FinQuery) ([]model.FinStatement, error) {
	rows, _, e := ListFinanceStatements(ctx, q, page.Query{PageNum: 1, PageSize: MaxExportRows + 1})
	if e == nil {
		e = checkExportSize(len(rows))
	}
	return rows, e
}
