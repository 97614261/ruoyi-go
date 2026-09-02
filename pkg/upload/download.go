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

// SafeJoin 把相对路径拼到 baseDir 下，并执行词法边界检查。
//
// 需要打开现有文件时必须继续调用 SafeJoinExisting，不能把词法检查误当成
// 真实路径检查；清理不存在的托管文件仍需要本函数只负责安全拼接。
func SafeJoin(baseDir, relPath string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("解析存储根目录失败: %w", err)
	}

	target, err := filepath.Abs(filepath.Join(absBase, filepath.FromSlash(relPath)))
	if err != nil {
		return "", fmt.Errorf("解析文件路径失败: %w", err)
	}

	if !isWithin(absBase, target) {
		return "", fmt.Errorf("非法的文件路径")
	}
	return target, nil
}

// SafeJoinExisting 在词法边界检查后解析真实路径，供下载现有文件使用。
// 返回解析后的路径，后续打开文件时不会再经过用户可控的软链接入口。
func SafeJoinExisting(baseDir, relPath string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("解析存储根目录失败: %w", err)
	}
	target, err := SafeJoin(absBase, relPath)
	if err != nil {
		return "", err
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("解析存储根目录真实路径失败: %w", err)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("解析文件真实路径失败: %w", err)
	}
	if !isWithin(realBase, realTarget) {
		return "", fmt.Errorf("文件真实路径不属于存储根目录")
	}
	return realTarget, nil
}

// PercentEncode 百分号编码，对齐 Java 版 FileUtils.percentEncode。
//
// Java 用 URLEncoder 再把 + 换成 %20，Go 的 QueryEscape 行为一致，
// 同样要把 + 换掉：Content-Disposition 里的 + 不会被解回空格。
func PercentEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
