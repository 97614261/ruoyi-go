package handler

import (
	"encoding/json"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

type PODHandler struct {
	Limits      model.PODGraphLimits
	AssetLimits model.PODAssetLimits
	Execution   model.PODExecutionLimits
}

func podBind(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.FailCode(c, 400, "请求 JSON 格式或字段不正确")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		response.FailCode(c, 400, "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}
func podProjectID(c *gin.Context) (int64, bool) {
	value := c.Param("id")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		response.FailCode(c, 400, "项目编号不合法")
		return 0, false
	}
	return id, true
}
func (h PODHandler) InitWorkspace(c *gin.Context) {
	w, err := service.InitPODWorkspace(c.Request.Context(), currentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, w)
}
func (h PODHandler) List(c *gin.Context) {
	deleted := c.Query("deleted")
	if deleted != "" && deleted != "false" && deleted != "true" {
		response.FailCode(c, 400, "回收站参数必须为 true 或 false")
		return
	}
	rows, total, err := service.ListPODProjectPage(c.Request.Context(), currentUser(c), deleted == "true", page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODHandler) Get(c *gin.Context) {
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	row, err := service.GetPODProject(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODHandler) Create(c *gin.Context) {
	var body model.PODProjectCreate
	if !podBind(c, &body) {
		return
	}
	row, err := service.CreatePODProject(c.Request.Context(), currentUser(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODHandler) Update(c *gin.Context) {
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	var body model.PODProjectUpdate
	if !podBind(c, &body) {
		return
	}
	row, err := service.EditPODProject(c.Request.Context(), currentUser(c), id, body, h.Limits)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODHandler) Delete(c *gin.Context)  { h.deletion(c, false) }
func (h PODHandler) Restore(c *gin.Context) { h.deletion(c, true) }
func (h PODHandler) deletion(c *gin.Context, restore bool) {
	id, ok := podProjectID(c)
	if !ok {
		return
	}
	var body model.PODProjectVersion
	if !podBind(c, &body) {
		return
	}
	row, err := service.ChangePODProjectDeletion(c.Request.Context(), currentUser(c), id, body.Revision, restore)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
