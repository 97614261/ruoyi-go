package excelx

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"ruoyi-go/pkg/types"
)

type excelFixture struct {
	ID         int        `excel:"name:编号;sort:1;cell:numeric"`
	Phone      string     `excel:"name:手机号;sort:2;cell:text"`
	Status     string     `excel:"name:状态;sort:3;converter:0=正常,1=停用"`
	Score      float64    `excel:"name:分数;sort:4;suffix:分"`
	When       types.Time `excel:"name:日期;sort:5;format:2006/01/02"`
	Optional   *int       `excel:"name:可选值;sort:6;default:无"`
	ImportOnly string     `excel:"name:仅导入;sort:7;type:import"`
	ExportOnly string     `excel:"name:仅导出;sort:8;type:export"`
	Ignored    string
}

func TestExportWritesConfiguredColumns(t *testing.T) {
	when := types.Time(time.Date(2026, 8, 24, 12, 30, 0, 0, types.Location))
	value := 9
	rows := []excelFixture{{
		ID: 7, Phone: "0013800000000", Status: "0", Score: 12.5,
		When: when, Optional: &value, ImportOnly: "hidden", ExportOnly: "shown",
	}}
	var output bytes.Buffer
	if err := Export(&output, "结果", rows); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	file := openWorkbook(t, output.Bytes())
	defer file.Close()
	got, err := file.GetRows("结果")
	if err != nil {
		t.Fatalf("GetRows() error = %v", err)
	}
	want := [][]string{
		{"编号", "手机号", "状态", "分数", "日期", "可选值", "仅导出"},
		{"7", "0013800000000", "正常", "12.5分", "2026/08/24", "9", "shown"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("export rows = %#v, want %#v", got, want)
	}
}

func TestExportHandlesNilPointerRowsAndDefaults(t *testing.T) {
	rows := []*excelFixture{nil, {ID: 1, Phone: "", Status: "1"}}
	var output bytes.Buffer
	if err := Export(&output, "", rows); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	file := openWorkbook(t, output.Bytes())
	defer file.Close()
	got, err := file.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("row count = %d, want 3; rows=%#v", len(got), got)
	}
	if got[2][2] != "停用" || got[2][5] != "无" {
		t.Fatalf("defaults/converter not applied: %#v", got[2])
	}
}

func TestTemplateUsesImportColumns(t *testing.T) {
	var output bytes.Buffer
	if err := Template[excelFixture](&output, "导入模板"); err != nil {
		t.Fatalf("Template() error = %v", err)
	}
	file := openWorkbook(t, output.Bytes())
	defer file.Close()
	rows, err := file.GetRows("导入模板")
	if err != nil {
		t.Fatalf("GetRows() error = %v", err)
	}
	want := []string{"编号", "手机号", "状态", "分数", "日期", "可选值", "仅导入"}
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("template header = %#v, want %#v", rows, want)
	}
}

type importFixture struct {
	ID       int        `excel:"name:编号"`
	Status   string     `excel:"name:状态;converter:0=正常,1=停用"`
	Amount   float64    `excel:"name:金额;suffix:元"`
	Enabled  bool       `excel:"name:启用"`
	When     types.Time `excel:"name:日期;format:2006/01/02"`
	Optional *int       `excel:"name:可选值"`
	Exported string     `excel:"name:仅导出;type:export"`
}

type legacyImportFixture struct {
	Code        string `excel:"name:Code"`
	Name        string `excel:"name:Name"`
	Description string `excel:"name:Description"`
}

func TestImportLegacyXLS(t *testing.T) {
	data, err := os.ReadFile("testdata/table.xls")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	rows, rowErrors, err := Import[legacyImportFixture](bytes.NewReader(data), "Table")
	if err != nil {
		t.Fatalf("Import() legacy xls error = %v", err)
	}
	if len(rowErrors) != 0 || len(rows) != 11 {
		t.Fatalf("legacy rows=%d rowErrors=%#v", len(rows), rowErrors)
	}
	if rows[0].Code != "code1" || rows[0].Name != "name1" || rows[10].Description != "description11" {
		t.Fatalf("legacy values not parsed: first=%#v last=%#v", rows[0], rows[10])
	}
}

func TestImportConvertsValuesAndCollectsRowErrors(t *testing.T) {
	data := workbookBytes(t, "数据", [][]any{
		{"额外列", "状态", "编号", "金额", "启用", "日期", "可选值", "仅导出"},
		{"ignored", "正常", "7", "12.5元", "是", "2026/08/24", "9", "must-ignore"},
		{"ignored", "停用", "bad", "2元", "否", "2026/08/24", "", ""},
		{"ignored", "停用", "8", "2元", "1", "bad-date", "", ""},
		{"", "", "", "", "", "", "", ""},
	})

	rows, rowErrors, err := Import[importFixture](bytes.NewReader(data), "数据")
	if err != nil {
		t.Fatalf("Import() fatal error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v, want one valid row", rows)
	}
	got := rows[0]
	if got.ID != 7 || got.Status != "0" || got.Amount != 12.5 || !got.Enabled || got.Optional == nil || *got.Optional != 9 {
		t.Fatalf("converted row = %#v", got)
	}
	if got.When.String() != "2026-08-24 00:00:00" || got.Exported != "" {
		t.Fatalf("time/export-only conversion = %#v", got)
	}
	if len(rowErrors) != 2 || rowErrors[0].Row != 3 || rowErrors[1].Row != 4 {
		t.Fatalf("row errors = %#v", rowErrors)
	}
	if !strings.Contains(rowErrors[0].Error(), "第 3 行") || !strings.Contains(rowErrors[0].Msg, "编号应为整数") {
		t.Fatalf("first row error = %#v", rowErrors[0])
	}
	if !strings.Contains(rowErrors[1].Msg, "日期格式应为 2006/01/02") {
		t.Fatalf("second row error = %#v", rowErrors[1])
	}
}

func TestImportFatalAndBoundaryCases(t *testing.T) {
	t.Run("invalid workbook", func(t *testing.T) {
		_, _, err := Import[importFixture](strings.NewReader("not xlsx"), "")
		if err == nil || !strings.Contains(err.Error(), "打开 Excel 失败") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing sheet", func(t *testing.T) {
		data := workbookBytes(t, "存在", [][]any{{"编号"}, {"1"}})
		_, _, err := Import[importFixture](bytes.NewReader(data), "不存在")
		if err == nil || !strings.Contains(err.Error(), "读取工作表") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("header only", func(t *testing.T) {
		data := workbookBytes(t, "数据", [][]any{{"编号"}})
		rows, rowErrors, err := Import[importFixture](bytes.NewReader(data), "")
		if err != nil || len(rows) != 0 || len(rowErrors) != 0 {
			t.Fatalf("rows=%#v rowErrors=%#v err=%v", rows, rowErrors, err)
		}
	})
	t.Run("pointer target", func(t *testing.T) {
		data := workbookBytes(t, "数据", [][]any{{"编号"}, {"1"}})
		_, _, err := Import[*importFixture](bytes.NewReader(data), "")
		if err == nil || !strings.Contains(err.Error(), "不能是指针") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unsupported field", func(t *testing.T) {
		type unsupported struct {
			Values []string `excel:"name:值"`
		}
		data := workbookBytes(t, "数据", [][]any{{"值"}, {"x"}})
		rows, rowErrors, err := Import[unsupported](bytes.NewReader(data), "")
		if err != nil || len(rows) != 0 || len(rowErrors) != 1 || !strings.Contains(rowErrors[0].Msg, "不支持的字段类型 slice") {
			t.Fatalf("rows=%#v rowErrors=%#v err=%v", rows, rowErrors, err)
		}
	})
	t.Run("non-blank row limit includes invalid rows", func(t *testing.T) {
		data := workbookBytes(t, "数据", [][]any{{"编号"}, {"bad"}, {"2"}})
		_, _, err := Import[importFixture](bytes.NewReader(data), "", 1)
		var limitErr RowLimitError
		if !errors.As(err, &limitErr) || limitErr.Limit != 1 {
			t.Fatalf("invalid rows must count toward limit, err=%v", err)
		}
	})
}

func TestExportRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		rows any
		want string
	}{
		{"nil", nil, "必须是切片"},
		{"not slice", excelFixture{}, "必须是切片"},
		{"primitive elements", []int{1}, "元素必须是结构体"},
		{"no tags", []struct{ Name string }{{Name: "x"}}, "没有可导出的字段"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := Export(&output, "", test.rows)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want contains %q", err, test.want)
			}
		})
	}
}

func TestWriteResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("success", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		if err := WriteResponse(ctx, "用户 表.xlsx", "用户", []excelFixture{}); err != nil {
			t.Fatalf("WriteResponse() error = %v", err)
		}
		if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != ContentType {
			t.Fatalf("status=%d content-type=%q", recorder.Code, recorder.Header().Get("Content-Type"))
		}
		if !strings.Contains(recorder.Header().Get("Content-Disposition"), "filename*=UTF-8''") {
			t.Fatalf("content-disposition=%q", recorder.Header().Get("Content-Disposition"))
		}
		file := openWorkbook(t, recorder.Body.Bytes())
		file.Close()
	})
	t.Run("error writes nothing", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		err := WriteResponse(ctx, "bad.xlsx", "", 123)
		if err == nil || recorder.Body.Len() != 0 || recorder.Header().Get("Content-Type") != "" {
			t.Fatalf("error=%v body=%q headers=%v", err, recorder.Body.String(), recorder.Header())
		}
	})
}

func openWorkbook(t *testing.T, data []byte) *excelize.File {
	t.Helper()
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	return file
}

func workbookBytes(t *testing.T, sheet string, rows [][]any) []byte {
	t.Helper()
	file := excelize.NewFile()
	defer file.Close()
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatalf("SetSheetName() error = %v", err)
	}
	for index, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if err := file.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatalf("SetSheetRow() error = %v", err)
		}
	}
	var output bytes.Buffer
	if err := file.Write(&output); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	return output.Bytes()
}
