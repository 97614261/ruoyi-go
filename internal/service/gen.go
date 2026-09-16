package service

import (
	"context"
	"encoding/json"
	"fmt"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

const maxGenTablesPerRequest = 200

var (
	genDBIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)
	genGoIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	genRouteSegment = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	genDictType     = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

func ListGenTablePage(ctx context.Context, query model.GenTableQuery, pg page.Query) ([]model.GenTable, int64, error) {
	return repository.SelectGenTablePage(ctx, query, pg)
}

func ListDBTablePage(ctx context.Context, query model.GenTableQuery, pg page.Query) ([]model.GenTable, int64, error) {
	return repository.SelectDBTablePage(ctx, query, pg)
}

func GetGenTableInfo(ctx context.Context, tableID int64) (*model.GenTable, []model.GenTable, error) {
	table, err := repository.SelectGenTableByID(ctx, tableID)
	if err != nil {
		return nil, nil, err
	}
	if table == nil {
		return nil, nil, errs.New("代码生成表不存在")
	}
	decodeGenOptions(table)
	tables, err := repository.SelectGenTablesAllWithColumns(ctx)
	if err != nil {
		return nil, nil, err
	}
	return table, tables, nil
}

func ListGenTableColumns(ctx context.Context, tableID int64) ([]model.GenTableColumn, error) {
	return repository.SelectGenTableColumns(ctx, tableID)
}

func ImportGenTables(ctx context.Context, tableNames []string, tplWebType, operator string) error {
	genWriteMu.Lock()
	defer genWriteMu.Unlock()
	return importGenTablesLocked(ctx, tableNames, tplWebType, operator)
}

func importGenTablesLocked(ctx context.Context, tableNames []string, tplWebType, operator string) error {
	names, err := normalizeGenTableNames(tableNames)
	if err != nil {
		return err
	}
	if !allowedGenWebType(tplWebType) {
		return errs.New("前端模板类型不合法")
	}
	existing, err := repository.SelectExistingGenTableNames(ctx, names)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return errs.Newf("表 %s 已经导入", existing[0])
	}
	tables, err := repository.SelectDBTablesByNames(ctx, names)
	if err != nil {
		return err
	}
	if len(tables) != len(names) {
		return errs.New("选择的表不存在或不允许导入")
	}
	columnsByTable, err := repository.SelectDBColumnsByTableNames(ctx, names)
	if err != nil {
		return err
	}
	now := types.Now()
	for i := range tables {
		initGenTable(&tables[i], tplWebType, operator, now)
		columns := columnsByTable[tables[i].TableName]
		if len(columns) == 0 {
			return errs.Newf("表 %s 没有可生成字段", tables[i].TableName)
		}
		for j := range columns {
			initGenColumn(&columns[j], operator, now)
		}
		tables[i].Columns = columns
	}
	return repository.InsertGenTables(ctx, tables)
}

func UpdateGenTable(ctx context.Context, table *model.GenTable, operator string) error {
	genWriteMu.Lock()
	defer genWriteMu.Unlock()

	if err := validateGenTable(table); err != nil {
		return err
	}
	existing, err := repository.SelectGenTableByID(ctx, table.TableID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("代码生成表不存在")
	}
	// 物理表名和物理列归属不允许由页面改写。
	if table.TableName != existing.TableName {
		return errs.New("表名称不能修改")
	}
	physicalByID := make(map[int64]model.GenTableColumn, len(existing.Columns))
	for _, column := range existing.Columns {
		physicalByID[column.ColumnID] = column
	}
	seen := make(map[int64]struct{}, len(table.Columns))
	for i := range table.Columns {
		column := &table.Columns[i]
		physical, ok := physicalByID[column.ColumnID]
		if !ok || physical.TableID != table.TableID {
			return errs.New("字段不存在或不属于当前表")
		}
		if _, duplicate := seen[column.ColumnID]; duplicate {
			return errs.New("字段列表包含重复项")
		}
		seen[column.ColumnID] = struct{}{}
		column.TableID = table.TableID
		column.ColumnName = physical.ColumnName
		column.IsPK = physical.IsPK
		column.IsIncrement = physical.IsIncrement
	}
	if len(seen) != len(physicalByID) {
		return errs.New("字段列表不完整，请先同步数据库")
	}
	options, err := json.Marshal(table.Params)
	if err != nil {
		return errs.Wrap(err, "生成参数格式错误")
	}
	optionsText := string(options)
	table.Options = &optionsText
	table.UpdateBy = operator
	table.UpdateTime = types.Now()
	return repository.UpdateGenTableWithColumns(ctx, table)
}

func DeleteGenTableConfigs(ctx context.Context, tableIDs []int64) error {
	genWriteMu.Lock()
	defer genWriteMu.Unlock()
	ids, err := normalizeRelationIDs(tableIDs, "代码生成表")
	if err != nil || len(ids) == 0 {
		return err
	}
	return repository.DeleteGenTables(ctx, ids)
}

func SyncGenTable(ctx context.Context, tableName, operator string) error {
	genWriteMu.Lock()
	defer genWriteMu.Unlock()
	if !genDBIdentifier.MatchString(tableName) {
		return errs.New("表名称不合法")
	}
	table, err := repository.SelectGenTableByName(ctx, tableName)
	if err != nil {
		return err
	}
	if table == nil {
		return errs.New("代码生成表不存在")
	}
	freshMap, err := repository.SelectDBColumnsByTableNames(ctx, []string{tableName})
	if err != nil {
		return err
	}
	fresh := freshMap[tableName]
	if len(fresh) == 0 {
		return errs.New("同步失败，原表结构不存在")
	}
	oldByName := make(map[string]model.GenTableColumn, len(table.Columns))
	for _, column := range table.Columns {
		oldByName[column.ColumnName] = column
	}
	now := types.Now()
	for i := range fresh {
		physical := fresh[i]
		if old, ok := oldByName[physical.ColumnName]; ok {
			old.ColumnComment = physical.ColumnComment
			old.ColumnType = physical.ColumnType
			old.IsPK = physical.IsPK
			old.IsIncrement = physical.IsIncrement
			old.IsRequired = physical.IsRequired
			old.Sort = physical.Sort
			old.UpdateBy = operator
			old.UpdateTime = now
			fresh[i] = old
		} else {
			initGenColumn(&fresh[i], operator, now)
		}
	}
	return repository.SyncGenTableColumns(ctx, table.TableID, fresh)
}

// CreateTablesAndImport 只接受 CREATE TABLE 语句。DDL 在 MySQL 中会隐式提交，
// 因此沿用 Java 的语义：先创建成功的表会保留，随后统一导入生成器配置。
func CreateTablesAndImport(ctx context.Context, sqlText, tplWebType, operator string) error {
	genWriteMu.Lock()
	defer genWriteMu.Unlock()
	statements, names, err := parseCreateTableStatements(sqlText)
	if err != nil {
		return err
	}
	if err := repository.ExecuteCreateTableStatements(ctx, statements); err != nil {
		return errs.Wrap(err, "创建表结构异常")
	}
	return importGenTablesLocked(ctx, names, tplWebType, operator)
}

func loadGenerationTable(ctx context.Context, tableName string) (*model.GenTable, error) {
	if !genDBIdentifier.MatchString(tableName) {
		return nil, errs.New("表名称不合法")
	}
	table, err := repository.SelectGenTableByName(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, errs.New("代码生成表不存在")
	}
	if err := prepareGenerationTable(ctx, table); err != nil {
		return nil, err
	}
	return table, nil
}

func initGenTable(table *model.GenTable, webType, operator string, now types.Time) {
	cfg := currentGeneratorConfig()
	className := upperCamel(removeConfiguredTablePrefix(table.TableName))
	businessName := table.TableName
	if index := strings.LastIndex(businessName, "_"); index >= 0 && index+1 < len(businessName) {
		businessName = businessName[index+1:]
	}
	formCols := 1
	genPath := "/"
	options := `{"parentMenuId":3,"genView":"0"}`
	table.ClassName = className
	table.TplCategory = "crud"
	table.TplWebType = webType
	table.PackageName = cfg.PackageName
	table.ModuleName = "system"
	table.BusinessName = businessName
	table.FunctionName = strings.NewReplacer("若依", "", "表", "").Replace(table.TableComment)
	if table.FunctionName == "" {
		table.FunctionName = className
	}
	table.FunctionAuthor = cfg.Author
	table.FormColNum = &formCols
	table.GenType = "0"
	table.GenPath = &genPath
	table.Options = &options
	table.CreateBy = operator
	table.CreateTime = now
}

func initGenColumn(column *model.GenTableColumn, operator string, now types.Time) {
	dataType := strings.ToLower(column.ColumnType)
	if index := strings.IndexByte(dataType, '('); index >= 0 {
		dataType = dataType[:index]
	}
	column.JavaField = lowerCamel(column.ColumnName)
	column.JavaType = "String"
	column.QueryType = "EQ"
	column.HTMLType = "input"
	column.IsInsert = "1"
	column.IsEdit, column.IsList, column.IsQuery = "0", "0", "0"
	if !containsFold([]string{"create_by", "create_time", "update_by", "update_time", "remark"}, column.ColumnName) && column.IsPK != "1" {
		column.IsEdit = "1"
	}
	if !containsFold([]string{"id", "create_by", "create_time", "update_by", "update_time", "remark"}, column.ColumnName) && column.IsPK != "1" {
		column.IsList = "1"
	}
	if !containsFold([]string{"id", "create_by", "create_time", "update_by", "update_time", "remark"}, column.ColumnName) && column.IsPK != "1" {
		column.IsQuery = "1"
	}
	switch dataType {
	case "datetime", "date", "timestamp", "time":
		column.JavaType, column.HTMLType = "Date", "datetime"
	case "tinyint", "smallint", "mediumint", "int", "integer":
		column.JavaType = "Integer"
	case "bigint":
		column.JavaType = "Long"
	case "decimal", "numeric":
		column.JavaType = "BigDecimal"
	case "float", "double", "real":
		column.JavaType = "Double"
	case "bit", "boolean":
		column.JavaType = "Boolean"
	case "text", "tinytext", "mediumtext", "longtext", "json":
		column.HTMLType = "textarea"
	}
	name := strings.ToLower(column.ColumnName)
	if strings.HasSuffix(name, "name") {
		column.QueryType = "LIKE"
	}
	switch {
	case strings.HasSuffix(name, "status"):
		column.HTMLType = "radio"
	case strings.HasSuffix(name, "type"), strings.HasSuffix(name, "sex"):
		column.HTMLType = "select"
	case strings.HasSuffix(name, "image"):
		column.HTMLType = "imageUpload"
	case strings.HasSuffix(name, "file"):
		column.HTMLType = "fileUpload"
	case strings.HasSuffix(name, "content"):
		column.HTMLType = "editor"
	}
	column.CreateBy = operator
	column.CreateTime = now
}

func validateGenTable(table *model.GenTable) error {
	if table == nil || table.TableID <= 0 {
		return errs.New("表编号不能为空")
	}
	if !genDBIdentifier.MatchString(table.TableName) || !genGoIdentifier.MatchString(table.ClassName) ||
		!token.IsIdentifier(table.ClassName) || exportedField(table.ClassName) != table.ClassName {
		return errs.New("表名称或实体类名称不合法")
	}
	if !genRouteSegment.MatchString(table.ModuleName) || !genRouteSegment.MatchString(table.BusinessName) {
		return errs.New("模块名或业务名不合法")
	}
	if strings.TrimSpace(table.TableComment) == "" || strings.TrimSpace(table.PackageName) == "" ||
		strings.TrimSpace(table.FunctionName) == "" || strings.TrimSpace(table.FunctionAuthor) == "" {
		return errs.New("代码生成基本信息不能为空")
	}
	if table.TplCategory != "crud" && table.TplCategory != "tree" && table.TplCategory != "sub" {
		return errs.New("生成模板类型不合法")
	}
	if !allowedGenWebType(table.TplWebType) {
		return errs.New("前端模板类型不合法")
	}
	if table.GenType != "0" && table.GenType != "1" {
		return errs.New("代码生成方式不合法")
	}
	if table.FormColNum == nil || *table.FormColNum < 1 || *table.FormColNum > 3 {
		return errs.New("表单布局不合法")
	}
	if len(table.Columns) == 0 || len(table.Columns) > 500 {
		return errs.New("字段数量不合法")
	}
	seenFields := make(map[string]struct{}, len(table.Columns))
	for _, column := range table.Columns {
		field := exportedField(column.JavaField)
		if !genDBIdentifier.MatchString(column.ColumnName) ||
			!genGoIdentifier.MatchString(column.JavaField) || !token.IsIdentifier(field) {
			return errs.Newf("字段 %s 的属性名不合法", column.ColumnName)
		}
		fieldKey := strings.ToLower(field)
		if _, duplicate := seenFields[fieldKey]; duplicate {
			return errs.Newf("字段属性 %s 重复", column.JavaField)
		}
		seenFields[fieldKey] = struct{}{}
		if !allowedValue(column.JavaType, "Long", "String", "Integer", "Double", "BigDecimal", "Date", "Boolean") ||
			!allowedValue(column.QueryType, "EQ", "NE", "GT", "GTE", "LT", "LTE", "LIKE", "BETWEEN") ||
			!allowedValue(column.HTMLType, "input", "textarea", "select", "checkbox", "radio", "datetime", "imageUpload", "fileUpload", "editor") {
			return errs.Newf("字段 %s 的生成配置不合法", column.ColumnName)
		}
		if column.DictType != nil && strings.TrimSpace(*column.DictType) != "" &&
			!genDictType.MatchString(strings.TrimSpace(*column.DictType)) {
			return errs.Newf("字段 %s 的字典类型不合法", column.ColumnName)
		}
		for _, flag := range []string{column.IsInsert, column.IsEdit, column.IsList, column.IsQuery, column.IsRequired} {
			if flag != "0" && flag != "1" {
				return errs.Newf("字段 %s 的开关值不合法", column.ColumnName)
			}
		}
	}
	if table.TplCategory == "tree" {
		if stringParam(table.Params, "treeCode") == "" {
			return errs.New("树编码字段不能为空")
		}
		if stringParam(table.Params, "treeParentCode") == "" {
			return errs.New("树父编码字段不能为空")
		}
		if stringParam(table.Params, "treeName") == "" {
			return errs.New("树名称字段不能为空")
		}
	}
	if table.TplCategory == "sub" {
		if table.SubTableName == nil || strings.TrimSpace(*table.SubTableName) == "" {
			return errs.New("关联子表的表名不能为空")
		}
		if table.SubTableFKName == nil || strings.TrimSpace(*table.SubTableFKName) == "" {
			return errs.New("子表关联的外键名不能为空")
		}
	}
	return nil
}

func validateGenTableForRender(table *model.GenTable) error {
	if table.PKColumn == nil {
		return errs.New("生成表没有可用字段")
	}
	if table.TplCategory == "tree" {
		for label, name := range map[string]string{
			"树编码字段": table.TreeCode, "树父编码字段": table.TreeParentCode, "树名称字段": table.TreeName,
		} {
			if findColumnByName(table.Columns, name) == nil {
				return errs.Newf("%s %s 不属于当前表", label, name)
			}
		}
		codeColumn := findColumnByName(table.Columns, table.TreeCode)
		parentColumn := findColumnByName(table.Columns, table.TreeParentCode)
		if codeColumn == nil || parentColumn == nil ||
			!allowedValue(codeColumn.JavaType, "Long", "Integer", "String") ||
			!allowedValue(parentColumn.JavaType, "Long", "Integer", "String") ||
			goColumnType(*codeColumn) != goColumnType(*parentColumn) {
			return errs.New("树编码与树父编码必须使用相同的整数或字符串类型")
		}
	}
	if table.TplCategory == "sub" && table.SubTable == nil {
		return errs.New("关联子表不存在或尚未导入")
	}
	if table.TplCategory == "sub" {
		if table.SubTable.TableName == table.TableName {
			return errs.New("主表不能关联自身")
		}
		if !genDBIdentifier.MatchString(table.SubTable.TableName) ||
			!genGoIdentifier.MatchString(table.SubTable.ClassName) ||
			!token.IsIdentifier(table.SubTable.ClassName) || exportedField(table.SubTable.ClassName) != table.SubTable.ClassName ||
			strings.TrimSpace(table.SubTable.FunctionName) == "" {
			return errs.New("关联子表的生成配置不合法")
		}
		if table.SubTable.ClassName == table.ClassName {
			return errs.New("主表和子表实体类名称不能相同")
		}
		for _, column := range table.Columns {
			if strings.EqualFold(exportedField(column.JavaField), table.SubTable.ClassName+"List") {
				return errs.New("主表字段与子表列表属性重名")
			}
		}
		setGenPKColumn(table.SubTable)
		if table.SubTable.PKColumn == nil {
			return errs.New("关联子表没有可用字段")
		}
		fk := findColumnByName(table.SubTable.Columns, strings.TrimSpace(*table.SubTableFKName))
		if fk == nil {
			return errs.New("子表关联外键不存在")
		}
		if goColumnType(*fk) != goColumnType(*table.PKColumn) {
			return errs.New("主表主键与子表外键类型不一致")
		}
		subFields := make(map[string]struct{}, len(table.SubTable.Columns))
		for _, column := range table.SubTable.Columns {
			if !genDBIdentifier.MatchString(column.ColumnName) ||
				!genGoIdentifier.MatchString(column.JavaField) || !token.IsIdentifier(exportedField(column.JavaField)) {
				return errs.Newf("子表字段 %s 的生成配置不合法", column.ColumnName)
			}
			field := strings.ToLower(exportedField(column.JavaField))
			if _, duplicate := subFields[field]; duplicate {
				return errs.Newf("子表字段属性 %s 重复", column.JavaField)
			}
			subFields[field] = struct{}{}
		}
	}
	return nil
}

func decodeGenOptions(table *model.GenTable) {
	table.Params = map[string]any{}
	if table.Options != nil && strings.TrimSpace(*table.Options) != "" {
		_ = json.Unmarshal([]byte(*table.Options), &table.Params)
	}
	table.TreeCode = stringParam(table.Params, "treeCode")
	table.TreeParentCode = stringParam(table.Params, "treeParentCode")
	table.TreeName = stringParam(table.Params, "treeName")
	table.ParentMenuName = stringParam(table.Params, "parentMenuName")
	table.ParentMenuID = int64Param(table.Params, "parentMenuId")
	table.View = boolParam(table.Params, "genView")
}

func setGenPKColumn(table *model.GenTable) {
	if len(table.Columns) > 0 {
		table.PKColumn = &table.Columns[0]
	}
	for i := range table.Columns {
		if table.Columns[i].IsPK == "1" {
			table.PKColumn = &table.Columns[i]
			break
		}
	}
	if table.SubTable != nil {
		setGenPKColumn(table.SubTable)
	}
}

func findColumnByName(columns []model.GenTableColumn, columnName string) *model.GenTableColumn {
	for i := range columns {
		if strings.EqualFold(columns[i].ColumnName, strings.TrimSpace(columnName)) {
			return &columns[i]
		}
	}
	return nil
}

func normalizeGenTableNames(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errs.New("请选择要导入的表")
	}
	if len(values) > maxGenTablesPerRequest {
		return nil, errs.Newf("一次最多导入%d张表", maxGenTablesPerRequest)
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		name := strings.TrimSpace(raw)
		if !genDBIdentifier.MatchString(name) {
			return nil, errs.Newf("表名称 %s 不合法", name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func upperCamel(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '_' || r == '-' || unicode.IsSpace(r) })
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		out.WriteString(string(runes))
	}
	return out.String()
}

func lowerCamel(value string) string {
	result := upperCamel(value)
	if result == "" {
		return result
	}
	runes := []rune(result)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if value, ok := params[key]; ok && value != nil {
		return fmt.Sprint(value)
	}
	return ""
}

func int64Param(params map[string]any, key string) int64 {
	value := stringParam(params, key)
	result, _ := strconv.ParseInt(value, 10, 64)
	return result
}

func boolParam(params map[string]any, key string) bool {
	switch strings.ToLower(stringParam(params, key)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func allowedGenWebType(value string) bool {
	return allowedValue(value, "element-ui", "element-plus", "element-plus-typescript")
}

func allowedValue(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}
