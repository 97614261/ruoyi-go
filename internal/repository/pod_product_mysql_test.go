package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func TestPODProductMySQL(t *testing.T) {
	addr := os.Getenv("POD_PRODUCT_TEST_MYSQL")
	if addr == "" {
		t.Skip("set POD_PRODUCT_TEST_MYSQL to a disposable MySQL 5.7 instance")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("test database must use an IPv4 loopback address")
	}
	dsn := mysql.NewConfig()
	dsn.Net, dsn.Addr, dsn.User = "tcp", addr, "root"
	dsn.DBName = "pod_studio_product_test_" + uuid.NewString()[:8]
	cfg := config.Config{}
	cfg.POD = config.PODConfig{Enabled: true, InstanceID: "pod-product-test", DatabaseName: dsn.DBName, RedisDB: 15, StorageRoot: t.TempDir()}
	cfg.Redis.DB = 15
	cfg.JWT.Secret = strings.Repeat("test-only-", 5)
	cfg.MySQL = config.MySQLConfig{DSN: dsn.FormatDSN(), MaxOpenConns: 4, MaxIdleConns: 4, SlowThreshold: time.Second, ConnectTimeout: 3 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	hash := strings.Repeat("test-no-login", 5)
	if err := InstallPODFoundation(ctx, &cfg, hash, []PODSeedUser{{"product_test_a", "A", hash}, {"product_test_b", "B", hash}}); err != nil {
		t.Fatal(err)
	}
	if err := MigratePODAssets(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer Close()
	if err := VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
		t.Fatal(err)
	}
	var users []model.SysUser
	if err := DB(ctx).Where("user_name IN ?", []string{"product_test_a", "product_test_b"}).Order("user_id ASC").Find(&users).Error; err != nil || len(users) != 2 {
		t.Fatalf("test users unavailable: %v", err)
	}
	spaces := [2]*model.PODWorkspace{}
	for i := range users {
		spaces[i], err = PODPersonalWorkspace(ctx, users[i].UserID)
		if err != nil || spaces[i] == nil {
			t.Fatalf("test workspace unavailable: %v", err)
		}
	}
	asset := model.PODAsset{WorkspaceID: spaces[0].WorkspaceID, Source: "upload", OriginalName: "test.png", MediaType: "image/png", ByteSize: 5, Width: 64, Height: 64, DelFlag: "0", Revision: 1, CreateTime: types.Now(), UpdateTime: types.Now()}
	if err := DB(ctx).Table("pod_asset").Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	create := func(workspace int64, user *model.SysUser, request, fingerprint string) (int64, error) {
		job := &model.PODProductJob{RequestID: request, RequestHash: request}
		pack := &model.PODProductPack{WorkspaceID: workspace, Fingerprint: fingerprint, ReferenceAssetID: asset.AssetID, CreateTime: types.Now(), PODProductFields: model.PODProductFields{ProductName: "My mug", SKU: "MUG-42"}}
		project := &model.PODProject{Name: "My mug", GraphJSON: json.RawMessage("{\"schemaVersion\":1,\"nodes\":[],\"edges\":[]}"), SchemaVersion: 1, Revision: 1, DelFlag: "0", CreateTime: types.Now(), UpdateTime: types.Now()}
		returnJob := CreatePODProductJob(ctx, user, job, pack, project)
		return job.ProjectID, returnJob
	}
	const key = "test-product-creation-id"
	const fingerprint = "product-fixture-fingerprint"
	id, err := create(spaces[0].WorkspaceID, &users[0], key, fingerprint)
	if err != nil || id == 0 {
		t.Fatalf("create project: id=%d %v", id, err)
	}
	duplicate, err := create(spaces[0].WorkspaceID, &users[0], key, fingerprint)
	if err != nil || duplicate != id {
		t.Fatalf("idempotency: id=%d expected=%d err=%v", duplicate, id, err)
	}
	if _, err = create(spaces[0].WorkspaceID, &users[0], "test-product-second-id", fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err = create(spaces[1].WorkspaceID, &users[1], key, fingerprint); !errors.Is(err, ErrPODAssetGraph) {
		t.Fatalf("foreign reference was accepted: %v", err)
	}
	pg := page.Query{PageNum: 1, PageSize: 12}
	packs, packCount, err := ListPODProductPacks(ctx, spaces[0].WorkspaceID, pg)
	if err != nil || packCount != 1 || len(packs) != 1 || packs[0].SKU != "MUG-42" {
		t.Fatalf("unexpected reusable packs: %+v count=%d %v", packs, packCount, err)
	}
	_, jobCount, err := ListPODProductJobs(ctx, spaces[0].WorkspaceID, pg)
	if err != nil || jobCount != 2 {
		t.Fatalf("unexpected job count: %d %v", jobCount, err)
	}
	_, jobCount, err = ListPODProductJobs(ctx, spaces[1].WorkspaceID, pg)
	if err != nil || jobCount != 0 {
		t.Fatalf("invalid create was not fully rolled back: %d %v", jobCount, err)
	}
	if _, _, _, _, err := SelectPODProductJob(ctx, spaces[1].WorkspaceID, id); !errors.Is(err, ErrPODProject) {
		t.Fatalf("job in another workspace was visible: %v", err)
	}
	addRun := func(mode, target string, state model.PODRunState, nodes ...model.PODNodeRun) int64 {
		t.Helper()
		now := podNow()
		r := model.PODRun{WorkspaceID: spaces[0].WorkspaceID, UserID: users[0].UserID, ProjectID: id, ProjectRevision: 1, ProjectName: "My mug", RequestID: uuid.NewString(), Mode: mode, TargetNodeID: target, ExecutionMode: "managed", GraphJSON: json.RawMessage("{}"), BaseOutputsJSON: json.RawMessage("{}"), State: state, CreateTime: now, UpdateTime: now}
		if err := DB(ctx).Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		for _, n := range nodes {
			n.RunID, n.WorkspaceID, n.UserID = r.RunID, r.WorkspaceID, r.UserID
			n.NodeType, n.InputJSON, n.CreateTime, n.UpdateTime = "imageGenerate", json.RawMessage("{}"), now, now
			if err := DB(ctx).Create(&n).Error; err != nil {
				t.Fatal(err)
			}
		}
		return r.RunID
	}
	main := model.PODNodeRun{NodeID: model.PODProductMain, State: "succeeded", OutputJSON: json.RawMessage("{\"assetIds\":[\"51\"]}")}
	scene := model.PODNodeRun{NodeID: model.PODProductScene, State: "failed", OutputJSON: json.RawMessage("{}")}
	original := addRun("workflow", "", model.PODRunFailed, main, scene)
	scene.State, scene.ErrorCode = "awaitingReconciliation", "archivePending"
	running := addRun("branch", model.PODProductScene, model.PODRunAwaitingReconciliation, scene)
	_, _, slots, _, err := SelectPODProductJob(ctx, spaces[0].WorkspaceID, id)
	if err != nil || slots[0].Result == nil || slots[0].Result.RunID != original || slots[0].Result.Output.AssetIDs[0] != "51" ||
		slots[1].Latest == nil || slots[1].Latest.RunID != running || slots[1].Latest.ErrorCode != "archivePending" || slots[1].Result != nil {
		t.Fatalf("incorrect independent results: %+v %v", slots, err)
	}
	overlap := func(nodeID string) error {
		return checkPODProductRunOverlap(DB(ctx), spaces[0].WorkspaceID, id, []model.PODNode{{ID: nodeID, Type: "imageGenerate"}})
	}
	if !errors.Is(overlap(model.PODProductScene), ErrPODProductBusy) || overlap(model.PODProductMain) != nil {
		t.Fatal("an in-flight output must block only the overlapping branch")
	}
}
