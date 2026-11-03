package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSWorkflowOptions(c *gin.Context) {
	s, e := service.CMSWorkflowSettings(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	type policy struct {
		Kind     string `json:"kind"`
		Required bool   `json:"required"`
		Version  int64  `json:"version"`
	}
	rows := []policy{}
	for _, p := range s.Policies {
		rows = append(rows, policy{p.Kind, p.Required, p.Version})
	}
	response.OkData(c, struct {
		Policies         []policy `json:"policies"`
		InquiryIsolation bool     `json:"inquiryIsolation"`
	}{rows, s.InquiryIsolation})
}
func CMSWorkflowSettings(c *gin.Context) {
	s, e := service.CMSWorkflowSettings(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, s)
}
func CMSSetWorkflow(c *gin.Context) {
	var in model.CMSWorkflowInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "协作设置参数错误")
		return
	}
	if e := service.CMSSetWorkflow(c.Request.Context(), in); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSWorkflowHistory(c *gin.Context) {
	rows, n, e := service.CMSWorkflowHistory(c.Request.Context(), page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSWorkflowPeople(c *gin.Context) {
	var q model.CMSPeopleQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "候选人条件错误")
		return
	}
	rows, n, e := service.CMSWorkflowPeople(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSSubmitReview(c *gin.Context) {
	var in model.CMSReviewInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "送审参数错误")
		return
	}
	id, e := service.CMSSubmitReview(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, id)
}
func CMSReviewList(c *gin.Context) {
	var q model.CMSReviewQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "审核查询参数错误")
		return
	}
	rows, n, e := service.CMSReviewPage(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSReviewGet(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	row, e := service.CMSReviewDetail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSReviewEvents(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	rows, n, e := service.CMSReviewEvents(c.Request.Context(), id, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSReviewDecide(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSReviewDecision
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "审核参数错误")
		return
	}
	if e = service.CMSDecideReview(c.Request.Context(), id, in); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSRestoreSiteDraft(c *gin.Context) {
	var in model.SiteAction
	if e := c.ShouldBindJSON(&in); e != nil || in.ReleaseId <= 0 {
		response.Fail(c, "历史版本参数错误")
		return
	}
	if e := service.CMSRestoreSiteDraft(c.Request.Context(), in); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSInquiryTasks(c *gin.Context) {
	var q model.CMSInquiryTaskQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "待办条件错误")
		return
	}
	rows, n, e := service.CMSInquiryTasks(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSInquiryTeamGet(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	row, e := service.CMSInquiryTeamDetail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSInquiryAssign(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSInquiryAssign
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "指派参数错误")
		return
	}
	if e = service.CMSAssignInquiry(c.Request.Context(), id, in); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSInquiryFollow(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSInquiryFollowInput
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "跟进参数错误")
		return
	}
	if e = service.CMSFollowInquiry(c.Request.Context(), id, in); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSInquiryFollowups(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	rows, n, e := service.CMSInquiryFollowups(c.Request.Context(), id, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSWorkbench(c *gin.Context) {
	s, e := service.CMSWorkbench(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, s)
}
func CMSOwnDrafts(c *gin.Context) {
	rows, n, e := service.CMSOwnDrafts(c.Request.Context(), page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSOwnFailedSchedules(c *gin.Context) {
	rows, n, e := service.CMSOwnFailedSchedules(c.Request.Context(), page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSNotifications(c *gin.Context) {
	rows, n, e := service.CMSNotifications(c.Request.Context(), c.Query("unread") == "true", page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSNotificationUnread(c *gin.Context) {
	n, e := service.CMSNotificationUnread(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, n)
}
func CMSReadNotification(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSVersion
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "消息版本错误")
		return
	}
	row, e := service.CMSReadNotification(c.Request.Context(), id, in.Version)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}

func CMSWorkflowImpact(c *gin.Context) {
	rows, e := service.CMSWorkflowImpact(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, rows)
}
