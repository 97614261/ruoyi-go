package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func PODAdminState(ctx context.Context) (*model.PODPlatformState, error) {
	var state model.PODPlatformState
	err := DB(ctx).Table("pod_platform_state").Where("state_id=1").Take(&state).Error
	return &state, err
}

// Access events contain no request body, query, headers, session token or user content.
func PODRecordAdminAccess(ctx context.Context, actor int64, allowed bool, trace string, snapshot model.PODAdminAuditSnapshot) error {
	safe, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	outcome := "denied"
	if allowed {
		outcome = "success"
	}
	return DB(ctx).Create(&model.PODAdminAudit{ActorUserID: actor, ActionType: "admin.access", ResourceType: "platform", ResourceID: 1, BeforeJSON: json.RawMessage(`{}`), AfterJSON: safe, TraceID: trace, Outcome: outcome, CreateBy: "pod-admin", CreateTime: types.Now()}).Error
}

func PODAdminOverview(ctx context.Context, begin, end time.Time) (*model.PODAdminOverview, error) {
	view := &model.PODAdminOverview{States: []model.PODAdminRunCount{}}
	// Counts concern metadata only; never load graph, prompt, output or storage keys.
	err := DB(ctx).Raw(`SELECT
 (SELECT COUNT(*) FROM pod_workspace WHERE status='0') creators,
 (SELECT COUNT(*) FROM pod_project WHERE del_flag='0') projects,
 (SELECT COUNT(*) FROM pod_asset WHERE del_flag='0') assets,
 (SELECT COALESCE(SUM(byte_size),0) FROM pod_asset_file) storage_bytes,
 (SELECT COUNT(*) FROM pod_run WHERE create_time>=? AND create_time<?) runs,
 (SELECT COUNT(*) FROM pod_batch WHERE create_time>=? AND create_time<?) batches,
 (SELECT COALESCE(SUM(charged_micros),0) FROM pod_provider_call WHERE create_time>=? AND create_time<?) charged_micros,
 (SELECT COALESCE(SUM(reserved_micros),0) FROM pod_provider_call WHERE create_time>=? AND create_time<?) reserved_micros`, begin, end, begin, end, begin, end, begin, end).Scan(view).Error
	if err != nil {
		return nil, err
	}
	if err := DB(ctx).Table("pod_run").Select("state,COUNT(*) count").Where("create_time>=? AND create_time<?", begin, end).Group("state").Order("state").Scan(&view.States).Error; err != nil {
		return nil, err
	}
	return view, nil
}

func podAdminAuditQuery(ctx context.Context, f model.PODAdminAuditFilter) *gorm.DB {
	q := DB(ctx).Model(&model.PODAdminAudit{}).Where("create_time>=? AND create_time<?", f.Begin, f.End)
	if f.ActorUserID > 0 {
		q = q.Where("actor_user_id=?", f.ActorUserID)
	}
	if f.OwnerUserID > 0 {
		q = q.Where("owner_user_id=?", f.OwnerUserID)
	}
	if f.ActionType != "" {
		q = q.Where("action_type=?", f.ActionType)
	}
	if f.Outcome != "" {
		q = q.Where("outcome=?", f.Outcome)
	}
	return q
}

func PODAdminAuditPage(ctx context.Context, f model.PODAdminAuditFilter, p page.Query) ([]model.PODAdminAudit, int64, error) {
	rows := []model.PODAdminAudit{}
	var total int64
	q := podAdminAuditQuery(ctx, f)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// Listing does not even read JSON snapshots.
	err := podAdminAuditQuery(ctx, f).Select("audit_id,actor_user_id,owner_user_id,action_type,resource_type,resource_id,reason,trace_id,outcome,create_time").Order(p.Stable("create_time DESC, audit_id DESC", "audit_id")).Offset(p.Offset()).Limit(p.PageSize).Find(&rows).Error
	return rows, total, err
}

func PODAdminAuditGet(ctx context.Context, id int64) (*model.PODAdminAudit, error) {
	var row model.PODAdminAudit
	err := DB(ctx).Where("audit_id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
