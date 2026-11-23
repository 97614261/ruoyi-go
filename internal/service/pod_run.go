package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func PODExecutionLimits(c config.PODExecutionConfig) model.PODExecutionLimits {
	l := workflow.ExecutionLimits(model.PODExecutionLimits{Mode: c.Mode, GlobalConcurrency: c.GlobalConcurrency, UserConcurrency: c.UserConcurrency, ProviderConcurrency: c.ProviderConcurrency, MaxPending: c.MaxPending, MaxUserPending: c.MaxUserPending, InitialCredits: c.InitialCredits})
	if c.Mode == "qwen" {
		l.TextModel = executor.QwenTextModel
		l.ImageModel = executor.QwenImageModel
	}
	l.ImageAIURL = c.ImageAIURL
	l.ImageAIEnabled = c.ImageAIURL != ""
	return l
}

type PODRunService struct {
	Limits      model.PODExecutionLimits
	GraphLimits model.PODGraphLimits
	BudgetFen   int64
}

var podRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

func podRunError(err error) error {
	switch {
	case errors.Is(err, repository.ErrPODRevision):
		return podProjectError(err)
	case errors.Is(err, repository.ErrPODRun):
		return errs.WithCode(404, "运行记录不存在或没有访问权限")
	case errors.Is(err, repository.ErrPODRequest):
		return errs.WithCode(409, "请求标识已用于其他内容，请创建新的执行请求")
	case errors.Is(err, repository.ErrPODProductBusy):
		return errs.WithCode(409, "这张商品图已有运行中的任务，请刷新查看原任务")
	case errors.Is(err, repository.ErrPODRunState):
		return errs.WithCode(409, "当前状态不能恢复；仅允许恢复已确认失败的运行，最多 5 次")
	case errors.Is(err, repository.ErrPODCredit):
		return errs.WithCode(409, "测试额度不足，请查看额度明细")
	case errors.Is(err, repository.ErrPODQueueFull):
		return errs.WithCode(429, "等待运行已达上限，请等待现有任务完成")
	case errors.Is(err, repository.ErrPODRunInput):
		return errs.WithCode(400, strings.TrimPrefix(err.Error(), repository.ErrPODRunInput.Error()+": "))
	default:
		return podAssetError(err)
	}
}
func (s PODRunService) enabled() error {
	if s.Limits.Mode != "simulation" && s.Limits.Mode != "qwen" && s.Limits.Mode != "managed" {
		return errs.WithCode(409, "当前未启用流程执行")
	}
	return nil
}
func runView(r *model.PODRun, rows []model.PODNodeRun, detail bool) (*model.PODRunView, error) {
	v := &model.PODRunView{ExecutionMode: r.ExecutionMode, RunID: r.RunID, ProjectID: r.ProjectID, ProjectRevision: r.ProjectRevision, ProjectName: r.ProjectName, Mode: r.Mode, TargetNodeID: r.TargetNodeID, State: r.State, Attempt: r.Attempt, TestOnly: r.ExecutionMode == "simulation", TestScenario: r.TestScenario, Reserved: r.Reserved, Spent: r.Spent, CreateTime: types.Time(r.CreateTime), UpdateTime: types.Time(r.UpdateTime)}
	if detail {
		var g model.PODGraph
		if err := json.Unmarshal(r.GraphJSON, &g); err != nil {
			return nil, err
		}
		v.Graph = &g
		v.Nodes = []model.PODNodeRunView{}
		byID := map[string]model.PODNodeRun{}
		for _, n := range rows {
			byID[n.NodeID] = n
		}
		for _, node := range g.Nodes {
			n, ok := byID[node.ID]
			if !ok {
				continue
			}
			var output model.PODRunOutput
			if err := json.Unmarshal(n.OutputJSON, &output); err != nil {
				return nil, err
			}
			v.Nodes = append(v.Nodes, model.PODNodeRunView{NodeID: n.NodeID, NodeType: n.NodeType, State: n.State, Attempt: n.Attempt, Output: output, ErrorCode: n.ErrorCode, ErrorMessage: n.ErrorMessage})
		}
	}
	return v, nil
}
func (s PODRunService) Create(ctx context.Context, user *model.SysUser, body model.PODRunCreate) (*model.PODRunView, error) {
	if err := s.enabled(); err != nil {
		return nil, err
	}
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	if body.ExecutionMode == "" {
		body.ExecutionMode = "simulation"
	}
	if body.ExecutionMode != "simulation" && body.ExecutionMode != "qwen" && body.ExecutionMode != "managed" {
		return nil, errs.WithCode(400, "不支持的执行模式")
	}
	if body.ExecutionMode != "simulation" && body.TestScenario != "" {
		return nil, errs.WithCode(400, "真实执行不接受测试场景")
	}
	if !podRequestID.MatchString(body.RequestID) {
		return nil, errs.WithCode(400, "requestId 须为 16 到 64 位字母、数字、下划线或短横线")
	}
	projectID, err := podRevision(body.ProjectID)
	if err != nil {
		return nil, errs.WithCode(400, "项目编号须为正整数字符串")
	}
	revision, err := podRevision(body.Revision)
	if err != nil {
		return nil, err
	}
	if body.Mode != "workflow" && body.Mode != "node" && body.Mode != "polish" && body.Mode != "branch" {
		return nil, errs.WithCode(400, "mode 须为 workflow、node、polish 或 branch")
	}
	if (body.Mode == "workflow" && (body.NodeID != "" || body.BaseRunID != "")) || (body.Mode != "workflow" && body.NodeID == "") || ((body.Mode == "polish" || body.Mode == "branch") && body.BaseRunID != "") {
		return nil, errs.WithCode(400, "节点操作须指定 nodeId；仅 node 模式允许指定历史上游运行")
	}
	baseID := int64(0)
	if body.BaseRunID != "" {
		baseID, err = podRevision(body.BaseRunID)
		if err != nil {
			return nil, err
		}
	}
	if body.TestScenario == "" && body.ExecutionMode == "simulation" {
		body.TestScenario = "success"
	}
	switch body.TestScenario {
	case "", "success", "delay", "failOnce", "fail", "uncertain":
	default:
		return nil, errs.WithCode(400, "不支持的测试场景")
	}
	if body.ExecutionMode != "managed" && body.QuoteID != "" {
		return nil, errs.WithCode(400, "旧执行模式不接受正式报价")
	}
	encoded, _ := json.Marshal(body)
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if body.ExecutionMode != s.Limits.Mode {
		existing, e := repository.FindPODRunRequest(ctx, user.UserID, body.RequestID, hash)
		if e != nil {
			return nil, podRunError(e)
		}
		if existing != nil {
			return s.Get(ctx, user, existing.RunID)
		}
		return nil, errs.WithCode(409, "执行模式已变更，请刷新页面；旧请求不会自动转换为付费调用")
	}
	row, err := repository.CreatePODRun(ctx, user, body, projectID, revision, baseID, hash, s.Limits, s.GraphLimits)
	if err != nil {
		return nil, podRunError(err)
	}
	return s.Get(ctx, user, row.RunID)
}
func normalizePODPage(pg page.Query) page.Query {
	if pg.PageNum < 1 {
		pg.PageNum = 1
	}
	if pg.PageSize < 1 {
		pg.PageSize = 10
	}
	if pg.PageSize > 100 {
		pg.PageSize = 100
	}
	return pg
}
func (s PODRunService) List(ctx context.Context, user *model.SysUser, projectID int64, pg page.Query) ([]model.PODRunView, int64, error) {
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, 0, err
	}
	rows, total, err := repository.SelectPODRunPage(ctx, user.UserID, projectID, normalizePODPage(pg))
	if err != nil {
		return nil, 0, podRunError(err)
	}
	result := []model.PODRunView{}
	for i := range rows {
		v, err := runView(&rows[i], nil, false)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, *v)
	}
	return result, total, nil
}
func (s PODRunService) Get(ctx context.Context, user *model.SysUser, id int64) (*model.PODRunView, error) {
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	r, n, err := repository.SelectPODRun(ctx, user.UserID, id)
	if err != nil {
		return nil, podRunError(err)
	}
	v, err := runView(r, n, true)
	if err != nil {
		return nil, err
	}
	if r.ExecutionMode == "managed" {
		v.Billing, err = repository.PODManagedBilling(ctx, user.UserID, "run", id)
	}
	return v, err
}
func (s PODRunService) Change(ctx context.Context, user *model.SysUser, id int64, retryKey string, quoteIDs ...string) (*model.PODRunView, error) {
	if err := s.enabled(); err != nil {
		return nil, err
	}
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	if retryKey != "" && !podRequestID.MatchString(retryKey) {
		return nil, errs.WithCode(400, "恢复请求标识格式不正确")
	}
	var r *model.PODRun
	var err error
	quote := ""
	if len(quoteIDs) > 0 {
		quote = quoteIDs[0]
	}
	if retryKey != "" && s.Limits.Mode == "managed" {
		r, err = repository.RetryPODManagedRun(ctx, user, id, retryKey, quote, s.Limits)
	} else {
		if quote != "" {
			return nil, errs.WithCode(400, "当前操作不接受正式报价")
		}
		r, err = repository.ChangePODRun(ctx, user, id, retryKey, s.Limits)
	}
	if err != nil {
		return nil, podRunError(err)
	}
	return s.Get(ctx, user, r.RunID)
}
func (s PODRunService) Credits(ctx context.Context, user *model.SysUser, pg page.Query) (*model.PODCreditAccount, []model.PODCreditEvent, int64, error) {
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, nil, 0, err
	}
	a, rows, total, err := repository.SelectPODCredits(ctx, user.UserID, normalizePODPage(pg))
	return a, rows, total, podRunError(err)
}
func PODPositiveID(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != value {
		return 0, errs.WithCode(400, "编号须为规范正整数字符串")
	}
	return n, nil
}
