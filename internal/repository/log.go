package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// ---------- 登录日志 ----------

func logininforFilter(db *gorm.DB, query model.LogininforQuery) *gorm.DB {
	if query.IPAddr != "" {
		db = db.Where("ipaddr LIKE ?", "%"+query.IPAddr+"%")
	}
	if query.UserName != "" {
		db = db.Where("user_name LIKE ?", "%"+query.UserName+"%")
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.BeginTime != "" {
		db = db.Where("date_format(login_time,'%Y%m%d') >= date_format(?,'%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("date_format(login_time,'%Y%m%d') <= date_format(?,'%Y%m%d')", query.EndTime)
	}
	return db
}

// SelectLogininforPage 分页查询登录日志。
func SelectLogininforPage(ctx context.Context, query model.LogininforQuery, pg page.Query) ([]model.SysLogininfor, int64, error) {
	db := logininforFilter(DB(ctx).Model(&model.SysLogininfor{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计登录日志总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysLogininfor{}, 0, nil
	}

	orderBy := pg.OrderBy
	if orderBy == "" {
		orderBy = "info_id DESC"
	}

	var list []model.SysLogininfor
	if err := db.Order(orderBy).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询登录日志失败: %w", err)
	}
	return list, total, nil
}

// SelectLogininforList 不分页查询，供导出使用。
func SelectLogininforList(ctx context.Context, query model.LogininforQuery) ([]model.SysLogininfor, error) {
	var list []model.SysLogininfor
	err := logininforFilter(DB(ctx).Model(&model.SysLogininfor{}), query).
		Order("info_id DESC").Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询登录日志失败: %w", err)
	}
	return list, nil
}

// InsertLogininfor 写入一条登录日志。
func InsertLogininfor(ctx context.Context, record *model.SysLogininfor) error {
	if err := DB(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("写入登录日志失败: %w", err)
	}
	return nil
}

// DeleteLogininforByIDs 批量删除登录日志。
func DeleteLogininforByIDs(ctx context.Context, infoIDs []int64) error {
	if len(infoIDs) == 0 {
		return nil
	}
	if err := DB(ctx).Where("info_id IN ?", infoIDs).Delete(&model.SysLogininfor{}).Error; err != nil {
		return fmt.Errorf("删除登录日志失败: %w", err)
	}
	return nil
}

// CleanLogininfor 清空登录日志。
//
// 用 DELETE 而不是 TRUNCATE：TRUNCATE 是 DDL，会隐式提交事务、
// 且在很多生产环境里权限受限。数据量大时由调用方承担耗时。
func CleanLogininfor(ctx context.Context) error {
	if err := DB(ctx).Where("1 = 1").Delete(&model.SysLogininfor{}).Error; err != nil {
		return fmt.Errorf("清空登录日志失败: %w", err)
	}
	return nil
}

// ---------- 操作日志 ----------

func operLogFilter(db *gorm.DB, query model.OperLogQuery) *gorm.DB {
	if query.Title != "" {
		db = db.Where("title LIKE ?", "%"+query.Title+"%")
	}
	if query.OperName != "" {
		db = db.Where("oper_name LIKE ?", "%"+query.OperName+"%")
	}
	if query.BusinessType != "" {
		db = db.Where("business_type = ?", query.BusinessType)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.BeginTime != "" {
		db = db.Where("date_format(oper_time,'%Y%m%d') >= date_format(?,'%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("date_format(oper_time,'%Y%m%d') <= date_format(?,'%Y%m%d')", query.EndTime)
	}
	return db
}

// SelectOperLogPage 分页查询操作日志。
func SelectOperLogPage(ctx context.Context, query model.OperLogQuery, pg page.Query) ([]model.SysOperLog, int64, error) {
	db := operLogFilter(DB(ctx).Model(&model.SysOperLog{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计操作日志总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysOperLog{}, 0, nil
	}

	orderBy := pg.OrderBy
	if orderBy == "" {
		orderBy = "oper_id DESC"
	}

	var list []model.SysOperLog
	if err := db.Order(orderBy).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询操作日志失败: %w", err)
	}
	return list, total, nil
}

// SelectOperLogList 不分页查询，供导出使用。
func SelectOperLogList(ctx context.Context, query model.OperLogQuery) ([]model.SysOperLog, error) {
	var list []model.SysOperLog
	err := operLogFilter(DB(ctx).Model(&model.SysOperLog{}), query).
		Order("oper_id DESC").Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询操作日志失败: %w", err)
	}
	return list, nil
}

// InsertOperLog 写入一条操作日志。
func InsertOperLog(ctx context.Context, record *model.SysOperLog) error {
	if err := DB(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("写入操作日志失败: %w", err)
	}
	return nil
}

// DeleteOperLogByIDs 批量删除操作日志。
func DeleteOperLogByIDs(ctx context.Context, operIDs []int64) error {
	if len(operIDs) == 0 {
		return nil
	}
	if err := DB(ctx).Where("oper_id IN ?", operIDs).Delete(&model.SysOperLog{}).Error; err != nil {
		return fmt.Errorf("删除操作日志失败: %w", err)
	}
	return nil
}

// CleanOperLog 清空操作日志。
func CleanOperLog(ctx context.Context) error {
	if err := DB(ctx).Where("1 = 1").Delete(&model.SysOperLog{}).Error; err != nil {
		return fmt.Errorf("清空操作日志失败: %w", err)
	}
	return nil
}
