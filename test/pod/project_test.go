package podtest

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
)

func exercisePODProjects(t *testing.T, ctx context.Context, call func(string, string, string, any) map[string]json.RawMessage, tokens map[string]string, owner model.PODBootstrap) {
	token, other := tokens["pod_creator"], tokens["pod_creator_b"]
	ids := []int64{}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if len(ids) > 0 {
			if err := repository.DB(cleanup).Table("pod_project").Where("project_id IN ?", ids).Delete(&model.PODProject{}).Error; err != nil {
				t.Error("test project cleanup failed")
			}
		}
	}()
	code := func(p map[string]json.RawMessage) int { var n int; _ = json.Unmarshal(p["code"], &n); return n }
	expect := func(t *testing.T, p map[string]json.RawMessage, want int) {
		t.Helper()
		if got := code(p); got != want {
			t.Fatalf("business code %d, want %d", got, want)
		}
	}
	view := func(t *testing.T, p map[string]json.RawMessage) model.PODProjectView {
		t.Helper()
		expect(t, p, 200)
		var v model.PODProjectView
		if err := json.Unmarshal(p["data"], &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	path := func(id int64) string { return "/pod/projects/" + strconv.FormatInt(id, 10) }
	rev := func(v model.PODProjectView) string { return strconv.FormatInt(v.Revision, 10) }
	var project model.PODProjectView
	t.Run("create_and_explicit_workspace_idempotency", func(t *testing.T) {
		expect(t, call("POST", "/pod/workspace", token, map[string]any{}), 200)
		expect(t, call("POST", "/pod/workspace", token, map[string]any{}), 200)
		project = view(t, call("POST", "/pod/projects", token, map[string]any{"name": "阶段2回归项目"}))
		ids = append(ids, project.ProjectID)
		if project.Graph == nil || project.Graph.Nodes == nil || project.Graph.Edges == nil || project.Revision != 1 || project.WorkspaceID != owner.Workspace.WorkspaceID {
			t.Fatal("new project contract invalid")
		}
	})
	if project.ProjectID == 0 {
		t.Fatal("test project not created")
	}
	graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "brief", Type: "textInput", Version: 1, Position: model.PODPosition{X: 120, Y: 240}, Config: json.RawMessage(`{"text":"蓝绿色印花","title":"创意"}`)}, {ID: "art", Type: "imageGenerate", Version: 1, Position: model.PODPosition{X: 500, Y: 240}, Config: json.RawMessage(`{"ratio":"3:4","count":2}`)}}, Edges: []model.PODEdge{{ID: "brief_art", Source: "brief", SourcePort: "text", Target: "art", TargetPort: "text"}}}
	t.Run("save_and_reopen_exact_graph", func(t *testing.T) {
		project = view(t, call("PUT", path(project.ProjectID), token, map[string]any{"revision": rev(project), "graph": graph, "name": "已保存图案"}))
		reopened := view(t, call("GET", path(project.ProjectID), token, nil))
		a, _ := json.Marshal(graph)
		b, _ := json.Marshal(reopened.Graph)
		var expected, actual any
		_ = json.Unmarshal(a, &expected)
		_ = json.Unmarshal(b, &actual)
		if !reflect.DeepEqual(expected, actual) || reopened.Name != "已保存图案" || reopened.Revision != 2 {
			t.Fatal("graph or revision did not survive reload")
		}
	})
	t.Run("foreign_reads_and_all_mutations_rejected", func(t *testing.T) {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			body := any(nil)
			if method == "PUT" {
				body = map[string]any{"revision": rev(project), "name": "cannot overwrite"}
			}
			if method == "DELETE" {
				body = map[string]any{"revision": rev(project)}
			}
			expect(t, call(method, path(project.ProjectID), other, body), 404)
		}
		expect(t, call("POST", path(project.ProjectID)+"/restore", other, map[string]any{"revision": rev(project)}), 404)
		listed := call("GET", "/pod/projects", other, nil)
		expect(t, listed, 200)
		var rows []model.PODProjectView
		_ = json.Unmarshal(listed["rows"], &rows)
		for _, row := range rows {
			if row.ProjectID == project.ProjectID {
				t.Fatal("foreign project leaked")
			}
		}
	})
	t.Run("stale_save_cannot_overwrite", func(t *testing.T) {
		expect(t, call("PUT", path(project.ProjectID), token, map[string]any{"revision": "1", "name": "stale"}), 409)
		unchanged := view(t, call("GET", path(project.ProjectID), token, nil))
		if unchanged.Name != project.Name || unchanged.Revision != project.Revision {
			t.Fatal("stale writer changed project")
		}
	})
	t.Run("concurrent_saves_have_one_winner", func(t *testing.T) {
		results := make(chan int, 2)
		for _, name := range []string{"并发 A", "并发 B"} {
			go func(name string) {
				results <- code(call("PUT", path(project.ProjectID), token, map[string]any{"revision": rev(project), "name": name}))
			}(name)
		}
		counts := map[int]int{}
		for i := 0; i < 2; i++ {
			select {
			case c := <-results:
				counts[c]++
			case <-ctx.Done():
				t.Fatal("concurrency check timed out")
			}
		}
		if counts[200] != 1 || counts[409] != 1 {
			t.Fatalf("concurrency codes: %v", counts)
		}
		project = view(t, call("GET", path(project.ProjectID), token, nil))
	})
	t.Run("strict_inputs_and_graph_validation", func(t *testing.T) {
		expect(t, call("POST", "/pod/projects", token, map[string]any{"name": "", "workspaceId": "1"}), 400)
		expect(t, call("POST", "/pod/projects", token, map[string]any{"name": "  "}), 400)
		expect(t, call("GET", "/pod/projects/0001", token, nil), 400)
		expect(t, call("PUT", path(project.ProjectID), token, map[string]any{"revision": project.Revision, "name": "numeric revision"}), 400)
		for _, invalid := range []any{
			map[string]any{"schemaVersion": 1, "nodes": nil, "edges": []any{}},
			map[string]any{"schemaVersion": 1, "nodes": []any{map[string]any{"id": "x", "type": "textInput", "version": 1, "position": map[string]any{"x": 0, "y": 0}, "config": map[string]any{"apiKey": "test-value"}}}, "edges": []any{}},
			map[string]any{"schemaVersion": 1, "nodes": graph.Nodes, "edges": []any{map[string]any{"id": "bad", "source": "brief", "sourcePort": "text", "target": "art", "targetPort": "images"}}},
		} {
			expect(t, call("PUT", path(project.ProjectID), token, map[string]any{"revision": rev(project), "graph": invalid}), 400)
		}
	})
	t.Run("deleted_projects_restore_and_increment_version", func(t *testing.T) {
		before := rev(project)
		project = view(t, call("DELETE", path(project.ProjectID), token, map[string]any{"revision": before}))
		if !project.Deleted {
			t.Fatal("delete was not soft")
		}
		expect(t, call("GET", path(project.ProjectID), token, nil), 404)
		expect(t, call("POST", path(project.ProjectID)+"/restore", other, map[string]any{"revision": rev(project)}), 404)
		expect(t, call("PUT", path(project.ProjectID), token, map[string]any{"revision": rev(project), "name": "deleted save"}), 404)
		expect(t, call("POST", path(project.ProjectID)+"/restore", token, map[string]any{"revision": before}), 409)
		project = view(t, call("POST", path(project.ProjectID)+"/restore", token, map[string]any{"revision": rev(project)}))
		if project.Deleted {
			t.Fatal("restore failed")
		}
		reopened := view(t, call("GET", path(project.ProjectID), token, nil))
		if reopened.Graph == nil || len(reopened.Graph.Nodes) != 2 {
			t.Fatal("deleted graph was lost")
		}
	})
	t.Run("first_workspace_for_new_creator", func(t *testing.T) {
		password := uuid.NewString()[:18]
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		dept := int64(100)
		user := model.SysUser{DeptID: &dept, UserName: "pod2_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12], NickName: "阶段2测试", UserType: "00", Password: string(hash), Status: "0", DelFlag: "0", CreateTime: types.Now(), UpdateTime: types.Now(), PwdUpdateDate: types.Now()}
		if err := repository.DB(ctx).Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var workspaceIDs []int64
			if err := repository.DB(cleanup).Table("pod_workspace").Where("owner_user_id=?", user.UserID).Pluck("workspace_id", &workspaceIDs).Error; err != nil {
				t.Error(err)
				return
			}
			if len(workspaceIDs) > 0 {
				cleanupCreatorFixtures(t, cleanup, workspaceIDs)
				if err := repository.DB(cleanup).Table("pod_project").Where("workspace_id IN ?", workspaceIDs).Delete(&model.PODProject{}).Error; err != nil {
					t.Error(err)
				}
				if err := repository.DB(cleanup).Table("pod_workspace_member").Where("user_id=?", user.UserID).Delete(&model.PODWorkspace{}).Error; err != nil {
					t.Error(err)
				}
				if err := repository.DB(cleanup).Table("pod_workspace").Where("owner_user_id=?", user.UserID).Delete(&model.PODWorkspace{}).Error; err != nil {
					t.Error(err)
				}
			}
			if err := repository.DB(cleanup).Table("sys_user_role").Where("user_id=?", user.UserID).Delete(&model.SysUser{}).Error; err != nil {
				t.Error(err)
			}
			if err := repository.DB(cleanup).Where("user_id=?", user.UserID).Delete(&model.SysUser{}).Error; err != nil {
				t.Error(err)
			}
		})
		var role struct{ RoleID int64 }
		if err := repository.DB(ctx).Table("sys_role").Select("role_id").Where("role_key='pod_creator'").Take(&role).Error; err != nil {
			t.Fatal(err)
		}
		if err := repository.DB(ctx).Table("sys_user_role").Create(map[string]any{"user_id": user.UserID, "role_id": role.RoleID}).Error; err != nil {
			t.Fatal(err)
		}
		captchaID := "pod-stage2-" + uuid.NewString()
		if err := redisx.C().Set(ctx, redisx.CaptchaKey(captchaID), "2468", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		login := call("POST", "/login", "", model.LoginBody{Username: user.UserName, Password: password, UUID: captchaID, Code: "2468"})
		expect(t, login, 200)
		var newToken string
		_ = json.Unmarshal(login["token"], &newToken)
		if newToken == "" {
			t.Fatal("token missing")
		}
		defer call("POST", "/logout", newToken, nil)
		expect(t, call("GET", "/pod/bootstrap", newToken, nil), 403)
		firstInit := call("POST", "/pod/workspace", newToken, map[string]any{})
		expect(t, firstInit, 200)
		secondInit := call("POST", "/pod/workspace", newToken, map[string]any{})
		expect(t, secondInit, 200)
		var a, b model.PODWorkspace
		_ = json.Unmarshal(firstInit["data"], &a)
		_ = json.Unmarshal(secondInit["data"], &b)
		if a.WorkspaceID <= 0 || a.WorkspaceID != b.WorkspaceID {
			t.Fatal("workspace initialization was not idempotent")
		}
		expect(t, call("GET", "/pod/bootstrap", newToken, nil), 200)
		created := view(t, call("POST", "/pod/projects", newToken, map[string]any{"name": "新空间第一个项目"}))
		if created.WorkspaceID != a.WorkspaceID {
			t.Fatal("new project owner mismatch")
		}
		expect(t, call("GET", path(created.ProjectID), token, nil), 404)
	})
	t.Run("revoked_membership_blocks_writes_and_reinitialization", func(t *testing.T) {
		q := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=? AND user_id=?", owner.Workspace.WorkspaceID, owner.UserID)
		if err := q.Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=? AND user_id=?", owner.Workspace.WorkspaceID, owner.UserID).Update("status", "0").Error; err != nil {
				t.Error(err)
			}
		}()
		expect(t, call("POST", "/pod/workspace", token, map[string]any{}), 403)
		expect(t, call("PUT", path(project.ProjectID), token, map[string]any{"revision": rev(project), "name": "denied"}), 403)
		expect(t, call("POST", "/pod/projects", token, map[string]any{"name": "denied"}), 403)
	})
	t.Run("bounded_pagination_and_unique_sort", func(t *testing.T) {
		now := types.Now()
		rows := make([]model.PODProject, 103)
		for i := range rows {
			rows[i] = model.PODProject{WorkspaceID: owner.Workspace.WorkspaceID, Name: "分页回归", GraphJSON: json.RawMessage(`{"schemaVersion":1,"nodes":[],"edges":[]}`), SchemaVersion: 1, Revision: 1, DelFlag: "0", CreateBy: "pod-stage2-test", UpdateBy: "pod-stage2-test", CreateTime: now, UpdateTime: now}
		}
		if err := repository.DB(ctx).Table("pod_project").Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			ids = append(ids, row.ProjectID)
		}
		first := call("GET", "/pod/projects?pageSize=9999", token, nil)
		expect(t, first, 200)
		var page []model.PODProjectView
		_ = json.Unmarshal(first["rows"], &page)
		if len(page) != 100 {
			t.Fatalf("page limit=%d", len(page))
		}
		seen := map[int64]bool{}
		for _, uri := range []string{"/pod/projects?pageSize=70&pageNum=1", "/pod/projects?pageSize=70&pageNum=2"} {
			payload := call(http.MethodGet, uri, token, nil)
			expect(t, payload, 200)
			_ = json.Unmarshal(payload["rows"], &page)
			for _, row := range page {
				if seen[row.ProjectID] {
					t.Fatal("duplicate across pages")
				}
				seen[row.ProjectID] = true
				if row.Graph != nil {
					t.Fatal("list unnecessarily returned graph")
				}
			}
		}
		expect(t, call("GET", "/pod/projects?deleted=anything", token, nil), 400)
	})
}
