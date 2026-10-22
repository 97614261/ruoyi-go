package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
)

func init() {
	generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPShopItemGenerated)
}
func registerERPShopItemGenerated(g *gin.RouterGroup) {
	group := g.Group("/erp/shopItem", middleware.CustomerWorkspaceOnly())
	group.GET("/list", middleware.HasPermission("erp:shopItem:list"), handler.ERPShopItemList)
	group.GET("/:itemId", middleware.HasPermission("erp:shopItem:query"), handler.ERPShopItemGet)
	group.POST("", noRepeat(), middleware.HasPermission("erp:shopItem:add"), middleware.OperLog("店铺商品", model.BusinessTypeInsert), handler.ERPShopItemAdd)
	group.PUT("/:itemId", middleware.HasPermission("erp:shopItem:edit"), middleware.OperLog("店铺商品", model.BusinessTypeUpdate), handler.ERPShopItemEdit)
	// Mapping success audits are synchronous and transactional in repository.
	group.PUT("/:itemId/mapping", middleware.HasPermission("erp:shopItem:map"), handler.ERPShopItemMap)
	group.DELETE("/:itemId/mapping", middleware.HasPermission("erp:shopItem:map"), handler.ERPShopItemUnmap)
}
