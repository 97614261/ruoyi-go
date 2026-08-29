package handler

import (
	"log/slog"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/types"
	"ruoyi-go/pkg/upload"
)

// maxUploadFiles 单次批量上传的文件数上限。
const maxUploadFiles = 20

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

	relURL, err := saveUpload(fileHeader)
	if err != nil {
		response.Fail(c, err.Error())
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
		relURL, err := saveUpload(fileHeader)
		if err != nil {
			rollbackUploads(c, saved)
			response.Fail(c, err.Error())
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

func rollbackUploads(c *gin.Context, resources []string) {
	for _, resource := range resources {
		removed, err := upload.RemoveManagedFile(uploadCfg.Path, uploadCfg.URLPrefix, "upload", resource)
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
// 下载 profile/download/ 下的临时文件，通常是别的接口刚生成的。
// 前端封装在 plugins/download.js 的 download.name()。
func CommonDownload(c *gin.Context) {
	fileName := c.Query("fileName")
	if !upload.CheckAllowDownload(fileName) {
		response.FailDownload(c, response.CodeError, "文件名称("+fileName+")非法，不允许下载。")
		return
	}

	absPath, err := upload.SafeJoin(filepath.Join(uploadCfg.Path, upload.DownloadSubDir), fileName)
	if err != nil {
		response.FailDownload(c, response.CodeError, "文件名称("+fileName+")非法，不允许下载。")
		return
	}

	// 下载名对齐 Java：时间戳 + 第一个下划线之后的部分。
	// 生成类接口落盘时用的是 <随机前缀>_<真实名称>，这里把前缀还原掉。
	realName := strconv.FormatInt(time.Now().In(types.Location).UnixMilli(), 10) + nameAfterUnderscore(fileName)

	serveDownload(c, absPath, realName, c.Query("delete") == "true")
}

// CommonDownloadResource GET /common/download/resource?resource=/profile/upload/2026/08/22/x.png
//
// 下载已上传的资源，resource 就是入库时存的那个相对地址。
// 前端封装在 plugins/download.js 的 download.resource()。
func CommonDownloadResource(c *gin.Context) {
	resource := c.Query("resource")
	if !upload.CheckAllowDownload(resource) {
		response.FailDownload(c, response.CodeError, "资源文件("+resource+")非法，不允许下载。")
		return
	}

	// 前缀不存在时 StripResourcePrefix 返回空串，落到 SafeJoin 会指向根目录本身，
	// 下面的 IsDir 判断会把它挡掉
	relPath := upload.StripResourcePrefix(resource, uploadCfg.URLPrefix)
	absPath, err := upload.SafeJoin(uploadCfg.Path, relPath)
	if err != nil {
		response.FailDownload(c, response.CodeError, "资源文件("+resource+")非法，不允许下载。")
		return
	}

	serveDownload(c, absPath, path.Base(filepath.ToSlash(absPath)), false)
}

// serveDownload 把文件写回响应。
//
// 【出错时必须走 FailDownload】前端的 blobValidate() 是严格相等比较
// data.type !== 'application/json'，带 charset 的都会被当成文件存下来，
// 用户拿到一个打不开的文件且看不到任何提示。
//
// 【用 c.File 而不是先 ReadFile】下载的可能是几百 MB 的导出包，
// 整个读进内存等于把进程撑爆。所以先 Stat 确认存在再流式写。
func serveDownload(c *gin.Context, absPath, downloadName string, deleteAfter bool) {
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		// 不回显磁盘路径，只说文件不存在
		response.FailDownload(c, response.CodeError, "文件不存在或已被删除")
		return
	}

	setAttachmentHeader(c, downloadName)
	c.Header("Content-Type", "application/octet-stream")
	c.File(absPath)

	if deleteAfter {
		// 删除失败不影响本次下载，只记日志
		if err := os.Remove(absPath); err != nil {
			slog.WarnContext(c.Request.Context(), "删除临时下载文件失败", "err", err)
		}
	}
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

// absoluteURL 拼出带协议和域名的完整地址。
//
// Java 版用 ServerConfig.getUrl() 拼，富文本编辑器的 <img src> 需要完整地址。
// 走反代时优先信 X-Forwarded-Proto，否则按 TLS 状态判断。
func absoluteURL(c *gin.Context, relURL string) string {
	scheme := "http"
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + relURL
}
