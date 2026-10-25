package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
)

func InsertFile(ctx context.Context, f *model.SysFile) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := checkFileQuota(tx, f.SizeBytes); err != nil {
			return err
		}
		return tx.Create(f).Error
	})
}
func FindFile(ctx context.Context, key string) (*model.SysFile, error) {
	var f model.SysFile
	err := DB(ctx).Where("storage_key = ?", key).Take(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &f, err
}
func DeleteFileRecord(ctx context.Context, key string) error {
	return DB(ctx).Where("storage_key = ?", key).Delete(&model.SysFile{}).Error
}

// DeleteOwnedFileRecord 使用读取时的登记ID和上传者，避免删除重建后的同名登记。
// DB(ctx) 继续施加企业隔离，数据库外键继续保护仍被业务引用的附件。
func DeleteOwnedFileRecord(ctx context.Context, fileID int64, key string, userID int64) (bool, error) {
	result := DB(ctx).Where("file_id = ? AND storage_key = ? AND uploader_user_id = ?", fileID, key, userID).Delete(&model.SysFile{})
	return result.RowsAffected == 1, result.Error
}
