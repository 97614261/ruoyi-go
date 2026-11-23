package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"strconv"
)

type PODTemplateHandler struct{ Service service.PODTemplateService }

func podTemplateNumber(c *gin.Context, key string) (int64, bool) {
	text := c.Param(key)
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text {
		response.FailCode(c, 400, "模板编号或版本不合法")
		return 0, false
	}
	return id, true
}
func (h PODTemplateHandler) List(c *gin.Context) {
	value := c.Query("deleted")
	if value != "" && value != "true" && value != "false" {
		response.FailCode(c, 400, "回收站参数必须为 true 或 false")
		return
	}
	rows, total, err := h.Service.List(c.Request.Context(), currentUser(c), value == "true", page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODTemplateHandler) Get(c *gin.Context) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	row, err := h.Service.Get(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODTemplateHandler) Save(c *gin.Context) {
	var id int64
	if c.Param("id") != "" {
		var ok bool
		id, ok = podTemplateNumber(c, "id")
		if !ok {
			return
		}
	}
	var body model.PODTemplateSave
	if !podBind(c, &body) {
		return
	}
	row, err := h.Service.Save(c.Request.Context(), currentUser(c), id, body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODTemplateHandler) Update(c *gin.Context) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	var body model.PODTemplateUpdate
	if !podBind(c, &body) {
		return
	}
	row, err := h.Service.Update(c.Request.Context(), currentUser(c), id, body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODTemplateHandler) Delete(c *gin.Context)  { h.deletion(c, false) }
func (h PODTemplateHandler) Restore(c *gin.Context) { h.deletion(c, true) }
func (h PODTemplateHandler) deletion(c *gin.Context, restore bool) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	var body model.PODProjectVersion
	if !podBind(c, &body) {
		return
	}
	row, err := h.Service.Deletion(c.Request.Context(), currentUser(c), id, body.Revision, restore)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODTemplateHandler) Versions(c *gin.Context) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	rows, total, err := h.Service.Versions(c.Request.Context(), currentUser(c), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODTemplateHandler) Version(c *gin.Context) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	version, ok := podTemplateNumber(c, "version")
	if !ok {
		return
	}
	row, err := h.Service.Version(c.Request.Context(), currentUser(c), id, version)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODTemplateHandler) Copy(c *gin.Context) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	var body model.PODTemplateCopy
	if !podBind(c, &body) {
		return
	}
	row, err := h.Service.Copy(c.Request.Context(), currentUser(c), id, body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODTemplateHandler) Use(c *gin.Context) {
	id, ok := podTemplateNumber(c, "id")
	if !ok {
		return
	}
	var body model.PODTemplateUse
	if !podBind(c, &body) {
		return
	}
	row, err := h.Service.Use(c.Request.Context(), currentUser(c), id, body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
