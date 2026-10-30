package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSPublicationCheck(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		row, err := service.CMSCheckPublication(c.Request.Context(), kind, id)
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.OkData(c, row)
	}
}
func CMSBatchChange(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in model.CMSBatchInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		rows, err := service.CMSBatchChange(c.Request.Context(), kind, in, currentUsername(c), middleware.CurrentUser(c).HasPermission("cms:asset:query"))
		if err != nil {
			fail(c, err)
			return
		}
		success := 0
		for _, r := range rows {
			if r.Success {
				success++
			}
		}
		if success < len(rows) {
			middleware.MarkOperLogFailure(c, fmt.Sprintf("批量操作存在失败：成功%d，失败%d", success, len(rows)-success))
		}
		response.OkData(c, map[string]any{"results": rows, "successCount": success, "failureCount": len(rows) - success})
	}
}
func CMSInspectSite(publicOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		pg := page.Parse(c, nil)
		if pg.PageNum > 10000 {
			response.Fail(c, "巡检页码过大")
			return
		}
		if pg.PageSize > 20 {
			pg.PageSize = 20
		}
		out, err := service.CMSInspectSite(c.Request.Context(), pg, publicOrigin)
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.OkData(c, out)
	}

}
