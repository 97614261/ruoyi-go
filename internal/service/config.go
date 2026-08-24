package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
)

// GetConfigValueByKey 按参数键取值，优先走 Redis。
//
// 缓存故障一律降级到数据库，不让缓存影响功能。
// 缓存不设过期，改参数时由 ClearConfigCache 主动清理（与 Java 一致）。
func GetConfigValueByKey(ctx context.Context, configKey string) (string, error) {
	cacheKey := redisx.SysConfigKey(configKey)

	value, err := redisx.C().Get(ctx, cacheKey).Result()
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, redis.Nil) {
		slog.Warn("读取参数缓存失败，回源查库", "configKey", configKey, "err", err)
	}

	value, err = repository.SelectConfigValueByKey(ctx, configKey)
	if err != nil {
		return "", err
	}
	if err := redisx.C().Set(ctx, cacheKey, value, 0).Err(); err != nil {
		slog.Warn("写入参数缓存失败", "configKey", configKey, "err", err)
	}
	return value, nil
}

// ClearConfigCache 清除参数缓存，configKey 为空时清全部。
func ClearConfigCache(ctx context.Context, configKey string) error {
	if configKey != "" {
		return redisx.C().Del(ctx, redisx.SysConfigKey(configKey)).Err()
	}
	return redisx.ScanKeys(ctx, redisx.KeySysConfig, 100, func(key string) error {
		return redisx.C().Del(ctx, key).Err()
	})
}

// ListConfigPage 分页查询参数。
func ListConfigPage(ctx context.Context, query model.ConfigQuery, pg page.Query) ([]model.SysConfig, int64, error) {
	return repository.SelectConfigPage(ctx, query, pg)
}

// ListConfigExport 导出用的全量查询。
func ListConfigExport(ctx context.Context, query model.ConfigQuery) ([]model.SysConfig, error) {
	list, err := repository.SelectConfigList(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// GetConfig 按 ID 查参数。
func GetConfig(ctx context.Context, configID int64) (*model.SysConfig, error) {
	config, err := repository.SelectConfigByID(ctx, configID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errs.New("参数不存在")
	}
	return config, nil
}

// CreateConfig 新增参数。
func CreateConfig(ctx context.Context, config *model.SysConfig, operator string) error {
	count, err := repository.CountConfigByKey(ctx, config.ConfigKey, 0)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("新增参数'%s'失败，参数键名已存在", config.ConfigName)
	}

	config.ConfigID = 0
	if config.ConfigType == "" {
		config.ConfigType = model.ConfigTypeCustom
	}
	config.CreateBy = operator
	config.CreateTime = types.Now()
	if err := repository.InsertConfig(ctx, config); err != nil {
		return err
	}
	return ClearConfigCache(ctx, config.ConfigKey)
}

// UpdateConfig 修改参数。
//
// 键名可能被改，所以新旧两个 key 的缓存都要清。
func UpdateConfig(ctx context.Context, config *model.SysConfig, operator string) error {
	if config.ConfigID == 0 {
		return errs.New("参数ID不能为空")
	}
	existing, err := repository.SelectConfigByID(ctx, config.ConfigID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("参数不存在")
	}

	count, err := repository.CountConfigByKey(ctx, config.ConfigKey, config.ConfigID)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("修改参数'%s'失败，参数键名已存在", config.ConfigName)
	}

	config.UpdateBy = operator
	config.UpdateTime = types.Now()
	if err := repository.UpdateConfig(ctx, config); err != nil {
		return err
	}
	if existing.ConfigKey != config.ConfigKey {
		if err := ClearConfigCache(ctx, existing.ConfigKey); err != nil {
			return err
		}
	}
	return ClearConfigCache(ctx, config.ConfigKey)
}

// DeleteConfigs 批量删除参数。
//
// 系统内置参数（config_type = 'Y'）不允许删除，与 Java 一致。
func DeleteConfigs(ctx context.Context, configIDs []int64) error {
	if len(configIDs) == 0 {
		return errs.New("请选择要删除的参数")
	}
	keys := make([]string, 0, len(configIDs))
	for _, configID := range configIDs {
		config, err := repository.SelectConfigByID(ctx, configID)
		if err != nil {
			return err
		}
		if config == nil {
			continue
		}
		if config.ConfigType == model.ConfigTypeBuiltin {
			return errs.Newf("内置参数【%s】不能删除", config.ConfigKey)
		}
		keys = append(keys, config.ConfigKey)
	}

	if err := repository.DeleteConfigByIDs(ctx, configIDs); err != nil {
		return err
	}
	for _, key := range keys {
		if err := ClearConfigCache(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
