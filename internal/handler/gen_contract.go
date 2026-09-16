package handler

import (
	"strings"

	"ruoyi-go/internal/model"
)

type genTableProjection int

const (
	genProjectionList genTableProjection = iota
	genProjectionDB
	genProjectionDetail
	genProjectionAll
)

func contractGenTables(values []model.GenTable, projection genTableProjection) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for i := range values {
		result = append(result, contractGenTable(&values[i], projection))
	}
	return result
}

// contractGenTable 显式复刻 Java 不同 mapper 投影产生的 null/实值差异。
func contractGenTable(t *model.GenTable, projection genTableProjection) map[string]any {
	result := map[string]any{
		"tableId": nil, "tableName": t.TableName, "tableComment": t.TableComment,
		"subTableName": nil, "subTableFkName": nil, "className": nil,
		"tplCategory": nil, "tplWebType": nil, "packageName": nil, "moduleName": nil,
		"businessName": nil, "functionName": nil, "functionAuthor": nil,
		"formColNum": nil, "genType": nil, "genPath": nil,
		"pkColumn": nil, "subTable": nil, "columns": nil, "options": nil,
		"treeCode": nil, "treeParentCode": nil, "treeName": nil,
		"parentMenuId": nil, "parentMenuName": nil, "view": false,
		"createBy": nil, "createTime": nil, "updateBy": nil, "updateTime": nil, "remark": nil,
	}
	if projection != genProjectionDB {
		result["tableId"] = t.TableID
		result["subTableName"] = t.SubTableName
		result["subTableFkName"] = t.SubTableFKName
		result["className"] = t.ClassName
		result["tplCategory"] = t.TplCategory
		result["tplWebType"] = t.TplWebType
		result["packageName"] = t.PackageName
		result["moduleName"] = t.ModuleName
		result["businessName"] = t.BusinessName
		result["functionName"] = t.FunctionName
		result["functionAuthor"] = t.FunctionAuthor
		result["formColNum"] = t.FormColNum
		result["options"] = t.Options
		result["remark"] = t.Remark
	}
	if projection == genProjectionList {
		result["genType"] = t.GenType
		result["genPath"] = t.GenPath
		result["createBy"] = t.CreateBy
		result["createTime"] = t.CreateTime
		result["updateBy"] = t.UpdateBy
		result["updateTime"] = t.UpdateTime
	}
	if projection == genProjectionDB {
		result["createTime"] = t.CreateTime
		result["updateTime"] = t.UpdateTime
	}
	if projection == genProjectionDetail || projection == genProjectionAll {
		result["columns"] = contractGenColumns(t.Columns)
	}
	if projection == genProjectionDetail {
		result["genType"] = t.GenType
		result["genPath"] = t.GenPath
		result["treeCode"] = nullableString(t.TreeCode)
		result["treeParentCode"] = nullableString(t.TreeParentCode)
		result["treeName"] = nullableString(t.TreeName)
		if t.ParentMenuID != 0 {
			result["parentMenuId"] = t.ParentMenuID
		}
		result["parentMenuName"] = nullableString(t.ParentMenuName)
		result["view"] = t.View
	}
	category, _ := result["tplCategory"].(string)
	result["sub"] = category == "sub"
	result["tree"] = category == "tree"
	result["crud"] = category == "crud"
	return result
}

func contractGenColumns(values []model.GenTableColumn) []map[string]any {
	if values == nil {
		return nil
	}
	result := make([]map[string]any, 0, len(values))
	for _, c := range values {
		capField := c.JavaField
		if capField != "" {
			capField = strings.ToUpper(capField[:1]) + capField[1:]
		}
		super := false
		for _, value := range []string{"createBy", "createTime", "updateBy", "updateTime", "remark", "parentName", "parentId", "orderNum", "ancestors"} {
			if strings.EqualFold(c.JavaField, value) {
				super = true
				break
			}
		}
		result = append(result, map[string]any{
			"columnId": c.ColumnID, "tableId": c.TableID, "columnName": c.ColumnName,
			"columnComment": c.ColumnComment, "columnType": c.ColumnType,
			"javaType": c.JavaType, "javaField": c.JavaField, "capJavaField": capField,
			"isPk": c.IsPK, "isIncrement": c.IsIncrement, "isRequired": c.IsRequired,
			"isInsert": c.IsInsert, "isEdit": c.IsEdit, "isList": c.IsList, "isQuery": c.IsQuery,
			"queryType": c.QueryType, "htmlType": c.HTMLType, "dictType": c.DictType, "sort": c.Sort,
			"createBy": c.CreateBy, "createTime": c.CreateTime,
			"updateBy": c.UpdateBy, "updateTime": c.UpdateTime, "remark": nil,
			"pk": c.IsPK == "1", "increment": c.IsIncrement == "1", "required": c.IsRequired == "1",
			"insert": c.IsInsert == "1", "edit": c.IsEdit == "1", "list": c.IsList == "1", "query": c.IsQuery == "1",
			"superColumn":  super,
			"usableColumn": strings.EqualFold(c.JavaField, "parentId") || strings.EqualFold(c.JavaField, "orderNum") || strings.EqualFold(c.JavaField, "remark"),
		})
	}
	return result
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
