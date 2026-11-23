package handler

import (
	"context"
	"mime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/storage"
)

func PODAdminTasks(batch bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		f, err := service.PODTaskFilter(c.Query("beginDate"), c.Query("endDate"), c.Query("userId"), c.Query("modelId"), c.Query("state"))
		if err != nil {
			fail(c, err)
			return
		}
		rows, total, err := service.PODTaskList(c.Request.Context(), middleware.CurrentUser(c), f, page.Parse(c, nil), batch)
		if err != nil {
			fail(c, err)
			return
		}
		response.Page(c, rows, total)
	}
}
func PODAdminTask(batch bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := podAssetID(c)
		if !ok {
			return
		}
		row, err := service.PODTaskGet(c.Request.Context(), middleware.CurrentUser(c), id, batch)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, row)
	}
}
func PODAdminTaskNodes(c *gin.Context) {
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	rows, total, err := service.PODTaskNodes(c.Request.Context(), middleware.CurrentUser(c), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func PODAdminUsage(c *gin.Context) {
	f, err := service.PODTaskFilter(c.Query("beginDate"), c.Query("endDate"), c.Query("userId"), c.Query("modelId"), "")
	if err != nil {
		fail(c, err)
		return
	}
	rows, total, err := service.PODAdminUsage(c.Request.Context(), middleware.CurrentUser(c), f, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func PODAdminGrants(c *gin.Context) {
	rows, total, err := service.PODAdminGrants(c.Request.Context(), middleware.CurrentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func PODOperation(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		if op != "private.grant" {
			var ok bool
			id, ok = podAssetID(c)
			if !ok {
				return
			}
		}
		var body model.PODOperationWrite
		if !podBind(c, &body) {
			return
		}
		result, err := service.PODOperation(c.Request.Context(), middleware.CurrentUser(c), op, id, body)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, result)
	}
}
func PODAdminPrivateRun(c *gin.Context) {
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	row, err := service.PODPrivateRun(c.Request.Context(), middleware.CurrentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func PODAdminPrivateAsset(s *service.PODAssetService, download bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		id, ok := podAssetID(c)
		if !ok {
			return
		}
		variant := c.DefaultQuery("variant", "thumbnail")
		if download {
			variant = "original"
		}
		file, meta, name, err := s.AdminOpen(ctx, middleware.CurrentUser(c), id, variant)
		if err != nil {
			fail(c, err)
			return
		}
		defer file.Close()
		disposition := "inline"
		if download {
			disposition = "attachment"
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Type", meta.MediaType)
		c.Header("Content-Length", strconv.FormatInt(meta.ByteSize, 10))
		c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
		_, _ = storage.Copy(ctx, c.Writer, file)
	}
}
