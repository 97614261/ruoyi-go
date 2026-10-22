package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"github.com/xuri/excelize/v2"
	"io"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
	"strconv"
	"strings"
	"time"
)

var orderImportHeaders = []string{"来源订单号", "来源行号", "店铺SKU编码", "商品标题", "数量", "收件人", "电话", "国家二字码", "省州", "城市", "邮编", "详细地址", "币种", "原始金额", "下单时间"}

type orderImportGroup struct {
	Input   model.ERPOrderWrite
	Rows    []int64
	Hash    string
	Invalid string
}

func ERPOrderImportTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "订单导入")
	for i, h := range orderImportHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr("订单导入", cell, h); err != nil {
			return nil, err
		}
	}
	style, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err = f.SetColStyle("订单导入", "A:O", style); err != nil {
		return nil, err
	}
	if err = f.SetColWidth("订单导入", "A", "O", 20); err != nil {
		return nil, err
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Unlike the general user importer, identifiers are never TrimSpace'd or
// silently converted from numeric Excel cells. Preflight bounds expansion and
// rejects formulas/macros/external links, including shared/array formulas.
func orderXLSXPreflight(ctx context.Context, data []byte) (map[int]bool, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errs.New("仅接受固定XLSX模板")
	}
	if len(z.File) > 1000 {
		return nil, errs.New("工作簿结构超过限制")
	}
	var total uint64
	bad := map[int]bool{}
	for _, f := range z.File {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		total += f.UncompressedSize64
		if total > 64<<20 || f.UncompressedSize64 > 32<<20 {
			return nil, errs.New("工作簿解压超过限制")
		}
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "vbaproject") || strings.Contains(name, "externallinks/") {
			return nil, errs.New("不接受宏或外部链接工作簿")
		}
		if !strings.HasPrefix(name, "xl/worksheets/") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		r, e := f.Open()
		if e != nil {
			return nil, errs.New("工作簿损坏")
		}
		dec := xml.NewDecoder(io.LimitReader(r, 32<<20))
		cellRow := 0
		col := ""
		cellType := ""
		for {
			token, e := dec.Token()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				r.Close()
				return nil, errs.New("工作表格式不正确")
			}
			if start, ok := token.(xml.StartElement); ok {
				if start.Name.Local == "c" {
					cellRow = 0
					col = ""
					cellType = ""
					for _, a := range start.Attr {
						if a.Name.Local == "r" {
							c, row, e := excelize.CellNameToCoordinates(a.Value)
							if e != nil || row > 10001 || c > 15 {
								r.Close()
								return nil, errs.New("仅接受A—O列、最多10000个物理数据行的固定模板")
							}
							cellRow = row
							col, _ = excelize.ColumnNumberToName(c)
						}
						if a.Name.Local == "t" {
							cellType = a.Value
						}
					}
				}
				if start.Name.Local == "f" {
					r.Close()
					return nil, errs.New("订单模板不得含公式，请粘贴为文本值")
				}
				if (start.Name.Local == "v" || start.Name.Local == "is") && cellRow > 1 && (col == "A" || col == "B" || col == "C" || col == "G" || col == "K") && cellType != "s" && cellType != "inlineStr" {
					bad[cellRow] = true
				}
			}
		}
		r.Close()
	}
	return bad, nil
}
func parseOrderImport(ctx context.Context, data []byte, shopID int64) ([]orderImportGroup, int64, error) {
	bad, err := orderXLSXPreflight(ctx, data)
	if err != nil {
		return nil, 0, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 32 << 20})
	if err != nil {
		return nil, 0, errs.New("XLSX格式不正确")
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "订单导入" {
		return nil, 0, errs.New("请使用单工作表的订单导入模板")
	}
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, 0, errs.New("工作表无法读取")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, 0, errs.New("缺少模板表头")
	}
	head, err := rows.Columns()
	if err != nil || len(head) != len(orderImportHeaders) {
		return nil, 0, errs.New("模板表头不正确")
	}
	for i, h := range head {
		if h != orderImportHeaders[i] {
			return nil, 0, errs.New("模板表头或列顺序不正确")
		}
	}
	groups := []orderImportGroup{}
	byNo := map[string]int{}
	var count int64
	rowNo := 1
	for rows.Next() {
		rowNo++
		if rowNo > 10001 {
			return nil, count, errs.New("工作表物理行超过限制")
		}
		if err = ctx.Err(); err != nil {
			return nil, count, err
		}
		values, e := rows.Columns(excelize.Options{RawCellValue: true})
		if e != nil {
			return nil, count, errs.New("数据行无法读取")
		}
		blank := true
		for _, v := range values {
			if v != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		count++
		if count > 5000 {
			return nil, count, errs.New("每文件最多5000个非空数据行")
		}
		v := make([]string, 15)
		copy(v, values)
		invalid := ""
		// Validate actual cell types too: OOXML may omit the optional cell r
		// attribute, so the streaming preflight alone is not sufficient.
		for _, column := range []int{1, 2, 3, 7, 11} {
			if v[column-1] != "" {
				cell, _ := excelize.CoordinatesToCellName(column, rowNo)
				kind, e := f.GetCellType(sheets[0], cell)
				if e != nil || (kind != excelize.CellTypeSharedString && kind != excelize.CellTypeInlineString) {
					bad[rowNo] = true
				}
			}
		}
		if len(values) > 15 || bad[rowNo] {
			invalid = "编号、电话和邮编须为文本单元格；不可使用数值或科学计数格式"
		}
		parsed, e := time.ParseInLocation(types.Layout, v[14], types.Location)
		if e != nil {
			invalid = "下单时间须为 yyyy-MM-dd HH:mm:ss 文本"
		}
		quantity, e := strconv.ParseInt(v[4], 10, 64)
		if e != nil {
			invalid = "数量须为正整数文本"
		}
		in := model.ERPOrderWrite{ShopID: shopID, SourceOrderNo: v[0], RequestKey: "import-validation", ERPOrderFields: model.ERPOrderFields{ReceiverName: v[5], ReceiverPhone: v[6], ReceiverCountry: v[7], ReceiverRegion: v[8], ReceiverCity: v[9], ReceiverPostcode: v[10], ReceiverAddress: v[11], Currency: v[12], OrderedAt: types.Time(parsed)}, Lines: []model.ERPOrderLineInput{{SourceLineNo: v[1], ShopItemCode: v[2], SourceTitle: v[3], Quantity: quantity}}}
		if v[13] != "" {
			in.OriginalAmount = &v[13]
		}
		if e = normalizeOrder(&in); e != nil {
			invalid = "必填字段、编号、金额、数量或长度不符合模板规则"
		}
		key := v[0]
		if !orderText(key, 128, true) {
			key = "invalid-row-" + strconv.Itoa(rowNo)
			in.SourceOrderNo = ""
		}
		if idx, ok := byNo[key]; ok {
			g := &groups[idx]
			if orderHash(g.Input.ERPOrderFields) != orderHash(in.ERPOrderFields) {
				g.Invalid = "同一订单的收件信息、币种、金额或时间不一致"
			}
			g.Rows = append(g.Rows, int64(rowNo))
			g.Input.Lines = append(g.Input.Lines, in.Lines...)
			if invalid != "" {
				g.Invalid = invalid
			}
		} else {
			byNo[key] = len(groups)
			groups = append(groups, orderImportGroup{Input: in, Rows: []int64{int64(rowNo)}, Invalid: invalid})
		}
	}
	if err = rows.Error(); err != nil {
		return nil, count, errs.New("工作表读取失败")
	}
	if count == 0 {
		return nil, 0, errs.New("导入文件没有数据")
	}
	for i := range groups {
		g := &groups[i]
		if e := normalizeOrder(&g.Input); e != nil && g.Invalid == "" {
			g.Invalid = "同单行号重复或超过100行，或数据不符合规则"
		}
		g.Hash = orderInputHash(g.Input)
		if g.Input.SourceOrderNo == "" {
			g.Hash = orderHash(g.Rows)
		}
		g.Input.RequestKey = "import:" + g.Hash[:56]
	}
	return groups, count, nil
}
