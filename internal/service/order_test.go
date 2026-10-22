package service

import (
	"bytes"
	"context"
	"github.com/xuri/excelize/v2"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/types"
	"strings"
	"testing"
	"time"
)

func sampleOrderInput() model.ERPOrderWrite {
	return model.ERPOrderWrite{ShopID: 1, SourceOrderNo: "00001", RequestKey: "test-key-00001", ERPOrderFields: model.ERPOrderFields{ReceiverName: "验收人", ReceiverCountry: "cn", ReceiverAddress: "测试地址", Currency: "cny", OrderedAt: types.Now()}, Lines: []model.ERPOrderLineInput{{SourceLineNo: "0001", ShopItemCode: "0002", SourceTitle: "测试商品", Quantity: 1}}}
}
func TestOrderInputDecimalAndFingerprint(t *testing.T) {
	in := sampleOrderInput()
	s := "0"
	in.OriginalAmount = &s
	if err := normalizeOrder(&in); err != nil {
		t.Fatal(err)
	}
	if *in.OriginalAmount != "0.0000" || in.Currency != "CNY" || in.SourceOrderNo != "00001" {
		t.Fatal("normalization")
	}
	hash := orderInputHash(in)
	in.RequestKey = "other-key-001"
	in.Version = 9
	if hash != orderInputHash(in) {
		t.Fatal("key/version leaked into original content fingerprint")
	}
	for _, bad := range []string{"-1", "1e3", "1.00001", "10000000000000000", "NaN", " 1", "01"} {
		v := sampleOrderInput()
		v.OriginalAmount = &bad
		if normalizeOrder(&v) == nil {
			t.Fatalf("accepted bad decimal %q", bad)
		}
	}
	for _, mutate := range []func(*model.ERPOrderWrite){func(v *model.ERPOrderWrite) { v.SourceOrderNo = " 0001" }, func(v *model.ERPOrderWrite) { v.Lines[0].Quantity = 0 }, func(v *model.ERPOrderWrite) { v.Lines = append(v.Lines, v.Lines[0]) }, func(v *model.ERPOrderWrite) { v.ReceiverName = "bad\nname" }, func(v *model.ERPOrderWrite) { v.ReceiverAddress = strings.Repeat("中", 501) }, func(v *model.ERPOrderWrite) { v.OrderedAt = types.Time(time.Time{}) }} {
		v := sampleOrderInput()
		mutate(&v)
		if normalizeOrder(&v) == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
func importTestWorkbook(t *testing.T, rows [][]string, edit func(*excelize.File)) []byte {
	t.Helper()
	b, e := ERPOrderImportTemplate()
	if e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenReader(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	for i, row := range rows {
		for j, v := range row {
			c, _ := excelize.CoordinatesToCellName(j+1, i+2)
			if e = f.SetCellStr("订单导入", c, v); e != nil {
				t.Fatal(e)
			}
		}
	}
	if edit != nil {
		edit(f)
	}
	out, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
func sampleImportRow(no, line string) []string {
	return []string{no, line, "000SKU", "测试标题", "2", "测试收件人", "0012345", "CN", "省", "市", "00100", "测试地址", "CNY", "12.3400", "2026-10-21 10:00:00"}
}
func TestOrderImportGroupingAndText(t *testing.T) {
	b := importTestWorkbook(t, [][]string{sampleImportRow("00001", "01"), sampleImportRow("00001", "02")}, nil)
	groups, n, e := parseOrderImport(context.Background(), b, 1)
	if e != nil || n != 2 || len(groups) != 1 || groups[0].Invalid != "" || len(groups[0].Input.Lines) != 2 || groups[0].Input.SourceOrderNo != "00001" {
		t.Fatal("group contract", e)
	}
	tests := []struct {
		name  string
		rows  [][]string
		edit  func(*excelize.File)
		fatal bool
	}{
		{"duplicate line", [][]string{sampleImportRow("0001", "01"), sampleImportRow("0001", "01")}, nil, false},
		{"conflicting recipient", [][]string{sampleImportRow("0001", "01"), sampleImportRow("0001", "02")}, func(f *excelize.File) { f.SetCellStr("订单导入", "F3", "另一个人") }, false},
		{"numeric identifier", [][]string{sampleImportRow("0001", "01")}, func(f *excelize.File) { f.SetCellInt("订单导入", "A2", 123456789012345678) }, false},
		{"formula", [][]string{sampleImportRow("0001", "01")}, func(f *excelize.File) { f.SetCellFormula("订单导入", "E2", "1+1") }, true},
		{"wrong header", [][]string{sampleImportRow("0001", "01")}, func(f *excelize.File) { f.SetCellStr("订单导入", "A1", "wrong") }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _, err := parseOrderImport(context.Background(), importTestWorkbook(t, tt.rows, tt.edit), 1)
			if tt.fatal {
				if err == nil {
					t.Fatal("fatal workbook accepted")
				}
			} else if err != nil || len(g) != 1 || g[0].Invalid == "" {
				t.Fatal("invalid group accepted", err)
			}
		})
	}
}
func TestOrderImportRowCaps(t *testing.T) {
	rows := make([][]string, 5001)
	for i := range rows {
		rows[i] = sampleImportRow("one", "line")
	}
	if _, _, err := parseOrderImport(context.Background(), importTestWorkbook(t, rows, nil), 1); err == nil {
		t.Fatal("5001 rows accepted")
	}
	rows = rows[:101]
	for i := range rows {
		rows[i] = sampleImportRow("one", strings.Repeat("0", i+1))
	}
	g, _, err := parseOrderImport(context.Background(), importTestWorkbook(t, rows, nil), 1)
	if err != nil || len(g) != 1 || g[0].Invalid == "" {
		t.Fatal("101 lines accepted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = parseOrderImport(ctx, importTestWorkbook(t, [][]string{sampleImportRow("one", "line")}, nil), 1); err == nil {
		t.Fatal("cancel ignored")
	}
}
