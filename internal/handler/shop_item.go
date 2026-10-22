package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPShopItemList(c *gin.Context) {
	var q model.ERPShopItemQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	rows, total, err := service.ListERPShopItems(c.Request.Context(), q, page.Parse(c, model.ERPShopItemSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ERPShopItemGet(c *gin.Context) {
	id, err := parseID(c.Param("itemId"))
	if err != nil {
		fail(c, err)
		return
	}
	result, err := service.GetERPShopItem(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, result)
}
func ERPShopItemAdd(c *gin.Context) {
	var input model.ERPShopItemWrite
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	result, err := service.CreateERPShopItem(c.Request.Context(), input, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, result)
}
func ERPShopItemEdit(c *gin.Context) {
	id, err := parseID(c.Param("itemId"))
	if err != nil {
		fail(c, err)
		return
	}
	var input model.ERPShopItemWrite
	if err = c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err = service.EditERPShopItem(c.Request.Context(), id, input, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func ERPShopItemMap(c *gin.Context) {
	id, err := parseID(c.Param("itemId"))
	if err != nil {
		fail(c, err)
		return
	}
	var input model.ERPMappingWrite
	if err = c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	result, err := service.SetERPShopItemMapping(c.Request.Context(), id, input, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, result)
}
func ERPShopItemUnmap(c *gin.Context) {
	id, err := parseID(c.Param("itemId"))
	if err != nil {
		fail(c, err)
		return
	}
	var input struct {
		Version int64 `form:"version" binding:"required,min=1"`
	}
	if err = c.ShouldBindQuery(&input); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	result, err := service.CancelERPShopItemMapping(c.Request.Context(), id, input.Version, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, result)
}
