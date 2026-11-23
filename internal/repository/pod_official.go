package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func PODOfficialList(ctx context.Context, admin bool, name string, p page.Query) ([]model.PODOfficialTemplate, int64, error) {
	q := DB(ctx).Model(&model.PODOfficialTemplate{})
	if !admin {
		q = q.Where("status='published' AND published_version_id IS NOT NULL")
	}
	if name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODOfficialTemplate{}
	err := q.Order("sort,template_id").Offset(p.Offset()).Limit(p.PageSize).Find(&rows).Error
	if !admin {
		for i := range rows {
			rows[i].CurrentVersionID = nil
			rows[i].Revision = 0
		}
	}
	return rows, total, err
}
func PODOfficialGet(ctx context.Context, id int64, admin bool) (*model.PODOfficialTemplate, error) {
	var row model.PODOfficialTemplate
	q := DB(ctx).Where("template_id=?", id)
	if !admin {
		q = q.Where("status='published' AND published_version_id IS NOT NULL")
	}
	err := q.Take(&row).Error
	return &row, operationMissing(err)
}
func PODOfficialVersion(ctx context.Context, id, versionID int64) (*model.PODOfficialVersion, error) {
	var v model.PODOfficialVersion
	err := DB(ctx).Where("template_id=? AND version_id=?", id, versionID).Take(&v).Error
	return &v, operationMissing(err)
}
func PODOfficialVersions(ctx context.Context, id int64, p page.Query) ([]model.PODOfficialVersion, int64, error) {
	q := DB(ctx).Model(&model.PODOfficialVersion{}).Where("template_id=?", id)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODOfficialVersion{}
	err := q.Omit("graph_json,spec_hash").Order("version DESC,version_id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&rows).Error
	return rows, total, err
}

func validateOfficialRefs(tx *gorm.DB, g model.PODGraph) error {
	ids := []int64{}
	seen := map[int64]bool{}
	for _, r := range officialRefs(g, 0, "") {
		if !seen[r.AssetID] {
			seen[r.AssetID] = true
			ids = append(ids, r.AssetID)
		}
	}
	if len(ids) > 16 {
		return errs.WithCode(400, "每份模板最多引用 16 张平台图片")
	}
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&model.PODOfficialAsset{}).Where("asset_id IN ? AND status='active' AND license_note<>''", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return errs.WithCode(400, "模板引用了不存在或未授权的平台素材")
	}
	return nil
}

// Publication checks explicit choices in one query. Per-user grants and current
// prices are still checked when the copied project requests an execution quote.
func validateOfficialModels(tx *gorm.DB, graph model.PODGraph) error {
	wants := map[int64]string{}
	for _, node := range graph.Nodes {
		kind := ""
		if node.Type == "textGenerate" {
			kind = "text"
		}
		if node.Type == "imageGenerate" {
			kind = "image"
		}
		if kind == "" {
			continue
		}
		var c struct {
			ModelID string `json:"modelId"`
		}
		if err := json.Unmarshal(node.Config, &c); err != nil {
			return err
		}
		if c.ModelID == "" {
			continue
		}
		id, err := strconv.ParseInt(c.ModelID, 10, 64)
		if err != nil || id < 1 || (wants[id] != "" && wants[id] != kind) {
			return errs.WithCode(400, "模板模型与节点类型不匹配")
		}
		wants[id] = kind
	}
	if len(wants) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(wants))
	for id := range wants {
		ids = append(ids, id)
	}
	var rows []struct {
		ModelID int64
		Kind    string
	}
	if err := tx.Table("pod_model m").Select("m.model_id,m.kind").Joins("JOIN pod_provider p ON p.provider_id=m.provider_id AND p.status='enabled'").Joins("JOIN pod_model_version v ON v.model_version_id=m.current_version_id AND v.model_id=m.model_id").Where("m.model_id IN ? AND m.status='enabled' AND m.visible=1", ids).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(wants) {
		return errs.WithCode(400, "模板指定的模型未上架或配置不完整")
	}
	for _, row := range rows {
		if wants[row.ModelID] != row.Kind {
			return errs.WithCode(400, "模板模型与节点类型不匹配")
		}
	}
	return nil
}
func officialRefs(g model.PODGraph, version int64, actor string) []model.PODOfficialRef {
	refs := []model.PODOfficialRef{}
	for _, n := range g.Nodes {
		if n.Type != "imageInput" {
			continue
		}
		var c struct {
			AssetIDs []string `json:"assetIds"`
		}
		_ = json.Unmarshal(n.Config, &c)
		for _, raw := range c.AssetIDs {
			id, _ := strconv.ParseInt(raw, 10, 64)
			refs = append(refs, model.PODOfficialRef{VersionID: version, NodeID: n.ID, AssetID: id, CreateBy: actor, CreateTime: types.Now()})
		}
	}
	return refs
}
func PODOfficialWrite(ctx context.Context, actor, id, revision, versionID int64, op, hash, trace string, r model.PODOfficialWrite) (*model.PODCatalogResult, error) {
	var result model.PODCatalogResult
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		perm := "podAdmin:template:edit"
		if strings.HasSuffix(op, "publish") {
			perm = "podAdmin:template:publish"
		}
		for _, p := range []string{model.PODAdminAccess, perm} {
			if err := catalogActor(tx, actor, p); err != nil {
				return err
			}
		}
		var old model.PODAdminAction
		err := tx.Where("actor_user_id=? AND request_id=?", actor, r.RequestID).Take(&old).Error
		if err == nil {
			if old.ActionType != op || old.RequestHash != hash {
				return errs.WithCode(409, "请求编号已用于其他操作")
			}
			return json.Unmarshal(old.ResultJSON, &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := types.Now()
		by := strconv.FormatInt(actor, 10)
		row := model.PODOfficialTemplate{}
		if id > 0 {
			if err := tx.Where("template_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; err != nil {
				return operationMissing(err)
			}
			if row.Revision != revision {
				return ErrPODRevision
			}
			row.Revision++
		} else {
			row = model.PODOfficialTemplate{Code: r.Code, Name: r.Name, Description: r.Description, Sort: r.Sort, Status: "draft", Revision: 1, CreateBy: by, CreateTime: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			id = row.TemplateID
		}
		switch op {
		case "official.create", "official.version":
			if r.Graph == nil {
				return errs.WithCode(400, "请选择工作流内容")
			}
			if err := validateOfficialRefs(tx, *r.Graph); err != nil {
				return err
			}
			var maxVersion int64
			if err := tx.Model(&model.PODOfficialVersion{}).Select("COALESCE(MAX(version),0)").Where("template_id=?", id).Scan(&maxVersion).Error; err != nil {
				return err
			}
			canonical, err := canonicalOfficialGraph(*r.Graph)
			if err != nil {
				return err
			}
			raw := podJSON(canonical)
			v := model.PODOfficialVersion{TemplateID: id, Version: maxVersion + 1, GraphJSON: raw, SpecHash: fmt.Sprintf("%x", sha256.Sum256(raw)), CreateBy: by, CreateTime: now}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			refs := officialRefs(*r.Graph, v.VersionID, by)
			if len(refs) > 0 {
				if err := tx.Create(&refs).Error; err != nil {
					return err
				}
			}
			row.CurrentVersionID = &v.VersionID
			result.Version = v.Version
		case "official.update":
			row.Name = r.Name
			row.Description = r.Description
			row.Sort = r.Sort
		case "official.publish":
			var v model.PODOfficialVersion
			if err := tx.Where("template_id=? AND version_id=?", id, versionID).Take(&v).Error; err != nil {
				return operationMissing(err)
			}
			var g model.PODGraph
			if json.Unmarshal(v.GraphJSON, &g) != nil {
				return errors.New("official snapshot invalid")
			}
			if err := validateOfficialRefs(tx, g); err != nil {
				return err
			}
			if err := validateOfficialModels(tx, g); err != nil {
				return err
			}
			if err := tx.Model(&v).Where("published_at IS NULL").Update("published_at", now).Error; err != nil {
				return err
			}
			row.Status = "published"
			row.PublishedVersionID = &v.VersionID
			result.Version = v.Version
		case "official.unpublish":
			row.Status = "unpublished"
		default:
			return errs.WithCode(400, "官方模板操作无效")
		}
		if err := tx.Model(&model.PODOfficialTemplate{}).Where("template_id=?", id).Updates(map[string]any{"name": row.Name, "description": row.Description, "sort": row.Sort, "status": row.Status, "current_version_id": row.CurrentVersionID, "published_version_id": row.PublishedVersionID, "revision": row.Revision, "update_by": by, "update_time": now}).Error; err != nil {
			return err
		}
		result.RequestID = r.RequestID
		result.ActionType = op
		result.ResourceType = "officialTemplate"
		result.ResourceID = id
		result.Revision = row.Revision
		if err := operationAudit(tx, actor, 0, id, op, "officialTemplate", r.Reason, trace, model.PODOperationAudit{State: row.Status, Revision: row.Revision, Version: result.Version}); err != nil {
			return err
		}
		return tx.Create(&model.PODAdminAction{ActorUserID: actor, RequestID: r.RequestID, RequestHash: hash, ActionType: op, ResourceType: result.ResourceType, ResourceID: id, State: "completed", ResultJSON: podJSON(result), CreateBy: by, CreateTime: now}).Error
	})
	return &result, err
}
func PODOfficialAssets(ctx context.Context, p page.Query) ([]model.PODOfficialAsset, int64, error) {
	q := DB(ctx).Model(&model.PODOfficialAsset{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODOfficialAsset{}
	err := q.Order("create_time DESC,asset_id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&rows).Error
	return rows, total, err
}
func PODOfficialFile(ctx context.Context, id int64, variant string) (*model.PODAssetFile, error) {
	var f model.PODAssetFile
	err := DB(ctx).Table("pod_catalog_asset_file").Where("asset_id=? AND variant=?", id, variant).Take(&f).Error
	return &f, operationMissing(err)
}
func PODOfficialInsertAsset(ctx context.Context, actor int64, request, hash, trace string, a *model.PODOfficialAsset, files []model.PODAssetFile) (*model.PODCatalogResult, error) {
	var out model.PODCatalogResult
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		for _, p := range []string{model.PODAdminAccess, "podAdmin:template:edit"} {
			if err := catalogActor(tx, actor, p); err != nil {
				return err
			}
		}
		var old model.PODAdminAction
		err := tx.Where("actor_user_id=? AND request_id=?", actor, request).Take(&old).Error
		if err == nil {
			if old.ActionType != "official.asset" || old.RequestHash != hash {
				return errs.WithCode(409, "请求编号冲突")
			}
			return json.Unmarshal(old.ResultJSON, &out)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(a).Error; err != nil {
			return err
		}
		for i := range files {
			files[i].AssetID = a.AssetID
		}
		if err := tx.Table("pod_catalog_asset_file").Create(&files).Error; err != nil {
			return err
		}
		out = model.PODCatalogResult{RequestID: request, ActionType: "official.asset", ResourceType: "officialAsset", ResourceID: a.AssetID, Revision: 1}
		if err := operationAudit(tx, actor, 0, a.AssetID, "official.asset", "officialAsset", "上传具有使用权说明的平台素材", trace, model.PODOperationAudit{Revision: 1}); err != nil {
			return err
		}
		return tx.Create(&model.PODAdminAction{ActorUserID: actor, RequestID: request, RequestHash: hash, ActionType: "official.asset", ResourceType: out.ResourceType, ResourceID: a.AssetID, State: "completed", ResultJSON: podJSON(out), CreateBy: strconv.FormatInt(actor, 10), CreateTime: types.Now()}).Error
	})
	return &out, err
}
func PODOfficialUseAction(ctx context.Context, user int64, request string) (*model.PODOfficialUseAction, error) {
	var row model.PODOfficialUseAction
	err := DB(ctx).Where("user_id=? AND request_id=?", user, request).Take(&row).Error
	return &row, operationMissing(err)
}

// The workspace lock serializes same-user use/retry; files are copied only after
// this transaction durably records their keys and reserves their total bytes.
func PODOfficialBeginUse(ctx context.Context, user, id, versionID int64, hash string, body model.PODTemplateUse, graph model.PODGraph) (*model.PODOfficialUseAction, error) {
	var out model.PODOfficialUseAction
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, user)
		if err != nil {
			return err
		}
		err = tx.Where("user_id=? AND request_id=?", user, body.RequestID).Take(&out).Error
		if err == nil {
			if out.RequestHash != hash {
				return ErrPODTemplateRequest
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var t model.PODOfficialTemplate
		if err := tx.Where("template_id=? AND status='published' AND published_version_id=?", id, versionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&t).Error; err != nil {
			return operationMissing(err)
		}
		platformGraph := graph
		platformGraph.Nodes = append([]model.PODNode(nil), graph.Nodes...)
		personalNodes := []string{}
		personalRefs := []model.PODProjectAssetRef{}
		for _, b := range body.Bindings {
			if b.AssetIDs == nil {
				continue
			}
			personalNodes = append(personalNodes, b.NodeID)
			for _, raw := range *b.AssetIDs {
				id, _ := strconv.ParseInt(raw, 10, 64)
				personalRefs = append(personalRefs, model.PODProjectAssetRef{NodeID: b.NodeID, AssetID: id})
			}
			for i := range platformGraph.Nodes {
				if platformGraph.Nodes[i].ID == b.NodeID {
					platformGraph.Nodes[i].Config = json.RawMessage(`{"assetIds":[]}`)
				}
			}
		}
		if err := validateTemplateRefs(tx, w.WorkspaceID, personalRefs); err != nil {
			return err
		}
		if err := validateOfficialRefs(tx, platformGraph); err != nil {
			return err
		}
		refs := officialRefs(platformGraph, versionID, "")
		ids := []int64{}
		seen := map[int64]bool{}
		for _, r := range refs {
			if !seen[r.AssetID] {
				seen[r.AssetID] = true
				ids = append(ids, r.AssetID)
			}
		}
		assets := []model.PODOfficialAsset{}
		files := []model.PODAssetFile{}
		if len(ids) > 0 {
			if err := tx.Where("asset_id IN ?", ids).Find(&assets).Error; err != nil {
				return err
			}
			if err := tx.Table("pod_catalog_asset_file").Where("asset_id IN ?", ids).Order("asset_id,variant").Find(&files).Error; err != nil {
				return err
			}
		}
		copies := []model.PODOfficialCopy{}
		var size int64
		by := strconv.FormatInt(user, 10)
		for _, a := range assets {
			cp := model.PODOfficialCopy{SourceAssetID: a.AssetID, Asset: model.PODAsset{Source: "official", OriginalName: a.Name, MediaType: a.MediaType, ByteSize: a.ByteSize, Width: a.Width, Height: a.Height, Revision: 1, DelFlag: "0", CreateBy: by, CreateTime: types.Now(), UpdateBy: by, UpdateTime: types.Now()}}
			for _, f := range files {
				if f.AssetID != a.AssetID {
					continue
				}
				cp.SourceFiles = append(cp.SourceFiles, f)
				copy := f
				key := fmt.Sprintf("%x", sha256.Sum256([]byte("official-copy:"+by+":"+body.RequestID+":"+strconv.FormatInt(a.AssetID, 10)+":"+f.Variant)))
				dot := strings.LastIndexByte(f.StorageKey, '.')
				if dot < 0 {
					return errors.New("invalid official file")
				}
				copy.StorageKey = key[:32] + f.StorageKey[dot:]
				copy.AssetID = 0
				copy.CreateBy = by
				copy.CreateTime = types.Now()
				copy.UpdateBy = by
				copy.UpdateTime = types.Now()
				cp.Files = append(cp.Files, copy)
				size += f.ByteSize
			}
			if len(cp.Files) != 2 {
				return errors.New("official asset needs original and thumbnail")
			}
			copies = append(copies, cp)
		}
		if size > 64<<20 {
			return errs.WithCode(400, "模板素材超过单次复制 64 MiB 上限")
		}
		key := "official:" + body.RequestID
		if size > 0 {
			if err := reservePODStorageTx(tx, user, key, size); err != nil {
				return err
			}
		}
		out = model.PODOfficialUseAction{UserID: user, WorkspaceID: w.WorkspaceID, RequestID: body.RequestID, RequestHash: hash, VersionID: versionID, State: "copying", CopyMapJSON: podJSON(model.PODOfficialCopyPlan{Copies: copies, PersonalNodes: personalNodes}), GraphJSON: podJSON(graph), Name: body.Name, StorageOperationKey: key, CreateBy: by, CreateTime: types.Now()}
		return tx.Create(&out).Error
	})
	return &out, err
}
func PODOfficialFinishUse(ctx context.Context, user int64, request, hash string) (*model.PODOfficialUseAction, error) {
	var a model.PODOfficialUseAction
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, user)
		if err != nil {
			return err
		}
		if err := tx.Where("user_id=? AND request_id=?", user, request).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&a).Error; err != nil {
			return operationMissing(err)
		}
		if a.RequestHash != hash {
			return ErrPODTemplateRequest
		}
		if a.State == "completed" {
			return nil
		}
		var plan model.PODOfficialCopyPlan
		var graph model.PODGraph
		if json.Unmarshal(a.CopyMapJSON, &plan) != nil || json.Unmarshal(a.GraphJSON, &graph) != nil {
			return errors.New("invalid official copy plan")
		}
		mapping := map[int64]int64{}
		allFiles := []model.PODAssetFile{}
		assetIDs := []int64{}
		assets := make([]model.PODAsset, 0, len(plan.Copies))
		for _, cp := range plan.Copies {
			asset := cp.Asset
			asset.WorkspaceID = w.WorkspaceID
			assets = append(assets, asset)
		}
		if len(assets) > 0 {
			if err := tx.Table("pod_asset").Create(&assets).Error; err != nil {
				return err
			}
		}
		for i, cp := range plan.Copies {
			asset := assets[i]
			mapping[cp.SourceAssetID] = asset.AssetID
			assetIDs = append(assetIDs, asset.AssetID)
			files := append([]model.PODAssetFile(nil), cp.Files...)
			for i := range files {
				files[i].AssetID = asset.AssetID
				files[i].UpdateBy = strconv.FormatInt(user, 10)
				files[i].UpdateTime = types.Now()
			}
			allFiles = append(allFiles, files...)
		}
		if len(allFiles) > 0 {
			if err := tx.Table("pod_asset_file").Create(&allFiles).Error; err != nil {
				return err
			}
			if err := commitPODStorage(tx, user, a.StorageOperationKey, assetIDs[0], allFiles); err != nil {
				return err
			}
			if err := tx.Table("pod_storage_reservation").Where("user_id=? AND operation_key=?", user, a.StorageOperationKey).Update("result_asset_ids_json", string(podJSON(assetIDs))).Error; err != nil {
				return err
			}
		}
		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if n.Type != "imageInput" {
				continue
			}
			personal := false
			for _, id := range plan.PersonalNodes {
				if id == n.ID {
					personal = true
				}
			}
			if personal {
				// syncPODProjectAssetRefs revalidates all personal and copied IDs together.
				continue
			}
			var c map[string]json.RawMessage
			if err := json.Unmarshal(n.Config, &c); err != nil {
				return err
			}
			var ids []string
			if err := json.Unmarshal(c["assetIds"], &ids); err != nil {
				return err
			}
			for j, raw := range ids {
				id, _ := strconv.ParseInt(raw, 10, 64)
				if mapping[id] == 0 {
					return errors.New("official asset mapping missing")
				}
				ids[j] = strconv.FormatInt(mapping[id], 10)
			}
			c["assetIds"] = podJSON(ids)
			n.Config = podJSON(c)
		}
		refs := []model.PODProjectAssetRef{}
		for _, r := range officialRefs(graph, 0, "") {
			refs = append(refs, model.PODProjectAssetRef{NodeID: r.NodeID, AssetID: r.AssetID})
		}
		p := model.PODProject{WorkspaceID: w.WorkspaceID, Name: a.Name, GraphJSON: podJSON(graph), SchemaVersion: graph.SchemaVersion, Revision: 1, DelFlag: "0", CreateBy: a.CreateBy, CreateTime: types.Now(), UpdateBy: a.CreateBy, UpdateTime: types.Now(), Remark: "pod-official:" + strconv.FormatInt(a.VersionID, 10)}
		if err := tx.Table("pod_project").Create(&p).Error; err != nil {
			return err
		}
		if err := syncPODProjectAssetRefs(tx, w.WorkspaceID, p.ProjectID, refs, a.CreateBy); err != nil {
			return err
		}
		a.State = "completed"
		a.ProjectID = p.ProjectID
		return tx.Model(&a).Updates(map[string]any{"state": "completed", "project_id": p.ProjectID, "update_by": a.CreateBy, "update_time": types.Now()}).Error
	})
	return &a, err
}

// Called under the same workspace lock as begin/finish, so a user's replacement
// images cannot disappear while an interrupted official copy is resumable.
func officialCopyUsesAsset(tx *gorm.DB, workspace, id int64) (bool, error) {
	var actions []model.PODOfficialUseAction
	if err := tx.Select("copy_map_json,graph_json").Where("workspace_id=? AND state='copying'", workspace).Find(&actions).Error; err != nil {
		return false, err
	}
	for _, a := range actions {
		var plan model.PODOfficialCopyPlan
		var graph model.PODGraph
		if json.Unmarshal(a.CopyMapJSON, &plan) != nil || json.Unmarshal(a.GraphJSON, &graph) != nil {
			return false, errors.New("official copy plan invalid")
		}
		personal := map[string]bool{}
		for _, node := range plan.PersonalNodes {
			personal[node] = true
		}
		for _, r := range officialRefs(graph, 0, "") {
			if personal[r.NodeID] && r.AssetID == id {
				return true, nil
			}
		}
	}
	return false, nil
}
