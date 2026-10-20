package service

// ERP-specific identifiers do not change the frozen Java baseline manifest.
var erpMenuPermissions = stringSet(
	"erp:sku:list", "erp:sku:query", "erp:sku:add", "erp:sku:edit", "erp:sku:remove", "erp:sku:export",
	"erp:warehouse:list", "erp:warehouse:query", "erp:warehouse:add", "erp:warehouse:edit", "erp:warehouse:remove", "erp:warehouse:export",
	"erp:receipt:list", "erp:receipt:query", "erp:receipt:add", "erp:receipt:confirm", "erp:receipt:cancel",
	"erp:sales:list", "erp:sales:query", "erp:sales:add", "erp:sales:confirm", "erp:sales:cancel", "erp:sales:ship",
	"erp:stock:list", "erp:ledger:list",
	"erp:warehouse:grant", "erp:receipt:edit", "erp:sales:edit",
	"erp:salesReturn:list", "erp:salesReturn:query", "erp:salesReturn:add", "erp:salesReturn:confirm", "erp:salesReturn:cancel", "erp:salesReturn:edit",
	"erp:purchaseReturn:list", "erp:purchaseReturn:query", "erp:purchaseReturn:add", "erp:purchaseReturn:confirm", "erp:purchaseReturn:cancel", "erp:purchaseReturn:edit",
	"erp:count:list", "erp:count:query", "erp:count:add", "erp:count:confirm", "erp:count:cancel", "erp:count:edit",
)
