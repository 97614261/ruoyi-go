package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/assetimage"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/storage"
	"ruoyi-go/pkg/types"
)

type PODOfficialService struct {
	Store    storage.Local
	Personal *PODAssetService
	Limits   model.PODGraphLimits
}

func officialUser(ctx context.Context, u *model.LoginUser) (int64, error) {
	if u == nil || u.User == nil || u.UserID != u.User.UserID || !u.HasPermission(model.PODStudioAccess) {
		return 0, errs.ErrUnauthorized
	}
	return podProjectUser(ctx, u.User)
}

func NewPODOfficialService(root string, limits model.PODGraphLimits, assets model.PODAssetLimits) *PODOfficialService {
	return &PODOfficialService{Store: storage.Local{Path: filepath.Join(root, "catalog")}, Personal: NewPODAssetService(root, assets), Limits: limits}
}
func (s *PODOfficialService) List(ctx context.Context, u *model.LoginUser, admin bool, name string, p page.Query) ([]model.PODOfficialTemplate, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if admin {
		if err := requirePODAdmin(u, "podAdmin:template:list"); err != nil {
			return nil, 0, err
		}
	} else {
		if _, err := officialUser(ctx, u); err != nil {
			return nil, 0, err
		}
	}
	if len([]rune(name)) > 100 {
		return nil, 0, errs.WithCode(400, "搜索词最多 100 字")
	}
	return repository.PODOfficialList(ctx, admin, name, normalizePODPage(p))
}
func officialVersionView(v *model.PODOfficialVersion) error {
	if err := json.Unmarshal(v.GraphJSON, &v.Graph); err != nil {
		return err
	}
	v.Slots, v.Outputs = podTemplateSlots(*v.Graph)
	return nil
}
func (s *PODOfficialService) Get(ctx context.Context, u *model.LoginUser, id, versionID int64, admin bool) (*model.PODOfficialDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if admin {
		if err := requirePODAdmin(u, "podAdmin:template:query"); err != nil {
			return nil, err
		}
	} else {
		if _, err := officialUser(ctx, u); err != nil {
			return nil, err
		}
	}
	row, err := repository.PODOfficialGet(ctx, id, admin)
	if err != nil {
		return nil, err
	}
	vID := row.PublishedVersionID
	if admin {
		vID = row.CurrentVersionID
		if versionID > 0 {
			vID = &versionID
		}
	} else {
		row.CurrentVersionID = nil
		row.Revision = 0
	}
	out := &model.PODOfficialDetail{Template: row}
	if vID != nil {
		v, err := repository.PODOfficialVersion(ctx, id, *vID)
		if err != nil {
			return nil, err
		}
		if err := officialVersionView(v); err != nil {
			return nil, err
		}
		out.Snapshot = v
	}
	return out, nil
}
func (s *PODOfficialService) Versions(ctx context.Context, u *model.LoginUser, id int64, p page.Query) ([]model.PODOfficialVersion, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:template:query"); err != nil {
		return nil, 0, err
	}
	return repository.PODOfficialVersions(ctx, id, normalizePODPage(p))
}
func (s *PODOfficialService) Write(ctx context.Context, u *model.LoginUser, id int64, op string, r model.PODOfficialWrite) (*model.PODCatalogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, CatalogPermission(op)); err != nil {
		return nil, err
	}
	req, err := uuid.Parse(r.RequestID)
	if err != nil || req == uuid.Nil || req.String() != r.RequestID || !catalogText(r.Reason, 200) {
		return nil, errs.WithCode(400, "请求编号或操作原因无效")
	}
	allowed := " requestId reason "
	switch op {
	case "official.create":
		allowed += "code name description sort graph "
	case "official.update":
		allowed += "revision name description sort "
	case "official.version":
		allowed += "revision graph "
	case "official.publish":
		allowed += "revision versionId "
	case "official.unpublish":
		allowed += "revision "
	default:
		return nil, errs.WithCode(400, "模板操作无效")
	}
	v := reflect.ValueOf(r)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if !v.Field(i).IsZero() && !strings.Contains(allowed, " "+key+" ") {
			return nil, errs.WithCode(400, "本操作包含不支持的字段")
		}
	}
	var revision, version int64
	if id > 0 {
		revision, err = podRevision(r.Revision)
		if err != nil {
			return nil, err
		}
	} else if op != "official.create" {
		return nil, errs.WithCode(400, "模板编号无效")
	}
	if op == "official.create" {
		if !catalogCode.MatchString(r.Code) {
			return nil, errs.WithCode(400, "模板编码须为 3—64 位小写字母数字或 _-，以字母开头")
		}
	}
	if op == "official.create" || op == "official.update" {
		r.Name, err = podTemplateName(r.Name)
		if err != nil {
			return nil, err
		}
		r.Description, err = podTemplateDescription(r.Description)
		if err != nil {
			return nil, err
		}
		if r.Sort < 0 || r.Sort > 100000 {
			return nil, errs.WithCode(400, "排序须为 0—100000")
		}
	}
	if op == "official.create" || op == "official.version" {
		if r.Graph == nil {
			return nil, errs.WithCode(400, "请提供工作流")
		}
		if err := workflow.ValidateWithLimits(*r.Graph, s.Limits); err != nil {
			return nil, errs.WithCode(400, err.Error())
		}
	}
	if op == "official.publish" {
		version, err = PODPositiveID(r.VersionID)
		if err != nil {
			return nil, err
		}
	}
	raw, _ := json.Marshal(struct {
		Op      string
		ID      int64
		Request model.PODOfficialWrite
	}{op, id, r})
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	out, err := repository.PODOfficialWrite(ctx, u.UserID, id, revision, version, op, hash, logx.TraceID(ctx), r)
	return out, podTemplateError(err)
}
func (s *PODOfficialService) Assets(ctx context.Context, u *model.LoginUser, p page.Query) ([]model.PODOfficialAsset, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:template:edit"); err != nil {
		return nil, 0, err
	}
	return repository.PODOfficialAssets(ctx, normalizePODPage(p))
}
func (s *PODOfficialService) Upload(ctx context.Context, u *model.LoginUser, request, license, name, declared string, r io.Reader) (*model.PODCatalogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := requirePODAdmin(u, "podAdmin:template:edit"); err != nil {
		return nil, err
	}
	req, err := uuid.Parse(request)
	if err != nil || req == uuid.Nil || req.String() != request || !catalogText(license, 500) {
		return nil, errs.WithCode(400, "请求编号或使用权说明无效")
	}
	select {
	case podUploadSlots <- struct{}{}:
		defer func() { <-podUploadSlots }()
	default:
		return nil, errs.WithCode(429, "正在处理其他图片，请稍后重试")
	}
	l := s.Personal.Limits
	prepared, err := assetimage.Prepare(ctx, r, name, declared, assetimage.Limits{MaxBytes: l.MaxUploadBytes, MaxPixels: l.MaxPixels, MaxDimension: l.MaxDimension})
	if err != nil {
		return nil, errs.WithCode(400, "图片格式、名称或大小无效")
	}
	raw, _ := json.Marshal([]string{request, license, prepared.Name, fmt.Sprintf("%x", sha256.Sum256(prepared.Original))})
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	if old, err := repository.PODCatalogAction(ctx, u.UserID, request); err == nil {
		if old.ActionType != "official.asset" || old.RequestHash != hash {
			return nil, errs.WithCode(409, "请求编号已用于其他内容")
		}
		var result model.PODCatalogResult
		if json.Unmarshal(old.ResultJSON, &result) != nil {
			return nil, errors.New("official upload result invalid")
		}
		return &result, nil
	} else if b := errs.As(err); b == nil || b.Code != 404 {
		return nil, err
	}
	by := strconv.FormatInt(u.UserID, 10)
	now := types.Now()
	files := []model.PODAssetFile{}
	for _, v := range []struct {
		variant, media string
		data           []byte
		width, height  int
	}{{"original", prepared.MediaType, prepared.Original, prepared.Width, prepared.Height}, {"thumbnail", "image/png", prepared.Thumbnail, prepared.ThumbWidth, prepared.ThumbHeight}} {
		ext := "png"
		if v.media == "image/jpeg" {
			ext = "jpg"
		}
		if v.media == "image/webp" {
			ext = "webp"
		}
		key := fmt.Sprintf("%x", sha256.Sum256([]byte("official-upload:"+by+":"+request+":"+v.variant)))
		key = key[:32] + "." + ext
		if err := s.Store.PutVerified(ctx, key, v.data); err != nil {
			return nil, errs.WithCode(409, "平台图片写入待核对，请用原请求和原文件重试")
		}
		files = append(files, model.PODAssetFile{Variant: v.variant, StorageKey: key, ByteSize: int64(len(v.data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(v.data)), MediaType: v.media, Width: v.width, Height: v.height, CreateBy: by, CreateTime: now})
	}
	a := &model.PODOfficialAsset{Name: prepared.Name, MediaType: prepared.MediaType, ByteSize: int64(len(prepared.Original)), Width: prepared.Width, Height: prepared.Height, LicenseNote: license, Status: "active", Revision: 1, CreateBy: by, CreateTime: now}
	return repository.PODOfficialInsertAsset(ctx, u.UserID, request, hash, logx.TraceID(ctx), a, files)
}
func (s *PODOfficialService) Open(ctx context.Context, u *model.LoginUser, id int64, variant string) (io.ReadCloser, *model.PODAssetFile, error) {
	if err := requirePODAdmin(u, "podAdmin:template:edit"); err != nil {
		return nil, nil, err
	}
	if variant != "original" && variant != "thumbnail" {
		return nil, nil, errs.WithCode(400, "文件类型无效")
	}
	meta, err := repository.PODOfficialFile(ctx, id, variant)
	if err != nil {
		return nil, nil, err
	}
	f, err := s.Store.Open(ctx, meta.StorageKey)
	if err != nil {
		return nil, nil, errs.WithCode(404, "平台素材文件暂不可用")
	}
	if f.Size != meta.ByteSize {
		f.Close()
		return nil, nil, errs.WithCode(404, "平台素材文件暂不可用")
	}
	return f, meta, nil
}
func officialUseView(a *model.PODOfficialUseAction) *model.PODOfficialUseView {
	return &model.PODOfficialUseView{RequestID: a.RequestID, State: a.State, ProjectID: a.ProjectID}
}
func (s *PODOfficialService) UseAction(ctx context.Context, u *model.LoginUser, request string) (*model.PODOfficialUseView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	uid, err := officialUser(ctx, u)
	if err != nil {
		return nil, err
	}
	a, err := repository.PODOfficialUseAction(ctx, uid, request)
	if err != nil {
		return nil, err
	}
	return officialUseView(a), nil
}
func (s *PODOfficialService) Use(ctx context.Context, u *model.LoginUser, id int64, r model.PODTemplateUse) (*model.PODOfficialUseView, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	uid, err := officialUser(ctx, u)
	if err != nil {
		return nil, err
	}
	r.Name, err = PODProjectName(r.Name)
	if err != nil {
		return nil, err
	}
	version, err := PODPositiveID(r.Version)
	if err != nil {
		return nil, err
	}
	action, err := podTemplateAction(r.RequestID, "official.use", id, r, u.User.UserName)
	if err != nil {
		return nil, err
	}
	a, err := repository.PODOfficialUseAction(ctx, uid, r.RequestID)
	if err == nil {
		if a.RequestHash != action.RequestHash {
			return nil, podTemplateError(repository.ErrPODTemplateRequest)
		}
		if a.State == "completed" {
			return officialUseView(a), nil
		}
	} else {
		if b := errs.As(err); b == nil || b.Code != 404 {
			return nil, err
		}
		detail, err := s.Get(ctx, u, id, 0, false)
		if err != nil {
			return nil, err
		}
		if detail.Snapshot == nil || detail.Snapshot.VersionID != version {
			return nil, errs.WithCode(409, "已发布模板版本变化，请重新查看后复制")
		}
		if err := bindPODTemplate(detail.Snapshot.Graph, r.Bindings); err != nil {
			return nil, err
		}
		if err := workflow.ValidateWithLimits(*detail.Snapshot.Graph, s.Limits); err != nil {
			return nil, errs.WithCode(400, err.Error())
		}
		a, err = repository.PODOfficialBeginUse(ctx, uid, id, version, action.RequestHash, r, *detail.Snapshot.Graph)
		if err != nil {
			return nil, podTemplateError(err)
		}
	}
	var plan model.PODOfficialCopyPlan
	if err := json.Unmarshal(a.CopyMapJSON, &plan); err != nil {
		return nil, err
	}
	for _, cp := range plan.Copies {
		for i, f := range cp.SourceFiles {
			src, err := s.Store.Open(ctx, f.StorageKey)
			if err != nil {
				return nil, errs.WithCode(409, "平台素材暂不可用，复制计划和空间预留已保留，可重试原操作")
			}
			raw, readErr := io.ReadAll(io.LimitReader(src, f.ByteSize+1))
			src.Close()
			if readErr != nil || int64(len(raw)) != f.ByteSize || fmt.Sprintf("%x", sha256.Sum256(raw)) != f.SHA256 {
				return nil, errs.WithCode(409, "平台素材摘要不一致，复制计划保留待核对")
			}
			dest, ok := s.Personal.Store.(storage.Local)
			if !ok {
				return nil, errors.New("official copy needs local store")
			}
			if err := dest.PutVerified(ctx, cp.Files[i].StorageKey, raw); err != nil {
				return nil, errs.WithCode(409, "图片复制待核对，请用原请求继续")
			}
		}
	}
	a, err = repository.PODOfficialFinishUse(ctx, uid, r.RequestID, action.RequestHash)
	if err != nil {
		return nil, podTemplateError(err)
	}
	return officialUseView(a), nil
}
