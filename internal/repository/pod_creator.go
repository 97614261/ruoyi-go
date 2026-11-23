package repository

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func creatorQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table("pod_creator_policy p").Joins("JOIN sys_user u ON u.user_id=p.user_id AND u.del_flag='0'")
}

const creatorFields = "p.*,u.user_name,u.nick_name,u.status account_status,(p.valid_until IS NOT NULL AND p.valid_until<=CURRENT_TIMESTAMP(3)) expired"

func PODCreators(ctx context.Context, name, status string, pg page.Query) ([]model.PODCreatorView, int64, error) {
	q := creatorQuery(DB(ctx))
	if name != "" {
		q = q.Where("u.user_name LIKE ? OR u.nick_name LIKE ?", "%"+name+"%", "%"+name+"%")
	}
	if status != "" {
		q = q.Where("p.status=?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODCreatorView{}
	err := q.Select(creatorFields).Order("p.user_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
func PODCreator(ctx context.Context, id int64) (*model.PODCreatorDetail, error) {
	d := &model.PODCreatorDetail{Creator: &model.PODCreatorView{}, ModelIDs: []string{}}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := creatorQuery(tx).Select(creatorFields).Where("p.user_id=?", id).Take(d.Creator).Error; err != nil {
			return creatorMissing(err)
		}
		var ids []int64
		if err := tx.Table("pod_user_model_grant").Where("user_id=? AND enabled=1", id).Order("model_id").Pluck("model_id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			d.ModelIDs = append(d.ModelIDs, strconv.FormatInt(id, 10))
		}
		if err := tx.Where("user_id=?", id).Take(&d.Storage).Error; err != nil {
			return err
		}
		var now time.Time
		if err := tx.Raw("SELECT CURRENT_TIMESTAMP(3)").Scan(&now).Error; err != nil {
			return err
		}
		d.BusinessDate = now.In(types.Location).Format("2006-01-02")
		return tx.Table("pod_user_daily_usage").Where("user_id=? AND business_date=?", id, d.BusinessDate).Select("accepted_submissions").Scan(&d.DailySubmissions).Error
	})
	return d, err
}
func creatorMissing(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errs.WithCode(404, "创作者尚未开通或不存在")
	}
	return err
}
func PODPoints(ctx context.Context, id int64) (*model.PODPointsAccount, error) {
	var a model.PODPointsAccount
	err := DB(ctx).Where("user_id=?", id).Take(&a).Error
	return &a, creatorMissing(err)
}
func PODPointEvents(ctx context.Context, id int64, pg page.Query) ([]model.PODPointsEventView, int64, error) {
	q := DB(ctx).Table("pod_points_event").Where("user_id=?", id)
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODPointsEventView{}
	err := q.Select("event_id,user_id,kind,amount,available_after,held_after,spent_after,reason,actor_user_id,create_time").Order("event_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, n, err
}

// Reads use current DB time. Mutation callers first hold the workspace lock.
func checkCreator(tx *gorm.DB, id int64, write, lock bool) (*model.PODCreatorPolicy, error) {
	var p model.PODCreatorPolicy
	q := tx.Where("user_id=?", id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.WithCode(403, "尚未开通创作资格，请联系管理员")
		}
		return nil, err
	}
	var allowed int64
	if err := tx.Table("sys_user u").Joins("JOIN pod_workspace w ON w.owner_user_id=u.user_id AND w.status='0'").Joins("JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=u.user_id AND m.status='0'").Where("u.user_id=? AND u.status='0' AND u.del_flag='0' AND w.workspace_id=?", id, p.WorkspaceID).Count(&allowed).Error; err != nil {
		return nil, err
	}
	if p.Status != "active" || allowed != 1 {
		return nil, errs.WithCode(403, "创作资格已停用，请联系管理员")
	}
	if write && p.ValidUntil != nil {
		var now time.Time
		if err := tx.Raw("SELECT CURRENT_TIMESTAMP(3)").Scan(&now).Error; err != nil {
			return nil, err
		}
		if !now.Before(*p.ValidUntil) {
			return nil, errs.WithCode(403, "创作资格已到期，可查看和下载已有内容")
		}
	}
	return &p, nil
}
func PODCheckCreator(ctx context.Context, id int64, write bool) error {
	_, err := checkCreator(DB(ctx), id, write, false)
	return err
}
func creatorDenied(err error) bool { b := errs.As(err); return b != nil && b.Code == 403 }
func initCreatorAccounts(tx *gorm.DB, id, workspace int64, status, actor string) error {
	if err := tx.Exec("INSERT INTO pod_creator_policy(user_id,workspace_id,status,max_concurrent_nodes,max_pending_runs,max_daily_submissions,max_storage_bytes,create_by) VALUES(?,?,?,1,10,1000,1073741824,?)", id, workspace, status, actor).Error; err != nil {
		return err
	}
	if err := tx.Exec("INSERT INTO pod_points_account(user_id,create_by) VALUES(?,?)", id, actor).Error; err != nil {
		return err
	}
	return tx.Exec("INSERT INTO pod_storage_account(user_id,create_by) VALUES(?,?)", id, actor).Error
}
func acceptCreatorSubmissions(tx *gorm.DB, id int64, count int) error {
	p, err := checkCreator(tx, id, true, true)
	if err != nil {
		return err
	}
	var now time.Time
	if err = tx.Raw("SELECT CURRENT_TIMESTAMP(3)").Scan(&now).Error; err != nil {
		return err
	}
	day := now.In(types.Location).Format("2006-01-02")
	if err = tx.Exec("INSERT IGNORE INTO pod_user_daily_usage(user_id,business_date,create_by) VALUES(?,?,'pod')", id, day).Error; err != nil {
		return err
	}
	r := tx.Exec("UPDATE pod_user_daily_usage SET accepted_submissions=accepted_submissions+?,update_by='pod',update_time=NOW(3) WHERE user_id=? AND business_date=? AND accepted_submissions<=?", count, id, day, p.MaxDailySubmissions-count)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errs.WithCode(429, "今日提交次数已达上限；失败或取消不返还次数")
	}
	return nil
}

type PODCreatorMutation struct {
	ActorID, ID, Revision                     int64
	Op, Permission, Hash, Trace, PasswordHash string
	Request                                   model.PODCreatorWrite
	Policy                                    model.PODCreatorPolicy
	ModelIDs                                  []int64
	Amount                                    int64
}

func PODCreatorMutate(ctx context.Context, w PODCreatorMutation) (*model.PODCatalogResult, error) {
	out := &model.PODCatalogResult{RequestID: w.Request.RequestID, ActionType: w.Op, ResourceType: "creator", ResourceID: w.ID}
	err := DB(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		var platform model.PODPlatformState
		if err := tx.Where("state_id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Table("pod_platform_state").Take(&platform).Error; err != nil {
			return err
		}
		for _, p := range []string{model.PODAdminAccess, w.Permission} {
			if err := catalogActor(tx, w.ActorID, p); err != nil {
				return err
			}
		}
		var a model.PODAdminAction
		err := tx.Where("actor_user_id=? AND request_id=?", w.ActorID, w.Request.RequestID).Take(&a).Error
		if err == nil {
			if a.ActionType != w.Op || !hmac.Equal([]byte(a.RequestHash), []byte(w.Hash)) {
				return errs.WithCode(409, "请求编号已用于其他内容")
			}
			if a.State != "completed" || json.Unmarshal(a.ResultJSON, out) != nil {
				return errors.New("原操作结果无法确认")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		actor := strconv.FormatInt(w.ActorID, 10)
		before := model.PODCreatorAuditSnapshot{}
		after := model.PODCreatorAuditSnapshot{}
		if w.Op == "creator.create" {
			var role struct{ RoleID int64 }
			if err := tx.Table("sys_role").Select("role_id").Where("role_key='pod_creator' AND status='0' AND del_flag='0'").Take(&role).Error; err != nil {
				return errors.New("专用创作者角色不可用")
			}
			// Only the dedicated role is assigned. No client role IDs or admin permissions.
			u := map[string]any{"dept_id": 100, "user_name": w.Request.UserName, "nick_name": w.Request.NickName, "user_type": "00", "password": w.PasswordHash, "status": "0", "del_flag": "0", "pwd_update_date": time.Now(), "create_by": actor, "create_time": time.Now()}
			if err := tx.Table("sys_user").Create(u).Error; err != nil {
				return err
			}
			var id int64
			if err := tx.Table("sys_user").Where("user_name=?", w.Request.UserName).Pluck("user_id", &id).Error; err != nil {
				return err
			}
			out.ResourceID = id
			if err := tx.Exec("INSERT INTO sys_user_role(user_id,role_id) VALUES(?,?)", id, role.RoleID).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO pod_workspace(owner_user_id,name,create_by,update_by) VALUES(?,?,?,?)", id, w.Request.NickName+"的工作空间", actor, actor).Error; err != nil {
				return err
			}
			var ws int64
			if err := tx.Table("pod_workspace").Where("owner_user_id=?", id).Pluck("workspace_id", &ws).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO pod_workspace_member(workspace_id,user_id,create_by,update_by) VALUES(?,?,?,?)", ws, id, actor, actor).Error; err != nil {
				return err
			}
			if err := initCreatorAccounts(tx, id, ws, w.Policy.Status, actor); err != nil {
				return err
			}
			w.ID = id
		}
		var ws int64
		if err := tx.Table("pod_creator_policy").Where("user_id=?", w.ID).Pluck("workspace_id", &ws).Error; err != nil {
			return err
		}
		if ws <= 0 {
			return errs.WithCode(404, "创作者不存在")
		}
		var workspace model.PODWorkspace
		if err := tx.Table("pod_workspace").Where("workspace_id=?", ws).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&workspace).Error; err != nil {
			return err
		}
		var p model.PODCreatorPolicy
		if err := tx.Where("user_id=?", w.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&p).Error; err != nil {
			return err
		}
		if w.Op != "creator.create" {
			copy := p
			before.Policy = &copy
		}
		if w.Op == "creator.models" {
			before.ModelIDs = []string{}
			var ids []int64
			if err := tx.Table("pod_user_model_grant").Where("user_id=? AND enabled=1", w.ID).Order("model_id").Pluck("model_id", &ids).Error; err != nil {
				return err
			}
			for _, id := range ids {
				before.ModelIDs = append(before.ModelIDs, strconv.FormatInt(id, 10))
			}
		}
		if w.Op != "points.adjust" && w.Op != "creator.create" && (p.Revision != w.Revision || p.Revision == math.MaxInt64) {
			return errs.WithCode(409, "用户权益已变更，请刷新后重试")
		}
		switch w.Op {
		case "creator.create", "creator.policy":
			if w.Op != "creator.create" {
				p.Revision++
			}
			p.Status = w.Policy.Status
			p.ValidUntil = w.Policy.ValidUntil
			p.MaxConcurrentNodes = w.Policy.MaxConcurrentNodes
			p.MaxPendingRuns = w.Policy.MaxPendingRuns
			p.MaxDailySubmissions = w.Policy.MaxDailySubmissions
			p.MaxStorageBytes = w.Policy.MaxStorageBytes
			if err := tx.Model(&p).Select("status", "valid_until", "max_concurrent_nodes", "max_pending_runs", "max_daily_submissions", "max_storage_bytes", "revision").Updates(&p).Error; err != nil {
				return err
			}
			out.Revision = p.Revision
		case "creator.models":
			if len(w.ModelIDs) > 0 {
				var count int64
				if err := tx.Table("pod_model").Where("model_id IN ?", w.ModelIDs).Count(&count).Error; err != nil {
					return err
				}
				if count != int64(len(w.ModelIDs)) {
					return errs.WithCode(400, "模型范围含不存在的记录")
				}
			}
			if err := tx.Table("pod_user_model_grant").Where("user_id=?", w.ID).Updates(map[string]any{"enabled": false, "revision": gorm.Expr("revision+1"), "update_by": actor, "update_time": time.Now()}).Error; err != nil {
				return err
			}
			for _, id := range w.ModelIDs {
				if err := tx.Exec("INSERT INTO pod_user_model_grant(user_id,model_id,enabled,create_by) VALUES(?,?,1,?) ON DUPLICATE KEY UPDATE enabled=1,update_by=VALUES(create_by),update_time=NOW(3)", w.ID, id, actor).Error; err != nil {
					return err
				}
			}
			p.Revision++
			if err := tx.Model(&p).Update("revision", p.Revision).Error; err != nil {
				return err
			}
			after.ModelIDs = *w.Request.ModelIDs
			out.Revision = p.Revision
		case "points.adjust":
			var a model.PODPointsAccount
			if err := tx.Where("user_id=?", w.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&a).Error; err != nil {
				return err
			}
			if a.Revision != w.Revision || a.Revision == math.MaxInt64 {
				return errs.WithCode(409, "点数账户已变更，请刷新")
			}
			copy := a
			before.Points = &copy
			if !validPODPoints(a) {
				return errors.New("points account invariant failed")
			}
			if w.Request.Kind == "grant" {
				if a.Available > math.MaxInt64-w.Amount || a.GrantedTotal > math.MaxInt64-w.Amount {
					return errs.WithCode(400, "点数累计值超出范围")
				}
				a.Available += w.Amount
				a.GrantedTotal += w.Amount
			} else {
				if w.Amount > a.Available {
					return errs.WithCode(409, "可用点数不足，不能扣减预占部分")
				}
				if a.DeductedTotal > math.MaxInt64-w.Amount {
					return errors.New("points total overflow")
				}
				a.Available -= w.Amount
				a.DeductedTotal += w.Amount
			}
			a.Revision++
			if err := tx.Model(&a).Updates(map[string]any{"available": a.Available, "granted_total": a.GrantedTotal, "deducted_total": a.DeductedTotal, "revision": a.Revision, "update_by": actor, "update_time": time.Now()}).Error; err != nil {
				return err
			}
			event := map[string]any{"user_id": w.ID, "workspace_id": ws, "event_key": "admin:" + strconv.FormatInt(w.ActorID, 10) + ":" + w.Request.RequestID, "kind": w.Request.Kind, "amount": w.Amount, "available_after": a.Available, "held_after": a.Held, "spent_after": a.Spent, "reason": w.Request.Reason, "actor_user_id": w.ActorID, "create_by": actor}
			if err := tx.Table("pod_points_event").Create(event).Error; err != nil {
				return err
			}
			after.Points = &a
			out.Revision = a.Revision
		default:
			return errs.WithCode(400, "用户管理操作无效")
		}
		if err := tx.Table("pod_creator_policy").Where("user_id=?", w.ID).Updates(map[string]any{"update_by": actor, "update_time": time.Now()}).Error; err != nil {
			return err
		}
		after.Policy = &p
		if err := tx.Create(&model.PODAdminAudit{ActorUserID: w.ActorID, OwnerUserID: w.ID, ActionType: w.Op, ResourceType: "creator", ResourceID: out.ResourceID, Reason: w.Request.Reason, BeforeJSON: podJSON(model.PODAdminAuditSnapshot{Creator: &before}), AfterJSON: podJSON(model.PODAdminAuditSnapshot{Creator: &after}), TraceID: w.Trace, Outcome: "success", CreateBy: actor, CreateTime: types.Now()}).Error; err != nil {
			return err
		}
		return tx.Create(&model.PODAdminAction{ActorUserID: w.ActorID, RequestID: w.Request.RequestID, RequestHash: w.Hash, ActionType: w.Op, ResourceType: "creator", ResourceID: out.ResourceID, State: "completed", ResultJSON: podJSON(out), CreateBy: actor, CreateTime: types.Now()}).Error
	})
	if err != nil {
		if e := PODCatalogRejected(ctx, w.ActorID, w.Op, w.Trace); e != nil {
			return nil, errors.New("用户管理失败且审计不可用，请核对原操作")
		}
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			return nil, errs.WithCode(409, "账号已存在或操作冲突")
		}
	}
	return out, err
}

func validPODPoints(a model.PODPointsAccount) bool {
	if a.Available < 0 || a.Held < 0 || a.Spent < 0 || a.GrantedTotal < 0 || a.DeductedTotal < 0 || a.Revision < 1 || a.DeductedTotal > a.GrantedTotal {
		return false
	}
	n := a.GrantedTotal - a.DeductedTotal
	return a.Available <= n && a.Held <= n-a.Available && a.Spent == n-a.Available-a.Held
}

// Grant operators see identities and status, never costs, credentials or private content.
func PODCreatorModelOptions(ctx context.Context, name string, pg page.Query) ([]model.PODCreatorModelOption, int64, error) {
	q := DB(ctx).Table("pod_model m").Joins("JOIN pod_provider p ON p.provider_id=m.provider_id")
	if name != "" {
		q = q.Where("m.name LIKE ? OR m.model_code LIKE ?", "%"+name+"%", "%"+name+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODCreatorModelOption{}
	err := q.Select("m.model_id,m.name,m.kind,m.status,p.status provider_status").Order("m.sort,m.model_id").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, total, err
}
