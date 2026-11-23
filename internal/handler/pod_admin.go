package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"time"
)

func PODAdminBootstrap(c *gin.Context) {
	view, err := service.PODAdminBootstrap(c.Request.Context(), middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
func PODAdminOverview(c *gin.Context) {
	view, err := service.PODAdminOverview(c.Request.Context(), middleware.CurrentUser(c), c.Query("beginDate"), c.Query("endDate"))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
func PODAdminAudits(c *gin.Context) {
	begin, end, err := service.PODAdminDateRange(c.Query("beginDate"), c.Query("endDate"), time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	f := model.PODAdminAuditFilter{Begin: begin, End: end, ActionType: c.Query("actionType"), Outcome: c.Query("outcome")}
	for key, target := range map[string]*int64{"actorUserId": &f.ActorUserID, "ownerUserId": &f.OwnerUserID} {
		if raw := c.Query(key); raw != "" {
			id, err := service.PODPositiveID(raw)
			if err != nil {
				fail(c, err)
				return
			}
			*target = id
		}
	}
	rows, total, err := service.PODAdminAuditList(c.Request.Context(), middleware.CurrentUser(c), f, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func PODAdminAudit(c *gin.Context) {
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	view, err := service.PODAdminAuditGet(c.Request.Context(), middleware.CurrentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
