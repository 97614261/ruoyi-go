//go:build !windows

package upload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveManagedFileDoesNotFollowFinalSymlink(t *testing.T) {
	baseDir := t.TempDir()
	outside := writeCleanupTestFile(t, baseDir, "outside.png")
	linkPath := filepath.Join(baseDir, "avatar", "link.png")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o750); err != nil {
		t.Fatalf("创建头像目录失败：%v", err)
	}
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("创建最终文件软链接失败：%v", err)
	}

	removed, err := RemoveManagedFile(baseDir, "/profile", "avatar", "/profile/avatar/link.png")
	if err != nil || !removed {
		t.Fatalf("应只删除头像目录中的软链接：removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("软链接指向的外部文件不能被删除：%v", err)
	}
}

func TestRemoveManagedFileRejectsSymlinkParent(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := filepath.Join(t.TempDir(), "outside")
	victim := writeCleanupTestFile(t, outsideDir, "victim.png")
	linkDir := filepath.Join(baseDir, "avatar", "linked")
	if err := os.MkdirAll(filepath.Dir(linkDir), 0o750); err != nil {
		t.Fatalf("创建头像目录失败：%v", err)
	}
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatalf("创建父目录软链接失败：%v", err)
	}

	if _, err := RemoveManagedFile(baseDir, "/profile", "avatar",
		"/profile/avatar/linked/victim.png"); err == nil {
		t.Fatal("中间目录软链接必须被拒绝")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("目录外文件不能被删除：%v", err)
	}
}
