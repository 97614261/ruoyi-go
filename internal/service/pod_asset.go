package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/assetimage"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/storage"
	"ruoyi-go/pkg/types"
	"time"
)

type PODAssetService struct {
	Store  storage.Store
	Limits model.PODAssetLimits
}

var podUploadSlots = make(chan struct{}, 2)

const assetCleanupTimeout = 5 * time.Second

func DefaultPODAssetLimits(l model.PODAssetLimits) model.PODAssetLimits {
	if l.MaxUploadBytes == 0 {
		l.MaxUploadBytes = 10 << 20
	}
	if l.MaxPixels == 0 {
		l.MaxPixels = 24_000_000
	}
	l.MaxDimension = 12000
	l.MaxImagesPerNode = 8
	return l
}
func NewPODAssetService(root string, l model.PODAssetLimits) *PODAssetService {
	return &PODAssetService{Store: storage.Local{Path: root}, Limits: DefaultPODAssetLimits(l)}
}
func podAssetError(err error) error {
	switch {
	case errors.Is(err, repository.ErrPODAsset):
		return errs.WithCode(404, "素材不存在或已移入回收站")
	case errors.Is(err, repository.ErrPODAssetReference):
		return errs.WithCode(409, "素材仍被项目、商品素材包、商品定稿、模板、批次或历史运行引用；回收站和模板历史版本也会保留引用")
	case errors.Is(err, repository.ErrPODAssetGraph):
		return errs.WithCode(400, "画布素材不存在、已删除或不属于当前空间")
	case errors.Is(err, repository.ErrPODRevision):
		return errs.WithCode(409, "素材状态已在其他页面变更，请刷新素材列表后重试")
	default:
		return podProjectError(err)
	}
}
func podAssetView(row *model.PODAsset) model.PODAssetView {
	return model.PODAssetView{AssetID: row.AssetID, WorkspaceID: row.WorkspaceID, ParentAssetID: row.ParentAssetID, Source: row.Source, OriginalName: row.OriginalName, MediaType: row.MediaType, ByteSize: row.ByteSize, Width: row.Width, Height: row.Height, Revision: row.Revision, Deleted: row.DelFlag == "2", ReferenceCount: row.ReferenceCount, CreateTime: row.CreateTime}
}
func ListPODAssetPage(ctx context.Context, user *model.SysUser, projectID int64, deleted bool, pg page.Query) ([]model.PODAssetView, int64, error) {
	if user == nil {
		return nil, 0, errs.ErrUnauthorized
	}
	workspace, err := repository.PODPersonalWorkspace(ctx, user.UserID)
	if err != nil {
		return nil, 0, err
	}
	if workspace == nil {
		return nil, 0, podAssetError(repository.ErrPODWorkspace)
	}
	pg.PageNum = max(1, pg.PageNum)
	if pg.PageSize < 1 {
		pg.PageSize = 10
	}
	pg.PageSize = min(100, pg.PageSize)
	rows, total, err := repository.SelectPODAssetPage(ctx, user.UserID, workspace.WorkspaceID, projectID, deleted, pg)
	if err != nil {
		return nil, 0, podAssetError(err)
	}
	result := make([]model.PODAssetView, 0, len(rows))
	for i := range rows {
		result = append(result, podAssetView(&rows[i]))
	}
	return result, total, nil
}
func GetPODAsset(ctx context.Context, user *model.SysUser, id int64) (*model.PODAssetView, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	row, err := repository.SelectPODAsset(ctx, userID, id)
	if err != nil {
		return nil, podAssetError(err)
	}
	view := podAssetView(row)
	return &view, nil
}
func ChangePODAssetDeletion(ctx context.Context, user *model.SysUser, id int64, value string, restore bool) (*model.PODAssetView, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	revision, err := podRevision(value)
	if err != nil {
		return nil, err
	}
	row, err := repository.ChangePODAssetDeletion(ctx, userID, id, revision, restore, user.UserName)
	if err != nil {
		return nil, podAssetError(err)
	}
	view := podAssetView(row)
	return &view, nil
}
func (s *PODAssetService) Create(ctx context.Context, user *model.SysUser, name, declared string, r io.Reader) (*model.PODAssetView, error) {
	return s.create(ctx, user, 0, name, declared, r)
}

func (s *PODAssetService) CreateProductPage(ctx context.Context, user *model.SysUser, projectID int64, name, declared string, r io.Reader) (*model.PODAssetView, error) {
	if projectID <= 0 {
		return nil, podAssetError(repository.ErrPODProject)
	}
	return s.create(ctx, user, projectID, name, declared, r)
}

func (s *PODAssetService) create(ctx context.Context, user *model.SysUser, projectID int64, name, declared string, r io.Reader) (*model.PODAssetView, error) {
	if user == nil || user.UserID <= 0 {
		return nil, errs.ErrUnauthorized
	}
	userID := user.UserID
	workspace, err := repository.PODPersonalWorkspace(ctx, userID)
	if err != nil {
		return nil, err
	}
	if workspace == nil {
		return nil, podAssetError(repository.ErrPODWorkspace)
	}
	select {
	case podUploadSlots <- struct{}{}:
		defer func() { <-podUploadSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, errs.WithCode(429, "正在处理其他图片，请稍后重试")
	}
	prepared, err := assetimage.Prepare(ctx, r, name, declared, assetimage.Limits{MaxBytes: s.Limits.MaxUploadBytes, MaxPixels: s.Limits.MaxPixels, MaxDimension: s.Limits.MaxDimension})
	if err != nil {
		if errors.Is(err, assetimage.ErrImage) || errors.Is(err, assetimage.ErrSize) || errors.Is(err, assetimage.ErrName) {
			return nil, errs.WithCode(400, err.Error())
		}
		return nil, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(prepared.Original))
	if projectID != 0 {
		if prepared.MediaType != "image/png" {
			return nil, errs.WithCode(400, "排版成品只支持 PNG")
		}
		existing, err := repository.FindPODProductPageAsset(ctx, workspace.WorkspaceID, projectID, prepared.Name, digest)
		if err != nil {
			return nil, podAssetError(err)
		}
		if existing != nil {
			view := podAssetView(existing)
			return &view, nil
		}
	}
	reservation := "upload:" + uuid.NewString()
	original, thumb, err := s.storePrepared(ctx, userID, reservation, prepared, false)
	if err != nil {
		return nil, err
	}
	now := types.Now()
	asset := &model.PODAsset{Source: "upload", OriginalName: prepared.Name, MediaType: prepared.MediaType, ByteSize: int64(len(prepared.Original)), Width: prepared.Width, Height: prepared.Height, Revision: 1, DelFlag: "0", CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}
	files := []model.PODAssetFile{
		{Variant: "original", StorageKey: original, MediaType: prepared.MediaType, ByteSize: int64(len(prepared.Original)), SHA256: digest, Width: prepared.Width, Height: prepared.Height, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now},
		{Variant: "thumbnail", StorageKey: thumb, MediaType: "image/png", ByteSize: int64(len(prepared.Thumbnail)), SHA256: fmt.Sprintf("%x", sha256.Sum256(prepared.Thumbnail)), Width: prepared.ThumbWidth, Height: prepared.ThumbHeight, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now},
	}
	// A commit error can have an unknown outcome. Keep files rather than deleting a possibly committed asset.
	saved, err := repository.InsertPODAsset(ctx, userID, projectID, asset, files, reservation)
	if (err == nil && saved != asset) || errors.Is(err, repository.ErrPODProject) || errors.Is(err, repository.ErrPODWorkspace) {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), assetCleanupTimeout)
		defer cancel()
		if errors.Join(s.Store.Remove(cleanup, original), s.Store.Remove(cleanup, thumb)) == nil {
			_ = repository.PODReleaseStorage(cleanup, userID, reservation)
		}
	}
	if err != nil {
		return nil, podAssetError(err)
	}
	view := podAssetView(saved)
	return &view, nil
}
func (s *PODAssetService) Open(ctx context.Context, user *model.SysUser, id int64, variant string) (io.ReadCloser, *model.PODAssetFile, string, error) {
	userID, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, nil, "", err
	}
	record, name, err := repository.SelectPODAssetFile(ctx, userID, id, variant)
	if err != nil {
		return nil, nil, "", podAssetError(err)
	}
	file, err := s.Store.Open(ctx, record.StorageKey)
	if err != nil {
		return nil, nil, "", errs.WithCode(404, "素材文件暂不可用，请联系管理员核对存储")
	}
	if file.Size != record.ByteSize {
		file.Close()
		return nil, nil, "", errs.WithCode(404, "素材文件暂不可用，请联系管理员核对存储")
	}
	return file, record, name, nil
}
