package handler

import (
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"strings"
)

func CatalogList(category bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.CatalogQuery
		if e := c.ShouldBindQuery(&q); e != nil {
			response.Fail(c, bindMessage(e))
			return
		}
		pg := page.Parse(c, nil)
		var rows any
		var n int64
		var e error
		if category {
			rows, n, e = service.ListCatalogCategories(c.Request.Context(), q, pg)
		} else {
			rows, n, e = service.ListCatalogProducts(c.Request.Context(), q, pg)
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.Page(c, rows, n)
	}
}
func CatalogGet(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	r, e := service.GetCatalogProduct(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, r)
}
func CatalogWrite(category bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		var e error
		if c.Param("id") != "" {
			id, e = parseID(c.Param("id"))
			if e != nil {
				fail(c, e)
				return
			}
		}
		var r *model.CatalogResult
		if category {
			var in model.CatalogCategoryWrite
			if e = c.ShouldBindJSON(&in); e != nil {
				response.Fail(c, bindMessage(e))
				return
			}
			r, e = service.WriteCatalogCategory(c.Request.Context(), id, in, currentUsername(c))
		} else {
			var in model.CatalogWrite
			if e = c.ShouldBindJSON(&in); e != nil {
				response.Fail(c, bindMessage(e))
				return
			}
			r, e = service.WriteCatalogProduct(c.Request.Context(), id, in, currentUsername(c))
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, r)
	}
}
func CatalogExport(c *gin.Context) {
	var q model.CatalogQuery
	if e := c.ShouldBind(&q); e != nil {
		response.FailDownload(c, response.CodeError, bindMessage(e))
		return
	}
	rows, e := service.ExportCatalog(c.Request.Context(), q)
	if e != nil {
		failDownload(c, e)
		return
	}
	if e = excelx.WriteResponse(c, "商品档案.xlsx", "商品档案", rows); e != nil {
		failDownload(c, e)
	}
}
func CatalogTemplate(c *gin.Context) {
	b, e := service.CatalogTemplate()
	if e != nil {
		failDownload(c, e)
		return
	}
	setAttachmentHeader(c, "商品导入模板.xlsx")
	c.Data(http.StatusOK, excelx.ContentType, b)
}
func CatalogImport(c *gin.Context) {
	fh, e := c.FormFile("file")
	if e != nil || fh.Size > 10<<20 || !strings.EqualFold(filepath.Ext(fh.Filename), ".xlsx") {
		response.Fail(c, "请选择不超过10MB的XLSX文件")
		return
	}
	f, e := fh.Open()
	if e != nil {
		response.Fail(c, "读取导入文件失败")
		return
	}
	defer f.Close()
	r, e := service.ImportCatalog(c.Request.Context(), c.PostForm("requestKey"), f, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, r)
}
func CatalogImageUpload(c *gin.Context) {
	fh, e := c.FormFile("file")
	if e != nil || fh.Size < 1 || fh.Size > 8<<20 {
		response.Fail(c, "请选择不超过8MB的商品图片")
		return
	}
	f, e := fh.Open()
	if e != nil {
		response.Fail(c, "读取图片失败")
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	kind := http.DetectContentType(head[:n])
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	allowed := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}
	if allowed[ext] != kind || kind == "" {
		response.Fail(c, "图片内容与格式不匹配，仅支持PNG/JPEG/GIF/WebP")
		return
	}
	if _, e = f.Seek(0, 0); e != nil {
		response.Fail(c, "读取图片失败")
		return
	}
	cfg, _, e := image.DecodeConfig(f)
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 10000 || cfg.Height > 10000 || int64(cfg.Width)*int64(cfg.Height) > 20000000 {
		response.Fail(c, "图片无效或像素超过2000万")
		return
	}
	fh.Header.Set("Content-Type", kind)
	key, e := saveScopedUpload(c, fh, "catalog")
	if e != nil {
		response.Fail(c, uploadFailureMessage(c, e))
		return
	}
	response.OkData(c, map[string]string{"storageKey": key, "url": key})
}
