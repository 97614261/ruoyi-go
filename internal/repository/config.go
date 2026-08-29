package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// SelectConfigValueByKey 按参数键取值，不存在时返回空串。
//
// 注意 Pluck 的目标必须是切片，不能直接给 *string。
func SelectConfigValueByKey(ctx context.Context, configKey string) (string, error) {
	var values []string
	err := DB(ctx).
		Table("sys_config").
		Where("config_key = ?", configKey).
		Limit(1).
		Pluck("config_value", &values).Error
	if err != nil {
		return "", fmt.Errorf("查询参数 %s 失败: %w", configKey, err)
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

func configFilter(db *gorm.DB, query model.ConfigQuery) *gorm.DB {
	if query.ConfigName != "" {
		db = db.Where("config_name LIKE ?", "%"+query.ConfigName+"%")
	}
	if query.ConfigKey != "" {
		db = db.Where("config_key LIKE ?", "%"+query.ConfigKey+"%")
	}
	if query.ConfigType != "" {
		db = db.Where("config_type = ?", query.ConfigType)
	}
	if query.BeginTime != "" {
		db = db.Where("date_format(create_time,'%Y%m%d') >= date_format(?,'%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("date_format(create_time,'%Y%m%d') <= date_format(?,'%Y%m%d')", query.EndTime)
	}
	return db
}

// SelectConfigPage 分页查询参数。
func SelectConfigPage(ctx context.Context, query model.ConfigQuery, pg page.Query) ([]model.SysConfig, int64, error) {
	db := configFilter(DB(ctx).Model(&model.SysConfig{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计参数总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysConfig{}, 0, nil
	}

	// 兜底主键，保证翻页行序确定，见 page.Query.Stable
	orderBy := pg.Stable("config_id", "config_id")

	var list []model.SysConfig
	err := db.Order(orderBy).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询参数列表失败: %w", err)
	}
	return list, total, nil
}

// SelectConfigList 不分页查询，供导出使用。
func SelectConfigList(ctx context.Context, query model.ConfigQuery, limit ...int) ([]model.SysConfig, error) {
	var list []model.SysConfig
	err := applyOptionalLimit(configFilter(DB(ctx).Model(&model.SysConfig{}), query), limit).
		Order("config_id").Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询参数列表失败: %w", err)
	}
	return list, nil
}

// SelectConfigByID 按 ID 查参数，不存在返回 (nil, nil)。
func SelectConfigByID(ctx context.Context, configID int64) (*model.SysConfig, error) {
	var config model.SysConfig
	err := DB(ctx).Where("config_id = ?", configID).Take(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询参数 %d 失败: %w", configID, err)
	}
	return &config, nil
}

// CountConfigByKey 同键名参数数量，excludeID 用于修改时排除自身。
func CountConfigByKey(ctx context.Context, configKey string, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysConfig{}).Where("config_key = ?", configKey)
	if excludeID > 0 {
		db = db.Where("config_id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验参数键名唯一性失败: %w", err)
	}
	return count, nil
}

// InsertConfig 新增参数。
func InsertConfig(ctx context.Context, config *model.SysConfig) error {
	if err := DB(ctx).Create(config).Error; err != nil {
		return fmt.Errorf("新增参数失败: %w", err)
	}
	return nil
}

// UpdateConfig 更新参数。
func UpdateConfig(ctx context.Context, config *model.SysConfig) error {
	updates := map[string]any{
		"config_name":  config.ConfigName,
		"config_key":   config.ConfigKey,
		"config_value": config.ConfigValue,
		"config_type":  config.ConfigType,
		"update_by":    config.UpdateBy,
		"update_time":  config.UpdateTime,
	}
	if config.Remark != nil {
		updates["remark"] = *config.Remark
	}
	result := DB(ctx).Model(&model.SysConfig{}).
		Where("config_id = ?", config.ConfigID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("更新参数 %d 失败: %w", config.ConfigID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("更新参数 %d 失败: 参数不存在", config.ConfigID)
	}
	return nil
}

// DeleteConfigByIDs 批量删除参数（物理删除，sys_config 没有 del_flag）。
func DeleteConfigByIDs(ctx context.Context, configIDs []int64) error {
	if len(configIDs) == 0 {
		return nil
	}
	err := DB(ctx).Where("config_id IN ?", configIDs).Delete(&model.SysConfig{}).Error
	if err != nil {
		return fmt.Errorf("删除参数失败: %w", err)
	}
	return nil
}
