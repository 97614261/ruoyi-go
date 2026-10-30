package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"

	"mime"
	"net/http"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"

	"ruoyi-go/pkg/response"
)

func CMSContentList(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.CMSContentQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			response.Fail(c, "查询参数错误")
			return
		}
		rows, total, err := service.CMSContentPage(c.Request.Context(), kind, q, page.Parse(c, nil))
		if err != nil {
			fail(c, err)
			return
		}
		response.Page(c, rows, total)
	}
}
func CMSContentGet(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		row, err := service.CMSContentDetail(c.Request.Context(), kind, id)
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("X-Robots-Tag", "noindex, nofollow")
		response.OkData(c, row)
	}
}
func CMSContentSave(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in model.CMSDraftInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		if (c.Request.Method == "POST" && in.ContentId != 0) || (c.Request.Method == "PUT" && in.ContentId < 1) {
			response.Fail(c, "内容ID错误")
			return
		}
		id, err := service.CMSSaveDraft(c.Request.Context(), kind, in, currentUsername(c), middleware.CurrentUser(c).HasPermission("cms:asset:query"))
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, map[string]any{"contentId": id})
	}
}
func CMSContentHistory(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		rows, total, err := service.CMSHistory(c.Request.Context(), kind, id, page.Parse(c, nil))
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Page(c, rows, total)
	}
}
func CMSContentChange(kind, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		var in model.CMSContentAction
		if err = c.ShouldBindJSON(&in); err != nil {
			response.Fail(c, "请提交当前版本")
			return
		}
		if err = service.CMSChangeContent(c.Request.Context(), kind, id, in, action, currentUsername(c), middleware.CurrentUser(c).HasPermission("cms:asset:query")); err != nil {
			fail(c, err)
			return
		}
		response.Ok(c)
	}
}

func CMSPublicMedia(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.Status(404)
		return
	}
	allowed, err := service.CMSPublicAssetAllowed(c.Request.Context(), id)
	if err != nil {
		c.Status(500)
		return
	}
	if !allowed {
		c.Status(404)
		return
	}
	file, row, err := service.CMSOpenAsset(c.Request.Context(), id)
	if err != nil {
		c.Status(404)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		c.Status(500)
		return
	}
	name := fmt.Sprintf("image-%d", id)
	c.Header("Content-Type", row.MimeType)
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), file)
}

func CMSPublicList(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, total, err := service.CMSPublicPage(c.Request.Context(), kind, page.Parse(c, nil))
		if err != nil {
			c.Status(500)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Page(c, rows, total)
	}
}
