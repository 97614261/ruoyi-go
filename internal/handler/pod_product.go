package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/storage"
)

type PODProductHandler struct{ Service service.PODProductService }

func (h PODProductHandler) Packs(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	rows, total, err := h.Service.Packs(c.Request.Context(), currentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}

func (h PODProductHandler) Jobs(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	rows, total, err := h.Service.Jobs(c.Request.Context(), currentUser(c), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}

func (h PODProductHandler) Create(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var body model.PODProductCreate
	if !podBind(c, &body) {
		return
	}
	view, err := h.Service.Create(c.Request.Context(), currentUser(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}

func (h PODProductHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	view, err := h.Service.Get(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}

func (h PODProductHandler) History(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	rows, total, err := h.Service.History(c.Request.Context(), currentUser(c), id, c.Param("node"), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}

func (h PODProductHandler) SetFinal(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var body model.PODProductFinalChange
	if !podBind(c, &body) {
		return
	}
	view, err := h.Service.SetFinal(c.Request.Context(), currentUser(c), id, c.Param("node"), body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}

func (h PODProductHandler) ZIP(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	selection := c.Query("selection")
	if selection != "" && selection != "final" {
		response.FailCode(c, 400, "下载类型不合法")
		return
	}
	archive, err := h.Service.ZIP(c.Request.Context(), currentUser(c), id, selection == "final")
	if err != nil {
		fail(c, err)
		return
	}
	file := archive.File
	defer os.Remove(file.Name())
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"product-%d.zip\"; filename*=UTF-8''%s", id, url.PathEscape(archive.Name)))
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = storage.Copy(c.Request.Context(), c.Writer, file)
}
