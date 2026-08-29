package upload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveManagedFile(t *testing.T) {
	baseDir := t.TempDir()
	target := writeCleanupTestFile(t, baseDir, "avatar/2026/08/28/current.png")

	removed, err := RemoveManagedFile(baseDir, "/profile", "avatar",
		"/profile/avatar/2026/08/28/current.png")
	if err != nil {
		t.Fatalf("删除托管头像失败：%v", err)
	}
	if !removed {
		t.Fatal("存在的托管头像应报告已删除")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("托管头像删除后仍存在：%v", err)
	}
}

func TestRemoveManagedFileRejectsUnmanagedResources(t *testing.T) {
	baseDir := t.TempDir()
	outside := writeCleanupTestFile(t, baseDir, "outside.png")
	cases := []struct {
		name     string
		resource string
	}{
		{name: "外部 URL", resource: "https://example.com/profile/avatar/outside.png"},
		{name: "相似前缀", resource: "/profile-other/avatar/outside.png"},
		{name: "其他业务目录", resource: "/profile/upload/outside.png"},
		{name: "带查询参数", resource: "/profile/avatar/outside.png?download=1"},
		{name: "空资源", resource: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			removed, err := RemoveManagedFile(baseDir, "/profile", "avatar", tt.resource)
			if err != nil {
				t.Fatalf("非托管资源应安全跳过：%v", err)
			}
			if removed {
				t.Fatal("非托管资源不应报告已删除")
			}
		})
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("目录外文件不应受影响：%v", err)
	}
}

func TestRemoveManagedFileRejectsTraversalAndDirectories(t *testing.T) {
	baseDir := t.TempDir()
	outside := writeCleanupTestFile(t, baseDir, "outside.png")
	if _, err := RemoveManagedFile(baseDir, "/profile", "avatar",
		"/profile/avatar/../outside.png"); err == nil {
		t.Fatal("目录上跳必须被拒绝")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("目录上跳不能删除外部文件：%v", err)
	}

	directory := filepath.Join(baseDir, "avatar", "keep")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatalf("创建测试目录失败：%v", err)
	}
	if _, err := RemoveManagedFile(baseDir, "/profile", "avatar",
		"/profile/avatar/keep"); err == nil {
		t.Fatal("目录不能按头像文件删除")
	}
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("被拒绝的目录应保持不变：info=%v err=%v", info, err)
	}
}

func writeCleanupTestFile(t *testing.T, baseDir, relative string) string {
	t.Helper()
	target := filepath.Join(baseDir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("创建测试目录失败：%v", err)
	}
	if err := os.WriteFile(target, []byte("avatar"), 0o640); err != nil {
		t.Fatalf("写测试文件失败：%v", err)
	}
	return target
}
