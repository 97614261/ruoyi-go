package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSRedirectList(c *gin.Context) {
	var q model.CMSRedirectQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	rows, n, e := service.CMSRedirectPage(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Page(c, rows, n)
}
func CMSRedirectGet(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	row, e := service.CMSRedirectDetail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.OkData(c, row)
}
func CMSRedirectSave(c *gin.Context) {
	var in model.CMSRedirectInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "参数错误")
		return
	}
	if c.Request.Method == http.MethodPost && in.RedirectId != 0 || c.Request.Method == http.MethodPut && in.RedirectId <= 0 {
		response.Fail(c, "新增或修改编号不正确")
		return
	}
	id, e := service.CMSSaveRedirect(c.Request.Context(), in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, map[string]any{"redirectId": id})
}
func CMSRedirectStatus(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSRedirectStatus
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "状态或版本错误")
		return
	}
	if e = service.CMSSetRedirectStatus(c.Request.Context(), id, in, currentUsername(c)); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}

// Only missing content routes and unknown public GET/HEAD paths use this handler.
func (p *PortalUI) tryRedirect(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}
	target, e := service.CMSResolveRedirect(c.Request.Context(), c.Request.URL.EscapedPath())
	c.Header("Cache-Control", "no-store")
	if e != nil {
		if service.IsCMSNotFound(e) {
			return false
		}
		c.Status(503)
		return true
	}
	c.Redirect(http.StatusMovedPermanently, target)
	return true
}
func (p *PortalUI) RedirectOr404(c *gin.Context) {
	if p.tryRedirect(c) {
		return
	}
	c.String(404, "404 page not found")
}
