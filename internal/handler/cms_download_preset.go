package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"mime"
	"net/http"
	"net/url"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/portalview"
	"ruoyi-go/pkg/response"
	"strconv"
	"strings"
	"time"
)

func CMSDownloadCategories(c *gin.Context) {
	var q model.CMSDownloadCategoryQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "分类筛选参数错误")
		return
	}
	rows, n, e := service.CMSDownloadCategories(c.Request.Context(), q, page.Parse(c, nil), false)
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSSaveDownloadCategory(c *gin.Context) {
	var in model.CMSDownloadCategoryInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "分类参数错误")
		return
	}
	id, e := service.CMSSaveDownloadCategory(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, map[string]int64{"categoryId": id})
}
func CMSRemoveDownloadCategory(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSVersion
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "请提交分类当前版本")
		return
	}
	if e = service.CMSRemoveDownloadCategory(c.Request.Context(), id, in.Version); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSDownloadFiles(c *gin.Context) {
	var q model.CMSDownloadFileQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "附件筛选参数错误")
		return
	}
	rows, n, e := service.CMSDownloadFiles(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSDownloadUpload(c *gin.Context) {
	file, e := c.FormFile("file")
	if e != nil {
		response.Fail(c, "请选择资料附件")
		return
	}
	if file.Size > service.CMSDownloadMaxBytes {
		response.Fail(c, "资料附件不能超过20MiB")
		return
	}
	if len(c.Request.MultipartForm.File) != 1 || len(c.Request.MultipartForm.File["file"]) != 1 {
		response.Fail(c, "每次只能上传一个资料附件")
		return
	}
	f, e := file.Open()
	if e != nil {
		response.Fail(c, "读取资料附件失败")
		return
	}
	defer f.Close()
	row, e := service.CMSUploadDownload(c.Request.Context(), f, file.Filename)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSDeleteDownloadFile(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSVersion
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "请提交附件当前版本")
		return
	}
	if e = service.CMSDeleteDownloadFile(c.Request.Context(), id, in.Version); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func serveCMSDownload(c *gin.Context, public bool) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		if public {
			c.Status(404)
		} else {
			failDownload(c, e)
		}
		return
	}
	f, row, e := service.CMSOpenDownloadFile(c.Request.Context(), id, public)
	if e != nil {
		if public {
			if service.IsCMSNotFound(e) {
				c.Status(404)
			} else {
				c.Status(500)
			}
		} else {
			failDownload(c, e)
		}
		return
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		c.Status(500)
		return
	}
	c.Header("Content-Type", row.MimeType)
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": row.Name}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("ETag", fmt.Sprintf("\"dl-%d-%s\"", row.FileId, row.SHA256))
	modified := info.ModTime()
	// A public resource can switch to a file uploaded earlier or in the same
	// second. Upload mtime is therefore not its representation modification time.
	// Use the immutable file version ETag; ignore date-only public conditions.
	if public {
		modified = time.Time{}
	}
	http.ServeContent(c.Writer, c.Request, row.Name, modified, f)
}
func CMSPrivateDownload(c *gin.Context) { serveCMSDownload(c, false) }
func PortalDownload(c *gin.Context)     { serveCMSDownload(c, true) }
func CMSDownloadList(c *gin.Context) {
	var q model.CMSDownloadQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "资料筛选参数错误")
		return
	}
	rows, n, e := service.CMSDownloadPage(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSDownloadDetail(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	row, e := service.CMSDownloadDetail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSSaveDownload(c *gin.Context) {
	var in model.CMSDownloadInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "资料标题、分类或附件参数错误")
		return
	}
	id, e := service.CMSSaveDownload(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, map[string]int64{"downloadId": id})
}
func CMSDownloadHistory(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	rows, n, e := service.CMSDownloadHistory(c.Request.Context(), id, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSChangeDownload(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		var in model.CMSVersion
		if e = c.ShouldBindJSON(&in); e != nil {
			response.Fail(c, "请提交资料当前版本")
			return
		}
		if e = service.CMSChangeDownload(c.Request.Context(), id, in.Version, action); e != nil {
			fail(c, e)
			return
		}
		response.Ok(c)
	}
}
func (p *PortalUI) Downloads(c *gin.Context) {
	var q model.CMSDownloadQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		c.Status(400)
		return
	}
	n := 1
	if s := c.Query("page"); s != "" {
		v, e := strconv.Atoi(s)
		if e != nil || v < 1 || v > 1000 {
			c.Status(400)
			return
		}
		n = v
	}
	rows, total, e := service.PortalDownloadPage(c.Request.Context(), q, (n-1)*12, 12)
	if e != nil {
		p.pageError(c, e)
		return
	}
	if n > 1 && int64((n-1)*12) >= total {
		p.pageError(c, service.PortalNotFound())
		return
	}
	d := portalview.PageData{Title: "资料下载中心", Description: "产品资料、操作指南与公开文档", Query: q.Title, NoIndex: q.Title != "" || q.CategoryId > 0, IsListing: true, DownloadCategoryId: q.CategoryId}
	for _, r := range rows {
		d.Downloads = append(d.Downloads, portalview.Download{Id: r.DownloadId, Title: r.Title, Summary: r.Summary, Category: r.CategoryName, FileName: r.FileName, Extension: strings.ToUpper(r.Extension), Size: r.ByteSize, SHA256: r.SHA256})
	}
	catPage := 1
	if raw := c.Query("categoryPage"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 1000 {
			c.Status(400)
			return
		}
		catPage = v
	}
	cats, catTotal, e := service.CMSDownloadCategories(c.Request.Context(), model.CMSDownloadCategoryQuery{}, page.Query{PageNum: catPage, PageSize: 100}, true)
	if e != nil {
		p.pageError(c, e)
		return
	}
	for _, cat := range cats {
		d.DownloadCategories = append(d.DownloadCategories, portalview.DownloadCategory{Id: cat.CategoryId, Name: cat.Name})
	}
	link := func(pg int) string {
		v := url.Values{}
		if q.Title != "" {
			v.Set("title", q.Title)
		}
		if q.CategoryId > 0 {
			v.Set("categoryId", strconv.FormatInt(q.CategoryId, 10))
		}
		if catPage > 1 {
			v.Set("categoryPage", strconv.Itoa(catPage))
		}
		v.Set("page", strconv.Itoa(pg))
		return "/downloads?" + v.Encode()
	}
	if n > 1 {
		d.Prev = link(n - 1)
	}
	if int64(n*12) < total {
		d.Next = link(n + 1)
	}
	d.Canonical = p.origin + link(n)
	catLink := func(pg int) string {
		v := url.Values{}
		v.Set("categoryPage", strconv.Itoa(pg))
		if q.Title != "" {
			v.Set("title", q.Title)
		}
		if q.CategoryId > 0 {
			v.Set("categoryId", strconv.FormatInt(q.CategoryId, 10))
		}
		return "/downloads?" + v.Encode()
	}
	if catPage > 1 {
		d.DownloadCategoryPrev = catLink(catPage - 1)
	}
	if int64(catPage*100) < catTotal {
		d.DownloadCategoryNext = catLink(catPage + 1)
	}
	p.render(c, 200, "downloads", d)
}
func CMSPresetDefinitions(c *gin.Context) {
	rows, n, e := service.CMSPresetDefinitions(c.Request.Context(), page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSPreviewPreset(c *gin.Context) {
	var in model.CMSPresetInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "预设计划参数错误")
		return
	}
	row, e := service.CMSPreviewPreset(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSApplyPreset(c *gin.Context) {
	var in model.CMSPresetApply
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "预设计划、请求ID或原因参数错误")
		return
	}
	row, e := service.CMSApplyPreset(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSPresetHistory(c *gin.Context) {
	rows, n, e := service.CMSPresetHistory(c.Request.Context(), page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func (p *PortalUI) PresetPreview(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	def, e := service.CMSPresetDefinition(c.Request.Context(), c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	manifest, ok := p.view.Manifest(def.Theme)
	if !ok {
		response.Fail(c, "预设主题未注册")
		return
	}
	d := portalview.PageData{SiteName: def.Name + " · 预设示意", Title: "行业模板预设示意", Description: def.Description, Theme: def.Theme, ThemeVersion: manifest.Version, Palette: def.Palette, Density: "normal", FontScale: "standard", FrameWidth: "normal", Radius: "soft", NoIndex: true, ListingLayout: "grid"}
	for _, b := range def.Blocks {
		s := portalview.Section{Kind: b.Kind, Title: b.Title, Layout: b.Layout}
		for _, v := range b.Items {
			s.Items = append(s.Items, portalview.WidgetItem{Title: v.Title, Text: v.Text})
		}
		if len(s.Items) == 0 {
			s.Cards = []portalview.Card{{Title: "示例内容 · 应用后替换为真实资料", Summary: "这是内置方案的布局示意，不会发布或读取你的草稿。", URL: "#", Kind: b.Kind}}
		}
		d.Sections = append(d.Sections, s)
	}
	body, e := p.view.Render(def.Theme, "home", d)
	if e != nil {
		response.Fail(c, "预设示意暂时不可用")
		return
	}
	response.OkData(c, string(body))
}
