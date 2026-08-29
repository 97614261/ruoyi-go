package upload

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// RemoveManagedFile 删除指定业务子目录内、由本服务生成的资源文件。
// 外部 URL、其他业务目录和不属于 URLPrefix 的资源会被安全跳过。
func RemoveManagedFile(baseDir, urlPrefix, subDir, resource string) (bool, error) {
	cleanPrefix := path.Clean("/" + strings.TrimSpace(urlPrefix))
	cleanSubDir := path.Clean(strings.TrimSpace(subDir))
	if cleanSubDir == "." || cleanSubDir == ".." || path.IsAbs(cleanSubDir) ||
		strings.HasPrefix(cleanSubDir, "../") {
		return false, fmt.Errorf("非法的资源子目录")
	}
	if strings.ContainsAny(resource, "?#") || !strings.HasPrefix(resource, cleanPrefix+"/") {
		return false, nil
	}

	relative := strings.TrimPrefix(resource, cleanPrefix+"/")
	subPrefix := cleanSubDir + "/"
	if !strings.HasPrefix(relative, subPrefix) {
		return false, nil
	}
	innerPath := strings.TrimPrefix(relative, subPrefix)
	if innerPath == "" {
		return false, nil
	}

	managedBase, err := filepath.Abs(filepath.Join(baseDir, filepath.FromSlash(cleanSubDir)))
	if err != nil {
		return false, fmt.Errorf("解析资源根目录失败: %w", err)
	}
	target, err := SafeJoin(managedBase, innerPath)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("检查待删除文件失败: %w", err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("拒绝删除目录")
	}

	// 最终文件即使是软链接，os.Remove 也只删除链接本身；真正危险的是中间目录
	// 软链接把子路径带出 avatar 根目录，因此逐级 Lstat 并拒绝这种目录。
	if err := rejectSymlinkParents(managedBase, filepath.Dir(target)); err != nil {
		return false, err
	}

	if err := os.Remove(target); err != nil {
		return false, fmt.Errorf("删除资源文件失败: %w", err)
	}
	return true, nil
}

func rejectSymlinkParents(baseDir, parentDir string) error {
	if !isWithin(baseDir, parentDir) {
		return fmt.Errorf("待删除文件不属于资源目录")
	}
	relative, err := filepath.Rel(baseDir, parentDir)
	if err != nil {
		return fmt.Errorf("解析资源父目录失败: %w", err)
	}

	current := baseDir
	parts := []string{"."}
	if relative != "." {
		parts = append(parts, strings.Split(relative, string(filepath.Separator))...)
	}
	for _, part := range parts {
		if part != "." {
			current = filepath.Join(current, part)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("检查资源父目录失败: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("资源父目录不能是软链接")
		}
	}
	return nil
}

func isWithin(baseDir, target string) bool {
	relative, err := filepath.Rel(baseDir, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
