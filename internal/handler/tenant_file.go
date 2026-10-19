package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"path"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"ruoyi-go/pkg/upload"
	"time"
)

func tenantFileDir(c *gin.Context, kind string) string {
	return fmt.Sprintf("tenants/%d/%s", tenantctx.ID(c.Request.Context()), kind)
}
func saveScopedUpload(c *gin.Context, fh *multipart.FileHeader, kind string) (string, error) {
	if _, err := tenantctx.Require(c.Request.Context()); err != nil {
		return "", err
	}
	ext := upload.DefaultExtensions
	if kind == "avatar" {
		ext = upload.ImageExtensions
	}
	maxSize, err := service.EffectiveUploadLimit(c.Request.Context(), uploadCfg.MaxSizeMB<<20)
	if err != nil {
		return "", err
	}
	url, err := upload.Save(fh, upload.Options{BaseDir: uploadCfg.Path, SubDir: tenantFileDir(c, kind), URLPrefix: uploadCfg.URLPrefix, MaxSize: maxSize, AllowedExt: ext, DatePath: time.Now().In(types.Location).Format("2006/01/02")})
	if err != nil {
		return "", err
	}
	u := middleware.CurrentUser(c)
	if err = service.RecordFile(c.Request.Context(), url, fh.Filename, fh.Header.Get("Content-Type"), fh.Size, u.UserID, u.User.UserName); err != nil {
		_, _ = upload.RemoveManagedFile(uploadCfg.Path, uploadCfg.URLPrefix, tenantFileDir(c, kind), url)
		return "", err
	}
	return url, nil
}
func TenantMedia(c *gin.Context) {
	resource := c.Request.URL.Path
	f, err := service.AccessibleFile(c.Request.Context(), resource)
	if err != nil {
		response.FailDownload(c, response.CodeForbidden, "文件不存在或无权访问")
		return
	}
	abs, err := upload.SafeJoinExisting(uploadCfg.Path, upload.StripResourcePrefix(f.StorageKey, uploadCfg.URLPrefix))
	if err != nil {
		response.FailDownload(c, response.CodeForbidden, "文件不存在或无权访问")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !upload.ImageExtensions[path.Ext(f.StorageKey)] {
		setAttachmentHeader(c, f.OriginalName)
	}
	c.File(abs)
}
