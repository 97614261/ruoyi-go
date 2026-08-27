package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// ---------- 字典数据 ----------

// DictDataByType GET /system/dict/data/type/:dictType
//
// 字典数据在 data 里。前端几乎每个页面的状态下拉、标签回显都依赖它，
// 查不到时返回空数组而不是 null —— 前端会直接 v-for，null 会报错。
func DictDataByType(c *gin.Context) {
	list := []model.SysDictData{}

	if dictType := c.Param("dictType"); dictType != "" {
		found, err := service.GetDictDataByType(c.Request.Context(), dictType)
		if err != nil {
			fail(c, err)
			return
		}
		if found != nil {
			list = found
		}
	}
	response.OkData(c, contractCachedDictData(list))
}

// DictDataList GET /system/dict/data/list
func DictDataList(c *gin.Context) {
	var query model.DictDataQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.DictDataSortColumns)

	list, total, err := service.ListDictDataPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractDictData(list), total)
}

// DictDataExport POST /system/dict/data/export
func DictDataExport(c *gin.Context) {
	var query model.DictDataQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListDictDataExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "字典数据.xlsx", "字典数据", list); err != nil {
		failDownload(c, err)
		return
	}
}

// DictDataGet GET /system/dict/data/:dictCode
func DictDataGet(c *gin.Context) {
	id, err := parseID(c.Param("dictCode"))
	if err != nil {
		fail(c, err)
		return
	}
	data, err := service.GetDictData(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	// selectDictDataById 走的是 selectDictDataVo，没选 update_by / update_time
	response.OkData(c, contractDictDatum(*data))
}

// DictDataAdd POST /system/dict/data
func DictDataAdd(c *gin.Context) {
	var data model.SysDictData
	if err := c.ShouldBindJSON(&data); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateDictData(c.Request.Context(), &data, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DictDataEdit PUT /system/dict/data
func DictDataEdit(c *gin.Context) {
	var data model.SysDictData
	if err := c.ShouldBindJSON(&data); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateDictData(c.Request.Context(), &data, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DictDataRemove DELETE /system/dict/data/:dictCodes
func DictDataRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("dictCodes"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteDictData(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ---------- 字典类型 ----------

// DictTypeList GET /system/dict/type/list
func DictTypeList(c *gin.Context) {
	var query model.DictTypeQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.DictTypeSortColumns)

	list, total, err := service.ListDictTypePage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractDictTypes(list), total)
}

// DictTypeExport POST /system/dict/type/export
func DictTypeExport(c *gin.Context) {
	var query model.DictTypeQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListDictTypeExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "字典类型.xlsx", "字典类型", list); err != nil {
		failDownload(c, err)
		return
	}
}

// DictTypeOptionSelect GET /system/dict/type/optionselect
func DictTypeOptionSelect(c *gin.Context) {
	list, err := service.ListDictTypeAll(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, contractDictTypes(list))
}

// DictTypeRefreshCache DELETE /system/dict/type/refreshCache
func DictTypeRefreshCache(c *gin.Context) {
	if err := service.ClearDictCache(c.Request.Context(), ""); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DictTypeGet GET /system/dict/type/:dictId
func DictTypeGet(c *gin.Context) {
	id, err := parseID(c.Param("dictId"))
	if err != nil {
		fail(c, err)
		return
	}
	dictType, err := service.GetDictType(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	// selectDictTypeById 走的是 selectDictTypeVo，没选 update_by / update_time
	response.OkData(c, contractDictType(*dictType))
}

// DictTypeAdd POST /system/dict/type
func DictTypeAdd(c *gin.Context) {
	var dictType model.SysDictType
	if err := c.ShouldBindJSON(&dictType); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateDictType(c.Request.Context(), &dictType, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DictTypeEdit PUT /system/dict/type
func DictTypeEdit(c *gin.Context) {
	var dictType model.SysDictType
	if err := c.ShouldBindJSON(&dictType); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateDictType(c.Request.Context(), &dictType, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DictTypeRemove DELETE /system/dict/type/:dictIds
func DictTypeRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("dictIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteDictTypes(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
