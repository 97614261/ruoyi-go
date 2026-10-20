package service

import (
	"context"
	"strings"
	"sync"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var erpWarehouseWriteMu sync.Mutex

func ListErpWarehousePage(ctx context.Context, query model.ErpWarehouseQuery, pg page.Query) ([]model.ErpWarehouse, int64, error) {
	return repository.SelectErpWarehousePage(ctx, query, pg)
}

func ListErpWarehouseExport(ctx context.Context, query model.ErpWarehouseQuery) ([]model.ErpWarehouse, error) {
	list, err := repository.SelectErpWarehouseList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

func GetErpWarehouse(ctx context.Context, id string) (*model.ErpWarehouse, error) {
	target, err := repository.SelectErpWarehouseByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errs.New("数据不存在")
	}
	return target, nil
}

func CreateErpWarehouse(ctx context.Context, target *model.ErpWarehouse, operator string) error {
	erpWarehouseWriteMu.Lock()
	defer erpWarehouseWriteMu.Unlock()
	if err := validateERPWarehouse(target); err != nil {
		return err
	}
	target.WarehouseId = 0
	target.CreateBy = operator
	target.CreateTime = types.Now()
	return repository.InsertErpWarehouse(ctx, target)
}

func UpdateErpWarehouse(ctx context.Context, target *model.ErpWarehouse, operator string) error {
	erpWarehouseWriteMu.Lock()
	defer erpWarehouseWriteMu.Unlock()
	if err := validateERPWarehouse(target); err != nil {
		return err
	}
	if target.WarehouseId <= 0 {
		return errs.New("主键不能为空")
	}
	target.UpdateBy = operator
	target.UpdateTime = types.Now()
	return repository.UpdateErpWarehouse(ctx, target)
}

func DeleteErpWarehouse(ctx context.Context, ids []string) error {
	erpWarehouseWriteMu.Lock()
	defer erpWarehouseWriteMu.Unlock()
	if len(ids) == 0 || len(ids) > 200 {
		return errs.New("删除参数数量不合法")
	}
	clean := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || len(id) > 128 {
			return errs.New("删除参数格式不合法")
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			clean = append(clean, id)
		}
	}
	return repository.DeleteErpWarehouseByIDs(ctx, clean)
}
