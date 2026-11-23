package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/storage"
	"strconv"
	"sync"
	"testing"
	"time"
)

func exercisePODAssets(t *testing.T, ctx context.Context, engine *gin.Engine, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage, tokens map[string]string, owner model.PODBootstrap) {
	token, other := tokens["pod_creator"], tokens["pod_creator_b"]
	assetIDs, projectIDs := []int64{}, []int64{}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if len(projectIDs) > 0 {
			if err := repository.DB(cleanup).Table("pod_project_asset_ref").Where("project_id IN ?", projectIDs).Delete(&model.PODProjectAssetRef{}).Error; err != nil {
				t.Error("reference cleanup failed")
			}
			if err := repository.DB(cleanup).Table("pod_project").Where("project_id IN ?", projectIDs).Delete(&model.PODProject{}).Error; err != nil {
				t.Error("project cleanup failed")
			}
		}
		if len(assetIDs) > 0 {
			var charges []struct {
				UserID int64
				Bytes  int64
			}
			if err := repository.DB(cleanup).Table("pod_asset a").Select("w.owner_user_id user_id,SUM(f.byte_size) bytes").Joins("JOIN pod_asset_file f ON f.asset_id=a.asset_id").Joins("JOIN pod_workspace w ON w.workspace_id=a.workspace_id").Where("a.asset_id IN ?", assetIDs).Group("w.owner_user_id").Scan(&charges).Error; err != nil {
				t.Error(err)
				return
			}
			var files []model.PODAssetFile
			if err := repository.DB(cleanup).Table("pod_asset_file").Where("asset_id IN ?", assetIDs).Find(&files).Error; err != nil {
				t.Error("file cleanup lookup failed")
				return
			}
			store := storage.Local{Path: cfg.POD.StorageRoot}
			for _, file := range files {
				if err := store.Remove(cleanup, file.StorageKey); err != nil {
					t.Error("private test file cleanup failed")
					return
				}
			}
			if err := repository.DB(cleanup).Transaction(func(tx *gorm.DB) error {
				if err := tx.Table("pod_asset_file").Where("asset_id IN ?", assetIDs).Delete(&model.PODAssetFile{}).Error; err != nil {
					return err
				}
				if err := tx.Table("pod_asset").Where("asset_id IN ?", assetIDs).Delete(&model.PODAsset{}).Error; err != nil {
					return err
				}
				if err := tx.Exec("DELETE FROM pod_storage_reservation WHERE state='committed' AND JSON_LENGTH(result_asset_ids_json)=1 AND CAST(JSON_UNQUOTE(JSON_EXTRACT(result_asset_ids_json,'$[0]')) AS UNSIGNED) IN ?", assetIDs).Error; err != nil {
					return err
				}
				for _, c := range charges {
					r := tx.Table("pod_storage_account").Where("user_id=? AND used_bytes>=?", c.UserID, c.Bytes).Updates(map[string]any{"used_bytes": gorm.Expr("used_bytes-?", c.Bytes), "revision": gorm.Expr("revision+1"), "update_by": "pod-test-cleanup", "update_time": time.Now()})
					if r.Error != nil {
						return r.Error
					}
					if r.RowsAffected != 1 {
						return errors.New("test storage account mismatch")
					}
				}
				return nil
			}); err != nil {
				t.Error(err)
			}
		}
	}()
	code := func(payload map[string]json.RawMessage) int {
		var n int
		_ = json.Unmarshal(payload["code"], &n)
		return n
	}
	expect := func(t *testing.T, p map[string]json.RawMessage, want int) {
		t.Helper()
		if got := code(p); got != want {
			t.Fatalf("business code %d, want %d", got, want)
		}
	}
	payload := func(t *testing.T, r *httptest.ResponseRecorder) map[string]json.RawMessage {
		t.Helper()
		var p map[string]json.RawMessage
		if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &p) != nil {
			t.Fatalf("unexpected response status/type %d", r.Code)
		}
		return p
	}
	getView := func(t *testing.T, p map[string]json.RawMessage) model.PODAssetView {
		t.Helper()
		expect(t, p, 200)
		var view model.PODAssetView
		if json.Unmarshal(p["data"], &view) != nil {
			t.Fatal("asset DTO invalid")
		}
		return view
	}
	route := func(id int64) string { return "/pod/assets/" + strconv.FormatInt(id, 10) }
	revision := func(n int64) string { return strconv.FormatInt(n, 10) }
	binary := func(path, session string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if session != "" {
			r.Header.Set("Authorization", "Bearer "+session)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	upload := func(name, media string, data []byte, session string, extra bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
		header.Set("Content-Type", media)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
		if extra {
			part, _ = writer.CreateFormField("url")
			_, _ = part.Write([]byte("http://127.0.0.1/private"))
		}
		_ = writer.Close()
		r := httptest.NewRequest("POST", "/pod/assets", &body)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		if session != "" {
			r.Header.Set("Authorization", "Bearer "+session)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	imageData := image.NewNRGBA(image.Rect(0, 0, 640, 320))
	imageData.SetNRGBA(20, 20, color.NRGBA{G: 255, A: 180})
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, imageData)
	raw := encoded.Bytes()
	createAsset := func(t *testing.T, session string) model.PODAssetView {
		t.Helper()
		view := getView(t, payload(t, upload("阶段3透明印花.png", "image/png", raw, session, false)))
		assetIDs = append(assetIDs, view.AssetID)
		return view
	}
	createProject := func(t *testing.T) model.PODProjectView {
		t.Helper()
		p := call("POST", "/pod/projects", token, map[string]any{"name": "阶段3素材引用测试"})
		expect(t, p, 200)
		var project model.PODProjectView
		_ = json.Unmarshal(p["data"], &project)
		projectIDs = append(projectIDs, project.ProjectID)
		return project
	}
	graph := func(id int64) model.PODGraph {
		return model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "reference", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":["` + revision(id) + `"]}`)}}, Edges: []model.PODEdge{}}
	}
	var asset, foreign model.PODAssetView
	t.Run("upload_metadata_and_registered_derivative", func(t *testing.T) {
		asset = createAsset(t, token)
		foreign = createAsset(t, other)
		if asset.AssetID == 0 || asset.WorkspaceID != owner.Workspace.WorkspaceID || asset.Width != 640 || asset.Height != 320 || asset.ByteSize != int64(len(raw)) || asset.Source != "upload" {
			t.Fatal("invalid asset metadata")
		}
		p := call("GET", route(asset.AssetID), token, nil)
		expect(t, p, 200)
		for _, private := range []string{"storageKey", "storage_key", ".local", "sha256"} {
			if bytes.Contains(p["data"], []byte(private)) {
				t.Fatal("private storage metadata leaked")
			}
		}
		var files []model.PODAssetFile
		if err := repository.DB(ctx).Table("pod_asset_file").Where("asset_id=?", asset.AssetID).Find(&files).Error; err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Fatal("original/thumbnail records missing")
		}
		for _, file := range files {
			if len(file.SHA256) != 64 || file.ByteSize < 1 {
				t.Fatal("file metadata invalid")
			}
		}
	})
	if asset.AssetID == 0 || foreign.AssetID == 0 {
		t.Fatal("asset setup failed")
	}
	t.Run("authenticated_original_and_thumbnail", func(t *testing.T) {
		original := binary(route(asset.AssetID)+"/download", token)
		if original.Code != 200 || !bytes.Equal(original.Body.Bytes(), raw) || original.Header().Get("Content-Type") != "image/png" || !bytes.Contains([]byte(original.Header().Get("Content-Disposition")), []byte("attachment")) {
			t.Fatal("download bytes or headers invalid")
		}
		if original.Header().Get("Cache-Control") != "private, no-store" || original.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("private image response lacks headers")
		}
		thumb := binary(route(asset.AssetID)+"/content?variant=thumbnail", token)
		decoded, err := png.Decode(bytes.NewReader(thumb.Body.Bytes()))
		if err != nil || decoded.Bounds().Dx() != 480 || decoded.Bounds().Dy() != 240 {
			t.Fatal("thumbnail not generated")
		}
		_, _, _, alpha := decoded.At(100, 100).RGBA()
		if alpha != 0 {
			t.Fatal("thumbnail alpha lost")
		}
	})
	t.Run("foreign_and_unauthenticated_content_blocked", func(t *testing.T) {
		for _, suffix := range []string{"", "/content", "/content?variant=original", "/download"} {
			expect(t, payload(t, binary(route(asset.AssetID)+suffix, other)), 404)
			expect(t, payload(t, binary(route(asset.AssetID)+suffix, "")), 401)
		}
		expect(t, call("DELETE", route(asset.AssetID), other, map[string]any{"revision": "1"}), 404)
		expect(t, call("POST", route(asset.AssetID)+"/restore", other, map[string]any{"revision": "1"}), 404)
		expect(t, payload(t, binary(route(asset.AssetID)+"/content?token="+token, "")), 401)
		listed := call("GET", "/pod/assets?pageSize=100", other, nil)
		expect(t, listed, 200)
		var rows []model.PODAssetView
		_ = json.Unmarshal(listed["rows"], &rows)
		for _, row := range rows {
			if row.AssetID == asset.AssetID {
				t.Fatal("foreign asset listed")
			}
		}
	})
	t.Run("invalid_uploads_and_path_inputs_rejected", func(t *testing.T) {
		for _, item := range []struct {
			name, media string
			bytes       []byte
			extra       bool
		}{
			{"wrong.jpg", "image/jpeg", raw, false}, {"wrong.png", "image/png", []byte("<svg onload='x'/>"), false}, {`..\\secret.png`, "image/png", raw, false}, {"ok.png", "image/png", raw, true}, {"large.png", "image/png", make([]byte, owner.AssetLimits.MaxUploadBytes+1), false},
		} {
			p := payload(t, upload(item.name, item.media, item.bytes, token, item.extra))
			if code(p) == 200 {
				var accidental model.PODAssetView
				_ = json.Unmarshal(p["data"], &accidental)
				assetIDs = append(assetIDs, accidental.AssetID)
			}
			if code(p) != 400 {
				t.Errorf("upload case %q / extra=%v: code %d, want 400", item.name, item.extra, code(p))
			}
		}
		expect(t, call("POST", "/pod/assets", token, map[string]any{"url": "http://127.0.0.1/private"}), 400)
		expect(t, payload(t, binary(route(asset.AssetID)+"/content?variant=../../secret", token)), 400)
		expect(t, call("GET", "/pod/assets/01", token, nil), 400)
	})
	t.Run("graph_ownership_and_reference_protection", func(t *testing.T) {
		project := createProject(t)
		path := "/pod/projects/" + revision(project.ProjectID)
		expect(t, call("PUT", path, token, map[string]any{"revision": "1", "graph": graph(foreign.AssetID)}), 400)
		expect(t, call("PUT", path, token, map[string]any{"revision": "1", "graph": graph(asset.AssetID)}), 200)
		if getView(t, call("GET", route(asset.AssetID), token, nil)).ReferenceCount != 1 {
			t.Fatal("asset reference not registered")
		}
		expect(t, call("PUT", path, token, map[string]any{"revision": "2", "graph": graph(foreign.AssetID)}), 400)
		if getView(t, call("GET", route(asset.AssetID), token, nil)).ReferenceCount != 1 {
			t.Fatal("failed graph save removed committed references")
		}
		expect(t, call("DELETE", route(asset.AssetID), token, map[string]any{"revision": "1"}), 409)
		expect(t, call("DELETE", path, token, map[string]any{"revision": "2"}), 200)
		expect(t, call("DELETE", route(asset.AssetID), token, map[string]any{"revision": "1"}), 409)
		expect(t, call("POST", path+"/restore", token, map[string]any{"revision": "3"}), 200)
		expect(t, call("PUT", path, token, map[string]any{"revision": "4", "graph": model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}}}), 200)
		if getView(t, call("GET", route(asset.AssetID), token, nil)).ReferenceCount != 0 {
			t.Fatal("removed references remain")
		}
	})
	t.Run("trash_restore_and_stale_revision", func(t *testing.T) {
		deleted := getView(t, call("DELETE", route(asset.AssetID), token, map[string]any{"revision": "1"}))
		if !deleted.Deleted || deleted.Revision != 2 {
			t.Fatal("soft delete invalid")
		}
		expect(t, payload(t, binary(route(asset.AssetID)+"/download", token)), 404)
		expect(t, call("POST", route(asset.AssetID)+"/restore", token, map[string]any{"revision": "1"}), 409)
		restored := getView(t, call("POST", route(asset.AssetID)+"/restore", token, map[string]any{"revision": "2"}))
		if restored.Deleted || restored.Revision != 3 {
			t.Fatal("restore invalid")
		}
		if !bytes.Equal(binary(route(asset.AssetID)+"/download", token).Body.Bytes(), raw) {
			t.Fatal("restored bytes changed")
		}
	})
	t.Run("revocation_blocks_upload_list_and_download", func(t *testing.T) {
		update := func(state string) {
			if err := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=? AND user_id=?", owner.Workspace.WorkspaceID, owner.UserID).Update("status", state).Error; err != nil {
				t.Fatal(err)
			}
		}
		update("1")
		defer update("0")
		expect(t, call("GET", "/pod/assets", token, nil), 403)
		expect(t, payload(t, binary(route(asset.AssetID)+"/download", token)), 403)
		expect(t, payload(t, upload("revoked.png", "image/png", raw, token, false)), 403)
	})
	t.Run("concurrent_save_delete_preserves_integrity", func(t *testing.T) {
		concurrent := createAsset(t, token)
		project := createProject(t)
		var wait sync.WaitGroup
		wait.Add(2)
		codes := make(chan int, 2)
		for _, job := range []struct {
			method, path string
			body         any
		}{{"PUT", "/pod/projects/" + revision(project.ProjectID), map[string]any{"revision": "1", "graph": graph(concurrent.AssetID)}}, {"DELETE", route(concurrent.AssetID), map[string]any{"revision": "1"}}} {
			go func(job struct {
				method, path string
				body         any
			}) {
				defer wait.Done()
				codes <- code(call(job.method, job.path, token, job.body))
			}(job)
		}
		wait.Wait()
		close(codes)
		success := 0
		for result := range codes {
			if result == 200 {
				success++
			} else if result != 400 && result != 409 {
				t.Fatalf("unexpected race result %d", result)
			}
		}
		if success != 1 {
			t.Fatal("both conflicting operations succeeded")
		}
		var count int64
		err := repository.DB(ctx).Table("pod_project_asset_ref r").Joins("JOIN pod_asset a ON a.asset_id=r.asset_id").Where("r.project_id=? AND a.del_flag<>'0'", project.ProjectID).Count(&count).Error
		if err != nil || count != 0 {
			t.Fatal("project references deleted asset")
		}
	})
	t.Run("migration_recheck_is_idempotent", func(t *testing.T) {
		if err := repository.MigratePODAssets(ctx, cfg); err != nil {
			t.Fatal("ready migration cannot be rechecked")
		}
	})
	t.Run("bounded_stable_asset_pagination", func(t *testing.T) {
		fixtures := make([]model.PODAsset, 103)
		stamp := asset.CreateTime
		for i := range fixtures {
			fixtures[i] = model.PODAsset{WorkspaceID: owner.Workspace.WorkspaceID, Source: "upload", OriginalName: "阶段3分页夹具.png", MediaType: "image/png", ByteSize: 1, Width: 1, Height: 1, Revision: 1, DelFlag: "0", CreateBy: "pod-asset-test", UpdateBy: "pod-asset-test", CreateTime: stamp, UpdateTime: stamp}
		}
		if err := repository.DB(ctx).Table("pod_asset").Create(&fixtures).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range fixtures {
			assetIDs = append(assetIDs, row.AssetID)
		}
		first := call("GET", "/pod/assets?pageNum=1&pageSize=9999", token, nil)
		second := call("GET", "/pod/assets?pageNum=2&pageSize=100", token, nil)
		expect(t, first, 200)
		expect(t, second, 200)
		var a, b []model.PODAssetView
		_ = json.Unmarshal(first["rows"], &a)
		_ = json.Unmarshal(second["rows"], &b)
		if len(a) != 100 || len(b) < 3 {
			t.Fatal("asset page bound or count invalid")
		}
		seen := map[int64]bool{}
		for _, row := range append(a, b...) {
			if seen[row.AssetID] {
				t.Fatal("asset repeated across static pages")
			}
			seen[row.AssetID] = true
			if row.WorkspaceID != owner.Workspace.WorkspaceID {
				t.Fatal("foreign workspace in asset page")
			}
		}
	})
	t.Run("invalid_disk_metadata_never_becomes_a_path", func(t *testing.T) {
		var original model.PODAssetFile
		if err := repository.DB(ctx).Table("pod_asset_file").Where("asset_id=? AND variant='original'", asset.AssetID).Take(&original).Error; err != nil {
			t.Fatal(err)
		}
		if err := repository.DB(ctx).Table("pod_asset_file").Where("asset_id=? AND variant='original'", asset.AssetID).Update("storage_key", "../outside.png").Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repository.DB(cleanup).Table("pod_asset_file").Where("asset_id=? AND variant='original'", asset.AssetID).Update("storage_key", original.StorageKey).Error; err != nil {
				t.Error("test metadata restoration failed")
			}
		}()
		expect(t, payload(t, binary(route(asset.AssetID)+"/download", token)), 404)
	})
	t.Run("asset_dto_and_deleted_filters_are_strict", func(t *testing.T) {
		expect(t, call("DELETE", route(asset.AssetID), token, map[string]any{"revision": 3}), 400)
		expect(t, call("DELETE", route(asset.AssetID), token, map[string]any{"revision": "3", "workspaceId": "999"}), 400)
		expect(t, call("GET", "/pod/assets?deleted=1", token, nil), 400)
	})
	t.Run("case_sensitive_node_ids_keep_distinct_references", func(t *testing.T) {
		project := createProject(t)
		g := graph(asset.AssetID)
		a := g.Nodes[0]
		a.ID = "A"
		b := a
		b.ID = "a"
		g.Nodes = []model.PODNode{a, b}
		expect(t, call("PUT", "/pod/projects/"+revision(project.ProjectID), token, map[string]any{"revision": "1", "graph": g}), 200)
		var count int64
		if err := repository.DB(ctx).Table("pod_project_asset_ref").Where("project_id=? AND asset_id=?", project.ProjectID, asset.AssetID).Count(&count).Error; err != nil || count != 2 {
			t.Fatal("case-sensitive node identity lost")
		}
	})
}
