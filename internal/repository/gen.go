package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

func genTableFilter(db *gorm.DB, query model.GenTableQuery) *gorm.DB {
	if query.TableName != "" {
		db = db.Where("LOWER(table_name) LIKE LOWER(?)", "%"+query.TableName+"%")
	}
	if query.TableComment != "" {
		db = db.Where("LOWER(table_comment) LIKE LOWER(?)", "%"+query.TableComment+"%")
	}
	if query.BeginTime != "" {
		db = db.Where("DATE_FORMAT(create_time, '%Y%m%d') >= DATE_FORMAT(?, '%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("DATE_FORMAT(create_time, '%Y%m%d') <= DATE_FORMAT(?, '%Y%m%d')", query.EndTime)
	}
	return db
}

// SelectGenTablePage 查询已经导入生成器的表。
func SelectGenTablePage(ctx context.Context, query model.GenTableQuery, pg page.Query) ([]model.GenTable, int64, error) {
	db := genTableFilter(DB(ctx).Table("gen_table"), query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计代码生成表失败: %w", err)
	}
	if total == 0 {
		return []model.GenTable{}, 0, nil
	}
	var list []model.GenTable
	if err := db.Order(pg.Stable("create_time DESC, table_id", "table_id")).
		Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询代码生成表失败: %w", err)
	}
	return list, total, nil
}

// SelectDBTablePage 查询当前数据库中尚未导入生成器的普通表。
func SelectDBTablePage(ctx context.Context, query model.GenTableQuery, pg page.Query) ([]model.GenTable, int64, error) {
	db := DB(ctx).Table("information_schema.tables AS t").
		Where("t.table_schema = DATABASE()").
		Where("t.table_name NOT LIKE ? ESCAPE '\\\\'", "qrtz\\_%").
		Where("t.table_name NOT LIKE ? ESCAPE '\\\\'", "gen\\_%").
		Where("NOT EXISTS (SELECT 1 FROM gen_table g WHERE g.table_name = t.table_name)")
	if query.TableName != "" {
		db = db.Where("LOWER(t.table_name) LIKE LOWER(?)", "%"+query.TableName+"%")
	}
	if query.TableComment != "" {
		db = db.Where("LOWER(t.table_comment) LIKE LOWER(?)", "%"+query.TableComment+"%")
	}
	if query.BeginTime != "" {
		db = db.Where("DATE_FORMAT(t.create_time, '%Y%m%d') >= DATE_FORMAT(?, '%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("DATE_FORMAT(t.create_time, '%Y%m%d') <= DATE_FORMAT(?, '%Y%m%d')", query.EndTime)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计数据库表失败: %w", err)
	}
	if total == 0 {
		return []model.GenTable{}, 0, nil
	}
	var list []model.GenTable
	if err := db.Select("t.table_name, t.table_comment, t.create_time, t.update_time").
		Order(pg.Stable("t.create_time DESC, t.table_name", "t.table_name")).
		Offset(pg.Offset()).Limit(pg.PageSize).Scan(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询数据库表失败: %w", err)
	}
	return list, total, nil
}

// SelectDBTablesByNames 批量读取当前库的表元数据。
func SelectDBTablesByNames(ctx context.Context, names []string) ([]model.GenTable, error) {
	if len(names) == 0 {
		return []model.GenTable{}, nil
	}
	var list []model.GenTable
	err := DB(ctx).Table("information_schema.tables AS t").
		Select("t.table_name, t.table_comment, t.create_time, t.update_time").
		Where("t.table_schema = DATABASE()").
		Where("t.table_name NOT LIKE ? ESCAPE '\\\\'", "qrtz\\_%").
		Where("t.table_name NOT LIKE ? ESCAPE '\\\\'", "gen\\_%").
		Where("t.table_name IN ?", names).
		Order("t.table_name").Scan(&list).Error
	if err != nil {
		return nil, fmt.Errorf("批量查询数据库表失败: %w", err)
	}
	return list, nil
}

func SelectExistingGenTableNames(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return []string{}, nil
	}
	var existing []string
	if err := DB(ctx).Table("gen_table").Where("table_name IN ?", names).
		Order("table_name").Pluck("table_name", &existing).Error; err != nil {
		return nil, fmt.Errorf("校验已导入表失败: %w", err)
	}
	return existing, nil
}

type dbColumnRow struct {
	TableName     string `gorm:"column:table_name"`
	ColumnName    string `gorm:"column:column_name"`
	ColumnComment string `gorm:"column:column_comment"`
	ColumnType    string `gorm:"column:column_type"`
	IsPK          string `gorm:"column:is_pk"`
	IsIncrement   string `gorm:"column:is_increment"`
	IsRequired    string `gorm:"column:is_required"`
	Sort          int    `gorm:"column:sort"`
}

// SelectDBColumnsByTableNames 一次查询多张表的列，避免导入时逐表访问 information_schema。
func SelectDBColumnsByTableNames(ctx context.Context, names []string) (map[string][]model.GenTableColumn, error) {
	result := make(map[string][]model.GenTableColumn, len(names))
	if len(names) == 0 {
		return result, nil
	}
	var rows []dbColumnRow
	err := DB(ctx).Table("information_schema.columns AS c").
		Select(`c.table_name, c.column_name, c.column_comment, c.column_type,
CASE WHEN c.column_key = 'PRI' THEN '1' ELSE '0' END AS is_pk,
CASE WHEN c.extra = 'auto_increment' THEN '1' ELSE '0' END AS is_increment,
CASE WHEN c.is_nullable = 'NO' AND c.column_key <> 'PRI' THEN '1' ELSE '0' END AS is_required,
c.ordinal_position AS sort`).
		Where("c.table_schema = DATABASE()").Where("c.table_name IN ?", names).
		Order("c.table_name, c.ordinal_position").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("批量查询数据库字段失败: %w", err)
	}
	for _, row := range rows {
		sortValue := row.Sort
		result[row.TableName] = append(result[row.TableName], model.GenTableColumn{
			ColumnName: row.ColumnName, ColumnComment: row.ColumnComment, ColumnType: row.ColumnType,
			IsPK: row.IsPK, IsIncrement: row.IsIncrement, IsRequired: row.IsRequired, Sort: &sortValue,
		})
	}
	return result, nil
}

func SelectGenTableByID(ctx context.Context, tableID int64) (*model.GenTable, error) {
	var table model.GenTable
	err := DB(ctx).Table("gen_table").Where("table_id = ?", tableID).Take(&table).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询代码生成表 %d 失败: %w", tableID, err)
	}
	columns, err := SelectGenTableColumns(ctx, tableID)
	if err != nil {
		return nil, err
	}
	table.Columns = columns
	return &table, nil
}

func SelectGenTableByName(ctx context.Context, tableName string) (*model.GenTable, error) {
	var table model.GenTable
	err := DB(ctx).Table("gen_table").Where("table_name = ?", tableName).Take(&table).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询代码生成表 %s 失败: %w", tableName, err)
	}
	columns, err := SelectGenTableColumns(ctx, table.TableID)
	if err != nil {
		return nil, err
	}
	table.Columns = columns
	return &table, nil
}

func SelectGenTableColumns(ctx context.Context, tableID int64) ([]model.GenTableColumn, error) {
	var columns []model.GenTableColumn
	err := DB(ctx).Table("gen_table_column").Where("table_id = ?", tableID).
		Order("sort, column_id").Find(&columns).Error
	if err != nil {
		return nil, fmt.Errorf("查询代码生成字段失败: %w", err)
	}
	return columns, nil
}

// SelectGenTablesAllWithColumns 批量装载所有生成表及字段，供主子表编辑下拉使用。
func SelectGenTablesAllWithColumns(ctx context.Context) ([]model.GenTable, error) {
	var tables []model.GenTable
	if err := DB(ctx).Table("gen_table").Order("table_id").Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("查询全部代码生成表失败: %w", err)
	}
	if len(tables) == 0 {
		return []model.GenTable{}, nil
	}
	ids := make([]int64, 0, len(tables))
	for _, table := range tables {
		ids = append(ids, table.TableID)
	}
	var columns []model.GenTableColumn
	if err := DB(ctx).Table("gen_table_column").Where("table_id IN ?", ids).
		Order("table_id, sort, column_id").Find(&columns).Error; err != nil {
		return nil, fmt.Errorf("查询全部代码生成字段失败: %w", err)
	}
	byTable := make(map[int64][]model.GenTableColumn, len(tables))
	for _, column := range columns {
		byTable[column.TableID] = append(byTable[column.TableID], column)
	}
	for i := range tables {
		tables[i].Columns = byTable[tables[i].TableID]
	}
	return tables, nil
}

// InsertGenTables 将表和列原子导入生成器配置表。
func InsertGenTables(ctx context.Context, tables []model.GenTable) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if len(tables) == 0 {
			return nil
		}
		if err := tx.Table("gen_table").CreateInBatches(&tables, 100).Error; err != nil {
			return fmt.Errorf("批量导入代码生成表失败: %w", err)
		}
		columns := make([]model.GenTableColumn, 0)
		for i := range tables {
			for j := range tables[i].Columns {
				tables[i].Columns[j].TableID = tables[i].TableID
			}
			columns = append(columns, tables[i].Columns...)
		}
		if len(columns) > 0 {
			if err := tx.Table("gen_table_column").CreateInBatches(columns, 100).Error; err != nil {
				return fmt.Errorf("批量导入代码生成字段失败: %w", err)
			}
		}
		return nil
	})
}

func UpdateGenTableWithColumns(ctx context.Context, table *model.GenTable) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		updates := map[string]any{
			"table_name": table.TableName, "table_comment": table.TableComment,
			"sub_table_name": table.SubTableName, "sub_table_fk_name": table.SubTableFKName,
			"class_name": table.ClassName, "tpl_category": table.TplCategory,
			"tpl_web_type": table.TplWebType, "package_name": table.PackageName,
			"module_name": table.ModuleName, "business_name": table.BusinessName,
			"function_name": table.FunctionName, "function_author": table.FunctionAuthor,
			"form_col_num": table.FormColNum, "gen_type": table.GenType, "gen_path": table.GenPath,
			"options": table.Options, "update_by": table.UpdateBy, "update_time": table.UpdateTime,
			"remark": table.Remark,
		}
		res := tx.Table("gen_table").Where("table_id = ?", table.TableID).Updates(updates)
		if res.Error != nil {
			return fmt.Errorf("更新代码生成表失败: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("更新代码生成表失败: 表不存在")
		}
		for i := range table.Columns {
			column := &table.Columns[i]
			if column.ColumnID <= 0 || column.TableID != table.TableID {
				return fmt.Errorf("更新代码生成字段失败: 字段归属不合法")
			}
			column.UpdateBy = table.UpdateBy
			column.UpdateTime = table.UpdateTime
			columnUpdates := map[string]any{
				"column_comment": column.ColumnComment, "column_type": column.ColumnType,
				"java_type": column.JavaType, "java_field": column.JavaField,
				"is_insert": column.IsInsert, "is_edit": column.IsEdit, "is_list": column.IsList,
				"is_query": column.IsQuery, "is_required": column.IsRequired,
				"query_type": column.QueryType, "html_type": column.HTMLType,
				"dict_type": column.DictType, "sort": column.Sort,
				"update_by": column.UpdateBy, "update_time": column.UpdateTime,
			}
			res = tx.Table("gen_table_column").Where("column_id = ? AND table_id = ?", column.ColumnID, table.TableID).
				Updates(columnUpdates)
			if res.Error != nil {
				return fmt.Errorf("更新代码生成字段失败: %w", res.Error)
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("更新代码生成字段失败: 字段不存在")
			}
		}
		return nil
	})
}

func ReplaceGenTableColumns(ctx context.Context, tableID int64, columns []model.GenTableColumn) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Table("gen_table_column").Where("table_id = ?", tableID).Delete(&model.GenTableColumn{}).Error; err != nil {
			return fmt.Errorf("同步前清理字段失败: %w", err)
		}
		for i := range columns {
			columns[i].ColumnID = 0
			columns[i].TableID = tableID
		}
		if len(columns) > 0 {
			if err := tx.Table("gen_table_column").CreateInBatches(columns, 100).Error; err != nil {
				return fmt.Errorf("同步字段失败: %w", err)
			}
		}
		return nil
	})
}

// SyncGenTableColumns 保留仍存在字段的 column_id 和生成配置，只增删数据库真实发生变化的字段。
func SyncGenTableColumns(ctx context.Context, tableID int64, columns []model.GenTableColumn) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		keepIDs := make([]int64, 0, len(columns))
		for i := range columns {
			column := &columns[i]
			column.TableID = tableID
			if column.ColumnID == 0 {
				if err := tx.Table("gen_table_column").Create(column).Error; err != nil {
					return fmt.Errorf("同步新增字段失败: %w", err)
				}
			} else {
				updates := map[string]any{
					"column_name": column.ColumnName, "column_comment": column.ColumnComment,
					"column_type": column.ColumnType, "is_pk": column.IsPK,
					"is_increment": column.IsIncrement, "is_required": column.IsRequired,
					"sort": column.Sort, "update_by": column.UpdateBy, "update_time": column.UpdateTime,
				}
				if err := tx.Table("gen_table_column").Where("column_id = ? AND table_id = ?", column.ColumnID, tableID).
					Updates(updates).Error; err != nil {
					return fmt.Errorf("同步已有字段失败: %w", err)
				}
			}
			keepIDs = append(keepIDs, column.ColumnID)
		}
		deleteQuery := tx.Table("gen_table_column").Where("table_id = ?", tableID)
		if len(keepIDs) > 0 {
			deleteQuery = deleteQuery.Where("column_id NOT IN ?", keepIDs)
		}
		if err := deleteQuery.Delete(&model.GenTableColumn{}).Error; err != nil {
			return fmt.Errorf("同步删除失效字段失败: %w", err)
		}
		return nil
	})
}

func DeleteGenTables(ctx context.Context, tableIDs []int64) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Table("gen_table_column").Where("table_id IN ?", tableIDs).
			Delete(&model.GenTableColumn{}).Error; err != nil {
			return fmt.Errorf("删除代码生成字段失败: %w", err)
		}
		if err := tx.Table("gen_table").Where("table_id IN ?", tableIDs).
			Delete(&model.GenTable{}).Error; err != nil {
			return fmt.Errorf("删除代码生成表失败: %w", err)
		}
		return nil
	})
}

func ExecuteCreateTableStatements(ctx context.Context, statements []string) error {
	for _, statement := range statements {
		if err := DB(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("创建表失败: %w", err)
		}
	}
	return nil
}
