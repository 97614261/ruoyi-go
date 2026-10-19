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
