package handler

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/storage"
)

type PODBatchHandler struct{ Service service.PODBatchService }

func (h PODBatchHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rows, total, err := h.Service.List(c.Request.Context(), currentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODBatchHandler) Get(c *gin.Context)  { h.action(c, "get") }
func (h PODBatchHandler) Stop(c *gin.Context) { h.action(c, "stop") }
func (h PODBatchHandler) action(c *gin.Context, action string) {
	c.Header("Cache-Control", "no-store")
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	var view *model.PODBatchView
	if action == "stop" {
		var body struct{}
		if !podBind(c, &body) {
			return
		}
		view, err = h.Service.Stop(c.Request.Context(), currentUser(c), id)
	} else {
		view, err = h.Service.Get(c.Request.Context(), currentUser(c), id)
	}
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
func (h PODBatchHandler) Preview(c *gin.Context) { h.create(c, true) }
func (h PODBatchHandler) Create(c *gin.Context)  { h.create(c, false) }
func (h PODBatchHandler) create(c *gin.Context, preview bool) {
	c.Header("Cache-Control", "no-store")
	var body model.PODBatchCreate
	if !podBind(c, &body) {
		return
	}
	if preview {
		v, err := h.Service.Preview(c.Request.Context(), currentUser(c), body)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, v)
		return
	}
	v, err := h.Service.Create(c.Request.Context(), currentUser(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, v)
}
func (h PODBatchHandler) ZIP(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	file, err := h.Service.ZIP(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"batch-%d.zip\"", id))
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = storage.Copy(c.Request.Context(), c.Writer, file)
}
