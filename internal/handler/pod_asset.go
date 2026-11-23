package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/storage"
	"strconv"
)

type PODAssetHandler struct{ Service *service.PODAssetService }

func podAssetID(c *gin.Context) (int64, bool) {
	value := c.Param("id")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		response.FailCode(c, 400, "素材编号不合法")
		return 0, false
	}
	return id, true
}
func (h PODAssetHandler) List(c *gin.Context) {
	h.list(c, 0)
}

func (h PODAssetHandler) ListProductPages(c *gin.Context) {
	if id, ok := podProjectID(c); ok {
		h.list(c, id)
	}
}

func (h PODAssetHandler) list(c *gin.Context, projectID int64) {
	c.Header("Cache-Control", "private, no-store")
	deleted := c.Query("deleted")
	if deleted != "" && deleted != "true" && deleted != "false" {
		response.FailCode(c, 400, "回收站参数必须为 true 或 false")
		return
	}
	rows, total, err := service.ListPODAssetPage(c.Request.Context(), currentUser(c), projectID, deleted == "true", page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (h PODAssetHandler) Get(c *gin.Context) {
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	row, err := service.GetPODAsset(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODAssetHandler) Delete(c *gin.Context)  { h.deletion(c, false) }
func (h PODAssetHandler) Restore(c *gin.Context) { h.deletion(c, true) }
func (h PODAssetHandler) deletion(c *gin.Context, restore bool) {
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	var body model.PODProjectVersion
	if !podBind(c, &body) {
		return
	}
	row, err := service.ChangePODAssetDeletion(c.Request.Context(), currentUser(c), id, body.Revision, restore)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}

// singleUpload rejects additional fields/files before an upload can be committed.
type singleUpload struct {
	part   *multipart.Part
	reader *multipart.Reader
	done   bool
}

func (s *singleUpload) Read(p []byte) (int, error) {
	n, err := s.part.Read(p)
	if err == io.EOF && !s.done {
		s.done = true
		_, next := s.reader.NextPart()
		if next != io.EOF {
			return n, errors.New("only one file is allowed")
		}
	}
	return n, err
}
func (h PODAssetHandler) Upload(c *gin.Context) {
	h.upload(c, 0)
}

func (h PODAssetHandler) UploadProductPage(c *gin.Context) {
	if id, ok := podProjectID(c); ok {
		h.upload(c, id)
	}
}

func (h PODAssetHandler) upload(c *gin.Context, projectID int64) {
	c.Header("Cache-Control", "private, no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.Service.Limits.MaxUploadBytes+(64<<10))
	reader, err := c.Request.MultipartReader()
	if err != nil {
		response.FailCode(c, 400, "请使用 multipart/form-data 上传图片")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		response.FailCode(c, 400, "请选择一张图片，文件字段名为 file")
		return
	}
	defer part.Close()
	_, parameters, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		response.FailCode(c, 400, "上传文件名不合法")
		return
	}
	// multipart.FileName strips path components on the host OS. Validate the original value first.
	file := &singleUpload{part: part, reader: reader}
	var row *model.PODAssetView
	if projectID > 0 {
		row, err = h.Service.CreateProductPage(c.Request.Context(), currentUser(c), projectID, parameters["filename"], part.Header.Get("Content-Type"), file)
	} else {
		row, err = h.Service.Create(c.Request.Context(), currentUser(c), parameters["filename"], part.Header.Get("Content-Type"), file)
	}
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, row)
}
func (h PODAssetHandler) Content(c *gin.Context)  { h.content(c, false) }
func (h PODAssetHandler) Download(c *gin.Context) { h.content(c, true) }
func (h PODAssetHandler) content(c *gin.Context, download bool) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	id, ok := podAssetID(c)
	if !ok {
		return
	}
	variant := c.DefaultQuery("variant", "thumbnail")
	if download {
		variant = "original"
	}
	if variant != "original" && variant != "thumbnail" {
		response.FailCode(c, 400, "素材文件类型不合法")
		return
	}
	file, meta, name, err := h.Service.Open(c.Request.Context(), currentUser(c), id, variant)
	if err != nil {
		fail(c, err)
		return
	}
	defer file.Close()
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	if variant == "thumbnail" {
		name = "preview.png"
	}
	c.Header("Content-Type", meta.MediaType)
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	c.Header("Content-Length", strconv.FormatInt(meta.ByteSize, 10))
	c.Status(http.StatusOK)
	if _, err := storage.Copy(c.Request.Context(), c.Writer, file); err != nil {
		slog.WarnContext(c.Request.Context(), "private asset stream interrupted")
	}
}
