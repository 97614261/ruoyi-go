package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func ValidateERPStockQuery(q model.ERPQuery) error {
	if q.StockState != "" && q.StockState != "ALL" && q.StockState != "IN_STOCK" && q.StockState != "ZERO" && q.StockState != "RESERVED_OUT" && q.StockState != "LOW" && q.StockState != "HAS_STOCK" && q.StockState != "HAS_RESERVED" {
		return errs.New("库存筛选不合法")
	}
	var begin, end time.Time
	for _, p := range []struct {
		s string
		t *time.Time
	}{{q.BeginTime, &begin}, {q.EndTime, &end}} {
		if p.s != "" {
			v, e := time.ParseInLocation("2006-01-02 15:04:05", p.s, time.FixedZone("CST", 8*3600))
			if e != nil {
				return errs.New("时间必须为yyyy-MM-dd HH:mm:ss")
			}
			*p.t = v
		}
	}
	if !begin.IsZero() && !end.IsZero() && begin.After(end) {
		return errs.New("开始时间不能晚于结束时间")
	}
	return nil
}
func ERPStockExport(ctx context.Context, q model.ERPQuery) ([]model.ERPStockView, error) {
	if e := ValidateERPStockQuery(q); e != nil {
		return nil, e
	}
	out, e := repository.ExportERPStocks(ctx, q, MaxExportRows+1)
	if e == nil {
		e = checkExportSize(len(out))
	}
	return out, e
}
func ERPLedgerExport(ctx context.Context, q model.ERPQuery) ([]model.ERPLedger, error) {
	if e := ValidateERPStockQuery(q); e != nil {
		return nil, e
	}
	out, e := repository.ExportERPLedger(ctx, q, MaxExportRows+1)
	if e == nil {
		e = checkExportSize(len(out))
	}
	return out, e
}
func ERPStockReconcile(ctx context.Context, q model.ERPQuery, pg page.Query) ([]model.ERPStockReconciliation, int64, error) {
	return repository.ReconcileERPStocks(ctx, q, pg)
}
func ERPStockReservations(ctx context.Context, q model.ERPQuery, pg page.Query) ([]model.ERPDocument, int64, error) {
	return repository.SelectERPReservations(ctx, q, pg)
}
func ERPAlertGet(ctx context.Context, w, s int64) (*model.ERPStockAlert, error) {
	if w < 1 || s < 1 {
		return nil, errs.New("仓库或SKU不合法")
	}
	return repository.GetERPStockAlert(ctx, w, s)
}
func ERPAlertSet(ctx context.Context, in model.ERPStockAlert, operator string) (*model.ERPStockAlert, error) {
	if in.WarehouseID < 1 || in.SkuID < 1 || in.Version < 0 || in.ThresholdQuantity != nil && (*in.ThresholdQuantity < 0 || *in.ThresholdQuantity > model.ERPMaxStock) {
		return nil, errs.New("预警参数不合法")
	}
	return repository.SetERPStockAlert(ctx, in, operator)
}
func prepareTransfer(ctx context.Context, id int64, action string, in model.ERPTransferInput) (model.ERPTransferInput, string, error) {
	if !erpRequestKey.MatchString(in.RequestKey) || in.Version < 0 || utf8.RuneCountInString(in.Remark) > 500 {
		return in, "", errs.New("请求标识、版本或备注不合法")
	}
	if action == "CREATE" || action == "EDIT" {
		if in.SourceWarehouseID < 1 || in.TargetWarehouseID < 1 || in.SourceWarehouseID == in.TargetWarehouseID || len(in.Lines) < 1 || len(in.Lines) > 100 {
			return in, "", errs.New("请选择两个不同仓库和1—100行商品")
		}
		in.Lines = append([]model.ERPLineInput(nil), in.Lines...)
		sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].SkuID < in.Lines[j].SkuID })
		for i, l := range in.Lines {
			if l.SkuID < 1 || l.Quantity < 1 || l.Quantity > 1000000000 || l.CountedQuantity != nil || i > 0 && l.SkuID == in.Lines[i-1].SkuID {
				return in, "", errs.New("商品不可重复，数量须为1—十亿整数")
			}
		}
		in.Remark = strings.TrimSpace(in.Remark)
	} else if action != "DISPATCH" && action != "RECEIVE" && action != "CANCEL" {
		return in, "", errs.New("调拨动作不合法")
	}
	if action == "CREATE" {
		in.Version = 0
	} else if id < 1 || in.Version < 1 {
		return in, "", errs.New("请刷新取得调拨版本")
	}
	raw, _ := json.Marshal(struct {
		ID     int64
		Actor  int64
		Action string
		Input  model.ERPTransferInput
	}{id, erpctx.UserID(ctx), action, in})
	h := sha256.Sum256(raw)
	return in, hex.EncodeToString(h[:]), nil
}
func ERPTransferCreate(ctx context.Context, in model.ERPTransferInput, operator string) (*model.ERPTransferResult, error) {
	in, hash, e := prepareTransfer(ctx, 0, "CREATE", in)
	if e != nil {
		return nil, e
	}
	return repository.InsertERPTransfer(ctx, in, hash, operator)
}
func ERPTransferChange(ctx context.Context, id int64, action string, in model.ERPTransferInput, operator string) (*model.ERPTransferResult, error) {
	in, hash, e := prepareTransfer(ctx, id, action, in)
	if e != nil {
		return nil, e
	}
	return repository.ChangeERPTransfer(ctx, id, action, in, hash, operator)
}
func ERPTransferList(ctx context.Context, q model.ERPTransferQuery, pg page.Query) ([]model.ERPTransfer, int64, error) {
	return repository.SelectERPTransfers(ctx, q, pg)
}
func ERPTransferGet(ctx context.Context, id int64) (*model.ERPTransfer, error) {
	if id < 1 {
		return nil, errs.New("调拨编号不合法")
	}
	return repository.GetERPTransfer(ctx, id)
}
