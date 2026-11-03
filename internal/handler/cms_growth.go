package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSTags(c *gin.Context) {
	var q model.CMSTagQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "查询条件错误")
		return
	}
	rows, n, e := service.CMSTags(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSSaveTag(c *gin.Context) {
	var in model.CMSTag
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "标签字段错误")
		return
	}
	if c.Request.Method == "POST" && in.TagId != 0 || c.Request.Method == "PUT" && in.TagId < 1 {
		response.Fail(c, "新增或修改标签编号错误")
		return
	}
	if c.Request.Method == "POST" {
		in.TagId = 0
		in.Version = 0
	}
	if e := service.CMSSaveTag(c.Request.Context(), in, currentUsername(c)); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSOps(kind string, save bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		if !save {
			v, e := service.CMSGetOps(c.Request.Context(), kind, id)
			if e != nil {
				fail(c, e)
				return
			}
			response.OkData(c, v)
			return
		}
		var in model.CMSOpsInput
		if e := c.ShouldBindJSON(&in); e != nil {
			response.Fail(c, "运营字段错误")
			return
		}
		if e := service.CMSSetOps(c.Request.Context(), kind, id, in, currentUsername(c), middleware.CurrentUser(c).HasPermission("cms:asset:query")); e != nil {
			fail(c, e)
			return
		}
		response.Ok(c)
	}
}
func CMSTrash(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.CMSContentQuery
		if e := c.ShouldBindQuery(&q); e != nil {
			response.Fail(c, "回收站条件错误")
			return
		}
		q.Deleted = true
		rows, n, e := service.CMSContentPage(c.Request.Context(), kind, q, page.Parse(c, nil))
		if e != nil {
			fail(c, e)
			return
		}
		response.Page(c, rows, n)
	}
}
func CMSRecover(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		var in model.CMSVersion
		if e := c.ShouldBindJSON(&in); e != nil {
			response.Fail(c, "内容版本错误")
			return
		}
		if e := service.CMSRecoverContent(c.Request.Context(), kind, id, in.Version, currentUsername(c)); e != nil {
			fail(c, e)
			return
		}
		response.Ok(c)
	}
}
func CMSVisits(c *gin.Context) {
	var q model.CMSVisitQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "统计条件错误")
		return
	}
	rows, n, e := service.CMSVisits(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Page(c, rows, n)
}

func CMSGetTag(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	row, e := service.CMSGetTag(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
