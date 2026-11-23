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
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/storage"
	"ruoyi-go/pkg/types"
)

const productMainPrompt = "制作电商白底主图。背景纯白，商品完整居中，光线均匀，边缘清晰。保持商品与参考图一致，不加道具、文字、水印或装饰。"
const productScenePrompt = "制作电商场景图。根据商品用途选择自然、简洁的使用场景，商品为画面主体。保持参考图中的商品本身不变，不添加文字或水印。"

var productOutputPrompts = map[string]string{
	model.PODProductMain: productMainPrompt, model.PODProductScene: productScenePrompt,
	model.PODProductAngle:  "从正面偏转约 45 度展示商品，完整保留主体和比例。延续参考图中的颜色、材质与标记；未展示的结构保持简洁，不添加部件、文字或水印。",
	model.PODProductSide:   "以自然、接近侧面的视角展示商品轮廓、厚度和材质。遵循参考图的结构与颜色，只表现可从原图判断的特征，不添加零件、文字或水印。",
	model.PODProductDetail: "制作商品细节特写。选取参考图可确认的纹理、缝线或表面工艺，保持局部结构、比例和颜色，柔和侧光，背景简洁，不添加标注、文字或水印。",
}

type PODProductService struct {
	Limits    model.PODGraphLimits
	Execution model.PODExecutionLimits
	Assets    *PODAssetService
}

func productWorkspace(ctx context.Context, user *model.SysUser) (int64, error) {
	if user == nil || user.UserID <= 0 {
		return 0, errs.ErrUnauthorized
	}
	space, err := repository.PODPersonalWorkspace(ctx, user.UserID)
	if err != nil {
		return 0, err
	}
	if space == nil {
		return 0, podProjectError(repository.ErrPODWorkspace)
	}
	return space.WorkspaceID, nil
}

func (s PODProductService) Packs(ctx context.Context, user *model.SysUser, pg page.Query) ([]model.PODProductPack, int64, error) {
	space, err := productWorkspace(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	return repository.ListPODProductPacks(ctx, space, normalizePODPage(pg))
}

func (s PODProductService) Jobs(ctx context.Context, user *model.SysUser, pg page.Query) ([]model.PODProductRecent, int64, error) {
	space, err := productWorkspace(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	return repository.ListPODProductJobs(ctx, space, normalizePODPage(pg))
}

func (s PODProductService) Get(ctx context.Context, user *model.SysUser, projectID int64) (*model.PODProductView, error) {
	space, err := productWorkspace(ctx, user)
	if err != nil {
		return nil, err
	}
	row, pack, slots, kind, err := repository.SelectPODProductJob(ctx, space, projectID)
	if err != nil {
		return nil, podProjectError(err)
	}
	project, err := podProjectView(&row, false)
	if err != nil {
		return nil, err
	}
	return &model.PODProductView{Project: project, Pack: pack, Slots: slots, WorkKind: kind, QuickRun: kind == model.PODProductImages && row.Revision == 1}, nil
}

func checkProductOutput(nodeID string) error {
	if model.PODProductOutputMask(nodeID) == 0 {
		return errs.WithCode(400, "未知商品图用途")
	}
	return nil
}

func (s PODProductService) History(ctx context.Context, user *model.SysUser, projectID int64, nodeID string, pg page.Query) ([]model.PODProductRun, int64, error) {
	if err := checkProductOutput(nodeID); err != nil {
		return nil, 0, err
	}
	space, err := productWorkspace(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := repository.SelectPODProductHistory(ctx, space, projectID, nodeID, normalizePODPage(pg))
	return rows, total, podProjectError(err)
}

func (s PODProductService) SetFinal(ctx context.Context, user *model.SysUser, projectID int64, nodeID string, body model.PODProductFinalChange) (*model.PODProductView, error) {
	if err := checkProductOutput(nodeID); err != nil {
		return nil, err
	}
	if user == nil || user.UserID <= 0 {
		return nil, errs.ErrUnauthorized
	}
	run, runErr := PODPositiveID(body.RunID)
	asset, assetErr := PODPositiveID(body.AssetID)
	if runErr != nil || assetErr != nil {
		return nil, errs.WithCode(400, "请选择一张已生成的图片")
	}
	expected := model.PODProductFinal{}
	if body.PreviousRunID != "" || body.PreviousAssetID != "" {
		expected.RunID, runErr = PODPositiveID(body.PreviousRunID)
		expected.AssetID, assetErr = PODPositiveID(body.PreviousAssetID)
		if runErr != nil || assetErr != nil {
			return nil, errs.WithCode(400, "原定稿编号不合法，请刷新后重试")
		}
	}
	err := repository.SetPODProductFinal(ctx, user.UserID, model.PODProductFinal{ProjectID: projectID, NodeID: nodeID, RunID: run, AssetID: asset}, expected)
	switch {
	case errors.Is(err, repository.ErrPODProductResult):
		return nil, errs.WithCode(404, "该用途的图片不存在或还未归档")
	case errors.Is(err, repository.ErrPODProductFinalChanged):
		return nil, errs.WithCode(409, "定稿已在其他页面变更，请重新选择")
	case err != nil:
		return nil, podAssetError(err)
	}
	return s.Get(ctx, user, projectID)
}

func productFields(fields *model.PODProductFields) error {
	for _, field := range []struct {
		label     string
		value     *string
		limit     int
		required  bool
		multiline bool
	}{
		{"商品名", &fields.ProductName, 80, true, false}, {"SKU", &fields.SKU, 60, false, false},
		{"真实卖点", &fields.SellingPoints, 400, false, true}, {"禁改项", &fields.ProtectedDetails, 400, false, true},
	} {
		value := strings.TrimSpace(strings.ReplaceAll(*field.value, "\r\n", "\n"))
		invalidControl := func(r rune) bool { return unicode.IsControl(r) && !(field.multiline && (r == '\n' || r == '\t')) }
		if (field.required && value == "") || utf8.RuneCountInString(value) > field.limit || strings.IndexFunc(value, invalidControl) >= 0 {
			return errs.WithCode(400, fmt.Sprintf("%s格式不正确，最多 %d 个字符", field.label, field.limit))
		}
		*field.value = value
	}
	return nil
}

func productPrompt(fields model.PODProductFields) string {
	lines := []string{"商品：" + fields.ProductName}
	if fields.SellingPoints != "" {
		lines = append(lines, "真实卖点："+fields.SellingPoints)
	}
	if fields.ProtectedDetails != "" {
		lines = append(lines, "必须保留："+fields.ProtectedDetails)
	}
	return strings.Join(lines, "\n")
}

func productGraph(body model.PODProductCreate, prompt string, mask uint8) model.PODGraph {
	graph := model.PODGraph{
		SchemaVersion: 1,
		Nodes: []model.PODNode{
			{ID: model.PODProductText, Type: "textInput", Version: 1, Position: model.PODPosition{X: 40, Y: 40}, Config: productJSON(map[string]any{"title": "商品资料", "text": prompt})},
			{ID: model.PODProductReference, Type: "imageInput", Version: 1, Position: model.PODPosition{X: 40, Y: 330}, Config: productJSON(map[string]any{"title": "商品参考图", "assetIds": []string{body.ReferenceAssetID}})},
		},
		Edges: []model.PODEdge{},
	}
	for i, output := range model.PODProductOutputs(mask) {
		graph.Nodes = append(graph.Nodes, model.PODNode{ID: output.ID, Type: "imageGenerate", Version: 1, Position: model.PODPosition{X: 460, Y: float64(40 + i*290)}, Config: productJSON(map[string]any{
			"title": output.Label, "prompt": productOutputPrompts[output.ID], "modelId": body.ModelID, "ratio": body.Ratio, "count": 1,
		})})
		graph.Edges = append(graph.Edges,
			model.PODEdge{ID: output.ID + "-text", Source: model.PODProductText, SourcePort: "text", Target: output.ID, TargetPort: "text"},
			model.PODEdge{ID: output.ID + "-ref", Source: model.PODProductReference, SourcePort: "images", Target: output.ID, TargetPort: "images"})
	}
	return graph
}

func productLocalGraph(body model.PODProductCreate, kind string) model.PODGraph {
	graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}}
	appendStep := func(id, nodeType string, config map[string]any) {
		if n := len(graph.Nodes); n != 0 {
			graph.Edges = append(graph.Edges, model.PODEdge{ID: id + "-input", Source: graph.Nodes[n-1].ID, SourcePort: "images", Target: id, TargetPort: "images"})
		}
		graph.Nodes = append(graph.Nodes, model.PODNode{ID: id, Type: nodeType, Version: 1, Position: model.PODPosition{X: float64(40 + len(graph.Nodes)*390), Y: 100}, Config: productJSON(config)})
	}
	appendStep(model.PODProductReference, "imageInput", map[string]any{"title": "印花原图", "assetIds": []string{body.ReferenceAssetID}})
	if kind == model.PODProductMockup {
		appendStep("product-shirt-preview", "productPreview", map[string]any{"title": "T 恤样机", "product": "tshirt", "side": "front", "color": "#233446", "printX": .5, "printY": .4, "printScale": .75})
		appendStep("product-shirt-export", "export", map[string]any{"title": "导出样机", "mode": "mockup", "format": "jpeg", "fileName": "T恤样机"})
	} else {
		appendStep("product-print-process", "imageProcess", map[string]any{"title": "整理印花", "operation": "resize", "method": "basic", "width": 2400, "height": 2400})
		appendStep("product-print-export", "export", map[string]any{"title": "导出印花", "mode": "print", "format": "png", "fileName": "印花", "printWidthMM": 300, "printHeightMM": 300})
	}
	return graph
}

func productWork(body *model.PODProductCreate) (string, uint8, error) {
	switch body.WorkKind {
	case model.PODProductPrint, model.PODProductMockup:
		if body.Outputs != nil || body.ModelID != "" || body.Ratio != "" {
			return "", 0, errs.WithCode(400, "印花和样机不需要生图模型、比例或商品图用途")
		}
		return body.WorkKind, 0, nil
	case "", model.PODProductImages:
		if body.Outputs == nil {
			return model.PODProductImages, model.PODProductDefaultOutputs, nil
		}
	}
	if body.WorkKind != "" && body.WorkKind != model.PODProductImages {
		return "", 0, errs.WithCode(400, "未知开工类型")
	}
	var mask uint8
	for _, id := range *body.Outputs {
		bit := model.PODProductOutputMask(id)
		if bit == 0 || mask&bit != 0 {
			return "", 0, errs.WithCode(400, "商品图用途不支持或重复")
		}
		mask |= bit
	}
	if mask == 0 {
		return "", 0, errs.WithCode(400, "至少选择一种商品图")
	}
	selection := make([]string, 0, len(*body.Outputs))
	for _, output := range model.PODProductOutputs(mask) {
		selection = append(selection, output.ID)
	}
	body.Outputs = &selection
	return model.PODProductImages, mask, nil
}

func productJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func productHash(value any) string { return fmt.Sprintf("%x", sha256.Sum256(productJSON(value))) }

func (s PODProductService) Create(ctx context.Context, user *model.SysUser, body model.PODProductCreate) (*model.PODProductView, error) {
	space, err := productWorkspace(ctx, user)
	if err != nil {
		return nil, err
	}
	if err = productFields(&body.PODProductFields); err != nil {
		return nil, err
	}
	assetID, err := PODPositiveID(body.ReferenceAssetID)
	if err != nil {
		return nil, errs.WithCode(400, "请选择一张商品参考图")
	}
	if !podRequestID.MatchString(body.RequestID) {
		return nil, errs.WithCode(400, "开工请求标识不合法")
	}
	kind, mask, err := productWork(&body)
	if err != nil {
		return nil, err
	}
	hash := productHash(body)
	existing, err := repository.FindPODProductJobRequest(ctx, space, body.RequestID, hash)
	if err != nil {
		return nil, podRunError(err)
	}
	if existing != nil {
		return s.Get(ctx, user, existing.ProjectID)
	}
	title := "商品图"
	var graph model.PODGraph
	if kind == model.PODProductImages {
		if s.Execution.Mode != "managed" {
			return nil, errs.WithCode(409, "商品图需要平台启用正式模型与点数报价")
		}
		models, err := PODCreatorModels(ctx, user, "image")
		if err != nil {
			return nil, err
		}
		index := slices.IndexFunc(models, func(candidate model.PODCreatorModel) bool { return fmt.Sprint(candidate.ModelID) == body.ModelID })
		if index < 0 || models[index].Capabilities.MaxReferenceImages < 1 || !slices.Contains(models[index].Capabilities.Ratios, body.Ratio) {
			return nil, errs.WithCode(400, "请选择支持参考图及当前比例的可用模型")
		}
		prompt := productPrompt(body.PODProductFields)
		for _, output := range model.PODProductOutputs(mask) {
			if utf8.RuneCountInString(prompt+"\n"+productOutputPrompts[output.ID]) > models[index].Capabilities.MaxPromptChars {
				return nil, errs.WithCode(400, "商品说明太长，请精简卖点或禁改项后重试")
			}
		}
		graph = productGraph(body, prompt, mask)
	} else {
		title = "印花"
		if kind == model.PODProductMockup {
			title = "T 恤样机"
		}
		graph = productLocalGraph(body, kind)
	}
	if err := workflow.ValidateWithLimits(graph, s.Limits); err != nil {
		return nil, errs.WithCode(400, err.Error())
	}
	now := types.Now()
	pack := &model.PODProductPack{
		WorkspaceID: space, PODProductFields: body.PODProductFields, ReferenceAssetID: assetID, CreateTime: now,
		Fingerprint: productHash(struct {
			model.PODProductFields
			AssetID int64
		}{body.PODProductFields, assetID}),
	}
	job := &model.PODProductJob{RequestID: body.RequestID, RequestHash: hash, WorkKind: kind, OutputMask: mask}
	project := &model.PODProject{Name: body.ProductName + " · " + title, GraphJSON: productJSON(graph), SchemaVersion: 1, Revision: 1,
		DelFlag: "0", CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}
	if err = repository.CreatePODProductJob(ctx, user, job, pack, project); err != nil {
		return nil, podRunError(err)
	}
	return s.Get(ctx, user, job.ProjectID)
}

type PODProductArchive struct {
	File *os.File
	Name string
}

func productFilenamePrefix(sku string, projectID int64) string {
	name := strings.Trim(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, sku), "-_")
	if name == "" {
		return fmt.Sprintf("product-%d", projectID)
	}
	return name
}

func (s PODProductService) ZIP(ctx context.Context, user *model.SysUser, projectID int64, finalOnly bool) (*PODProductArchive, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case podZipSlots <- struct{}{}:
		defer func() { <-podZipSlots }()
	default:
		return nil, errs.WithCode(429, "正在打包其他图片，请稍后重试")
	}
	view, err := s.Get(ctx, user, projectID)
	if err != nil {
		return nil, err
	}
	if len(view.Slots) == 0 {
		return nil, errs.WithCode(409, "请从画布结果下载印花或样机")
	}
	ids := make([]string, 0, len(view.Slots))
	suffix := "-results.zip"
	var revision int64
	var testOnly bool
	if finalOnly {
		suffix = "-final.zip"
	}
	for i, slot := range view.Slots {
		if finalOnly {
			final := slot.Final
			if final == nil || (i > 0 && final.TestOnly != testOnly) {
				return nil, errs.WithCode(409, "请先为每种已选用途设定同类定稿")
			}
			testOnly = final.TestOnly
			ids = append(ids, fmt.Sprint(final.AssetID))
		} else {
			result := slot.Result
			if result == nil || len(result.Output.AssetIDs) == 0 || (i > 0 && (result.TestOnly != testOnly || result.ProjectRevision != revision)) {
				return nil, errs.WithCode(409, "需要所有已选用途在同版画布下归档后才能下载整套")
			}
			testOnly, revision = result.TestOnly, result.ProjectRevision
			ids = append(ids, result.Output.AssetIDs[0])
		}
	}
	prefix := productFilenamePrefix(view.Pack.SKU, projectID)
	assets, err := repository.SelectPODImageInputs(ctx, user.UserID, ids)
	if err != nil {
		return nil, podAssetError(err)
	}
	var total int64
	for _, id := range ids {
		asset, ok := assets[id]
		if !ok {
			return nil, errs.WithCode(409, "部分归档图片暂不可用，请核对素材")
		}
		total += asset.ByteSize
	}
	if total > 100<<20 {
		return nil, errs.WithCode(409, "图片超过 100 MiB，请分别下载")
	}
	file, err := os.CreateTemp("", "pod-product-*.zip")
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
	for i, slot := range view.Slots {
		name := model.PODProductOutputs(model.PODProductOutputMask(slot.NodeID))[0].Filename
		asset := assets[ids[i]]
		input, err := s.Assets.Store.Open(ctx, asset.StorageKey)
		if err != nil {
			return nil, errs.WithCode(409, "归档文件缺失，请核对存储")
		}
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[asset.MediaType]
		err = addProductImage(ctx, archive, prefix+"-"+name, ext, input, asset.ByteSize, asset.SHA256)
		input.Close()
		if err != nil {
			return nil, err
		}
	}
	if err = archive.Close(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	complete = true
	return &PODProductArchive{File: file, Name: prefix + suffix}, nil
}

func addProductImage(ctx context.Context, archive *zip.Writer, name, extension string, input io.Reader, size int64, checksum string) error {
	if extension == "" {
		return errs.WithCode(409, "图片格式不支持打包")
	}
	output, err := archive.CreateHeader(&zip.FileHeader{Name: name + "." + extension, Method: zip.Store})
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := storage.Copy(ctx, io.MultiWriter(output, hash), io.LimitReader(input, size+1))
	if err != nil || n != size || fmt.Sprintf("%x", hash.Sum(nil)) != checksum {
		return errs.WithCode(409, "归档文件校验失败，请核对存储")
	}
	return nil
}
