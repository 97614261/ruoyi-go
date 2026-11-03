package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func CMSAssetStatistics(c *gin.Context) {
	var q model.CMSAssetQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	out, e := service.CMSAssetStatistics(c.Request.Context(), q)
	if e != nil {
		fail(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.OkData(c, out)
}
