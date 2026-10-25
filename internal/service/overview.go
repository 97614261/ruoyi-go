package service

import (
	"context"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func overviewWindow(now time.Time) (today, seven, through string) {
	n := now.In(types.Location)
	start := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, types.Location)
	return start.Format(types.Layout), start.AddDate(0, 0, -6).Format(types.Layout), start.AddDate(0, 0, 1).Format(types.Layout)
}
func OverviewOrders(ctx context.Context) (*model.OverviewOrders, error) {
	return repository.SelectOverviewOrders(ctx)
}
func OverviewShipments(ctx context.Context) (*model.OverviewShipments, error) {
	a, b, c := overviewWindow(time.Now())
	return repository.SelectOverviewShipments(ctx, a, b, c)
}
func OverviewStocks(ctx context.Context) (*model.OverviewStocks, error) {
	return repository.SelectOverviewStocks(ctx)
}
func OverviewPurchases(ctx context.Context) (*model.OverviewPurchases, error) {
	return repository.SelectOverviewPurchases(ctx)
}

var overviewCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func OverviewMoney(ctx context.Context, in model.OverviewMoneyQuery, pg page.Query, sales bool) (*model.OverviewMoneyPage, error) {
	if in.Currency != "" && !overviewCurrency.MatchString(in.Currency) {
		return nil, errs.New("币种须为三位大写字母")
	}
	if pg.PageNum < 1 {
		pg.PageNum = 1
	}
	if pg.PageSize < 1 {
		pg.PageSize = page.DefaultPageSize
	}
	if pg.PageSize > page.MaxPageSize {
		pg.PageSize = page.MaxPageSize
	}
	return repository.SelectOverviewMoney(ctx, in.Currency, pg, sales)
}
