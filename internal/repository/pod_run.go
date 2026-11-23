package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var (
	ErrPODRun       = errors.New("POD run not found")
	ErrPODRequest   = errors.New("POD request conflict")
	ErrPODRunState  = errors.New("POD run state does not allow action")
	ErrPODCredit    = errors.New("POD insufficient credits")
	ErrPODQueueFull = errors.New("POD pending limit")
	ErrPODLease     = errors.New("POD stale lease")
)
var podActiveRunStates = []string{"queued", "running", "cancelRequested", "awaitingReconciliation"}

const podRunOwner = "EXISTS (SELECT 1 FROM pod_workspace w JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=? AND m.status='0' WHERE w.workspace_id=pod_run.workspace_id AND w.owner_user_id=? AND w.status='0')"

func lockPODGate(tx *gorm.DB) error {
	var row struct{ GateID int }
	return tx.Table("pod_execution_gate").Select("gate_id").Where("gate_id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
}
func podJSON(v any) json.RawMessage { data, _ := json.Marshal(v); return data }
func podNow() time.Time             { return time.Now().In(types.Location) }

func podAccount(tx *gorm.DB, workspaceID, grant int64, operator string) (*model.PODCreditAccount, error) {
	var row model.PODCreditAccount
	err := tx.Where("workspace_id=?", workspaceID).Take(&row).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	now := podNow()
	row = model.PODCreditAccount{WorkspaceID: workspaceID, Available: grant, CreateBy: operator, UpdateBy: operator, CreateTime: now, UpdateTime: now}
	if err := tx.Create(&row).Error; err != nil {
		return nil, err
	}
	err = podCreditEvent(tx, &row, 0, "initial-grant", "grant", grant, operator)
	return &row, err
}
func podCreditEvent(tx *gorm.DB, a *model.PODCreditAccount, runID int64, key, kind string, amount int64, operator string) error {
	if a.Available < 0 || a.Held < 0 || a.Spent < 0 {
		return errors.New("POD ledger invariant violation")
	}
	now := types.Now()
	event := model.PODCreditEvent{WorkspaceID: a.WorkspaceID, RunID: runID, EventKey: key, Kind: kind, Amount: amount, Available: a.Available, Held: a.Held, Spent: a.Spent, CreateBy: operator, UpdateBy: operator, CreateTime: now, UpdateTime: now}
	if err := tx.Create(&event).Error; err != nil {
		return err
	}
	return tx.Model(a).Updates(map[string]any{"available": a.Available, "held": a.Held, "spent": a.Spent, "update_time": podNow(), "update_by": operator}).Error
}
func reservePODCredits(tx *gorm.DB, r *model.PODRun, amount, grant int64) error {
	if r.ExecutionMode == "qwen" || r.ExecutionMode == "managed" {
		r.Reserved = 0
		r.Settled = false
		return nil
	}
	a, err := podAccount(tx, r.WorkspaceID, grant, r.CreateBy)
	if err != nil {
		return err
	}
	if a.Available < amount {
		return ErrPODCredit
	}
	a.Available -= amount
	a.Held += amount
	r.Reserved = amount
	r.Settled = false
	return podCreditEvent(tx, a, r.RunID, fmt.Sprintf("reserve:%d:%d", r.RunID, r.Attempt), "reserve", amount, r.CreateBy)
}
func settlePODCredits(tx *gorm.DB, r *model.PODRun, rows []model.PODNodeRun) error {
	if r.Settled {
		return nil
	}
	if r.ExecutionMode == "managed" {
		return settleManagedRun(tx, r)
	}
	if r.ExecutionMode == "qwen" {
		r.Settled = true
		r.Reserved = 0
		return nil
	}
	var amount int64
	for _, n := range rows {
		if n.NodeType == "textGenerate" && n.State == "succeeded" && n.Attempt == r.Attempt {
			amount++
		}
	}
	if amount > r.Reserved {
		return errors.New("POD settlement exceeds reservation")
	}
	a, err := podAccount(tx, r.WorkspaceID, 0, "pod-worker")
	if err != nil {
		return err
	}
	if a.Held < r.Reserved {
		return errors.New("POD held amount mismatch")
	}
	a.Held -= amount
	a.Spent += amount
	if err := podCreditEvent(tx, a, r.RunID, fmt.Sprintf("settle:%d:%d", r.RunID, r.Attempt), "settle", amount, "pod-worker"); err != nil {
		return err
	}
	release := r.Reserved - amount
	a.Held -= release
	a.Available += release
	if err := podCreditEvent(tx, a, r.RunID, fmt.Sprintf("release:%d:%d", r.RunID, r.Attempt), "release", release, "pod-worker"); err != nil {
		return err
	}
	r.Spent += amount
	r.Reserved = 0
	r.Settled = true
	return nil
}

func FindPODRunRequest(ctx context.Context, userID int64, requestID, hash string) (*model.PODRun, error) {
	var row model.PODRun
	err := DB(ctx).Where("request_id=?", requestID).Where(podRunOwner, userID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != hash {
		return nil, ErrPODRequest
	}
	return &row, nil
}

func CreatePODRun(ctx context.Context, user *model.SysUser, body model.PODRunCreate, projectID, revision, baseID int64, hash string, limits model.PODExecutionLimits, graphLimits model.PODGraphLimits) (*model.PODRun, error) {
	var result *model.PODRun
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = createPODRunTx(tx, user, body, projectID, revision, baseID, hash, limits, graphLimits)
		return err
	})
	return result, err
}

// The caller owns the transaction so batch dispatch and run/outbox creation commit together.
func createPODRunTx(tx *gorm.DB, user *model.SysUser, body model.PODRunCreate, projectID, revision, baseID int64, hash string, limits model.PODExecutionLimits, graphLimits model.PODGraphLimits, precounted ...bool) (*model.PODRun, error) {
	var result model.PODRun
	err := func() error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if limits.Mode == "managed" {
			p, e := lockManagedPlatform(tx)
			if e != nil {
				return e
			}
			if !p.ManagedEnabled {
				return ErrPODRunState
			}
		}
		w, err := lockedPODWorkspace(tx, user.UserID)
		if err != nil {
			return err
		}
		err = tx.Where("workspace_id=? AND request_id=?", w.WorkspaceID, body.RequestID).Take(&result).Error
		if err == nil {
			if result.RequestHash != hash {
				return ErrPODRequest
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if len(precounted) == 0 || !precounted[0] {
			if err := acceptCreatorSubmissions(tx, user.UserID, 1); err != nil {
				return err
			}
		}
		prepared, err := preparePODRun(tx, w, body, projectID, revision, baseID, limits, graphLimits)
		if err != nil {
			return err
		}
		project, g, graphJSON, base, plan := prepared.Project, prepared.Graph, prepared.GraphJSON, prepared.Base, prepared.Plan
		if err := checkPODProductRunOverlap(tx, w.WorkspaceID, projectID, plan); err != nil {
			return err
		}
		if err := checkPODPending(tx, w.WorkspaceID, limits, precounted...); err != nil {
			return err
		}
		assetIDs := map[int64]bool{}
		for _, n := range g.Nodes {
			if n.Type == "imageInput" {
				var c struct {
					AssetIDs []string `json:"assetIds"`
				}
				_ = json.Unmarshal(n.Config, &c)
				for _, s := range c.AssetIDs {
					id, _ := strconv.ParseInt(s, 10, 64)
					assetIDs[id] = true
				}
			}
		}
		for _, o := range base {
			for _, s := range o.AssetIDs {
				id, _ := strconv.ParseInt(s, 10, 64)
				assetIDs[id] = true
			}
		}
		ids := []int64{}
		for id := range assetIDs {
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			var count int64
			if err := tx.Table("pod_asset").Where("workspace_id=? AND del_flag='0' AND asset_id IN ?", w.WorkspaceID, ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return ErrPODAssetGraph
			}
		}
		now := podNow()
		actionTarget := ""
		if body.Mode == "node" {
			actionTarget = body.NodeID
		}
		if body.Mode == "polish" || body.Mode == "branch" {
			actionTarget = body.NodeID
		}
		result = model.PODRun{WorkspaceID: w.WorkspaceID, UserID: user.UserID, ProjectID: projectID, ProjectRevision: revision, ProjectName: project.Name, RequestID: body.RequestID, RequestHash: hash, Mode: body.Mode, ExecutionMode: limits.Mode, TargetNodeID: actionTarget, BaseRunID: baseID, GraphJSON: graphJSON, BaseOutputsJSON: podJSON(base), State: model.PODRunQueued, TestScenario: body.TestScenario, Attempt: 1, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		rows := []model.PODNodeRun{}
		var cost int64
		for _, n := range plan {
			provider := "local"
			providerModel := ""
			if n.Type == "textGenerate" {
				provider = "simulation"
				cost++
			}
			if limits.Mode == "qwen" || limits.Mode == "managed" {
				if n.Type == "textGenerate" {
					provider = limits.Mode
					providerModel = executor.QwenTextModel
				}
				if n.Type == "imageGenerate" {
					provider = limits.Mode
					providerModel = executor.QwenImageModel
				}
			}
			rows = append(rows, model.PODNodeRun{RunID: result.RunID, WorkspaceID: w.WorkspaceID, UserID: user.UserID, NodeID: n.ID, NodeType: n.Type, Provider: provider, ProviderModel: providerModel, State: "waiting", InputJSON: podJSON(model.PODRunOutput{TestOnly: limits.Mode == "simulation"}), OutputJSON: podJSON(model.PODRunOutput{TestOnly: limits.Mode == "simulation"}), CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now})
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		refs := []model.PODRunAssetRef{}
		for _, id := range ids {
			refs = append(refs, model.PODRunAssetRef{RunID: result.RunID, AssetID: id, WorkspaceID: w.WorkspaceID, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now})
		}
		if len(refs) > 0 {
			if err := tx.CreateInBatches(&refs, 200).Error; err != nil {
				return err
			}
		}
		if limits.Mode == "managed" {
			if err := attachManagedRun(tx, &result, body, prepared, len(precounted) > 0 && precounted[0]); err != nil {
				return err
			}
		}
		if err := reservePODCredits(tx, &result, cost, limits.InitialCredits); err != nil {
			return err
		}
		if err := tx.Model(&result).Updates(map[string]any{"reserved": result.Reserved}).Error; err != nil {
			return err
		}
		return tx.Create(&model.PODOutbox{RunID: result.RunID, Active: true, NextDeliveryAt: now, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}).Error
	}()
	return &result, err
}

var ErrPODRunInput = errors.New("POD execution input invalid")

func checkPODPending(tx *gorm.DB, workspaceID int64, l model.PODExecutionLimits, dispatch ...bool) error {
	var p model.PODCreatorPolicy
	if err := tx.Where("workspace_id=?", workspaceID).Take(&p).Error; err != nil {
		return err
	}
	l.MaxUserPending = min(l.MaxUserPending, p.MaxPendingRuns)
	var global, own int64
	if err := tx.Model(&model.PODRun{}).Where("state IN ?", podActiveRunStates).Count(&global).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.PODRun{}).Where("state IN ? AND workspace_id=?", podActiveRunStates, workspaceID).Count(&own).Error; err != nil {
		return err
	}
	if len(dispatch) == 0 || !dispatch[0] {
		var waiting int64
		if err := tx.Table("pod_batch_item").Where("workspace_id=? AND state='waiting'", workspaceID).Count(&waiting).Error; err != nil {
			return err
		}
		own += waiting
	}
	if global >= int64(l.MaxPending) || own >= int64(l.MaxUserPending) {
		return ErrPODQueueFull
	}
	return nil
}
func SelectPODRunPage(ctx context.Context, userID, projectID int64, pg page.Query) ([]model.PODRun, int64, error) {
	rows := []model.PODRun{}
	q := DB(ctx).Model(&model.PODRun{}).Where("workspace_id=(SELECT workspace_id FROM pod_workspace WHERE owner_user_id=?)", userID).Where(podRunOwner, userID, userID)
	if projectID > 0 {
		q = q.Where("project_id=?", projectID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Omit("graph_json", "base_outputs_json").Order("create_time DESC,run_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func SelectPODRun(ctx context.Context, userID, id int64) (*model.PODRun, []model.PODNodeRun, error) {
	var r model.PODRun
	err := DB(ctx).Where("run_id=?", id).Where(podRunOwner, userID, userID).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrPODRun
	}
	if err != nil {
		return nil, nil, err
	}
	rows := []model.PODNodeRun{}
	err = DB(ctx).Where("run_id=?", id).Order("node_id ASC").Find(&rows).Error
	return &r, rows, err
}
func SelectPODCredits(ctx context.Context, userID int64, pg page.Query) (*model.PODCreditAccount, []model.PODCreditEvent, int64, error) {
	w, err := PODPersonalWorkspace(ctx, userID)
	if err != nil {
		return nil, nil, 0, err
	}
	if w == nil {
		return nil, nil, 0, ErrPODWorkspace
	}
	a := model.PODCreditAccount{WorkspaceID: w.WorkspaceID}
	err = DB(ctx).Where("workspace_id=?", w.WorkspaceID).Take(&a).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, 0, err
	}
	q := DB(ctx).Model(&model.PODCreditEvent{}).Where("workspace_id=?", w.WorkspaceID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, nil, 0, err
	}
	rows := []model.PODCreditEvent{}
	err = q.Order("event_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return &a, rows, total, err
}

// Mutations share gate -> workspace -> run -> account lock order. No provider I/O under locks.
func ChangePODRun(ctx context.Context, user *model.SysUser, id int64, retryKey string, l model.PODExecutionLimits) (*model.PODRun, error) {
	var r model.PODRun
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		w, err := lockedPODWorkspaceAccess(tx, user.UserID, retryKey != "")
		if err != nil {
			return err
		}
		err = tx.Where("run_id=? AND workspace_id=?", id, w.WorkspaceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODRun
		}
		if err != nil {
			return err
		}
		if retryKey != "" {
			if r.ExecutionMode == "managed" {
				return ErrPODRunState
			}
			if r.ExecutionMode != l.Mode {
				return ErrPODRunState
			}
			var request model.PODRetryRequest
			err = tx.Where("workspace_id=? AND request_id=?", w.WorkspaceID, retryKey).Take(&request).Error
			if err == nil {
				if request.RunID != id {
					return ErrPODRequest
				}
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if r.State != model.PODRunFailed || !r.Settled || r.Attempt >= 5 {
				return ErrPODRunState
			}
			if err := checkPODPending(tx, w.WorkspaceID, l); err != nil {
				return err
			}
			if err := acceptCreatorSubmissions(tx, user.UserID, 1); err != nil {
				return err
			}
			rows := []model.PODNodeRun{}
			if err := tx.Where("run_id=?", id).Find(&rows).Error; err != nil {
				return err
			}
			var cost int64
			for _, n := range rows {
				if n.State != "succeeded" && n.NodeType == "textGenerate" {
					cost++
				}
			}
			r.Attempt++
			r.State = model.PODRunQueued
			if err := reservePODCredits(tx, &r, cost, l.InitialCredits); err != nil {
				return err
			}
			if err := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND state<>'succeeded'", id).Updates(map[string]any{"state": "waiting", "lease_until": nil, "lease_token": "", "submission_key": "", "error_code": "", "error_message": "", "update_time": podNow()}).Error; err != nil {
				return err
			}
			now := podNow()
			if err := tx.Create(&model.PODRetryRequest{WorkspaceID: w.WorkspaceID, RequestID: retryKey, RunID: id, Attempt: r.Attempt, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}).Error; err != nil {
				return err
			}
		} else {
			if workflow.RunTerminal(r.State) {
				return nil
			}
			r.State = model.PODRunCancelRequested
		}
		if err := tx.Model(&r).Updates(map[string]any{"state": r.State, "attempt": r.Attempt, "reserved": r.Reserved, "settled": r.Settled, "update_time": podNow(), "update_by": user.UserName}).Error; err != nil {
			return err
		}
		return tx.Model(&model.PODOutbox{}).Where("run_id=?", id).Updates(map[string]any{"active": true, "next_delivery_at": podNow(), "update_time": podNow()}).Error
	})
	return &r, err
}

type PODClaim struct {
	Run        model.PODRun
	Node       model.PODNodeRun
	Definition model.PODNode
	Input      model.PODRunOutput
	Reconcile  bool
}

func advancePODRun(tx *gorm.DB, r *model.PODRun, g model.PODGraph, rows []model.PODNodeRun, base map[string]model.PODRunOutput) error {
	changes, state := workflow.Advance(g, rows, base, r.State == model.PODRunCancelRequested)
	groups := map[string][]string{}
	for id, s := range changes {
		groups[s] = append(groups[s], id)
	}
	for s, ids := range groups {
		if err := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND node_id IN ?", r.RunID, ids).Updates(map[string]any{"state": s, "update_time": podNow()}).Error; err != nil {
			return err
		}
	}
	for i := range rows {
		if s, ok := changes[rows[i].NodeID]; ok {
			rows[i].State = s
		}
	}
	r.State = state
	if workflow.RunTerminal(state) {
		if err := settlePODCredits(tx, r, rows); err != nil {
			return err
		}
	}
	if err := tx.Model(r).Updates(map[string]any{"state": r.State, "reserved": r.Reserved, "spent": r.Spent, "settled": r.Settled, "update_time": podNow(), "update_by": "pod-worker"}).Error; err != nil {
		return err
	}
	return tx.Model(&model.PODOutbox{}).Where("run_id=?", r.RunID).Updates(map[string]any{"active": !workflow.RunTerminal(state), "next_delivery_at": podNow(), "update_time": podNow()}).Error
}

func ClaimPODNode(ctx context.Context, runID int64, l model.PODExecutionLimits, lease time.Duration) (*PODClaim, error) {
	var claim *PODClaim
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		var r model.PODRun
		err := tx.Where("run_id=?", runID).Take(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if workflow.RunTerminal(r.State) {
			return nil
		}
		if r.ExecutionMode == "managed" {
			var snapshot model.PODExecutionSnapshot
			if err := tx.Where("scope_kind='run' AND scope_id=? AND workspace_id=? AND user_id=? AND billing_scheme='managedPointsV1'", r.RunID, r.WorkspaceID, r.UserID).Take(&snapshot).Error; err != nil {
				return err
			}
		}
		// Mode cutover drains accepted calls, but never starts another legacy step.
		if r.ExecutionMode != l.Mode {
			r.State = model.PODRunCancelRequested
		}
		_, err = lockedPODWorkspaceRaw(tx, r.UserID)
		if errors.Is(err, ErrPODWorkspace) {
			r.State = model.PODRunCancelRequested
		} else if err != nil {
			return err
		}
		var activeUser int64
		if err := tx.Table("sys_user").Where("user_id=? AND status='0' AND del_flag='0'", r.UserID).Count(&activeUser).Error; err != nil {
			return err
		}
		if activeUser == 0 {
			r.State = model.PODRunCancelRequested
		}
		policy, policyErr := checkCreator(tx, r.UserID, true, true)
		if creatorDenied(policyErr) {
			r.State = model.PODRunCancelRequested
		} else if policyErr != nil {
			return policyErr
		}
		rows := []model.PODNodeRun{}
		if err := tx.Where("run_id=?", runID).Order("node_id ASC").Find(&rows).Error; err != nil {
			return err
		}
		var now time.Time
		if err := tx.Raw("SELECT CURRENT_TIMESTAMP(3)").Scan(&now).Error; err != nil {
			return err
		}
		expired := []string{}
		for i := range rows {
			n := &rows[i]
			if n.State == "running" && n.LeaseUntil != nil && !n.LeaseUntil.After(now) {
				n.State = "awaitingReconciliation"
				expired = append(expired, n.NodeID)
			}
		}
		if len(expired) > 0 {
			if err := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND node_id IN ?", runID, expired).Updates(map[string]any{"state": "awaitingReconciliation", "lease_token": "", "lease_until": nil, "error_code": "leaseExpired", "error_message": "执行进程中断，正在确认原调用结果", "update_time": now}).Error; err != nil {
				return err
			}
		}
		var g model.PODGraph
		var base map[string]model.PODRunOutput
		if json.Unmarshal(r.GraphJSON, &g) != nil || json.Unmarshal(r.BaseOutputsJSON, &base) != nil {
			return errors.New("POD stored run invalid")
		}
		if err := advancePODRun(tx, &r, g, rows, base); err != nil {
			return err
		}
		if workflow.RunTerminal(r.State) {
			return nil
		}
		changes, _ := workflow.Advance(g, rows, base, r.State == model.PODRunCancelRequested)
		for i := range rows {
			if s, ok := changes[rows[i].NodeID]; ok {
				rows[i].State = s
			}
		}
		outputs := map[string]model.PODRunOutput{}
		for id, o := range base {
			outputs[id] = o
		}
		for _, n := range rows {
			if n.State == "succeeded" {
				var o model.PODRunOutput
				if json.Unmarshal(n.OutputJSON, &o) != nil {
					return errors.New("POD stored node output invalid")
				}
				outputs[n.NodeID] = o
			}
		}
		var global, own int64
		if err := tx.Model(&model.PODNodeRun{}).Where("state='running'").Count(&global).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PODNodeRun{}).Where("state='running' AND user_id=?", r.UserID).Count(&own).Error; err != nil {
			return err
		}
		if policy != nil {
			l.UserConcurrency = min(l.UserConcurrency, policy.MaxConcurrentNodes)
		}
		if global >= int64(l.GlobalConcurrency) || own >= int64(l.UserConcurrency) {
			return nil
		}
		definitions := make(map[string]model.PODNode, len(g.Nodes))
		positions := make(map[string]int, len(g.Nodes))
		for i, node := range g.Nodes {
			definitions[node.ID] = node
			positions[node.ID] = i
		}
		// Ready branches run first; pending calls rotate by their last check, so one uncertain call cannot starve another.
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].State == "awaitingReconciliation" && rows[j].State == "awaitingReconciliation" && !rows[i].UpdateTime.Equal(rows[j].UpdateTime) {
				return rows[i].UpdateTime.Before(rows[j].UpdateTime)
			}
			if rows[i].State != rows[j].State {
				return rows[i].State < rows[j].State
			}
			return positions[rows[i].NodeID] < positions[rows[j].NodeID]
		})
		for _, wantedState := range []string{"ready", "awaitingReconciliation"} {
			for _, candidate := range rows {
				if candidate.State != wantedState {
					continue
				}
				node, ok := definitions[candidate.NodeID]
				if !ok {
					return errors.New("POD stored node definition missing")
				}
				reconcile := candidate.State == "awaitingReconciliation"
				if r.State == model.PODRunCancelRequested && !reconcile {
					continue
				}
				if candidate.Provider == "simulation" || candidate.Provider == "qwen" || strings.HasPrefix(candidate.Provider, "managed:") {
					var count int64
					if err := tx.Model(&model.PODNodeRun{}).Where("state='running' AND provider=?", candidate.Provider).Count(&count).Error; err != nil {
						return err
					}
					if count >= int64(l.ProviderConcurrency) {
						return nil
					}
				}
				input := workflow.NodeInputs(g, node.ID, outputs)
				candidate.LeaseToken = uuid.NewString()
				candidate.LeaseUntil = new(time.Time)
				*candidate.LeaseUntil = now.Add(lease)
				if !reconcile {
					candidate.Attempt = r.Attempt
					candidate.SubmissionKey = uuid.NewString()
					candidate.InputJSON = podJSON(input)
				} else {
					if json.Unmarshal(candidate.InputJSON, &input) != nil {
						return errors.New("POD stored input invalid")
					}
				}
				candidate.State = "running"
				if err := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND node_id=?", runID, node.ID).Updates(map[string]any{"state": "running", "attempt": candidate.Attempt, "submission_key": candidate.SubmissionKey, "lease_token": candidate.LeaseToken, "lease_until": candidate.LeaseUntil, "input_json": string(candidate.InputJSON), "update_time": now}).Error; err != nil {
					return err
				}
				if err := tx.Table("pod_creator_policy").Where("user_id=?", r.UserID).Update("last_dispatched_at", now).Error; err != nil {
					return err
				}
				claim = &PODClaim{Run: r, Node: candidate, Definition: node, Input: input, Reconcile: reconcile}
				return nil
			}
		}
		return nil
	})
	return claim, err
}

func HeartbeatPODNode(ctx context.Context, c *PODClaim, lease time.Duration) error {
	result := DB(ctx).Model(&model.PODNodeRun{}).Where("run_id=? AND node_id=? AND state='running' AND lease_token=? AND lease_until>CURRENT_TIMESTAMP(3)", c.Run.RunID, c.Node.NodeID, c.Node.LeaseToken).Updates(map[string]any{"lease_until": gorm.Expr("DATE_ADD(CURRENT_TIMESTAMP(3), INTERVAL ? MICROSECOND)", lease.Microseconds()), "update_time": podNow()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPODLease
	}
	return nil
}
func FinishPODNode(ctx context.Context, c *PODClaim, state string, output model.PODRunOutput, code, message string) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		var node model.PODNodeRun
		if err := tx.Where("run_id=? AND node_id=? AND state='running' AND lease_token=? AND lease_until>CURRENT_TIMESTAMP(3)", c.Run.RunID, c.Node.NodeID, c.Node.LeaseToken).Take(&node).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPODLease
			}
			return err
		}
		if state != "succeeded" && state != "failed" && state != "awaitingReconciliation" {
			return ErrPODRunState
		}
		if err := tx.Model(&node).Updates(map[string]any{"state": state, "output_json": string(podJSON(output)), "error_code": code, "error_message": message, "lease_until": nil, "lease_token": "", "update_time": podNow()}).Error; err != nil {
			return err
		}
		var r model.PODRun
		if err := tx.Where("run_id=?", c.Run.RunID).Take(&r).Error; err != nil {
			return err
		}
		rows := []model.PODNodeRun{}
		if err := tx.Where("run_id=?", r.RunID).Find(&rows).Error; err != nil {
			return err
		}
		var g model.PODGraph
		var base map[string]model.PODRunOutput
		if json.Unmarshal(r.GraphJSON, &g) != nil || json.Unmarshal(r.BaseOutputsJSON, &base) != nil {
			return errors.New("POD stored graph invalid")
		}
		return advancePODRun(tx, &r, g, rows, base)
	})
}

// Outbox remains active until terminal state. An acknowledged Redis task is never the sole recovery record.
func DuePODOutbox(ctx context.Context, interval time.Duration) ([]model.PODOutbox, error) {
	rows := []model.PODOutbox{}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if err := tx.Where("active=1 AND next_delivery_at<=CURRENT_TIMESTAMP(3)").Order("next_delivery_at ASC,run_id ASC").Limit(50).Find(&rows).Error; err != nil {
			return err
		}
		ids := []int64{}
		for i := range rows {
			rows[i].DeliverySeq++
			ids = append(ids, rows[i].RunID)
		}
		if len(ids) == 0 {
			return nil
		}
		return tx.Model(&model.PODOutbox{}).Where("run_id IN ?", ids).Updates(map[string]any{"delivery_seq": gorm.Expr("delivery_seq+1"), "next_delivery_at": gorm.Expr("DATE_ADD(CURRENT_TIMESTAMP(3), INTERVAL ? MICROSECOND)", interval.Microseconds()), "update_time": podNow()}).Error
	})
	return rows, err
}

type preparedPODRun struct {
	Project   model.PODProject
	Graph     model.PODGraph
	GraphJSON json.RawMessage
	Base      map[string]model.PODRunOutput
	Plan      []model.PODNode
}

func preparePODRun(tx *gorm.DB, w *model.PODWorkspace, body model.PODRunCreate, projectID, revision, baseID int64, limits model.PODExecutionLimits, graphLimits model.PODGraphLimits) (*preparedPODRun, error) {
	var project model.PODProject
	var g model.PODGraph
	var graphJSON json.RawMessage
	var base map[string]model.PODRunOutput
	var plan []model.PODNode
	var err error
	err = func() error {
		err = tx.Table("pod_project").Where("project_id=? AND workspace_id=? AND del_flag='0'", projectID, w.WorkspaceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&project).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODProject
		}
		if err != nil {
			return err
		}
		if project.Revision != revision {
			return ErrPODRevision
		}
		if err := json.Unmarshal(project.GraphJSON, &g); err != nil {
			return err
		}
		graphJSON = project.GraphJSON
		if body.Mode == "polish" || body.Mode == "branch" {
			if err := workflow.ValidateWithLimits(g, graphLimits); err != nil {
				return fmt.Errorf("%w: %s", ErrPODRunInput, err)
			}
			g, err = workflow.ActionGraph(g, body.Mode, body.NodeID)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrPODRunInput, err)
			}
			graphJSON = podJSON(g)
		}
		base = map[string]model.PODRunOutput{}
		if baseID > 0 {
			var previous model.PODRun
			err = tx.Where("run_id=? AND workspace_id=? AND project_id=? AND project_revision=? AND execution_mode=? AND mode<>'polish'", baseID, w.WorkspaceID, projectID, revision, limits.Mode).Take(&previous).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPODRun
			}
			if err != nil {
				return err
			}
			var nodes []model.PODNodeRun
			if err = tx.Where("run_id=? AND state='succeeded'", baseID).Find(&nodes).Error; err != nil {
				return err
			}
			for _, n := range nodes {
				var o model.PODRunOutput
				if json.Unmarshal(n.OutputJSON, &o) != nil {
					return errors.New("POD stored output invalid")
				}
				base[n.NodeID] = o
			}
			// A previous single-node execution also carries immutable upstream outputs.
			var upstream map[string]model.PODRunOutput
			if json.Unmarshal(previous.BaseOutputsJSON, &upstream) != nil {
				return errors.New("POD stored base output invalid")
			}
			for id, o := range upstream {
				if _, ok := base[id]; !ok {
					base[id] = o
				}
			}
		}
		target := ""
		if body.Mode == "node" {
			target = body.NodeID
		}
		plan, err = workflow.ExecutionPlanMode(g, graphLimits, target, base, limits.Mode)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrPODRunInput, err)
		}
		for _, n := range plan {
			var config struct {
				Method    string `json:"method"`
				Operation string `json:"operation"`
				Format    string `json:"format"`
			}
			_ = json.Unmarshal(n.Config, &config)
			if n.Type == "imageProcess" && config.Method == "ai" && (config.Operation == "upscale" || config.Operation == "removeBackground") && !limits.ImageAIEnabled {
				return fmt.Errorf("%w: 本地 AI 图片服务尚未配置，请先启用或选择普通处理", ErrPODRunInput)
			}
			if n.Type == "export" && config.Format == "webp" {
				return fmt.Errorf("%w: 当前导出支持 PNG/JPEG，请将旧 WebP 配置改为 PNG", ErrPODRunInput)
			}
		}
		return nil
	}()
	return &preparedPODRun{project, g, graphJSON, base, plan}, err
}
