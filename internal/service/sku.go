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

var erpSkuWriteMu sync.Mutex

func ListErpSkuPage(ctx context.Context, query model.ErpSkuQuery, pg page.Query) ([]model.ErpSku, int64, error) {
	return repository.SelectErpSkuPage(ctx, query, pg)
}

func ListErpSkuExport(ctx context.Context, query model.ErpSkuQuery) ([]model.ErpSku, error) {
	list, err := repository.SelectErpSkuList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

func GetErpSku(ctx context.Context, id string) (*model.ErpSku, error) {
	target, err := repository.SelectErpSkuByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errs.New("数据不存在")
	}
	return target, nil
}

func CreateErpSku(ctx context.Context, target *model.ErpSku, operator string) error {
	erpSkuWriteMu.Lock()
	defer erpSkuWriteMu.Unlock()
	if err := validateERPSku(target); err != nil {
		return err
	}
	target.SkuId = 0
	target.CreateBy = operator
	target.CreateTime = types.Now()
	return repository.InsertErpSku(ctx, target)
}

func UpdateErpSku(ctx context.Context, target *model.ErpSku, operator string) error {
	erpSkuWriteMu.Lock()
	defer erpSkuWriteMu.Unlock()
	if err := validateERPSku(target); err != nil {
		return err
	}
	if target.SkuId <= 0 {
		return errs.New("主键不能为空")
	}
	target.UpdateBy = operator
	target.UpdateTime = types.Now()
	return repository.UpdateErpSku(ctx, target)
}

func DeleteErpSku(ctx context.Context, ids []string) error {
	erpSkuWriteMu.Lock()
	defer erpSkuWriteMu.Unlock()
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
	return repository.DeleteErpSkuByIDs(ctx, clean)
}
