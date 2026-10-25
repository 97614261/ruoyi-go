package service

import (
	"context"
	"fmt"
	"path"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"ruoyi-go/pkg/upload"
	"strings"
)

func RecordFile(ctx context.Context, key, name, contentType string, size, userID int64, operator string) error {
	return repository.InsertFile(ctx, &model.SysFile{StorageKey: key, OriginalName: name, ContentType: contentType, SizeBytes: size, UploaderUserID: userID, CreateBy: operator, CreateTime: types.Now()})
}
func AccessibleFile(ctx context.Context, key string) (*model.SysFile, error) {
	if strings.HasPrefix(key, model.ERPImportFilePrefix) {
		return nil, errs.New("文件不存在或无权访问")
	}
	f, err := repository.FindFile(ctx, key)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, errs.New("文件不存在或无权访问")
	}
	return f, nil
}
func ForgetFile(ctx context.Context, key string) error { return repository.DeleteFileRecord(ctx, key) }

// ValidateTemporaryDownload 不把企业共享附件的读权限当成删除权限。
// 临时下载文件必须由当前用户登记在本企业独立的 download 目录；普通上传
// （尤其是商品图片、头像）即使可读，也不能通过下载参数删除。
func ValidateTemporaryDownload(ctx context.Context, f *model.SysFile, userID int64, urlPrefix string) error {
	scope := tenantctx.Get(ctx)
	if f == nil || scope.ID < 1 || scope.Platform || scope.System || userID < 1 ||
		f.FileID < 1 || f.TenantID != scope.ID || f.UploaderUserID != userID {
		return errs.New("文件不存在或无权删除")
	}
	prefix := path.Clean("/"+strings.TrimSpace(urlPrefix)) + fmt.Sprintf("/tenants/%d/%s/", scope.ID, upload.DownloadSubDir)
	if !strings.HasPrefix(f.StorageKey, prefix) || path.Clean(f.StorageKey) != f.StorageKey ||
		strings.ContainsAny(f.StorageKey, "\\?#\x00") || !upload.CheckAllowDownload(f.StorageKey) {
		return errs.New("文件不存在或无权删除")
	}
	return nil
}

// ForgetDownloadedFile 先删除精确的临时文件登记；受外键保护或登记已变时，
// 调用方不得删除物理文件。已发送的下载响应不再改成错误响应。
func ForgetDownloadedFile(ctx context.Context, f *model.SysFile, userID int64, urlPrefix string) (bool, error) {
	if err := ValidateTemporaryDownload(ctx, f, userID, urlPrefix); err != nil {
		return false, err
	}
	return repository.DeleteOwnedFileRecord(ctx, f.FileID, f.StorageKey, userID)
}

func EffectiveUploadLimit(ctx context.Context, global int64) (int64, error) {
	limit, err := repository.CurrentTenantUploadLimit(ctx)
	if err != nil {
		return 0, err
	}
	if limit > 0 && limit < global {
		return limit, nil
	}
	return global, nil
}
