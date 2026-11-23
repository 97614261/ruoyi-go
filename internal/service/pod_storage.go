package service

import (
	"context"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/assetimage"
)

func (s *PODAssetService) storePrepared(ctx context.Context, user int64, key string, p *assetimage.Prepared, archive bool) (string, string, error) {
	if err := repository.PODReserveStorage(ctx, user, key, int64(len(p.Original)+len(p.Thumbnail)), archive); err != nil {
		return "", "", err
	}
	original, err := s.Store.Put(ctx, p.Extension, p.Original)
	if err != nil {
		return "", "", err
	}
	thumb, err := s.Store.Put(ctx, "png", p.Thumbnail)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), assetCleanupTimeout)
		defer cancel()
		if s.Store.Remove(cleanup, original) == nil {
			_ = repository.PODReleaseStorage(cleanup, user, key)
		}
		return "", "", err
	}
	return original, thumb, nil
}
