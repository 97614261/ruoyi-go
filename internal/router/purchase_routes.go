package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPPurchases) }
func registerERPPurchases(g *gin.RouterGroup) {
	p := g.Group("/erp/purchase", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	p.GET("/list", middleware.HasPermission("erp:purchase:list"), handler.ERPPurchaseList)
	p.GET("/receivable/list", middleware.HasPermission("erp:purchase:receive"), handler.ERPPurchaseReceivables)
	p.GET("/:purchaseId/receivable", middleware.HasPermission("erp:purchase:receive"), handler.ERPPurchaseReceivable)
	p.POST("/:purchaseId/receipts", middleware.HasPermission("erp:purchase:receive"), handler.ERPPurchaseReceive)
	p.GET("/:purchaseId", middleware.HasPermission("erp:purchase:query"), handler.ERPPurchaseGet)
	p.GET("/:purchaseId/events", middleware.HasPermission("erp:purchase:query"), handler.ERPPurchaseEvents)
	// Domain idempotency and same-transaction minimal audits; never generic body capture.
	p.POST("", middleware.HasPermission("erp:purchase:add"), handler.ERPPurchaseAdd)
	p.PUT("/:purchaseId", middleware.HasPermission("erp:purchase:edit"), handler.ERPPurchaseEdit)
	p.PUT("/:purchaseId/confirm", middleware.HasPermission("erp:purchase:confirm"), handler.ERPPurchaseAct("CONFIRM"))
	p.PUT("/:purchaseId/cancel", middleware.HasPermission("erp:purchase:cancel"), handler.ERPPurchaseAct("CANCEL"))
	p.PUT("/:purchaseId/close", middleware.HasPermission("erp:purchase:close"), handler.ERPPurchaseAct("CLOSE"))
	r := g.Group("/erp/purchaseReceipt", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	r.GET("/list", middleware.HasPermission("erp:purchaseReceipt:list"), handler.ERPPurchaseReceiptList)
	r.GET("/:receiptId", middleware.HasPermission("erp:purchaseReceipt:query"), handler.ERPPurchaseReceiptGet)
}
