package excelx

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"ruoyi-go/pkg/types"
)

// RowError 一行数据的解析错误。
//
// 导入不能遇错就停：用户上传 500 行、第 3 行格式错就整批失败，
// 改完再传又发现第 7 行错，体验极差。所以逐行收集，一次性反馈。
type RowError struct {
	// Row 是 Excel 里的行号（从 1 开始，含表头），方便用户对照文件定位
	Row int
	Msg string
}

func (e RowError) Error() string { return fmt.Sprintf("第 %d 行：%s", e.Row, e.Msg) }

// Import 解析 xlsx 为结构体切片。
//
// 按表头名称匹配 excel tag 的 name，列顺序无所谓，多余的列忽略。
// 返回解析成功的行、逐行错误、以及致命错误（文件打不开等）。
func Import[T any](r io.Reader, sheetName string) ([]T, []RowError, error) {
	file, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("打开 Excel 失败: %w", err)
	}
	defer file.Close()

	if sheetName == "" {
		sheets := file.GetSheetList()
		if len(sheets) == 0 {
			return nil, nil, fmt.Errorf("Excel 中没有工作表")
		}
		sheetName = sheets[0]
	}

	rows, err := file.GetRows(sheetName)
	if err != nil {
		return nil, nil, fmt.Errorf("读取工作表 %s 失败: %w", sheetName, err)
	}
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("Excel 中没有数据行")
	}

	var sample T
	elemType := reflect.TypeOf(sample)
	// 只支持结构体：下面用 item.Interface().(T) 取值，
	// T 若是指针类型这里拿到的是结构体值，断言会直接 panic。
	// 与其埋雷不如明确报错。
	if elemType == nil || elemType.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("导入目标必须是结构体类型，不能是指针或接口")
	}
	columns := filterColumns(parseColumns(elemType), column.forImport)

	// 表头名 -> 列下标
	headerIndex := make(map[string]int, len(rows[0]))
	for i, name := range rows[0] {
		headerIndex[strings.TrimSpace(name)] = i
	}

	result := make([]T, 0, len(rows)-1)
	var rowErrors []RowError

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if isBlankRow(row) {
			continue
		}

		item := reflect.New(elemType).Elem()
		var rowErr error
		for _, col := range columns {
			idx, ok := headerIndex[col.name]
			if !ok || idx >= len(row) {
				continue // 模板里没这列，跳过
			}
			if err := setField(item.Field(col.fieldIndex), strings.TrimSpace(row[idx]), col); err != nil {
				rowErr = fmt.Errorf("%s%w", col.name, err)
				break
			}
		}
		if rowErr != nil {
			rowErrors = append(rowErrors, RowError{Row: i + 1, Msg: rowErr.Error()})
			continue
		}
		result = append(result, item.Interface().(T))
	}
	return result, rowErrors, nil
}

// Template 生成只有表头的导入模板。
//
// 列集合与 Import 完全一致（含 type:import 的列），否则用户按模板填的数据
// 会有列对不上。
func Template[T any](w io.Writer, sheetName string) error {
	return WriteTemplate(w, sheetName, []T{})
}

func isBlankRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// setField 把单元格文本写入字段，按列配置做反向转换。
func setField(field reflect.Value, text string, col column) error {
	// 值映射反查："正常" -> "0"
	if len(col.converter) > 0 && text != "" {
		for value, label := range col.converter {
			if label == text {
				text = value
				break
			}
		}
	}
	if col.suffix != "" {
		text = strings.TrimSuffix(text, col.suffix)
	}

	// 指针字段：空值留 nil，非空则分配后写入指向的值。
	//
	// 递归时保留 timeLayout —— converter 和 suffix 上面已经处理过了，
	// 但时间格式要传下去，否则 *types.Time 字段会按默认格式解析而报错。
	if field.Kind() == reflect.Pointer {
		if text == "" {
			return nil
		}
		ptr := reflect.New(field.Type().Elem())
		if err := setField(ptr.Elem(), text, column{timeLayout: col.timeLayout}); err != nil {
			return err
		}
		field.Set(ptr)
		return nil
	}

	// 时间类型单独处理
	if field.Type() == reflect.TypeOf(types.Time{}) {
		if text == "" {
			return nil
		}
		layout := col.timeLayout
		if layout == "" {
			layout = types.Layout
		}
		parsed, err := timeParse(layout, text)
		if err != nil {
			return fmt.Errorf("格式应为 %s", layout)
		}
		field.Set(reflect.ValueOf(parsed))
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(text)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if text == "" {
			return nil
		}
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return fmt.Errorf("应为整数")
		}
		field.SetInt(n)
	case reflect.Float32, reflect.Float64:
		if text == "" {
			return nil
		}
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return fmt.Errorf("应为数字")
		}
		field.SetFloat(f)
	case reflect.Bool:
		field.SetBool(text == "true" || text == "是" || text == "1")
	default:
		return fmt.Errorf("不支持的字段类型 %s", field.Kind())
	}
	return nil
}

func timeParse(layout, value string) (types.Time, error) {
	parsed, err := time.ParseInLocation(layout, value, types.Location)
	if err != nil {
		return types.Time{}, err
	}
	return types.Time(parsed), nil
}
