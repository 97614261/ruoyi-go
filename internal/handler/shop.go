package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPChannelList(c *gin.Context) {
	rows, total, err := service.ListERPChannels(c.Request.Context(), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ErpShopList(c *gin.Context) {
	var query model.ErpShopQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	rows, total, err := service.ListErpShopPage(c.Request.Context(), query, page.Parse(c, model.ErpShopSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ErpShopGet(c *gin.Context) {
	id, err := parseID(c.Param("shopId"))
	if err != nil {
		fail(c, err)
		return
	}
	target, err := service.GetErpShop(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, target)
}
func ErpShopAdd(c *gin.Context) {
	var input model.ERPShopWrite
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	target, err := service.CreateErpShop(c.Request.Context(), input, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, target)
}
func ErpShopEdit(c *gin.Context) {
	id, err := parseID(c.Param("shopId"))
	if err != nil {
		fail(c, err)
		return
	}
	var input model.ERPShopWrite
	if err = c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err = service.UpdateErpShop(c.Request.Context(), id, input, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func ERPShopAccessGet(c *gin.Context) {
	id, err := parseID(c.Param("userId"))
	if err != nil {
		fail(c, err)
		return
	}
	v, err := service.GetERPShopAccess(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, v)
}
func ERPShopAccessSet(c *gin.Context) {
	id, err := parseID(c.Param("userId"))
	if err != nil {
		fail(c, err)
		return
	}
	var input model.ERPShopAccessRequest
	if err = c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err = service.SetERPShopAccess(c.Request.Context(), id, input.Version, *input.ShopIDs, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
