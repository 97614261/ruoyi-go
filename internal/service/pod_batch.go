package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/storage"
	"ruoyi-go/pkg/types"
)

type PODBatchService struct {
	Runs   PODRunService
	Assets *PODAssetService
}

func podBatchError(err error) error {
	if errors.Is(err, repository.ErrPODBatch) {
		return errs.WithCode(404, "批次不存在或没有访问权限")
	}
	if errors.Is(err, repository.ErrPODTemplate) {
		return podTemplateError(err)
	}
	return podRunError(err)
}
func batchView(row *model.PODBatch) (*model.PODBatchView, error) {
	out := &model.PODBatchView{BatchID: row.BatchID, TemplateID: row.TemplateID, TemplateVersion: row.TemplateVersion, Name: row.Name, ExecutionMode: row.ExecutionMode, State: row.State, CreateTime: row.CreateTime}
	if err := json.Unmarshal(row.ImpactJSON, &out.Impact); err != nil {
		return nil, err
	}
	return out, nil
}
func (s PODBatchService) normalize(body model.PODBatchCreate, replayOnly ...bool) (model.PODBatchCreate, string, error) {
	if err := s.Runs.enabled(); err != nil {
		return body, "", err
	}
	if body.ExecutionMode != s.Runs.Limits.Mode && (len(replayOnly) == 0 || !replayOnly[0]) {
		return body, "", errs.WithCode(409, "执行模式已变更，请刷新；不会转换原批次的执行模式")
	}
	if body.ExecutionMode != "simulation" && body.ExecutionMode != "qwen" && body.ExecutionMode != "managed" {
		return body, "", errs.WithCode(400, "不支持的执行模式")
	}
	if body.ExecutionMode != "managed" && body.QuoteID != "" {
		return body, "", errs.WithCode(400, "旧批次模式不接受正式报价")
	}
	if !podRequestID.MatchString(body.RequestID) {
		return body, "", errs.WithCode(400, "请提供 16 到 64 位 requestId")
	}
	if _, err := PODPositiveID(body.TemplateID); err != nil {
		return body, "", err
	}
	if _, err := podRevision(body.Version); err != nil {
		return body, "", err
	}
	var err error
	body.Name, err = PODProjectName(body.Name)
	if err != nil {
		return body, "", err
	}
	if len(body.Items) < 1 || len(body.Items) > model.PODBatchMaxItems {
		return body, "", errs.WithCode(400, "每个批次须有 1 到 10 份明确输入")
	}
	// Copy before normalizing so a caller's draft is never modified in place.
	body.Items = append([]model.PODBatchInput(nil), body.Items...)
	for i := range body.Items {
		body.Items[i].Name, err = PODProjectName(body.Items[i].Name)
		if err != nil {
			return body, "", errs.WithCode(400, fmt.Sprintf("第 %d 份名称须为 1 到 100 个字符", i+1))
		}
	}
	encoded, _ := json.Marshal(body)
	return body, fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}
func (s PODBatchService) prepare(ctx context.Context, user *model.SysUser, body model.PODBatchCreate) ([]model.PODBatchItem, []model.PODProjectAssetRef, model.PODBatchImpact, error) {
	impact := model.PODBatchImpact{Items: len(body.Items)}
	id, _ := PODPositiveID(body.TemplateID)
	version, _ := podRevision(body.Version)
	snapshot, err := (PODTemplateService{Limits: s.Runs.GraphLimits}).Version(ctx, user, id, version)
	if err != nil {
		return nil, nil, impact, err
	}
	now := types.Now()
	items := []model.PODBatchItem{}
	refs := []model.PODProjectAssetRef{}
	for i, input := range body.Items {
		graph := *snapshot.Graph
		graph.Nodes = append([]model.PODNode(nil), snapshot.Graph.Nodes...)
		err := bindPODTemplate(&graph, input.Bindings)
		if err != nil {
			return nil, nil, impact, err
		}
		plan, err := workflow.ExecutionPlanMode(graph, s.Runs.GraphLimits, "", nil, s.Runs.Limits.Mode)
		if err != nil {
			return nil, nil, impact, errs.WithCode(400, fmt.Sprintf("第 %d 份流程不可执行：%s", i+1, err))
		}
		for _, node := range plan {
			var c struct {
				Count     int    `json:"count"`
				Method    string `json:"method"`
				Operation string `json:"operation"`
				Format    string `json:"format"`
			}
			_ = json.Unmarshal(node.Config, &c)
			if node.Type == "textGenerate" {
				impact.TextCalls++
				impact.TestCredits++
			}
			if node.Type == "imageGenerate" {
				impact.ImageCalls++
				if c.Count == 0 {
					c.Count = 1
				}
				impact.Images += c.Count
			}
			if node.Type == "export" && c.Format == "webp" {
				return nil, nil, impact, errs.WithCode(400, "请将模板中的 WebP 导出改为 PNG/JPEG")
			}
			if node.Type == "imageProcess" && c.Method == "ai" && !s.Runs.Limits.ImageAIEnabled {
				return nil, nil, impact, errs.WithCode(409, "本地 AI 图片处理服务尚未启用")
			}
		}
		raw, _ := json.Marshal(graph)
		items = append(items, model.PODBatchItem{ItemNo: i + 1, Name: input.Name, GraphJSON: raw, State: "waiting", CheckedAt: now, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now})
		refs = append(refs, podTemplateRefs(graph)...)
	}
	if body.ExecutionMode == "qwen" {
		impact.ReservationMicros = int64(impact.TextCalls)*25000 + int64(impact.Images)*100000
		impact.TestCredits = 0
	}
	if body.ExecutionMode == "managed" {
		impact.TestCredits = 0
	}
	// One ownership read covers all explicit input substitutions, including preview.
	unique := map[string]bool{}
	ids := []string{}
	for _, ref := range refs {
		id := strconv.FormatInt(ref.AssetID, 10)
		if !unique[id] {
			unique[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		assets, err := repository.SelectPODImageInputs(ctx, user.UserID, ids)
		if err != nil {
			return nil, nil, impact, err
		}
		if len(assets) != len(ids) {
			return nil, nil, impact, podAssetError(repository.ErrPODAssetGraph)
		}
	}
	return items, refs, impact, nil
}
func (s PODBatchService) Preview(ctx context.Context, user *model.SysUser, body model.PODBatchCreate) (*model.PODBatchImpact, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	body, _, err := s.normalize(body)
	if err != nil {
		return nil, err
	}
	_, _, impact, err := s.prepare(ctx, user, body)
	return &impact, err
}
func (s PODBatchService) Create(ctx context.Context, user *model.SysUser, body model.PODBatchCreate) (*model.PODBatchView, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	body, hash, err := s.normalize(body, true)
	if err != nil {
		return nil, err
	}
	existing, err := repository.FindPODBatchRequest(ctx, user.UserID, body.RequestID, hash)
	if err != nil {
		return nil, podBatchError(err)
	}
	if existing != nil {
		return s.Get(ctx, user, existing.BatchID)
	}
	if body.ExecutionMode != s.Runs.Limits.Mode {
		return nil, errs.WithCode(409, "执行模式已变更，请刷新；不会转换原批次的执行模式")
	}
	items, refs, impact, err := s.prepare(ctx, user, body)
	if err != nil {
		return nil, err
	}
	id, _ := PODPositiveID(body.TemplateID)
	version, _ := podRevision(body.Version)
	raw, _ := json.Marshal(impact)
	now := types.Now()
	batch := model.PODBatch{RequestID: body.RequestID, RequestHash: hash, TemplateID: id, TemplateVersion: version, QuoteID: body.QuoteID, SourceHash: repository.ManagedQuoteSourceHash(model.PODQuoteRequest{Kind: "batch", Batch: &body}), Name: body.Name, ExecutionMode: body.ExecutionMode, State: "active", ImpactJSON: raw, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}
	row, err := repository.CreatePODBatch(ctx, user, &batch, items, refs, s.Runs.Limits)
	if err != nil {
		return nil, podBatchError(err)
	}
	return s.Get(ctx, user, row.BatchID)
}
func (s PODBatchService) Get(ctx context.Context, user *model.SysUser, id int64) (*model.PODBatchView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	row, items, err := repository.SelectPODBatch(ctx, uid, id)
	if err != nil {
		return nil, podBatchError(err)
	}
	view, err := batchView(row)
	if err == nil {
		if row.ExecutionMode == "managed" {
			view.Billing, err = repository.PODManagedBilling(ctx, uid, "batch", id)
		}
		view.Items = items
	}
	return view, err
}
func (s PODBatchService) List(ctx context.Context, user *model.SysUser, pg page.Query) ([]model.PODBatchView, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := repository.SelectPODBatchPage(ctx, uid, normalizePODPage(pg))
	if err != nil {
		return nil, 0, podBatchError(err)
	}
	out := []model.PODBatchView{}
	for i := range rows {
		v, e := batchView(&rows[i])
		if e != nil {
			return nil, 0, e
		}
		out = append(out, *v)
	}
	return out, total, nil
}
func (s PODBatchService) Stop(ctx context.Context, user *model.SysUser, id int64) (*model.PODBatchView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	if err = repository.StopPODBatch(ctx, uid, id, user.UserName); err != nil {
		return nil, podBatchError(err)
	}
	return s.Get(ctx, user, id)
}

var podZipSlots = make(chan struct{}, 2)

func (s PODBatchService) ZIP(ctx context.Context, user *model.SysUser, id int64) (*os.File, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	uid, err := podProjectUser(ctx, user)
	if err != nil {
		return nil, err
	}
	select {
	case podZipSlots <- struct{}{}:
		defer func() { <-podZipSlots }()
	default:
		return nil, errs.WithCode(429, "正在打包其他图片，请稍后重试")
	}
	rows, err := repository.SelectPODBatchOutputs(ctx, uid, id)
	if err != nil {
		return nil, podBatchError(err)
	}
	view, err := s.Get(ctx, user, id)
	if err != nil {
		return nil, err
	}
	itemNo := map[int64]int{}
	for _, item := range view.Items {
		itemNo[item.RunID] = item.ItemNo
	}
	type entry struct {
		ID     string
		RunID  int64
		NodeID string
	}
	entries := []entry{}
	ids := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.NodeType != "imageGenerate" && row.NodeType != "imageProcess" && row.NodeType != "productPreview" && row.NodeType != "export" {
			continue
		}
		var output model.PODRunOutput
		if err = json.Unmarshal(row.OutputJSON, &output); err != nil {
			return nil, err
		}
		for _, assetID := range output.AssetIDs {
			key := fmt.Sprint(row.RunID, ":", assetID)
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, entry{assetID, row.RunID, row.NodeID})
			ids = append(ids, assetID)
		}
	}
	if len(entries) == 0 {
		return nil, errs.WithCode(409, "尚无已归档图片，请等待运行完成后再下载")
	}
	if len(entries) > 100 {
		return nil, errs.WithCode(409, "本批次超过 100 张，请分别下载")
	}
	assets, err := repository.SelectPODImageInputs(ctx, uid, ids)
	if err != nil {
		return nil, podAssetError(err)
	}
	var total int64
	for _, entry := range entries {
		a, ok := assets[entry.ID]
		if !ok {
			return nil, errs.WithCode(409, "部分归档图片暂不可用，请核对素材")
		}
		total += a.ByteSize
		if total > 100<<20 {
			return nil, errs.WithCode(409, "打包图片超过 100 MiB，请分别下载")
		}
	}
	file, err := os.CreateTemp("", "pod-batch-*.zip")
	if err != nil {
		return nil, errors.New("无法创建下载文件")
	}
	complete := false
	defer func() {
		if !complete {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	archive := zip.NewWriter(file)
	for _, entry := range entries {
		a := assets[entry.ID]
		kind := "print"
		if a.Source == "mockup" || a.AssetRemark == "pod-kind:mockup" {
			kind = "mockup"
		}
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[a.MediaType]
		if ext == "" {
			archive.Close()
			return nil, errs.WithCode(409, "图片格式不支持打包")
		}
		input, err := s.Assets.Store.Open(ctx, a.StorageKey)
		if err != nil {
			archive.Close()
			return nil, errs.WithCode(409, "归档文件缺失，请核对存储")
		}
		header := &zip.FileHeader{Name: fmt.Sprintf("item-%02d/%s/asset-%s.%s", itemNo[entry.RunID], kind, entry.ID, ext), Method: zip.Store}
		writer, err := archive.CreateHeader(header)
		if err != nil {
			input.Close()
			archive.Close()
			return nil, err
		}
		hash := sha256.New()
		n, copyErr := storage.Copy(ctx, io.MultiWriter(writer, hash), io.LimitReader(input, a.ByteSize+1))
		input.Close()
		if copyErr != nil || n != a.ByteSize || fmt.Sprintf("%x", hash.Sum(nil)) != a.SHA256 {
			archive.Close()
			return nil, errs.WithCode(409, "归档文件校验失败，请核对存储")
		}
	}
	// The manifest is safe creator data; no provider receipts, private paths or tokens.
	manifest, err := archive.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Store})
	if err != nil {
		archive.Close()
		return nil, err
	}
	if err = json.NewEncoder(manifest).Encode(view); err != nil {
		archive.Close()
		return nil, err
	}
	if err = archive.Close(); err != nil {
		return nil, err
	}
	if err = file.Sync(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	complete = true
	return file, nil
}
