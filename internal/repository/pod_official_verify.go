package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"ruoyi-go/internal/model"
)

type PODOfficialRestoreCounts struct{ Templates, Versions, Assets, UseActions, Copying, Completed int }

// Offline restore verifies historical identity and references without requiring
// current publication, user eligibility, or model availability.
func PODVerifyOfficialData(ctx context.Context) (*PODOfficialRestoreCounts, error) {
	out := &PODOfficialRestoreCounts{}
	checks := []string{
		`SELECT COUNT(*) FROM pod_catalog_template t LEFT JOIN pod_catalog_template_version c ON c.version_id=t.current_version_id AND c.template_id=t.template_id LEFT JOIN pod_catalog_template_version p ON p.version_id=t.published_version_id AND p.template_id=t.template_id WHERE c.version_id IS NULL OR t.status NOT IN ('draft','published','unpublished') OR (t.published_version_id IS NOT NULL AND (p.version_id IS NULL OR p.published_at IS NULL)) OR (t.status='published' AND t.published_version_id IS NULL)`,
		`SELECT COUNT(*) FROM pod_catalog_template_version v LEFT JOIN pod_catalog_template t ON t.template_id=v.template_id WHERE t.template_id IS NULL`,
		`SELECT COUNT(*) FROM pod_catalog_asset a WHERE a.license_note='' OR a.status<>'active' OR (SELECT COUNT(*) FROM pod_catalog_asset_file f WHERE f.asset_id=a.asset_id)<>2 OR NOT EXISTS (SELECT 1 FROM pod_catalog_asset_file f WHERE f.asset_id=a.asset_id AND f.variant='original' AND f.byte_size=a.byte_size AND f.media_type=a.media_type AND f.width=a.width AND f.height=a.height) OR NOT EXISTS (SELECT 1 FROM pod_catalog_asset_file f WHERE f.asset_id=a.asset_id AND f.variant='thumbnail')`,
		`SELECT COUNT(*) FROM pod_catalog_asset_file f LEFT JOIN pod_catalog_asset a ON a.asset_id=f.asset_id WHERE a.asset_id IS NULL OR f.byte_size<=0 OR LENGTH(f.sha256)<>64 OR f.variant NOT IN ('original','thumbnail')`,
		`SELECT COUNT(*) FROM pod_catalog_template_asset_ref r LEFT JOIN pod_catalog_template_version v ON v.version_id=r.version_id LEFT JOIN pod_catalog_asset a ON a.asset_id=r.asset_id WHERE v.version_id IS NULL OR a.asset_id IS NULL`,
		`SELECT COUNT(*) FROM pod_catalog_use_action a LEFT JOIN pod_catalog_template_version v ON v.version_id=a.version_id LEFT JOIN pod_workspace w ON w.workspace_id=a.workspace_id AND w.owner_user_id=a.user_id LEFT JOIN pod_project p ON p.project_id=a.project_id AND p.workspace_id=a.workspace_id WHERE v.version_id IS NULL OR w.workspace_id IS NULL OR a.state NOT IN ('copying','completed') OR (a.state='copying' AND a.project_id<>0) OR (a.state='completed' AND p.project_id IS NULL)`,
	}
	for i, query := range checks {
		var n int64
		if err := DB(ctx).Raw(query).Scan(&n).Error; err != nil {
			return nil, err
		}
		if n != 0 {
			return nil, fmt.Errorf("官方目录关联校验失败 (%d)", i+1)
		}
	}
	var versions []model.PODOfficialVersion
	var refs []model.PODOfficialRef
	var platform []model.PODAssetFile
	var actions []model.PODOfficialUseAction
	var files []model.PODAssetFile
	var reservations []model.PODStorageReservation
	if err := DB(ctx).Find(&versions).Error; err != nil {
		return nil, err
	}
	if err := DB(ctx).Find(&refs).Error; err != nil {
		return nil, err
	}
	if err := DB(ctx).Table("pod_catalog_asset_file").Find(&platform).Error; err != nil {
		return nil, err
	}
	if err := DB(ctx).Find(&actions).Error; err != nil {
		return nil, err
	}
	if err := DB(ctx).Table("pod_asset_file").Find(&files).Error; err != nil {
		return nil, err
	}
	if err := DB(ctx).Find(&reservations).Error; err != nil {
		return nil, err
	}
	actualRefs := map[int64]map[string]bool{}
	for _, r := range refs {
		if actualRefs[r.VersionID] == nil {
			actualRefs[r.VersionID] = map[string]bool{}
		}
		actualRefs[r.VersionID][fmt.Sprintf("%s:%d", r.NodeID, r.AssetID)] = true
	}
	for _, v := range versions {
		var graph model.PODGraph
		if json.Unmarshal(v.GraphJSON, &graph) != nil {
			return nil, errors.New("官方版本图无效")
		}
		// MySQL JSON rewrites whitespace; re-encode config maps before hashing.
		canonical, err := canonicalOfficialGraph(graph)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(podJSON(canonical))) != v.SpecHash {
			return nil, errors.New("官方版本摘要校验失败")
		}
		expected := map[string]bool{}
		for _, r := range officialRefs(graph, v.VersionID, "") {
			expected[fmt.Sprintf("%s:%d", r.NodeID, r.AssetID)] = true
		}
		if len(expected) == 0 && len(actualRefs[v.VersionID]) == 0 {
			continue
		}
		if !reflect.DeepEqual(expected, actualRefs[v.VersionID]) {
			return nil, errors.New("官方版本素材引用校验失败")
		}
	}
	platformMap := map[string]model.PODAssetFile{}
	for _, f := range platform {
		platformMap[fmt.Sprintf("%d:%s", f.AssetID, f.Variant)] = f
	}
	privateMap := map[string]model.PODAssetFile{}
	for _, f := range files {
		privateMap[f.StorageKey] = f
	}
	var assets []model.PODAsset
	if err := DB(ctx).Table("pod_asset").Select("asset_id,workspace_id").Find(&assets).Error; err != nil {
		return nil, err
	}
	assetOwners := map[int64]int64{}
	for _, a := range assets {
		assetOwners[a.AssetID] = a.WorkspaceID
	}
	reservationMap := map[string]model.PODStorageReservation{}
	for _, r := range reservations {
		reservationMap[fmt.Sprintf("%d:%s", r.UserID, r.OperationKey)] = r
	}
	for _, a := range actions {
		var plan model.PODOfficialCopyPlan
		var graph model.PODGraph
		if json.Unmarshal(a.CopyMapJSON, &plan) != nil || json.Unmarshal(a.GraphJSON, &graph) != nil || a.StorageOperationKey != "official:"+a.RequestID {
			return nil, errors.New("官方复制计划无效")
		}
		var size int64
		for _, cp := range plan.Copies {
			if len(cp.Files) != 2 || len(cp.SourceFiles) != 2 {
				return nil, errors.New("官方复制文件计划不完整")
			}
			var privateAsset int64
			for i, src := range cp.SourceFiles {
				original, ok := platformMap[fmt.Sprintf("%d:%s", cp.SourceAssetID, src.Variant)]
				dst := cp.Files[i]
				if !ok || original.SHA256 != src.SHA256 || original.StorageKey != src.StorageKey || dst.ByteSize != src.ByteSize || dst.SHA256 != src.SHA256 || dst.Variant != src.Variant {
					return nil, errors.New("官方复制源与计划不一致")
				}
				size += dst.ByteSize
				if a.State == "completed" {
					actual, ok := privateMap[dst.StorageKey]
					if !ok || actual.ByteSize != dst.ByteSize || actual.SHA256 != dst.SHA256 || assetOwners[actual.AssetID] != a.WorkspaceID || (privateAsset > 0 && privateAsset != actual.AssetID) {
						return nil, errors.New("官方复制结果文件不一致")
					}
					privateAsset = actual.AssetID
				}
			}
		}
		if size > 0 {
			r, ok := reservationMap[fmt.Sprintf("%d:%s", a.UserID, a.StorageOperationKey)]
			if !ok || r.ReservedBytes != size || (a.State == "copying" && r.State != "held") || (a.State == "completed" && (r.State != "committed" || r.CommittedBytes != size)) {
				return nil, errors.New("官方复制与存储预留不一致")
			}
		}
		if a.State == "copying" {
			out.Copying++
		} else {
			out.Completed++
		}
	}
	var n int64
	if err := DB(ctx).Table("pod_catalog_template").Count(&n).Error; err != nil {
		return nil, err
	}
	out.Templates = int(n)
	if err := DB(ctx).Table("pod_catalog_asset").Count(&n).Error; err != nil {
		return nil, err
	}
	out.Assets = int(n)
	out.Versions = len(versions)
	out.UseActions = len(actions)
	return out, nil
}

func PODOfficialPendingCopyFiles(ctx context.Context) ([]model.PODAssetFile, error) {
	var rows []model.PODOfficialUseAction
	if err := DB(ctx).Select("copy_map_json").Where("state='copying'").Find(&rows).Error; err != nil {
		return nil, err
	}
	files := []model.PODAssetFile{}
	for _, a := range rows {
		var plan model.PODOfficialCopyPlan
		if err := json.Unmarshal(a.CopyMapJSON, &plan); err != nil {
			return nil, err
		}
		for _, cp := range plan.Copies {
			files = append(files, cp.Files...)
		}
	}
	return files, nil
}

func canonicalOfficialGraph(g model.PODGraph) (model.PODGraph, error) {
	g.Nodes = append([]model.PODNode(nil), g.Nodes...)
	for i := range g.Nodes {
		var c any
		if err := json.Unmarshal(g.Nodes[i].Config, &c); err != nil {
			return g, err
		}
		g.Nodes[i].Config = podJSON(c)
	}
	return g, nil
}
