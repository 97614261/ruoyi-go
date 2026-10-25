package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strings"
	"unicode/utf8"
)

var catalogCode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$`)
var catalogKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,63}$`)

func catalogHash(kind string, id int64, v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(fmt.Sprintf("%s:%d:", kind, id)), b...)))
}
func catalogText(v string, max int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= max && !strings.ContainsAny(v, "\x00\r\n")
}
func validateCatalog(in *model.CatalogWrite, id int64) error {
	if !catalogKey.MatchString(in.RequestKey) || id < 0 || id == 0 && in.Version != 0 || id > 0 && in.Version < 1 {
		return errs.New("商品请求标识或版本不合法")
	}
	in.ProductCode = strings.TrimSpace(in.ProductCode)
	in.ProductName = strings.TrimSpace(in.ProductName)
	in.Remark = strings.TrimSpace(in.Remark)
	if !catalogCode.MatchString(in.ProductCode) || in.ProductName == "" || !catalogText(in.ProductName, 120) || !catalogText(in.Remark, 500) || in.CategoryID != nil && *in.CategoryID < 1 {
		return errs.New("商品编码、名称、分类或备注不合法")
	}
	if len(in.Variants) < 1 || len(in.Variants) > 50 || len(in.Images) > 5 {
		return errs.New("每商品需要1—50个SKU，最多5张图片")
	}
	codes, bars, ids, imgs := map[string]bool{}, map[string]bool{}, map[int64]bool{}, map[string]bool{}
	for i := range in.Variants {
		v := &in.Variants[i]
		v.SkuCode = strings.TrimSpace(v.SkuCode)
		v.Unit = strings.TrimSpace(v.Unit)
		v.Specification = strings.TrimSpace(v.Specification)
		if v.SkuID < 0 || v.SkuID > 0 && ids[v.SkuID] || !catalogCode.MatchString(v.SkuCode) || codes[strings.ToLower(v.SkuCode)] || v.Unit == "" || !catalogText(v.Unit, 20) || !catalogText(v.Specification, 120) || (v.Status != "0" && v.Status != "1") {
			return errs.Newf("第%d个SKU编码、规格、单位或状态不合法/重复", i+1)
		}
		ids[v.SkuID] = true
		codes[strings.ToLower(v.SkuCode)] = true
		if v.Barcode != nil {
			b := strings.TrimSpace(*v.Barcode)
			if b == "" {
				v.Barcode = nil
			} else {
				if !catalogCode.MatchString(b) || bars[b] {
					return errs.Newf("第%d个SKU条码不合法或重复", i+1)
				}
				v.Barcode = &b
				bars[b] = true
			}
		}
		for _, n := range []*int64{v.WeightGrams, v.LengthMM, v.WidthMM, v.HeightMM} {
			if n != nil && (*n < 1 || *n > 1000000000) {
				return errs.New("重量和尺寸应为1—1000000000的整数，未知留空")
			}
		}
	}
	for _, key := range in.Images {
		if len(key) > 255 || !strings.HasPrefix(key, "/profile/") || strings.ContainsAny(key, "?\x00#\\") || imgs[key] {
			return errs.New("商品图片标识不合法或重复")
		}
		imgs[key] = true
	}
	return nil
}
func WriteCatalogProduct(ctx context.Context, id int64, in model.CatalogWrite, operator string) (*model.CatalogResult, error) {
	if e := validateCatalog(&in, id); e != nil {
		return nil, e
	}
	return repository.WriteCatalogProduct(ctx, id, in, catalogHash("PRODUCT", id, in), operator)
}
func WriteCatalogCategory(ctx context.Context, id int64, in model.CatalogCategoryWrite, operator string) (*model.CatalogResult, error) {
	in.CategoryCode = strings.TrimSpace(in.CategoryCode)
	in.CategoryName = strings.TrimSpace(in.CategoryName)
	in.Remark = strings.TrimSpace(in.Remark)
	if id < 0 || !catalogKey.MatchString(in.RequestKey) || id == 0 && in.Version != 0 || id > 0 && in.Version < 1 || !catalogCode.MatchString(in.CategoryCode) || in.CategoryName == "" || !catalogText(in.CategoryName, 120) || !catalogText(in.Remark, 500) || (in.Status != "0" && in.Status != "1") {
		return nil, errs.New("分类编码、名称、状态或版本不合法")
	}
	return repository.WriteCatalogCategory(ctx, id, in, catalogHash("CATEGORY", id, in), operator)
}
func ListCatalogProducts(ctx context.Context, q model.CatalogQuery, pg page.Query) ([]model.CatalogProduct, int64, error) {
	return repository.SelectCatalogProducts(ctx, q, pg)
}
func GetCatalogProduct(ctx context.Context, id int64) (*model.CatalogProduct, error) {
	return repository.SelectCatalogProduct(ctx, id)
}
func ListCatalogCategories(ctx context.Context, q model.CatalogQuery, pg page.Query) ([]model.CatalogCategory, int64, error) {
	return repository.SelectCatalogCategories(ctx, q, pg)
}
func ExportCatalog(ctx context.Context, q model.CatalogQuery) ([]model.CatalogExcelRow, error) {
	rows, e := repository.SelectCatalogExport(ctx, q, MaxExportRows+1)
	if e != nil {
		return nil, e
	}
	return rows, checkExportSize(len(rows))
}
