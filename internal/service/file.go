package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
)

func RecordFile(ctx context.Context, key, name, contentType string, size, userID int64, operator string) error {
	return repository.InsertFile(ctx, &model.SysFile{StorageKey: key, OriginalName: name, ContentType: contentType, SizeBytes: size, UploaderUserID: userID, CreateBy: operator, CreateTime: types.Now()})
}
func AccessibleFile(ctx context.Context, key string) (*model.SysFile, error) {
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
