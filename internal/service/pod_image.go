package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"strconv"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/assetimage"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/imagework"
	"ruoyi-go/pkg/types"
)

func podImageNode(kind string) bool {
	return kind == "imageProcess" || kind == "productPreview" || kind == "export"
}

type PODImageExecutor struct {
	Assets *PODAssetService
	AIURL  string
}

func (s *PODImageExecutor) Execute(ctx context.Context, claim *repository.PODClaim) executor.Result {
	fail := func(code, message string) executor.Result {
		return executor.Result{State: "failed", Code: code, Message: message}
	}
	if s == nil || s.Assets == nil {
		return fail("imageUnavailable", "图片执行器未配置")
	}
	select {
	case podUploadSlots <- struct{}{}:
		defer func() { <-podUploadSlots }()
	case <-ctx.Done():
		return fail("imageInterrupted", "图片处理已中断，可恢复失败步骤")
	}
	c, err := imagework.Parse(claim.Definition.Config)
	if err != nil {
		return fail("invalidConfig", "图片配置无法读取")
	}
	if len(claim.Input.AssetIDs) < 1 || len(claim.Input.AssetIDs) > 8 {
		return fail("invalidInput", "图片节点需要 1—8 张上游图片")
	}
	archived, err := repository.SelectPODProviderAssets(ctx, claim.Node.SubmissionKey, claim.Run.WorkspaceID)
	if err != nil {
		return executor.Result{State: "awaitingReconciliation", Code: "archivePending", Message: "正在核对已归档的处理结果"}
	}
	out := model.PODRunOutput{TestOnly: claim.Run.ExecutionMode != "qwen"}
	inputs, err := repository.SelectPODImageInputs(ctx, claim.Run.UserID, claim.Input.AssetIDs)
	if err != nil || len(inputs) != len(claim.Input.AssetIDs) {
		return fail("inputUnavailable", "输入图片不存在或没有访问权限")
	}
	archivedIDs := []string{}
	for _, id := range archived {
		archivedIDs = append(archivedIDs, id)
	}
	existing := map[string]repository.PODImageInput{}
	if len(archivedIDs) > 0 {
		existing, err = repository.SelectPODImageInputs(ctx, claim.Run.UserID, archivedIDs)
		if err != nil {
			return executor.Result{State: "awaitingReconciliation", Code: "archivePending", Message: "处理结果待核对"}
		}
	}
	for index, text := range claim.Input.AssetIDs {
		if err := ctx.Err(); err != nil {
			return fail("imageInterrupted", "图片处理已中断，可恢复失败步骤")
		}
		id, err := PODPositiveID(text)
		if err != nil {
			return fail("invalidInput", "输入素材编号无效")
		}
		kind := "artwork"
		if inputs[text].Source == "mockup" || inputs[text].AssetRemark == "pod-kind:mockup" {
			kind = "mockup"
		}
		if claim.Definition.Type == "export" && ((c.Mode == "print" && kind == "mockup") || (c.Mode == "mockup" && kind != "mockup")) {
			return fail("exportKind", "印花与商品展示图来源不匹配，请连接对应上游节点")
		}
		if claim.Definition.Type == "productPreview" {
			if kind == "mockup" {
				return fail("previewKind", "商品预览需要印花原图，不能再次使用商品展示图")
			}
			kind = "mockup"
		}
		name := fmt.Sprintf("%s-%s-%d.png", claim.Definition.Type, kind, index+1)
		if claim.Definition.Type == "export" {
			name = fmt.Sprintf("%s-%s-%d.%s", c.FileName, c.Mode, index+1, c.Format)
			if c.Format == "jpeg" {
				name = fmt.Sprintf("%s-%s-%d.jpg", c.FileName, c.Mode, index+1)
			}
		}
		assetID := archived[index]
		var w, h int
		if assetID != "" {
			meta, ok := existing[assetID]
			if !ok {
				return fail("archiveUnavailable", "已归档结果暂不可用")
			}
			w, h = meta.Width, meta.Height
		} else {
			meta := inputs[text]
			file, openErr := s.Assets.Store.Open(ctx, meta.StorageKey)
			if openErr != nil {
				return fail("inputUnavailable", "输入素材暂不可用或没有访问权限")
			}
			data, readErr := io.ReadAll(io.LimitReader(file, s.Assets.Limits.MaxUploadBytes+1))
			_ = file.Close()
			if readErr != nil || file.Size != meta.ByteSize || int64(len(data)) != meta.ByteSize || int64(len(data)) > s.Assets.Limits.MaxUploadBytes {
				return fail("inputUnavailable", "输入图片读取失败或超过上限")
			}
			src, decodeErr := imagework.Decode(data)
			if decodeErr != nil {
				return fail("invalidImage", decodeErr.Error())
			}
			var result image.Image
			format := "png"
			switch claim.Definition.Type {
			case "imageProcess":
				if c.Method == "ai" && (c.Operation == "removeBackground" || c.Operation == "upscale") {
					if c.Operation == "upscale" {
						if err = imagework.Dimensions(src.Bounds().Dx()*c.Scale, src.Bounds().Dy()*c.Scale); err != nil {
							return fail("imageSize", err.Error())
						}
					}
					var output []byte
					output, err = imagework.AI(ctx, s.AIURL, data, c.Operation, c.Scale, s.Assets.Limits.MaxUploadBytes)
					if err == nil {
						result, err = imagework.Decode(output)
						if err == nil {
							factor := 1
							if c.Operation == "upscale" {
								factor = c.Scale
							}
							if result.Bounds().Dx() != src.Bounds().Dx()*factor || result.Bounds().Dy() != src.Bounds().Dy()*factor {
								err = errors.New("本地 AI 返回尺寸不符合当前操作，未保存结果")
							}
						}
					}
				} else {
					result, err = imagework.Process(ctx, src, c)
				}
			case "productPreview":
				result, err = imagework.Mockup(ctx, src, c)
			case "export":
				result = src
				format = c.Format
			}
			if err != nil {
				return fail("imageProcessing", err.Error())
			}
			if result == nil {
				return fail("imageProcessing", "图片处理未产生结果")
			}
			w, h = result.Bounds().Dx(), result.Bounds().Dy()
			encoded, media, encodeErr := imagework.Encode(ctx, result, format)
			if encodeErr != nil {
				return fail("imageEncoding", encodeErr.Error())
			}
			assetID, err = s.archive(ctx, claim, index, id, name, media, kind, encoded)
			if err != nil {
				if errors.Is(err, assetimage.ErrSize) || errors.Is(err, assetimage.ErrImage) {
					return fail("imageSize", "处理结果超过素材大小或像素限制，请减小尺寸")
				}
				return executor.Result{State: "awaitingReconciliation", Code: "archivePending", Message: "处理结果归档待确认，将按原步骤恢复，不调用付费模型"}
			}
		}
		info := model.PODImageInfo{AssetID: assetID, Width: w, Height: h, Kind: kind, Format: "png"}
		if claim.Definition.Type == "export" {
			info.Format = c.Format
			info.Kind = c.Mode
			if c.Mode == "print" {
				info.DPI = imagework.EffectiveDPI(w, h, c.PrintWidthMM, c.PrintHeightMM)
				if info.DPI < 150 {
					info.Warning = "按目标尺寸计算低于 150 DPI，建议减小印刷尺寸或使用更清晰的原图"
				}
				if c.Format == "jpeg" {
					info.Warning += " JPEG 不支持透明，已铺白底。"
				}
			}
		}
		out.AssetIDs = append(out.AssetIDs, assetID)
		out.Images = append(out.Images, info)
	}
	return executor.Result{State: "succeeded", Output: out}
}

func (s *PODImageExecutor) archive(ctx context.Context, c *repository.PODClaim, index int, parent int64, name, media, kind string, data []byte) (string, error) {
	p, err := assetimage.Prepare(ctx, bytes.NewReader(data), name, media, assetimage.Limits{MaxBytes: s.Assets.Limits.MaxUploadBytes, MaxPixels: s.Assets.Limits.MaxPixels, MaxDimension: s.Assets.Limits.MaxDimension})
	if err != nil {
		return "", err
	}
	reservation := fmt.Sprintf("generated:%s:%d:%s", c.Node.SubmissionKey, index, c.Node.LeaseToken)
	original, thumb, err := s.Assets.storePrepared(ctx, c.Run.UserID, reservation, p, true)
	if err != nil {
		return "", err
	}
	now := types.Now()
	a := &model.PODAsset{ParentAssetID: parent, Source: "processed", OriginalName: p.Name, MediaType: p.MediaType, ByteSize: int64(len(p.Original)), Width: p.Width, Height: p.Height, Revision: 1, DelFlag: "0", CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}
	a.Remark = "pod-kind:" + kind
	if c.Definition.Type == "productPreview" {
		a.Source = "mockup"
	}
	if c.Definition.Type == "export" {
		a.Source = "export"
	}
	files := []model.PODAssetFile{{Variant: "original", StorageKey: original, MediaType: p.MediaType, ByteSize: int64(len(p.Original)), SHA256: fmt.Sprintf("%x", sha256.Sum256(p.Original)), Width: p.Width, Height: p.Height, CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}, {Variant: "thumbnail", StorageKey: thumb, MediaType: "image/png", ByteSize: int64(len(p.Thumbnail)), SHA256: fmt.Sprintf("%x", sha256.Sum256(p.Thumbnail)), Width: p.ThumbWidth, Height: p.ThumbHeight, CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}}
	id, err := repository.InsertPODGeneratedAsset(ctx, c, index, a, files, reservation)
	if err != nil {
		return "", errors.New("processed archive pending")
	}
	if id != a.AssetID {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), assetCleanupTimeout)
		defer cancel()
		if s.Assets.Store.Remove(cleanup, original) == nil && s.Assets.Store.Remove(cleanup, thumb) == nil {
			_ = repository.PODReleaseStorage(cleanup, c.Run.UserID, reservation)
		}
	}
	return strconv.FormatInt(id, 10), nil
}
