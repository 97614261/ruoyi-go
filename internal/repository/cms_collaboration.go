package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
	"strconv"
	"strings"
	"time"
)

type cmsActorKey struct{}

func WithCMSActor(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, cmsActorKey{}, id)
}
func cmsActor(ctx context.Context) int64 { id, _ := ctx.Value(cmsActorKey{}).(int64); return id }

type cmsAccess struct {
	ID    int64
	Name  string
	Perms map[string]bool
}

func (a cmsAccess) has(p string) bool { return a.Perms[p] || a.Perms[model.AllPermission] }
func (a cmsAccess) source(kind, action string) bool {
	return a.has("cms:" + strings.ToLower(kind) + ":" + action)
}
func cmsCurrentAccess(tx *gorm.DB, id int64) (cmsAccess, error) {
	a := cmsAccess{ID: id, Perms: map[string]bool{}}
	var u struct {
		UserId   int64
		UserName string
		Status   string
		DelFlag  string
	}
	if e := tx.Table("sys_user").Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=?", id).Take(&u).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return a, errs.ErrForbidden
		}
		return a, e
	}
	if u.Status != "0" || u.DelFlag != "0" {
		return a, errs.ErrForbidden
	}
	a.Name = u.UserName
	if id == model.AdminUserID {
		a.Perms[model.AllPermission] = true
		return a, nil
	}
	var rows []struct{ Perms string }
	e := tx.Table("sys_user_role ur").Select("m.perms").Joins("JOIN sys_role r ON r.role_id=ur.role_id AND r.status='0' AND r.del_flag='0'").Joins("JOIN sys_role_menu rm ON rm.role_id=r.role_id").Joins("JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0'").Where("ur.user_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&rows).Error
	for _, r := range rows {
		for _, p := range strings.Split(r.Perms, ",") {
			a.Perms[strings.TrimSpace(p)] = true
		}
	}
	return a, e
}

// Permission expressions take only server-built constants, never request column/SQL fragments.
func cmsUserPermission(db *gorm.DB, perm string) *gorm.DB {
	return db.Where("(u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.role_id=ur.role_id AND r.status='0' AND r.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET(?,m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0)))", perm)
}
func cmsPeopleDB(tx *gorm.DB, purpose, kind string) *gorm.DB {
	q := tx.Table("sys_user u").Where("u.status='0' AND u.del_flag='0'")
	if purpose == "review" {
		q = cmsUserPermission(q, "cms:"+strings.ToLower(kind)+":query")
		return cmsUserPermission(q, "cms:"+strings.ToLower(kind)+":review")
	}
	q = cmsUserPermission(q, "cms:inquiry:list")
	q = cmsUserPermission(q, "cms:inquiry:query")
	return cmsUserPermission(q, "cms:inquiry:edit")
}
func CMSWorkflowPeople(ctx context.Context, q model.CMSPeopleQuery, pg page.Query) ([]model.CMSWorkflowPerson, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return nil, 0, e
	}
	if !(a.has("cms:workflow:edit") || (q.Purpose == "inquiry" && a.has("cms:inquiry:assign"))) {
		return nil, 0, errs.ErrForbidden
	}
	db := cmsPeopleDB(tx, q.Purpose, q.Kind)
	if q.Name != "" {
		db = db.Where("(u.user_name LIKE ? OR u.nick_name LIKE ?)", q.Name+"%", q.Name+"%")
	}
	var n int64
	if e = db.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSWorkflowPerson{}
	e = db.Select("u.user_id,u.user_name,u.nick_name").Order("u.user_id").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, n, e
}
func cmsWorkflowSettings(tx *gorm.DB) (model.CMSWorkflowSettings, error) {
	var s model.CMSWorkflowSettings
	e := tx.Model(&model.CMSCollaborationSetting{}).Where("setting_id=1").Take(&s.CMSCollaborationSetting).Error
	if e == nil {
		e = tx.Order("kind").Find(&s.Policies).Error
	}
	return s, e
}
func CMSWorkflowSettings(ctx context.Context) (model.CMSWorkflowSettings, error) {
	return cmsWorkflowSettings(cmsDB(ctx))
}
func cmsEvent(tx *gorm.DB, reviewID int64, kind string, objectID int64, action, reason, details string) error {
	return tx.Create(&model.CMSReviewEvent{ReviewId: reviewID, Kind: kind, ObjectId: objectID, Action: action, ActorUserId: cmsActor(tx.Statement.Context), Reason: reason, Details: details, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: "workflow", CreateTime: types.Now()}}).Error
}
func CMSSetWorkflow(ctx context.Context, in model.CMSWorkflowInput) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsCurrentAccess(tx, cmsActor(ctx))
		if e != nil {
			return e
		}
		if !a.has("cms:workflow:edit") {
			return errs.ErrForbidden
		}
		s, e := cmsWorkflowSettings(tx)
		if e != nil {
			return e
		}
		if s.Version != in.Version {
			return errs.New("协作设置已变化，请刷新")
		}

		var available []struct {
			Kind  string
			Total int64
		}
		if e = tx.Raw(`SELECT p.kind,COUNT(*) AS total FROM cms_review_policy p CROSS JOIN sys_user u WHERE u.status='0' AND u.del_flag='0' AND u.user_id<>? AND (u.user_id=1 OR (EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.role_id=ur.role_id AND r.status='0' AND r.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET(CONCAT('cms:',LOWER(p.kind),':query'),m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0)) AND EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.role_id=ur.role_id AND r.status='0' AND r.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET(CONCAT('cms:',LOWER(p.kind),':review'),m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0)))) GROUP BY p.kind`, a.ID).Scan(&available).Error; e != nil {
			return e
		}
		has := map[string]bool{}
		for _, v := range available {
			has[v.Kind] = v.Total > 0
		}
		for _, p := range s.Policies {
			if in.Required[p.Kind] && !p.Required && !has[p.Kind] {
				return errs.New(p.Kind + "需要另一名启用且具备查询/审核权限的审核人")
			}
		}
		if len(in.ManagerIds) > 0 {
			var n int64
			if e = cmsPeopleDB(tx, "inquiry", "").Where("u.user_id IN ?", in.ManagerIds).Count(&n).Error; e != nil {
				return e
			}
			if n != int64(len(in.ManagerIds)) {
				return errs.New("询盘接收人必须启用并具有列表、详情和处理权限")
			}
			if in.InquiryIsolation {
				q := cmsPeopleDB(tx, "inquiry", "").Where("u.user_id IN ?", in.ManagerIds)
				// A union of the two explicit full-scope permissions; no automatic grants.
				base := tx.Session(&gorm.Session{NewDB: true}).Table("sys_user u")
				left := cmsUserPermission(base, "cms:inquiry:all")
				right := cmsUserPermission(tx.Session(&gorm.Session{NewDB: true}).Table("sys_user u"), "cms:inquiry:assign")
				q = q.Where(tx.Where(left).Or(right))
				if e = q.Count(&n).Error; e != nil {
					return e
				}
				if n != int64(len(in.ManagerIds)) {
					return errs.New("启用隔离时，未分配询盘接收人须有全部范围或指派权限")
				}
			}

		}
		before, _ := json.Marshal(s)
		affected := []string{}
		for _, p := range s.Policies {
			if p.Required != in.Required[p.Kind] {
				affected = append(affected, p.Kind)
			}
		}
		// Fixed four policy rows are updated as one statement, not N per-policy writes/reads.
		if len(affected) > 0 {
			enabled := []string{}
			for k, v := range in.Required {
				if v {
					enabled = append(enabled, k)
				}
			}
			expr := gorm.Expr("0")
			if len(enabled) > 0 {
				expr = gorm.Expr("kind IN ?", enabled)
			}
			if e = tx.Model(&model.CMSReviewPolicy{}).Where("kind IN ?", affected).Updates(map[string]any{"required": expr, "version": gorm.Expr("version+1"), "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
				return e
			}
			if e = cmsInvalidateReview(tx, affected, 0, "审核策略变化"); e != nil {
				return e
			}
			if e = tx.Model(&model.CMSSchedule{}).Where("kind IN ? AND status='PENDING' AND action='publish'", affected).Updates(map[string]any{"status": "CANCELED", "result_message": "审核策略变化，请重新安排", "version": gorm.Expr("version+1"), "finished_at": types.Now(), "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
				return e
			}
		}
		managers, _ := json.Marshal(in.ManagerIds)
		if e = tx.Model(&model.CMSCollaborationSetting{}).Where("setting_id=1 AND version=?", in.Version).Updates(map[string]any{"inquiry_isolation": in.InquiryIsolation, "manager_ids": string(managers), "version": in.Version + 1, "update_by": a.Name, "update_time": types.Now(), "remark": in.Reason}).Error; e != nil {
			return e
		}
		after, _ := json.Marshal(in)
		return cmsEvent(tx, 0, "POLICY", 0, "POLICY_CHANGED", in.Reason, string(before)+"\n"+string(after))
	})
}
func CMSWorkflowHistory(ctx context.Context, pg page.Query) ([]model.CMSReviewEvent, int64, error) {
	return cmsEventPage(cmsDB(ctx).Where("review_id=0"), pg)
}
func cmsEventPage(q *gorm.DB, pg page.Query) ([]model.CMSReviewEvent, int64, error) {
	var n int64
	if e := q.Model(&model.CMSReviewEvent{}).Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSReviewEvent{}
	e := q.Order("event_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, n, e
}
func cmsInvalidateReview(tx *gorm.DB, kinds []string, objectID int64, reason string) error {
	predicate := "kind IN ? AND status IN ('PENDING','APPROVED')"
	args := []any{kinds}
	if objectID > 0 {
		predicate += " AND object_id=?"
		args = append(args, objectID)
	}
	values := []any{cmsActor(tx.Statement.Context), reason, types.Now()}
	values = append(values, args...)
	if e := tx.Exec("INSERT INTO cms_review_event(review_id,kind,object_id,action,actor_user_id,reason,details,create_by,create_time,update_by,remark) SELECT review_id,kind,object_id,'STALE',?,?,'','workflow',?,'','' FROM cms_review_request WHERE "+predicate, values...).Error; e != nil {
		return e
	}
	return tx.Model(&model.CMSReviewRequest{}).Where(predicate, args...).Updates(map[string]any{"status": "STALE", "opinion": reason, "version": gorm.Expr("version+1"), "update_by": "workflow", "update_time": types.Now()}).Error
}
func cmsRecordDraftActor(tx *gorm.DB, kind string, objectID int64, key string, operator string) error {
	row := model.CMSDraftActor{Kind: kind, ObjectId: objectID, SnapshotKey: key, AuthorUserId: cmsActor(tx.Statement.Context), CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: operator, CreateTime: types.Now()}}
	if e := tx.Create(&row).Error; e != nil {
		return e
	}
	return cmsInvalidateReview(tx, []string{kind}, objectID, "工作稿已修改，请重新送审")
}
func cmsSnapshot(tx *gorm.DB, kind string, id int64) (string, int64, model.CMSReviewSnapshot, error) {
	out := model.CMSReviewSnapshot{}
	if kind == "SITE" {
		var s model.CMSSite
		if id != 1 {
			return "", 0, out, errs.ErrNotFound
		}
		if e := tx.Where("site_id=1").Take(&s).Error; e != nil {
			return "", 0, out, e
		}
		var cfg model.SiteConfig
		if e := json.Unmarshal([]byte(s.DraftJSON), &cfg); e != nil {
			return "", 0, out, e
		}
		out.Site = &cfg
		return strconv.FormatInt(s.Version, 10), s.Version, out, nil
	}
	c, e := cmsFindContent(tx, kind, id)
	if e != nil {
		return "", 0, out, e
	}
	if c.WorkingRevisionId == nil {
		return "", 0, out, errs.New("请先保存草稿")
	}
	var r model.CMSRevision
	if e = tx.Where("revision_id=? AND content_id=?", *c.WorkingRevisionId, id).Take(&r).Error; e != nil {
		return "", 0, out, e
	}
	if e = cmsAttachOps(tx, &r); e != nil {
		return "", 0, out, e
	}
	out.Content = &model.CMSContentDetail{CMSContent: c, Draft: r}
	out.Operations = r.Ops
	return strconv.FormatInt(r.RevisionId, 10), c.Version, out, nil
}
func cmsSnapshotJSON(s model.CMSReviewSnapshot) (string, string, error) {
	b, e := json.Marshal(s)
	return string(b), fmt.Sprintf("%x", sha256.Sum256(b)), e
}
func CMSSubmitReview(ctx context.Context, in model.CMSReviewInput) (int64, error) {
	var id int64
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsCurrentAccess(tx, cmsActor(ctx))
		if e != nil {
			return e
		}
		if !a.source(in.Kind, "query") || !a.source(in.Kind, "edit") {
			return errs.ErrForbidden
		}
		var p model.CMSReviewPolicy
		if e = tx.Where("kind=?", in.Kind).Take(&p).Error; e != nil {
			return e
		}
		if !p.Required {
			return errs.New("该模块为直接发布，无需送审")
		}
		key, version, snapshot, e := cmsSnapshot(tx, in.Kind, in.ObjectId)
		if e != nil {
			return e
		}
		if version != in.ContentVersion {
			return errs.New("工作稿已变化，请刷新")
		}
		var author model.CMSDraftActor
		if e = tx.Where("kind=? AND snapshot_key=? AND object_id=?", in.Kind, key, in.ObjectId).Take(&author).Error; errors.Is(e, gorm.ErrRecordNotFound) {
			return errs.New("历史工作稿缺少可靠保存人，请显式保存一版再送审")
		} else if e != nil {
			return e
		}
		if author.AuthorUserId == 0 {
			return errs.New("请由已登录编辑显式保存一版再送审")
		}
		var n int64
		if e = tx.Model(&model.CMSReviewRequest{}).Where("kind=? AND object_id=? AND status IN ('PENDING','APPROVED')", in.Kind, in.ObjectId).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return errs.New("已有有效送审申请，请先处理或撤回")
		}
		var recipients []int64
		if e = cmsPeopleDB(tx, "review", in.Kind).Where("u.user_id NOT IN ?", []int64{a.ID, author.AuthorUserId}).Order("u.user_id").Limit(100).Pluck("u.user_id", &recipients).Error; e != nil {
			return e
		}
		if len(recipients) == 0 {
			return errs.New("当前没有可审核这版稿件的其他审核人")
		}
		body, hash, e := cmsSnapshotJSON(snapshot)
		if e != nil {
			return e
		}
		row := model.CMSReviewRequest{Kind: in.Kind, ObjectId: in.ObjectId, SnapshotKey: key, ContentVersion: version, SnapshotHash: hash, SnapshotJSON: body, AuthorUserId: author.AuthorUserId, SubmitterUserId: a.ID, PolicyVersion: p.Version, Status: "PENDING", Version: 1, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		id = row.ReviewId
		if e = cmsEvent(tx, id, row.Kind, row.ObjectId, "SUBMITTED", "", ""); e != nil {
			return e
		}
		return cmsEnqueue(tx, "REVIEW", id, row.Kind, "REVIEW_SUBMITTED", row.Version, recipients, "有一份工作稿等待审核")
	})
	return id, e
}
func cmsReviewVisible(q *gorm.DB, a cmsAccess) *gorm.DB {
	kinds := []string{}
	for _, k := range []string{"ARTICLE", "PRODUCT", "PAGE", "SITE"} {
		if a.source(k, "query") {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		return q.Where("1=0")
	}
	q = q.Where("cms_review_request.kind IN ?", kinds)
	reviewKinds := []string{}
	for _, k := range kinds {
		if a.source(k, "review") {
			reviewKinds = append(reviewKinds, k)
		}
	}
	if len(reviewKinds) == 0 {
		return q.Where("submitter_user_id=? OR author_user_id=?", a.ID, a.ID)
	}
	return q.Where("(submitter_user_id=? OR author_user_id=? OR cms_review_request.kind IN ?)", a.ID, a.ID, reviewKinds)
}
func CMSReviewPage(ctx context.Context, in model.CMSReviewQuery, pg page.Query) ([]model.CMSReviewRequest, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return nil, 0, e
	}
	q := cmsReviewVisible(tx.Model(&model.CMSReviewRequest{}), a)
	if in.Kind != "" {
		q = q.Where("kind=?", in.Kind)
	}
	if in.Status != "" {
		q = q.Where("status=?", in.Status)
	}
	if in.ObjectId > 0 {
		q = q.Where("object_id=?", in.ObjectId)
	}
	if in.Mine {
		q = q.Where("submitter_user_id=?", a.ID)
	}
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSReviewRequest{}
	e = q.Omit("snapshot_json").Order("review_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}
func cmsReviewByID(tx *gorm.DB, a cmsAccess, id int64) (model.CMSReviewRequest, error) {
	var r model.CMSReviewRequest
	e := cmsReviewVisible(tx.Model(&model.CMSReviewRequest{}), a).Where("review_id=?", id).Take(&r).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return r, errs.ErrNotFound
	}
	return r, e
}
func CMSReviewDetail(ctx context.Context, id int64) (model.CMSReviewDetail, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return model.CMSReviewDetail{}, e
	}
	r, e := cmsReviewByID(tx, a, id)
	if e != nil {
		return model.CMSReviewDetail{}, e
	}
	out := model.CMSReviewDetail{CMSReviewRequest: r, Events: []model.CMSReviewEvent{}}
	if e = json.Unmarshal([]byte(r.SnapshotJSON), &out.Snapshot); e != nil {
		return out, e
	}
	key, v, _, e := cmsSnapshot(tx, r.Kind, r.ObjectId)
	if e != nil && errs.As(e) == nil {
		return out, e
	}
	out.IsCurrent = e == nil && key == r.SnapshotKey && v == r.ContentVersion
	return out, nil
}
func CMSReviewEvents(ctx context.Context, id int64, pg page.Query) ([]model.CMSReviewEvent, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return nil, 0, e
	}
	if _, e = cmsReviewByID(tx, a, id); e != nil {
		return nil, 0, e
	}
	return cmsEventPage(tx.Where("review_id=?", id), pg)
}
func CMSDecideReview(ctx context.Context, id int64, in model.CMSReviewDecision) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsCurrentAccess(tx, cmsActor(ctx))
		if e != nil {
			return e
		}
		r, e := cmsReviewByID(tx, a, id)
		if e != nil {
			return e
		}
		if r.Version != in.Version {
			return errs.New("审核申请已处理或变化，请刷新")
		}
		status := ""
		if in.Action == "withdraw" {
			if r.SubmitterUserId != a.ID || (r.Status != "PENDING" && r.Status != "APPROVED") {
				return errs.ErrForbidden
			}
			status = "WITHDRAWN"
		} else {
			if !a.source(r.Kind, "review") || a.ID == r.SubmitterUserId || a.ID == r.AuthorUserId {
				return errs.New("提交人和实际保存人不能审核自己的版本")
			}
			if r.Status != "PENDING" {
				return errs.New("审核申请已处理，请刷新")
			}
			var p model.CMSReviewPolicy
			if e = tx.Where("kind=?", r.Kind).Take(&p).Error; e != nil {
				return e
			}
			key, v, _, e := cmsSnapshot(tx, r.Kind, r.ObjectId)
			if e != nil {
				return e
			}
			if !p.Required || p.Version != r.PolicyVersion || key != r.SnapshotKey || v != r.ContentVersion {
				return errs.New("审核策略或工作稿已变化，请重新送审")
			}
			if in.Action == "approve" {
				status = "APPROVED"
			} else {
				status = "REJECTED"
			}
		}
		stamp := ""
		if status == "APPROVED" {
			stamp, e = cmsReviewPermissionStamp(ctx, a.ID)
			if e != nil {
				return e
			}
		}
		result := tx.Model(&model.CMSReviewRequest{}).Where("review_id=? AND version=?", id, in.Version).Updates(map[string]any{"remark": stamp, "status": status, "opinion": in.Opinion, "reviewer_user_id": a.ID, "version": r.Version + 1, "update_by": a.Name, "update_time": types.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errs.New("审核已被他人处理，请刷新")
		}
		if e = cmsEvent(tx, id, r.Kind, r.ObjectId, strings.ToUpper(in.Action), in.Opinion, ""); e != nil {
			return e
		}
		recipients := []int64{r.SubmitterUserId}
		if r.AuthorUserId != r.SubmitterUserId {
			recipients = append(recipients, r.AuthorUserId)
		}
		return cmsEnqueue(tx, "REVIEW", id, r.Kind, "REVIEW_RESULT", r.Version+1, recipients, "送审工作稿已有处理结果")
	})
}

// Called inside the same publication transaction, including batch/scheduler/site paths.
func cmsRequireApproval(tx *gorm.DB, kind string, id int64, consume bool) error {
	var p model.CMSReviewPolicy
	if e := tx.Where("kind=?", kind).Take(&p).Error; e != nil {
		return e
	}
	if !p.Required {
		return nil
	}
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return e
	}
	if !a.source(kind, "publish") {
		return errs.ErrForbidden
	}
	key, v, snapshot, e := cmsSnapshot(tx, kind, id)
	if e != nil {
		return e
	}
	_, hash, e := cmsSnapshotJSON(snapshot)
	if e != nil {
		return e
	}
	var r model.CMSReviewRequest
	e = tx.Where("kind=? AND object_id=? AND snapshot_key=? AND content_version=? AND snapshot_hash=? AND policy_version=? AND status='APPROVED'", kind, id, key, v, hash, p.Version).Take(&r).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return errs.New("当前工作稿尚未通过审核，请送审后发布")
	}
	if e != nil {
		return e
	}
	reviewer, e := cmsCurrentAccess(tx, r.ReviewerUserId)
	if e != nil {
		return errs.New("原审核人已停用或撤权，请重新送审")
	}
	if reviewer.ID == r.AuthorUserId || reviewer.ID == r.SubmitterUserId || !reviewer.source(kind, "query") || !reviewer.source(kind, "review") {
		return errs.New("原审核人已无有效审核权限，请撤回后重新送审")
	}
	stamp, e := cmsReviewPermissionStamp(tx.Statement.Context, r.ReviewerUserId)
	if e != nil {
		return e
	}
	if r.Remark != stamp {
		return errs.New("审核人权限版本已变化，请撤回后重新送审")
	}
	if !consume {
		return nil
	}
	res := tx.Model(&model.CMSReviewRequest{}).Where("review_id=? AND version=? AND status='APPROVED'", r.ReviewId, r.Version).Updates(map[string]any{"status": "PUBLISHED", "version": r.Version + 1, "update_by": a.Name, "update_time": types.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errs.New("批准已被使用，请重新审核")
	}
	return cmsEvent(tx, r.ReviewId, kind, id, "PUBLISHED", "本次批准已消费", "")
}

func CMSRestoreSiteDraft(ctx context.Context, in model.SiteAction) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsCurrentAccess(tx, cmsActor(ctx))
		if e != nil {
			return e
		}
		if !a.source("SITE", "query") || !a.source("SITE", "edit") {
			return errs.ErrForbidden
		}
		var site model.CMSSite
		if e = tx.Where("site_id=1").Take(&site).Error; e != nil {
			return e
		}
		if site.Version != in.Version {
			return errs.New("站点已变化，请刷新")
		}
		var old model.SiteRelease
		if e = tx.Where("site_id=1 AND release_id=?", in.ReleaseId).Take(&old).Error; e != nil {
			return errs.New("外观历史版本不存在")
		}
		var cfg model.SiteConfig
		if e = json.Unmarshal([]byte(old.ConfigJSON), &cfg); e != nil {
			return e
		}
		if e = siteValidateRefs(tx, cfg, false); e != nil {
			return e
		}
		if e = siteSaveRefs(tx, cfg, "SITE_DRAFT", 1, a.Name); e != nil {
			return e
		}
		if e = cmsRecordDraftActor(tx, "SITE", 1, strconv.FormatInt(site.Version+1, 10), a.Name); e != nil {
			return e
		}
		return tx.Model(&model.CMSSite{}).Where("site_id=1 AND version=?", in.Version).Updates(map[string]any{"draft_json": old.ConfigJSON, "version": site.Version + 1, "update_by": a.Name, "update_time": types.Now()}).Error
	})
}

// Reuse the existing durable permission generation; revocation and regrant never resurrect old approvals.
func cmsReviewPermissionStamp(ctx context.Context, id int64) (string, error) {
	limited, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	version, e := redisx.C().Get(limited, redisx.LoginUserPermissionVersionKey(id)).Int64()
	if errors.Is(e, redis.Nil) {
		e = nil
	}
	if e != nil {
		return "", e
	}
	return "permission-version:" + strconv.FormatInt(version, 10), nil
}

func CMSWorkflowImpact(ctx context.Context) ([]model.CMSWorkflowImpact, error) {
	rows := []model.CMSWorkflowImpact{}
	e := cmsDB(ctx).Table("cms_review_policy p").Select("p.kind,COALESCE(r.pending,0) AS pending,COALESCE(r.approved,0) AS approved,COALESCE(s.scheduled,0) AS scheduled").Joins("LEFT JOIN (SELECT kind,SUM(status='PENDING') AS pending,SUM(status='APPROVED') AS approved FROM cms_review_request WHERE status IN ('PENDING','APPROVED') GROUP BY kind) r ON r.kind=p.kind").Joins("LEFT JOIN (SELECT kind,COUNT(*) AS scheduled FROM cms_schedule WHERE status='PENDING' AND action='publish' GROUP BY kind) s ON s.kind COLLATE utf8mb4_unicode_ci=p.kind").Order("p.kind").Scan(&rows).Error
	return rows, e
}
