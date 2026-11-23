package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/page"
)

func PODTaskFilter(begin, end, user, modelID, state string) (model.PODTaskFilter, error) {
	b, e, err := PODAdminDateRange(begin, end, time.Now())
	f := model.PODTaskFilter{Begin: b, End: e, State: state}
	if err != nil {
		return f, err
	}
	for _, field := range []struct {
		raw    string
		target *int64
	}{{user, &f.UserID}, {modelID, &f.ModelID}} {
		if field.raw != "" {
			id, err := PODPositiveID(field.raw)
			if err != nil {
				return f, err
			}
			*field.target = id
		}
	}
	if state != "" && state != "active" && state != "stopped" && state != "completed" && state != "queued" && state != "running" && state != "cancelRequested" && state != "succeeded" && state != "failed" && state != "cancelled" && state != "awaitingReconciliation" && state != "partiallySucceeded" {
		return f, errs.WithCode(400, "任务状态无效")
	}
	return f, nil
}
func PODTaskList(ctx context.Context, u *model.LoginUser, f model.PODTaskFilter, p page.Query, batch bool) (any, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:task:list"); err != nil {
		return nil, 0, err
	}
	if err := validatePODTaskFilter(f); err != nil {
		return nil, 0, err
	}
	if batch {
		return repository.PODAdminBatches(ctx, f, normalizePODPage(p))
	}
	return repository.PODAdminTasks(ctx, f, normalizePODPage(p))
}
func PODTaskGet(ctx context.Context, u *model.LoginUser, id int64, batch bool) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:task:query"); err != nil {
		return nil, err
	}
	if batch {
		row, items, err := repository.PODAdminBatch(ctx, id)
		return struct {
			Batch *model.PODAdminBatchView  `json:"batch"`
			Items []model.PODAdminBatchItem `json:"items"`
		}{row, items}, err
	}
	return repository.PODAdminTask(ctx, id)
}
func PODTaskNodes(ctx context.Context, u *model.LoginUser, id int64, p page.Query) ([]model.PODTaskNodeView, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:task:query"); err != nil {
		return nil, 0, err
	}
	return repository.PODAdminTaskNodes(ctx, id, normalizePODPage(p))
}
func PODAdminUsage(ctx context.Context, u *model.LoginUser, f model.PODTaskFilter, p page.Query) ([]model.PODUsageGroup, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:usage:query"); err != nil {
		return nil, 0, err
	}
	if err := validatePODTaskFilter(f); err != nil {
		return nil, 0, err
	}
	return repository.PODAdminUsage(ctx, f, normalizePODPage(p))
}

func validatePODTaskFilter(f model.PODTaskFilter) error {
	if f.Begin.IsZero() || f.End.IsZero() || !f.Begin.Before(f.End) || f.End.Sub(f.Begin) > 31*24*time.Hour || f.UserID < 0 || f.ModelID < 0 {
		return errs.WithCode(400, "查询需要最多 31 天的日期范围和有效编号")
	}
	return nil
}
func PODAdminGrants(ctx context.Context, u *model.LoginUser, p page.Query) ([]model.PODPrivateGrant, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:privacy:grant"); err != nil {
		return nil, 0, err
	}
	return repository.PODPrivateGrants(ctx, normalizePODPage(p))
}
func PODOperation(ctx context.Context, u *model.LoginUser, op string, id int64, r model.PODOperationWrite) (*model.PODCatalogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	perm := CatalogPermission(op)
	if perm == "" {
		return nil, errs.WithCode(400, "管理动作无效")
	}
	if err := requirePODAdmin(u, perm); err != nil {
		return nil, err
	}
	request, err := uuid.Parse(r.RequestID)
	if err != nil || request == uuid.Nil || request.String() != r.RequestID || !catalogText(r.Reason, 200) {
		return nil, errs.WithCode(400, "请求编号须为规范 UUID，操作原因须为 1—200 字")
	}
	in := model.PODOperationMutation{ActorID: u.UserID, ID: id, Op: op, Permission: perm, Trace: logx.TraceID(ctx), Request: r}
	switch op {
	case "private.grant":
		if r.Revision != "" || id != 0 {
			return nil, errs.WithCode(400, "新增授权参数无效")
		}
		in.Grant.OperatorUserID, err = PODPositiveID(r.OperatorUserID)
		if err != nil {
			return nil, err
		}
		in.Grant.OwnerUserID, err = PODPositiveID(r.OwnerUserID)
		if err != nil {
			return nil, err
		}
		in.Grant.ResourceID, err = PODPositiveID(r.ResourceID)
		if err != nil {
			return nil, err
		}
		if r.ResourceType != "run" && r.ResourceType != "asset" {
			return nil, errs.WithCode(400, "仅允许指定单个运行或素材")
		}
		in.Grant.ResourceType = r.ResourceType
		in.Grant.Purpose = r.Reason
		in.Grant.ExpiresAt, err = time.Parse(time.RFC3339, r.ExpiresAt)
		if err != nil || !in.Grant.ExpiresAt.After(time.Now()) || in.Grant.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
			return nil, errs.WithCode(400, "授权到期时间须在未来 24 小时内")
		}
	case "private.revoke", "task.cancel", "batch.stop":
		if id < 1 || r.OperatorUserID != "" || r.OwnerUserID != "" || r.ResourceType != "" || r.ResourceID != "" || r.ExpiresAt != "" {
			return nil, errs.WithCode(400, "操作参数无效")
		}
		if op == "private.revoke" {
			in.Revision, err = podRevision(r.Revision)
			if err != nil {
				return nil, err
			}
		} else if r.Revision != "" {
			return nil, errs.WithCode(400, "取消操作不接收版本字段")
		}
	}
	raw, _ := json.Marshal(struct {
		Op      string
		ID      int64
		Request model.PODOperationWrite
	}{op, id, r})
	in.Hash = fmt.Sprintf("%x", sha256.Sum256(raw))
	result, err := repository.PODOperationWrite(ctx, in)
	return result, podTemplateError(err)
}
func PODPrivateRun(ctx context.Context, u *model.LoginUser, id int64) (*model.PODPrivateRun, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:private:read"); err != nil {
		return nil, err
	}
	return repository.PODAdminPrivateRun(ctx, u.UserID, id, logx.TraceID(ctx))
}
func (s *PODAssetService) AdminOpen(ctx context.Context, u *model.LoginUser, id int64, variant string) (io.ReadCloser, *model.PODAssetFile, string, error) {
	if err := requirePODAdmin(u, "podAdmin:private:read"); err != nil {
		return nil, nil, "", err
	}
	if variant != "original" && variant != "thumbnail" {
		return nil, nil, "", errs.WithCode(400, "文件类型无效")
	}
	meta, name, err := repository.PODAdminPrivateAsset(ctx, u.UserID, id, variant, logx.TraceID(ctx))
	if err != nil {
		return nil, nil, "", err
	}
	f, err := s.Store.Open(ctx, meta.StorageKey)
	if err != nil {
		return nil, nil, "", errs.WithCode(404, "文件暂不可用")
	}
	if f.Size != meta.ByteSize {
		f.Close()
		return nil, nil, "", errs.WithCode(404, "文件暂不可用")
	}
	return f, meta, name, nil
}
