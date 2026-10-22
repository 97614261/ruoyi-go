package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"strings"
	"time"
)

func importScope(tx *gorm.DB) *gorm.DB {
	tx = erpShopColumnScope(tx, "erp_import_job.shop_id")
	tid, uid := tenantctx.ID(tx.Statement.Context), erpctx.UserID(tx.Statement.Context)
	return tx.Where("(actor_user_id=? OR EXISTS (SELECT 1 FROM sys_tenant it WHERE it.tenant_id=? AND it.owner_user_id=?))", uid, tid, uid)
}
func importQuery(tx *gorm.DB, q model.ERPImportQuery) *gorm.DB {
	tx = importScope(tx.Model(&model.ERPImportJob{}))
	if q.ShopID > 0 {
		tx = tx.Where("shop_id=?", q.ShopID)
	}
	if q.Status != "" {
		tx = tx.Where("status=?", q.Status)
	}
	return tx
}
func SelectERPImportJobs(ctx context.Context, q model.ERPImportQuery, pg page.Query) ([]model.ERPImportJob, int64, error) {
	tx := importQuery(DB(ctx), q)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPImportJob{}
	err := tx.Omit("remark").Order("job_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	if err == nil && len(rows) > 0 {
		ids := make([]int64, 0, len(rows))
		for _, j := range rows {
			ids = append(ids, j.JobID)
		}
		type progress struct {
			JobID int64
			N     int64
		}
		var counts []progress
		err = DB(ctx).Model(&model.ERPImportResult{}).Select("job_id,COUNT(*) AS n").Where("job_id IN ?", ids).Group("job_id").Scan(&counts).Error
		byID := map[int64]int64{}
		for _, c := range counts {
			byID[c.JobID] = c.N
		}
		for i := range rows {
			rows[i].ProcessedRows = byID[rows[i].JobID]
		}
	}
	return rows, total, err
}
func SelectERPImportJob(ctx context.Context, id int64) (*model.ERPImportJob, error) {
	var j model.ERPImportJob
	err := importScope(DB(ctx).Model(&j)).Where("job_id=?", id).Take(&j).Error
	if err == nil {
		err = DB(ctx).Model(&model.ERPImportResult{}).Where("job_id=?", j.JobID).Count(&j.ProcessedRows).Error
	}
	return &j, erpReferenceError(err)
}
func SelectERPImportResults(ctx context.Context, id int64, pg page.Query) ([]model.ERPImportResult, int64, error) {
	var rows = []model.ERPImportResult{}
	var total int64
	err := Transaction(ctx, func(tx *gorm.DB) error {
		var j model.ERPImportJob
		if err := importScope(tx.Model(&j)).Where("job_id=?", id).Take(&j).Error; err != nil {
			return erpReferenceError(err)
		}
		q := tx.Model(&model.ERPImportResult{}).Where("job_id=? AND shop_id=?", id, j.ShopID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("source_row").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	})
	return rows, total, err
}

// Re-read function authorization in the same transaction as every imported
// order. The tenant-menu guard intersects employee grants with enabled features.
func requireImportActor(tx *gorm.DB) error {
	var t model.SysTenant
	if err := tx.Where("tenant_id=?", tenantctx.ID(tx.Statement.Context)).Take(&t).Error; err != nil {
		return err
	}
	if t.TenantKind != "01" || t.Status != model.TenantActive || (!t.ExpireTime.IsZero() && !t.ExpireTime.Std().After(time.Now())) {
		return errs.New("导入权限已失效")
	}
	if err := lockERPActor(tx); err != nil {
		return err
	}
	q := tx.Model(&model.SysMenu{}).Where("sys_menu.status='0'")
	if t.OwnerUserID == nil || *t.OwnerUserID != erpctx.UserID(tx.Statement.Context) {
		q = q.Joins("JOIN sys_role_menu rm ON rm.menu_id=sys_menu.menu_id").Joins("JOIN sys_role r ON r.role_id=rm.role_id").Joins("JOIN sys_user_role ur ON ur.role_id=r.role_id").Where("ur.user_id=? AND r.status='0' AND r.del_flag='0'", erpctx.UserID(tx.Statement.Context))
	}
	var perms []string
	if err := q.Distinct().Pluck("sys_menu.perms", &perms).Error; err != nil {
		return err
	}
	for _, p := range perms {
		for _, v := range strings.Split(p, ",") {
			if strings.TrimSpace(v) == "erp:order:import" {
				return nil
			}
		}
	}
	return errs.New("导入权限已失效")
}
func InsertERPImportJob(ctx context.Context, j *model.ERPImportJob) (*model.ERPImportJob, error) {
	var result *model.ERPImportJob
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		// Quota/lifecycle parent -> quota -> actor -> shop -> job.
		if err := lockQuotaTenantRoot(tx, tenantctx.ID(ctx)); err != nil {
			return err
		}
		if _, err := lockedTenantPolicy(tx, tenantctx.ID(ctx)); err != nil {
			return err
		}
		if err := requireImportActor(tx); err != nil {
			return err
		}
		if _, err := lockShopForItem(tx, j.ShopID, true); err != nil {
			return err
		}
		var old model.ERPImportJob
		err := tx.Where("actor_user_id=? AND request_key=?", j.ActorUserID, j.RequestKey).Take(&old).Error
		if err == nil {
			if old.RequestHash != j.RequestHash || old.ShopID != j.ShopID {
				return errs.New("导入请求标识已用于不同文件或店铺")
			}
			result = &old
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var n int64
		if err = tx.Model(&model.ERPImportJob{}).Where("status IN ?", []string{"PENDING", "RUNNING"}).Count(&n).Error; err != nil {
			return err
		}
		if n >= 20 {
			return errs.New("当前企业待处理导入过多，请稍后再试")
		}
		if err = checkFileQuota(tx, j.FileBytes); err != nil {
			return err
		}
		if err = tx.Create(j).Error; err != nil {
			return err
		}
		f := model.SysFile{StorageKey: model.ERPImportFilePrefix + j.FileKey, OriginalName: "order-import.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", SizeBytes: j.FileBytes, UploaderUserID: j.ActorUserID, CreateBy: j.CreateBy, CreateTime: types.Now()}
		if err = tx.Create(&f).Error; err != nil {
			return err
		}
		result = j
		return nil
	}))
	return result, err
}
func ERPImportQueueSize(ctx context.Context) (int64, error) {
	var n int64
	err := DB(tenantctx.System(ctx)).Model(&model.ERPImportJob{}).Where("status IN ?", []string{"PENDING", "RUNNING"}).Count(&n).Error
	return n, err
}
func SetERPImportTotal(ctx context.Context, j model.ERPImportJob, total int64) error {
	return DB(ctx).Model(&model.ERPImportJob{}).Where("job_id=? AND status='RUNNING' AND attempt=?", j.JobID, j.Attempt).Update("total_rows", total).Error
}
func InterruptStaleERPImports(ctx context.Context) error {
	return DB(tenantctx.System(ctx)).Model(&model.ERPImportJob{}).Where("status='RUNNING' AND started_at<?", time.Now().Add(-10*time.Minute)).Updates(map[string]any{"status": "INTERRUPTED", "version": gorm.Expr("version+1"), "finished_at": types.Now(), "update_time": types.Now()}).Error
}
func RetryERPImport(ctx context.Context, id int64, in model.ERPImportRetry, replayHash string) (*model.ERPImportJob, error) {
	var result *model.ERPImportJob
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := requireImportActor(tx); err != nil {
			return err
		}
		var locator model.ERPImportJob
		if err := importScope(tx.Model(&locator)).Where("job_id=?", id).Take(&locator).Error; err != nil {
			return erpReferenceError(err)
		}
		if _, err := lockShopForItem(tx, locator.ShopID, true); err != nil {
			return err
		}
		var j model.ERPImportJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id=?", id).Take(&j).Error; err != nil {
			return err
		}
		var used []string
		if j.Remark != "" {
			if err := json.Unmarshal([]byte(j.Remark), &used); err != nil {
				return errs.New("任务重试记录异常")
			}
		}
		for _, v := range used {
			if v == replayHash {
				result = &j
				return nil
			}
		}
		if j.Version != in.Version || (j.Status != "FAILED" && j.Status != "PARTIAL_FAILED" && j.Status != "INTERRUPTED") {
			return errs.New("任务状态或版本已变化，请刷新")
		}
		if j.FileState != "PRESENT" || !j.RetainUntil.Std().After(time.Now()) {
			return errs.New("原文件已过期，请重新上传")
		}
		if len(used) >= 5 {
			return errs.New("已达5次重试上限，请检查后重新上传")
		}
		var queued int64
		if err := tx.Model(&model.ERPImportJob{}).Where("status IN ?", []string{"PENDING", "RUNNING"}).Count(&queued).Error; err != nil {
			return err
		}
		if queued >= 20 {
			return errs.New("当前企业待处理导入过多，请稍后再试")
		}
		used = append(used, replayHash)
		b, _ := json.Marshal(used)
		if err := tx.Model(&model.ERPImportJob{}).Where("job_id=? AND version=?", id, j.Version).Updates(map[string]any{"status": "PENDING", "version": j.Version + 1, "remark": string(b), "update_time": types.Now()}).Error; err != nil {
			return err
		}
		j.Version++
		j.Status = "PENDING"
		result = &j
		return nil
	}))
	return result, err
}
func InitERPImportRecovery(ctx context.Context) (bool, error) {
	q := DB(tenantctx.System(ctx))
	if !q.Migrator().HasTable(&model.ERPImportJob{}) {
		return false, nil
	}
	err := q.Model(&model.ERPImportJob{}).Where("status IN ?", []string{"RUNNING", "PENDING"}).Updates(map[string]any{"status": "INTERRUPTED", "version": gorm.Expr("version+1"), "finished_at": types.Now(), "update_time": types.Now()}).Error
	return err == nil, err
}
func ClaimERPImportJob(ctx context.Context) (*model.ERPImportJob, error) {
	var result *model.ERPImportJob
	err := Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		var j model.ERPImportJob
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status='PENDING'").Order("job_id").Take(&j).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !j.RetainUntil.Std().After(time.Now()) {
			return tx.Model(&model.ERPImportJob{}).Where("job_id=?", j.JobID).Updates(map[string]any{"status": "FAILED", "version": j.Version + 1, "finished_at": types.Now()}).Error
		}
		if err = tx.Model(&model.ERPImportJob{}).Where("job_id=?", j.JobID).Updates(map[string]any{"status": "RUNNING", "version": j.Version + 1, "attempt": j.Attempt + 1, "started_at": types.Now(), "finished_at": nil}).Error; err != nil {
			return err
		}
		j.Status = "RUNNING"
		j.Version++
		j.Attempt++
		result = &j
		return nil
	})
	return result, err
}

// One order and all its result rows commit together. A revoked actor stops the
// batch, but does not pretend that earlier independent orders rolled back.
func CommitERPImportedOrder(ctx context.Context, j model.ERPImportJob, in model.ERPOrderWrite, hash, invalid string, rows []int64) (*model.ERPOrderResult, error) {
	var result *model.ERPOrderResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := requireImportActor(tx); err != nil {
			return err
		}
		if _, err := lockShopForItem(tx, j.ShopID, true); err != nil {
			return err
		}
		var current model.ERPImportJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id=? AND status='RUNNING' AND attempt=?", j.JobID, j.Attempt).Take(&current).Error; err != nil {
			return err
		}
		var old []model.ERPImportResult
		if err := tx.Where("job_id=? AND source_row IN ?", j.JobID, rows).Find(&old).Error; err != nil {
			return err
		}
		if len(old) == len(rows) && len(old) > 0 && (old[0].ResultCode == "CREATED" || old[0].ResultCode == "EXISTING") {
			return nil
		}
		code, msg := "INVALID", invalid
		if invalid == "" {
			var e error
			result, e = createOrderLocked(tx, in, "IMPORT", hash, j.CreateBy)
			// createOrderLocked returns business conflicts only before its first write;
			// every SQL/audit failure aborts this entire order+result transaction.
			if e != nil {
				if errs.As(e) == nil {
					return e
				}
				code = "CONFLICT"
				msg = "来源订单或请求内容冲突，未覆盖原订单"
			} else {
				code = "CREATED"
				if result.Existing {
					code = "EXISTING"
				}
			}
		}
		if err := tx.Where("job_id=? AND source_row IN ?", j.JobID, rows).Delete(&model.ERPImportResult{}).Error; err != nil {
			return err
		}
		out := make([]model.ERPImportResult, 0, len(rows))
		for _, row := range rows {
			r := model.ERPImportResult{ERPRecord: model.ERPRecord{CreateBy: j.CreateBy, CreateTime: types.Now()}, ShopID: j.ShopID, JobID: j.JobID, SourceRow: row, SourceOrderNo: in.SourceOrderNo, InputHash: hash, ResultCode: code, ErrorSummary: msg, Attempt: j.Attempt}
			if result != nil {
				r.OrderID = &result.OrderID
			}
			out = append(out, r)
		}
		return tx.Create(&out).Error
	}))
	return result, err
}
func FinishERPImportJob(ctx context.Context, j model.ERPImportJob, total int64, interrupted bool) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		type count struct {
			Code string
			N    int64
		}
		var counts []count
		// Each order group's rows share input_hash. Invalid ungroupable rows have a
		// row-specific hash. This counts orders rather than misleading row successes.
		if err := tx.Model(&model.ERPImportResult{}).Select("result_code AS code,COUNT(DISTINCT input_hash) AS n").Where("job_id=?", j.JobID).Group("result_code").Scan(&counts).Error; err != nil {
			return err
		}
		var created, existing, failed int64
		for _, c := range counts {
			switch c.Code {
			case "CREATED":
				created += c.N
			case "EXISTING":
				existing += c.N
			default:
				failed += c.N
			}
		}
		status := "SUCCEEDED"
		if failed > 0 {
			status = "PARTIAL_FAILED"
			if created+existing == 0 {
				status = "FAILED"
			}
		}
		if interrupted {
			status = "INTERRUPTED"
		}
		return tx.Model(&model.ERPImportJob{}).Where("job_id=? AND status='RUNNING' AND attempt=?", j.JobID, j.Attempt).Updates(map[string]any{"status": status, "version": gorm.Expr("version+1"), "total_rows": total, "created_orders": created, "existing_orders": existing, "failed_orders": failed, "finished_at": types.Now(), "update_time": types.Now()}).Error
	})
}
func ExpiredERPImportJobs(ctx context.Context) ([]model.ERPImportJob, error) {
	var jobs []model.ERPImportJob
	err := DB(tenantctx.System(ctx)).Where("file_state='PRESENT' AND retain_until<=? AND status NOT IN ?", types.Now(), []string{"PENDING", "RUNNING"}).Order("retain_until,job_id").Limit(100).Find(&jobs).Error
	return jobs, err
}
func ForgetERPImportFile(ctx context.Context, j model.ERPImportJob) error {
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		// Cleanup must work even after a customer is suspended, without reactivating it.
		var t model.SysTenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=?", j.TenantID).Take(&t).Error; err != nil {
			return err
		}
		if _, err := lockedTenantPolicy(tx, j.TenantID); err != nil {
			return err
		}
		r := tx.Model(&model.ERPImportJob{}).Where("tenant_id=? AND job_id=? AND file_key=? AND file_state='PRESENT' AND retain_until<=? AND status NOT IN ?", j.TenantID, j.JobID, j.FileKey, types.Now(), []string{"PENDING", "RUNNING"}).Updates(map[string]any{"file_state": "DELETED", "version": gorm.Expr("version+1"), "update_time": types.Now()})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		return tx.Where("tenant_id=? AND storage_key=?", j.TenantID, model.ERPImportFilePrefix+j.FileKey).Delete(&model.SysFile{}).Error
	})
}
func ExistingERPImportFileKeys(ctx context.Context, keys []string) ([]string, error) {
	var found []string
	err := DB(tenantctx.System(ctx)).Model(&model.ERPImportJob{}).Where("file_key IN ? AND file_state='PRESENT'", keys).Pluck("file_key", &found).Error
	return found, err
}
