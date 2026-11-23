package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

type PODRunHandler struct{ Service service.PODRunService }

func (h PODRunHandler) Usage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	view, err := h.Service.Usage(c.Request.Context(), currentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}

func (h PODRunHandler) Create(c *gin.Context) {
	var body model.PODRunCreate
	if !podBind(c, &body) {
		return
	}
	r, err := h.Service.Create(c.Request.Context(), currentUser(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, r)
}
func (h PODRunHandler) List(c *gin.Context) {
	id := int64(0)
	if c.Query("projectId") != "" {
		var err error
		id, err = service.PODPositiveID(c.Query("projectId"))
		if err != nil {
			fail(c, err)
			return
		}
	}
	rows, total, err := h.Service.List(c.Request.Context(), currentUser(c), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODRunHandler) Get(c *gin.Context) {
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	r, err := h.Service.Get(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, r)
}
func (h PODRunHandler) Cancel(c *gin.Context) { h.change(c, false) }
func (h PODRunHandler) Retry(c *gin.Context)  { h.change(c, true) }
func (h PODRunHandler) change(c *gin.Context, retry bool) {
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	key := ""
	quote := ""
	if retry {
		var body model.PODRunRetry
		if !podBind(c, &body) {
			return
		}
		if body.RequestID == "" {
			response.FailCode(c, 400, "请提供恢复请求标识")
			return
		}
		key = body.RequestID
		quote = body.QuoteID
	} else {
		var body struct{}
		if !podBind(c, &body) {
			return
		}
	}
	r, err := h.Service.Change(c.Request.Context(), currentUser(c), id, key, quote)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, r)
}
func (h PODRunHandler) Credits(c *gin.Context) {
	a, rows, total, err := h.Service.Credits(c.Request.Context(), currentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.New(200, response.MsgQuery).Put("account", a).Put("rows", rows).Put("total", total).JSON(c)
}
