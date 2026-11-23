package service

import (
	"context"
	"strconv"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
)

func PODBootstrap(ctx context.Context, user *model.SysUser, limits model.PODGraphLimits) (*model.PODBootstrap, error) {
	if user == nil || user.UserID <= 0 {
		return nil, errs.ErrUnauthorized
	}
	workspace, err := repository.PODPersonalWorkspace(ctx, user.UserID)
	if err != nil {
		return nil, err
	}
	if workspace == nil {
		return nil, errs.WithCode(403, "尚未开通个人工作空间，请联系管理员")
	}
	return &model.PODBootstrap{
		SchemaVersion: model.PODGraphVersion, Workspace: *workspace,
		UserID: strconv.FormatInt(user.UserID, 10), UserName: user.UserName, NickName: user.NickName,
		NodeTypes: workflow.Definitions(), Capabilities: model.PODCapabilities{ProjectEditing: true, Assets: true}, Limits: workflow.DefaultLimits(limits),
	}, nil
}
