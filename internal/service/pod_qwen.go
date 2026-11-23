package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/assetimage"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/modelcatalog"
	"ruoyi-go/pkg/types"
)

type PODQwenExecutor struct {
	Provider      *executor.Qwen
	Client        *http.Client
	Images        *http.Client
	CatalogImages *http.Client
	Assets        *PODAssetService
	BudgetFen     int64
	ImageAIURL    string
}

func NewPODQwenExecutor(c *config.Config) (*PODQwenExecutor, error) {
	q := c.POD.Qwen
	if (c.POD.Execution.Mode != "qwen" && c.POD.Execution.Mode != "managed") || !q.AllowPaidCalls || q.BudgetFen < 1 || q.BudgetFen > 5000 {
		return nil, errors.New("千问付费模式或预算未显式配置")
	}
	if c.POD.Execution.Mode == "managed" {
		return &PODQwenExecutor{Images: executor.NewQwenImageClient(), CatalogImages: executor.NewCatalogHTTPClient(45 * time.Second), Assets: NewPODAssetService(c.POD.StorageRoot, model.PODAssetLimits{MaxUploadBytes: c.POD.MaxUploadBytes, MaxPixels: c.POD.MaxImagePixels}), BudgetFen: q.BudgetFen, ImageAIURL: c.POD.Execution.ImageAIURL}, nil
	}
	key := q.APIKey
	if key == "" {
		key = os.Getenv(q.APIKeyEnv)
	}
	p, err := executor.NewQwen(q.BaseURL, key, nil)
	if err != nil {
		return nil, err
	}
	return &PODQwenExecutor{Provider: p, Images: executor.NewQwenImageClient(), Assets: NewPODAssetService(c.POD.StorageRoot, model.PODAssetLimits{MaxUploadBytes: c.POD.MaxUploadBytes, MaxPixels: c.POD.MaxImagePixels}), BudgetFen: q.BudgetFen, ImageAIURL: c.POD.Execution.ImageAIURL}, nil
}

func qwenPending(code, message string) executor.Result {
	return executor.Result{State: "awaitingReconciliation", Code: code, Message: message}
}

func resolvePODProviderCredential(credential *model.PODProviderCredential) (string, error) {
	vault := catalogVault.Load()
	switch credential.SourceType {
	case "encrypted":
		return vault.Open(credential.ProviderID, credential.Version, credential.KeyID, credential.Nonce, credential.Ciphertext)
	case "env":
		return vault.ResolveEnv(credential.EnvName)
	default:
		return "", errors.New("invalid credential source")
	}
}

// Execute uses durable receipts for reconciliation; it never resubmits an uncertain paid call.
func (s *PODQwenExecutor) Execute(ctx context.Context, c *repository.PODClaim) executor.Result {
	if podImageNode(c.Definition.Type) {
		return (&PODImageExecutor{Assets: s.Assets, AIURL: s.ImageAIURL}).Execute(ctx, c)
	}
	if c.Node.Provider == "local" {
		r := executor.Simulation{}.Query(ctx, executor.Request{Node: c.Definition, Input: c.Input})
		r.Output.TestOnly = false
		return r
	}
	if s == nil || s.Assets == nil || s.Images == nil {
		return qwenPending("providerUnavailable", "千问执行器未就绪，未重新调用模型")
	}
	row, err := repository.SelectPODProviderCall(ctx, c)
	if err != nil {
		return qwenPending("receiptUnavailable", "调用记录暂不可用，未调用模型")
	}
	var receipt executor.QwenReceipt
	if row != nil {
		if row.State == "confirmed" {
			if json.Unmarshal(row.ReceiptJSON, &receipt) != nil {
				return qwenPending("receiptInvalid", "原调用回执需要核对；不会重新调用模型")
			}
		} else if c.Run.ExecutionMode == "managed" && row.State == "submitted" {
			store, storeErr := s.Assets.receiptStore()
			if storeErr != nil {
				return qwenPending("providerUncertain", "原调用结果待确认；不会自动重发")
			}
			receipt, err = loadPODInlineReceipt(ctx, store, c.Node.SubmissionKey)
			if err != nil {
				return qwenPending("providerUncertain", "原调用结果待确认；不会自动重发")
			}
			if err := repository.SaveManagedReceipt(ctx, c, receipt); err != nil {
				return qwenPending("receiptUncertain", "原图片已保存在本地，等待确认回执；不会重新生图")
			}
		} else {
			return qwenPending("providerUncertain", "原千问调用结果待确认；不会自动重发，请核对供应商请求明细")
		}
	} else {
		if c.Reconcile {
			return executor.Result{State: "failed", Code: "notSubmitted", Message: "未找到已提交的供应商调用记录，未自动调用模型；可检查后恢复失败步骤"}
		}
		// A managed worker can archive legacy receipts without retaining the old API key.
		if c.Run.ExecutionMode != "managed" && s.Provider == nil {
			return executor.Result{State: "failed", Code: "legacyModeDisabled", Message: "原执行模式已停用，未发起新的模型调用"}
		}
		images := []string{}
		if c.Definition.Type == "imageGenerate" {
			if len(c.Input.AssetIDs) > 3 {
				return executor.Result{State: "failed", Code: "tooManyImages", Message: "千问参考图最多 3 张，未调用模型"}
			}
			for _, idText := range c.Input.AssetIDs {
				id, parseErr := PODPositiveID(idText)
				if parseErr != nil {
					return executor.Result{State: "failed", Code: "invalidImage", Message: "参考图编号无效，未调用模型"}
				}
				file, meta, _, openErr := s.Assets.Open(ctx, &model.SysUser{UserID: c.Run.UserID}, id, "original")
				if openErr != nil {
					return executor.Result{State: "failed", Code: "inputUnavailable", Message: "私有参考图暂不可用，未调用模型"}
				}
				data, readErr := io.ReadAll(io.LimitReader(file, s.Assets.Limits.MaxUploadBytes+1))
				_ = file.Close()
				if readErr != nil || int64(len(data)) > s.Assets.Limits.MaxUploadBytes || int64(len(data)) != meta.ByteSize {
					return executor.Result{State: "failed", Code: "inputUnavailable", Message: "参考图读取失败或过大，未调用模型"}
				}
				images = append(images, "data:"+meta.MediaType+";base64,"+base64.StdEncoding.EncodeToString(data))
			}
		}
		provider := s.Provider
		request := executor.QwenRequest{Model: c.Node.ProviderModel, Node: c.Definition, Input: c.Input, Images: images}
		var binding model.PODBoundModel
		var fresh bool
		var beginErr error
		if c.Run.ExecutionMode == "managed" {
			var credential *model.PODProviderCredential
			binding, credential, beginErr = repository.ManagedBound(ctx, c)
			if beginErr != nil {
				return executor.Result{State: "failed", Code: "modelUnavailable", Message: "模型配置或凭据暂不可用，尚未调用"}
			}
			key, credentialErr := resolvePODProviderCredential(credential)
			if credentialErr != nil {
				return executor.Result{State: "failed", Code: "credentialUnavailable", Message: "供应商凭据不可用，尚未调用"}
			}
			provider, beginErr = executor.NewCatalogProvider(binding.Version.AdapterKind, binding.Version.Endpoint, key, s.Client)
			if beginErr != nil {
				return executor.Result{State: "failed", Code: "credentialUnavailable", Message: "供应商凭据格式无效，尚未调用"}
			}
			request.Model = binding.Version.OfficialModelID
			request.Capabilities = &binding.Capabilities
			if binding.Kind == "image" && binding.Version.AdapterKind == "openai-compatible-v1" && binding.Capabilities.ImageResponse != "url" {
				store, storeErr := s.Assets.receiptStore()
				if storeErr != nil || store.Check(ctx) != nil {
					return executor.Result{State: "failed", Code: "receiptStorage", Message: "本地图片回执目录不可写，尚未调用模型；请联系管理员"}
				}
			}
			fresh, beginErr = repository.BeginManagedCall(ctx, c, credential.CredentialVersionID, s.BudgetFen*10000)
		} else {
			_, fresh, beginErr = repository.BeginPODProviderCall(ctx, c, executor.QwenReservation(c.Definition), s.BudgetFen*10000)
		}
		if errors.Is(beginErr, repository.ErrPODBudget) {
			return executor.Result{State: "failed", Code: "budgetExceeded", Message: "本地估算预算不足，未调用千问；请核对用量和预占"}
		}
		if c.Run.ExecutionMode == "managed" && errs.As(beginErr) != nil {
			return executor.Result{State: "failed", Code: "modelChanged", Message: "模型或用户授权已变化，尚未调用；请检查后重新报价"}
		}
		if beginErr != nil || !fresh {
			return qwenPending("intentUncertain", "调用意图未确认，未提交新的模型请求")
		}
		callCtx, cancelCall := context.WithCancel(ctx)
		if c.Run.ExecutionMode == "managed" {
			cancelCall()
			callCtx, cancelCall = context.WithTimeout(ctx, time.Duration(binding.Version.TimeoutSeconds)*time.Second)
		}
		receipt = provider.Submit(callCtx, request)
		cancelCall()
		if c.Run.ExecutionMode == "managed" {
			cost, _, _, e := modelcatalog.UsagePrice(binding, receipt.Usage)
			if e != nil {
				return qwenPending("usageUncertain", "用量无法安全计价，保留预占并等待核对")
			}
			receipt.Usage.EstimatedCostMicros = cost
			if hasPODImageRef(receipt.ImageURLs, "data:") {
				store, storeErr := s.Assets.receiptStore()
				if storeErr != nil {
					return qwenPending("receiptStorage", "图片回执未能落盘，需核对原调用；不会重新生图")
				}
				stageCtx, stopStage := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				receipt, err = stagePODInlineReceipt(stageCtx, store, c.Node.SubmissionKey, receipt, s.Assets.Limits)
				stopStage()
				if err != nil {
					return qwenPending("receiptStorage", "图片回执未能落盘，需核对原调用；不会重新生图")
				}
			}
		}
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if c.Run.ExecutionMode == "managed" {
			err = repository.SaveManagedReceipt(saveCtx, c, receipt)
		} else {
			var encoded []byte
			encoded, err = json.Marshal(receipt)
			if err == nil {
				err = repository.SavePODProviderReceipt(saveCtx, c, encoded, receipt.Usage.EstimatedCostMicros, receipt.State != "awaitingReconciliation")
			}
		}
		cancel()
		if err != nil {
			return qwenPending("receiptUncertain", "千问响应保存未确认；保留预占，不会重新提交模型")
		}
		if receipt.State == "awaitingReconciliation" {
			return qwenPending(receipt.Code, receipt.Message)
		}
	}
	if receipt.State != "succeeded" && receipt.State != "failed" {
		return qwenPending("receiptInvalid", "原调用记录需人工核对，未重新调用模型")
	}
	output := model.PODRunOutput{TestOnly: false, Text: receipt.Text}
	if receipt.Usage.RequestID != "" {
		output.Usage = &receipt.Usage
	}
	if len(receipt.ImageURLs) > 0 {
		assets, err := repository.SelectPODProviderAssets(ctx, c.Node.SubmissionKey, c.Run.WorkspaceID)
		if err != nil {
			return qwenPending("archivePending", "图片已生成，归档暂不可用；稍后只重试归档")
		}
		for index, raw := range receipt.ImageURLs {
			id := assets[index]
			if id == "" {
				id, err = s.archiveResult(ctx, c, index, raw, receipt.ImageSource == "catalog-v1" && c.Run.ExecutionMode == "managed")
				if err != nil {
					return executor.Result{State: "awaitingReconciliation", Output: output, Code: "archivePending", Message: "图片已生成但尚未全部归档；只重试原结果下载，不重新生图"}
				}
			}
			output.AssetIDs = append(output.AssetIDs, id)
		}
		if hasPODImageRef(receipt.ImageURLs, localReceiptRef) {
			if store, err := s.Assets.receiptStore(); err == nil {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), assetCleanupTimeout)
				_ = store.Remove(cleanup, receiptFileName(c.Node.SubmissionKey))
				cancel()
			}
		}
	}
	return executor.Result{State: receipt.State, Output: output, Code: receipt.Code, Message: receipt.Message}
}

func (s *PODQwenExecutor) archive(ctx context.Context, c *repository.PODClaim, index int, raw string) (string, error) {
	return s.archiveResult(ctx, c, index, raw, false)
}

func (s *PODQwenExecutor) downloadCatalogResult(ctx context.Context, c *repository.PODClaim, raw string) ([]byte, error) {
	limit := s.Assets.Limits.MaxUploadBytes
	if executor.IsBBRootPrivateImageURL(raw) {
		credential, err := repository.SelectManagedBBRootImageCredential(ctx, c)
		if err != nil {
			return nil, err
		}
		if credential != nil {
			key, err := resolvePODProviderCredential(credential)
			if err != nil {
				return nil, err
			}
			return executor.DownloadBBRootPrivateImage(ctx, s.CatalogImages, raw, limit, key)
		}
	}
	return executor.DownloadCatalogImage(ctx, s.CatalogImages, raw, limit)
}

func (s *PODQwenExecutor) archiveResult(ctx context.Context, c *repository.PODClaim, index int, raw string, catalog bool) (string, error) {
	select {
	case podUploadSlots <- struct{}{}:
		defer func() { <-podUploadSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	var data []byte
	var err error
	if catalog {
		if raw == localReceiptRef+strconv.Itoa(index) {
			store, storeErr := s.Assets.receiptStore()
			if storeErr != nil {
				return "", storeErr
			}
			data, err = readPODReceiptEntry(ctx, store, c.Node.SubmissionKey, receiptImageEntry(index), s.Assets.Limits.MaxUploadBytes)
		} else {
			data, err = s.downloadCatalogResult(ctx, c, raw)
		}
	} else {
		data, err = executor.DownloadQwenImage(ctx, s.Images, raw, s.Assets.Limits.MaxUploadBytes)
	}
	if err != nil {
		return "", err
	}
	media := http.DetectContentType(data)
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[media]
	if ext == "" {
		return "", assetimage.ErrImage
	}
	prefix := "qwen"
	if catalog {
		prefix = "model"
	}
	name := fmt.Sprintf("%s-%d-%s-%d.%s", prefix, c.Run.RunID, c.Node.NodeID, index+1, ext)
	prepared, err := assetimage.Prepare(ctx, bytes.NewReader(data), name, media, assetimage.Limits{MaxBytes: s.Assets.Limits.MaxUploadBytes, MaxPixels: s.Assets.Limits.MaxPixels, MaxDimension: s.Assets.Limits.MaxDimension})
	if err != nil {
		return "", err
	}
	reservation := fmt.Sprintf("generated:%s:%d:%s", c.Node.SubmissionKey, index, c.Node.LeaseToken)
	original, thumb, err := s.Assets.storePrepared(ctx, c.Run.UserID, reservation, prepared, true)
	if err != nil {
		return "", err
	}
	now := types.Now()
	asset := &model.PODAsset{Source: "generated", OriginalName: prepared.Name, MediaType: prepared.MediaType, ByteSize: int64(len(prepared.Original)), Width: prepared.Width, Height: prepared.Height, Revision: 1, DelFlag: "0", CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}
	files := []model.PODAssetFile{
		{Variant: "original", StorageKey: original, MediaType: prepared.MediaType, ByteSize: int64(len(prepared.Original)), SHA256: fmt.Sprintf("%x", sha256.Sum256(prepared.Original)), Width: prepared.Width, Height: prepared.Height, CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now},
		{Variant: "thumbnail", StorageKey: thumb, MediaType: "image/png", ByteSize: int64(len(prepared.Thumbnail)), SHA256: fmt.Sprintf("%x", sha256.Sum256(prepared.Thumbnail)), Width: prepared.ThumbWidth, Height: prepared.ThumbHeight, CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now},
	}
	id, err := repository.InsertPODGeneratedAsset(ctx, c, index, asset, files, reservation)
	// Retain on ambiguous commits. If another lease already archived the index, these files are unreferenced.
	if err != nil {
		return "", err
	}
	if id != asset.AssetID {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), assetCleanupTimeout)
		defer cancel()
		if s.Assets.Store.Remove(cleanup, original) == nil && s.Assets.Store.Remove(cleanup, thumb) == nil {
			_ = repository.PODReleaseStorage(cleanup, c.Run.UserID, reservation)
		}
	}
	return strconv.FormatInt(id, 10), nil
}
