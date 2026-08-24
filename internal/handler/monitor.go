package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// ---------- 在线用户 ----------

// OnlineList GET /monitor/online/list
func OnlineList(c *gin.Context) {
	var query model.OnlineQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, nil)

	list, total, err := service.ListOnlineUsers(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// OnlineForceLogout DELETE /monitor/online/:tokenId
//
// 强退自己会导致当前会话立刻失效，这里挡掉 —— 点错的代价是被自己踢出登录。
func OnlineForceLogout(c *gin.Context) {
	tokenID := c.Param("tokenId")
	if tokenID == "" {
		response.Fail(c, "会话标识不能为空")
		return
	}
	if loginUser := middleware.CurrentUser(c); loginUser != nil && loginUser.Token == tokenID {
		response.Fail(c, "不能强退自己的会话")
		return
	}

	if err := service.ForceLogout(c.Request.Context(), tokenID); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ---------- 服务监控 ----------

// ServerInfo GET /monitor/server
func ServerInfo(c *gin.Context) {
	info, err := service.ServerStatus(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, info)
}

// ---------- 缓存监控 ----------

// CacheInfo GET /monitor/cache
func CacheInfo(c *gin.Context) {
	data, err := service.CacheOverview(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, data)
}

// CacheNames GET /monitor/cache/getNames
func CacheNames(c *gin.Context) {
	response.OkData(c, service.CacheNames())
}

// CacheKeys GET /monitor/cache/getKeys/:cacheName
func CacheKeys(c *gin.Context) {
	keys, err := service.CacheKeys(c.Request.Context(), c.Param("cacheName"))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, keys)
}

// CacheValue GET /monitor/cache/getValue/:cacheName/:cacheKey
func CacheValue(c *gin.Context) {
	item, err := service.CacheValue(c.Request.Context(), c.Param("cacheName"), c.Param("cacheKey"))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, item)
}

// CacheClearName DELETE /monitor/cache/clearCacheName/:cacheName
func CacheClearName(c *gin.Context) {
	if err := service.ClearCacheByName(c.Request.Context(), c.Param("cacheName")); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// CacheClearKey DELETE /monitor/cache/clearCacheKey/:cacheKey
func CacheClearKey(c *gin.Context) {
	if err := service.ClearCacheByKey(c.Request.Context(), c.Param("cacheKey")); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// CacheClearAll DELETE /monitor/cache/clearCacheAll
func CacheClearAll(c *gin.Context) {
	if err := service.ClearCacheAll(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
