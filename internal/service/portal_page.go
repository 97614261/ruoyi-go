package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

func PortalResults(ctx context.Context, kind, channel, keyword string, offset, limit int) (*model.PortalResults, error) {
	return repository.PortalResults(ctx, kind, channel, keyword, offset, limit)
}

func PortalNotFound() error { return errs.ErrNotFound }
