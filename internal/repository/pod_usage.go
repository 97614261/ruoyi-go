package repository

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

const podUsageCalls = `COALESCE(SUM(c.charged_micros),0) AS charged_micros,
 COALESCE(SUM(c.reserved_micros),0) AS reserved_micros,
 COUNT(c.submission_key) AS provider_calls,
 COALESCE(SUM(c.state='submitted'),0) AS pending_calls,
 COALESCE(SUM(CASE WHEN c.state='confirmed' THEN
 COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(c.receipt_json,'$.usage.imageCount')) AS UNSIGNED),0)
 ELSE 0 END),0) AS generated_images`

func SelectPODUsage(ctx context.Context, userID int64, pg page.Query) (*model.PODUsageView, error) {
	view := &model.PODUsageView{Rows: []model.PODUsageRun{}}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var w model.PODWorkspace
		// Recheck access inside the same snapshot as both totals and details.
		err := tx.Table("pod_workspace").Where("owner_user_id=? AND status='0'", userID).
			Where("EXISTS (SELECT 1 FROM pod_workspace_member m WHERE m.workspace_id=pod_workspace.workspace_id AND m.user_id=? AND m.status='0')", userID).Take(&w).Error
		if err == gorm.ErrRecordNotFound {
			return ErrPODWorkspace
		}
		if err != nil {
			return err
		}
		runs := func() *gorm.DB { return tx.Table("pod_run").Where("workspace_id=?", w.WorkspaceID) }
		if err := runs().Select(`COUNT(*) AS run_count, COALESCE(SUM(state='failed'),0) AS failed_runs,
 COALESCE(SUM(state='awaitingReconciliation'),0) AS pending_runs`).Scan(&view.Summary).Error; err != nil {
			return err
		}
		view.Total = view.Summary.RunCount
		calls := func() *gorm.DB {
			return tx.Table("pod_run r").Joins("JOIN pod_provider_call c ON c.run_id=r.run_id AND c.workspace_id=r.workspace_id").Where("r.workspace_id=?", w.WorkspaceID)
		}
		if err := calls().Select(podUsageCalls).Scan(&view.Summary.PODUsageStats).Error; err != nil {
			return err
		}
		if err := runs().Select("run_id,project_id,project_name,state,attempt,execution_mode = 'simulation' AS test_only,create_time").
			Order("create_time DESC,run_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&view.Rows).Error; err != nil {
			return err
		}
		if len(view.Rows) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(view.Rows))
		for _, row := range view.Rows {
			ids = append(ids, row.RunID)
		}
		var stats []struct {
			RunID int64
			model.PODUsageStats
		}
		if err := calls().Where("r.run_id IN ?", ids).Select("r.run_id," + podUsageCalls).Group("r.run_id").Scan(&stats).Error; err != nil {
			return err
		}
		byID := make(map[int64]model.PODUsageStats, len(stats))
		for _, stat := range stats {
			byID[stat.RunID] = stat.PODUsageStats
		}
		for i := range view.Rows {
			view.Rows[i].PODUsageStats = byID[view.Rows[i].RunID]
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return view, err
}
