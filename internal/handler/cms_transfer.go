package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func CMSTransferTemplate(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		response.OkData(c, service.CMSTransferTemplate(kind))
	}
}
func CMSImportDrafts(kind string, apply bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		b, e := io.ReadAll(io.LimitReader(c.Request.Body, service.CMSTransferLimit+1))
		if e != nil || len(b) > service.CMSTransferLimit {
			response.Fail(c, "导入JSON最多1MiB")
			return
		}
		var in model.CMSTransfer
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if e = decoder.Decode(&in); e != nil {
			response.Fail(c, "JSON格式、字段或字段类型不正确")
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			response.Fail(c, "只允许一个JSON对象")
			return
		}
		out, e := service.CMSImportDrafts(c.Request.Context(), kind, in, currentUsername(c), middleware.CurrentUser(c).HasPermission("cms:asset:query"), apply)
		if e != nil {
			fail(c, e)
			return
		}
		if apply && out.FailureCount > 0 {
			middleware.MarkOperLogFailure(c, fmt.Sprintf("导入存在失败：成功%d，失败%d", out.SuccessCount, out.FailureCount))
		}
		c.Header("Cache-Control", "no-store")
		response.OkData(c, out)
	}
}
func CMSExportDrafts(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			Items []model.CMSBatchItem `json:"items" binding:"required,min=1,max=20,dive"`
		}
		if e := c.ShouldBindJSON(&in); e != nil {
			response.Fail(c, "请选择1—20条有效内容")
			return
		}
		out, e := service.CMSExportDrafts(c.Request.Context(), kind, in.Items, middleware.CurrentUser(c).HasPermission("cms:asset:query"))
		if e != nil {
			fail(c, e)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.OkData(c, out)
	}
}
