package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

type generatedFile struct {
	PreviewKey string
	Path       string
	Content    string
}

const maxGeneratedArchiveBytes = 64 << 20

func PreviewGenCode(ctx context.Context, tableID int64) (map[string]string, error) {
	genWriteMu.RLock()
	defer genWriteMu.RUnlock()
	table, err := repository.SelectGenTableByID(ctx, tableID)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, errs.New("代码生成表不存在")
	}
	if err := prepareGenerationTable(ctx, table); err != nil {
		return nil, err
	}
	files, err := renderGeneratedFiles(table)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(files))
	for _, file := range files {
		result[file.PreviewKey] = file.Content
	}
	return result, nil
}

func DownloadGenCode(ctx context.Context, tableNames []string) ([]byte, error) {
	genWriteMu.RLock()
	defer genWriteMu.RUnlock()
	names, err := normalizeGenTableNames(tableNames)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	seenPaths := make(map[string]struct{})
	totalBytes := 0
	for _, name := range names {
		table, err := loadGenerationTable(ctx, name)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		files, err := renderGeneratedFiles(table)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		for _, file := range files {
			totalBytes += len(file.Content)
			if totalBytes > maxGeneratedArchiveBytes {
				_ = writer.Close()
				return nil, errs.New("批量生成内容超过64MB，请减少选择的表")
			}
			clean := filepath.ToSlash(filepath.Clean(file.Path))
			if _, duplicate := seenPaths[clean]; duplicate {
				_ = writer.Close()
				return nil, errs.Newf("批量生成出现重复文件：%s", clean)
			}
			seenPaths[clean] = struct{}{}
			entry, err := writer.Create(clean)
			if err != nil {
				_ = writer.Close()
				return nil, fmt.Errorf("创建生成压缩包失败: %w", err)
			}
			if _, err := entry.Write([]byte(file.Content)); err != nil {
				_ = writer.Close()
				return nil, fmt.Errorf("写入生成压缩包失败: %w", err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("完成生成压缩包失败: %w", err)
	}
	return buffer.Bytes(), nil
}

func WriteGenCode(ctx context.Context, tableName string) error {
	genWriteMu.RLock()
	defer genWriteMu.RUnlock()
	cfg := currentGeneratorConfig()
	if !cfg.AllowOverwrite {
		return errs.New("【系统预设】不允许生成文件覆盖到本地")
	}
	table, err := loadGenerationTable(ctx, tableName)
	if err != nil {
		return err
	}
	files, err := renderGeneratedFiles(table)
	if err != nil {
		return err
	}
	base := cfg.OutputRoot
	if table.GenPath != nil && strings.TrimSpace(*table.GenPath) != "" && strings.TrimSpace(*table.GenPath) != "/" {
		candidate := strings.TrimSpace(*table.GenPath)
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(base, candidate)
		}
		base = filepath.Clean(candidate)
	}
	if !pathWithin(cfg.OutputRoot, base) {
		return errs.New("自定义生成路径必须位于 gen.outputRoot 内")
	}
	for _, file := range files {
		target := filepath.Join(base, filepath.FromSlash(file.Path))
		if !pathWithin(base, target) {
			return errs.New("生成文件路径不合法")
		}
		if err := writeGeneratedFile(target, []byte(file.Content)); err != nil {
			return err
		}
	}
	return nil
}

func prepareGenerationTable(ctx context.Context, table *model.GenTable) error {
	decodeGenOptions(table)
	if err := validateGenTable(table); err != nil {
		return err
	}
	if table.SubTableName != nil && strings.TrimSpace(*table.SubTableName) != "" {
		sub, err := repository.SelectGenTableByName(ctx, strings.TrimSpace(*table.SubTableName))
		if err != nil {
			return err
		}
		if sub == nil {
			return errs.New("关联子表不存在或尚未导入")
		}
		decodeGenOptions(sub)
		table.SubTable = sub
	}
	setGenPKColumn(table)
	return validateGenTableForRender(table)
}

func renderGeneratedFiles(table *model.GenTable) ([]generatedFile, error) {
	modelSource := renderModelSource(table)
	repositorySource := renderRepositorySource(table)
	serviceSource := renderServiceSource(table)
	handlerSource := renderHandlerSource(table)
	routerSource := renderRouterSource(table)
	goSources := []*string{&modelSource, &repositorySource, &serviceSource, &handlerSource, &routerSource}
	for _, source := range goSources {
		formatted, err := format.Source([]byte(*source))
		if err != nil {
			return nil, fmt.Errorf("格式化生成的 Go 代码失败: %w", err)
		}
		*source = string(formatted)
	}
	ext := ".js"
	if table.TplWebType == "element-plus-typescript" {
		ext = ".ts"
	}
	business := strings.ToLower(table.BusinessName)
	module := strings.ToLower(table.ModuleName)
	files := []generatedFile{
		// 保留 domain.java 这个首个预览标签，兼容现有 Vue3 页面的默认 activeName。
		{PreviewKey: "vm/go/domain.java.vm", Path: "internal/model/" + business + ".go", Content: modelSource},
		{PreviewKey: "vm/go/repository.go.vm", Path: "internal/repository/" + business + ".go", Content: repositorySource},
		{PreviewKey: "vm/go/service.go.vm", Path: "internal/service/" + business + ".go", Content: serviceSource},
		{PreviewKey: "vm/go/handler.go.vm", Path: "internal/handler/" + business + ".go", Content: handlerSource},
		{PreviewKey: "vm/go/router.go.vm", Path: "internal/router/" + business + "_routes.go", Content: routerSource},
		{PreviewKey: "vm/js/api" + ext + ".vm", Path: "vue/src/api/" + module + "/" + business + ext, Content: renderAPISource(table)},
		{PreviewKey: "vm/vue/index.vue.vm", Path: "vue/src/views/" + module + "/" + business + "/index.vue", Content: renderVueSource(table)},
		{PreviewKey: "vm/sql/sql.vm", Path: "sql/" + business + "Menu.sql", Content: renderMenuSQL(table)},
		{PreviewKey: "vm/go/README.md.vm", Path: "GENERATED_" + strings.ToUpper(business) + ".md", Content: renderGeneratedReadme(table)},
	}
	return files, nil
}

func renderModelSource(table *model.GenTable) string {
	var b strings.Builder
	b.WriteString("package model\n\n")
	if hasJavaType(table.Columns, "Date") || table.SubTable != nil && hasJavaType(table.SubTable.Columns, "Date") {
		b.WriteString("import \"ruoyi-go/pkg/types\"\n\n")
	}
	fmt.Fprintf(&b, "// %s %s。\ntype %s struct {\n", safeComment(table.ClassName), safeComment(table.FunctionName), table.ClassName)
	for _, column := range table.Columns {
		field := exportedField(column.JavaField)
		fmt.Fprintf(&b, "\t%s %s `gorm:\"column:%s\" json:\"%s\"%s%s` // %s\n",
			field, goColumnType(column), column.ColumnName, column.JavaField, generatedBindingTag(column), generatedExcelTag(column), safeComment(column.ColumnComment))
	}
	if table.TplCategory == "sub" && table.SubTable != nil {
		fmt.Fprintf(&b, "\t%s []%s `gorm:\"-\" json:\"%s\"` // %s\n",
			subListField(table), table.SubTable.ClassName, lowerFirst(table.SubTable.ClassName)+"List", safeComment(table.SubTable.FunctionName))
	}
	b.WriteString("}\n\n")
	if table.TplCategory == "sub" && table.SubTable != nil {
		fmt.Fprintf(&b, "// %s %s。\ntype %s struct {\n", safeComment(table.SubTable.ClassName), safeComment(table.SubTable.FunctionName), table.SubTable.ClassName)
		for _, column := range table.SubTable.Columns {
			binding := generatedBindingTag(column)
			if strings.EqualFold(column.ColumnName, strings.TrimSpace(*table.SubTableFKName)) {
				binding = ""
			}
			fmt.Fprintf(&b, "\t%s %s `gorm:\"column:%s\" json:\"%s\"%s` // %s\n",
				exportedField(column.JavaField), goColumnType(column), column.ColumnName, column.JavaField, binding, safeComment(column.ColumnComment))
		}
		b.WriteString("}\n\n")
	}
	fmt.Fprintf(&b, "// %sQuery 查询条件。\ntype %sQuery struct {\n", table.ClassName, table.ClassName)
	for _, column := range table.Columns {
		if column.IsQuery != "1" {
			continue
		}
		field := exportedField(column.JavaField)
		if column.QueryType == "BETWEEN" {
			fmt.Fprintf(&b, "\tBegin%s string `form:\"params[begin%s]\"`\n", field, field)
			fmt.Fprintf(&b, "\tEnd%s string `form:\"params[end%s]\"`\n", field, field)
		} else {
			fmt.Fprintf(&b, "\t%s string `form:\"%s\"`\n", field, column.JavaField)
		}
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "var %sSortColumns = map[string]string{\n", table.ClassName)
	for _, column := range table.Columns {
		if column.IsList == "1" || column.IsPK == "1" {
			fmt.Fprintf(&b, "\t%q: %q,\n", column.JavaField, column.ColumnName)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func renderRepositorySource(table *model.GenTable) string {
	class := table.ClassName
	tableName := table.TableName
	pk := table.PKColumn
	var b strings.Builder
	b.WriteString("package repository\n\nimport (\n\t\"context\"\n\t\"errors\"\n\t\"fmt\"\n\n\t\"gorm.io/gorm\"\n\n")
	fmt.Fprintf(&b, "\t\"ruoyi-go/internal/model\"\n\t\"ruoyi-go/pkg/page\"\n)\n\n")
	fmt.Fprintf(&b, "func %sFilter(db *gorm.DB, query model.%sQuery) *gorm.DB {\n", lowerFirst(class), class)
	for _, column := range table.Columns {
		if column.IsQuery != "1" {
			continue
		}
		field := exportedField(column.JavaField)
		if column.QueryType == "BETWEEN" {
			fmt.Fprintf(&b, "\tif query.Begin%s != \"\" { db = db.Where(%q, query.Begin%s) }\n", field, column.ColumnName+" >= ?", field)
			fmt.Fprintf(&b, "\tif query.End%s != \"\" { db = db.Where(%q, query.End%s) }\n", field, column.ColumnName+" <= ?", field)
		} else {
			op := map[string]string{"EQ": "=", "NE": "<>", "GT": ">", "GTE": ">=", "LT": "<", "LTE": "<="}[column.QueryType]
			if column.QueryType == "LIKE" {
				fmt.Fprintf(&b, "\tif query.%s != \"\" { db = db.Where(%q, \"%%\"+query.%s+\"%%\") }\n", field, column.ColumnName+" LIKE ?", field)
			} else {
				fmt.Fprintf(&b, "\tif query.%s != \"\" { db = db.Where(%q, query.%s) }\n", field, column.ColumnName+" "+op+" ?", field)
			}
		}
	}
	b.WriteString("\treturn db\n}\n\n")
	fmt.Fprintf(&b, "func Select%sPage(ctx context.Context, query model.%sQuery, pg page.Query) ([]model.%s, int64, error) {\n", class, class, class)
	fmt.Fprintf(&b, "\tdb := %sFilter(DB(ctx).Table(%q), query)\n", lowerFirst(class), tableName)
	b.WriteString("\tvar total int64\n\tif err := db.Count(&total).Error; err != nil { return nil, 0, fmt.Errorf(\"统计数据失败: %w\", err) }\n")
	fmt.Fprintf(&b, "\tif total == 0 { return []model.%s{}, 0, nil }\n\tvar list []model.%s\n", class, class)
	fmt.Fprintf(&b, "\tif err := db.Order(pg.Stable(%q, %q)).Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil { return nil, 0, fmt.Errorf(\"查询数据失败: %%w\", err) }\n", pk.ColumnName, pk.ColumnName)
	b.WriteString("\treturn list, total, nil\n}\n\n")
	fmt.Fprintf(&b, "func Select%sList(ctx context.Context, query model.%sQuery, limit int) ([]model.%s, error) {\n", class, class, class)
	fmt.Fprintf(&b, "\tvar list []model.%s\n\tif err := %sFilter(DB(ctx).Table(%q), query).Order(%q).Limit(limit).Find(&list).Error; err != nil { return nil, fmt.Errorf(\"查询导出数据失败: %%w\", err) }\n\treturn list, nil\n}\n\n", class, lowerFirst(class), tableName, pk.ColumnName)
	if table.TplCategory == "tree" {
		renderTreeParentValidator(&b, table)
	}
	fmt.Fprintf(&b, "func Select%sByID(ctx context.Context, id string) (*model.%s, error) {\n\tvar target model.%s\n", class, class, class)
	fmt.Fprintf(&b, "\terr := DB(ctx).Table(%q).Where(%q, id).Take(&target).Error\n", tableName, pk.ColumnName+" = ?")
	b.WriteString("\tif errors.Is(err, gorm.ErrRecordNotFound) { return nil, nil }\n\tif err != nil { return nil, fmt.Errorf(\"查询详情失败: %w\", err) }\n")
	if table.TplCategory == "sub" && table.SubTable != nil {
		fmt.Fprintf(&b, "\tif err := DB(ctx).Table(%q).Where(%q, target.%s).Order(%q).Find(&target.%s).Error; err != nil { return nil, fmt.Errorf(\"查询子表数据失败: %%w\", err) }\n",
			table.SubTable.TableName, strings.TrimSpace(*table.SubTableFKName)+" = ?", exportedField(pk.JavaField), table.SubTable.PKColumn.ColumnName, subListField(table))
	}
	b.WriteString("\treturn &target, nil\n}\n\n")
	fmt.Fprintf(&b, "func Insert%s(ctx context.Context, target *model.%s) error {\n", class, class)
	if table.TplCategory == "sub" && table.SubTable != nil {
		renderSubInsertRepository(&b, table, "\t", "tx")
	} else {
		fmt.Fprintf(&b, "\tif err := DB(ctx).Table(%q).Create(target).Error; err != nil { return fmt.Errorf(\"新增数据失败: %%w\", err) }\n\treturn nil\n", tableName)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "func Update%s(ctx context.Context, target *model.%s) error {\n\tupdates := map[string]any{\n", class, class)
	for _, column := range table.Columns {
		if column.IsEdit == "1" && column.IsPK != "1" {
			fmt.Fprintf(&b, "\t\t%q: target.%s,\n", column.ColumnName, exportedField(column.JavaField))
		}
	}
	b.WriteString("\t}\n")
	if table.TplCategory == "sub" && table.SubTable != nil {
		fmt.Fprintf(&b, "\treturn Transaction(ctx, func(tx *gorm.DB) error {\n\t\tresult := tx.Table(%q).Where(%q, target.%s).Updates(updates)\n", tableName, pk.ColumnName+" = ?", exportedField(pk.JavaField))
		b.WriteString("\t\tif result.Error != nil { return fmt.Errorf(\"更新数据失败: %w\", result.Error) }\n\t\tif result.RowsAffected == 0 { return fmt.Errorf(\"更新数据失败: 数据不存在\") }\n")
		fmt.Fprintf(&b, "\t\tif err := tx.Table(%q).Where(%q, target.%s).Delete(&model.%s{}).Error; err != nil { return fmt.Errorf(\"清理子表数据失败: %%w\", err) }\n",
			table.SubTable.TableName, strings.TrimSpace(*table.SubTableFKName)+" = ?", exportedField(pk.JavaField), table.SubTable.ClassName)
		renderSubBatchInsert(&b, table, "\t\t", "tx")
		b.WriteString("\t\treturn nil\n\t})\n")
	} else {
		fmt.Fprintf(&b, "\tresult := DB(ctx).Table(%q).Where(%q, target.%s).Updates(updates)\n", tableName, pk.ColumnName+" = ?", exportedField(pk.JavaField))
		b.WriteString("\tif result.Error != nil { return fmt.Errorf(\"更新数据失败: %w\", result.Error) }\n\tif result.RowsAffected == 0 { return fmt.Errorf(\"更新数据失败: 数据不存在\") }\n\treturn nil\n")
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "func Delete%sByIDs(ctx context.Context, ids []string) error {\n", class)
	if table.TplCategory == "sub" && table.SubTable != nil {
		fmt.Fprintf(&b, "\treturn Transaction(ctx, func(tx *gorm.DB) error {\n\t\tif err := tx.Table(%q).Where(%q, ids).Delete(&model.%s{}).Error; err != nil { return fmt.Errorf(\"删除子表数据失败: %%w\", err) }\n",
			table.SubTable.TableName, strings.TrimSpace(*table.SubTableFKName)+" IN ?", table.SubTable.ClassName)
		fmt.Fprintf(&b, "\t\tif err := tx.Table(%q).Where(%q, ids).Delete(&model.%s{}).Error; err != nil { return fmt.Errorf(\"删除数据失败: %%w\", err) }\n\t\treturn nil\n\t})\n", tableName, pk.ColumnName+" IN ?", class)
	} else {
		fmt.Fprintf(&b, "\tif err := DB(ctx).Table(%q).Where(%q, ids).Delete(&model.%s{}).Error; err != nil { return fmt.Errorf(\"删除数据失败: %%w\", err) }\n", tableName, pk.ColumnName+" IN ?", class)
		b.WriteString("\treturn nil\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderSubInsertRepository(b *strings.Builder, table *model.GenTable, indent, dbName string) {
	fmt.Fprintf(b, "%sreturn Transaction(ctx, func(%s *gorm.DB) error {\n", indent, dbName)
	fmt.Fprintf(b, "%s\tif err := %s.Table(%q).Create(target).Error; err != nil { return fmt.Errorf(\"新增数据失败: %%w\", err) }\n", indent, dbName, table.TableName)
	renderSubBatchInsert(b, table, indent+"\t", dbName)
	fmt.Fprintf(b, "%s\treturn nil\n%s})\n", indent, indent)
}

func renderTreeParentValidator(b *strings.Builder, table *model.GenTable) {
	code := findColumnByName(table.Columns, table.TreeCode)
	parent := findColumnByName(table.Columns, table.TreeParentCode)
	valueType := goColumnType(*code)
	zero := zeroValueForColumn(*code)
	fmt.Fprintf(b, "func Validate%sParent(ctx context.Context, currentID, parentID %s) error {\n", table.ClassName, valueType)
	fmt.Fprintf(b, "\tif parentID == %s { return nil }\n", zero)
	fmt.Fprintf(b, "\tvar rows []struct { CodeID %s `gorm:\"column:%s\"`; ParentID %s `gorm:\"column:%s\"` }\n", valueType, code.ColumnName, valueType, parent.ColumnName)
	fmt.Fprintf(b, "\tif err := DB(ctx).Table(%q).Select(%q).Find(&rows).Error; err != nil { return fmt.Errorf(\"校验上级节点失败: %%w\", err) }\n", table.TableName, code.ColumnName+", "+parent.ColumnName)
	fmt.Fprintf(b, "\tparents := make(map[%s]%s, len(rows))\n", valueType, valueType)
	b.WriteString("\tfor _, row := range rows {\n\t\tif _, exists := parents[row.CodeID]; exists { return fmt.Errorf(\"树编码不唯一\") }\n\t\tparents[row.CodeID] = row.ParentID\n\t}\n")
	fmt.Fprintf(b, "\tseen := map[%s]struct{}{currentID: {}}\n\tnext := parentID\n", valueType)
	fmt.Fprintf(b, "\tfor next != %s {\n", zero)
	b.WriteString("\t\tif _, exists := seen[next]; exists { return fmt.Errorf(\"上级节点不能是当前节点或其子节点\") }\n")
	b.WriteString("\t\tseen[next] = struct{}{}\n\t\tif len(seen) > 10000 { return fmt.Errorf(\"树层级超过安全上限或已经存在环\") }\n")
	b.WriteString("\t\tparent, exists := parents[next]\n\t\tif !exists { return fmt.Errorf(\"上级节点不存在\") }\n\t\tnext = parent\n\t}\n\treturn nil\n}\n\n")
}

func renderSubBatchInsert(b *strings.Builder, table *model.GenTable, indent, dbName string) {
	fk := findColumnByName(table.SubTable.Columns, strings.TrimSpace(*table.SubTableFKName))
	fmt.Fprintf(b, "%sfor i := range target.%s { target.%s[i].%s = target.%s }\n",
		indent, subListField(table), subListField(table), exportedField(fk.JavaField), exportedField(table.PKColumn.JavaField))
	fmt.Fprintf(b, "%sif len(target.%s) > 0 { if err := %s.Table(%q).CreateInBatches(target.%s, 100).Error; err != nil { return fmt.Errorf(\"新增子表数据失败: %%w\", err) } }\n",
		indent, subListField(table), dbName, table.SubTable.TableName, subListField(table))
}

func renderServiceSource(table *model.GenTable) string {
	class := table.ClassName
	var b strings.Builder
	b.WriteString("package service\n\nimport (\n\t\"context\"\n\t\"strings\"\n\t\"sync\"\n\n\t\"ruoyi-go/internal/model\"\n\t\"ruoyi-go/internal/repository\"\n\t\"ruoyi-go/pkg/errs\"\n\t\"ruoyi-go/pkg/page\"\n")
	createTimeColumn := findColumn(table.Columns, "createTime")
	updateTimeColumn := findColumn(table.Columns, "updateTime")
	needsTime := createTimeColumn != nil && createTimeColumn.JavaType == "Date" ||
		updateTimeColumn != nil && updateTimeColumn.JavaType == "Date" || goColumnType(*table.PKColumn) == "types.Time"
	if table.SubTable != nil {
		subCreateTime := findColumn(table.SubTable.Columns, "createTime")
		subUpdateTime := findColumn(table.SubTable.Columns, "updateTime")
		needsTime = needsTime || subCreateTime != nil && subCreateTime.JavaType == "Date" ||
			subUpdateTime != nil && subUpdateTime.JavaType == "Date"
	}
	if needsTime {
		b.WriteString("\t\"ruoyi-go/pkg/types\"\n")
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "var %sWriteMu sync.Mutex\n\n", lowerFirst(class))
	fmt.Fprintf(&b, "func List%sPage(ctx context.Context, query model.%sQuery, pg page.Query) ([]model.%s, int64, error) { return repository.Select%sPage(ctx, query, pg) }\n\n", class, class, class, class)
	fmt.Fprintf(&b, "func List%sExport(ctx context.Context, query model.%sQuery) ([]model.%s, error) {\n\tlist, err := repository.Select%sList(ctx, query, MaxExportRows+1)\n\tif err != nil { return nil, err }\n\tif err := checkExportSize(len(list)); err != nil { return nil, err }\n\treturn list, nil\n}\n\n", class, class, class, class)
	fmt.Fprintf(&b, "func Get%s(ctx context.Context, id string) (*model.%s, error) {\n\ttarget, err := repository.Select%sByID(ctx, id)\n\tif err != nil { return nil, err }\n\tif target == nil { return nil, errs.New(\"数据不存在\") }\n\treturn target, nil\n}\n\n", class, class, class)
	fmt.Fprintf(&b, "func Create%s(ctx context.Context, target *model.%s, operator string) error {\n\t%sWriteMu.Lock()\n\tdefer %sWriteMu.Unlock()\n", class, class, lowerFirst(class), lowerFirst(class))
	if table.PKColumn.IsIncrement == "1" {
		fmt.Fprintf(&b, "\ttarget.%s = %s\n", exportedField(table.PKColumn.JavaField), zeroValueForColumn(*table.PKColumn))
	} else {
		fmt.Fprintf(&b, "\tif target.%s == %s { return errs.New(\"主键不能为空\") }\n", exportedField(table.PKColumn.JavaField), zeroValueForColumn(*table.PKColumn))
	}
	if table.TplCategory == "tree" {
		code := findColumnByName(table.Columns, table.TreeCode)
		parent := findColumnByName(table.Columns, table.TreeParentCode)
		fmt.Fprintf(&b, "\tif err := repository.Validate%sParent(ctx, target.%s, target.%s); err != nil { return err }\n", class, exportedField(code.JavaField), exportedField(parent.JavaField))
	}
	writeAuditAssignments(&b, table, "Create", "operator")
	writeSubAuditAssignments(&b, table, "operator")
	fmt.Fprintf(&b, "\treturn repository.Insert%s(ctx, target)\n}\n\n", class)
	fmt.Fprintf(&b, "func Update%s(ctx context.Context, target *model.%s, operator string) error {\n\t%sWriteMu.Lock()\n\tdefer %sWriteMu.Unlock()\n", class, class, lowerFirst(class), lowerFirst(class))
	fmt.Fprintf(&b, "\tif target.%s == %s { return errs.New(\"主键不能为空\") }\n", exportedField(table.PKColumn.JavaField), zeroValueForColumn(*table.PKColumn))
	if table.TplCategory == "tree" {
		code := findColumnByName(table.Columns, table.TreeCode)
		parent := findColumnByName(table.Columns, table.TreeParentCode)
		fmt.Fprintf(&b, "\tif target.%s == %s { return errs.New(\"树编码不能为空\") }\n", exportedField(code.JavaField), zeroValueForColumn(*code))
		fmt.Fprintf(&b, "\tif err := repository.Validate%sParent(ctx, target.%s, target.%s); err != nil { return err }\n", class, exportedField(code.JavaField), exportedField(parent.JavaField))
	}
	writeAuditAssignments(&b, table, "Update", "operator")
	writeSubAuditAssignments(&b, table, "operator")
	fmt.Fprintf(&b, "\treturn repository.Update%s(ctx, target)\n}\n\n", class)
	fmt.Fprintf(&b, "func Delete%s(ctx context.Context, ids []string) error {\n\t%sWriteMu.Lock()\n\tdefer %sWriteMu.Unlock()\n\tif len(ids) == 0 || len(ids) > 200 { return errs.New(\"删除参数数量不合法\") }\n\tclean := make([]string, 0, len(ids))\n\tseen := make(map[string]struct{}, len(ids))\n\tfor _, raw := range ids { id := strings.TrimSpace(raw); if id == \"\" || len(id) > 128 { return errs.New(\"删除参数格式不合法\") }; if _, ok := seen[id]; !ok { seen[id] = struct{}{}; clean = append(clean, id) } }\n\treturn repository.Delete%sByIDs(ctx, clean)\n}\n", class, lowerFirst(class), lowerFirst(class), class)
	return b.String()
}

func renderHandlerSource(table *model.GenTable) string {
	class := table.ClassName
	var b strings.Builder
	b.WriteString("package handler\n\nimport (\n\t\"strings\"\n\n\t\"github.com/gin-gonic/gin\"\n\n\t\"ruoyi-go/internal/model\"\n\t\"ruoyi-go/internal/service\"\n\t\"ruoyi-go/pkg/excelx\"\n\t\"ruoyi-go/pkg/page\"\n\t\"ruoyi-go/pkg/response\"\n)\n\n")
	fmt.Fprintf(&b, "func %sList(c *gin.Context) {\n\tvar query model.%sQuery\n\tif err := c.ShouldBindQuery(&query); err != nil { response.Fail(c, \"查询参数错误\"); return }\n", class, class)
	fmt.Fprintf(&b, "\tlist, total, err := service.List%sPage(c.Request.Context(), query, page.Parse(c, model.%sSortColumns))\n\tif err != nil { fail(c, err); return }\n\tresponse.Page(c, list, total)\n}\n\n", class, class)
	fmt.Fprintf(&b, "func %sExport(c *gin.Context) {\n\tvar query model.%sQuery\n\tif err := c.ShouldBind(&query); err != nil { response.FailDownload(c, response.CodeError, bindMessage(err)); return }\n\tlist, err := service.List%sExport(c.Request.Context(), query)\n\tif err != nil { failDownload(c, err); return }\n\tif err := excelx.WriteResponse(c, %q, %q, list); err != nil { failDownload(c, err) }\n}\n\n", class, class, class, safeComment(table.FunctionName)+"数据.xlsx", safeComment(table.FunctionName)+"数据")
	fmt.Fprintf(&b, "func %sGet(c *gin.Context) {\n\tid := strings.TrimSpace(c.Param(\"id\"))\n\tif id == \"\" { response.Fail(c, \"参数格式错误\"); return }\n\ttarget, err := service.Get%s(c.Request.Context(), id)\n\tif err != nil { fail(c, err); return }\n\tresponse.OkData(c, target)\n}\n\n", class, class)
	fmt.Fprintf(&b, "func %sAdd(c *gin.Context) {\n\tvar target model.%s\n\tif err := c.ShouldBindJSON(&target); err != nil { response.Fail(c, bindMessage(err)); return }\n\tif err := service.Create%s(c.Request.Context(), &target, currentUsername(c)); err != nil { fail(c, err); return }\n\tresponse.Ok(c)\n}\n\n", class, class, class)
	fmt.Fprintf(&b, "func %sEdit(c *gin.Context) {\n\tvar target model.%s\n\tif err := c.ShouldBindJSON(&target); err != nil { response.Fail(c, bindMessage(err)); return }\n\tif err := service.Update%s(c.Request.Context(), &target, currentUsername(c)); err != nil { fail(c, err); return }\n\tresponse.Ok(c)\n}\n\n", class, class, class)
	fmt.Fprintf(&b, "func %sRemove(c *gin.Context) {\n\traw := strings.Split(c.Param(\"ids\"), \",\")\n\tif len(raw) == 0 || len(raw) > 200 { response.Fail(c, \"参数格式错误\"); return }\n\tids := make([]string, 0, len(raw))\n\tfor _, item := range raw { if id := strings.TrimSpace(item); id != \"\" { ids = append(ids, id) } }\n\tif err := service.Delete%s(c.Request.Context(), ids); err != nil { fail(c, err); return }\n\tresponse.Ok(c)\n}\n", class, class)
	return b.String()
}

func renderRouterSource(table *model.GenTable) string {
	class := table.ClassName
	module := strings.ToLower(table.ModuleName)
	business := strings.ToLower(table.BusinessName)
	permission := module + ":" + business
	var b strings.Builder
	b.WriteString("package router\n\nimport (\n\t\"github.com/gin-gonic/gin\"\n\n\t\"ruoyi-go/internal/handler\"\n\t\"ruoyi-go/internal/middleware\"\n\t\"ruoyi-go/internal/model\"\n)\n\n")
	fmt.Fprintf(&b, "func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, register%sGenerated) }\n\n", class)
	fmt.Fprintf(&b, "func register%sGenerated(g *gin.RouterGroup) {\n\tgroup := g.Group(%q)\n", class, "/"+module+"/"+business)
	fmt.Fprintf(&b, "\tgroup.GET(\"/list\", middleware.HasPermission(%q), handler.%sList)\n", permission+":list", class)
	fmt.Fprintf(&b, "\tgroup.POST(\"/export\", middleware.ExportLimit(), middleware.HasPermission(%q), middleware.OperLog(%q, model.BusinessTypeExport), handler.%sExport)\n", permission+":export", table.FunctionName, class)
	fmt.Fprintf(&b, "\tgroup.GET(\"/:id\", middleware.HasPermission(%q), handler.%sGet)\n", permission+":query", class)
	fmt.Fprintf(&b, "\tgroup.POST(\"\", noRepeat(), middleware.HasPermission(%q), middleware.OperLog(%q, model.BusinessTypeInsert), handler.%sAdd)\n", permission+":add", table.FunctionName, class)
	fmt.Fprintf(&b, "\tgroup.PUT(\"\", middleware.HasPermission(%q), middleware.OperLog(%q, model.BusinessTypeUpdate), handler.%sEdit)\n", permission+":edit", table.FunctionName, class)
	fmt.Fprintf(&b, "\tgroup.DELETE(\"/:ids\", middleware.HasPermission(%q), middleware.OperLog(%q, model.BusinessTypeDelete), handler.%sRemove)\n", permission+":remove", table.FunctionName, class)
	b.WriteString("}\n")
	return b.String()
}

func renderAPISource(table *model.GenTable) string {
	module := strings.ToLower(table.ModuleName)
	business := strings.ToLower(table.BusinessName)
	class := table.ClassName
	base := "/" + module + "/" + business
	var b strings.Builder
	b.WriteString("import request from '@/utils/request'\n\n")
	queryType, idType, dataType := "", "", ""
	if table.TplWebType == "element-plus-typescript" {
		queryType, idType, dataType = ": Record<string, unknown>", ": string | number | Array<string | number>", ": Record<string, unknown>"
	}
	fmt.Fprintf(&b, "export function list%s(query%s) { return request({ url: %q + '/list', method: 'get', params: query }) }\n", class, queryType, base)
	fmt.Fprintf(&b, "export function get%s(id%s) { return request({ url: %q + '/' + id, method: 'get' }) }\n", class, idType, base)
	fmt.Fprintf(&b, "export function add%s(data%s) { return request({ url: %q, method: 'post', data }) }\n", class, dataType, base)
	fmt.Fprintf(&b, "export function update%s(data%s) { return request({ url: %q, method: 'put', data }) }\n", class, dataType, base)
	fmt.Fprintf(&b, "export function del%s(ids%s) { return request({ url: %q + '/' + ids, method: 'delete' }) }\n", class, idType, base)
	return b.String()
}

func renderVueSource(table *model.GenTable) string {
	if table.TplWebType == "element-ui" {
		return renderVue2Source(table)
	}
	class := table.ClassName
	module := strings.ToLower(table.ModuleName)
	business := strings.ToLower(table.BusinessName)
	pk := table.PKColumn
	var b strings.Builder
	b.WriteString("<template>\n  <div class=\"app-container\">\n")
	queries := selectedColumns(table.Columns, func(c model.GenTableColumn) bool { return c.IsQuery == "1" })
	if len(queries) > 0 {
		b.WriteString("    <el-form :model=\"queryParams\" ref=\"queryRef\" :inline=\"true\">\n")
		for _, column := range queries {
			fmt.Fprintf(&b, "      <el-form-item label=%q prop=%q>", safeComment(column.ColumnComment), column.JavaField)
			b.WriteString(renderVueControl(column, "queryParams."+column.JavaField, true))
			b.WriteString("</el-form-item>\n")
		}
		b.WriteString("      <el-form-item><el-button type=\"primary\" icon=\"Search\" @click=\"handleQuery\">搜索</el-button><el-button icon=\"Refresh\" @click=\"resetQuery\">重置</el-button></el-form-item>\n    </el-form>\n")
	}
	fmt.Fprintf(&b, "    <el-row :gutter=\"10\" class=\"mb8\"><el-col :span=\"1.5\"><el-button type=\"primary\" plain icon=\"Plus\" @click=\"handleAdd()\" v-hasPermi=\"['%s:%s:add']\">新增</el-button></el-col><el-col :span=\"1.5\"><el-button type=\"danger\" plain icon=\"Delete\" :disabled=\"multiple\" @click=\"handleDelete()\" v-hasPermi=\"['%s:%s:remove']\">删除</el-button></el-col><el-col :span=\"1.5\"><el-button type=\"warning\" plain icon=\"Download\" @click=\"handleExport\" v-hasPermi=\"['%s:%s:export']\">导出</el-button></el-col></el-row>\n", module, business, module, business, module, business)
	b.WriteString("    <el-table v-loading=\"loading\" :data=\"rows\" @selection-change=\"handleSelectionChange\"")
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, " row-key=%q :tree-props=\"{ children: 'children' }\"", lowerCamel(table.TreeCode))
	}
	b.WriteString("><el-table-column type=\"selection\" width=\"55\" align=\"center\" />\n")
	for _, column := range selectedColumns(table.Columns, func(c model.GenTableColumn) bool { return c.IsList == "1" || c.IsPK == "1" }) {
		if dict := columnDict(column); dict != "" {
			fmt.Fprintf(&b, "      <el-table-column label=%q prop=%q align=\"center\"><template #default=\"scope\"><dict-tag :options=\"%s\" :value=\"scope.row.%s\" /></template></el-table-column>\n", safeComment(column.ColumnComment), column.JavaField, dict, column.JavaField)
		} else {
			fmt.Fprintf(&b, "      <el-table-column label=%q prop=%q align=\"center\" :show-overflow-tooltip=\"true\" />\n", safeComment(column.ColumnComment), column.JavaField)
		}
	}
	fmt.Fprintf(&b, "      <el-table-column label=\"操作\" align=\"center\" width=\"200\"><template #default=\"scope\"><el-button link type=\"primary\" icon=\"Edit\" @click=\"handleUpdate(scope.row)\" v-hasPermi=\"['%s:%s:edit']\">修改</el-button>", module, business)
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "<el-button link type=\"primary\" icon=\"Plus\" @click=\"handleAdd(scope.row)\" v-hasPermi=\"['%s:%s:add']\">新增</el-button>", module, business)
	}
	fmt.Fprintf(&b, "<el-button link type=\"primary\" icon=\"Delete\" @click=\"handleDelete(scope.row)\" v-hasPermi=\"['%s:%s:remove']\">删除</el-button></template></el-table-column>\n    </el-table>\n", module, business)
	if table.TplCategory != "tree" {
		b.WriteString("    <pagination v-show=\"total > 0\" :total=\"total\" v-model:page=\"queryParams.pageNum\" v-model:limit=\"queryParams.pageSize\" @pagination=\"getList\" />\n")
	}
	b.WriteString("    <el-dialog :title=\"title\" v-model=\"open\" width=\"600px\" append-to-body><el-form ref=\"formRef\" :model=\"form\" :rules=\"rules\" label-width=\"100px\">\n")
	if table.TplCategory == "tree" {
		parentField := lowerCamel(table.TreeParentCode)
		codeField := lowerCamel(table.TreeCode)
		nameField := lowerCamel(table.TreeName)
		fmt.Fprintf(&b, "      <el-form-item label=\"上级节点\" prop=%q><el-tree-select v-model=\"form.%s\" :data=\"treeOptions\" :props=\"{ value: '%s', label: '%s', children: 'children' }\" value-key=%q placeholder=\"选择上级节点\" check-strictly /></el-form-item>\n", parentField, parentField, codeField, nameField, codeField)
	}
	for _, column := range selectedColumns(table.Columns, func(c model.GenTableColumn) bool { return c.IsInsert == "1" || c.IsEdit == "1" }) {
		if column.IsPK == "1" && column.IsIncrement == "1" {
			continue
		}
		if table.TplCategory == "tree" && strings.EqualFold(column.ColumnName, table.TreeParentCode) {
			continue
		}
		fmt.Fprintf(&b, "      <el-form-item label=%q prop=%q>", safeComment(column.ColumnComment), column.JavaField)
		b.WriteString(renderVueControl(column, "form."+column.JavaField, false))
		b.WriteString("</el-form-item>\n")
	}
	if table.TplCategory == "sub" && table.SubTable != nil {
		listField := lowerFirst(table.SubTable.ClassName) + "List"
		fmt.Fprintf(&b, "      <el-divider content-position=\"center\">%s信息</el-divider><el-button type=\"primary\" link @click=\"addSubRow\">添加一行</el-button>\n", safeComment(table.SubTable.FunctionName))
		fmt.Fprintf(&b, "      <el-table :data=\"form.%s || []\"><el-table-column type=\"index\" width=\"50\" />\n", listField)
		for _, column := range editableSubColumns(table) {
			fmt.Fprintf(&b, "        <el-table-column label=%q><template #default=\"scope\">%s</template></el-table-column>\n",
				safeComment(column.ColumnComment), renderVueControl(column, "scope.row."+column.JavaField, false))
		}
		b.WriteString("        <el-table-column label=\"操作\" width=\"70\"><template #default=\"scope\"><el-button link type=\"danger\" @click=\"removeSubRow(scope.$index)\">删除</el-button></template></el-table-column></el-table>\n")
	}
	b.WriteString("    </el-form><template #footer><el-button type=\"primary\" @click=\"submitForm\">确定</el-button><el-button @click=\"open = false\">取消</el-button></template></el-dialog>\n  </div>\n</template>\n\n<script setup name=\"")
	b.WriteString(class)
	if table.TplWebType == "element-plus-typescript" {
		b.WriteString("\" lang=\"ts\">\n")
	} else {
		b.WriteString("\">\n")
	}
	if table.TplWebType == "element-plus-typescript" {
		b.WriteString("import type { ComponentInternalInstance } from 'vue'\n")
	}
	fmt.Fprintf(&b, "import { list%s, get%s, add%s, update%s, del%s } from '@/api/%s/%s'\n", class, class, class, class, class, module, business)
	if table.TplWebType == "element-plus-typescript" {
		b.WriteString("const { proxy } = getCurrentInstance() as ComponentInternalInstance\nconst formRef = ref()\nconst loading = ref(false)\nconst rows = ref<Record<string, any>[]>([])\nconst total = ref(0)\nconst open = ref(false)\nconst title = ref('')\nconst ids = ref<Array<string | number>>([])\nconst multiple = ref(true)\nconst form = ref<Record<string, any>>({})\nconst queryParams = reactive<Record<string, any>>({ pageNum: 1, pageSize: 10 })\n")
	} else {
		b.WriteString("const { proxy } = getCurrentInstance()\nconst formRef = ref()\nconst loading = ref(false)\nconst rows = ref([])\nconst total = ref(0)\nconst open = ref(false)\nconst title = ref('')\nconst ids = ref([])\nconst multiple = ref(true)\nconst form = ref({})\nconst queryParams = reactive({ pageNum: 1, pageSize: 10 })\n")
	}
	if table.TplCategory == "tree" {
		if table.TplWebType == "element-plus-typescript" {
			b.WriteString("const treeOptions = ref<Array<Record<string, any>>>([])\n")
		} else {
			b.WriteString("const treeOptions = ref([])\n")
		}
	}
	dictColumns := append([]model.GenTableColumn(nil), table.Columns...)
	if table.SubTable != nil {
		dictColumns = append(dictColumns, table.SubTable.Columns...)
	}
	dicts := tableDicts(dictColumns)
	if len(dicts) > 0 {
		fmt.Fprintf(&b, "const { %s } = proxy.useDict(%s)\n", strings.Join(dicts, ", "), quoteJSArgs(dicts))
	}
	b.WriteString("const rules = reactive({\n")
	for _, column := range table.Columns {
		if column.IsRequired == "1" && (column.IsInsert == "1" || column.IsEdit == "1") {
			fmt.Fprintf(&b, "  %s: [{ required: true, message: %q, trigger: 'blur' }],\n", column.JavaField, safeComment(column.ColumnComment)+"不能为空")
		}
	}
	b.WriteString("})\n")
	if table.TplCategory == "tree" {
		parentField := lowerCamel(table.TreeParentCode)
		allType := ""
		if table.TplWebType == "element-plus-typescript" {
			allType = ": Array<Record<string, any>>"
		}
		fmt.Fprintf(&b, "async function getList() { loading.value = true; try { let pageNum = 1; let all%s = []; let count = 0; do { const res = await list%s({ ...queryParams, pageNum, pageSize: 100 }); if (!res.rows.length) break; all.push(...res.rows); count = res.total; pageNum++ } while (all.length < count); rows.value = proxy.handleTree(all, '%s', '%s'); total.value = count } finally { loading.value = false } }\n", allType, class, lowerCamel(table.TreeCode), parentField)
	} else {
		fmt.Fprintf(&b, "async function getList() { loading.value = true; try { const res = await list%s(queryParams); rows.value = res.rows; total.value = res.total } finally { loading.value = false } }\n", class)
	}
	selectionType, rowType, indexType := "", "", ""
	if table.TplWebType == "element-plus-typescript" {
		selectionType, rowType, indexType = ": Array<Record<string, any>>", ": Record<string, any>", ": number"
	}
	if table.TplCategory == "tree" {
		codeField := lowerCamel(table.TreeCode)
		nameField := lowerCamel(table.TreeName)
		parentField := lowerCamel(table.TreeParentCode)
		if table.TplWebType == "element-plus-typescript" {
			fmt.Fprintf(&b, "function makeTreeOptions(source: Array<Record<string, any>>, excludeId: string | number | null = null) { const clone = (nodes: Array<Record<string, any>>): Array<Record<string, any>> => nodes.filter(node => node.%s !== excludeId).map(node => ({ ...node, children: clone(node.children || []) })); return [{ %s: 0, %s: '顶级节点', children: clone(source) }] }\n", codeField, codeField, nameField)
		} else {
			fmt.Fprintf(&b, "function makeTreeOptions(source, excludeId = null) { const clone = nodes => nodes.filter(node => node.%s !== excludeId).map(node => ({ ...node, children: clone(node.children || []) })); return [{ %s: 0, %s: '顶级节点', children: clone(source) }] }\n", codeField, codeField, nameField)
		}
		allType := ""
		excludeParam := "excludeId = null"
		if table.TplWebType == "element-plus-typescript" {
			allType = ": Array<Record<string, any>>"
			excludeParam = "excludeId: string | number | null = null"
		}
		fmt.Fprintf(&b, "async function loadTreeOptions(%s) { let pageNum = 1; let all%s = []; let count = 0; do { const res = await list%s({ pageNum, pageSize: 100 }); if (!res.rows.length) break; all.push(...res.rows); count = res.total; pageNum++ } while (all.length < count); treeOptions.value = makeTreeOptions(proxy.handleTree(all, '%s', '%s'), excludeId) }\n", excludeParam, allType, class, codeField, parentField)
	}
	fmt.Fprintf(&b, "function handleQuery() { queryParams.pageNum = 1; getList() }\nfunction resetQuery() { proxy.resetForm('queryRef'); handleQuery() }\nfunction handleSelectionChange(selection%s) { ids.value = selection.map(item => item.", selectionType)
	b.WriteString(pk.JavaField)
	b.WriteString("); multiple.value = !selection.length }\n")
	if table.TplCategory == "tree" {
		addSignature := "row = {}"
		if rowType != "" {
			addSignature = "row" + rowType + " = {}"
		}
		fmt.Fprintf(&b, "async function handleAdd(%s) { form.value = { %s: row.%s ?? 0 }; await loadTreeOptions(); title.value = '新增%s'; open.value = true }\n", addSignature, lowerCamel(table.TreeParentCode), lowerCamel(table.TreeCode), safeJSString(table.FunctionName))
	} else {
		b.WriteString("function handleAdd() { form.value = ")
		if table.TplCategory == "sub" && table.SubTable != nil {
			fmt.Fprintf(&b, "{ %s: [] }", lowerFirst(table.SubTable.ClassName)+"List")
		} else {
			b.WriteString("{}")
		}
		b.WriteString("; title.value = '新增")
		b.WriteString(safeJSString(table.FunctionName))
		b.WriteString("'; open.value = true }\n")
	}
	if table.TplCategory == "sub" && table.SubTable != nil {
		listField := lowerFirst(table.SubTable.ClassName) + "List"
		fmt.Fprintf(&b, "function addSubRow() { if (!form.value.%s) form.value.%s = []; form.value.%s.push({}) }\nfunction removeSubRow(index%s) { form.value.%s.splice(index, 1) }\n", listField, listField, listField, indexType, listField)
	}
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "async function handleUpdate(row%s) { await loadTreeOptions(row.%s); const res = await get%s(row.%s); form.value = res.data; title.value = '修改%s'; open.value = true }\n", rowType, lowerCamel(table.TreeCode), class, pk.JavaField, safeJSString(table.FunctionName))
	} else {
		fmt.Fprintf(&b, "async function handleUpdate(row%s) { const res = await get%s(row.%s); form.value = res.data; title.value = '修改%s'; open.value = true }\n", rowType, class, pk.JavaField, safeJSString(table.FunctionName))
	}
	fmt.Fprintf(&b, "async function submitForm() { await formRef.value.validate(); if (form.value.%s != null) await update%s(form.value); else await add%s(form.value); proxy.$modal.msgSuccess('操作成功'); open.value = false; getList() }\n", pk.JavaField, class, class)
	deleteSignature := "row = {}"
	if rowType != "" {
		deleteSignature = "row" + rowType + " = {}"
	}
	fmt.Fprintf(&b, "async function handleDelete(%s) { const target = row.%s ?? ids.value; await proxy.$modal.confirm('确认删除选中数据？'); await del%s(target); proxy.$modal.msgSuccess('删除成功'); getList() }\n", deleteSignature, pk.JavaField, class)
	fmt.Fprintf(&b, "function handleExport() { proxy.download('%s/%s/export', { ...queryParams }, '%s_' + new Date().getTime() + '.xlsx') }\n", module, business, business)
	b.WriteString("getList()\n</script>\n")
	return b.String()
}

func renderVue2Source(table *model.GenTable) string {
	class := table.ClassName
	module := strings.ToLower(table.ModuleName)
	business := strings.ToLower(table.BusinessName)
	pk := table.PKColumn.JavaField
	var b strings.Builder
	b.WriteString("<template>\n  <div class=\"app-container\">\n    <el-form :model=\"queryParams\" ref=\"queryForm\" :inline=\"true\">\n")
	for _, column := range selectedColumns(table.Columns, func(c model.GenTableColumn) bool { return c.IsQuery == "1" }) {
		fmt.Fprintf(&b, "      <el-form-item label=%q prop=%q><el-input v-model=\"queryParams.%s\" clearable @keyup.enter.native=\"handleQuery\" /></el-form-item>\n", safeComment(column.ColumnComment), column.JavaField, column.JavaField)
	}
	fmt.Fprintf(&b, "      <el-form-item><el-button type=\"primary\" icon=\"el-icon-search\" @click=\"handleQuery\">搜索</el-button><el-button icon=\"el-icon-refresh\" @click=\"resetQuery\">重置</el-button></el-form-item>\n    </el-form>\n    <el-button type=\"primary\" plain icon=\"el-icon-plus\" @click=\"handleAdd()\">新增</el-button><el-button type=\"warning\" plain icon=\"el-icon-download\" @click=\"handleExport\" v-hasPermi=\"['%s:%s:export']\">导出</el-button>\n    <el-table v-loading=\"loading\" :data=\"rows\"", module, business)
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, " row-key=%q :tree-props=\"{ children: 'children' }\"", lowerCamel(table.TreeCode))
	}
	b.WriteString(">\n")
	for _, column := range selectedColumns(table.Columns, func(c model.GenTableColumn) bool { return c.IsList == "1" || c.IsPK == "1" }) {
		fmt.Fprintf(&b, "      <el-table-column label=%q prop=%q align=\"center\" />\n", safeComment(column.ColumnComment), column.JavaField)
	}
	fmt.Fprintf(&b, "      <el-table-column label=\"操作\" align=\"center\"><template slot-scope=\"scope\"><el-button type=\"text\" @click=\"handleUpdate(scope.row)\" v-hasPermi=\"['%s:%s:edit']\">修改</el-button>", module, business)
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "<el-button type=\"text\" @click=\"handleAdd(scope.row)\" v-hasPermi=\"['%s:%s:add']\">新增</el-button>", module, business)
	}
	fmt.Fprintf(&b, "<el-button type=\"text\" @click=\"handleDelete(scope.row)\" v-hasPermi=\"['%s:%s:remove']\">删除</el-button></template></el-table-column>\n    </el-table>\n", module, business)
	if table.TplCategory != "tree" {
		b.WriteString("    <pagination v-show=\"total > 0\" :total=\"total\" :page.sync=\"queryParams.pageNum\" :limit.sync=\"queryParams.pageSize\" @pagination=\"getList\" />\n")
	}
	b.WriteString("    <el-dialog :title=\"title\" :visible.sync=\"open\" width=\"600px\"><el-form ref=\"form\" :model=\"form\" label-width=\"100px\">\n")
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "      <el-form-item label=\"上级节点\" prop=%q><el-cascader v-model=\"form.%s\" :options=\"treeOptions\" :props=\"{ value: '%s', label: '%s', children: 'children', emitPath: false, checkStrictly: true }\" clearable /></el-form-item>\n", lowerCamel(table.TreeParentCode), lowerCamel(table.TreeParentCode), lowerCamel(table.TreeCode), lowerCamel(table.TreeName))
	}
	for _, column := range selectedColumns(table.Columns, func(c model.GenTableColumn) bool { return c.IsInsert == "1" || c.IsEdit == "1" }) {
		if column.IsPK == "1" && column.IsIncrement == "1" {
			continue
		}
		if table.TplCategory == "tree" && strings.EqualFold(column.ColumnName, table.TreeParentCode) {
			continue
		}
		fmt.Fprintf(&b, "      <el-form-item label=%q prop=%q><el-input v-model=\"form.%s\" /></el-form-item>\n", safeComment(column.ColumnComment), column.JavaField, column.JavaField)
	}
	if table.TplCategory == "sub" && table.SubTable != nil {
		listField := lowerFirst(table.SubTable.ClassName) + "List"
		fmt.Fprintf(&b, "      <el-divider content-position=\"center\">%s信息</el-divider><el-button type=\"text\" @click=\"addSubRow\">添加一行</el-button>\n      <el-table :data=\"form.%s || []\"><el-table-column type=\"index\" width=\"50\" />\n", safeComment(table.SubTable.FunctionName), listField)
		for _, column := range editableSubColumns(table) {
			fmt.Fprintf(&b, "        <el-table-column label=%q><template slot-scope=\"scope\"><el-input v-model=\"scope.row.%s\" /></template></el-table-column>\n", safeComment(column.ColumnComment), column.JavaField)
		}
		b.WriteString("        <el-table-column label=\"操作\" width=\"70\"><template slot-scope=\"scope\"><el-button type=\"text\" @click=\"removeSubRow(scope.$index)\">删除</el-button></template></el-table-column></el-table>\n")
	}
	b.WriteString("    </el-form><div slot=\"footer\"><el-button type=\"primary\" @click=\"submitForm\">确定</el-button><el-button @click=\"open = false\">取消</el-button></div></el-dialog>\n  </div>\n</template>\n\n<script>\n")
	fmt.Fprintf(&b, "import { list%s, get%s, add%s, update%s, del%s } from '@/api/%s/%s'\n", class, class, class, class, class, module, business)
	b.WriteString("export default { name: '")
	b.WriteString(class)
	b.WriteString("', data() { return { loading: false, rows: [], treeOptions: [], total: 0, open: false, title: '', form: {}, queryParams: { pageNum: 1, pageSize: 10 } } }, created() { this.getList() }, methods: {\n")
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "async getList() { this.loading = true; try { let pageNum = 1; let all = []; let count = 0; do { const res = await list%s({ ...this.queryParams, pageNum, pageSize: 100 }); if (!res.rows.length) break; all.push(...res.rows); count = res.total; pageNum++ } while (all.length < count); this.rows = this.handleTree(all, '%s', '%s'); this.total = count } finally { this.loading = false } },\n", class, lowerCamel(table.TreeCode), lowerCamel(table.TreeParentCode))
	} else {
		fmt.Fprintf(&b, "async getList() { this.loading = true; try { const res = await list%s(this.queryParams); this.rows = res.rows; this.total = res.total } finally { this.loading = false } },\n", class)
	}
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "makeTreeOptions(source, excludeId = null) { const clone = nodes => nodes.filter(node => node.%s !== excludeId).map(node => ({ ...node, children: clone(node.children || []) })); return [{ %s: 0, %s: '顶级节点', children: clone(source) }] },\n", lowerCamel(table.TreeCode), lowerCamel(table.TreeCode), lowerCamel(table.TreeName))
		fmt.Fprintf(&b, "async loadTreeOptions(excludeId = null) { let pageNum = 1; let all = []; let count = 0; do { const res = await list%s({ pageNum, pageSize: 100 }); if (!res.rows.length) break; all.push(...res.rows); count = res.total; pageNum++ } while (all.length < count); this.treeOptions = this.makeTreeOptions(this.handleTree(all, '%s', '%s'), excludeId) },\n", class, lowerCamel(table.TreeCode), lowerCamel(table.TreeParentCode))
	}
	b.WriteString("handleQuery() { this.queryParams.pageNum = 1; this.getList() }, resetQuery() { this.resetForm('queryForm'); this.handleQuery() }, ")
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "async handleAdd(row = {}) { this.form = { %s: row.%s || 0 }; await this.loadTreeOptions(); this.title = '新增%s'; this.open = true },\n", lowerCamel(table.TreeParentCode), lowerCamel(table.TreeCode), safeJSString(table.FunctionName))
	} else {
		b.WriteString("handleAdd() { this.form = ")
		if table.TplCategory == "sub" && table.SubTable != nil {
			fmt.Fprintf(&b, "{ %s: [] }", lowerFirst(table.SubTable.ClassName)+"List")
		} else {
			b.WriteString("{}")
		}
		b.WriteString("; this.title = '新增")
		b.WriteString(safeJSString(table.FunctionName))
		b.WriteString("'; this.open = true },\n")
	}
	if table.TplCategory == "sub" && table.SubTable != nil {
		listField := lowerFirst(table.SubTable.ClassName) + "List"
		fmt.Fprintf(&b, "addSubRow() { if (!this.form.%s) this.$set(this.form, '%s', []); this.form.%s.push({}) }, removeSubRow(index) { this.form.%s.splice(index, 1) },\n", listField, listField, listField, listField)
	}
	if table.TplCategory == "tree" {
		fmt.Fprintf(&b, "async handleUpdate(row) { await this.loadTreeOptions(row.%s); const res = await get%s(row.%s); this.form = res.data; this.title = '修改%s'; this.open = true },\n", lowerCamel(table.TreeCode), class, pk, safeJSString(table.FunctionName))
	} else {
		fmt.Fprintf(&b, "async handleUpdate(row) { const res = await get%s(row.%s); this.form = res.data; this.title = '修改%s'; this.open = true },\n", class, pk, safeJSString(table.FunctionName))
	}
	fmt.Fprintf(&b, "async submitForm() { if (this.form.%s != null) await update%s(this.form); else await add%s(this.form); this.$modal.msgSuccess('操作成功'); this.open = false; this.getList() },\n", pk, class, class)
	fmt.Fprintf(&b, "async handleDelete(row) { await this.$modal.confirm('确认删除该数据？'); await del%s(row.%s); this.$modal.msgSuccess('删除成功'); this.getList() },\n", class, pk)
	fmt.Fprintf(&b, "handleExport() { this.download('%s/%s/export', { ...this.queryParams }, '%s_' + new Date().getTime() + '.xlsx') }\n", module, business, business)
	b.WriteString("} }\n</script>\n")
	return b.String()
}

func renderVueControl(column model.GenTableColumn, target string, query bool) string {
	dict := columnDict(column)
	clearable := ""
	if query {
		clearable = " clearable"
	}
	switch column.HTMLType {
	case "textarea":
		return fmt.Sprintf(`<el-input v-model="%s" type="textarea"%s />`, target, clearable)
	case "datetime":
		return fmt.Sprintf(`<el-date-picker v-model="%s" type="datetime" value-format="YYYY-MM-DD HH:mm:ss"%s />`, target, clearable)
	case "imageUpload":
		return fmt.Sprintf(`<image-upload v-model="%s" />`, target)
	case "fileUpload":
		return fmt.Sprintf(`<file-upload v-model="%s" />`, target)
	case "editor":
		return fmt.Sprintf(`<editor v-model="%s" :min-height="192" />`, target)
	case "select":
		if dict != "" {
			return fmt.Sprintf(`<el-select v-model="%s"%s><el-option v-for="dict in %s" :key="dict.value" :label="dict.label" :value="dict.value" /></el-select>`, target, clearable, dict)
		}
	case "radio":
		if dict != "" {
			return fmt.Sprintf(`<el-radio-group v-model="%s"><el-radio v-for="dict in %s" :key="dict.value" :value="dict.value">{{ dict.label }}</el-radio></el-radio-group>`, target, dict)
		}
	case "checkbox":
		if dict != "" {
			return fmt.Sprintf(`<el-checkbox-group v-model="%s"><el-checkbox v-for="dict in %s" :key="dict.value" :value="dict.value">{{ dict.label }}</el-checkbox></el-checkbox-group>`, target, dict)
		}
	}
	enter := ""
	if query {
		enter = ` @keyup.enter="handleQuery"`
	}
	return fmt.Sprintf(`<el-input v-model="%s"%s%s />`, target, clearable, enter)
}

func columnDict(column model.GenTableColumn) string {
	if column.DictType == nil {
		return ""
	}
	return strings.TrimSpace(*column.DictType)
}

func tableDicts(columns []model.GenTableColumn) []string {
	set := make(map[string]struct{})
	for _, column := range columns {
		if value := columnDict(column); value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func quoteJSArgs(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+safeJSString(value)+"'")
	}
	return strings.Join(quoted, ", ")
}

func renderMenuSQL(table *model.GenTable) string {
	module := strings.ToLower(table.ModuleName)
	business := strings.ToLower(table.BusinessName)
	permission := module + ":" + business
	parent := table.ParentMenuID
	if parent <= 0 {
		parent = 3
	}
	name := sqlString(table.FunctionName)
	return fmt.Sprintf(`-- 生成后请先核对 parent_id，再在目标环境执行。
SET @parentId = %d;
INSERT INTO sys_menu (menu_name, parent_id, order_num, path, component, query, route_name, is_frame, is_cache, menu_type, visible, status, perms, icon, create_by, create_time, remark)
VALUES ('%s', @parentId, 1, '%s', '%s/%s/index', '', '', 1, 0, 'C', '0', '0', '%s:list', '#', 'admin', NOW(), '%s菜单');
SET @menuId = LAST_INSERT_ID();
INSERT INTO sys_menu (menu_name, parent_id, order_num, path, component, query, route_name, is_frame, is_cache, menu_type, visible, status, perms, icon, create_by, create_time)
VALUES
('%s查询', @menuId, 1, '#', '', '', '', 1, 0, 'F', '0', '0', '%s:query', '#', 'admin', NOW()),
('%s新增', @menuId, 2, '#', '', '', '', 1, 0, 'F', '0', '0', '%s:add', '#', 'admin', NOW()),
('%s修改', @menuId, 3, '#', '', '', '', 1, 0, 'F', '0', '0', '%s:edit', '#', 'admin', NOW()),
('%s删除', @menuId, 4, '#', '', '', '', 1, 0, 'F', '0', '0', '%s:remove', '#', 'admin', NOW()),
('%s导出', @menuId, 5, '#', '', '', '', 1, 0, 'F', '0', '0', '%s:export', '#', 'admin', NOW());
`, parent, name, business, module, business, permission, name, name, permission, name, permission, name, permission, name, permission, name, permission)
}

func renderGeneratedReadme(table *model.GenTable) string {
	return fmt.Sprintf(`# %s 生成结果

- 后端文件复制到 ruoyi-go 对应目录后会通过 router 包 init 自动登记路由。
- 前端文件复制到 Vue3 项目的 src 目录。
- 菜单 SQL 不会自动执行；核对上级菜单和权限标识后再手工执行。
- 导出接口受并发数和最大 100000 行限制，超过上限会明确失败，不会返回截断文件。
- 生成器不会修改数据库结构，也不会自动覆盖现有业务文件。
- 当前模板类别：%s；前端类别：%s。
`, table.FunctionName, table.TplCategory, table.TplWebType)
}

func writeAuditAssignments(b *strings.Builder, table *model.GenTable, prefix, operator string) {
	if column := findColumn(table.Columns, strings.ToLower(prefix)+"By"); column != nil && column.JavaType == "String" {
		fmt.Fprintf(b, "\ttarget.%s = %s\n", exportedField(column.JavaField), operator)
	}
	if column := findColumn(table.Columns, strings.ToLower(prefix)+"Time"); column != nil && column.JavaType == "Date" {
		fmt.Fprintf(b, "\ttarget.%s = types.Now()\n", exportedField(column.JavaField))
	}
}

func writeSubAuditAssignments(b *strings.Builder, table *model.GenTable, operator string) {
	if table.TplCategory != "sub" || table.SubTable == nil {
		return
	}
	createBy := findColumn(table.SubTable.Columns, "createBy")
	createTime := findColumn(table.SubTable.Columns, "createTime")
	updateBy := findColumn(table.SubTable.Columns, "updateBy")
	updateTime := findColumn(table.SubTable.Columns, "updateTime")
	if createBy == nil && createTime == nil && updateBy == nil && updateTime == nil {
		return
	}
	fmt.Fprintf(b, "\tfor i := range target.%s {\n", subListField(table))
	if createBy != nil && createBy.JavaType == "String" {
		fmt.Fprintf(b, "\t\ttarget.%s[i].%s = %s\n", subListField(table), exportedField(createBy.JavaField), operator)
	}
	if createTime != nil && createTime.JavaType == "Date" {
		fmt.Fprintf(b, "\t\ttarget.%s[i].%s = types.Now()\n", subListField(table), exportedField(createTime.JavaField))
	}
	if updateBy != nil && updateBy.JavaType == "String" {
		fmt.Fprintf(b, "\t\ttarget.%s[i].%s = %s\n", subListField(table), exportedField(updateBy.JavaField), operator)
	}
	if updateTime != nil && updateTime.JavaType == "Date" {
		fmt.Fprintf(b, "\t\ttarget.%s[i].%s = types.Now()\n", subListField(table), exportedField(updateTime.JavaField))
	}
	b.WriteString("\t}\n")
}

func goColumnType(column model.GenTableColumn) string {
	switch column.JavaType {
	case "Long":
		return "int64"
	case "Integer":
		return "int"
	case "Double", "BigDecimal":
		return "float64"
	case "Date":
		return "types.Time"
	case "Boolean":
		return "bool"
	default:
		return "string"
	}
}

func zeroValueForColumn(column model.GenTableColumn) string {
	switch goColumnType(column) {
	case "string":
		return `""`
	case "types.Time":
		return "types.Time{}"
	case "bool":
		return "false"
	default:
		return "0"
	}
}

func generatedBindingTag(column model.GenTableColumn) string {
	if column.IsRequired == "1" && column.IsIncrement != "1" &&
		!containsFold([]string{"createBy", "createTime", "updateBy", "updateTime"}, column.JavaField) {
		return ` binding:"required"`
	}
	return ""
}

func generatedExcelTag(column model.GenTableColumn) string {
	if column.IsList != "1" {
		return ""
	}
	name := strings.NewReplacer(";", " ", ":", " ").Replace(safeComment(column.ColumnComment))
	options := []string{"name:" + name}
	switch goColumnType(column) {
	case "int", "int64", "float64":
		options = append(options, "cell:numeric")
	case "types.Time":
		options = append(options, "format:2006-01-02 15:04:05", "width:30")
	}
	return ` excel:"` + strings.Join(options, ";") + `"`
}

func hasJavaType(columns []model.GenTableColumn, javaType string) bool {
	for _, column := range columns {
		if column.JavaType == javaType {
			return true
		}
	}
	return false
}

func findColumn(columns []model.GenTableColumn, javaField string) *model.GenTableColumn {
	for i := range columns {
		if strings.EqualFold(columns[i].JavaField, javaField) {
			return &columns[i]
		}
	}
	return nil
}

func selectedColumns(columns []model.GenTableColumn, keep func(model.GenTableColumn) bool) []model.GenTableColumn {
	result := make([]model.GenTableColumn, 0, len(columns))
	for _, column := range columns {
		if keep(column) {
			result = append(result, column)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return intValue(result[i].Sort) < intValue(result[j].Sort)
	})
	return result
}

func subListField(table *model.GenTable) string {
	return table.SubTable.ClassName + "List"
}

func editableSubColumns(table *model.GenTable) []model.GenTableColumn {
	if table.SubTable == nil || table.SubTableFKName == nil {
		return nil
	}
	fkName := strings.TrimSpace(*table.SubTableFKName)
	return selectedColumns(table.SubTable.Columns, func(column model.GenTableColumn) bool {
		return column.IsPK != "1" && !strings.EqualFold(column.ColumnName, fkName) &&
			(column.IsInsert == "1" || column.IsEdit == "1")
	})
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func exportedField(value string) string {
	if value == "" {
		return "Field"
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func safeComment(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ", "*/", "* /").Replace(strings.TrimSpace(value))
	if value == "" {
		return "字段"
	}
	return value
}

func safeJSString(value string) string {
	return strings.NewReplacer("\\", "\\\\", "'", "\\'", "\r", " ", "\n", " ").Replace(value)
}

func sqlString(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "'", "''")
}

func pathWithin(base, target string) bool {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(baseAbs), filepath.Clean(targetAbs))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func writeGeneratedFile(target string, content []byte) error {
	parent := filepath.Dir(target)
	if err := rejectSymlinkComponents(parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("创建生成目录失败: %w", err)
	}
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errs.New("拒绝覆盖符号链接文件")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查生成文件失败: %w", err)
	}
	temp, err := os.CreateTemp(parent, ".ruoyi-gen-*")
	if err != nil {
		return fmt.Errorf("创建生成临时文件失败: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("写入生成文件失败: %w", err)
	}
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("设置生成文件权限失败: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭生成临时文件失败: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		// Windows 不能直接覆盖已有文件。先把旧文件原子移到同目录备份，
		// 新文件替换失败时再恢复，不能为了覆盖而先删除用户文件。
		if _, statErr := os.Lstat(target); statErr != nil {
			return fmt.Errorf("完成生成文件失败: %w", err)
		}
		backup, backupErr := os.CreateTemp(parent, ".ruoyi-gen-backup-*")
		if backupErr != nil {
			return fmt.Errorf("创建覆盖备份失败: %w", backupErr)
		}
		backupName := backup.Name()
		if closeErr := backup.Close(); closeErr != nil {
			_ = os.Remove(backupName)
			return fmt.Errorf("关闭覆盖备份失败: %w", closeErr)
		}
		if removeErr := os.Remove(backupName); removeErr != nil {
			return fmt.Errorf("准备覆盖备份失败: %w", removeErr)
		}
		if moveErr := os.Rename(target, backupName); moveErr != nil {
			return fmt.Errorf("备份原生成文件失败: %w", moveErr)
		}
		if moveErr := os.Rename(tempName, target); moveErr != nil {
			if restoreErr := os.Rename(backupName, target); restoreErr != nil {
				return fmt.Errorf("完成生成文件失败: %v；恢复原文件也失败: %w", moveErr, restoreErr)
			}
			return fmt.Errorf("完成生成文件失败: %w", moveErr)
		}
		if removeErr := os.Remove(backupName); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("清理生成备份失败: %w", removeErr)
		}
	}
	return nil
}

func rejectSymlinkComponents(target string) error {
	volume := filepath.VolumeName(target)
	remainder := strings.TrimPrefix(filepath.Clean(target), volume)
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.Trim(remainder, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("检查生成目录失败: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errs.New("生成路径不能包含符号链接")
		}
	}
	return nil
}
