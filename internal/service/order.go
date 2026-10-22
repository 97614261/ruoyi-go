package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var orderRequestKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_:-]{8,64}$`)
var orderAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,15})(\.[0-9]{1,4})?$`)
var orderCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
var orderCountryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

func orderHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func orderText(s string, max int, required bool) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max || s != strings.TrimSpace(s) || (required && s == "") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func normalizeOrder(in *model.ERPOrderWrite) error {
	if in.ShopID < 1 || !orderText(in.SourceOrderNo, 128, true) || !orderRequestKeyPattern.MatchString(in.RequestKey) {
		return errs.New("店铺、来源订单号或请求标识不正确")
	}
	f := &in.ERPOrderFields
	if !orderText(f.ReceiverName, 120, true) || !orderText(f.ReceiverPhone, 64, false) || !orderText(f.ReceiverRegion, 120, false) || !orderText(f.ReceiverCity, 120, false) || !orderText(f.ReceiverPostcode, 32, false) || !orderText(f.ReceiverAddress, 500, true) {
		return errs.New("收件字段必填项、长度或格式不正确")
	}
	f.Currency = strings.ToUpper(f.Currency)
	f.ReceiverCountry = strings.ToUpper(f.ReceiverCountry)
	if !orderCurrencyPattern.MatchString(f.Currency) || !orderCountryPattern.MatchString(f.ReceiverCountry) || f.OrderedAt.IsZero() || f.OrderedAt.Std().Year() < 1970 || f.OrderedAt.Std().Year() > 9999 {
		return errs.New("币种、国家或下单时间格式不正确")
	}
	if f.OriginalAmount != nil {
		s := *f.OriginalAmount
		if !orderAmountPattern.MatchString(s) {
			return errs.New("金额须为最多16位整数、4位小数的非负十进制字符串；未知用null")
		}
		parts := strings.SplitN(s, ".", 2)
		fraction := ""
		if len(parts) == 2 {
			fraction = parts[1]
		}
		s = parts[0] + "." + fraction + strings.Repeat("0", 4-len(fraction))
		f.OriginalAmount = &s
	}
	if len(in.Lines) < 1 || len(in.Lines) > 100 {
		return errs.New("每单须有1—100行")
	}
	seen := map[string]bool{}
	for _, l := range in.Lines {
		if !orderText(l.SourceLineNo, 128, true) || !orderText(l.ShopItemCode, 128, true) || !orderText(l.SourceTitle, 200, true) || l.Quantity < 1 || l.Quantity > model.ERPMaxStock || seen[l.SourceLineNo] {
			return errs.New("订单行编号须唯一，编码/标题须有效，数量须为范围内正整数")
		}
		seen[l.SourceLineNo] = true
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].SourceLineNo < in.Lines[j].SourceLineNo })
	return nil
}
func orderInputHash(in model.ERPOrderWrite) string {
	in.RequestKey = ""
	in.Version = 0
	return orderHash(in)
}
func CreateERPOrder(ctx context.Context, in model.ERPOrderWrite, operator string) (*model.ERPOrderResult, error) {
	if err := normalizeOrder(&in); err != nil {
		return nil, err
	}
	return repository.CreateERPOrder(ctx, in, "MANUAL", orderInputHash(in), operator)
}
func EditERPOrder(ctx context.Context, id int64, in model.ERPOrderWrite, operator string) (*model.ERPOrderResult, error) {
	if err := normalizeOrder(&in); err != nil {
		return nil, err
	}
	if in.Version < 1 {
		return nil, errs.New("缺少订单版本")
	}
	return repository.EditERPOrder(ctx, id, in, orderHash(in), operator)
}
func validOrderQuery(q model.ERPOrderQuery) error {
	if q.ShopID < 0 || !orderText(q.SourceOrderNo, 128, false) {
		return errs.New("订单查询条件不正确")
	}
	switch q.Status {
	case "", "PENDING_REVIEW", "READY_TO_SHIP", "SHIPPED", "CANCELLED":
		return nil
	}
	return errs.New("订单状态不正确")
}
func ListERPOrders(ctx context.Context, q model.ERPOrderQuery, pg page.Query) ([]model.ERPOrderSummary, int64, error) {
	if err := validOrderQuery(q); err != nil {
		return nil, 0, err
	}
	return repository.SelectERPOrderPage(ctx, q, pg)
}
func GetERPOrder(ctx context.Context, id int64) (*model.ERPOrder, error) {
	return repository.SelectERPOrder(ctx, id)
}
func ExportERPOrders(ctx context.Context, q model.ERPOrderQuery) ([]model.ERPOrderSummary, error) {
	if err := validOrderQuery(q); err != nil {
		return nil, err
	}
	rows, err := repository.SelectERPOrderExport(ctx, q, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if len(rows) > MaxExportRows {
		return nil, errs.New("订单超出导出上限，请按店铺/状态缩小范围")
	}
	return rows, nil
}
