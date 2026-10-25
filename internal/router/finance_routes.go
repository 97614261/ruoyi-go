package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerFinance) }
func registerFinance(g *gin.RouterGroup) {
	p := g.Group("/erp/finance", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	a := p.Group("/accounts")
	a.GET("", middleware.HasPermission("erp:financeAccount:list"), handler.FinanceList("account"))
	a.GET("/:id", middleware.HasPermission("erp:financeAccount:query"), handler.FinanceGet("account"))
	a.POST("", middleware.HasPermission("erp:financeAccount:add"), handler.FinanceWrite("ACCOUNT_CREATE"))
	a.PUT("/:id", middleware.HasPermission("erp:financeAccount:edit"), handler.FinanceWrite("ACCOUNT_EDIT"))
	s := p.Group("/sources", middleware.HasPermission("erp:payable:add"))
	s.GET("", handler.FinanceList("source"))
	s.GET("/:id", handler.FinanceGet("source"))
	b := p.Group("/payables")
	b.GET("", middleware.HasPermission("erp:payable:list"), handler.FinanceList("payable"))
	b.GET("/:id", middleware.HasPermission("erp:payable:query"), handler.FinanceGet("payable"))
	b.POST("", middleware.HasPermission("erp:payable:add"), handler.FinanceWrite("PAYABLE_CREATE"))
	b.GET("/:id/returns", middleware.HasPermission("erp:payable:adjust"), handler.FinanceReturns)
	b.POST("/:id/adjust", middleware.HasPermission("erp:payable:adjust"), handler.FinanceWrite("PAYABLE_ADJUST"))
	b.POST("/export", middleware.ExportLimit(), middleware.HasPermission("erp:payable:export"), handler.FinanceExport(false))
	m := p.Group("/payments")
	m.GET("", middleware.HasPermission("erp:payment:list"), handler.FinanceList("payment"))
	m.GET("/:id", middleware.HasPermission("erp:payment:query"), handler.FinanceGet("payment"))
	m.POST("", middleware.HasPermission("erp:payment:add"), handler.FinanceWrite("PAYMENT_CREATE"))
	m.POST("/:id/reverse", middleware.HasPermission("erp:payment:reverse"), handler.FinanceWrite("PAYMENT_REVERSE"))
	p.GET("/statements", middleware.HasPermission("erp:financeStatement:list"), handler.FinanceList("statement"))
	p.POST("/statements/export", middleware.ExportLimit(), middleware.HasPermission("erp:financeStatement:export"), handler.FinanceExport(true))
}
