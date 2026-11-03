package service

import (
	"context"
	"encoding/hex"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"sort"
)

func ListERPReplenishments(ctx context.Context, q model.ERPReplenishmentQuery, pg page.Query) ([]model.ERPReplenishment, int64, error) {
	if q.WarehouseID < 1 || q.SkuID < 0 {
		return nil, 0, errs.New("请选择有效仓库和SKU")
	}
	if pg.PageSize < 1 {
		pg.PageSize = 10
	}
	if pg.PageSize > 100 {
		pg.PageSize = 100
	}
	if pg.PageNum < 1 {
		pg.PageNum = 1
	}
	return repository.SelectERPReplenishments(ctx, q, pg)
}
func normalizeReplenishment(in *model.ERPReplenishmentWrite) error {
	if e := normalizePurchase(&in.ERPPurchaseWrite); e != nil {
		return e
	}
	if len(in.Basis) != len(in.Lines) {
		return errs.New("每行补货商品都必须有当前建议依据")
	}
	in.Basis = append([]model.ERPReplenishmentBasis(nil), in.Basis...)
	sort.Slice(in.Basis, func(i, j int) bool { return in.Basis[i].SkuID < in.Basis[j].SkuID })
	for i, b := range in.Basis {
		raw, e := hex.DecodeString(b.Basis)
		if e != nil || len(raw) != 32 || len(b.Basis) != 64 || b.SkuID != in.Lines[i].SkuID {
			return errs.New("补货建议依据不完整，请刷新后重新选择")
		}
	}
	return nil
}
func CreateERPReplenishmentPurchase(ctx context.Context, in model.ERPReplenishmentWrite, operator string) (*model.ERPPurchaseResult, error) {
	if e := normalizeReplenishment(&in); e != nil {
		return nil, e
	}
	return repository.CreateERPReplenishmentPurchase(ctx, in, purchaseHash(ctx, "REPLENISH", 0, in), operator)
}
