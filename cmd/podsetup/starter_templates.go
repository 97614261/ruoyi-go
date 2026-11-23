package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
)

func seedTemplates(ctx context.Context, cfg *config.Config) error {
	if err := bundleOffline(ctx, cfg); err != nil {
		return err
	}
	if err := repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	if err := repository.VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
		return err
	}
	if err := service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
		return err
	}
	admin := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1, UserName: "admin"}, Permissions: []string{"*:*:*"}}
	svc := service.NewPODOfficialService(cfg.POD.StorageRoot, model.PODGraphLimits{}, model.PODAssetLimits{})
	templates, err := workflow.StarterTemplates()
	if err != nil {
		return err
	}
	for _, seed := range templates {
		request := func(op string) string {
			return uuid.NewSHA1(uuid.NameSpaceOID, []byte(cfg.POD.InstanceID+":starter-v1:"+seed.Code+":"+op)).String()
		}
		created, err := svc.Write(ctx, admin, 0, "official.create", model.PODOfficialWrite{RequestID: request("create"), Reason: "安装正式入门模板 v1，不执行模型，不修改已有内容", Code: seed.Code, Name: seed.Name, Description: seed.Description, Sort: seed.Sort, Graph: &seed.Graph})
		if err != nil {
			return fmt.Errorf("创建入门模板 %s: %w", seed.Code, err)
		}
		if _, err := service.PODCatalogGetAction(ctx, admin, request("publish")); err == nil {
			fmt.Printf("保留已安装模板：%s\n", seed.Name)
			continue
		} else if e := errs.As(err); e == nil || e.Code != 404 {
			return err
		}
		detail, err := svc.Get(ctx, admin, created.ResourceID, 0, true)
		if err != nil {
			return err
		}
		if detail.Template.Revision != created.Revision || detail.Snapshot == nil {
			return fmt.Errorf("模板 %s 已被编辑，停止安装，不覆盖或重新发布", seed.Code)
		}
		_, err = svc.Write(ctx, admin, created.ResourceID, "official.publish", model.PODOfficialWrite{RequestID: request("publish"), Revision: strconv.FormatInt(created.Revision, 10), VersionID: strconv.FormatInt(detail.Snapshot.VersionID, 10), Reason: "发布正式入门模板 v1，用户复制后自主选择模型并确认执行"})
		if err != nil {
			return err
		}
		fmt.Printf("已发布入门模板：%s\n", seed.Name)
	}
	return nil
}
