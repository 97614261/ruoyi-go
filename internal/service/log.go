package service

import (
	"context"
	"log/slog"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
)

// ---------- 登录日志 ----------

// ListLogininforPage 分页查询登录日志。
func ListLogininforPage(ctx context.Context, query model.LogininforQuery, pg page.Query) ([]model.SysLogininfor, int64, error) {
	return repository.SelectLogininforPage(ctx, query, pg)
}

// ListLogininforExport 导出用的全量查询。
func ListLogininforExport(ctx context.Context, query model.LogininforQuery) ([]model.SysLogininfor, error) {
	list, err := repository.SelectLogininforList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// RecordLogininfor 记录一次登录尝试。
//
// 失败不影响登录主流程 —— 日志写不进去也不该阻止用户登录，
// 所以这里吞掉错误只记 warn。
func RecordLogininfor(ctx context.Context, userName, ip, browser, os, status, msg string) {
	record := &model.SysLogininfor{
		UserName:  userName,
		IPAddr:    ip,
		Browser:   browser,
		OS:        os,
		Status:    status,
		Msg:       msg,
		LoginTime: types.Now(),
	}
	if err := repository.InsertLogininfor(ctx, record); err != nil {
		slog.Warn("记录登录日志失败", "userName", userName, "err", err)
	}
}

// DeleteLogininfor 批量删除登录日志。
func DeleteLogininfor(ctx context.Context, infoIDs []int64) error {
	if len(infoIDs) == 0 {
		return errs.New("请选择要删除的记录")
	}
	return repository.DeleteLogininforByIDs(ctx, infoIDs)
}

// CleanLogininfor 清空登录日志。
func CleanLogininfor(ctx context.Context) error {
	return repository.CleanLogininfor(ctx)
}

// UnlockAccount 解锁账号，清掉密码错误计数。
func UnlockAccount(ctx context.Context, userName string) error {
	if userName == "" {
		return errs.New("用户账号不能为空")
	}
	// 与登录使用同一套规范账号规则：在大小写不敏感的数据库中，
	// 用 ADMIN 发起解锁也要删掉实际账号 admin 对应的计数键。
	user, err := repository.SelectUserAccountByUserName(ctx, userName)
	if err != nil {
		return err
	}
	if user != nil {
		userName = user.UserName
	}
	if err := redisx.C().Del(ctx, redisx.PwdErrCntKey(userName)).Err(); err != nil {
		return errs.Wrap(err, "解锁账号失败")
	}
	return nil
}

// ---------- 操作日志 ----------

// ListOperLogPage 分页查询操作日志。
func ListOperLogPage(ctx context.Context, query model.OperLogQuery, pg page.Query) ([]model.SysOperLog, int64, error) {
	return repository.SelectOperLogPage(ctx, query, pg)
}

// ListOperLogExport 导出用的全量查询。
func ListOperLogExport(ctx context.Context, query model.OperLogQuery) ([]model.SysOperLog, error) {
	list, err := repository.SelectOperLogList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// RecordOperLog 写入一条操作日志。
//
// 由 middleware.OperLog 异步调用，失败只向上返回让调用方记 warn，
// 不能影响已经完成的业务操作。
func RecordOperLog(ctx context.Context, record *model.SysOperLog) error {
	return repository.InsertOperLog(ctx, record)
}

// DeleteOperLog 批量删除操作日志。
func DeleteOperLog(ctx context.Context, operIDs []int64) error {
	if len(operIDs) == 0 {
		return errs.New("请选择要删除的记录")
	}
	return repository.DeleteOperLogByIDs(ctx, operIDs)
}

// CleanOperLog 清空操作日志。
func CleanOperLog(ctx context.Context) error {
	return repository.CleanOperLog(ctx)
}
