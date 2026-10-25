// Product aggregate enhanced from the actual010 generator baseline; raw ZIPs
// are retained with local evidence. Internal child tables have no public CRUD.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"sort"
	"strings"
)

func requireCatalog(q *gorm.DB) error {
	ctx := q.Statement.Context
	if erpctx.UserID(ctx) < 1 || tenantctx.ID(ctx) < 1 || tenantctx.Get(ctx).Platform {
		return errs.New("缺少企业商品身份")
	}
	var n int64
	if e := q.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_product'").Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errs.New("请先备份并显式安装商品基础010")
	}
	return nil
}
func catalogReplay(tx *gorm.DB, key, hash, kind string) (*model.CatalogResult, error) {
	var a model.CatalogAction
	e := tx.Where("request_key=?", key).Take(&a).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if a.ActorID != erpctx.UserID(tx.Statement.Context) || a.RequestHash != hash || a.ActionKind != kind {
		return nil, errs.New("同一请求标识不能用于不同内容、动作或员工")
	}
	var r model.CatalogResult
	if json.Unmarshal([]byte(a.ResultJSON), &r) != nil || r.Rows < 1 || r.Version < 1 {
		return nil, errs.New("商品动作结果记录异常")
	}
	r.Existing = true
	return &r, nil
}
func catalogRecord(tx *gorm.DB, key, hash, kind, operator string, out *model.CatalogResult) error {
	b, e := json.Marshal(out)
	if e != nil {
		return e
	}
	a := model.CatalogAction{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, RequestKey: key, RequestHash: hash, ActionKind: kind, ActorID: erpctx.UserID(tx.Statement.Context), ResultJSON: string(b)}
	if e = tx.Create(&a).Error; e != nil {
		return e
	}
	audit, _ := json.Marshal(map[string]any{"kind": kind, "rows": out.Rows, "productIds": out.ProductIDs, "categoryId": out.CategoryID, "version": out.Version})
	return tx.Create(&model.SysOperLog{Title: "ERP商品档案", BusinessType: model.BusinessTypeUpdate, Method: "erp.catalog." + kind, RequestMethod: "POST", OperName: operator, OperParam: string(audit), JSONResult: `{"code":200}`, OperTime: types.Now()}).Error
}
func catalogVersion(v int64) error {
	if v < 1 || v == math.MaxInt64 {
		return errs.New("商品版本异常，请刷新核对")
	}
	return nil
}
func WriteCatalogCategory(ctx context.Context, id int64, in model.CatalogCategoryWrite, hash, operator string) (*model.CatalogResult, error) {
	var out *model.CatalogResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireCatalog(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		r, e := catalogReplay(tx, in.RequestKey, hash, "CATEGORY")
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		a := model.CatalogCategory{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, CategoryCode: in.CategoryCode, CategoryName: in.CategoryName, Status: in.Status, Version: 1}
		if id == 0 {
			e = tx.Create(&a).Error
		} else {
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("category_id=?", id).Take(&a).Error; e != nil {
				return erpReferenceError(e)
			}
			if e = catalogVersion(a.Version); e != nil {
				return e
			}
			if in.Version != a.Version {
				return errs.New("分类已变更，请刷新")
			}
			if a.CategoryCode != in.CategoryCode {
				return errs.New("分类编码建立后不可修改")
			}
			a.Version++
			e = tx.Model(&a).Updates(map[string]any{"category_name": in.CategoryName, "status": in.Status, "version": a.Version, "remark": in.Remark, "update_by": operator, "update_time": types.Now()}).Error
		}
		if e != nil {
			return e
		}
		out = &model.CatalogResult{CategoryID: a.CategoryID, Version: a.Version, Rows: 1}
		return catalogRecord(tx, in.RequestKey, hash, "CATEGORY", operator, out)
	})
	return out, erpError(e)
}

// One grouped lookup, including optional modules only if installed; no per-SKU
// read loops. All nested predicates carry tenant_id explicitly.
func catalogSKUReferences(tx *gorm.DB, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	specs := map[string]string{"erp_document_line": "sku_id", "erp_stock": "sku_id", "erp_shop_sku_mapping": "sku_id", "erp_order_line": "sku_id", "erp_purchase_order_line": "sku_id", "erp_transfer_line": "sku_id"}
	names := []string{}
	for n := range specs {
		names = append(names, n)
	}
	sort.Strings(names)
	var present []string
	ctx := tx.Statement.Context
	if e := tx.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name IN ?", names).Pluck("table_name", &present).Error; e != nil {
		return nil, e
	}
	predicates := []string{}
	args := []any{}
	for _, n := range present {
		predicates = append(predicates, "EXISTS (SELECT 1 FROM "+n+" cr WHERE cr.tenant_id=? AND cr."+specs[n]+"=erp_sku.sku_id)")
		args = append(args, tenantctx.ID(ctx))
	}
	if len(predicates) == 0 {
		return out, nil
	}
	var used []int64
	if e := tx.Model(&model.ErpSku{}).Where("sku_id IN ?", ids).Where("("+strings.Join(predicates, " OR ")+")", args...).Pluck("sku_id", &used).Error; e != nil {
		return nil, e
	}
	for _, id := range used {
		out[id] = true
	}
	return out, nil
}
func requireCatalogCategory(tx *gorm.DB, id, previous *int64) error {
	if id == nil {
		return nil
	}
	var c model.CatalogCategory
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("category_id=?", *id).Take(&c).Error; e != nil {
		return erpReferenceError(e)
	}
	if c.Status != "0" && (previous == nil || *previous != *id) {
		return errs.New("分类已停用，不能用于新商品")
	}
	return nil
}
func requireCatalogImages(tx *gorm.DB, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	var fs []model.SysFile
	if e := tx.Where("storage_key IN ?", keys).Find(&fs).Error; e != nil {
		return e
	}
	if len(fs) != len(keys) {
		return errs.New("图片不存在或不属于当前企业")
	}
	for _, f := range fs {
		if !strings.HasPrefix(f.StorageKey, "/profile/tenants/"+fmt.Sprint(tenantctx.ID(tx.Statement.Context))+"/catalog/") {
			return errs.New("请从商品档案上传图片")
		}
		switch f.ContentType {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return errs.New("商品图片仅支持PNG、JPEG、GIF和WebP")
		}
	}
	return nil
}
func WriteCatalogProduct(ctx context.Context, id int64, in model.CatalogWrite, hash, operator string) (*model.CatalogResult, error) {
	var out *model.CatalogResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireCatalog(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		r, e := catalogReplay(tx, in.RequestKey, hash, "PRODUCT")
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		p := model.CatalogProduct{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}, ProductCode: in.ProductCode, ProductName: in.ProductName, CategoryID: in.CategoryID, Version: 1}
		existing := []model.CatalogProfile{}
		if id > 0 {
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id=?", id).Take(&p).Error; e != nil {
				return erpReferenceError(e)
			}
			if e = catalogVersion(p.Version); e != nil {
				return e
			}
			if in.Version != p.Version {
				return errs.New("商品已变更，请刷新后重试")
			}
			if p.ProductCode != in.ProductCode {
				return errs.New("商品编码建立后不可修改")
			}
			if e = tx.Where("product_id=?", id).Find(&existing).Error; e != nil {
				return e
			}
		}
		var previousCategory *int64
		if id > 0 {
			previousCategory = p.CategoryID
		}
		if e = requireCatalogCategory(tx, in.CategoryID, previousCategory); e != nil {
			return e
		}
		if e = requireCatalogImages(tx, in.Images); e != nil {
			return e
		}
		ids := []int64{}
		wanted := map[int64]bool{}
		for _, v := range in.Variants {
			if v.SkuID > 0 {
				ids = append(ids, v.SkuID)
				wanted[v.SkuID] = true
			}
		}
		for _, v := range existing {
			if !wanted[v.SkuID] {
				return errs.New("已有SKU不能从商品移除，请停用保留")
			}
		}
		old := []model.ErpSku{}
		profiles := []model.CatalogProfile{}
		if len(ids) > 0 {
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sku_id IN ?", ids).Order("sku_id").Find(&old).Error; e != nil {
				return e
			}
			if len(old) != len(ids) {
				return errs.New("SKU不存在或不属于当前企业")
			}
			if e = tx.Where("sku_id IN ?", ids).Find(&profiles).Error; e != nil {
				return e
			}
		}
		byID := map[int64]model.ErpSku{}
		for _, v := range old {
			byID[v.SkuId] = v
		}
		profileByID := map[int64]model.CatalogProfile{}
		for _, v := range profiles {
			if id == 0 || v.ProductID != id {
				return errs.New("SKU已属于其他商品，不能重复归组")
			}
			profileByID[v.SkuID] = v
		}
		refs, e := catalogSKUReferences(tx, ids)
		if e != nil {
			return e
		}
		for _, v := range in.Variants {
			if o, ok := byID[v.SkuID]; ok && refs[v.SkuID] && (o.SkuCode != v.SkuCode || o.Unit != v.Unit || o.Specification != v.Specification) {
				return errs.New("SKU已被业务引用，不能修改编码、单位或规格")
			}
		}
		if id == 0 {
			if e = tx.Omit("Variants", "Images", "CategoryName").Create(&p).Error; e != nil {
				return e
			}
		} else {
			p.Version++
			if e = tx.Model(&model.CatalogProduct{}).Where("product_id=?", id).Updates(map[string]any{"product_name": in.ProductName, "category_id": in.CategoryID, "remark": in.Remark, "version": p.Version, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
				return e
			}
		}
		additions := []model.ErpSku{}
		positions := []int{}
		for i, v := range in.Variants {
			if v.SkuID == 0 {
				positions = append(positions, i)
				additions = append(additions, model.ErpSku{SkuCode: v.SkuCode, ProductName: in.ProductName, Specification: v.Specification, Unit: v.Unit, Status: v.Status, CreateBy: operator, CreateTime: types.Now()})
			} else {
				if e = tx.Model(&model.ErpSku{}).Where("sku_id=?", v.SkuID).Updates(map[string]any{"sku_code": v.SkuCode, "product_name": in.ProductName, "specification": v.Specification, "unit": v.Unit, "status": v.Status, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
					return e
				}
			}
		}
		if len(additions) > 0 {
			if e = tx.Create(&additions).Error; e != nil {
				return e
			}
			for i, v := range additions {
				in.Variants[positions[i]].SkuID = v.SkuId
			}
		}
		newProfiles := []model.CatalogProfile{}
		for _, v := range in.Variants {
			if _, ok := profileByID[v.SkuID]; ok {
				if e = tx.Model(&model.CatalogProfile{}).Where("sku_id=?", v.SkuID).Updates(map[string]any{"barcode": v.Barcode, "weight_grams": v.WeightGrams, "length_mm": v.LengthMM, "width_mm": v.WidthMM, "height_mm": v.HeightMM, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
					return e
				}
			} else {
				newProfiles = append(newProfiles, model.CatalogProfile{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ProductID: p.ProductID, SkuID: v.SkuID, Barcode: v.Barcode, WeightGrams: v.WeightGrams, LengthMM: v.LengthMM, WidthMM: v.WidthMM, HeightMM: v.HeightMM})
			}
		}
		if len(newProfiles) > 0 {
			if e = tx.Create(&newProfiles).Error; e != nil {
				return e
			}
		}
		if e = tx.Where("product_id=?", p.ProductID).Delete(&model.CatalogImage{}).Error; e != nil {
			return e
		}
		imgs := []model.CatalogImage{}
		for i, key := range in.Images {
			imgs = append(imgs, model.CatalogImage{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ProductID: p.ProductID, StorageKey: key, Position: i})
		}
		if len(imgs) > 0 {
			if e = tx.Create(&imgs).Error; e != nil {
				return e
			}
		}
		out = &model.CatalogResult{ProductIDs: []int64{p.ProductID}, Version: p.Version, Rows: len(in.Variants)}
		return catalogRecord(tx, in.RequestKey, hash, "PRODUCT", operator, out)
	})
	return out, erpError(e)
}
func catalogFilter(q *gorm.DB, in model.CatalogQuery) *gorm.DB {
	if in.ProductCode != "" {
		q = q.Where("p.product_code=?", in.ProductCode)
	}
	if in.ProductName != "" {
		q = q.Where("p.product_name LIKE ?", "%"+in.ProductName+"%")
	}
	if in.CategoryID > 0 {
		q = q.Where("p.category_id=?", in.CategoryID)
	}
	tid := tenantctx.ID(q.Statement.Context)
	if in.SkuCode != "" {
		q = q.Where("EXISTS (SELECT 1 FROM erp_sku_profile cp JOIN erp_sku cs ON cs.tenant_id=cp.tenant_id AND cs.sku_id=cp.sku_id WHERE cp.tenant_id=? AND cp.product_id=p.product_id AND cs.sku_code=?)", tid, in.SkuCode)
	}
	if in.Barcode != "" {
		q = q.Where("EXISTS (SELECT 1 FROM erp_sku_profile cp WHERE cp.tenant_id=? AND cp.product_id=p.product_id AND cp.barcode=?)", tid, in.Barcode)
	}
	return q
}
func SelectCatalogProducts(ctx context.Context, in model.CatalogQuery, pg page.Query) ([]model.CatalogProduct, int64, error) {
	q := DB(ctx)
	if e := requireCatalog(q); e != nil {
		return nil, 0, e
	}
	q = catalogFilter(q.Table("erp_product p"), in)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CatalogProduct{}
	if e := q.Select("p.*").Order("p.product_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error; e != nil {
		return nil, 0, e
	}
	if e := catalogDecorate(DB(ctx), rows, false); e != nil {
		return nil, 0, e
	}
	return rows, n, nil
}
func SelectCatalogProduct(ctx context.Context, id int64) (*model.CatalogProduct, error) {
	var p model.CatalogProduct
	e := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := requireCatalog(tx); e != nil {
			return e
		}
		if e := tx.Where("product_id=?", id).Take(&p).Error; e != nil {
			return erpReferenceError(e)
		}
		rows := []model.CatalogProduct{p}
		if e := catalogDecorate(tx, rows, true); e != nil {
			return e
		}
		p = rows[0]
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return &p, e
}
func catalogDecorate(q *gorm.DB, rows []model.CatalogProduct, detail bool) error {
	if len(rows) == 0 {
		return nil
	}
	ids, cids := []int64{}, []int64{}
	for _, v := range rows {
		ids = append(ids, v.ProductID)
		if v.CategoryID != nil {
			cids = append(cids, *v.CategoryID)
		}
	}
	cats := []model.CatalogCategory{}
	if len(cids) > 0 {
		if e := q.Where("category_id IN ?", cids).Find(&cats).Error; e != nil {
			return e
		}
	}
	names := map[int64]string{}
	for _, v := range cats {
		names[v.CategoryID] = v.CategoryName
	}
	imgs := []model.CatalogImage{}
	if e := q.Where("product_id IN ?", ids).Order("product_id,position").Find(&imgs).Error; e != nil {
		return e
	}
	pictures := map[int64][]string{}
	for _, v := range imgs {
		pictures[v.ProductID] = append(pictures[v.ProductID], v.StorageKey)
	}
	variants := map[int64][]model.CatalogVariantView{}
	if detail {
		ps := []model.CatalogProfile{}
		if e := q.Where("product_id IN ?", ids).Order("sku_id").Find(&ps).Error; e != nil {
			return e
		}
		sids := []int64{}
		for _, v := range ps {
			sids = append(sids, v.SkuID)
		}
		skus := []model.ErpSku{}
		if len(sids) > 0 {
			if e := q.Where("sku_id IN ?", sids).Find(&skus).Error; e != nil {
				return e
			}
		}
		sm := map[int64]model.ErpSku{}
		for _, v := range skus {
			sm[v.SkuId] = v
		}
		refs, e := catalogSKUReferences(q, sids)
		if e != nil {
			return e
		}
		for _, v := range ps {
			a, ok := sm[v.SkuID]
			if !ok {
				return errs.New("商品SKU档案缺失，请核对")
			}
			variants[v.ProductID] = append(variants[v.ProductID], model.CatalogVariantView{CatalogVariant: model.CatalogVariant{SkuID: v.SkuID, SkuCode: a.SkuCode, Specification: a.Specification, Unit: a.Unit, Status: a.Status, Barcode: v.Barcode, WeightGrams: v.WeightGrams, LengthMM: v.LengthMM, WidthMM: v.WidthMM, HeightMM: v.HeightMM}, Referenced: refs[v.SkuID]})
		}
	}
	for i := range rows {
		rows[i].Images = append([]string{}, pictures[rows[i].ProductID]...)
		if rows[i].CategoryID != nil {
			rows[i].CategoryName = names[*rows[i].CategoryID]
		}
		if detail {
			rows[i].Variants = append([]model.CatalogVariantView{}, variants[rows[i].ProductID]...)
		}
	}
	return nil
}
func SelectCatalogCategories(ctx context.Context, in model.CatalogQuery, pg page.Query) ([]model.CatalogCategory, int64, error) {
	q := DB(ctx)
	if e := requireCatalog(q); e != nil {
		return nil, 0, e
	}
	q = q.Model(&model.CatalogCategory{})
	if in.CategoryCode != "" {
		q = q.Where("category_code=?", in.CategoryCode)
	}
	if in.CategoryName != "" {
		q = q.Where("category_name LIKE ?", "%"+in.CategoryName+"%")
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CatalogCategory{}
	e := q.Order("category_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, n, e
}
func SelectCatalogExport(ctx context.Context, in model.CatalogQuery, limit int) ([]model.CatalogExcelRow, error) {
	q := DB(ctx)
	if e := requireCatalog(q); e != nil {
		return nil, e
	}
	rows := []model.CatalogExcelRow{}
	e := catalogFilter(q.Table("erp_product p"), in).Joins("JOIN erp_sku_profile cp ON cp.product_id=p.product_id AND cp.tenant_id=p.tenant_id").Joins("JOIN erp_sku s ON s.sku_id=cp.sku_id AND s.tenant_id=p.tenant_id").Joins("LEFT JOIN erp_product_category c ON c.category_id=p.category_id AND c.tenant_id=p.tenant_id").
		Select("p.product_code,p.product_name,IFNULL(c.category_code,'') category_code,s.sku_code,s.specification,s.unit,s.status,IFNULL(cp.barcode,'') barcode,IFNULL(CAST(cp.weight_grams AS CHAR),'') weight_grams,IFNULL(CAST(cp.length_mm AS CHAR),'') length_mm,IFNULL(CAST(cp.width_mm AS CHAR),'') width_mm,IFNULL(CAST(cp.height_mm AS CHAR),'') height_mm,p.remark").Order("p.product_id,cp.sku_id").Limit(limit).Scan(&rows).Error
	return rows, e
}
