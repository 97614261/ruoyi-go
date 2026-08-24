package service

import (
	"context"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

// ListJobLogPage 分页查询调度日志。
func ListJobLogPage(ctx context.Context, query model.JobLogQuery, pg page.Query) ([]model.SysJobLog, int64, error) {
	return repository.SelectJobLogPage(ctx, query, pg)
}

// ListJobLogExport 导出用的全量查询。
func ListJobLogExport(ctx context.Context, query model.JobLogQuery) ([]model.SysJobLog, error) {
	list, err := repository.SelectJobLogList(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// GetJobLog 查日志详情。
func GetJobLog(ctx context.Context, jobLogID int64) (*model.SysJobLog, error) {
	record, err := repository.SelectJobLogByID(ctx, jobLogID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errs.New("调度日志不存在")
	}
	return record, nil
}

// DeleteJobLogs 删除若干条日志。
func DeleteJobLogs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return errs.New("请选择要删除的日志")
	}
	return repository.DeleteJobLogByIDs(ctx, ids)
}

// CleanJobLog 清空调度日志。
func CleanJobLog(ctx context.Context) error {
	return repository.CleanJobLog(ctx)
}
