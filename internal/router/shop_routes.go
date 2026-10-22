package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerErpShopGenerated) }
func registerErpShopGenerated(g *gin.RouterGroup) {
	channels := g.Group("/erp/channel", middleware.CustomerWorkspaceOnly())
	channels.GET("/list", middleware.HasPermission("erp:channel:list"), handler.ERPChannelList)
	group := g.Group("/erp/shop", middleware.CustomerWorkspaceOnly())
	group.GET("/list", middleware.HasPermission("erp:shop:list"), handler.ErpShopList)
	group.GET("/access/:userId", middleware.HasPermission("erp:shop:grant"), handler.ERPShopAccessGet)
	// Success audits are inserted atomically with the policy, not by async OperLog.
	group.PUT("/access/:userId", middleware.HasPermission("erp:shop:grant"), handler.ERPShopAccessSet)
	group.GET("/:shopId", middleware.HasPermission("erp:shop:query"), handler.ErpShopGet)
	group.POST("", noRepeat(), middleware.HasPermission("erp:shop:add"), middleware.OperLog("店铺", model.BusinessTypeInsert), handler.ErpShopAdd)
	group.PUT("/:shopId", middleware.HasPermission("erp:shop:edit"), middleware.OperLog("店铺", model.BusinessTypeUpdate), handler.ErpShopEdit)
}
