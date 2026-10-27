package repository

import (
	"strings"

	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// Offset still applies to the exact authorized, filtered ID sequence. Only the
// selected page fetches summary columns; moving scope or filters outside the ID
// query would change pages and expose the existence of inaccessible orders.
// Keep IDs and summaries in one statement, including tenant identity in the
// join and the normal outer tenant/live-shop guards. No separately cached IDs.
const orderDeferredPageOffset = 10000

func orderPageQuery(tx *gorm.DB, q model.ERPOrderQuery, pg page.Query) *gorm.DB {
	// Local alternating probes show the original index-ordered read is faster
	// at offsets 0..5000, while deferred fetch wins at 10000 and 39980. Keep
	// shallow pages unchanged; this conservative cutoff is not a universal
	// optimizer guarantee. Re-measure with representative data before tuning.
	if pg.Offset() < orderDeferredPageOffset {
		return orderListQuery(tx, q).Select(orderSummaryColumns).Order("order_id DESC").Limit(pg.PageSize).Offset(pg.Offset())
	}
	ids := orderListQuery(tx.Session(&gorm.Session{}), q).
		Select("tenant_id,order_id").Order("order_id DESC").Limit(pg.PageSize).Offset(pg.Offset())
	return orderScope(tx.Session(&gorm.Session{}).Model(&model.ERPOrder{}).Table("(?) AS order_page", ids)).
		Joins("INNER JOIN erp_order ON erp_order.tenant_id=order_page.tenant_id AND erp_order.order_id=order_page.order_id").
		Select("erp_order." + strings.ReplaceAll(orderSummaryColumns, ",", ",erp_order.")).
		Order("erp_order.order_id DESC")
}
