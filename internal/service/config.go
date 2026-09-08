package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
)

// configCacheTTL 既限制异常旧值的存活时间，也让升级前的永久缓存逐步收敛。
const configCacheTTL = 30 * time.Minute

// GetConfigValueByKey 按参数键取值，优先走 Redis。
//
// 缓存故障一律降级到数据库，不让缓存影响功能。
// 缓存使用有限 TTL；修改参数时推进代数并主动清理，旧数据库快照不能越过代数写回。
func GetConfigValueByKey(ctx context.Context, configKey string) (string, error) {
	cacheKey := redisx.SysConfigKey(configKey)

	value, err := redisx.GetConfigCache(ctx, cacheKey, configCacheTTL)
	if err == nil {
		return value, nil
	}
	cacheAvailable := errors.Is(err, redis.Nil)
	if !errors.Is(err, redis.Nil) {
		slog.Warn("读取参数缓存失败，回源查库", "configKey", configKey, "err", err)
	}

	var revision int64
	if cacheAvailable {
		revision, err = redisx.ConfigCacheRevision(ctx)
		if err != nil {
			cacheAvailable = false
			slog.Warn("读取参数缓存代数失败，本次不回填缓存", "configKey", configKey, "err", err)
		}
	}

	value, err = repository.SelectConfigValueByKey(ctx, configKey)
	if err != nil {
		return "", err
	}
	if cacheAvailable {
		stored, storeErr := redisx.StoreConfigCacheIfRevision(ctx, cacheKey, value, revision, configCacheTTL)
		if storeErr != nil {
			slog.Warn("写入参数缓存失败", "configKey", configKey, "err", storeErr)
		} else if !stored {
			slog.Debug("参数已在回源期间更新，跳过旧值回填", "configKey", configKey)
		}
	}
	return value, nil
}

// ClearConfigCache 清除参数缓存，configKey 为空时清全部。
func ClearConfigCache(ctx context.Context, configKey string) error {
	if configKey != "" {
		return clearConfigCacheKeys(ctx, configKey)
	}
	if err := redisx.InvalidateConfigCache(ctx); err != nil {
		return err
	}
	return redisx.ScanKeys(ctx, redisx.KeySysConfig, 100, func(key string) error {
		if redisx.IsConfigCacheMetadataKey(key) {
			return nil
		}
		return redisx.C().Del(ctx, key).Err()
	})
}

func clearConfigCacheKeys(ctx context.Context, configKeys ...string) error {
	keys := make([]string, 0, len(configKeys))
	for _, configKey := range configKeys {
		if configKey != "" {
			keys = append(keys, redisx.SysConfigKey(configKey))
		}
	}
	return redisx.InvalidateConfigCache(ctx, keys...)
}

// ListConfigPage 分页查询参数。
func ListConfigPage(ctx context.Context, query model.ConfigQuery, pg page.Query) ([]model.SysConfig, int64, error) {
	return repository.SelectConfigPage(ctx, query, pg)
}

// ListConfigExport 导出用的全量查询。
func ListConfigExport(ctx context.Context, query model.ConfigQuery) ([]model.SysConfig, error) {
	list, err := repository.SelectConfigList(ctx, query, MaxExportRows+1)
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
	configWriteMu.Lock()
	defer configWriteMu.Unlock()

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
	if err := validateConfigCreate(config); err != nil {
		return err
	}
	config.CreateBy = operator
	config.CreateTime = types.Now()
	if err := repository.InsertConfig(ctx, config); err != nil {
		return err
	}
	return runPostCommit(ctx, func(postCtx context.Context) error {
		return ClearConfigCache(postCtx, config.ConfigKey)
	})
}

// UpdateConfig 修改参数。
//
// 键名可能被改，所以新旧两个 key 的缓存都要清。
func UpdateConfig(ctx context.Context, config *model.SysConfig, operator string) error {
	configWriteMu.Lock()
	defer configWriteMu.Unlock()

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
	if err := validateConfigUpdate(existing, config); err != nil {
		return err
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
	return runPostCommit(ctx, func(postCtx context.Context) error {
		return clearConfigCacheKeys(postCtx, existing.ConfigKey, config.ConfigKey)
	})
}

// DeleteConfigs 批量删除参数。
//
// 系统内置参数（config_type = 'Y'）不允许删除，与 Java 一致。
func DeleteConfigs(ctx context.Context, configIDs []int64) error {
	configWriteMu.Lock()
	defer configWriteMu.Unlock()

	if len(configIDs) == 0 {
		return errs.New("请选择要删除的参数")
	}
	var err error
	configIDs, err = normalizeRelationIDs(configIDs, "参数")
	if err != nil {
		return err
	}
	configs, err := repository.SelectConfigsByIDs(ctx, configIDs)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(configs))
	for _, config := range configs {
		if config.ConfigType == model.ConfigTypeBuiltin || isContractConfigKey(config.ConfigKey) {
			return errs.Newf("内置参数【%s】不能删除", config.ConfigKey)
		}
		keys = append(keys, config.ConfigKey)
	}

	if err := repository.DeleteConfigByIDs(ctx, configIDs); err != nil {
		return err
	}
	return runPostCommit(ctx, func(postCtx context.Context) error {
		return clearConfigCacheKeys(postCtx, keys...)
	})
}
