package handler

import (
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/types"
	"ruoyi-go/pkg/upload"
)

// maxUploadFiles 单次批量上传的文件数上限。
const maxUploadFiles = 20

var forgetUploadedFile = service.ForgetFile

// CommonUpload POST /common/upload
//
// 富文本编辑器插图、FileUpload / ImageUpload 组件都走这个接口。
//
// 【平铺字段】url、fileName、newFileName、originalFilename 全在顶层。
func CommonUpload(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, "请选择要上传的文件")
		return
	}

	relURL, err := saveScopedUpload(c, fileHeader, "upload")
	if err != nil {
		response.Fail(c, uploadFailureMessage(c, err))
		return
	}

	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("url", absoluteURL(c, relURL)).
		Put("fileName", relURL).
		Put("newFileName", path.Base(relURL)).
		Put("originalFilename", fileHeader.Filename).
		JSON(c)
}

// CommonUploads POST /common/uploads
//
// 【平铺字段】urls、fileNames、newFileNames、originalFilenames，
// 都是用逗号拼接的字符串（不是数组），与 Java 版一致。
func CommonUploads(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		response.Fail(c, "请选择要上传的文件")
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		response.Fail(c, "请选择要上传的文件")
		return
	}
	if len(files) > maxUploadFiles {
		response.Fail(c, "单次最多上传 20 个文件")
		return
	}

	var urls, fileNames, newFileNames, originalFilenames []string
	saved := make([]string, 0, len(files))
	for _, fileHeader := range files {
		relURL, err := saveScopedUpload(c, fileHeader, "upload")
		if err != nil {
			rollbackUploads(c, saved)
			response.Fail(c, uploadFailureMessage(c, err))
			return
		}
		saved = append(saved, relURL)
		urls = append(urls, absoluteURL(c, relURL))
		fileNames = append(fileNames, relURL)
		newFileNames = append(newFileNames, path.Base(relURL))
		originalFilenames = append(originalFilenames, fileHeader.Filename)
	}

	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("urls", strings.Join(urls, ",")).
		Put("fileNames", strings.Join(fileNames, ",")).
		Put("newFileNames", strings.Join(newFileNames, ",")).
		Put("originalFilenames", strings.Join(originalFilenames, ",")).
		JSON(c)
}

func uploadFailureMessage(c *gin.Context, err error) string {
	if biz := errs.As(err); biz != nil {
		return biz.Msg
	}
	if message, ok := upload.ClientMessage(err); ok {
		return message
	}
	slog.ErrorContext(c.Request.Context(), "保存上传文件失败", "err", err)
	return "文件上传失败，请稍后重试"
}

func rollbackUploads(c *gin.Context, resources []string) {
	for _, resource := range resources {
		removed, err := upload.RemoveManagedFile(uploadCfg.Path, uploadCfg.URLPrefix, tenantFileDir(c, "upload"), resource)
		_ = forgetUploadedFile(c.Request.Context(), resource)
		if err != nil {
			slog.WarnContext(c.Request.Context(), "回滚批量上传文件失败", "resource", resource, "err", err)
			continue
		}
		if !removed {
			slog.WarnContext(c.Request.Context(), "回滚批量上传时文件未找到", "resource", resource)
		}
	}
}

// CommonDownload GET /common/download?fileName=xxx.xlsx&delete=true
//
// 可下载当前企业已登记的文件；delete=true 仅用于当前上传者的
// profile/tenants/<tenantId>/download/ 临时文件，不删除普通附件。
// 前端封装在 plugins/download.js 的 download.name()。
func CommonDownload(c *gin.Context) {
	fileName := c.Query("fileName")
	f, err := service.AccessibleFile(c.Request.Context(), uploadCfg.URLPrefix+"/"+fileName)
	if err != nil {
		response.FailDownload(c, response.CodeForbidden, "文件不存在或无权访问")
		return
	}
	deleteAfter := c.Query("delete") == "true"
	var userID int64
	if user := middleware.CurrentUser(c); user != nil {
		userID = user.UserID
	}
	if deleteAfter {
		if err := service.ValidateTemporaryDownload(c.Request.Context(), f, userID, uploadCfg.URLPrefix); err != nil {
			response.FailDownload(c, response.CodeForbidden, "文件不存在或无权删除")
			return
		}
	}
	if !upload.CheckAllowDownload(fileName) {
		response.FailDownload(c, response.CodeError, "文件名称("+fileName+")非法，不允许下载。")
		return
	}

	absPath, err := upload.SafeJoinExisting(uploadCfg.Path, fileName)
	if err != nil {
		response.FailDownload(c, response.CodeError, "文件名称("+fileName+")非法，不允许下载。")
		return
	}

	// 下载名对齐 Java：时间戳 + 第一个下划线之后的部分。
	// 生成类接口落盘时用的是 <随机前缀>_<真实名称>，这里把前缀还原掉。
	realName := strconv.FormatInt(time.Now().In(types.Location).UnixMilli(), 10) + nameAfterUnderscore(path.Base(fileName))

	if !serveDownload(c, absPath, realName) || !deleteAfter {
		return
	}
	forgotten, err := service.ForgetDownloadedFile(c.Request.Context(), f, userID, uploadCfg.URLPrefix)
	if err != nil || !forgotten {
		slog.WarnContext(c.Request.Context(), "保留临时下载文件：登记未删除", "fileId", f.FileID, "err", err)
		return
	}
	if _, err := upload.RemoveManagedFile(uploadCfg.Path, uploadCfg.URLPrefix, tenantFileDir(c, upload.DownloadSubDir), f.StorageKey); err != nil {
		slog.WarnContext(c.Request.Context(), "删除临时下载文件失败", "fileId", f.FileID, "err", err)
	}
}

// CommonDownloadResource GET /common/download/resource?resource=/profile/upload/2026/08/22/x.png
//
// 下载已上传的资源，resource 就是入库时存的那个相对地址。
// 前端封装在 plugins/download.js 的 download.resource()。
func CommonDownloadResource(c *gin.Context) {
	resource := c.Query("resource")
	if _, err := service.AccessibleFile(c.Request.Context(), resource); err != nil {
		response.FailDownload(c, response.CodeForbidden, "文件不存在或无权访问")
		return
	}
	if !upload.CheckAllowDownload(resource) {
		response.FailDownload(c, response.CodeError, "资源文件("+resource+")非法，不允许下载。")
		return
	}

	// 前缀不存在时 StripResourcePrefix 返回空串，落到 SafeJoin 会指向根目录本身，
	// 下面的 IsDir 判断会把它挡掉
	relPath := upload.StripResourcePrefix(resource, uploadCfg.URLPrefix)
	absPath, err := upload.SafeJoinExisting(uploadCfg.Path, relPath)
	if err != nil {
		response.FailDownload(c, response.CodeError, "资源文件("+resource+")非法，不允许下载。")
		return
	}

	serveDownload(c, absPath, path.Base(filepath.ToSlash(absPath)))
}

// serveDownload 把文件写回响应。
//
// 【出错时必须走 FailDownload】前端的 blobValidate() 是严格相等比较
// data.type !== 'application/json'，带 charset 的都会被当成文件存下来，
// 用户拿到一个打不开的文件且看不到任何提示。
//
// 【用 c.File 而不是先 ReadFile】下载的可能是几百 MB 的导出包，
// 整个读进内存等于把进程撑爆。所以先 Stat 确认存在再流式写。
// 只有完整GET成功写回且请求未取消才允许调用方继续清理临时文件。
// Range、HEAD和304不会触发清理；成功写回也不代表客户端已保存到磁盘。
func serveDownload(c *gin.Context, absPath, downloadName string) bool {
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		// 不回显磁盘路径，只说文件不存在
		response.FailDownload(c, response.CodeError, "文件不存在或已被删除")
		return false
	}

	setAttachmentHeader(c, downloadName)
	c.Header("Content-Type", "application/octet-stream")
	c.File(absPath)

	return c.Request.Method == http.MethodGet && c.Request.Header.Get("Range") == "" &&
		c.Request.Context().Err() == nil && c.Writer.Status() == http.StatusOK &&
		int64(c.Writer.Size()) == info.Size() && len(c.Errors) == 0
}

// setAttachmentHeader 设置下载响应头，对齐 Java 版 FileUtils.setAttachmentResponseHeader。
//
// download-filename 这个自定义头是前端读取文件名的唯一来源
// （plugins/download.js 里 decodeURIComponent(res.headers['download-filename'])），
// 且必须在 Access-Control-Expose-Headers 里放行，否则跨域时 JS 读不到。
func setAttachmentHeader(c *gin.Context, fileName string) {
	encoded := upload.PercentEncode(fileName)
	c.Header("Content-Disposition", "attachment; filename="+encoded+";filename*=utf-8''"+encoded)
	c.Header("download-filename", encoded)
	c.Header("Access-Control-Expose-Headers", "Content-Disposition,download-filename")
}

// nameAfterUnderscore 取第一个下划线之后的部分，没有下划线则返回原串。
func nameAfterUnderscore(name string) string {
	if index := strings.Index(name, "_"); index >= 0 {
		return name[index+1:]
	}
	return name
}

// Legacy low-level test helper. HTTP handlers must use saveScopedUpload so
// tenant paths and file ownership records are both present.
func saveUpload(fileHeader *multipart.FileHeader) (string, error) {
	return upload.Save(fileHeader, upload.Options{
		BaseDir:    uploadCfg.Path,
		SubDir:     "upload",
		URLPrefix:  uploadCfg.URLPrefix,
		MaxSize:    uploadCfg.MaxSizeMB * 1024 * 1024,
		AllowedExt: upload.DefaultExtensions,
		DatePath:   time.Now().In(types.Location).Format("2006/01/02"),
	})
}

// absoluteURL 在多租户前端返回同源相对URL，图片请求经代理并携带媒体会话。
// 不再相信任意 X-Forwarded-Proto 拼出另一个来源的公开文件地址。
func absoluteURL(c *gin.Context, relURL string) string {
	return relURL
}
