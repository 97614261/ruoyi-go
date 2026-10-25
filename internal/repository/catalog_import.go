package repository

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func ImportCatalogProducts(ctx context.Context, key, hash string, products []model.CatalogWrite, categoryCodes []string, operator string) (*model.CatalogResult, error) {
	var out *model.CatalogResult
	e := erpTransaction(ctx, func(tx *gorm.DB) error {
		if e := requireCatalog(tx); e != nil {
			return e
		}
		if e := lockERPActor(tx); e != nil {
			return e
		}
		r, e := catalogReplay(tx, key, hash, "IMPORT")
		if e != nil {
			return e
		}
		if r != nil {
			out = r
			return nil
		}
		codes := []string{}
		for _, code := range categoryCodes {
			if code != "" {
				codes = append(codes, code)
			}
		}
		cats := []model.CatalogCategory{}
		if len(codes) > 0 {
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("category_code IN ?", codes).Order("category_id").Find(&cats).Error; e != nil {
				return e
			}
		}
		byCode := map[string]int64{}
		for _, c := range cats {
			if c.Status == "0" {
				byCode[c.CategoryCode] = c.CategoryID
			}
		}
		heads := make([]model.CatalogProduct, len(products))
		for i, p := range products {
			var cat *int64
			if categoryCodes[i] != "" {
				id, ok := byCode[categoryCodes[i]]
				if !ok {
					return errs.New("导入分类不存在、已停用或不属于当前企业")
				}
				cat = &id
			}
			heads[i] = model.CatalogProduct{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: p.Remark}, ProductCode: p.ProductCode, ProductName: p.ProductName, CategoryID: cat, Version: 1}
		}
		if e = tx.Create(&heads).Error; e != nil {
			return e
		}
		skus := []model.ErpSku{}
		variants := []model.CatalogVariant{}
		parent := []int{}
		for i, p := range products {
			for _, v := range p.Variants {
				skus = append(skus, model.ErpSku{SkuCode: v.SkuCode, ProductName: p.ProductName, Specification: v.Specification, Unit: v.Unit, Status: v.Status, CreateBy: operator, CreateTime: types.Now()})
				variants = append(variants, v)
				parent = append(parent, i)
			}
		}
		if e = tx.Create(&skus).Error; e != nil {
			return e
		}
		profiles := make([]model.CatalogProfile, len(skus))
		for i, a := range skus {
			v := variants[i]
			profiles[i] = model.CatalogProfile{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ProductID: heads[parent[i]].ProductID, SkuID: a.SkuId, Barcode: v.Barcode, WeightGrams: v.WeightGrams, LengthMM: v.LengthMM, WidthMM: v.WidthMM, HeightMM: v.HeightMM}
		}
		if e = tx.Create(&profiles).Error; e != nil {
			return e
		}
		ids := []int64{}
		for _, p := range heads {
			ids = append(ids, p.ProductID)
		}
		out = &model.CatalogResult{ProductIDs: ids, Version: 1, Rows: len(skus)}
		return catalogRecord(tx, key, hash, "IMPORT", operator, out)
	})
	return out, erpError(e)
}

// Compatibility endpoints must not bypass the aggregate version or lock.
func denyManagedCatalogSKU(q *gorm.DB, ids any) error {
	var n int64
	if e := q.WithContext(tenantctx.System(q.Statement.Context)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name='erp_sku_profile'").Count(&n).Error; e != nil {
		return e
	}
	if n == 0 {
		return nil
	}
	if e := q.Model(&model.CatalogProfile{}).Where("sku_id IN ?", ids).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return errs.New("SKU已归入商品档案，请从商品档案编辑或停用")
	}
	return nil
}
