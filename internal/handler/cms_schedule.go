package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSSchedules(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.CMSScheduleQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		rows, total, err := service.CMSSchedulePage(c.Request.Context(), kind, q, page.Parse(c, nil))
		if err != nil {
			fail(c, err)
			return
		}
		response.Page(c, rows, total)
	}
}
func CMSScheduleAdd(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in model.CMSScheduleInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		id, err := service.CMSCreateSchedule(c.Request.Context(), kind, in, middleware.CurrentUser(c).UserID)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, map[string]any{"scheduleId": id})
	}
}
func CMSScheduleCancel(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("taskId"))
		if err != nil {
			fail(c, err)
			return
		}
		var in model.CMSVersion
		if err = c.ShouldBindJSON(&in); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		if err = service.CMSCancelSchedule(c.Request.Context(), kind, id, in.Version, middleware.CurrentUser(c).UserID); err != nil {
			fail(c, err)
			return
		}
		response.Ok(c)
	}
}
