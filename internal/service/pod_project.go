package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func podProjectError(err error) error {
	switch {
	case errors.Is(err, repository.ErrPODWorkspace):
		return errs.WithCode(403, "工作空间已停用或没有访问权限")
	case errors.Is(err, repository.ErrPODProject):
		return errs.WithCode(404, "项目不存在或已移入回收站")
	case errors.Is(err, repository.ErrPODRevision):
		return errs.WithCode(409, "项目已在其他页面修改，请重新加载后再保存；本地修改仍保留")
	default:
		return err
	}
}
func podProjectUser(ctx context.Context, user *model.SysUser) (int64, error) {
	if user == nil || user.UserID <= 0 {
		return 0, errs.ErrUnauthorized
	}
	w, err := repository.PODPersonalWorkspace(ctx, user.UserID)
	if err != nil {
		return 0, err
	}
	if w == nil {
		return 0, podProjectError(repository.ErrPODWorkspace)
	}
	return user.UserID, nil
}
func PODProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", errs.WithCode(400, "项目名称须为 1 到 100 个字符，且不能包含控制字符")
	}
	return name, nil
}
func podRevision(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != value {
		return 0, errs.WithCode(400, "版本号必须是正整数的十进制字符串")
	}
	return n, nil
}
func podProjectView(row *model.PODProject, detail bool) (*model.PODProjectView, error) {
	result := &model.PODProjectView{ProjectID: row.ProjectID, WorkspaceID: row.WorkspaceID, Name: row.Name, Revision: row.Revision, Deleted: row.DelFlag == "2", CreateTime: row.CreateTime, UpdateTime: row.UpdateTime}
	if detail {
		var g model.PODGraph
		if err := json.Unmarshal(row.GraphJSON, &g); err != nil {
			return nil, err
		}
		result.Graph = &g
	}
	return result, nil
}
func InitPODWorkspace(ctx context.Context, user *model.SysUser) (*model.PODWorkspace, error) {
	if user == nil || user.UserID <= 0 {
		return nil, errs.ErrUnauthorized
	}
	name := user.NickName
	if strings.TrimSpace(name) == "" {
		name = user.UserName
	}
	w, err := repository.EnsurePODPersonalWorkspace(ctx, user.UserID, name+"的空间", user.UserName)
	return w, podProjectError(err)
}
func ListPODProjectPage(ctx context.Context, user *model.SysUser, deleted bool, pg page.Query) ([]model.PODProjectView, int64, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	if pg.PageNum < 1 {
		pg.PageNum = 1
	}
	if pg.PageSize < 1 {
		pg.PageSize = 10
	}
	if pg.PageSize > 100 {
		pg.PageSize = 100
	}
	rows, total, err := repository.SelectPODProjectPage(ctx, userID, deleted, pg)
	if err != nil {
		return nil, 0, podProjectError(err)
	}
	result := make([]model.PODProjectView, 0, len(rows))
	for i := range rows {
		v, _ := podProjectView(&rows[i], false)
		result = append(result, *v)
	}
	return result, total, nil
}
func GetPODProject(ctx context.Context, user *model.SysUser, id int64) (*model.PODProjectView, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	row, err := repository.SelectPODProjectByID(ctx, userID, id)
	if err != nil {
		return nil, podProjectError(err)
	}
	return podProjectView(row, true)
}
func CreatePODProject(ctx context.Context, user *model.SysUser, body model.PODProjectCreate) (*model.PODProjectView, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	name, err := PODProjectName(body.Name)
	if err != nil {
		return nil, err
	}
	graph, _ := json.Marshal(model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}})
	row := &model.PODProject{Name: name, GraphJSON: graph, SchemaVersion: 1, Revision: 1, DelFlag: "0", CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: types.Now(), UpdateTime: types.Now()}
	if err := repository.InsertPODProject(ctx, userID, row); err != nil {
		return nil, podProjectError(err)
	}
	return podProjectView(row, true)
}
func EditPODProject(ctx context.Context, user *model.SysUser, id int64, body model.PODProjectUpdate, limits model.PODGraphLimits) (*model.PODProjectView, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	revision, err := podRevision(body.Revision)
	if err != nil {
		return nil, err
	}
	if body.Name == nil && body.Graph == nil {
		return nil, errs.WithCode(400, "请提供名称或画布内容")
	}
	updates := map[string]any{"update_by": user.UserName}
	var refs *[]model.PODProjectAssetRef
	if body.Name != nil {
		name, err := PODProjectName(*body.Name)
		if err != nil {
			return nil, err
		}
		updates["name"] = name
	}
	if body.Graph != nil {
		if err := workflow.ValidateWithLimits(*body.Graph, limits); err != nil {
			return nil, errs.WithCode(400, err.Error())
		}
		graph, err := json.Marshal(body.Graph)
		if err != nil {
			return nil, errs.WithCode(400, "画布格式不合法")
		}
		updates["graph_json"] = string(graph)
		updates["schema_version"] = body.Graph.SchemaVersion
		items := []model.PODProjectAssetRef{}
		for _, node := range body.Graph.Nodes {
			if node.Type == "imageInput" {
				var config struct {
					AssetIDs []string `json:"assetIds"`
				}
				if err := json.Unmarshal(node.Config, &config); err != nil {
					return nil, errs.WithCode(400, "素材配置格式不合法")
				}
				for _, value := range config.AssetIDs {
					id, _ := strconv.ParseInt(value, 10, 64)
					items = append(items, model.PODProjectAssetRef{NodeID: node.ID, AssetID: id})
				}
			}
		}
		refs = &items
	}
	row, err := repository.UpdatePODProject(ctx, userID, id, revision, "0", updates, refs)
	if err != nil {
		return nil, podAssetError(err)
	}
	return podProjectView(row, true)
}
func ChangePODProjectDeletion(ctx context.Context, user *model.SysUser, id int64, value string, restore bool) (*model.PODProjectView, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	revision, err := podRevision(value)
	if err != nil {
		return nil, err
	}
	from, to := "0", "2"
	if restore {
		from, to = to, from
	}
	row, err := repository.UpdatePODProject(ctx, userID, id, revision, from, map[string]any{"del_flag": to, "update_by": user.UserName}, nil)
	if err != nil {
		return nil, podProjectError(err)
	}
	return podProjectView(row, false)
}
