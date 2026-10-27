package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func orderScope(tx *gorm.DB) *gorm.DB { return erpShopColumnScope(tx, "erp_order.shop_id") }

const orderSummaryColumns = "order_id,shop_id,order_no,source_type,source_order_no,status,version,currency,original_amount,ordered_at"

func orderListQuery(tx *gorm.DB, q model.ERPOrderQuery) *gorm.DB {
	tx = orderScope(tx.Model(&model.ERPOrder{}))
	if q.ShopID > 0 {
		tx = tx.Where("shop_id=?", q.ShopID)
	}
	if q.Status != "" {
		tx = tx.Where("status=?", q.Status)
	}
	if q.SourceOrderNo != "" {
		tx = tx.Where("source_order_no=?", q.SourceOrderNo)
	}
	return tx
}
func SelectERPOrderPage(ctx context.Context, q model.ERPOrderQuery, pg page.Query) ([]model.ERPOrderSummary, int64, error) {
	tx := orderListQuery(DB(ctx), q)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ERPOrderSummary{}
	err := orderPageQuery(DB(ctx), q, pg).Scan(&rows).Error
	return rows, total, err
}
func SelectERPOrderExport(ctx context.Context, q model.ERPOrderQuery, limit int) ([]model.ERPOrderSummary, error) {
	rows := []model.ERPOrderSummary{}
	err := orderListQuery(DB(ctx), q).Select(orderSummaryColumns).Order("order_id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}
func SelectERPOrder(ctx context.Context, id int64) (*model.ERPOrder, error) {
	var o model.ERPOrder
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := orderScope(tx.Model(&o)).Where("order_id=?", id).Take(&o).Error; err != nil {
			return erpReferenceError(err)
		}
		o.Lines = []model.ERPOrderLine{}
		if err := tx.Where("order_id=? AND shop_id=?", id, o.ShopID).Order("line_id").Limit(101).Find(&o.Lines).Error; err != nil {
			return err
		}
		var p model.ERPPackage
		err := tx.Select("package_id,status,version").Where("order_id=?", id).Take(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		o.Package = &model.ERPOrderPackageLink{PackageID: p.PackageID, Status: p.Status, Version: p.Version}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &o, err
}
func orderAudit(tx *gorm.DB, o model.ERPOrder, action, key, hash, operator string, from int64) error {
	e := model.ERPOrderEvent{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, OrderID: o.OrderID, ActorUserID: erpctx.UserID(tx.Statement.Context), Action: action, RequestKey: key, RequestHash: hash, FromStatus: o.Status, ToStatus: o.Status, FromVersion: from, ToVersion: o.Version}
	if from == 0 {
		e.FromStatus = ""
	}
	if err := tx.Create(&e).Error; err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]any{"orderId": o.OrderID, "shopId": o.ShopID, "action": action, "fromVersion": from, "version": o.Version})
	return tx.Create(&model.SysOperLog{Title: "ERP订单", BusinessType: model.BusinessTypeUpdate, Method: "erp.order." + action, RequestMethod: "POST", OperName: operator, OperParam: string(b), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}

// Each branch uses one existing unique key. UNION (not UNION ALL) preserves the
// original OR's row set when both keys identify the same record. Keeping this
// as one SELECT also preserves a single read view; do not split it into two
// independent reads or remove current shop scope for idempotency replays.
// The derived table is a reviewed server constant: both base reads explicitly
// constrain tenant_id, and the normal outer tenant and live-shop guards remain.
const orderCreateCandidateTable = `(SELECT * FROM erp_order WHERE tenant_id=? AND create_request_key=?
UNION SELECT * FROM erp_order WHERE tenant_id=? AND shop_id=? AND source_order_no=?) AS erp_order`

func orderCreateCandidates(tx *gorm.DB, in model.ERPOrderWrite) *gorm.DB {
	tid := tenantctx.ID(tx.Statement.Context)
	return orderScope(tx.Model(&model.ERPOrder{}).Table(orderCreateCandidateTable,
		tid, in.RequestKey, tid, in.ShopID, in.SourceOrderNo))
}
func createOrderLocked(tx *gorm.DB, in model.ERPOrderWrite, source, hash, operator string) (*model.ERPOrderResult, error) {
	// The current actor and parent shop must already be locked. Idempotency never
	// bypasses current scope. The fingerprint deliberately survives later edits.
	var olds []model.ERPOrder
	if err := orderCreateCandidates(tx, in).Find(&olds).Error; err != nil {
		return nil, err
	}
	if len(olds) > 0 {
		if len(olds) != 1 || olds[0].ShopID != in.ShopID || olds[0].SourceOrderNo != in.SourceOrderNo || olds[0].CreateRequestHash != hash {
			return nil, errs.New("订单号或请求标识已存在且内容不同，未覆盖原订单")
		}
		return &model.ERPOrderResult{OrderID: olds[0].OrderID, Version: olds[0].Version, Existing: true}, nil
	}
	o := model.ERPOrder{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ERPOrderFields: in.ERPOrderFields, ShopID: in.ShopID, OrderNo: "O" + uuid.NewString(), SourceType: source, SourceOrderNo: in.SourceOrderNo, CreateRequestKey: in.RequestKey, CreateRequestHash: hash, Status: "PENDING_REVIEW", Version: 1}
	if err := tx.Omit("Lines", "Package").Create(&o).Error; err != nil {
		return nil, err
	}
	if err := replacePendingOrderLines(tx, o, in.Lines, operator); err != nil {
		return nil, err
	}
	if err := orderAudit(tx, o, "CREATE", in.RequestKey, hash, operator, 0); err != nil {
		return nil, err
	}
	return &model.ERPOrderResult{OrderID: o.OrderID, Version: 1}, nil
}
func replacePendingOrderLines(tx *gorm.DB, o model.ERPOrder, inputs []model.ERPOrderLineInput, operator string) error {
	codes := make([]string, 0, len(inputs))
	for _, l := range inputs {
		codes = append(codes, l.ShopItemCode)
	}
	var items []model.ErpShopItem
	if err := tx.Select("item_id,shop_item_code").Where("shop_id=? AND shop_item_code IN ?", o.ShopID, codes).Find(&items).Error; err != nil {
		return err
	}
	byCode := map[string]int64{}
	for _, i := range items {
		byCode[i.ShopItemCode] = i.ItemID
	}
	lines := make([]model.ERPOrderLine, 0, len(inputs))
	for _, l := range inputs {
		row := model.ERPOrderLine{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ShopID: o.ShopID, OrderID: o.OrderID, SourceLineNo: l.SourceLineNo, ShopItemCode: l.ShopItemCode, SourceTitle: l.SourceTitle, Quantity: l.Quantity}
		if id, ok := byCode[l.ShopItemCode]; ok {
			row.ShopItemID = &id
		}
		lines = append(lines, row)
	}
	return tx.Create(&lines).Error
}
func CreateERPOrder(ctx context.Context, in model.ERPOrderWrite, source, hash, operator string) (*model.ERPOrderResult, error) {
	var result *model.ERPOrderResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		if _, err := lockShopForItem(tx, in.ShopID, true); err != nil {
			return err
		}
		var err error
		result, err = createOrderLocked(tx, in, source, hash, operator)
		return err
	}))
	return result, err
}
func EditERPOrder(ctx context.Context, id int64, in model.ERPOrderWrite, hash, operator string) (*model.ERPOrderResult, error) {
	var result *model.ERPOrderResult
	err := erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := lockERPActor(tx); err != nil {
			return err
		}
		if _, err := lockShopForItem(tx, in.ShopID, true); err != nil {
			return err
		}
		var old model.ERPOrder
		if err := orderScope(tx.Model(&old)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id=? AND shop_id=?", id, in.ShopID).Take(&old).Error; err != nil {
			return erpReferenceError(err)
		}
		var event model.ERPOrderEvent
		err := tx.Where("order_id=? AND action='EDIT' AND request_key=?", id, in.RequestKey).Take(&event).Error
		if err == nil {
			if event.RequestHash != hash {
				return errs.New("请求标识已用于不同编辑")
			}
			result = &model.ERPOrderResult{OrderID: id, Version: event.ToVersion, Existing: true}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.Status != "PENDING_REVIEW" || old.Version != in.Version || old.Version == math.MaxInt64 {
			return errs.New("仅可编辑当前版本的待审核订单，请刷新")
		}
		if old.SourceOrderNo != in.SourceOrderNo {
			return errs.New("来源订单号不可修改")
		}
		// Save only an explicit whitelist, not client-supplied state/audit/stock IDs.
		u := map[string]any{"receiver_name": in.ReceiverName, "receiver_phone": in.ReceiverPhone, "receiver_country": in.ReceiverCountry, "receiver_region": in.ReceiverRegion, "receiver_city": in.ReceiverCity, "receiver_postcode": in.ReceiverPostcode, "receiver_address": in.ReceiverAddress, "currency": in.Currency, "original_amount": in.OriginalAmount, "ordered_at": in.OrderedAt, "version": old.Version + 1, "update_by": operator, "update_time": types.Now()}
		if err = tx.Model(&model.ERPOrder{}).Where("order_id=? AND version=?", id, old.Version).Updates(u).Error; err != nil {
			return err
		}
		if err = tx.Where("order_id=?", id).Delete(&model.ERPOrderLine{}).Error; err != nil {
			return err
		}
		if err = replacePendingOrderLines(tx, old, in.Lines, operator); err != nil {
			return err
		}
		from := old.Version
		old.Version++
		if err = orderAudit(tx, old, "EDIT", in.RequestKey, hash, operator, from); err != nil {
			return err
		}
		result = &model.ERPOrderResult{OrderID: id, Version: old.Version}
		return nil
	}))
	return result, err
}
