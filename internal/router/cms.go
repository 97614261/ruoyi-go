package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
)

// Generated channel/asset CRUD routes are replaced with this explicit permission and private-file boundary.
func registerCMS(r *gin.Engine, cfg *config.Config) {
	g := r.Group("/cms", middleware.Auth(), middleware.RequestBodyLimit(cfg.Server.MaxRequestBodyMB<<20), middleware.MultipartBodyLimit(cfg.Upload.MaxRequestSizeMB<<20, cfg.Upload.MaxSizeMB<<20))
	g.GET("/site", middleware.HasPermission("cms:site:query"), handler.CMSGetSite)
	g.PUT("/site", middleware.HasPermission("cms:site:edit"), middleware.OperLogWithoutParameters("站点草稿", model.BusinessTypeUpdate), handler.CMSSaveSite)
	g.POST("/site/publish", middleware.HasPermission("cms:site:publish"), middleware.OperLogWithoutParameters("外观发布或恢复", model.BusinessTypeUpdate), handler.CMSPublishSite)
	g.GET("/site/inspect", middleware.HasPermission("cms:site:query"), middleware.HasPermission("cms:asset:query"), middleware.HasPermission("cms:channel:query"), middleware.HasPermission("cms:article:query"), middleware.HasPermission("cms:product:query"), middleware.HasPermission("cms:page:query"), handler.CMSInspectSite(cfg.Portal.PublicURL))
	g.GET("/site/history", middleware.HasPermission("cms:site:query"), handler.CMSSiteHistory)
	g.GET("/channel/list", middleware.HasPermission("cms:channel:list"), handler.CMSChannelList)
	g.GET("/channel/:id", middleware.HasPermission("cms:channel:query"), handler.CMSChannelGet)
	g.POST("/channel", middleware.HasPermission("cms:channel:add"), middleware.OperLog("门户栏目", model.BusinessTypeInsert), handler.CMSChannelSave)
	g.PUT("/channel", middleware.HasPermission("cms:channel:edit"), middleware.OperLog("门户栏目", model.BusinessTypeUpdate), handler.CMSChannelSave)
	g.DELETE("/channel/:id", middleware.HasPermission("cms:channel:remove"), middleware.OperLog("门户栏目", model.BusinessTypeDelete), handler.CMSChannelRemove)
	g.GET("/asset/list", middleware.HasPermission("cms:asset:list"), handler.CMSAssetList)
	g.POST("/asset/upload", middleware.HasPermission("cms:asset:upload"), middleware.OperLog("门户素材", model.BusinessTypeInsert), handler.CMSAssetUpload)
	g.GET("/asset/:id", middleware.HasPermission("cms:asset:query"), handler.CMSAssetGet)
	g.PUT("/asset/:id", middleware.HasPermission("cms:asset:edit"), middleware.OperLog("门户素材", model.BusinessTypeUpdate), handler.CMSAssetEdit)
	g.DELETE("/asset/:id", middleware.HasPermission("cms:asset:remove"), middleware.OperLog("门户素材", model.BusinessTypeDelete), handler.CMSAssetRemove)
	g.GET("/asset/:id/references", middleware.HasPermission("cms:asset:query"), middleware.HasPermission("cms:channel:query"), handler.CMSAssetReferences)
	g.GET("/asset/:id/file", middleware.HasPermission("cms:asset:query"), handler.CMSAssetFile)
	g.HEAD("/asset/:id/file", middleware.HasPermission("cms:asset:query"), handler.CMSAssetFile)

	for _, item := range []struct{ path, kind string }{{"article", "ARTICLE"}, {"page", "PAGE"}, {"product", "PRODUCT"}} {
		prefix := "cms:" + item.path + ":"
		base := "/" + item.path
		g.GET(base+"/schedules", middleware.HasPermission(prefix+"query"), handler.CMSSchedules(item.kind))
		g.POST(base+"/schedules", middleware.HasPermission(prefix+"publish"), middleware.OperLogWithoutParameters("门户定时安排", model.BusinessTypeInsert), handler.CMSScheduleAdd(item.kind))
		g.POST(base+"/schedules/:taskId/cancel", middleware.HasPermission(prefix+"publish"), middleware.OperLogWithoutParameters("取消门户定时任务", model.BusinessTypeUpdate), handler.CMSScheduleCancel(item.kind))
		g.GET(base+"/list", middleware.HasPermission(prefix+"list"), handler.CMSContentList(item.kind))
		g.GET(base+"/:id", middleware.HasPermission(prefix+"query"), handler.CMSContentGet(item.kind))
		g.GET(base+"/:id/check", middleware.HasPermission(prefix+"query"), handler.CMSPublicationCheck(item.kind))
		g.POST(base+"/batch", middleware.HasPermission(prefix+"publish"), middleware.OperLogWithoutParameters("门户批量发布下架", model.BusinessTypeUpdate), handler.CMSBatchChange(item.kind))
		g.GET(base+"/:id/history", middleware.HasPermission(prefix+"query"), handler.CMSContentHistory(item.kind))
		g.POST(base, middleware.HasPermission(prefix+"add"), middleware.OperLogWithoutParameters("门户内容草稿", model.BusinessTypeInsert), handler.CMSContentSave(item.kind))
		g.PUT(base, middleware.HasPermission(prefix+"edit"), middleware.OperLogWithoutParameters("门户内容草稿", model.BusinessTypeUpdate), handler.CMSContentSave(item.kind))
		for _, action := range []string{"publish", "unpublish", "restore"} {
			perm := "publish"
			if action == "restore" {
				perm = "edit"
			}
			g.POST(base+"/:id/"+action, middleware.HasPermission(prefix+perm), middleware.OperLogWithoutParameters("门户内容"+action, model.BusinessTypeUpdate), handler.CMSContentChange(item.kind, action))
		}
		g.DELETE(base+"/:id", middleware.HasPermission(prefix+"remove"), middleware.OperLogWithoutParameters("门户内容删除", model.BusinessTypeDelete), handler.CMSContentChange(item.kind, "remove"))
	}
}
