// Generated handlers specialized for privacy: operator audit excludes request/response bodies.
package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/portalview"
	"ruoyi-go/pkg/response"
	"strings"
)

func (p *PortalUI) Inquiry(c *gin.Context) {
	token, e := service.CMSInquiryProof(c.Request.Context(), c.ClientIP())
	if e != nil {
		p.render(c, 503, "page", portalview.PageData{Title: "留言暂不可用", Body: "请稍后再试", NoIndex: true})
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "portal_inquiry", Value: token, Path: "/inquiry", MaxAge: 600, HttpOnly: true, Secure: c.Request.TLS != nil || strings.HasPrefix(p.origin, "https://"), SameSite: http.SameSiteStrictMode})
	p.render(c, 200, "inquiry", portalview.PageData{Title: "联系与留言", Query: token, NoIndex: true})
}
func (p *PortalUI) SubmitInquiry(c *gin.Context) {
	if o := c.GetHeader("Origin"); o != "" {
		u, e := url.Parse(p.origin)
		if e != nil || strings.TrimRight(o, "/") != u.Scheme+"://"+u.Host {
			p.render(c, 403, "page", portalview.PageData{Title: "提交来源无效", NoIndex: true})
			return
		}
	}
	if !strings.HasPrefix(c.ContentType(), "application/x-www-form-urlencoded") {
		p.render(c, 400, "page", portalview.PageData{Title: "请从留言表单提交", NoIndex: true})
		return
	}
	var in model.CMSInquiryInput
	if e := c.ShouldBind(&in); e != nil {
		p.render(c, 400, "page", portalview.PageData{Title: "填写有误", Body: "姓名60字、联系方式120字、留言2000字以内；请勾选同意联系后重新打开留言页。", NoIndex: true})
		return
	}
	cookie, _ := c.Cookie("portal_inquiry")
	if e := service.CMSSubmitInquiry(c.Request.Context(), in, c.ClientIP(), cookie); e != nil {
		p.render(c, 400, "page", portalview.PageData{Title: "提交未完成", Body: "请重新打开留言页，填写后稍候提交；多次提交请一小时后再试。", NoIndex: true})
		return
	}
	c.Redirect(http.StatusSeeOther, "/inquiry/thanks")
}
func (p *PortalUI) InquiryThanks(c *gin.Context) {
	p.render(c, 200, "page", portalview.PageData{Title: "留言已提交", Body: "我们会按您留下的联系方式回复，请勿重复提交。", NoIndex: true})
}
func CMSInquiryList(c *gin.Context) {
	var q model.CMSInquiryQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	rows, n, e := service.CMSInquiryPage(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSInquiryGet(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	row, e := service.CMSInquiryDetail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSInquiryChange(erase bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		var in model.CMSInquiryUpdate
		if erase {
			var v model.CMSVersion
			if e = c.ShouldBindJSON(&v); e != nil {
				response.Fail(c, "版本错误")
				return
			}
			in.Version = v.Version
			in.Status = "SPAM"
		} else if e = c.ShouldBindJSON(&in); e != nil {
			response.Fail(c, "参数错误")
			return
		}
		if e = service.CMSUpdateInquiry(c.Request.Context(), id, in, currentUsername(c), erase); e != nil {
			fail(c, e)
			return
		}
		response.Ok(c)
	}
}
