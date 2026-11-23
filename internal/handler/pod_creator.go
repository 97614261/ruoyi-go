package handler

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func PODCreators(c *gin.Context) {
	rows, n, err := service.PODCreatorList(c.Request.Context(), middleware.CurrentUser(c), c.Query("name"), c.Query("status"), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, n)
}
func PODCreatorLimits(l model.PODExecutionLimits) gin.HandlerFunc {
	return func(c *gin.Context) {
		response.OkData(c, gin.H{"maxConcurrentNodes": l.UserConcurrency, "maxPendingRuns": l.MaxUserPending, "maxDailySubmissions": 10000, "maxStorageBytes": "10737418240", "writesReady": service.PODCatalogReady()})
	}
}
func PODCreatorModelOptions(c *gin.Context) {
	rows, n, err := service.PODCreatorModelOptions(c.Request.Context(), middleware.CurrentUser(c), c.Query("name"), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, n)
}
func PODCreatorGet(c *gin.Context) {
	id, e := service.PODPositiveID(c.Param("userId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.PODCreatorGet(c.Request.Context(), middleware.CurrentUser(c), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func PODCreatorPoints(c *gin.Context) {
	id, e := service.PODPositiveID(c.Param("userId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.PODCreatorPoints(c.Request.Context(), middleware.CurrentUser(c), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func PODCreatorEvents(c *gin.Context) {
	id, e := service.PODPositiveID(c.Param("userId"))
	if e != nil {
		fail(c, e)
		return
	}
	rows, n, e := service.PODCreatorEvents(c.Request.Context(), middleware.CurrentUser(c), id, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func PODCreatorWrite(op string, l model.PODExecutionLimits) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		var e error
		if op != "creator.create" {
			id, e = service.PODPositiveID(c.Param("userId"))
			if e != nil {
				fail(c, e)
				return
			}
		}
		if c.ContentType() != "application/json" {
			fail(c, errs.WithCode(400, "请使用 application/json"))
			return
		}
		b, e := io.ReadAll(io.LimitReader(c.Request.Body, (128<<10)+1))
		if e != nil || len(b) > 128<<10 {
			fail(c, errs.WithCode(400, "请求体无效或超限"))
			return
		}
		var r model.PODCreatorWrite
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
			fail(c, errs.WithCode(400, "请求 JSON 或字段无效"))
			return
		}
		v, e := service.PODCreatorWrite(c.Request.Context(), middleware.CurrentUser(c), op, id, r, l)
		r.InitialPassword = ""
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, v)
	}
}
func PODMyEligibility(c *gin.Context) {
	u := middleware.CurrentUser(c)
	v, e := service.PODMyEligibility(c.Request.Context(), u.UserID)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func PODMyPoints(c *gin.Context) {
	u := middleware.CurrentUser(c)
	v, e := service.PODMyPoints(c.Request.Context(), u.UserID, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
