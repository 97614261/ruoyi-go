package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

func ERPOrderWorkbench(ctx context.Context, q model.ERPOrderQuery) (*model.ERPOrderWorkbench, error) {
	if err := validOrderQuery(q); err != nil {
		return nil, err
	}
	return repository.SelectERPOrderWorkbench(ctx, q)
}
func CheckERPOrderReadiness(ctx context.Context, id, warehouse int64) (*model.ERPOrderReadiness, error) {
	if id < 1 || warehouse < 1 {
		return nil, errs.New("订单与仓库编号必须为正整数")
	}
	return repository.CheckERPOrderReadiness(ctx, id, warehouse)
}
