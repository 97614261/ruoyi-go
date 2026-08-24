// Package excelx 基于 struct tag 的 Excel 导出，对齐 Java 版 @Excel 注解。
//
// tag 语法：`excel:"key:value;key:value"`
//
//	name       列名，必填，缺失则该字段不导出
//	sort       列顺序，越小越靠前，缺省排在末尾
//	type       export | import | all(默认)，导出时跳过 import
//	cell       numeric | text | string(默认)
//	           numeric 写成数字；text 强制文本（手机号、长数字防止变成科学计数法）
//	converter  值映射，如 "0=正常,1=停用"，对应 readConverterExp
//	format     时间格式，Go 布局串，如 "2006-01-02 15:04:05"
//	width      列宽
//	suffix     值后缀，如 "毫秒"
//	default    值为空时的占位
//
// 示例：
//
//	PostID   int64      `excel:"name:岗位序号;cell:numeric;sort:1"`
//	Status   string     `excel:"name:状态;converter:0=正常,1=停用;sort:5"`
//	CreateTime types.Time `excel:"name:创建时间;format:2006-01-02 15:04:05;width:30"`
//
// 未实现 @Excel 的 dictType（走字典表翻译）：RuoYi 现有实体都没用它，
// 需要时在 service 层预处理成字符串再导出。
package excelx

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"ruoyi-go/pkg/types"
)

// defaultSortWeight 未指定 sort 的列排在最后，对齐 Java 的 Integer.MAX_VALUE。
const defaultSortWeight = 1 << 30

type column struct {
	fieldIndex int
	name       string
	sortWeight int
	cellType   string
	converter  map[string]string
	timeLayout string
	width      float64
	suffix     string
	defaultVal string
	// mode 为 "export" / "import" / ""（两者都参与），对应 Java @Excel 的 type
	mode string
}

// forExport 该列是否参与导出。
func (c column) forExport() bool { return c.mode != "import" }

// forImport 该列是否参与导入与模板。
func (c column) forImport() bool { return c.mode != "export" }

// Export 把切片写成 xlsx。
//
// rows 必须是结构体切片（或结构体指针切片）。整份文件在内存中构建后
// 一次性写出 —— 这样中途出错还能返回 JSON 错误，不会写出半个损坏的文件。
func Export(w io.Writer, sheetName string, rows any) error {
	return write(w, sheetName, rows, column.forExport)
}

// WriteTemplate 写出只含表头的导入模板。
//
// 【必须用 forImport 过滤，不能复用 Export】
// Export 走的是 forExport，会把标了 type:import 的列排除掉 ——
// 用户导入模板里最关键的"部门编号"正是这类列，用 Export 生成的模板缺这一列，
// 用户照着填完导入，所有人都没有部门。
func WriteTemplate(w io.Writer, sheetName string, empty any) error {
	return write(w, sheetName, empty, column.forImport)
}

func write(w io.Writer, sheetName string, rows any, keep func(column) bool) error {
	elemType, err := elementType(rows)
	if err != nil {
		return err
	}
	columns := filterColumns(parseColumns(elemType), keep)
	if len(columns) == 0 {
		return fmt.Errorf("类型 %s 没有可导出的字段（检查 excel tag 的 name）", elemType.Name())
	}

	if sheetName == "" {
		sheetName = "Sheet1"
	}
	file := excelize.NewFile()
	defer file.Close()

	if err := file.SetSheetName("Sheet1", sheetName); err != nil {
		return fmt.Errorf("设置工作表名失败: %w", err)
	}

	// 用流式写入器，对齐 Java 版的 SXSSFWorkbook：
	// 行数据顺序写出、只保留少量行在内存，几十万行也不会把进程撑爆。
	// 代价是只能顺序写、不能回头改单元格 —— 导出场景正好只需要顺序写。
	stream, err := file.NewStreamWriter(sheetName)
	if err != nil {
		return fmt.Errorf("创建流式写入器失败: %w", err)
	}

	// 列宽必须在写入任何行之前设置
	for i, col := range columns {
		if err := stream.SetColWidth(i+1, i+1, col.width); err != nil {
			return fmt.Errorf("设置列宽失败: %w", err)
		}
	}
	if err := writeHeader(file, stream, columns); err != nil {
		return err
	}
	if err := writeRows(stream, columns, reflect.ValueOf(rows)); err != nil {
		return err
	}
	if err := stream.Flush(); err != nil {
		return fmt.Errorf("刷新流式写入器失败: %w", err)
	}

	if _, err := file.WriteTo(w); err != nil {
		return fmt.Errorf("写出 Excel 失败: %w", err)
	}
	return nil
}

func elementType(rows any) (reflect.Type, error) {
	t := reflect.TypeOf(rows)
	if t == nil || t.Kind() != reflect.Slice {
		return nil, fmt.Errorf("导出数据必须是切片，实际是 %T", rows)
	}
	elem := t.Elem()
	if elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct {
		return nil, fmt.Errorf("导出数据的元素必须是结构体，实际是 %s", elem.Kind())
	}
	return elem, nil
}

func parseColumns(t reflect.Type) []column {
	var columns []column
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		tag, ok := field.Tag.Lookup("excel")
		if !ok {
			continue
		}
		col, ok := parseTag(tag)
		if !ok {
			continue
		}
		col.fieldIndex = i
		columns = append(columns, col)
	}

	// 稳定排序：sort 相同时保持字段声明顺序
	sort.SliceStable(columns, func(a, b int) bool {
		return columns[a].sortWeight < columns[b].sortWeight
	})
	return columns
}

// filterColumns 按 keep 谓词筛列，保持原顺序。
func filterColumns(columns []column, keep func(column) bool) []column {
	result := make([]column, 0, len(columns))
	for _, col := range columns {
		if keep(col) {
			result = append(result, col)
		}
	}
	return result
}

func parseTag(tag string) (column, bool) {
	col := column{sortWeight: defaultSortWeight, width: 16}

	for _, part := range strings.Split(tag, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(part), ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "name":
			col.name = value
		case "sort":
			if n, err := strconv.Atoi(value); err == nil {
				col.sortWeight = n
			}
		case "type":
			col.mode = value
		case "cell":
			col.cellType = value
		case "converter":
			col.converter = parseConverter(value)
		case "format":
			col.timeLayout = value
		case "width":
			if f, err := strconv.ParseFloat(value, 64); err == nil && f > 0 {
				col.width = f
			}
		case "suffix":
			col.suffix = value
		case "default":
			col.defaultVal = value
		}
	}
	// 没有列名的字段不导出，对齐 @Excel 必须写 name 的用法
	return col, col.name != ""
}

// parseConverter 解析 "0=正常,1=停用"。
func parseConverter(exp string) map[string]string {
	mapping := make(map[string]string)
	for _, pair := range strings.Split(exp, ",") {
		k, v, found := strings.Cut(strings.TrimSpace(pair), "=")
		if found {
			mapping[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return mapping
}

// writeHeader 写表头。
//
// 样式仍然要通过 file.NewStyle 创建，再以 excelize.Cell 的形式带进流式写入。
func writeHeader(file *excelize.File, stream *excelize.StreamWriter, columns []column) error {
	style, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#DDEBF7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return fmt.Errorf("创建表头样式失败: %w", err)
	}

	row := make([]any, len(columns))
	for i, col := range columns {
		row[i] = excelize.Cell{StyleID: style, Value: col.name}
	}
	if err := stream.SetRow("A1", row); err != nil {
		return fmt.Errorf("写入表头失败: %w", err)
	}
	return nil
}

// writeRows 逐行写出数据。
//
// 流式写入要求行号严格递增，所以这里不能跳过任何行 ——
// 遇到 nil 元素也要占位写空行，否则后续 SetRow 的行号会与预期错位。
func writeRows(stream *excelize.StreamWriter, columns []column, data reflect.Value) error {
	row := make([]any, len(columns))

	for r := 0; r < data.Len(); r++ {
		item := data.Index(r)
		if item.Kind() == reflect.Pointer {
			if item.IsNil() {
				for i := range row {
					row[i] = ""
				}
			} else {
				item = item.Elem()
				fillRow(row, columns, item)
			}
		} else {
			fillRow(row, columns, item)
		}

		cell, err := excelize.CoordinatesToCellName(1, r+2) // 第 1 行是表头
		if err != nil {
			return fmt.Errorf("计算单元格坐标失败: %w", err)
		}
		if err := stream.SetRow(cell, row); err != nil {
			return fmt.Errorf("写入第 %d 行失败: %w", r+2, err)
		}
	}
	return nil
}

func fillRow(row []any, columns []column, item reflect.Value) {
	for i, col := range columns {
		row[i] = cellValue(item.Field(col.fieldIndex), col)
	}
}

// cellValue 把字段值转成写入 Excel 的内容。
func cellValue(field reflect.Value, col column) any {
	raw := field.Interface()

	// 指针解引用，nil 走默认值
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return col.defaultVal
		}
		field = field.Elem()
		raw = field.Interface()
	}

	// 时间单独处理：零值输出空串而不是 0001-01-01
	if t, ok := raw.(types.Time); ok {
		if t.IsZero() {
			return col.defaultVal
		}
		layout := col.timeLayout
		if layout == "" {
			layout = types.Layout
		}
		return t.Std().In(types.Location).Format(layout)
	}

	text := fmt.Sprintf("%v", raw)

	// 值映射优先于数字化：状态列即使底层是 "0" 也要显示成"正常"
	if len(col.converter) > 0 {
		if label, ok := col.converter[text]; ok {
			return label + col.suffix
		}
	}

	if text == "" {
		return col.defaultVal
	}

	switch col.cellType {
	case "numeric":
		// 只有无后缀时才真正写成数字，否则拼上后缀就不再是数值了
		if col.suffix == "" {
			if n, err := strconv.ParseFloat(text, 64); err == nil {
				return n
			}
		}
	case "text":
		// 强制文本：手机号、身份证等长数字，写成数值会变科学计数法
		return text + col.suffix
	}
	return text + col.suffix
}
