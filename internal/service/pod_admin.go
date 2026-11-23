package service

import (
	"context"
	"encoding/json"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func requirePODAdmin(user *model.LoginUser, perm string) error {
	if user == nil || user.User == nil || user.UserID <= 0 {
		return errs.ErrUnauthorized
	}
	if !user.HasPermission(model.PODAdminAccess) || !user.HasPermission(perm) {
		return errs.WithCode(403, "没有 POD 管理权限，请使用创作端或联系管理员")
	}
	return nil
}

// Audit successful entry and authenticated denials synchronously. Audit failure
// denies entry; it never falls back to an unrecorded management session.
func CheckPODAdminEntry(ctx context.Context, user *model.LoginUser, recordEntry bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := requirePODAdmin(user, model.PODAdminAccess)
	if user != nil && user.UserID > 0 && (recordEntry || err != nil) {
		snapshot := model.PODAdminAuditSnapshot{}
		if err == nil {
			snapshot.SchemaVersion = "012"
		}
		if auditErr := repository.PODRecordAdminAccess(ctx, user.UserID, err == nil, logx.TraceID(ctx), snapshot); auditErr != nil {
			return errs.WithCode(500, "管理访问审计暂不可用，请稍后重试")
		}
	}
	return err
}

func PODAdminBootstrap(ctx context.Context, user *model.LoginUser) (*model.PODAdminBootstrap, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, model.PODAdminAccess); err != nil {
		return nil, err
	}
	state, err := repository.PODAdminState(ctx)
	if err != nil {
		return nil, err
	}
	permissions := []string{}
	for _, perm := range model.PODAdminPermissions {
		if user.HasPermission(perm) {
			permissions = append(permissions, perm)
		}
	}
	return &model.PODAdminBootstrap{UserID: user.UserID, UserName: user.User.UserName, NickName: user.User.NickName, Permissions: permissions, SchemaVersion: "012", Stage: 7, ManagedEnabled: state.ManagedEnabled, Capabilities: model.PODAdminCapabilities{Overview: user.HasPermission(model.PODAdminOverviewQuery), Audits: user.HasPermission(model.PODAdminAuditList), Creators: user.HasPermission("podAdmin:creator:list"), Models: user.HasPermission("podAdmin:model:list") || user.HasPermission("podAdmin:provider:list"), Tasks: user.HasPermission("podAdmin:task:list"), Usage: user.HasPermission("podAdmin:usage:query"), Templates: user.HasPermission("podAdmin:template:list"), PrivateContent: user.HasPermission("podAdmin:private:read")}, CatalogWrites: PODCatalogReady()}, nil
}

func PODAdminDateRange(beginDate, endDate string, now time.Time) (time.Time, time.Time, error) {
	today := now.In(types.Location).Format("2006-01-02")
	if endDate == "" {
		endDate = today
	}
	end, err := time.ParseInLocation("2006-01-02", endDate, types.Location)
	if err != nil || endDate != end.Format("2006-01-02") {
		return time.Time{}, time.Time{}, errs.WithCode(400, "结束日期格式应为 YYYY-MM-DD")
	}
	if beginDate == "" {
		beginDate = end.AddDate(0, 0, -6).Format("2006-01-02")
	}
	begin, err := time.ParseInLocation("2006-01-02", beginDate, types.Location)
	if err != nil || beginDate != begin.Format("2006-01-02") {
		return time.Time{}, time.Time{}, errs.WithCode(400, "开始日期格式应为 YYYY-MM-DD")
	}
	until := end.AddDate(0, 0, 1)
	if begin.After(end) || until.Sub(begin) > 31*24*time.Hour || endDate > today {
		return time.Time{}, time.Time{}, errs.WithCode(400, "请选择不晚于今天、最多 31 天的日期范围")
	}
	return begin, until, nil
}

func PODAdminOverview(ctx context.Context, user *model.LoginUser, beginDate, endDate string) (*model.PODAdminOverview, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, model.PODAdminOverviewQuery); err != nil {
		return nil, err
	}
	begin, end, err := PODAdminDateRange(beginDate, endDate, time.Now())
	if err != nil {
		return nil, err
	}
	view, err := repository.PODAdminOverview(ctx, begin, end)
	if err != nil {
		return nil, err
	}
	state, err := repository.PODAdminState(ctx)
	if err != nil {
		return nil, err
	}
	view.BeginDate = begin.Format("2006-01-02")
	view.EndDate = end.AddDate(0, 0, -1).Format("2006-01-02")
	view.ManagedEnabled = state.ManagedEnabled
	return view, nil
}

func podAdminAuditView(row model.PODAdminAudit, detail bool) model.PODAdminAuditView {
	view := model.PODAdminAuditView{AuditID: row.AuditID, ActorUserID: row.ActorUserID, OwnerUserID: row.OwnerUserID, ActionType: row.ActionType, ResourceType: row.ResourceType, ResourceID: row.ResourceID, Reason: row.Reason, TraceID: row.TraceID, Outcome: row.Outcome, CreateTime: row.CreateTime}
	if detail {
		var before, after model.PODAdminAuditSnapshot
		if json.Unmarshal(row.BeforeJSON, &before) == nil {
			view.Before = &before
		}
		if json.Unmarshal(row.AfterJSON, &after) == nil {
			view.After = &after
		}
	}
	return view
}

func PODAdminAuditList(ctx context.Context, user *model.LoginUser, f model.PODAdminAuditFilter, p page.Query) ([]model.PODAdminAuditView, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, model.PODAdminAuditList); err != nil {
		return nil, 0, err
	}
	if f.Begin.IsZero() || f.End.IsZero() || !f.Begin.Before(f.End) || f.End.Sub(f.Begin) > 31*24*time.Hour {
		return nil, 0, errs.WithCode(400, "审计查询需要最多 31 天的日期范围")
	}
	if f.Outcome != "" && f.Outcome != "success" && f.Outcome != "denied" && f.Outcome != "failure" {
		return nil, 0, errs.WithCode(400, "审计结果无效")
	}
	if f.ActionType != "" && f.ActionType != "admin.access" && CatalogPermission(f.ActionType) == "" {
		return nil, 0, errs.WithCode(400, "审计动作无效")
	}
	rows, total, err := repository.PODAdminAuditPage(ctx, f, normalizePODPage(p))
	if err != nil {
		return nil, 0, err
	}
	views := make([]model.PODAdminAuditView, 0, len(rows))
	for _, row := range rows {
		views = append(views, podAdminAuditView(row, false))
	}
	return views, total, nil
}

func PODAdminAuditGet(ctx context.Context, user *model.LoginUser, id int64) (*model.PODAdminAuditView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, model.PODAdminAuditQuery); err != nil {
		return nil, err
	}
	row, err := repository.PODAdminAuditGet(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errs.WithCode(404, "审计记录不存在")
	}
	view := podAdminAuditView(*row, true)
	return &view, nil
}
