package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// NoticeListTop GET /system/notice/listTop
//
// 【混合形态】公告列表在 data 里，unreadCount 平铺在顶层。
// 这是 CONVENTIONS 里列出的三个混合形态接口之一，
// 不要把 unreadCount 塞进 data，也不要把 list 平铺出来。
func NoticeListTop(c *gin.Context) {
	loginUser := middleware.CurrentUser(c)

	list, unread, err := service.ListNoticeTop(c.Request.Context(), loginUser.UserID)
	if err != nil {
		fail(c, err)
		return
	}
	if list == nil {
		list = []model.NoticeTopItem{}
	}

	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("data", list).
		Put("unreadCount", unread).
		JSON(c)
}

// NoticeList GET /system/notice/list
func NoticeList(c *gin.Context) {
	var query model.NoticeQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.NoticeSortColumns)

	list, total, err := service.ListNoticePage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// NoticeGet GET /system/notice/:noticeId
func NoticeGet(c *gin.Context) {
	id, err := parseID(c.Param("noticeId"))
	if err != nil {
		fail(c, err)
		return
	}
	notice, err := service.GetNotice(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, notice)
}

// NoticeAdd POST /system/notice
func NoticeAdd(c *gin.Context) {
	var notice model.SysNotice
	if err := c.ShouldBindJSON(&notice); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateNotice(c.Request.Context(), &notice, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// NoticeEdit PUT /system/notice
func NoticeEdit(c *gin.Context) {
	var notice model.SysNotice
	if err := c.ShouldBindJSON(&notice); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateNotice(c.Request.Context(), &notice, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// NoticeRemove DELETE /system/notice/:noticeIds
func NoticeRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("noticeIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteNotices(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// NoticeMarkRead POST /system/notice/markRead?noticeId=1
//
// 【注意】是 POST 且参数在 query string 里，不是请求体。
func NoticeMarkRead(c *gin.Context) {
	id, err := parseID(c.Query("noticeId"))
	if err != nil {
		fail(c, err)
		return
	}
	loginUser := middleware.CurrentUser(c)

	if err := service.MarkNoticeRead(c.Request.Context(), loginUser.UserID, id); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// NoticeMarkReadAll POST /system/notice/markReadAll?ids=1,2,3
//
// 【参数在查询串里】前端 markNoticeReadAll 用的是 `params: { ids }`，
// Java 是 `markReadAll(String ids)`，都走查询串。
// 顶栏"全部已读"传的是当前下拉里那几条的 ID，逗号拼接。
//
// 【ids 为空时的行为与 Java 不同，是有意的】
// Java 的 markReadBatch 对空数组直接 return，等于什么都不做；
// 这里标记该用户全部未读公告。前端在 ids 为空时会提前返回、根本不发请求，
// 所以这条路径只有直接调 API 才走得到 —— 而一个叫 markReadAll 的接口
// 收到"没指定哪几条"时把全部标记已读，比静默什么都不做更符合直觉。
func NoticeMarkReadAll(c *gin.Context) {
	loginUser := middleware.CurrentUser(c)

	var ids []int64
	if raw := c.Query("ids"); raw != "" {
		parsed, err := parseIDs(raw)
		if err != nil {
			fail(c, err)
			return
		}
		ids = parsed
	}

	if err := service.MarkNoticeReadBatch(c.Request.Context(), loginUser.UserID, ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// NoticeReadUsers GET /system/notice/readUsers/list?noticeId=1
func NoticeReadUsers(c *gin.Context) {
	var query model.NoticeReadUserQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, nil)

	list, total, err := service.ListNoticeReadUsers(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}
