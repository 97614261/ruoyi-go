package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

var cmsDownloadUploadSlots = make(chan struct{}, 2)

func registerDownloadUpload(r *gin.Engine) {
	r.POST("/cms/download/files/upload", middleware.Auth(), middleware.HasPermission("cms:download:upload"), middleware.HasPermission("cms:download:query"), func(c *gin.Context) {
		select {
		case cmsDownloadUploadSlots <- struct{}{}:
			defer func() { <-cmsDownloadUploadSlots }()
		default:
			response.Fail(c, "资料上传繁忙，请稍后重试")
			c.Abort()
			return
		}
		if u := middleware.CurrentUser(c); u != nil {
			c.Request = c.Request.WithContext(service.CMSActorContext(c.Request.Context(), u.UserID))
		}
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}, middleware.MultipartBodyLimit(22<<20, 1<<20), middleware.OperLogWithoutParameters("资料附件上传", model.BusinessTypeInsert), handler.CMSDownloadUpload)
}

var cmsPublicDownloadSlots = make(chan struct{}, 8)

func publicDownloadLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		select {
		case cmsPublicDownloadSlots <- struct{}{}:
			defer func() { <-cmsPublicDownloadSlots }()
		default:
			c.Status(503)
			c.Abort()
			return
		}
		c.Next()
	}
}
