package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
)

// Generated channel/asset CRUD routes are replaced with this explicit permission and private-file boundary.
func registerCMS(r *gin.Engine, cfg *config.Config) {
	g := r.Group("/cms", middleware.Auth(), func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		if u := middleware.CurrentUser(c); u != nil {
			c.Request = c.Request.WithContext(service.CMSActorContext(c.Request.Context(), u.UserID))
		}
		c.Next()
	}, middleware.RequestBodyLimit(cfg.Server.MaxRequestBodyMB<<20), middleware.MultipartBodyLimit(cfg.Upload.MaxRequestSizeMB<<20, cfg.Upload.MaxSizeMB<<20))

	g.GET("/download/categories", middleware.HasPermission("cms:download:list"), handler.CMSDownloadCategories)
	g.POST("/download/categories", middleware.HasPermission("cms:download:category"), middleware.OperLogWithoutParameters("资料分类", model.BusinessTypeUpdate), handler.CMSSaveDownloadCategory)
	g.DELETE("/download/categories/:id", middleware.HasPermission("cms:download:category"), middleware.OperLogWithoutParameters("资料分类删除", model.BusinessTypeDelete), handler.CMSRemoveDownloadCategory)
	g.GET("/download/files", middleware.HasPermission("cms:download:query"), handler.CMSDownloadFiles)
	g.GET("/download/files/:id/file", middleware.HasPermission("cms:download:query"), handler.CMSPrivateDownload)
	g.DELETE("/download/files/:id", middleware.HasPermission("cms:download:fileRemove"), middleware.OperLogWithoutParameters("资料附件删除", model.BusinessTypeDelete), handler.CMSDeleteDownloadFile)
	g.GET("/download/list", middleware.HasPermission("cms:download:list"), handler.CMSDownloadList)
	g.GET("/download/:id", middleware.HasPermission("cms:download:query"), handler.CMSDownloadDetail)
	g.GET("/download/:id/history", middleware.HasPermission("cms:download:query"), handler.CMSDownloadHistory)
	g.POST("/download", middleware.HasPermission("cms:download:add"), middleware.OperLogWithoutParameters("资料草稿", model.BusinessTypeInsert), handler.CMSSaveDownload)
	g.PUT("/download", middleware.HasPermission("cms:download:edit"), middleware.OperLogWithoutParameters("资料草稿", model.BusinessTypeUpdate), handler.CMSSaveDownload)
	g.POST("/download/:id/publish", middleware.HasPermission("cms:download:publish"), middleware.OperLogWithoutParameters("资料发布", model.BusinessTypeUpdate), handler.CMSChangeDownload("publish"))
	g.POST("/download/:id/unpublish", middleware.HasPermission("cms:download:publish"), middleware.OperLogWithoutParameters("资料下架", model.BusinessTypeUpdate), handler.CMSChangeDownload("unpublish"))
	g.DELETE("/download/:id", middleware.HasPermission("cms:download:remove"), middleware.OperLogWithoutParameters("资料删除", model.BusinessTypeDelete), handler.CMSChangeDownload("remove"))
	g.GET("/presets/list", middleware.HasPermission("cms:preset:list"), handler.CMSPresetDefinitions)
	g.POST("/presets/plan", middleware.HasPermission("cms:preset:list"), handler.CMSPreviewPreset)
	g.POST("/presets/apply", middleware.HasPermission("cms:preset:apply"), middleware.OperLogWithoutParameters("行业预设应用草稿", model.BusinessTypeUpdate), handler.CMSApplyPreset)
	g.GET("/presets/history", middleware.HasPermission("cms:preset:list"), handler.CMSPresetHistory)

	g.POST("/compare", middleware.RequestBodyLimit(16<<10), handler.CMSCompare)
	g.GET("/data/policy", middleware.HasPermission("cms:data:list"), handler.CMSDataPolicyGet)
	g.PUT("/data/policy", middleware.HasPermission("cms:data:edit"), middleware.OperLogWithoutParameters("保留策略", model.BusinessTypeUpdate), handler.CMSDataPolicySet)
	g.POST("/data/cleanup/preview", middleware.HasPermission("cms:data:cleanup"), middleware.OperLogWithoutParameters("清理预览", model.BusinessTypeOther), handler.CMSCleanupPreview)
	g.POST("/data/cleanup/:id/execute", middleware.HasPermission("cms:data:cleanup"), middleware.OperLogWithoutParameters("确认清理", model.BusinessTypeDelete), handler.CMSCleanupExecute)
	g.GET("/data/cleanup/list", middleware.HasPermission("cms:data:cleanup"), handler.CMSCleanupBatches)
	g.GET("/data/cleanup/:id/items", middleware.HasPermission("cms:data:cleanup"), handler.CMSCleanupItems)
	g.GET("/data/events", middleware.HasPermission("cms:data:audit"), handler.CMSDataEvents(false))
	g.POST("/inquiry/export", middleware.HasPermission("cms:inquiry:export"), middleware.ExportLimit(), middleware.OperLogWithoutParameters("询盘导出", model.BusinessTypeExport), handler.CMSInquiryExport)
	g.GET("/inquiry/export/history", middleware.HasPermission("cms:inquiry:export"), handler.CMSDataEvents(true))
	g.GET("/search-index/status", middleware.HasPermission("cms:search:list"), handler.CMSSearchStatus)
	g.GET("/search-index/list", middleware.HasPermission("cms:search:list"), handler.CMSSearchTasks)
	g.POST("/search-index/rebuild", middleware.HasPermission("cms:search:edit"), middleware.OperLogWithoutParameters("公开索引重建", model.BusinessTypeUpdate), handler.CMSSearchRebuild)
	g.GET("/workflow/options", handler.CMSWorkflowOptions)
	g.GET("/workflow/settings", middleware.HasPermission("cms:workflow:list"), handler.CMSWorkflowSettings)
	g.PUT("/workflow/settings", middleware.HasPermission("cms:workflow:edit"), middleware.OperLogWithoutParameters("协作策略", model.BusinessTypeUpdate), handler.CMSSetWorkflow)
	g.GET("/workflow/impact", middleware.HasPermission("cms:workflow:list"), handler.CMSWorkflowImpact)
	g.GET("/workflow/history", middleware.HasPermission("cms:workflow:list"), handler.CMSWorkflowHistory)
	g.GET("/workflow/people", handler.CMSWorkflowPeople)
	g.GET("/review/list", middleware.HasPermission("cms:review:list"), handler.CMSReviewList)
	g.GET("/review/object", handler.CMSReviewList)
	g.POST("/review", middleware.OperLogWithoutParameters("提交审核", model.BusinessTypeInsert), handler.CMSSubmitReview)
	g.GET("/review/:id", handler.CMSReviewGet)
	g.GET("/review/:id/events", handler.CMSReviewEvents)
	g.POST("/review/:id/decision", middleware.OperLogWithoutParameters("审核处理", model.BusinessTypeUpdate), handler.CMSReviewDecide)
	g.POST("/site/restore-draft", middleware.HasPermission("cms:site:query"), middleware.HasPermission("cms:site:edit"), middleware.OperLogWithoutParameters("历史外观恢复草稿", model.BusinessTypeUpdate), handler.CMSRestoreSiteDraft)
	g.GET("/inquiry/tasks", middleware.HasPermission("cms:inquiry:list"), handler.CMSInquiryFilteredTasks)
	g.GET("/inquiry/:id/team", middleware.HasPermission("cms:inquiry:query"), handler.CMSInquiryTeamGet)
	g.POST("/inquiry/:id/assign", middleware.HasPermission("cms:inquiry:query"), middleware.HasPermission("cms:inquiry:assign"), middleware.OperLogWithoutParameters("询盘指派", model.BusinessTypeUpdate), handler.CMSInquiryAssign)
	g.GET("/inquiry/:id/followups", middleware.HasPermission("cms:inquiry:query"), handler.CMSInquiryFollowups)
	g.POST("/inquiry/:id/followups", middleware.HasPermission("cms:inquiry:query"), middleware.HasPermission("cms:inquiry:edit"), middleware.OperLogWithoutParameters("询盘跟进", model.BusinessTypeUpdate), handler.CMSInquiryFollow)
	g.GET("/workbench", handler.CMSWorkbench)
	g.GET("/workbench/drafts", handler.CMSOwnDrafts)
	g.GET("/workbench/schedules", handler.CMSOwnFailedSchedules)
	g.GET("/notifications/list", middleware.HasPermission("cms:notification:list"), handler.CMSNotifications)
	g.GET("/notifications/unread", middleware.HasPermission("cms:notification:list"), handler.CMSNotificationUnread)
	g.POST("/notifications/:id/read", middleware.HasPermission("cms:notification:list"), middleware.OperLogWithoutParameters("消息已读", model.BusinessTypeUpdate), handler.CMSReadNotification)
	g.GET("/tag/:id", middleware.HasPermission("cms:tag:query"), handler.CMSGetTag)
	g.GET("/tag/list", middleware.HasPermission("cms:tag:list"), handler.CMSTags)
	g.POST("/tag", middleware.HasPermission("cms:tag:add"), middleware.OperLogWithoutParameters("标签新增", model.BusinessTypeInsert), handler.CMSSaveTag)
	g.PUT("/tag", middleware.HasPermission("cms:tag:edit"), middleware.OperLogWithoutParameters("标签修改", model.BusinessTypeUpdate), handler.CMSSaveTag)
	g.GET("/statistics/list", middleware.HasPermission("cms:statistics:list"), handler.CMSVisits)
	g.GET("/redirect/list", middleware.HasPermission("cms:redirect:list"), handler.CMSRedirectList)
	g.GET("/redirect/:id", middleware.HasPermission("cms:redirect:query"), handler.CMSRedirectGet)
	g.POST("/redirect", middleware.HasPermission("cms:redirect:add"), middleware.OperLogWithoutParameters("旧路径草稿", model.BusinessTypeInsert), handler.CMSRedirectSave)
	g.PUT("/redirect", middleware.HasPermission("cms:redirect:edit"), middleware.OperLogWithoutParameters("旧路径草稿", model.BusinessTypeUpdate), handler.CMSRedirectSave)
	g.POST("/redirect/:id/status", middleware.HasPermission("cms:redirect:publish"), middleware.OperLogWithoutParameters("旧路径启停", model.BusinessTypeUpdate), handler.CMSRedirectStatus)
	g.GET("/inquiry/list", middleware.HasPermission("cms:inquiry:list"), handler.CMSInquiryList)
	g.GET("/inquiry/:id", middleware.HasPermission("cms:inquiry:query"), handler.CMSInquiryGet)
	g.PUT("/inquiry/:id", middleware.HasPermission("cms:inquiry:edit"), middleware.OperLogWithoutParameters("处理询盘", model.BusinessTypeUpdate), handler.CMSInquiryChange(false))
	g.DELETE("/inquiry/:id", middleware.HasPermission("cms:inquiry:remove"), middleware.OperLogWithoutParameters("清除询盘", model.BusinessTypeDelete), handler.CMSInquiryChange(true))
	g.GET("/site/themes", middleware.HasPermission("cms:site:query"), handler.CMSThemes)
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
	g.GET("/asset/statistics", middleware.HasPermission("cms:asset:list"), handler.CMSAssetStatistics)
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
		recovery := g.Group("/"+item.path+"/:id/autosave", middleware.RequestBodyLimit(service.CMSRecoveryLimit))
		recovery.GET("", handler.CMSRecovery(item.kind, "get"))
		recovery.PUT("", handler.CMSRecovery(item.kind, "put"))
		recovery.DELETE("", middleware.OperLogWithoutParameters("清除个人恢复副本", model.BusinessTypeDelete), handler.CMSRecovery(item.kind, "clear"))
		recovery.POST("/commit", middleware.OperLogWithoutParameters("提交个人恢复草稿", model.BusinessTypeUpdate), handler.CMSRecovery(item.kind, "commit"))
		base := "/" + item.path
		g.GET(base+"/transfer/template", middleware.HasPermission(prefix+"import"), handler.CMSTransferTemplate(item.kind))
		g.POST(base+"/transfer/check", middleware.HasPermission(prefix+"import"), middleware.HasPermission(prefix+"add"), middleware.RequestBodyLimit(service.CMSTransferLimit), handler.CMSImportDrafts(item.kind, false))
		g.POST(base+"/transfer/import", middleware.HasPermission(prefix+"import"), middleware.HasPermission(prefix+"add"), middleware.RequestBodyLimit(service.CMSTransferLimit), middleware.OperLogWithoutParameters("导入门户草稿", model.BusinessTypeInsert), handler.CMSImportDrafts(item.kind, true))
		g.POST(base+"/transfer/export", middleware.HasPermission(prefix+"export"), middleware.HasPermission(prefix+"query"), middleware.ExportLimit(), middleware.OperLogWithoutParameters("导出门户草稿", model.BusinessTypeExport), handler.CMSExportDrafts(item.kind))
		g.GET(base+"/schedules", middleware.HasPermission(prefix+"query"), handler.CMSSchedules(item.kind))
		g.POST(base+"/schedules", middleware.HasPermission(prefix+"publish"), middleware.OperLogWithoutParameters("门户定时安排", model.BusinessTypeInsert), handler.CMSScheduleAdd(item.kind))
		g.POST(base+"/schedules/:taskId/cancel", middleware.HasPermission(prefix+"publish"), middleware.OperLogWithoutParameters("取消门户定时任务", model.BusinessTypeUpdate), handler.CMSScheduleCancel(item.kind))
		g.GET(base+"/trash", middleware.HasPermission(prefix+"list"), middleware.HasPermission(prefix+"remove"), handler.CMSTrash(item.kind))
		g.POST(base+"/:id/recover", middleware.HasPermission(prefix+"query"), middleware.HasPermission(prefix+"edit"), middleware.HasPermission(prefix+"remove"), middleware.OperLogWithoutParameters("回收站恢复草稿", model.BusinessTypeUpdate), handler.CMSRecover(item.kind))
		g.GET(base+"/:id/operations", middleware.HasPermission(prefix+"query"), handler.CMSOps(item.kind, false))
		g.PUT(base+"/:id/operations", middleware.HasPermission(prefix+"query"), middleware.HasPermission(prefix+"edit"), middleware.OperLogWithoutParameters("运营分享草稿", model.BusinessTypeUpdate), handler.CMSOps(item.kind, true))
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
