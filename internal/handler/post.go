package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// PostList GET /system/post/list
//
// 分页响应：total 和 rows 平铺在顶层，没有 data 包裹。
func PostList(c *gin.Context) {
	var query model.PostQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	pg := page.Parse(c, model.PostSortColumns)

	list, total, err := service.ListPostPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractPosts(list), total)
}

// PostExport POST /system/post/export
//
// 【注意】前端的 download() 是 POST + application/x-www-form-urlencoded，
// 查询条件在**请求体**里而不是 query string，所以用 ShouldBind
// （按 Content-Type 自动选表单绑定），不能用 ShouldBindQuery。
//
// 文件名由前端 saveAs 决定（post_时间戳.xlsx），这里给的只是兜底。
func PostExport(c *gin.Context) {
	var query model.PostQuery
	if err := c.ShouldBind(&query); err != nil {
		// 下载接口的错误一律走 failDownload，普通 Fail 会被前端误判成文件
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListPostExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "岗位数据.xlsx", "岗位数据", list); err != nil {
		failDownload(c, err)
		return
	}
}

// PostOptionSelect GET /system/post/optionselect
func PostOptionSelect(c *gin.Context) {
	list, err := service.ListPostAll(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, contractPosts(list))
}

// PostGet GET /system/post/:postId
func PostGet(c *gin.Context) {
	id, err := parseID(c.Param("postId"))
	if err != nil {
		fail(c, err)
		return
	}
	post, err := service.GetPost(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	// selectPostById 走的是 selectPostVo，没选 update_by / update_time
	response.OkData(c, contractPost(*post))
}

// PostAdd POST /system/post
func PostAdd(c *gin.Context) {
	var post model.SysPost
	if err := c.ShouldBindJSON(&post); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreatePost(c.Request.Context(), &post, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// PostEdit PUT /system/post
func PostEdit(c *gin.Context) {
	var post model.SysPost
	if err := c.ShouldBindJSON(&post); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdatePost(c.Request.Context(), &post, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// PostRemove DELETE /system/post/:postIds
//
// 支持逗号分隔批量删除，如 /system/post/1,2,3。
func PostRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("postIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeletePosts(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
