package service

import (
	"context"
	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"time"
)

func (s PODRunService) Quote(ctx context.Context, user *model.SysUser, source model.PODQuoteRequest) (*model.PODQuoteView, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if s.Limits.Mode != "managed" {
		return nil, errs.WithCode(409, "当前没有启用正式模型执行")
	}
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	var items []model.PODBatchItem
	switch source.Kind {
	case "run":
		if source.Run == nil || source.Batch != nil || source.RunID != "" {
			return nil, errs.WithCode(400, "请提供且只提供 run 描述")
		}
		b := *source.Run
		if b.ExecutionMode != "managed" || b.TestScenario != "" {
			return nil, errs.WithCode(400, "正式报价不接受旧执行模式或测试场景")
		}
		if _, err := PODPositiveID(b.ProjectID); err != nil {
			return nil, err
		}
		if _, err := PODPositiveID(b.Revision); err != nil {
			return nil, err
		}
		if b.Mode != "workflow" && b.Mode != "node" && b.Mode != "branch" && b.Mode != "polish" {
			return nil, errs.WithCode(400, "执行范围无效")
		}
		if b.Mode == "workflow" && (b.NodeID != "" || b.BaseRunID != "") || b.Mode != "workflow" && b.NodeID == "" || (b.Mode == "polish" || b.Mode == "branch") && b.BaseRunID != "" {
			return nil, errs.WithCode(400, "节点目标或历史上游无效")
		}
		if b.BaseRunID != "" {
			if _, err := PODPositiveID(b.BaseRunID); err != nil {
				return nil, err
			}
		}
		source.Run = &b
	case "batch":
		if source.Batch == nil || source.Run != nil || source.RunID != "" {
			return nil, errs.WithCode(400, "请提供且只提供 batch 描述")
		}
		b := *source.Batch
		if b.RequestID == "" {
			b.RequestID = uuid.NewString()
		}
		svc := PODBatchService{Runs: s}
		var err error
		b, _, err = svc.normalize(b)
		if err != nil {
			return nil, err
		}
		items, _, _, err = svc.prepare(ctx, user, b)
		if err != nil {
			return nil, err
		}
		source.Batch = &b
	case "retry":
		if source.Run != nil || source.Batch != nil {
			return nil, errs.WithCode(400, "恢复报价只接受 runId")
		}
		if _, err := PODPositiveID(source.RunID); err != nil {
			return nil, err
		}
	default:
		return nil, errs.WithCode(400, "kind 须为 run、batch 或 retry")
	}
	view, err := repository.CreatePODQuote(ctx, user, source, items, s.Limits, s.GraphLimits)
	return view, podRunError(err)
}
func PODCreatorModels(ctx context.Context, user *model.SysUser, kind string) ([]model.PODCreatorModel, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if kind != "" && kind != "text" && kind != "image" {
		return nil, errs.WithCode(400, "模型类型须为 text 或 image")
	}
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	return repository.PODManagedModels(ctx, user.UserID, kind)
}

func PODCreatorQuote(ctx context.Context, user *model.SysUser, id string) (*model.PODQuoteView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	return repository.GetPODQuote(ctx, user.UserID, id)
}
