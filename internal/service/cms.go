package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var cmsSlug = regexp.MustCompile(`^[a-z][a-z0-9-]{0,79}$`)
var cmsFileKey = regexp.MustCompile(`^[a-f0-9-]{36}\.(png|jpg)$`)
var cmsAssetRoot string
var cmsUploadSlots = make(chan struct{}, 2)

const cmsMaxImageBytes = 8 << 20

func InitCMSAssets(privateRoot, publicRoot string) error {
	if err := os.MkdirAll(privateRoot, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(publicRoot, 0750); err != nil {
		return err
	}
	private, err := filepath.EvalSymlinks(privateRoot)
	if err != nil {
		return err
	}
	public, err := filepath.EvalSymlinks(publicRoot)
	if err != nil {
		return err
	}
	private, err = filepath.Abs(private)
	if err != nil {
		return err
	}
	public, err = filepath.Abs(public)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(public, private)
	if err == nil && (rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")) {
		return fmt.Errorf("CMS私有素材目录不能位于公共profile目录内")
	}
	cmsAssetRoot = private
	return nil
}

func CMSChannelPage(ctx context.Context, q model.CMSChannelQuery, pg page.Query) ([]model.CmsChannel, int64, error) {
	return repository.CMSChannelPage(ctx, q, pg)
}
func CMSChannelDetail(ctx context.Context, id int64) (*model.CmsChannel, error) {
	return repository.CMSChannelByID(ctx, id)
}

func validateCMSGraph(rows []model.CmsChannel, in model.CMSChannelInput) error {
	byID := map[int64]model.CmsChannel{}
	for _, r := range rows {
		byID[r.ChannelId] = r
	}
	id := in.ChannelId
	if id == 0 {
		id = -1
	}
	byID[id] = model.CmsChannel{ChannelId: id, ParentId: in.ParentId, Name: in.Name, Kind: in.Kind, Status: in.Status}
	for _, row := range byID {
		seen := map[int64]bool{}
		current := row
		depth := 1
		for current.ParentId != 0 {
			if current.ParentId == row.ChannelId || seen[current.ParentId] {
				return errs.New("上级栏目不能是自己或后代栏目")
			}
			seen[current.ParentId] = true
			parent, ok := byID[current.ParentId]
			if !ok {
				return errs.New("上级栏目不存在")
			}
			if parent.Kind != current.Kind {
				return errs.New("父子栏目必须属于相同内容类型")
			}
			if current.Status == "0" && parent.Status != "0" {
				return errs.New("请先停用子栏目；启用栏目时上级必须已启用")
			}
			depth++
			if depth > 8 {
				return errs.New("栏目层级最多支持8层")
			}
			current = parent
		}
	}
	return nil
}
func CMSSaveChannel(ctx context.Context, in model.CMSChannelInput, operator string) (int64, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 60 {
		return 0, errs.New("栏目名称需1到60个字符")
	}
	if !cmsSlug.MatchString(in.Slug) {
		return 0, errs.New("栏目地址需小写字母开头，只含小写字母、数字和连字符，最多80位")
	}
	reserved := map[string]bool{"admin": true, "assets": true, "cms": true, "health": true, "search": true, "c": true, "media": true, "login": true}
	if reserved[in.Slug] {
		return 0, errs.New("栏目地址使用了保留名称")
	}
	if in.ChannelId > 0 && in.Version < 1 {
		return 0, errs.New("修改栏目必须提交当前版本")
	}
	return repository.CMSSaveChannel(ctx, in, operator, validateCMSGraph)
}
func CMSDeleteChannel(ctx context.Context, id, version int64) error {
	return repository.CMSDeleteChannel(ctx, id, version)
}

func CMSAssetPage(ctx context.Context, q model.CMSAssetQuery, pg page.Query) ([]model.CmsAsset, int64, error) {
	return repository.CMSAssetPage(ctx, q, pg)
}
func CMSAssetDetail(ctx context.Context, id int64) (*model.CmsAsset, error) {
	return repository.CMSAssetByID(ctx, id)
}
func CMSUpdateAsset(ctx context.Context, id int64, in model.CMSAssetInput, operator string) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errs.New("素材名称不能为空")
	}
	return repository.CMSUpdateAsset(ctx, id, in, operator)
}
func CMSAssetReferences(ctx context.Context, id int64, pg page.Query) ([]model.CMSAssetReference, int64, error) {
	if _, err := repository.CMSAssetByID(ctx, id); err != nil {
		return nil, 0, err
	}
	return repository.CMSAssetReferences(ctx, id, pg)
}

type cmsLimitedImage struct{ buf bytes.Buffer }

func (w *cmsLimitedImage) Write(b []byte) (int, error) {
	if len(b) > cmsMaxImageBytes-w.buf.Len() {
		return 0, errs.New("处理后的图片超过8MB")
	}
	return w.buf.Write(b)
}
func CMSUploadAsset(ctx context.Context, input io.Reader, name, operator string) (*model.CmsAsset, error) {
	select {
	case cmsUploadSlots <- struct{}{}:
		defer func() { <-cmsUploadSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if cmsAssetRoot == "" {
		return nil, fmt.Errorf("CMS私有存储未初始化")
	}
	data, err := io.ReadAll(io.LimitReader(input, cmsMaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > cmsMaxImageBytes {
		return nil, errs.New("图片不得超过8MB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
		return nil, errs.New("只支持有效PNG、JPEG、WebP图片，不接受SVG或其他文件")
	}
	if config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 12000000 {
		return nil, errs.New("图片像素最多1200万")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errs.New("图片内容损坏")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var encoded cmsLimitedImage
	extension, mimeType := "png", "image/png"
	if format == "jpeg" {
		extension, mimeType = "jpg", "image/jpeg"
		err = jpeg.Encode(&encoded, decoded, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&encoded, decoded)
	}
	if err != nil {
		return nil, errs.New("图片处理失败或处理后超过8MB")
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "图片"
	}
	if strings.ContainsAny(name, "<>") {
		return nil, errs.New("文件名不能包含尖括号")
	}
	runes := []rune(name)
	if len(runes) > 150 {
		name = string(runes[:150])
	}
	key := uuid.NewString() + "." + extension
	path := filepath.Join(cmsAssetRoot, key)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.Write(encoded.buf.Bytes())
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return nil, writeErr
		}
		return nil, closeErr
	}
	row := &model.CmsAsset{StorageKey: key, Name: name, MimeType: mimeType, ByteSize: int64(encoded.buf.Len()), Width: config.Width, Height: config.Height, Version: 1, CreateBy: operator, CreateTime: types.Now()}
	if err = repository.CMSInsertAsset(ctx, row); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return row, nil
}
func CMSOpenAsset(ctx context.Context, id int64) (*os.File, *model.CmsAsset, error) {
	row, err := repository.CMSAssetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !cmsFileKey.MatchString(row.StorageKey) || cmsAssetRoot == "" {
		return nil, nil, fmt.Errorf("素材存储标识异常")
	}
	file, err := os.Open(filepath.Join(cmsAssetRoot, row.StorageKey))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, errs.New("素材文件缺失，请重新上传")
		}
		return nil, nil, err
	}
	return file, row, nil
}
func CMSDeleteAsset(ctx context.Context, id, version int64, operator string) error {
	key, err := repository.CMSDeleteAsset(ctx, id, version, operator)
	if err != nil {
		return err
	}
	if !cmsFileKey.MatchString(key) || cmsAssetRoot == "" {
		return fmt.Errorf("素材存储标识异常")
	}
	if err = os.Remove(filepath.Join(cmsAssetRoot, key)); err != nil && !os.IsNotExist(err) {
		slog.Warn("私有素材已停用，物理文件待清理", "assetId", id)
	}
	return nil
}
