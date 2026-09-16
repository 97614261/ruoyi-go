package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func GenList(c *gin.Context) {
	var query model.GenTableQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	list, total, err := service.ListGenTablePage(c.Request.Context(), query, page.Parse(c, model.GenTableSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractGenTables(list, genProjectionList), total)
}

func GenDBList(c *gin.Context) {
	var query model.GenTableQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	list, total, err := service.ListDBTablePage(c.Request.Context(), query, page.Parse(c, model.GenDBTableSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractGenTables(list, genProjectionDB), total)
}

func GenGet(c *gin.Context) {
	tableID, err := parseID(c.Param("tableId"))
	if err != nil {
		fail(c, err)
		return
	}
	table, tables, err := service.GetGenTableInfo(c.Request.Context(), tableID)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, map[string]any{
		"info":   contractGenTable(table, genProjectionDetail),
		"rows":   contractGenColumns(table.Columns),
		"tables": contractGenTables(tables, genProjectionAll),
	})
}

func GenColumnList(c *gin.Context) {
	tableID, err := parseID(c.Param("tableId"))
	if err != nil {
		fail(c, err)
		return
	}
	columns, err := service.ListGenTableColumns(c.Request.Context(), tableID)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractGenColumns(columns), int64(len(columns)))
}

func GenImport(c *gin.Context) {
	names := splitNonEmpty(c.Query("tables"))
	if err := service.ImportGenTables(c.Request.Context(), names, c.Query("tplWebType"), currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func GenCreateTable(c *gin.Context) {
	if err := service.CreateTablesAndImport(c.Request.Context(), c.Query("sql"), c.Query("tplWebType"), currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func GenEdit(c *gin.Context) {
	var table model.GenTable
	if err := c.ShouldBindJSON(&table); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateGenTable(c.Request.Context(), &table, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func GenRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("tableIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteGenTableConfigs(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func GenPreview(c *gin.Context) {
	tableID, err := parseID(c.Param("tableId"))
	if err != nil {
		fail(c, err)
		return
	}
	files, err := service.PreviewGenCode(c.Request.Context(), tableID)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, files)
}

func GenDownload(c *gin.Context) {
	writeGenZip(c, []string{c.Param("tableName")})
}

func GenBatchDownload(c *gin.Context) {
	writeGenZip(c, splitNonEmpty(c.Query("tables")))
}

func writeGenZip(c *gin.Context, names []string) {
	data, err := service.DownloadGenCode(c.Request.Context(), names)
	if err != nil {
		failDownload(c, err)
		return
	}
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "Content-Disposition")
	c.Header("Content-Disposition", `attachment; filename="ruoyi.zip"`)
	c.Data(200, "application/octet-stream; charset=UTF-8", data)
}

func GenWriteCode(c *gin.Context) {
	if err := service.WriteGenCode(c.Request.Context(), c.Param("tableName")); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func GenSync(c *gin.Context) {
	if err := service.SyncGenTable(c.Request.Context(), c.Param("tableName"), currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}
