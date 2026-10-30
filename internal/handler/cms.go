package handler

import (
	"github.com/gin-gonic/gin"
	"mime"
	"net/http"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSChannelList(c *gin.Context) {
	var q model.CMSChannelQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	rows, total, err := service.CMSChannelPage(c.Request.Context(), q, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func CMSChannelGet(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	row, err := service.CMSChannelDetail(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func CMSChannelSave(c *gin.Context) {
	var in model.CMSChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if (c.Request.Method == "POST" && in.ChannelId != 0) || (c.Request.Method == "PUT" && in.ChannelId < 1) {
		response.Fail(c, "栏目ID错误")
		return
	}
	// Assigning a private cover requires the separate material read permission.
	if in.CoverAssetId > 0 && !middleware.CurrentUser(c).HasPermission("cms:asset:query") {
		response.FailCode(c, response.CodeForbidden, "选择封面需要素材预览权限")
		return
	}
	id, err := service.CMSSaveChannel(c.Request.Context(), in, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, map[string]any{"channelId": id})
}
func CMSChannelRemove(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	var in model.CMSVersion
	if err = c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, "请提交当前版本")
		return
	}
	if err = service.CMSDeleteChannel(c.Request.Context(), id, in.Version); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func CMSAssetList(c *gin.Context) {
	var q model.CMSAssetQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	rows, total, err := service.CMSAssetPage(c.Request.Context(), q, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func CMSAssetGet(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	row, err := service.CMSAssetDetail(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func CMSAssetUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, "请选择图片文件")
		return
	}
	stream, err := file.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer stream.Close()
	row, err := service.CMSUploadAsset(c.Request.Context(), stream, file.Filename, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func CMSAssetEdit(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	var in model.CMSAssetInput
	if err = c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err = service.CMSUpdateAsset(c.Request.Context(), id, in, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func CMSAssetRemove(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	var in model.CMSVersion
	if err = c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, "请提交当前版本")
		return
	}
	if err = service.CMSDeleteAsset(c.Request.Context(), id, in.Version, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func CMSAssetReferences(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	rows, total, err := service.CMSAssetReferences(c.Request.Context(), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func CMSAssetFile(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		failDownload(c, err)
		return
	}
	file, row, err := service.CMSOpenAsset(c.Request.Context(), id)
	if err != nil {
		failDownload(c, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		failDownload(c, err)
		return
	}
	c.Header("Content-Type", row.MimeType)
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": row.Name}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(c.Writer, c.Request, row.Name, info.ModTime(), file)
}
