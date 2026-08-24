package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// ---------- 字典数据 ----------

// SelectDictDataByType 按字典类型查启用的字典数据，按 dict_sort 升序。
func SelectDictDataByType(ctx context.Context, dictType string) ([]model.SysDictData, error) {
	var list []model.SysDictData
	err := DB(ctx).
		Where("status = ?", model.StatusNormal).
		Where("dict_type = ?", dictType).
		Order("dict_sort ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询字典 %s 失败: %w", dictType, err)
	}
	return list, nil
}

func dictDataFilter(db *gorm.DB, query model.DictDataQuery) *gorm.DB {
	if query.DictType != "" {
		db = db.Where("dict_type = ?", query.DictType)
	}
	if query.DictLabel != "" {
		db = db.Where("dict_label LIKE ?", "%"+query.DictLabel+"%")
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	return db
}

// SelectDictDataPage 分页查询字典数据。
func SelectDictDataPage(ctx context.Context, query model.DictDataQuery, pg page.Query) ([]model.SysDictData, int64, error) {
	db := dictDataFilter(DB(ctx).Model(&model.SysDictData{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计字典数据总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysDictData{}, 0, nil
	}

	orderBy := pg.OrderBy
	if orderBy == "" {
		orderBy = "dict_sort"
	}

	var list []model.SysDictData
	if err := db.Order(orderBy).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询字典数据失败: %w", err)
	}
	return list, total, nil
}

// SelectDictDataList 不分页查询，供导出使用。
func SelectDictDataList(ctx context.Context, query model.DictDataQuery) ([]model.SysDictData, error) {
	var list []model.SysDictData
	err := dictDataFilter(DB(ctx).Model(&model.SysDictData{}), query).
		Order("dict_sort").Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询字典数据失败: %w", err)
	}
	return list, nil
}

// SelectDictDataByCode 按主键查字典数据。
func SelectDictDataByCode(ctx context.Context, dictCode int64) (*model.SysDictData, error) {
	var data model.SysDictData
	err := DB(ctx).Where("dict_code = ?", dictCode).Take(&data).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询字典数据 %d 失败: %w", dictCode, err)
	}
	return &data, nil
}

// CountDictDataByType 统计某个字典类型下的数据条数，删除类型前检查用。
func CountDictDataByType(ctx context.Context, dictType string) (int64, error) {
	var count int64
	err := DB(ctx).Model(&model.SysDictData{}).Where("dict_type = ?", dictType).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计字典 %s 的数据失败: %w", dictType, err)
	}
	return count, nil
}

// InsertDictData 新增字典数据。
func InsertDictData(ctx context.Context, data *model.SysDictData) error {
	if err := DB(ctx).Create(data).Error; err != nil {
		return fmt.Errorf("新增字典数据失败: %w", err)
	}
	return nil
}

// UpdateDictData 更新字典数据。
func UpdateDictData(ctx context.Context, data *model.SysDictData) error {
	updates := map[string]any{
		"dict_sort":   model.IntValue(data.DictSort),
		"dict_label":  data.DictLabel,
		"dict_value":  data.DictValue,
		"dict_type":   data.DictType,
		"css_class":   data.CssClass,
		"list_class":  data.ListClass,
		"is_default":  data.IsDefault,
		"status":      data.Status,
		"update_by":   data.UpdateBy,
		"update_time": data.UpdateTime,
	}
	if data.Remark != nil {
		updates["remark"] = *data.Remark
	}
	err := DB(ctx).Model(&model.SysDictData{}).
		Where("dict_code = ?", data.DictCode).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("更新字典数据 %d 失败: %w", data.DictCode, err)
	}
	return nil
}

// DeleteDictDataByCodes 批量删除字典数据。
func DeleteDictDataByCodes(ctx context.Context, dictCodes []int64) error {
	if len(dictCodes) == 0 {
		return nil
	}
	err := DB(ctx).Where("dict_code IN ?", dictCodes).Delete(&model.SysDictData{}).Error
	if err != nil {
		return fmt.Errorf("删除字典数据失败: %w", err)
	}
	return nil
}

// UpdateDictDataType 字典类型改名后，同步改字典数据里的引用。
func UpdateDictDataType(ctx context.Context, oldType, newType string) error {
	if oldType == newType {
		return nil
	}
	err := DB(ctx).Model(&model.SysDictData{}).
		Where("dict_type = ?", oldType).
		Update("dict_type", newType).Error
	if err != nil {
		return fmt.Errorf("同步字典数据类型失败: %w", err)
	}
	return nil
}

// ---------- 字典类型 ----------

func dictTypeFilter(db *gorm.DB, query model.DictTypeQuery) *gorm.DB {
	if query.DictName != "" {
		db = db.Where("dict_name LIKE ?", "%"+query.DictName+"%")
	}
	if query.DictType != "" {
		db = db.Where("dict_type LIKE ?", "%"+query.DictType+"%")
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.BeginTime != "" {
		db = db.Where("date_format(create_time,'%Y%m%d') >= date_format(?,'%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("date_format(create_time,'%Y%m%d') <= date_format(?,'%Y%m%d')", query.EndTime)
	}
	return db
}

// SelectDictTypePage 分页查询字典类型。
func SelectDictTypePage(ctx context.Context, query model.DictTypeQuery, pg page.Query) ([]model.SysDictType, int64, error) {
	db := dictTypeFilter(DB(ctx).Model(&model.SysDictType{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计字典类型总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysDictType{}, 0, nil
	}

	orderBy := pg.OrderBy
	if orderBy == "" {
		orderBy = "dict_id"
	}

	var list []model.SysDictType
	if err := db.Order(orderBy).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询字典类型失败: %w", err)
	}
	return list, total, nil
}

// SelectDictTypeList 不分页查询，供导出使用。
func SelectDictTypeList(ctx context.Context, query model.DictTypeQuery) ([]model.SysDictType, error) {
	var list []model.SysDictType
	err := dictTypeFilter(DB(ctx).Model(&model.SysDictType{}), query).
		Order("dict_id").Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询字典类型失败: %w", err)
	}
	return list, nil
}

// SelectDictTypeAll 查全部字典类型，供下拉选择。
func SelectDictTypeAll(ctx context.Context) ([]model.SysDictType, error) {
	var list []model.SysDictType
	if err := DB(ctx).Order("dict_id").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询全部字典类型失败: %w", err)
	}
	return list, nil
}

// SelectDictTypeByID 按主键查字典类型。
func SelectDictTypeByID(ctx context.Context, dictID int64) (*model.SysDictType, error) {
	var dictType model.SysDictType
	err := DB(ctx).Where("dict_id = ?", dictID).Take(&dictType).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询字典类型 %d 失败: %w", dictID, err)
	}
	return &dictType, nil
}

// CountDictTypeByType 同字典类型的数量，excludeID 用于修改时排除自身。
func CountDictTypeByType(ctx context.Context, dictType string, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysDictType{}).Where("dict_type = ?", dictType)
	if excludeID > 0 {
		db = db.Where("dict_id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验字典类型唯一性失败: %w", err)
	}
	return count, nil
}

// InsertDictType 新增字典类型。
func InsertDictType(ctx context.Context, dictType *model.SysDictType) error {
	if err := DB(ctx).Create(dictType).Error; err != nil {
		return fmt.Errorf("新增字典类型失败: %w", err)
	}
	return nil
}

// UpdateDictType 更新字典类型，并同步字典数据里的类型引用。
//
// 两步必须同一事务：类型改了名而数据没跟着改，那批字典数据就成了孤儿。
func UpdateDictType(ctx context.Context, dictType *model.SysDictType, oldType string) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if oldType != dictType.DictType {
			if err := tx.Model(&model.SysDictData{}).
				Where("dict_type = ?", oldType).
				Update("dict_type", dictType.DictType).Error; err != nil {
				return fmt.Errorf("同步字典数据类型失败: %w", err)
			}
		}

		updates := map[string]any{
			"dict_name":   dictType.DictName,
			"dict_type":   dictType.DictType,
			"status":      dictType.Status,
			"update_by":   dictType.UpdateBy,
			"update_time": dictType.UpdateTime,
		}
		if dictType.Remark != nil {
			updates["remark"] = *dictType.Remark
		}
		if err := tx.Model(&model.SysDictType{}).
			Where("dict_id = ?", dictType.DictID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("更新字典类型 %d 失败: %w", dictType.DictID, err)
		}
		return nil
	})
}

// DeleteDictTypeByIDs 批量删除字典类型。
func DeleteDictTypeByIDs(ctx context.Context, dictIDs []int64) error {
	if len(dictIDs) == 0 {
		return nil
	}
	err := DB(ctx).Where("dict_id IN ?", dictIDs).Delete(&model.SysDictType{}).Error
	if err != nil {
		return fmt.Errorf("删除字典类型失败: %w", err)
	}
	return nil
}
