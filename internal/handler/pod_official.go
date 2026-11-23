package handler

import (
	"context"
	"mime"
	"net/http"
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

type PODOfficialHandler struct{ Service *service.PODOfficialService }

func (h PODOfficialHandler) List(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, total, err := h.Service.List(c.Request.Context(), middleware.CurrentUser(c), admin, c.Query("name"), page.Parse(c, nil))
		if err != nil {
			fail(c, err)
			return
		}
		response.Page(c, rows, total)
	}
}
func (h PODOfficialHandler) Get(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := podAssetID(c)
		if !ok {
			return
		}
		var version int64
		if raw := c.Query("versionId"); admin && raw != "" {
			var err error
			version, err = service.PODPositiveID(raw)
			if err != nil {
				fail(c, err)
				return
			}
		}
		out, err := h.Service.Get(c.Request.Context(), middleware.CurrentUser(c), id, version, admin)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, out)
	}
}
func (h PODOfficialHandler) Versions(c *gin.Context) {
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	rows, total, err := h.Service.Versions(c.Request.Context(), middleware.CurrentUser(c), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODOfficialHandler) Write(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		if op != "official.create" {
			var ok bool
			id, ok = podAssetID(c)
			if !ok {
				return
			}
		}
		var r model.PODOfficialWrite
		if !podBind(c, &r) {
			return
		}
		out, err := h.Service.Write(c.Request.Context(), middleware.CurrentUser(c), id, op, r)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, out)
	}
}
func (h PODOfficialHandler) Assets(c *gin.Context) {
	rows, total, err := h.Service.Assets(c.Request.Context(), middleware.CurrentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODOfficialHandler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.Service.Personal.Limits.MaxUploadBytes+(64<<10))
	reader, err := c.Request.MultipartReader()
	if err != nil {
		response.FailCode(c, 400, "请使用 multipart/form-data 上传图片")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		response.FailCode(c, 400, "请选择一张平台图片，字段名为 file")
		return
	}
	defer part.Close()
	_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		response.FailCode(c, 400, "文件名无效")
		return
	}
	out, err := h.Service.Upload(c.Request.Context(), middleware.CurrentUser(c), c.Query("requestId"), c.Query("licenseNote"), params["filename"], part.Header.Get("Content-Type"), &singleUpload{part: part, reader: reader})
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func (h PODOfficialHandler) Content(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	f, meta, err := h.Service.Open(ctx, middleware.CurrentUser(c), id, c.DefaultQuery("variant", "thumbnail"))
	if err != nil {
		fail(c, err)
		return
	}
	defer f.Close()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", meta.MediaType)
	c.Header("Content-Length", strconv.FormatInt(meta.ByteSize, 10))
	_, _ = storage.Copy(ctx, c.Writer, f)
}
func (h PODOfficialHandler) Use(c *gin.Context) {
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	var r model.PODTemplateUse
	if !podBind(c, &r) {
		return
	}
	out, err := h.Service.Use(c.Request.Context(), middleware.CurrentUser(c), id, r)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func (h PODOfficialHandler) UseAction(c *gin.Context) {
	out, err := h.Service.UseAction(c.Request.Context(), middleware.CurrentUser(c), c.Param("requestId"))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
