package handler

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func PODCatalogList(providers bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		f := model.PODCatalogFilter{Code: c.Query("code"), Name: c.Query("name"), Kind: c.Query("kind"), Status: c.Query("status")}
		if raw := c.Query("providerId"); raw != "" {
			id, err := service.PODPositiveID(raw)
			if err != nil {
				fail(c, err)
				return
			}
			f.ProviderID = id
		}
		rows, total, err := service.PODCatalogList(c.Request.Context(), middleware.CurrentUser(c), providers, f, page.Parse(c, nil))
		if err != nil {
			fail(c, err)
			return
		}
		response.Page(c, rows, total)
	}
}
func PODCatalogGet(providers bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := service.PODPositiveID(c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		view, err := service.PODCatalogGet(c.Request.Context(), middleware.CurrentUser(c), providers, id)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, view)
	}
}
func PODCatalogWrite(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := int64(0)
		if raw := c.Param("id"); raw != "" {
			var err error
			id, err = service.PODPositiveID(raw)
			if err != nil {
				fail(c, err)
				return
			}
		}
		if c.ContentType() != "application/json" {
			fail(c, errs.WithCode(400, "请使用 JSON 请求"))
			return
		}
		var request model.PODCatalogWrite
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, (128<<10)+1))
		if err != nil || len(body) > 128<<10 {
			fail(c, errs.WithCode(400, "模型管理请求不得超过 128 KiB"))
			return
		}
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		if d.Decode(&request) != nil || d.Decode(new(any)) != io.EOF {
			fail(c, errs.WithCode(400, "模型管理 JSON 无效或包含未知字段"))
			return
		}
		view, err := service.PODCatalogWrite(c.Request.Context(), middleware.CurrentUser(c), op, id, request)
		request.Secret = ""
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, view)
	}
}
func PODCatalogVersions(c *gin.Context) {
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	rows, total, err := service.PODCatalogVersions(c.Request.Context(), middleware.CurrentUser(c), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func PODCatalogDefaults(c *gin.Context) {
	view, err := service.PODCatalogGetDefaults(c.Request.Context(), middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
func PODCatalogAction(c *gin.Context) {
	view, err := service.PODCatalogGetAction(c.Request.Context(), middleware.CurrentUser(c), c.Param("requestId"))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
func PODCatalogCheck(c *gin.Context) {
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	view, err := service.PODCatalogCheck(c.Request.Context(), middleware.CurrentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
