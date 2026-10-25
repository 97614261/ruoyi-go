package service

import (
	"bytes"
	"fmt"
	"github.com/xuri/excelize/v2"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func validCatalog() model.CatalogWrite {
	return model.CatalogWrite{RequestKey: "catalog-test-key", ProductCode: "P", ProductName: "学习商品", Variants: []model.CatalogVariant{{SkuCode: "S", Unit: "件", Status: "0"}}}
}
func catalogBook(t *testing.T, n int, change func(*excelize.File)) []byte {
	t.Helper()
	raw, e := CatalogTemplate()
	if e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenReader(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	for i := 0; i < n; i++ {
		code := fmt.Sprintf("P%03d", i)
		vals := []string{code, "测试", "", code, "规格", "件", "0", "00" + code, "10", "20", "30", "40", ""}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			if e = f.SetCellStr("Sheet1", cell, v); e != nil {
				t.Fatal(e)
			}
		}
	}
	if change != nil {
		change(f)
	}
	b, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestCatalogValidation(t *testing.T) {
	p := validCatalog()
	if validateCatalog(&p, 0) != nil {
		t.Fatal("valid")
	}
	for _, edit := range []func(*model.CatalogWrite){func(p *model.CatalogWrite) { p.Variants[0].Unit = "" }, func(p *model.CatalogWrite) { p.Variants = append(p.Variants, p.Variants[0]) }, func(p *model.CatalogWrite) { n := int64(0); p.Variants[0].WeightGrams = &n }, func(p *model.CatalogWrite) { p.ProductName = "a\x00b" }, func(p *model.CatalogWrite) { p.Images = []string{"https://evil/a.png"} }, func(p *model.CatalogWrite) { p.Variants[0].Status = "" }} {
		p = validCatalog()
		edit(&p)
		if validateCatalog(&p, 0) == nil {
			t.Fatal("invalid accepted", p)
		}
	}
	b := "0012345"
	p = validCatalog()
	p.Variants[0].Barcode = &b
	if validateCatalog(&p, 0) != nil || *p.Variants[0].Barcode != b {
		t.Fatal("leading zeros")
	}
}
func TestCatalogImportStrictAndBounded(t *testing.T) {
	raw := catalogBook(t, 2, nil)
	p, c, e := parseCatalogImport("catalog-import-key", bytes.NewReader(raw))
	if e != nil || len(p) != 2 || len(c) != 2 || !strings.HasPrefix(*p[0].Variants[0].Barcode, "00") {
		t.Fatal(e)
	}
	for _, change := range []func(*excelize.File){func(f *excelize.File) { f.SetCellFormula("Sheet1", "B2", "1+1") }, func(f *excelize.File) { f.SetCellInt("Sheet1", "H2", 123) }, func(f *excelize.File) { f.SetCellStr("Sheet1", "D3", "-A") }, func(f *excelize.File) { f.SetCellStr("Sheet1", "D3", "P000") }, func(f *excelize.File) { f.SetCellStr("Sheet1", "A1", "错表头") }, func(f *excelize.File) {
		f.SetCellStr("Sheet1", "A2", "x-A")
		f.SetCellStr("Sheet1", "A3", "x-A")
		f.SetCellStr("Sheet1", "B3", "冲突名称")
	}} {
		b := catalogBook(t, 2, change)
		if _, _, e = parseCatalogImport("catalog-import-key", bytes.NewReader(b)); e == nil {
			t.Fatal("invalid workbook accepted")
		}
	}
	b := catalogBook(t, 101, nil)
	if _, _, e = parseCatalogImport("catalog-import-key", bytes.NewReader(b)); e == nil {
		t.Fatal("101 rows accepted")
	}
}
