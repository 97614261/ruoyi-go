package upload

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// DownloadSubDir 通用下载的子目录，对齐 Java 版 RuoYiConfig.getDownloadPath()
// （profile 目录下的 download/）。
const DownloadSubDir = "download"

// CheckAllowDownload 是否允许下载这个文件名。
//
// 对齐 Java 版 FileUtils.checkAllowDownload：禁止 .. 上跳，且扩展名必须在白名单里。
// 白名单复用上传的 DefaultExtensions —— 能传上来的才允许下下去。
func CheckAllowDownload(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	return DefaultExtensions[strings.ToLower(path.Ext(name))]
}

// StripResourcePrefix 去掉资源地址的对外前缀，得到相对存储路径。
//
// 对齐 Java 版 FileUtils.stripPrefix + StringUtils.substringAfter：
// **前缀不存在时返回空串**（不是原样返回），由调用方当作非法路径拒掉。
func StripResourcePrefix(resource, prefix string) string {
	index := strings.Index(resource, prefix)
	if index < 0 {
		return ""
	}
	return resource[index+len(prefix):]
}

// SafeJoin 把相对路径拼到 baseDir 下，并确认结果没有跑出 baseDir。
//
// 【比 Java 版更严】Java 只挡了 .. 和扩展名，没有校验最终路径的归属。
// 绝对路径、软链接、Windows 盘符（C:/Windows/...）这些都绕得过去。
// 这里统一收敛到"必须落在 baseDir 内"，正常下载完全无感。
func SafeJoin(baseDir, relPath string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("解析存储根目录失败: %w", err)
	}

	target, err := filepath.Abs(filepath.Join(absBase, filepath.FromSlash(relPath)))
	if err != nil {
		return "", fmt.Errorf("解析文件路径失败: %w", err)
	}

	if target != absBase && !strings.HasPrefix(target, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("非法的文件路径")
	}
	return target, nil
}

// PercentEncode 百分号编码，对齐 Java 版 FileUtils.percentEncode。
//
// Java 用 URLEncoder 再把 + 换成 %20，Go 的 QueryEscape 行为一致，
// 同样要把 + 换掉：Content-Disposition 里的 + 不会被解回空格。
func PercentEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
