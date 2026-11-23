package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

type PODTemplateService struct{ Limits model.PODGraphLimits }

func podTemplateError(err error) error {
	switch {
	case errors.Is(err, repository.ErrPODTemplate):
		return errs.WithCode(404, "模板或版本不存在，或没有访问权限")
	case errors.Is(err, repository.ErrPODTemplateRequest):
		return errs.WithCode(409, "该请求标识已用于其他操作，请核对原操作结果")
	case errors.Is(err, repository.ErrPODRevision):
		return errs.WithCode(409, "模板或来源项目已修改，请重新加载后再保存")
	default:
		return podAssetError(err)
	}
}

func podTemplateName(value string) (string, error) {
	name, err := PODProjectName(value)
	if err != nil {
		return "", errs.WithCode(400, "模板名称须为 1 到 100 个字符，且不能包含控制字符")
	}
	return name, nil
}

func podTemplateDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > 500 || strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
		return "", errs.WithCode(400, "模板说明最多 500 个字符，不能包含非法控制字符")
	}
	return value, nil
}

func podTemplatePage(pg page.Query) page.Query {
	if pg.PageNum < 1 {
		pg.PageNum = 1
	}
	if pg.PageSize < 1 {
		pg.PageSize = 12
	}
	if pg.PageSize > 100 {
		pg.PageSize = 100
	}
	return pg
}

func podTemplateView(row *model.PODTemplate) *model.PODTemplateView {
	return &model.PODTemplateView{TemplateID: row.TemplateID, Name: row.Name, Description: row.Description, CurrentVersion: row.CurrentVersion, Revision: row.Revision, Deleted: row.DelFlag == "2", CreateTime: row.CreateTime, UpdateTime: row.UpdateTime}
}

func podTemplateSnapshot(row *model.PODTemplateVersion) (*model.PODTemplateVersionView, error) {
	var graph model.PODGraph
	if err := json.Unmarshal(row.GraphJSON, &graph); err != nil {
		return nil, err
	}
	slots, outputs := podTemplateSlots(graph)
	return &model.PODTemplateVersionView{TemplateID: row.TemplateID, Version: row.Version, CreateTime: row.CreateTime, Graph: &graph, Slots: slots, Outputs: outputs}, nil
}

func podTemplateSlots(graph model.PODGraph) ([]model.PODTemplateSlot, []model.PODTemplateOutput) {
	slots := []model.PODTemplateSlot{}
	outputs := []model.PODTemplateOutput{}
	labels := map[string]string{}
	for _, d := range workflow.Definitions() {
		labels[d.Type] = d.Label
	}
	outgoing := map[string]bool{}
	for _, e := range graph.Edges {
		outgoing[e.Source] = true
	}
	for _, n := range graph.Nodes {
		var c struct {
			Title    string   `json:"title"`
			Text     string   `json:"text"`
			AssetIDs []string `json:"assetIds"`
		}
		_ = json.Unmarshal(n.Config, &c)
		label := strings.TrimSpace(c.Title)
		if label == "" {
			label = labels[n.Type]
		}
		switch n.Type {
		case "textInput":
			text := c.Text
			slots = append(slots, model.PODTemplateSlot{NodeID: n.ID, Type: n.Type, Label: label, Text: &text})
		case "imageInput":
			slots = append(slots, model.PODTemplateSlot{NodeID: n.ID, Type: n.Type, Label: label, AssetIDs: c.AssetIDs})
		}
		if !outgoing[n.ID] {
			outputs = append(outputs, model.PODTemplateOutput{NodeID: n.ID, Type: n.Type, Label: label})
		}
	}
	return slots, outputs
}

func podTemplateRefs(g model.PODGraph) []model.PODProjectAssetRef {
	refs := []model.PODProjectAssetRef{}
	for _, n := range g.Nodes {
		if n.Type == "imageInput" {
			var c struct {
				AssetIDs []string `json:"assetIds"`
			}
			_ = json.Unmarshal(n.Config, &c)
			for _, text := range c.AssetIDs {
				id, _ := strconv.ParseInt(text, 10, 64)
				refs = append(refs, model.PODProjectAssetRef{NodeID: n.ID, AssetID: id})
			}
		}
	}
	return refs
}

func podTemplateAction(request, kind string, target int64, body any, operator string) (model.PODTemplateAction, error) {
	id, err := uuid.Parse(request)
	if err != nil || id.String() != request {
		return model.PODTemplateAction{}, errs.WithCode(400, "requestId 必须是规范的 UUID 字符串")
	}
	raw, err := json.Marshal(struct {
		Kind   string
		Target int64
		Body   any
	}{kind, target, body})
	if err != nil {
		return model.PODTemplateAction{}, errs.WithCode(400, "模板请求格式不合法")
	}
	now := types.Now()
	return model.PODTemplateAction{RequestID: request, Action: kind, RequestHash: fmt.Sprintf("%x", sha256.Sum256(raw)), CreateBy: operator, UpdateBy: operator, CreateTime: now, UpdateTime: now}, nil
}

func (s PODTemplateService) List(ctx context.Context, user *model.SysUser, deleted bool, pg page.Query) ([]model.PODTemplateView, int64, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := repository.SelectPODTemplatePage(ctx, uid, deleted, podTemplatePage(pg))
	if err != nil {
		return nil, 0, podTemplateError(err)
	}
	out := make([]model.PODTemplateView, 0, len(rows))
	for i := range rows {
		out = append(out, *podTemplateView(&rows[i]))
	}
	return out, total, nil
}

func (s PODTemplateService) Get(ctx context.Context, user *model.SysUser, id int64) (*model.PODTemplateView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	row, err := repository.SelectPODTemplate(ctx, uid, id)
	if err != nil {
		return nil, podTemplateError(err)
	}
	snapshot, err := s.Version(ctx, user, id, row.CurrentVersion)
	if err != nil {
		return nil, err
	}
	out := podTemplateView(row)
	out.Snapshot = snapshot
	return out, nil
}

func (s PODTemplateService) Versions(ctx context.Context, user *model.SysUser, id int64, pg page.Query) ([]model.PODTemplateVersionView, int64, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	if _, err = repository.SelectPODTemplate(ctx, uid, id); err != nil {
		return nil, 0, podTemplateError(err)
	}
	rows, total, err := repository.SelectPODTemplateVersions(ctx, uid, id, podTemplatePage(pg))
	if err != nil {
		return nil, 0, podTemplateError(err)
	}
	out := make([]model.PODTemplateVersionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.PODTemplateVersionView{TemplateID: r.TemplateID, Version: r.Version, CreateTime: r.CreateTime})
	}
	return out, total, nil
}

func (s PODTemplateService) Version(ctx context.Context, user *model.SysUser, id, version int64) (*model.PODTemplateVersionView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	row, err := repository.SelectPODTemplateVersion(ctx, uid, id, version)
	if err != nil {
		return nil, podTemplateError(err)
	}
	return podTemplateSnapshot(row)
}

func (s PODTemplateService) actionResult(ctx context.Context, user *model.SysUser, a *model.PODTemplateAction) (*model.PODTemplateView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	row, err := repository.SelectPODTemplate(ctx, uid, a.TemplateID)
	if err != nil {
		return nil, podTemplateError(err)
	}
	snapshot, err := s.Version(ctx, user, a.TemplateID, a.Version)
	if err != nil {
		return nil, err
	}
	out := podTemplateView(row)
	out.Snapshot = snapshot
	return out, nil
}

func (s PODTemplateService) Save(ctx context.Context, user *model.SysUser, id int64, body model.PODTemplateSave) (*model.PODTemplateView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	body.Name, err = podTemplateName(body.Name)
	if err != nil {
		return nil, err
	}
	body.Description, err = podTemplateDescription(body.Description)
	if err != nil {
		return nil, err
	}
	projectID, err := PODPositiveID(body.ProjectID)
	if err != nil {
		return nil, errs.WithCode(400, "来源项目编号不合法")
	}
	projectRevision, err := podRevision(body.ProjectRevision)
	if err != nil {
		return nil, err
	}
	var revision int64
	if id > 0 {
		revision, err = podRevision(body.Revision)
		if err != nil {
			return nil, err
		}
	} else if body.Revision != "" {
		return nil, errs.WithCode(400, "创建模板不接受已有模板版本号")
	}
	kind := "create"
	if id > 0 {
		kind = "version"
	}
	action, err := podTemplateAction(body.RequestID, kind, id, body, user.UserName)
	if err != nil {
		return nil, err
	}
	old, err := repository.FindPODTemplateAction(ctx, uid, action.RequestID, action.Action, action.RequestHash)
	if err != nil {
		return nil, podTemplateError(err)
	}
	if old != nil {
		return s.actionResult(ctx, user, old)
	}
	p, err := repository.SelectPODProjectByID(ctx, uid, projectID)
	if err != nil {
		return nil, podTemplateError(err)
	}
	if p.Revision != projectRevision {
		return nil, podTemplateError(repository.ErrPODRevision)
	}
	var graph model.PODGraph
	if json.Unmarshal(p.GraphJSON, &graph) != nil {
		return nil, errs.WithCode(400, "来源项目画布无法读取")
	}
	if err = workflow.ValidateWithLimits(graph, s.Limits); err != nil {
		return nil, errs.WithCode(400, err.Error())
	}
	if len(graph.Nodes) == 0 {
		return nil, errs.WithCode(400, "请先添加节点，再保存为模板")
	}
	now := types.Now()
	result, err := repository.WritePODTemplate(ctx, uid, repository.PODTemplateWrite{TargetID: id, Revision: revision, ProjectID: projectID, ProjectRevision: projectRevision,
		Template: model.PODTemplate{Name: body.Name, Description: body.Description, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now},
		Version:  model.PODTemplateVersion{GraphJSON: p.GraphJSON, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}, Refs: podTemplateRefs(graph), Action: action})
	if err != nil {
		return nil, podTemplateError(err)
	}
	return s.actionResult(ctx, user, result)
}

func (s PODTemplateService) Update(ctx context.Context, user *model.SysUser, id int64, body model.PODTemplateUpdate) (*model.PODTemplateView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	revision, err := podRevision(body.Revision)
	if err != nil {
		return nil, err
	}
	name, err := podTemplateName(body.Name)
	if err != nil {
		return nil, err
	}
	description, err := podTemplateDescription(body.Description)
	if err != nil {
		return nil, err
	}
	row, err := repository.UpdatePODTemplate(ctx, uid, id, revision, "0", map[string]any{"name": name, "description": description, "update_by": user.UserName})
	if err != nil {
		return nil, podTemplateError(err)
	}
	return podTemplateView(row), nil
}

func (s PODTemplateService) Deletion(ctx context.Context, user *model.SysUser, id int64, value string, restore bool) (*model.PODTemplateView, error) {
	uid, err := podProjectUser(ctx, user)
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
	row, err := repository.UpdatePODTemplate(ctx, uid, id, revision, from, map[string]any{"del_flag": to, "update_by": user.UserName})
	if err != nil {
		return nil, podTemplateError(err)
	}
	return podTemplateView(row), nil
}

func (s PODTemplateService) Copy(ctx context.Context, user *model.SysUser, id int64, body model.PODTemplateCopy) (*model.PODTemplateView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	body.Name, err = podTemplateName(body.Name)
	if err != nil {
		return nil, err
	}
	version, err := podRevision(body.Version)
	if err != nil {
		return nil, err
	}
	action, err := podTemplateAction(body.RequestID, "copy", id, body, user.UserName)
	if err != nil {
		return nil, err
	}
	old, err := repository.FindPODTemplateAction(ctx, uid, action.RequestID, action.Action, action.RequestHash)
	if err != nil {
		return nil, podTemplateError(err)
	}
	if old != nil {
		return s.actionResult(ctx, user, old)
	}
	source, err := repository.SelectPODTemplate(ctx, uid, id)
	if err != nil {
		return nil, podTemplateError(err)
	}
	snapshot, err := s.Version(ctx, user, id, version)
	if err != nil {
		return nil, err
	}
	if err = workflow.ValidateWithLimits(*snapshot.Graph, s.Limits); err != nil {
		return nil, errs.WithCode(400, err.Error())
	}
	raw, err := json.Marshal(snapshot.Graph)
	if err != nil {
		return nil, err
	}
	now := types.Now()
	result, err := repository.WritePODTemplate(ctx, uid, repository.PODTemplateWrite{SourceID: id, SourceVersion: version,
		Template: model.PODTemplate{Name: body.Name, Description: source.Description, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now},
		Version:  model.PODTemplateVersion{GraphJSON: raw, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}, Refs: podTemplateRefs(*snapshot.Graph), Action: action})
	if err != nil {
		return nil, podTemplateError(err)
	}
	return s.actionResult(ctx, user, result)
}

func bindPODTemplate(graph *model.PODGraph, bindings []model.PODTemplateBinding) error {
	nodes := map[string]*model.PODNode{}
	for i := range graph.Nodes {
		nodes[graph.Nodes[i].ID] = &graph.Nodes[i]
	}
	seen := map[string]bool{}
	if len(bindings) > len(nodes) {
		return errs.WithCode(400, "输入槽位数量不合法")
	}
	for _, b := range bindings {
		n := nodes[b.NodeID]
		if n == nil || seen[b.NodeID] {
			return errs.WithCode(400, "输入槽位不存在或重复")
		}
		seen[b.NodeID] = true
		var config map[string]json.RawMessage
		if err := json.Unmarshal(n.Config, &config); err != nil {
			return err
		}
		switch {
		case n.Type == "textInput" && b.Text != nil && b.AssetIDs == nil:
			raw, _ := json.Marshal(*b.Text)
			config["text"] = raw
		case n.Type == "imageInput" && b.Text == nil && b.AssetIDs != nil && *b.AssetIDs != nil:
			raw, _ := json.Marshal(*b.AssetIDs)
			config["assetIds"] = raw
		default:
			return errs.WithCode(400, "槽位只接受对应的文字或图片，不能修改其他节点")
		}
		raw, err := json.Marshal(config)
		if err != nil {
			return err
		}
		n.Config = raw
	}
	return nil
}

func (s PODTemplateService) Use(ctx context.Context, user *model.SysUser, id int64, body model.PODTemplateUse) (*model.PODProjectView, error) {
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	body.Name, err = PODProjectName(body.Name)
	if err != nil {
		return nil, err
	}
	version, err := podRevision(body.Version)
	if err != nil {
		return nil, err
	}
	action, err := podTemplateAction(body.RequestID, "use", id, body, user.UserName)
	if err != nil {
		return nil, err
	}
	old, err := repository.FindPODTemplateAction(ctx, uid, action.RequestID, action.Action, action.RequestHash)
	if err != nil {
		return nil, podTemplateError(err)
	}
	if old != nil {
		return GetPODProject(ctx, user, old.ProjectID)
	}
	snapshot, err := s.Version(ctx, user, id, version)
	if err != nil {
		return nil, err
	}
	if err = bindPODTemplate(snapshot.Graph, body.Bindings); err != nil {
		return nil, err
	}
	if err = workflow.ValidateWithLimits(*snapshot.Graph, s.Limits); err != nil {
		return nil, errs.WithCode(400, err.Error())
	}
	raw, err := json.Marshal(snapshot.Graph)
	if err != nil {
		return nil, err
	}
	now := types.Now()
	project := &model.PODProject{Name: body.Name, GraphJSON: raw, SchemaVersion: snapshot.Graph.SchemaVersion, Revision: 1, DelFlag: "0", CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now, Remark: fmt.Sprintf("pod-template:%d:%d", id, version)}
	action.TemplateID = id
	action.Version = version
	result, err := repository.UsePODTemplate(ctx, uid, project, podTemplateRefs(*snapshot.Graph), action)
	if err != nil {
		return nil, podTemplateError(err)
	}
	return GetPODProject(ctx, user, result.ProjectID)
}
