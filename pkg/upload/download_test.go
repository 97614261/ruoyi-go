package upload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinExistingResolvesRegularFile(t *testing.T) {
	baseDir := t.TempDir()
	target := filepath.Join(baseDir, "nested", "report.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	if err := os.WriteFile(target, []byte("report"), 0o640); err != nil {
		t.Fatalf("创建文件失败：%v", err)
	}

	resolved, err := SafeJoinExisting(baseDir, "nested/report.txt")
	if err != nil {
		t.Fatalf("解析普通文件失败：%v", err)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("解析预期路径失败：%v", err)
	}
	if resolved != realTarget {
		t.Fatalf("应返回文件真实路径，实际 %q，期望 %q", resolved, realTarget)
	}
}

func TestSafeJoinExistingRejectsSymlinkEscapes(t *testing.T) {
	t.Run("最终文件软链接", func(t *testing.T) {
		baseDir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.txt")
		if err := os.WriteFile(outside, []byte("secret"), 0o640); err != nil {
			t.Fatalf("创建目录外文件失败：%v", err)
		}
		link := filepath.Join(baseDir, "report.txt")
		createSymlinkOrSkip(t, outside, link)
		if _, err := SafeJoinExisting(baseDir, "report.txt"); err == nil {
			t.Fatal("指向存储目录外的文件软链接必须被拒绝")
		}
	})

	t.Run("中间目录软链接", func(t *testing.T) {
		baseDir := t.TempDir()
		outsideDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(outsideDir, "report.txt"), []byte("secret"), 0o640); err != nil {
			t.Fatalf("创建目录外文件失败：%v", err)
		}
		createSymlinkOrSkip(t, outsideDir, filepath.Join(baseDir, "linked"))
		if _, err := SafeJoinExisting(baseDir, "linked/report.txt"); err == nil {
			t.Fatal("指向存储目录外的中间目录软链接必须被拒绝")
		}
	})
}

func createSymlinkOrSkip(t *testing.T, oldName, newName string) {
	t.Helper()
	if err := os.Symlink(oldName, newName); err != nil {
		t.Skipf("当前平台不允许创建软链接：%v", err)
	}
}
