package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/service"
)

func TestReconcileAvatarFiles(t *testing.T) {
	previousConfig := uploadCfg
	uploadCfg = config.UploadConfig{Path: t.TempDir(), URLPrefix: "/profile"}
	t.Cleanup(func() { uploadCfg = previousConfig })

	t.Run("数据库未提交时回收新头像", func(t *testing.T) {
		newAvatar := "/profile/avatar/new.png"
		newPath := writeHandlerAvatar(t, "new.png")
		reconcileAvatarFiles(context.Background(), newAvatar, service.AvatarUpdateResult{})
		if _, err := os.Stat(newPath); !os.IsNotExist(err) {
			t.Fatalf("未入库的新头像应被回收：%v", err)
		}
	})

	t.Run("数据库已提交时删除旧头像并保留新头像", func(t *testing.T) {
		oldAvatar := "/profile/avatar/old.png"
		newAvatar := "/profile/avatar/current.png"
		oldPath := writeHandlerAvatar(t, "old.png")
		newPath := writeHandlerAvatar(t, "current.png")

		reconcileAvatarFiles(context.Background(), newAvatar, service.AvatarUpdateResult{
			OldAvatar: oldAvatar,
			Persisted: true,
		})
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Fatalf("已替换的旧头像应被删除：%v", err)
		}
		if _, err := os.Stat(newPath); err != nil {
			t.Fatalf("数据库引用的新头像必须保留：%v", err)
		}
	})

	t.Run("新旧地址相同时不删除当前头像", func(t *testing.T) {
		avatar := "/profile/avatar/same.png"
		avatarPath := writeHandlerAvatar(t, "same.png")
		reconcileAvatarFiles(context.Background(), avatar, service.AvatarUpdateResult{
			OldAvatar: avatar,
			Persisted: true,
		})
		if _, err := os.Stat(avatarPath); err != nil {
			t.Fatalf("相同地址仍被数据库引用，不能删除：%v", err)
		}
	})
}

func writeHandlerAvatar(t *testing.T, name string) string {
	t.Helper()
	target := filepath.Join(uploadCfg.Path, "avatar", name)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("创建头像目录失败：%v", err)
	}
	if err := os.WriteFile(target, []byte("avatar"), 0o640); err != nil {
		t.Fatalf("写头像测试文件失败：%v", err)
	}
	return target
}
