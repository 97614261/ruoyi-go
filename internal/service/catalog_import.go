package service

import (
	"bytes"
	"context"
	"github.com/xuri/excelize/v2"
	"io"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"strconv"
	"strings"
)

var catalogHeaders = []string{"商品编码", "商品名称", "分类编码", "SKU编码", "规格", "单位", "状态", "条码", "重量克", "长毫米", "宽毫米", "高毫米", "备注"}
var catalogInteger = regexp.MustCompile(`^[0-9]{1,10}$`)

func CatalogTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	for i, h := range catalogHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if e := f.SetCellStr("Sheet1", cell, h); e != nil {
			return nil, e
		}
	}
	style, e := f.NewStyle(&excelize.Style{NumFmt: 49})
	if e != nil {
		return nil, e
	}
	if e = f.SetColStyle("Sheet1", "A:M", style); e != nil {
		return nil, e
	}
	b, e := f.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func catalogNumber(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	if !catalogInteger.MatchString(s) {
		return nil, errs.New("重量、尺寸只能填整数")
	}
	n, e := strconv.ParseInt(s, 10, 64)
	return &n, e
}
func parseCatalogImport(key string, r io.Reader) ([]model.CatalogWrite, []string, error) {
	if !catalogKey.MatchString(key) {
		return nil, nil, errs.New("导入请求标识不合法")
	}
	raw, e := io.ReadAll(io.LimitReader(r, (10<<20)+1))
	if e != nil || len(raw) > 10<<20 {
		return nil, nil, errs.New("导入文件读取失败或超过10MB")
	}
	f, e := excelize.OpenReader(bytes.NewReader(raw), excelize.Options{UnzipSizeLimit: 32 << 20, UnzipXMLSizeLimit: 8 << 20})
	if e != nil {
		return nil, nil, errs.New("无法读取XLSX文件")
	}
	defer f.Close()
	if len(f.GetSheetList()) != 1 {
		return nil, nil, errs.New("导入文件只能有一个工作表")
	}
	sheet := f.GetSheetName(0)
	rows, e := f.Rows(sheet)
	if e != nil {
		return nil, nil, errs.New("无法读取工作表")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil, errs.New("导入表头为空")
	}
	header, e := rows.Columns()
	if e != nil || len(header) != len(catalogHeaders) {
		return nil, nil, errs.New("请使用商品导入模板")
	}
	for i, h := range catalogHeaders {
		if header[i] != h {
			return nil, nil, errs.New("表头与商品模板不一致")
		}
	}
	products := []model.CatalogWrite{}
	categoryCodes := []string{}
	groups := map[string]int{}
	skuCodes, bars := map[string]bool{}, map[string]bool{}
	count, rownum := 0, 1
	for rows.Next() {
		rownum++
		if rownum > 1000 {
			return nil, nil, errs.New("文件物理行过多，请移除多余格式或空行")
		}
		values, e := rows.Columns(excelize.Options{RawCellValue: true})
		if e != nil {
			return nil, nil, errs.New("读取导入行失败")
		}
		nonempty := false
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				nonempty = true
				break
			}
		}
		if !nonempty {
			continue
		}
		count++
		if count > 100 {
			return nil, nil, errs.New("一次最多导入100个SKU")
		}
		if len(values) > 13 {
			return nil, nil, errs.New("导入行存在额外列")
		}
		for len(values) < 13 {
			values = append(values, "")
		}
		for i := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, rownum)
			formula, e := f.GetCellFormula(sheet, cell)
			if e != nil || formula != "" {
				return nil, nil, errs.Newf("第%d行不允许公式", rownum)
			}
			values[i] = strings.TrimSpace(values[i])
		}
		for _, i := range []int{0, 2, 3, 7} {
			if values[i] != "" {
				cell, _ := excelize.CoordinatesToCellName(i+1, rownum)
				typ, e := f.GetCellType(sheet, cell)
				if e != nil || (typ != excelize.CellTypeInlineString && typ != excelize.CellTypeSharedString) {
					return nil, nil, errs.Newf("第%d行编码和条码必须以文本格式填写", rownum)
				}
			}
		}
		if values[6] == "启用" {
			values[6] = "0"
		}
		if values[6] == "停用" {
			values[6] = "1"
		}
		v := model.CatalogVariant{SkuCode: values[3], Specification: values[4], Unit: values[5], Status: values[6]}
		if values[7] != "" {
			v.Barcode = &values[7]
		}
		nums := []**int64{&v.WeightGrams, &v.LengthMM, &v.WidthMM, &v.HeightMM}
		for i, p := range nums {
			n, e := catalogNumber(values[8+i])
			if e != nil {
				return nil, nil, errs.Newf("第%d行重量或尺寸不合法", rownum)
			}
			*p = n
		}
		p := model.CatalogWrite{RequestKey: key, ProductCode: values[0], ProductName: values[1], Remark: values[12], Variants: []model.CatalogVariant{v}, Images: []string{}}
		if e = validateCatalog(&p, 0); e != nil {
			return nil, nil, errs.Newf("第%d行：%s", rownum, errs.As(e).Msg)
		}
		if values[2] != "" && !catalogCode.MatchString(values[2]) {
			return nil, nil, errs.Newf("第%d行分类编码不合法", rownum)
		}
		if skuCodes[strings.ToLower(v.SkuCode)] || v.Barcode != nil && bars[*v.Barcode] {
			return nil, nil, errs.Newf("第%d行SKU编码或条码重复", rownum)
		}
		skuCodes[strings.ToLower(v.SkuCode)] = true
		if v.Barcode != nil {
			bars[*v.Barcode] = true
		}
		if i, ok := groups[p.ProductCode]; ok {
			old := &products[i]
			if old.ProductName != p.ProductName || old.Remark != p.Remark || categoryCodes[i] != values[2] {
				return nil, nil, errs.Newf("第%d行同商品编码的名称、分类、备注必须一致", rownum)
			}
			old.Variants = append(old.Variants, p.Variants[0])
			if len(old.Variants) > 50 {
				return nil, nil, errs.New("每商品最多50个SKU")
			}
		} else {
			groups[p.ProductCode] = len(products)
			products = append(products, p)
			categoryCodes = append(categoryCodes, values[2])
		}
	}
	if rows.Error() != nil {
		return nil, nil, errs.New("工作表读取失败")
	}
	if count == 0 {
		return nil, nil, errs.New("导入数据不能为空")
	}
	return products, categoryCodes, nil
}
func ImportCatalog(ctx context.Context, key string, r io.Reader, operator string) (*model.CatalogResult, error) {
	products, categories, e := parseCatalogImport(key, r)
	if e != nil {
		return nil, e
	}
	hash := catalogHash("IMPORT", 0, struct {
		Products   []model.CatalogWrite
		Categories []string
	}{products, categories})
	return repository.ImportCatalogProducts(ctx, key, hash, products, categories, operator)
}
