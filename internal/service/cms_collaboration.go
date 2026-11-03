package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
	"time"
	"unicode"
)

func CMSActorContext(ctx context.Context, id int64) context.Context {
	return repository.WithCMSActor(ctx, id)
}
func cmsWorkflowText(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' })
}
func cmsValidateWorkflow(in model.CMSWorkflowInput) error {
	if len(in.Required) != 4 {
		return errs.New("请明确提交文章、产品、单页和外观四项策略")
	}
	for k := range in.Required {
		if k != "ARTICLE" && k != "PRODUCT" && k != "PAGE" && k != "SITE" {
			return errs.New("审核模块不受支持")
		}
	}
	for _, k := range []string{"ARTICLE", "PRODUCT", "PAGE", "SITE"} {
		if _, ok := in.Required[k]; !ok {
			return errs.New("审核策略缺失")
		}
	}
	ids := map[int64]bool{}
	for _, id := range in.ManagerIds {
		if id <= 0 || ids[id] {
			return errs.New("询盘接收人重复或无效")
		}
		ids[id] = true
	}
	if !cmsWorkflowText(in.Reason) {
		return errs.New("原因含无效控制字符")
	}
	return nil
}
func CMSWorkflowSettings(ctx context.Context) (model.CMSWorkflowSettings, error) {
	return repository.CMSWorkflowSettings(ctx)
}
func CMSSetWorkflow(ctx context.Context, in model.CMSWorkflowInput) error {
	if e := cmsValidateWorkflow(in); e != nil {
		return e
	}
	return repository.CMSSetWorkflow(ctx, in)
}
func CMSWorkflowPeople(ctx context.Context, q model.CMSPeopleQuery, pg page.Query) ([]model.CMSWorkflowPerson, int64, error) {
	if q.Purpose == "review" && q.Kind == "" {
		return nil, 0, errs.New("请选择审核模块")
	}
	return repository.CMSWorkflowPeople(ctx, q, pg)
}
func CMSWorkflowHistory(ctx context.Context, pg page.Query) ([]model.CMSReviewEvent, int64, error) {
	return repository.CMSWorkflowHistory(ctx, pg)
}
func CMSSubmitReview(ctx context.Context, in model.CMSReviewInput) (int64, error) {
	return repository.CMSSubmitReview(ctx, in)
}
func CMSReviewPage(ctx context.Context, q model.CMSReviewQuery, pg page.Query) ([]model.CMSReviewRequest, int64, error) {
	return repository.CMSReviewPage(ctx, q, pg)
}
func CMSReviewDetail(ctx context.Context, id int64) (model.CMSReviewDetail, error) {
	return repository.CMSReviewDetail(ctx, id)
}
func CMSReviewEvents(ctx context.Context, id int64, pg page.Query) ([]model.CMSReviewEvent, int64, error) {
	return repository.CMSReviewEvents(ctx, id, pg)
}
func CMSDecideReview(ctx context.Context, id int64, in model.CMSReviewDecision) error {
	in.Opinion = strings.TrimSpace(in.Opinion)
	if (in.Action == "reject" && in.Opinion == "") || !cmsWorkflowText(in.Opinion) {
		return errs.New("退回须填写纯文本意见，不能含控制字符")
	}
	// Same lock as role/user/menu permission writes; capture a committed permission generation.
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	return repository.CMSDecideReview(ctx, id, in)
}
func CMSRestoreSiteDraft(ctx context.Context, in model.SiteAction) error {
	return repository.CMSRestoreSiteDraft(ctx, in)
}
func CMSInquiryTeamDetail(ctx context.Context, id int64) (model.CMSInquiryTeam, error) {
	return repository.CMSInquiryTeamDetail(ctx, id)
}
func CMSInquiryTasks(ctx context.Context, q model.CMSInquiryTaskQuery, pg page.Query) ([]model.CMSInquiryTask, int64, error) {
	return repository.CMSInquiryTasks(ctx, q, pg)
}
func CMSAssignInquiry(ctx context.Context, id int64, in model.CMSInquiryAssign) error {
	return repository.CMSAssignInquiry(ctx, id, in)
}
func CMSFollowInquiry(ctx context.Context, id int64, in model.CMSInquiryFollowInput) error {
	in.Text = strings.TrimSpace(in.Text)
	in.Result = strings.TrimSpace(in.Result)
	if !cmsWorkflowText(in.Text + in.Result) {
		return errs.New("跟进记录含无效控制字符")
	}
	if in.NextFollowAt != "" {
		due, e := cmsScheduleTime(in.NextFollowAt, time.Now())
		if e != nil {
			return e
		}
		in.Due = types.Time(due)
	}
	return repository.CMSFollowInquiry(ctx, id, in)
}
func CMSInquiryFollowups(ctx context.Context, id int64, pg page.Query) ([]model.CMSInquiryFollowup, int64, error) {
	return repository.CMSInquiryFollowups(ctx, id, pg)
}
func CMSWorkbench(ctx context.Context) (model.CMSWorkbench, error) {
	return repository.CMSWorkbench(ctx)
}
func CMSOwnDrafts(ctx context.Context, pg page.Query) ([]model.CMSContentView, int64, error) {
	return repository.CMSOwnDrafts(ctx, pg)
}
func CMSOwnFailedSchedules(ctx context.Context, pg page.Query) ([]model.CMSSchedule, int64, error) {
	return repository.CMSOwnFailedSchedules(ctx, pg)
}
func CMSNotifications(ctx context.Context, unread bool, pg page.Query) ([]model.CMSNotification, int64, error) {
	return repository.CMSNotifications(ctx, unread, pg)
}
func CMSNotificationUnread(ctx context.Context) (int64, error) {
	return repository.CMSNotificationUnread(ctx)
}
func CMSReadNotification(ctx context.Context, id, version int64) (model.CMSNotification, error) {
	return repository.CMSReadNotification(ctx, id, version)
}

func CMSWorkflowImpact(ctx context.Context) ([]model.CMSWorkflowImpact, error) {
	return repository.CMSWorkflowImpact(ctx)
}
