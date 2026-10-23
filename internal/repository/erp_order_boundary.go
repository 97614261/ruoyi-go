package repository

import (
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
)

// Trusted column constants only. Every nested table has an explicit tenant
// predicate: the guard does not parse arbitrary subqueries. Both the managed
// SALE and an SR referencing it inherit shop scope; standalone vouchers remain
// warehouse-only. Reads never lock order/shop after the voucher lock.
func erpManagedDocumentScope(q *gorm.DB, column string) *gorm.DB {
	ctx := q.Statement.Context
	tid, uid := tenantctx.ID(ctx), erpctx.UserID(ctx)
	if tid < 1 || uid < 1 || tenantctx.Get(ctx).Platform {
		q.AddError(errs.New("缺少企业ERP身份"))
		return q
	}
	return q.Where(`NOT EXISTS (SELECT 1 FROM erp_order mo WHERE mo.tenant_id=? AND (mo.stock_document_id=`+column+` OR mo.stock_document_id=(SELECT md.source_document_id FROM erp_document md WHERE md.tenant_id=? AND md.document_id=`+column+`)) AND NOT (EXISTS (SELECT 1 FROM sys_tenant mt WHERE mt.tenant_id=? AND mt.owner_user_id=?) OR mo.shop_id IN (SELECT ma.shop_id FROM erp_shop_access ma WHERE ma.tenant_id=? AND ma.user_id=?)))`, tid, tid, tid, uid, tid, uid)
}
func requireERPDocumentRead(tx *gorm.DB, id int64) error {
	var n int64
	if err := erpManagedDocumentScope(erpWarehouseScope(tx.Model(&model.ERPDocument{}), "erp_document.warehouse_id"), "erp_document.document_id").Where("document_id=?", id).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return errs.New("单据不存在或无关联店铺权限")
	}
	return nil
}
func requireERPDocumentNotManaged(tx *gorm.DB, id int64) error {
	// Do not reveal a managed-voucher discriminator before current read scope.
	if err := requireERPDocumentRead(tx, id); err != nil {
		return err
	}
	var hits []int
	if err := tx.Model(&model.ERPOrder{}).Select("1").Where("stock_document_id=?", id).Limit(1).Scan(&hits).Error; err != nil {
		return err
	}
	if len(hits) > 0 {
		return errs.New("该库存凭据由商业订单托管，请从订单/包裹执行操作")
	}
	return requireERPPurchaseVoucherIndependent(tx, id)
}
