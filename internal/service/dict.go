package service

import (
	"context"
	"encoding/json"
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

// dictCacheTTL 字典缓存过期时间。
//
// Java 版是启动时全量加载、改字典时主动清理，缓存本身不设过期。
// 这里额外加了 TTL 兜底：万一某条清理路径漏了，最多 30 分钟自动纠正，
// 不至于永久读到旧数据。清理逻辑仍然要写全，TTL 只是保险。
const dictCacheTTL = 30 * time.Minute

// GetDictDataByType 按类型取字典数据，优先走 Redis。
//
// 缓存读写失败一律降级到数据库，不让缓存故障影响功能。
func GetDictDataByType(ctx context.Context, dictType string) ([]model.SysDictData, error) {
	key := redisx.SysDictKey(dictType)

	raw, cacheErr := redisx.C().Get(ctx, key).Bytes()
	cacheAvailable := errors.Is(cacheErr, redis.Nil)
	if cacheErr == nil {
		var cached []model.SysDictData
		if err := json.Unmarshal(raw, &cached); err == nil {
			return cached, nil
		}
		slog.Warn("字典缓存解析失败，回源查库", "dictType", dictType)
		if err := redisx.InvalidateDictCache(ctx, key); err != nil {
			slog.Warn("清理损坏的字典缓存失败，本次不回填", "dictType", dictType, "err", err)
		} else {
			cacheAvailable = true
		}
	} else if !errors.Is(cacheErr, redis.Nil) {
		slog.Warn("读取字典缓存失败，回源查库", "dictType", dictType, "err", cacheErr)
	}

	var revision int64
	if cacheAvailable {
		var err error
		revision, err = redisx.DictCacheRevision(ctx)
		if err != nil {
			cacheAvailable = false
			slog.Warn("读取字典缓存代数失败，本次不回填", "dictType", dictType, "err", err)
		}
	}

	list, err := repository.SelectDictDataByType(ctx, dictType)
	if err != nil {
		return nil, err
	}
	fillDictDefault(list)

	if cacheAvailable {
		if data, err := json.Marshal(list); err == nil {
			stored, storeErr := redisx.StoreDictCacheIfRevision(ctx, key, data, revision, dictCacheTTL)
			if storeErr != nil {
				slog.Warn("写入字典缓存失败", "dictType", dictType, "err", storeErr)
			} else if !stored {
				slog.Debug("字典已在回源期间更新，跳过旧值回填", "dictType", dictType)
			}
		}
	}
	return list, nil
}

// ClearDictCache 清除指定类型的字典缓存，dictType 为空时清除全部。
func ClearDictCache(ctx context.Context, dictType string) error {
	if dictType != "" {
		return redisx.InvalidateDictCache(ctx, redisx.SysDictKey(dictType))
	}
	if err := redisx.InvalidateDictCache(ctx); err != nil {
		return err
	}
	return redisx.ScanKeys(ctx, redisx.KeySysDict, 100, func(key string) error {
		if redisx.IsDictCacheMetadataKey(key) {
			return nil
		}
		return redisx.C().Del(ctx, key).Err()
	})
}

// fillDictDefault 填充非表字段 default，对齐 Java 版 getDefault() 的序列化结果。
func fillDictDefault(list []model.SysDictData) {
	for i := range list {
		list[i].Default = list[i].IsDefaultDict()
	}
}

// ---------- 字典类型 ----------

// ListDictTypePage 分页查询字典类型。
func ListDictTypePage(ctx context.Context, query model.DictTypeQuery, pg page.Query) ([]model.SysDictType, int64, error) {
	return repository.SelectDictTypePage(ctx, query, pg)
}

// ListDictTypeExport 导出用的全量查询。
func ListDictTypeExport(ctx context.Context, query model.DictTypeQuery) ([]model.SysDictType, error) {
	list, err := repository.SelectDictTypeList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// ListDictTypeAll 查全部字典类型，供下拉选择。
func ListDictTypeAll(ctx context.Context) ([]model.SysDictType, error) {
	return repository.SelectDictTypeAll(ctx)
}

// GetDictType 按主键查字典类型。
func GetDictType(ctx context.Context, dictID int64) (*model.SysDictType, error) {
	dictType, err := repository.SelectDictTypeByID(ctx, dictID)
	if err != nil {
		return nil, err
	}
	if dictType == nil {
		return nil, errs.New("字典类型不存在")
	}
	return dictType, nil
}

// CreateDictType 新增字典类型。
func CreateDictType(ctx context.Context, dictType *model.SysDictType, operator string) error {
	count, err := repository.CountDictTypeByType(ctx, dictType.DictType, 0)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("新增字典'%s'失败，字典类型已存在", dictType.DictName)
	}

	dictType.DictID = 0
	if dictType.Status == "" {
		dictType.Status = model.StatusNormal
	}
	dictType.CreateBy = operator
	dictType.CreateTime = types.Now()
	return repository.InsertDictType(ctx, dictType)
}

// UpdateDictType 修改字典类型。
//
// 类型名改了要同步 sys_dict_data 里的引用，并把新旧两个 key 的缓存都清掉。
func UpdateDictType(ctx context.Context, dictType *model.SysDictType, operator string) error {
	if dictType.DictID == 0 {
		return errs.New("字典类型ID不能为空")
	}
	existing, err := repository.SelectDictTypeByID(ctx, dictType.DictID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("字典类型不存在")
	}

	count, err := repository.CountDictTypeByType(ctx, dictType.DictType, dictType.DictID)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("修改字典'%s'失败，字典类型已存在", dictType.DictName)
	}

	dictType.UpdateBy = operator
	dictType.UpdateTime = types.Now()
	if err := repository.UpdateDictType(ctx, dictType, existing.DictType); err != nil {
		return err
	}

	if existing.DictType != dictType.DictType {
		if err := ClearDictCache(ctx, existing.DictType); err != nil {
			return err
		}
	}
	return ClearDictCache(ctx, dictType.DictType)
}

// DeleteDictTypes 批量删除字典类型。
//
// 下面还有字典数据的类型不允许删除，与 Java 一致。
func DeleteDictTypes(ctx context.Context, dictIDs []int64) error {
	if len(dictIDs) == 0 {
		return errs.New("请选择要删除的字典类型")
	}
	var err error
	dictIDs, err = normalizeRelationIDs(dictIDs, "字典类型")
	if err != nil {
		return err
	}
	dictTypes, counts, err := repository.SelectDictTypesForDelete(ctx, dictIDs)
	if err != nil {
		return err
	}
	typeNames := make([]string, 0, len(dictTypes))
	for _, dictType := range dictTypes {
		if counts[dictType.DictType] > 0 {
			return errs.Newf("%s已分配,不能删除", dictType.DictName)
		}
		typeNames = append(typeNames, dictType.DictType)
	}

	if err := repository.DeleteDictTypeByIDs(ctx, dictIDs); err != nil {
		return err
	}
	for _, name := range typeNames {
		if err := ClearDictCache(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 字典数据 ----------

// ListDictDataPage 分页查询字典数据。
func ListDictDataPage(ctx context.Context, query model.DictDataQuery, pg page.Query) ([]model.SysDictData, int64, error) {
	list, total, err := repository.SelectDictDataPage(ctx, query, pg)
	if err != nil {
		return nil, 0, err
	}
	fillDictDefault(list)
	return list, total, nil
}

// ListDictDataExport 导出用的全量查询。
func ListDictDataExport(ctx context.Context, query model.DictDataQuery) ([]model.SysDictData, error) {
	list, err := repository.SelectDictDataList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	fillDictDefault(list)
	return list, nil
}

// GetDictData 按主键查字典数据。
func GetDictData(ctx context.Context, dictCode int64) (*model.SysDictData, error) {
	data, err := repository.SelectDictDataByCode(ctx, dictCode)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, errs.New("字典数据不存在")
	}
	data.Default = data.IsDefaultDict()
	return data, nil
}

// CreateDictData 新增字典数据。
func CreateDictData(ctx context.Context, data *model.SysDictData, operator string) error {
	data.DictCode = 0
	if data.Status == "" {
		data.Status = model.StatusNormal
	}
	if data.IsDefault == "" {
		data.IsDefault = "N"
	}
	data.CreateBy = operator
	data.CreateTime = types.Now()
	if err := repository.InsertDictData(ctx, data); err != nil {
		return err
	}
	return ClearDictCache(ctx, data.DictType)
}

// UpdateDictData 修改字典数据。
func UpdateDictData(ctx context.Context, data *model.SysDictData, operator string) error {
	if data.DictCode == 0 {
		return errs.New("字典数据ID不能为空")
	}
	existing, err := repository.SelectDictDataByCode(ctx, data.DictCode)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("字典数据不存在")
	}

	data.UpdateBy = operator
	data.UpdateTime = types.Now()
	if err := repository.UpdateDictData(ctx, data); err != nil {
		return err
	}

	// 字典类型可能被改到另一个类型下，两边缓存都要清
	if existing.DictType != data.DictType {
		if err := ClearDictCache(ctx, existing.DictType); err != nil {
			return err
		}
	}
	return ClearDictCache(ctx, data.DictType)
}

// DeleteDictData 批量删除字典数据。
func DeleteDictData(ctx context.Context, dictCodes []int64) error {
	if len(dictCodes) == 0 {
		return errs.New("请选择要删除的字典数据")
	}
	// 不要用 types 做变量名 —— 会遮蔽 pkg/types 包，本函数虽然用不到，
	// 但以后有人在这里加一行 types.Now() 就会莫名其妙编译不过
	var err error
	dictCodes, err = normalizeRelationIDs(dictCodes, "字典数据")
	if err != nil {
		return err
	}
	dataRows, err := repository.SelectDictDataByCodes(ctx, dictCodes)
	if err != nil {
		return err
	}
	affectedTypes := make(map[string]struct{})
	for _, data := range dataRows {
		affectedTypes[data.DictType] = struct{}{}
	}

	if err := repository.DeleteDictDataByCodes(ctx, dictCodes); err != nil {
		return err
	}
	for dictType := range affectedTypes {
		if err := ClearDictCache(ctx, dictType); err != nil {
			return err
		}
	}
	return nil
}
