package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"strings"
)

func CMSSiteReferences(c *gin.Context) {
	var q model.CMSSiteReferenceQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	u := middleware.CurrentUser(c)
	allowed := []string{}
	for _, kind := range []string{"ARTICLE", "PRODUCT", "PAGE"} {
		perm := "cms:" + strings.ToLower(kind) + ":list"
		if q.Type == "channel" {
			perm = "cms:channel:list"
		}
		if u.HasPermission(perm) {
			allowed = append(allowed, kind)
		}
	}
	rows, total, e := service.CMSSiteReferences(c.Request.Context(), q, allowed, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, total)
}
