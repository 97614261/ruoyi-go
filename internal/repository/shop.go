package repository

import (
	"context"
	"encoding/json"
	"errors"
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

// Live scope: never use a cached/JWT/browser list, including for old sessions.
func erpShopScope(q *gorm.DB) *gorm.DB {
	return erpShopColumnScope(q, "erp_shop.shop_id")
}

// column is a reviewed server constant, never a request parameter.
func erpShopColumnScope(q *gorm.DB, column string) *gorm.DB {
	ctx := q.Statement.Context
	uid, tid := erpctx.UserID(ctx), tenantctx.ID(ctx)
	if uid < 1 || tid < 1 || tenantctx.Get(ctx).Platform {
		q.AddError(errs.New("缺少企业ERP身份"))
		return q
	}
	return q.Where("(EXISTS (SELECT 1 FROM sys_tenant et WHERE et.tenant_id=? AND et.owner_user_id=?) OR "+column+" IN (SELECT shop_id FROM erp_shop_access WHERE tenant_id=? AND user_id=?))", tid, uid, tid, uid)
}
func erpShopFilter(db *gorm.DB, query model.ErpShopQuery) *gorm.DB {
	if query.ShopCode != "" {
		db = db.Where("shop_code=?", query.ShopCode)
	}
	if query.ShopName != "" {
		db = db.Where("shop_name LIKE ?", "%"+query.ShopName+"%")
	}
	if query.ChannelCode != "" {
		db = db.Where("channel_code=?", query.ChannelCode)
	}
	if query.Status != "" {
		db = db.Where("status=?", query.Status)
	}
	return db
}
func SelectErpShopPage(ctx context.Context, query model.ErpShopQuery, pg page.Query) ([]model.ErpShop, int64, error) {
	q := erpShopFilter(erpShopScope(TenantTable(DB(ctx), "erp_shop")), query)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.ErpShop{}
	if total == 0 {
		return rows, 0, nil
	}
	err := q.Order(pg.Stable("shop_id", "shop_id")).Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func SelectErpShopByID(ctx context.Context, id int64) (*model.ErpShop, error) {
	var target model.ErpShop
	err := erpShopScope(DB(ctx).Model(&model.ErpShop{})).Where("shop_id=?", id).Take(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &target, err
}
func requireERPShopOwner(tx *gorm.DB) error {
	if err := lockERPActor(tx); err != nil {
		return err
	}
	owner, err := erpOwner(tx)
	if err != nil {
		return err
	}
	if !owner {
		return errs.New("仅企业所有者可维护店铺和管理店铺授权")
	}
	return nil
}
func InsertErpShop(ctx context.Context, target *model.ErpShop) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := requireERPShopOwner(tx); err != nil {
			return err
		}
		return tx.Create(target).Error
	}))
}
func UpdateErpShop(ctx context.Context, target *model.ErpShop) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := requireERPShopOwner(tx); err != nil {
			return err
		}
		var old model.ErpShop
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shop_id=?", target.ShopId).Take(&old).Error; err != nil {
			return erpReferenceError(err)
		}
		if old.Version != target.Version {
			return errs.New("店铺已变更，请刷新后重试")
		}
		if old.Version == math.MaxInt64 {
			return errs.New("店铺版本已达上限")
		}
		if old.ShopCode != target.ShopCode || old.ChannelCode != target.ChannelCode || old.SiteCode != target.SiteCode || old.Currency != target.Currency {
			// Bounded reference checks, each uses a tenant/shop leading index.
			for _, table := range []string{"erp_shop_item", "erp_order", "erp_import_job"} {
				var found []int
				if err := TenantTable(tx, table).Select("1").Where("shop_id=?", old.ShopId).Limit(1).Scan(&found).Error; err != nil {
					return err
				}
				if len(found) > 0 {
					return errs.New("店铺已被商品、订单或导入任务引用，编码/渠道/站点/币种不能修改")
				}
			}
		}
		updates := map[string]any{"shop_code": target.ShopCode, "shop_name": target.ShopName, "channel_code": target.ChannelCode, "site_code": target.SiteCode, "currency": target.Currency, "status": target.Status, "version": old.Version + 1, "remark": target.Remark, "update_by": target.UpdateBy, "update_time": target.UpdateTime}
		result := tx.Model(&model.ErpShop{}).Where("shop_id=? AND version=?", old.ShopId, old.Version).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errs.New("店铺已变更，请刷新后重试")
		}
		return nil
	}))
}
func GetERPShopAccess(ctx context.Context, userID int64) (*model.ERPShopAccessView, error) {
	var result *model.ERPShopAccessView
	err := erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := requireERPShopOwner(tx); err != nil {
			return err
		}
		var user model.SysUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND del_flag='0'", userID).Take(&user).Error; err != nil {
			return erpReferenceError(err)
		}
		if userID == erpctx.UserID(ctx) {
			return errs.New("所有者拥有全部店铺，无需给自己授权")
		}
		var p model.ERPShopPolicy
		err := tx.Where("user_id=?", userID).Take(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p.Version = 1
		} else if err != nil {
			return err
		}
		ids := []int64{}
		if err = tx.Model(&model.ERPShopAccess{}).Where("user_id=?", userID).Order("shop_id").Pluck("shop_id", &ids).Error; err != nil {
			return err
		}
		result = &model.ERPShopAccessView{UserID: userID, Version: p.Version, ShopIDs: ids}
		return nil
	})
	return result, err
}
func ReplaceERPShopAccess(ctx context.Context, userID, version int64, ids []int64, operator string) error {
	return erpError(erpTransaction(ctx, func(tx *gorm.DB) error {
		if err := requireERPShopOwner(tx); err != nil {
			return err
		}
		var user model.SysUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND del_flag='0'", userID).Take(&user).Error; err != nil {
			return erpReferenceError(err)
		}
		if userID == erpctx.UserID(ctx) {
			return errs.New("所有者拥有全部店铺，无需给自己授权")
		}
		var p model.ERPShopPolicy
		err := tx.Where("user_id=?", userID).Take(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p = model.ERPShopPolicy{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, UserID: userID, Version: 1}
			if err = tx.Create(&p).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if version != p.Version {
			return errs.New("店铺授权已变更，请刷新后重试")
		}
		if p.Version == math.MaxInt64 {
			return errs.New("店铺授权版本已达上限")
		}
		if len(ids) > 0 {
			var rows []model.ErpShop
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shop_id IN ?", ids).Order("shop_id").Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) != len(ids) {
				return errs.New("店铺不存在或不属于当前企业")
			}
		}
		if err = tx.Where("user_id=?", userID).Delete(&model.ERPShopAccess{}).Error; err != nil {
			return err
		}
		grants := make([]model.ERPShopAccess, 0, len(ids))
		for _, id := range ids {
			grants = append(grants, model.ERPShopAccess{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, UserID: userID, ShopID: id})
		}
		if len(grants) > 0 {
			if err = tx.Create(&grants).Error; err != nil {
				return err
			}
		}
		if err = tx.Model(&model.ERPShopPolicy{}).Where("policy_id=?", p.PolicyID).Updates(map[string]any{"version": p.Version + 1, "update_by": operator, "update_time": types.Now()}).Error; err != nil {
			return err
		}
		// Keep up to 200 int64 IDs completely, splitting bounded audit rows rather
		// than truncating sys_oper_log.oper_param(2000). All parts are transactional.
		chunks := 1
		if len(ids) > 0 {
			chunks = (len(ids) + 59) / 60
		}
		logs := make([]model.SysOperLog, 0, chunks)
		for part := 0; part < chunks; part++ {
			start, end := part*60, (part+1)*60
			if end > len(ids) {
				end = len(ids)
			}
			encoded, err := json.Marshal(map[string]any{"userId": userID, "shopIds": ids[start:end], "previousVersion": p.Version, "version": p.Version + 1, "part": part + 1, "parts": chunks})
			if err != nil {
				return err
			}
			if len(encoded) > 1900 {
				return errs.New("授权审计信息过长")
			}
			logs = append(logs, model.SysOperLog{Title: "ERP店铺授权", BusinessType: model.BusinessTypeGrant, Method: "erp.shop.access", RequestMethod: "PUT", OperName: operator, OperParam: string(encoded), JSONResult: `{"code":200}`, OperTime: types.Now()})
		}
		return tx.Create(&logs).Error
	}))
}
