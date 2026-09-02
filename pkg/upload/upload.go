// Package upload 本地文件上传，对齐 Java 版 FileUploadUtils 的落盘规则。
package upload

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ClientError is safe to return to the caller. Filesystem and operating-system
// errors deliberately use ordinary wrapped errors so handlers can hide them.
type ClientError struct{ message string }

func (e *ClientError) Error() string { return e.message }

// ClientMessage returns only messages that cannot expose server internals.
func ClientMessage(err error) (string, bool) {
	var clientErr *ClientError
	if errors.As(err, &clientErr) {
		return clientErr.message, true
	}
	return "", false
}

// ImageExtensions 头像等图片上传允许的扩展名。
//
// 白名单而不是黑名单：黑名单永远漏，比如 .phtml、.svg（可内嵌脚本）。
var ImageExtensions = map[string]bool{
	".bmp": true, ".gif": true, ".jpg": true, ".jpeg": true,
	".png": true, ".webp": true,
}

// DefaultExtensions 通用上传允许的扩展名，对齐 Java 版 MimeTypeUtils.DEFAULT_ALLOWED_EXTENSION。
//
// 仍然是白名单。特别注意**没有** .html/.htm/.svg —— 它们能内嵌脚本，
// 上传后通过 /profile 静态路径访问就是存储型 XSS。
var DefaultExtensions = map[string]bool{
	// 图片
	".bmp": true, ".gif": true, ".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	// 文档
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".pdf": true, ".txt": true,
	// 压缩包
	".zip": true, ".rar": true, ".gz": true, ".bz2": true,
	// 音视频
	".mp3": true, ".mp4": true, ".avi": true, ".rmvb": true,
}

// Options 上传参数。
type Options struct {
	// BaseDir 存储根目录
	BaseDir string
	// SubDir 业务子目录，如 "avatar"
	SubDir string
	// URLPrefix 对外访问前缀，如 "/profile"
	URLPrefix string
	// MaxSize 单文件字节上限，<=0 表示不限制
	MaxSize int64
	// AllowedExt 允许的扩展名（含点、小写），nil 表示不限制
	AllowedExt map[string]bool
	// DatePath 形如 "2026/08/21"，由调用方传入以便测试
	DatePath string
}

// Save 保存上传文件，返回对外访问的相对 URL。
//
// 返回值形如 /profile/avatar/2026/08/21/<uuid>.png，
// 与 Java 版 FileUploadUtils.upload 的产出格式一致。
func Save(fh *multipart.FileHeader, opt Options) (string, error) {
	if fh == nil {
		return "", &ClientError{message: "未选择文件"}
	}
	if opt.MaxSize > 0 && fh.Size > opt.MaxSize {
		return "", &ClientError{message: fmt.Sprintf("文件大小超过上限 %d MB", opt.MaxSize/1024/1024)}
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if opt.AllowedExt != nil && !opt.AllowedExt[ext] {
		return "", &ClientError{message: fmt.Sprintf("不支持的文件格式 %s", ext)}
	}

	// 用 uuid 重命名：原始文件名可能带路径穿越（../）或非法字符，
	// 直接落盘等于把目录结构交给上传者控制
	name := uuid.NewString() + ext
	relDir := path.Join(opt.SubDir, opt.DatePath)
	absDir := filepath.Join(opt.BaseDir, filepath.FromSlash(relDir))

	if err := os.MkdirAll(absDir, 0o750); err != nil {
		return "", fmt.Errorf("创建上传目录失败: %w", err)
	}

	src, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("读取上传文件失败: %w", err)
	}
	defer src.Close()

	absPath := filepath.Join(absDir, name)
	dst, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		// 写了一半失败，把残File 删掉，避免留下损坏文件
		os.Remove(absPath)
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return path.Join(opt.URLPrefix, relDir, name), nil
}
