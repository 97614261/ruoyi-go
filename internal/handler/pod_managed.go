package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func PODCreatorModels(c *gin.Context) {
	rows, err := service.PODCreatorModels(c.Request.Context(), currentUser(c), c.Query("kind"))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, rows)
}
func PODCreatorModel(c *gin.Context) {
	id, err := service.PODPositiveID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	rows, err := service.PODCreatorModels(c.Request.Context(), currentUser(c), "")
	if err != nil {
		fail(c, err)
		return
	}
	for _, r := range rows {
		if r.ModelID == id {
			response.OkData(c, r)
			return
		}
	}
	response.FailCode(c, 404, "模型不存在或无权使用")
}
func (h PODRunHandler) Quote(c *gin.Context) {
	var body model.PODQuoteRequest
	if !podBind(c, &body) {
		return
	}
	view, err := h.Service.Quote(c.Request.Context(), currentUser(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
func PODQuoteGet(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil || id.String() != c.Param("id") {
		response.FailCode(c, 400, "报价编号无效")
		return
	}
	view, err := service.PODCreatorQuote(c.Request.Context(), currentUser(c), id.String())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, view)
}
