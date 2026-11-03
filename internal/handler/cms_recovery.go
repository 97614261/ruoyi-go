package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
	"strconv"
	"strings"
)

func CMSRecovery(kind, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Robots-Tag", "noindex, nofollow")
		id, e := strconv.ParseInt(c.Param("id"), 10, 64)
		if e != nil || id < 0 {
			response.Fail(c, "内容编号错误")
			return
		}
		u := middleware.CurrentUser(c)
		prefix := "cms:" + strings.ToLower(kind) + ":"
		permitted := u.HasPermission(prefix + "add")
		if id > 0 {
			permitted = u.HasPermission(prefix+"query") && u.HasPermission(prefix+"edit")
		}
		if !permitted {
			response.FailCode(c, 403, "需要当前内容的新增或查询及编辑权限")
			return
		}
		switch action {
		case "get":
			v, e := service.CMSGetRecovery(c.Request.Context(), kind, id, u.UserID)
			if e != nil {
				fail(c, e)
				return
			}
			response.OkData(c, v)
		case "put":
			var in model.CMSRecoveryInput
			if e = c.ShouldBindJSON(&in); e != nil {
				response.Fail(c, "自动保存字段不正确或超限")
				return
			}
			v, e := service.CMSPutRecovery(c.Request.Context(), kind, id, u.UserID, in, currentUsername(c), u.HasPermission("cms:asset:query"))
			if e != nil {
				fail(c, e)
				return
			}
			response.OkData(c, v)
		case "clear":
			var in model.CMSRecoveryToken
			if e = c.ShouldBindJSON(&in); e != nil {
				response.Fail(c, "副本版本错误")
				return
			}
			v, e := service.CMSClearRecovery(c.Request.Context(), kind, id, u.UserID, in, currentUsername(c))
			if e != nil {
				fail(c, e)
				return
			}
			response.OkData(c, v)
		case "commit":
			var in model.CMSRecoveryCommit
			if e = c.ShouldBindJSON(&in); e != nil {
				response.Fail(c, bindMessage(e))
				return
			}
			v, e := service.CMSCommitRecovery(c.Request.Context(), kind, id, u.UserID, in, currentUsername(c), u.HasPermission("cms:asset:query"))
			if e != nil {
				fail(c, e)
				return
			}
			response.OkData(c, v)
		}
	}
}
