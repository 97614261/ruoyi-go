package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPWorkflow) }
func registerERPWorkflow(g *gin.RouterGroup) {
	erp := g.Group("/erp", middleware.CustomerWorkspaceOnly())
	for _, spec := range []struct{ path, kind string }{{"receipt", "IN"}, {"sales", "SALE"}, {"salesReturn", "SR"}, {"purchaseReturn", "PR"}, {"count", "COUNT"}} {
		p := spec.path
		k := spec.kind
		group := erp.Group("/" + p)
		group.GET("/list", middleware.HasPermission("erp:"+p+":list"), handler.ERPDocumentList(k))
		group.GET("/:documentId", middleware.HasPermission("erp:"+p+":query"), handler.ERPDocumentGet(k))
		group.POST("", middleware.HasPermission("erp:"+p+":add"), middleware.OperLog("ERP单据", model.BusinessTypeInsert), handler.ERPDocumentAdd(k))
		group.PUT("/:documentId", middleware.HasPermission("erp:"+p+":edit"), middleware.OperLog("ERP草稿编辑", model.BusinessTypeUpdate), handler.ERPDocumentEdit(k))
		group.PUT("/:documentId/confirm", middleware.HasPermission("erp:"+p+":confirm"), middleware.OperLog("ERP确认", model.BusinessTypeUpdate), handler.ERPDocumentAction(k, "confirm"))
		group.PUT("/:documentId/cancel", middleware.HasPermission("erp:"+p+":cancel"), middleware.OperLog("ERP取消", model.BusinessTypeUpdate), handler.ERPDocumentAction(k, "cancel"))
		if k == "SALE" {
			group.PUT("/:documentId/ship", middleware.HasPermission("erp:sales:ship"), middleware.OperLog("ERP出库", model.BusinessTypeUpdate), handler.ERPDocumentAction(k, "ship"))
		}
	}
	erp.GET("/stock/list", middleware.HasPermission("erp:stock:list"), handler.ERPStockList)
	erp.GET("/ledger/list", middleware.HasPermission("erp:ledger:list"), handler.ERPLedgerList)
	erp.GET("/warehouse/access/:userId", middleware.HasPermission("erp:warehouse:grant"), handler.ERPWarehouseAccessGet)
	erp.PUT("/warehouse/access/:userId", middleware.HasPermission("erp:warehouse:grant"), handler.ERPWarehouseAccessSet)
}
