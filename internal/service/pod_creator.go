package service

import (
	"context"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var creatorUsername = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{2,19}$`)

func validateCreatorWrite(op string, id int64, r model.PODCreatorWrite, l model.PODExecutionLimits) (repository.PODCreatorMutation, error) {
	w := repository.PODCreatorMutation{ID: id, Op: op, Request: r, Permission: CatalogPermission(op)}
	u, err := uuid.Parse(r.RequestID)
	if err != nil || u == uuid.Nil || u.String() != r.RequestID || !catalogText(r.Reason, 200) {
		return w, errs.WithCode(400, "请求编号或操作原因无效")
	}
	allowed := "requestId reason "
	switch op {
	case "creator.create":
		allowed += "userName nickName initialPassword policy"
	case "creator.policy":
		allowed += "revision policy"
	case "creator.models":
		allowed += "revision modelIds"
	case "points.adjust":
		allowed += "accountRevision kind amount"
	default:
		return w, errs.WithCode(400, "操作无效")
	}
	v := reflect.ValueOf(r)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if !v.Field(i).IsZero() && !strings.Contains(" "+allowed+" ", " "+key+" ") {
			return w, errs.WithCode(400, "本操作包含不支持的字段")
		}
	}
	if op != "creator.create" {
		raw := r.Revision
		if op == "points.adjust" {
			raw = r.AccountRevision
		}
		w.Revision, err = PODPositiveID(raw)
		if err != nil || id <= 0 {
			return w, errs.WithCode(400, "用户编号或版本无效")
		}
	}
	if op == "creator.create" {
		if !creatorUsername.MatchString(r.UserName) || !catalogText(r.NickName, 30) || len([]rune(r.InitialPassword)) < 8 || !validPasswordLength(r.InitialPassword) || strings.Contains(r.Reason, r.InitialPassword) {
			return w, errs.WithCode(400, "账号须为 3—20 位字母开头的字母数字或 _-，昵称 1—30 字，初始密码 8—20 字且不能写入原因")
		}
	}
	if op == "creator.create" || op == "creator.policy" {
		p := r.Policy
		if p == nil || (p.Status != "active" && p.Status != "suspended") || p.MaxConcurrentNodes < 1 || p.MaxConcurrentNodes > l.UserConcurrency || p.MaxPendingRuns < 1 || p.MaxPendingRuns > l.MaxUserPending || p.MaxDailySubmissions < 1 || p.MaxDailySubmissions > 10000 {
			return w, errs.WithCode(400, "资格状态或资源限制无效；并发和待执行数不能超过实例上限，每日 1—10000 次")
		}
		storage, err := PODPositiveID(p.MaxStorageBytes)
		if err != nil || storage > 10<<30 {
			return w, errs.WithCode(400, "存储上限须为 1—10737418240 字节")
		}
		w.Policy = model.PODCreatorPolicy{Status: p.Status, MaxConcurrentNodes: p.MaxConcurrentNodes, MaxPendingRuns: p.MaxPendingRuns, MaxDailySubmissions: p.MaxDailySubmissions, MaxStorageBytes: storage}
		if p.ValidUntil != nil {
			until, e := time.ParseInLocation("2006-01-02 15:04:05", *p.ValidUntil, types.Location)
			if e != nil || until.Format("2006-01-02 15:04:05") != *p.ValidUntil || until.Year() < 2000 || until.Year() > 9999 {
				return w, errs.WithCode(400, "有效期请使用上海时间 YYYY-MM-DD HH:mm:ss，永久有效请传 null")
			}
			w.Policy.ValidUntil = &until
		}
	}
	if op == "creator.models" {
		if r.ModelIDs == nil || len(*r.ModelIDs) > 100 {
			return w, errs.WithCode(400, "请明确提交模型范围，最多 100 项；空数组表示无模型权限")
		}
		seen := map[int64]bool{}
		for _, s := range *r.ModelIDs {
			id, e := PODPositiveID(s)
			if e != nil || seen[id] {
				return w, errs.WithCode(400, "模型编号无效或重复")
			}
			seen[id] = true
			w.ModelIDs = append(w.ModelIDs, id)
		}
		sort.Slice(w.ModelIDs, func(i, j int) bool { return w.ModelIDs[i] < w.ModelIDs[j] })
	}
	if op == "points.adjust" {
		w.Amount, err = PODPositiveID(r.Amount)
		if err != nil || w.Amount > 1_000_000_000 || (r.Kind != "grant" && r.Kind != "deduct") {
			return w, errs.WithCode(400, "点数仅允许 grant/deduct 和 1—1000000000 的正整数")
		}
	}
	return w, nil
}
func PODCreatorWrite(ctx context.Context, user *model.LoginUser, op string, id int64, r model.PODCreatorWrite, l model.PODExecutionLimits) (*model.PODCatalogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, CatalogPermission(op)); err != nil {
		return nil, err
	}
	w, err := validateCreatorWrite(op, id, r, l)
	if err != nil {
		if e := repository.PODCatalogRejected(ctx, user.UserID, op, logx.TraceID(ctx)); e != nil {
			return nil, errs.New("请求校验失败且审计不可用")
		}
		return nil, err
	}
	vault := catalogVault.Load()
	hash, err := vault.Fingerprint(op, struct {
		ID   int64
		Body model.PODCreatorWrite
	}{id, r})
	if err != nil {
		return nil, errs.WithCode(503, "独立幂等主密钥未就绪，当前只读")
	}
	if op == "creator.create" {
		w.PasswordHash, err = HashPassword(r.InitialPassword)
		if err != nil {
			return nil, err
		}
	}
	w.ActorID = user.UserID
	w.Hash = hash
	w.Trace = logx.TraceID(ctx)
	w.Request.InitialPassword = "" // Only the HMAC and bcrypt hash cross the persistence boundary.
	return repository.PODCreatorMutate(ctx, w)
}
func PODCreatorList(ctx context.Context, user *model.LoginUser, name, status string, pg page.Query) ([]model.PODCreatorView, int64, error) {
	if err := requirePODAdmin(user, "podAdmin:creator:list"); err != nil {
		return nil, 0, err
	}
	if len([]rune(name)) > 100 || (status != "" && status != "active" && status != "suspended") {
		return nil, 0, errs.WithCode(400, "筛选条件无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODCreators(ctx, name, status, normalizePODPage(pg))
}
func PODCreatorModelOptions(ctx context.Context, user *model.LoginUser, name string, pg page.Query) ([]model.PODCreatorModelOption, int64, error) {
	if err := requirePODAdmin(user, "podAdmin:creator:grant"); err != nil {
		return nil, 0, err
	}
	if len([]rune(name)) > 100 {
		return nil, 0, errs.WithCode(400, "名称过长")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODCreatorModelOptions(ctx, name, normalizePODPage(pg))
}
func PODCreatorGet(ctx context.Context, user *model.LoginUser, id int64) (*model.PODCreatorDetail, error) {
	if err := requirePODAdmin(user, "podAdmin:creator:query"); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODCreator(ctx, id)
}
func PODCreatorPoints(ctx context.Context, user *model.LoginUser, id int64) (*model.PODPointsAccount, error) {
	if err := requirePODAdmin(user, "podAdmin:points:query"); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODPoints(ctx, id)
}
func PODCreatorEvents(ctx context.Context, user *model.LoginUser, id int64, pg page.Query) ([]model.PODPointsEventView, int64, error) {
	if err := requirePODAdmin(user, "podAdmin:points:query"); err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODPointEvents(ctx, id, normalizePODPage(pg))
}
func PODCheckCreator(ctx context.Context, id int64, write bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODCheckCreator(ctx, id, write)
}
func PODMyEligibility(ctx context.Context, id int64) (*model.PODCreatorDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return repository.PODCreator(ctx, id)
}
func PODMyPoints(ctx context.Context, id int64, pg page.Query) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	a, err := repository.PODPoints(ctx, id)
	if err != nil {
		return nil, err
	}
	events, n, err := repository.PODPointEvents(ctx, id, normalizePODPage(pg))
	if err != nil {
		return nil, err
	}
	platform, err := repository.PODAdminState(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": a, "rows": events, "total": n, "billingEnabled": platform.ManagedEnabled, "userId": strconv.FormatInt(id, 10)}, err
}
