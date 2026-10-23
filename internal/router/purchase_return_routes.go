package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPPurchaseReturns) }
func registerERPPurchaseReturns(g *gin.RouterGroup) {
	noStore := func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }
	p := g.Group("/erp/purchaseReturnOrder", middleware.CustomerWorkspaceOnly(), noStore)
	p.GET("/returnable/list", middleware.HasPermission("erp:purchaseReturnOrder:list"), handler.ERPPurchaseReturnableList)
	p.GET("/list", middleware.HasPermission("erp:purchaseReturnOrder:list"), handler.ERPPurchaseReturnList)
	p.GET("/:returnId", middleware.HasPermission("erp:purchaseReturnOrder:query"), handler.ERPPurchaseReturnGet)
	r := g.Group("/erp/purchaseReceipt", middleware.CustomerWorkspaceOnly(), noStore)
	r.GET("/:receiptId/returnable", middleware.HasPermission("erp:purchaseReturnOrder:return"), handler.ERPPurchaseReturnableGet)
	r.POST("/:receiptId/returns", middleware.HasPermission("erp:purchaseReturnOrder:return"), handler.ERPPurchaseReturnPost)
}
