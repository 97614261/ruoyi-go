package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// Private capability, never a request/context bypass. FK links and this check
// bind one newly inserted SR to its immutable head, shipped package and SALE.
type salesReturnSource struct{ returnID, packageID int64 }

func (p *salesReturnSource) verify(tx *gorm.DB, doc, source *model.ERPDocument) error {
	if p == nil || p.returnID < 1 || p.packageID < 1 || doc.Kind != "SR" || doc.Status != "DRAFT" || doc.Reversal || doc.SourceDocumentID == nil || *doc.SourceDocumentID != source.DocumentID {
		return errs.New("订单退货来源证明不匹配")
	}
	var head model.ERPSalesReturn
	if e := tx.Where("return_id=? AND package_id=? AND stock_document_id=? AND warehouse_id=?", p.returnID, p.packageID, doc.DocumentID, doc.WarehouseID).Take(&head).Error; e != nil {
		return erpReferenceError(e)
	}
	var n int64
	if e := tx.Model(&model.ERPOrder{}).Where("order_id=? AND shop_id=? AND warehouse_id=? AND stock_document_id=? AND status='SHIPPED'", head.OrderID, head.ShopID, head.WarehouseID, source.DocumentID).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errs.New("退货与原发货关联异常")
	}
	if e := tx.Model(&model.ERPPackage{}).Where("package_id=? AND order_id=? AND warehouse_id=? AND status='SHIPPED'", head.PackageID, head.OrderID, head.WarehouseID).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errs.New("退货包裹关联异常")
	}
	return nil
}

func salesReturnInstalled(tx *gorm.DB) (bool, error) {
	var n int64
	// The only System probe is this literal metadata name, not business data.
	e := tx.WithContext(tenantctx.System(tx.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_sales_return_order'").Count(&n).Error
	return n > 0, e
}
func requireERPSalesReturnIndependent(tx *gorm.DB, id int64) error {
	exists, e := salesReturnInstalled(tx)
	if e != nil || !exists {
		return e
	}
	var n int64
	e = tx.Model(&model.ERPSalesReturn{}).Where(`stock_document_id=? OR stock_document_id=(SELECT sd.source_document_id FROM erp_document sd WHERE sd.tenant_id=? AND sd.document_id=?)`, id, tenantctx.ID(tx.Statement.Context), id).Count(&n).Error
	if e != nil {
		return e
	}
	if n > 0 {
		return errs.New("该库存凭据由订单退货托管，不支持独立操作")
	}
	return nil
}

// Called with the original SALE lock held. Existing standalone/manual returns
// remain supported until commercial returns exist; cancellation remains safe.
func requireERPSalesSourceIndependent(tx *gorm.DB, sourceID int64) error {
	exists, e := salesReturnInstalled(tx)
	if e != nil || !exists {
		return e
	}
	var n int64
	e = tx.Model(&model.ERPSalesReturn{}).Where(`order_id IN (SELECT so.order_id FROM erp_order so WHERE so.tenant_id=? AND so.stock_document_id=?)`, tenantctx.ID(tx.Statement.Context), sourceID).Count(&n).Error
	if e != nil {
		return e
	}
	if n > 0 {
		return errs.New("原发货已有商业退货，请使用订单退货入库，不可混用库存纠错")
	}
	return nil
}

type salesReturnTotal struct {
	SourceDocumentID int64
	Quantity         int64
}

func legacySalesReturns(tx *gorm.DB, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []salesReturnTotal
	e := tx.Model(&model.ERPDocument{}).Select("source_document_id,COUNT(*) AS quantity").Where("source_document_id IN ? AND kind='SR' AND status='POSTED'", ids).Where(`NOT EXISTS (SELECT 1 FROM erp_sales_return_order sr WHERE sr.tenant_id=? AND sr.stock_document_id=erp_document.document_id)`, tenantctx.ID(tx.Statement.Context)).Group("source_document_id").Scan(&rows).Error
	for _, r := range rows {
		out[r.SourceDocumentID] = r.Quantity > 0
	}
	return out, e
}
func salesReturnableQuery(tx *gorm.DB, in model.ERPSalesReturnQuery) *gorm.DB {
	q := packageScope(tx.Model(&model.ERPPackage{})).Where("erp_package.status='SHIPPED' AND o.status='SHIPPED'")
	if in.OrderID > 0 {
		q = q.Where("erp_package.order_id=?", in.OrderID)
	}
	if in.PackageID > 0 {
		q = q.Where("erp_package.package_id=?", in.PackageID)
	}
	if in.ShopID > 0 {
		q = q.Where("o.shop_id=?", in.ShopID)
	}
	if in.WarehouseID > 0 {
		q = q.Where("erp_package.warehouse_id=?", in.WarehouseID)
	}
	if in.PackageNo != "" {
		q = q.Where("erp_package.package_no=?", in.PackageNo)
	}
	return q
}

const salesReturnableColumns = "erp_package.package_id,erp_package.package_no,erp_package.order_id,erp_package.warehouse_id,erp_package.shipped_at,o.shop_id,o.stock_document_id"

func salesReturnProgress(tx *gorm.DB, rows []model.ERPSalesReturnable) error {
	if len(rows) == 0 {
		return nil
	}
	ids, source := make([]int64, len(rows)), make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.PackageID
		source[i] = r.StockDocumentID
	}
	type total struct {
		PackageID int64
		Quantity  int64
	}
	var gross, used, revision []total
	if e := tx.Model(&model.ERPPackageLine{}).Select("package_id,SUM(quantity) AS quantity").Where("package_id IN ?", ids).Group("package_id").Scan(&gross).Error; e != nil {
		return e
	}
	if e := tx.Model(&model.ERPSalesReturnLine{}).Select("package_id,SUM(quantity) AS quantity").Where("package_id IN ?", ids).Group("package_id").Scan(&used).Error; e != nil {
		return e
	}
	if e := tx.Model(&model.ERPSalesReturn{}).Select("package_id,COUNT(*) AS quantity").Where("package_id IN ?", ids).Group("package_id").Scan(&revision).Error; e != nil {
		return e
	}
	legacy, e := legacySalesReturns(tx, source)
	if e != nil {
		return e
	}
	g, u, v := map[int64]int64{}, map[int64]int64{}, map[int64]int64{}
	for _, x := range gross {
		g[x.PackageID] = x.Quantity
	}
	for _, x := range used {
		u[x.PackageID] = x.Quantity
	}
	for _, x := range revision {
		v[x.PackageID] = x.Quantity
	}
	for i := range rows {
		r := &rows[i]
		if g[r.PackageID] < 1 || u[r.PackageID] < 0 || u[r.PackageID] > g[r.PackageID] || v[r.PackageID] >= math.MaxInt64-1 {
			return errs.New("原发货或累计退货记录异常")
		}
		r.ShippedQuantity = g[r.PackageID]
		r.ReturnedQuantity = u[r.PackageID]
		net := r.ShippedQuantity - r.ReturnedQuantity
		r.NetShippedQuantity = &net
		r.Version = v[r.PackageID] + 1
		r.LegacyStockReturn = legacy[r.StockDocumentID]
		if r.LegacyStockReturn {
			r.NetShippedQuantity = nil
		}
	}
	return nil
}
func SelectERPSalesReturnables(ctx context.Context, in model.ERPSalesReturnQuery, pg page.Query) ([]model.ERPSalesReturnable, int64, error) {
	rows := []model.ERPSalesReturnable{}
	var n int64
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		q := salesReturnableQuery(tx, in)
		if e := q.Count(&n).Error; e != nil {
			return e
		}
		if e := q.Select(salesReturnableColumns).Order("erp_package.package_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error; e != nil {
			return e
		}
		return salesReturnProgress(tx, rows)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return rows, n, e
}
func salesReturnLines(tx *gorm.DB, p model.ERPPackage) ([]model.ERPSalesReturnableLine, error) {
	var lines []model.ERPPackageLine
	if e := tx.Where("package_id=? AND order_id=?", p.PackageID, p.OrderID).Order("line_id").Limit(101).Find(&lines).Error; e != nil {
		return nil, e
	}
	if len(lines) < 1 || len(lines) > 100 {
		return nil, errs.New("原发货明细记录异常")
	}
	var used []struct {
		PackageLineID int64
		Quantity      int64
	}
	if e := tx.Model(&model.ERPSalesReturnLine{}).Select("package_line_id,SUM(quantity) AS quantity").Where("package_id=?", p.PackageID).Group("package_line_id").Scan(&used).Error; e != nil {
		return nil, e
	}
	quantities := map[int64]int64{}
	for _, r := range used {
		quantities[r.PackageLineID] = r.Quantity
	}
	out := make([]model.ERPSalesReturnableLine, 0, len(lines))
	for _, l := range lines {
		u := quantities[l.LineID]
		if l.Quantity < 1 || l.Quantity > 1000000000 || u < 0 || u > l.Quantity {
			return nil, errs.New("原行或累计退货数量异常")
		}
		out = append(out, model.ERPSalesReturnableLine{ERPPackageLine: l, ReturnedQuantity: u, NetShippedQuantity: l.Quantity - u})
		delete(quantities, l.LineID)
	}
	if len(quantities) > 0 {
		return nil, errs.New("退货与原包裹行记录不一致")
	}
	return out, nil
}
func SelectERPSalesReturnable(ctx context.Context, id int64) (*model.ERPSalesReturnable, error) {
	var out model.ERPSalesReturnable
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := salesReturnableQuery(tx, model.ERPSalesReturnQuery{}).Select(salesReturnableColumns).Where("erp_package.package_id=?", id).Take(&out).Error; e != nil {
			return erpReferenceError(e)
		}
		rows := []model.ERPSalesReturnable{out}
		if e := salesReturnProgress(tx, rows); e != nil {
			return e
		}
		out = rows[0]
		if out.LegacyStockReturn {
			return errs.New("原发货已有无法归属到原行的库存退货，请核对历史，不可重复入库")
		}
		var e error
		out.Lines, e = salesReturnLines(tx, model.ERPPackage{PackageID: id, OrderID: out.OrderID})
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &out, e
}
func salesReturnScope(tx *gorm.DB) *gorm.DB {
	return erpShopColumnScope(erpWarehouseScope(tx.Model(&model.ERPSalesReturn{}), "erp_sales_return_order.warehouse_id"), "erp_sales_return_order.shop_id")
}
func SelectERPSalesReturns(ctx context.Context, in model.ERPSalesReturnQuery, pg page.Query) ([]model.ERPSalesReturn, int64, error) {
	q := salesReturnScope(DB(ctx))
	if in.OrderID > 0 {
		q = q.Where("order_id=?", in.OrderID)
	}
	if in.PackageID > 0 {
		q = q.Where("package_id=?", in.PackageID)
	}
	if in.WarehouseID > 0 {
		q = q.Where("warehouse_id=?", in.WarehouseID)
	}
	if in.ShopID > 0 {
		q = q.Where("shop_id=?", in.ShopID)
	}
	if in.ReturnNo != "" {
		q = q.Where("return_no=?", in.ReturnNo)
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.ERPSalesReturn{}
	e := q.Select("return_id,return_no,order_id,shop_id,package_id,warehouse_id,stock_document_id,actor_user_id,from_version,to_version,posted_at").Order("return_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, n, e
}
func SelectERPSalesReturn(ctx context.Context, id int64) (*model.ERPSalesReturn, error) {
	var out model.ERPSalesReturn
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := salesReturnScope(tx).Where("return_id=?", id).Take(&out).Error; e != nil {
			return erpReferenceError(e)
		}
		out.Lines = []model.ERPSalesReturnLine{}
		if e := tx.Where("return_id=? AND package_id=? AND order_id=?", id, out.PackageID, out.OrderID).Order("line_id").Limit(101).Find(&out.Lines).Error; e != nil {
			return e
		}
		if len(out.Lines) < 1 || len(out.Lines) > 100 {
			return errs.New("退货明细异常")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &out, e
}
func replaySalesReturn(tx *gorm.DB, p model.ERPPackage, key, hash string) (*model.ERPSalesReturnResult, error) {
	var h model.ERPSalesReturn
	e := tx.Where("package_id=? AND request_key=?", p.PackageID, key).Take(&h).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if h.RequestHash != hash || h.ActorUserID != erpctx.UserID(tx.Statement.Context) {
		return nil, errs.New("退货请求标识已用于不同内容或操作者")
	}
	var out model.ERPSalesReturnResult
	if e = json.Unmarshal([]byte(h.ResultJSON), &out); e != nil || out.ReturnID != h.ReturnID || out.PackageID != p.PackageID || out.OrderID != p.OrderID || out.StockDocumentID != h.StockDocumentID || out.Version != h.ToVersion {
		return nil, errs.New("退货原结果记录异常")
	}
	out.Existing = true
	return &out, nil
}
func ReturnERPSalesPackage(ctx context.Context, id int64, in model.ERPSalesReturnInput, hash, operator string) (*model.ERPSalesReturnResult, error) {
	var out *model.ERPSalesReturnResult
	e := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := lockERPActor(tx); e != nil {
			return e
		}
		var hint model.ERPPackage
		if e := packageScope(tx.Model(&hint)).Select("erp_package.order_id").Where("erp_package.package_id=?", id).Take(&hint).Error; e != nil {
			return erpReferenceError(e)
		}
		o, e := lockFulfillmentOrder(tx, hint.OrderID, false)
		if e != nil {
			return e
		}
		p, e := lockOrderPackage(tx, o)
		if e != nil {
			return e
		}
		if p.PackageID != id {
			return errs.New("包裹不匹配")
		}
		if e = requireERPWarehouse(tx, p.WarehouseID); e != nil {
			return e
		}
		if out, e = replaySalesReturn(tx, p, in.RequestKey, hash); e != nil || out != nil {
			return e
		}
		if o.Status != "SHIPPED" || p.Status != "SHIPPED" || o.StockDocumentID == nil {
			return errs.New("只有已发货订单包裹可退货")
		}
		var source model.ERPDocument
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id=?", *o.StockDocumentID).Take(&source).Error; e != nil {
			return erpReferenceError(e)
		}
		if source.Kind != "SALE" || source.Status != "SHIPPED" || source.WarehouseID != p.WarehouseID {
			return errs.New("原销售库存凭据异常")
		}
		legacy, e := legacySalesReturns(tx, []int64{source.DocumentID})
		if e != nil {
			return e
		}
		if legacy[source.DocumentID] {
			return errs.New("原发货已有无法归属到原行的库存退货，请核对历史，不可重复入库")
		}
		var revisions int64
		if e = tx.Model(&model.ERPSalesReturn{}).Where("package_id=?", id).Count(&revisions).Error; e != nil {
			return e
		}
		if revisions >= math.MaxInt64-1 || in.Version != revisions+1 {
			return errs.New("原包裹退货版本已变化，请刷新后重新核对")
		}
		lines, e := salesReturnLines(tx, p)
		if e != nil {
			return e
		}
		byID := map[int64]model.ERPSalesReturnableLine{}
		for _, l := range lines {
			byID[l.LineID] = l
		}
		if len(in.Lines) < 1 || len(in.Lines) > 100 {
			return errs.New("本次须有1—100行正数退货")
		}
		selected := make([]model.ERPSalesReturnableLine, 0, len(in.Lines))
		quantities, skuTotals := map[int64]int64{}, map[int64]int64{}
		for _, input := range in.Lines {
			l, ok := byID[input.PackageLineID]
			if !ok || input.Quantity < 1 || input.Quantity > 1000000000 || input.Quantity > l.NetShippedQuantity || quantities[input.PackageLineID] != 0 || skuTotals[l.SkuID] > 1000000000-input.Quantity {
				return errs.New("退货行不属于原包裹、重复、超过原行可退量或SKU汇总上限")
			}
			quantities[l.LineID] = input.Quantity
			skuTotals[l.SkuID] += input.Quantity
			selected = append(selected, l)
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].LineID < selected[j].LineID })
		ids := make([]int64, 0, len(skuTotals))
		for sku := range skuTotals {
			ids = append(ids, sku)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		warehouse, skus, e := lockERPReferences(tx, p.WarehouseID, ids, false)
		if e != nil {
			return e
		}
		record := model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}
		doc := model.ERPDocument{ERPRecord: record, DocumentNo: "SR6" + uuid.NewString(), Kind: "SR", WarehouseID: p.WarehouseID, WarehouseName: warehouse.WarehouseName, SourceDocumentID: &source.DocumentID, Status: "DRAFT", Version: 1, RequestKey: "S6_" + uuid.NewString(), RequestHash: hash}
		if e = tx.Omit("Lines").Create(&doc).Error; e != nil {
			return e
		}
		inputs := make([]model.ERPLineInput, len(ids))
		for i, sku := range ids {
			inputs[i] = model.ERPLineInput{SkuID: sku, Quantity: skuTotals[sku]}
		}
		stockLines, e := buildERPLines(tx, &doc, inputs, skus)
		if e != nil {
			return e
		}
		if e = tx.Create(&stockLines).Error; e != nil {
			return e
		}
		h := model.ERPSalesReturn{ERPRecord: record, ReturnNo: "SRT" + uuid.NewString(), OrderID: o.OrderID, ShopID: o.ShopID, PackageID: id, WarehouseID: p.WarehouseID, StockDocumentID: doc.DocumentID, ActorUserID: erpctx.UserID(ctx), RequestKey: in.RequestKey, RequestHash: hash, FromVersion: in.Version, ToVersion: in.Version + 1, Reason: in.Reason, PostedAt: types.Now()}
		if e = tx.Omit("Lines").Create(&h).Error; e != nil {
			return e
		}
		details := make([]model.ERPSalesReturnLine, 0, len(selected))
		for _, l := range selected {
			details = append(details, model.ERPSalesReturnLine{ERPRecord: record, ReturnID: h.ReturnID, PackageID: id, PackageLineID: l.LineID, OrderID: o.OrderID, OrderLineID: l.OrderLineID, SkuID: l.SkuID, Quantity: quantities[l.LineID], SkuCode: l.SkuCode, ProductName: l.ProductName, Unit: l.Unit})
		}
		if e = tx.Create(&details).Error; e != nil {
			return e
		}
		if e = transitionERPDocumentWithReturnLocked(tx, &doc, "confirm", operator, &salesReturnSource{returnID: h.ReturnID, packageID: id}); e != nil {
			return e
		}
		if doc.Status != "POSTED" {
			return errs.New("订单退货库存过账未完成")
		}
		out = &model.ERPSalesReturnResult{ReturnID: h.ReturnID, PackageID: id, OrderID: o.OrderID, StockDocumentID: doc.DocumentID, Version: h.ToVersion}
		raw, e := json.Marshal(out)
		if e != nil {
			return e
		}
		r := tx.Model(&model.ERPSalesReturn{}).Where("return_id=? AND result_json=''", h.ReturnID).Update("result_json", string(raw))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errs.New("退货原结果未登记")
		}
		audit, _ := json.Marshal(map[string]any{"returnId": h.ReturnID, "packageId": id, "orderId": o.OrderID, "stockDocumentId": doc.DocumentID, "fromVersion": h.FromVersion, "version": h.ToVersion})
		return tx.Create(&model.SysOperLog{Title: "ERP订单退货入库", BusinessType: model.BusinessTypeInsert, Method: "erp.order.RETURN", RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
	}))
	if e != nil {
		return nil, e
	}
	return out, nil
}
