package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/page"
)

func ListERPDocuments(ctx context.Context, kind string, q model.ERPQuery, pg page.Query) ([]model.ERPDocument, int64, error) {
	return repository.SelectERPDocuments(ctx, kind, q, pg)
}
func ListERPStocks(ctx context.Context, q model.ERPQuery, pg page.Query) ([]model.ERPStockView, int64, error) {
	return repository.SelectERPStocks(ctx, q, pg)
}
func ListERPLedger(ctx context.Context, q model.ERPQuery, pg page.Query) ([]model.ERPLedger, int64, error) {
	return repository.SelectERPLedger(ctx, q, pg)
}
