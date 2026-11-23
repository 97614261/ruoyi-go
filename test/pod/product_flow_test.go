package podtest

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

func TestPODProductManagedFlow(t *testing.T) {
	addr := os.Getenv("POD_PRODUCT_TEST_MYSQL")
	if addr == "" {
		t.Skip("set POD_PRODUCT_TEST_MYSQL to a disposable MySQL instance")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("a loopback MySQL instance is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dsn := mysql.NewConfig()
	dsn.Net, dsn.Addr, dsn.User, dsn.DBName = "tcp", addr, "root", "pod_studio_product_flow_"+uuid.NewString()[:8]
	root := t.TempDir()
	cfg := config.Config{
		MySQL: config.MySQLConfig{DSN: dsn.FormatDSN(), ConnectTimeout: 3 * time.Second, SlowThreshold: time.Second},
		POD:   config.PODConfig{Enabled: true, InstanceID: "pod-product-flow", DatabaseName: dsn.DBName, RedisDB: 15, StorageRoot: filepath.Join(root, "assets")},
	}
	cfg.Redis.DB = 15
	cfg.JWT.Secret = strings.Repeat("isolated-test-only", 3)
	cfg.POD.Catalog.KeyringFile = filepath.Join(root, "keyring.json")
	key := func(value byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32)) }
	ring := fmt.Sprintf("{\"activeKeyId\":\"test\",\"keys\":{\"test\":%q},\"fingerprintKey\":%q}", key(1), key(2))
	if err := os.WriteFile(cfg.POD.Catalog.KeyringFile, []byte(ring), 0600); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("no-test-login", 5)
	if err := repository.InstallPODFoundation(ctx, &cfg, hash, []repository.PODSeedUser{{UserName: "flow_owner", NickName: "Owner", PasswordHash: hash}, {UserName: "flow_other", NickName: "Other", PasswordHash: hash}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnablePODManaged(ctx); err != nil {
		t.Fatal(err)
	}
	var users []model.SysUser
	if err := repository.DB(ctx).Where("user_name IN ?", []string{"flow_owner", "flow_other"}).Order("user_name DESC").Find(&users).Error; err != nil || len(users) != 2 {
		t.Fatalf("seed users missing: %v", err)
	}
	owner, other := &users[0], &users[1]
	admin := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1, UserName: "admin"}, Permissions: []string{"*:*:*"}}
	limits := service.PODExecutionLimits(config.PODExecutionConfig{Mode: "managed"})
	id := func(num int64) string { return fmt.Sprint(num) }
	catalog := func(op string, target int64, write model.PODCatalogWrite) *model.PODCatalogResult {
		t.Helper()
		write.RequestID, write.Reason = uuid.NewString(), "isolated product fixture"
		item, err := service.PODCatalogWrite(ctx, admin, op, target, write)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		return item
	}
	provider := catalog("provider.create", 0, model.PODCatalogWrite{Code: "product-fixture", Name: "Product fixture", AdapterKind: "openai-compatible-v1", Region: "global", Endpoint: "https://relay.example.com/v1"})
	cred := catalog("provider.credential", provider.ResourceID, model.PODCatalogWrite{Revision: id(provider.Revision), SourceType: "encrypted", Secret: "fixture-key-never-used-on-network"})
	catalog("provider.status", provider.ResourceID, model.PODCatalogWrite{Revision: id(cred.Revision), Status: "enabled"})
	visible := true
	entry := catalog("model.create", 0, model.PODCatalogWrite{Code: "image-fixture", Name: "Image fixture", Kind: "image", ProviderID: id(provider.ResourceID), Visible: &visible})
	version := catalog("model.version", entry.ResourceID, model.PODCatalogWrite{Revision: id(entry.Revision), OfficialModelID: "fixture-image-model", TimeoutSeconds: 20,
		Capabilities: &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, MaxReferenceImages: 1, Ratios: []string{"1:1"}, ImageResponse: "b64_json"},
		CostRule:     &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", PerImage: 100}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", PerImage: 10},
	})
	catalog("model.status", entry.ResourceID, model.PODCatalogWrite{Revision: id(version.Revision), Status: "enabled"})
	apply := func(op string, write model.PODCreatorWrite) {
		t.Helper()
		write.RequestID, write.Reason = uuid.NewString(), "isolated product fixture"
		if _, err := service.PODCreatorWrite(ctx, admin, op, owner.UserID, write, limits); err != nil {
			t.Fatal(err)
		}
	}
	grants := []string{id(entry.ResourceID)}
	apply("creator.models", model.PODCreatorWrite{Revision: "1", ModelIDs: &grants})
	apply("points.adjust", model.PODCreatorWrite{AccountRevision: "1", Kind: "grant", Amount: "100"})
	var pngBytes bytes.Buffer
	pixels := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	if err := png.Encode(&pngBytes, pixels); err != nil {
		t.Fatal(err)
	}
	assets := service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{})
	asset, err := assets.Create(ctx, owner, "reference.png", "image/png", bytes.NewReader(pngBytes.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	products := service.PODProductService{Execution: limits, Assets: assets}
	input := model.PODProductCreate{PODProductFields: model.PODProductFields{ProductName: "Mug", SKU: "M-01"}, ReferenceAssetID: id(asset.AssetID), ModelID: id(entry.ResourceID), Ratio: "1:1", RequestID: uuid.NewString()}
	product, err := products.Create(ctx, owner, input)
	if err != nil {
		t.Fatal(err)
	}
	pid := product.Project.ProjectID
	if _, err := products.Get(ctx, other, pid); errs.As(err) == nil || errs.As(err).Code != 404 {
		t.Fatalf("unexpected reply to foreign account: %v", err)
	}
	t.Run("layout PNG belongs to the product and the same upload is reusable", func(t *testing.T) {
		save := func(user *model.SysUser) (*model.PODAssetView, error) {
			return assets.CreateProductPage(ctx, user, pid, "M-01-卖点图.png", "image/png", bytes.NewReader(pngBytes.Bytes()))
		}
		if _, err := save(other); errs.As(err) == nil || errs.As(err).Code != 404 {
			t.Fatalf("foreign account saved to product: %v", err)
		}
		first, err := save(owner)
		if err != nil || first.Source != model.PODAssetProductPage || first.AssetID == asset.AssetID {
			t.Fatalf("save layout: %+v %v", first, err)
		}
		repeat, err := save(owner)
		if err != nil || repeat.AssetID != first.AssetID {
			t.Fatalf("repeat layout: %+v %v", repeat, err)
		}
		rows, total, err := service.ListPODAssetPage(ctx, owner, pid, false, page.Query{PageNum: 1, PageSize: 10})
		if err != nil || total != 1 || len(rows) != 1 || rows[0].AssetID != first.AssetID {
			t.Fatalf("product layouts: %+v %d %v", rows, total, err)
		}
		if _, _, err := service.ListPODAssetPage(ctx, other, pid, false, page.Query{}); errs.As(err) == nil || errs.As(err).Code != 404 {
			t.Fatalf("foreign account listed product layouts: %v", err)
		}
		rows, total, err = service.ListPODAssetPage(ctx, owner, 0, false, page.Query{PageNum: 1, PageSize: 10})
		if err != nil || total != 2 || len(rows) != 2 || rows[0].AssetID != first.AssetID {
			t.Fatalf("personal asset library: %+v %d %v", rows, total, err)
		}
		file, _, _, err := assets.Open(ctx, owner, first.AssetID, "original")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		stored, err := io.ReadAll(file)
		if err != nil || !bytes.Equal(stored, pngBytes.Bytes()) {
			t.Fatalf("stored PNG differs: %v", err)
		}
	})
	runs := service.PODRunService{Limits: limits}
	submit := func(mode, node string, points int64) *model.PODRunView {
		t.Helper()
		run := model.PODRunCreate{ProjectID: id(pid), Revision: "1", RequestID: uuid.NewString(), Mode: mode, NodeID: node, ExecutionMode: "managed"}
		quote, err := runs.Quote(ctx, owner, model.PODQuoteRequest{Kind: "run", Run: &run})
		if err != nil || quote.Points != points || len(quote.Lines) != int(points/10) {
			t.Fatalf("unexpected product quote: %+v %v", quote, err)
		}
		run.QuoteID = quote.QuoteID
		view, err := runs.Create(ctx, owner, run)
		if err != nil {
			t.Fatal(err)
		}
		return view
	}
	posts := 0
	generated := map[int][]byte{}
	supplier := &http.Client{Transport: podQwenTransport(func(req *http.Request) (*http.Response, error) {
		posts++
		if req.Method != http.MethodPost || req.URL.Path != "/v1/images/edits" || !strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("reference image was not sent as an edit: %s %s", req.Method, req.URL.Path)
		}
		var imageBytes bytes.Buffer
		pixels.SetNRGBA(0, 0, color.NRGBA{R: uint8(posts), A: 255})
		if err := png.Encode(&imageBytes, pixels); err != nil {
			t.Fatal(err)
		}
		generated[posts] = imageBytes.Bytes()
		response := "{\"data\":[{\"b64_json\":\"" + base64.StdEncoding.EncodeToString(imageBytes.Bytes()) + "\"}]}"
		status := 200
		if posts == 2 {
			status, response = 400, "{\"error\":{\"message\":\"fixture failure\"}}"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	noNetwork := &http.Client{Transport: podQwenTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("inline fixture unexpectedly requested an external download")
		return nil, io.ErrUnexpectedEOF
	})}
	executor := &service.PODQwenExecutor{Assets: assets, Client: supplier, Images: noNetwork, CatalogImages: noNetwork, BudgetFen: 100}
	finish := func(runID int64, want model.PODRunState) {
		t.Helper()
		for i := 0; i < 7; i++ {
			if _, err := service.ExecutePODQwenStep(ctx, runID, limits, executor); err != nil {
				t.Fatal(err)
			}
			view, err := runs.Get(ctx, owner, runID)
			if err != nil {
				t.Fatal(err)
			}
			if view.State == want {
				return
			}
		}
		t.Fatalf("run %d did not reach %s", runID, want)
	}
	first := submit("workflow", "", 20)
	finish(first.RunID, model.PODRunFailed)
	view, err := products.Get(ctx, owner, pid)
	if err != nil || view.Slots[0].Result == nil || view.Slots[1].Result != nil || posts != 2 {
		t.Fatalf("partial result did not persist: %+v %v", view, err)
	}
	mainID := view.Slots[0].Result.Output.AssetIDs[0]
	main := model.PODProductFinalChange{RunID: id(first.RunID), AssetID: mainID}
	if _, err := products.SetFinal(ctx, other, pid, model.PODProductMain, main); err == nil {
		t.Fatal("foreign account could change the final")
	}
	if _, err := products.SetFinal(ctx, owner, pid, model.PODProductScene, main); err == nil {
		t.Fatal("main image was accepted as the scene")
	}
	if _, err := products.SetFinal(ctx, owner, pid, model.PODProductMain, main); err != nil {
		t.Fatal(err)
	}
	retry := submit("branch", model.PODProductScene, 10)
	finish(retry.RunID, model.PODRunSucceeded)
	view, err = products.Get(ctx, owner, pid)
	if err != nil || view.Slots[0].Result.Output.AssetIDs[0] != mainID || view.Slots[1].Result.RunID != retry.RunID || posts != 3 {
		t.Fatalf("single-branch result was not merged safely: %+v %v", view, err)
	}
	scene := model.PODProductFinalChange{RunID: id(retry.RunID), AssetID: view.Slots[1].Result.Output.AssetIDs[0]}
	if _, err := products.ZIP(ctx, owner, pid, true); err == nil {
		t.Fatal("final-only ZIP silently included an unselected image")
	}
	if _, err := products.SetFinal(ctx, owner, pid, model.PODProductScene, scene); err != nil {
		t.Fatal(err)
	}
	newScene := submit("branch", model.PODProductScene, 10)
	finish(newScene.RunID, model.PODRunSucceeded)
	view, err = products.Get(ctx, owner, pid)
	if err != nil || view.Slots[1].Final.RunID != retry.RunID || view.Slots[1].Result.RunID != newScene.RunID {
		t.Fatalf("redo changed the pinned result: %+v %v", view, err)
	}
	history, total, err := products.History(ctx, owner, pid, model.PODProductScene, page.Query{PageNum: 1, PageSize: 1})
	if err != nil || total != 2 || len(history) != 1 || history[0].RunID != newScene.RunID {
		t.Fatalf("unexpected candidate history: %v %d %v", history, total, err)
	}
	replacement := model.PODProductFinalChange{RunID: id(newScene.RunID), AssetID: view.Slots[1].Result.Output.AssetIDs[0]}
	if _, err := products.SetFinal(ctx, owner, pid, model.PODProductScene, replacement); err == nil {
		t.Fatal("stale final selection overwrote a saved choice")
	}
	finished, err := products.ZIP(ctx, owner, pid, true)
	if err != nil {
		t.Fatal(err)
	}
	file := finished.File
	defer os.Remove(file.Name())
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(file, stat.Size())
	if err != nil || finished.Name != "M-01-final.zip" || len(archive.File) != 2 || archive.File[0].Name != "M-01-main-white.png" || archive.File[1].Name != "M-01-scene.png" || posts != 4 {
		t.Fatalf("unexpected ZIP or provider submission: %v", err)
	}
	for i, index := range []int{1, 3} {
		imageFile, err := archive.File[i].Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(imageFile)
		imageFile.Close()
		if err != nil || !bytes.Equal(data, generated[index]) {
			t.Fatalf("archive used a different candidate for %s: %v", archive.File[i].Name, err)
		}
	}
	account, err := repository.PODPoints(ctx, owner.UserID)
	if err != nil || account.Spent != 30 || account.Held != 0 {
		t.Fatalf("unexpected points after three successful images: %+v %v", account, err)
	}

	selected := []string{model.PODProductDetail, model.PODProductSide}
	choice := input
	choice.RequestID, choice.Outputs = uuid.NewString(), &selected
	expanded, err := products.Create(ctx, owner, choice)
	if err != nil || len(expanded.Slots) != 2 || expanded.Slots[0].NodeID != model.PODProductSide || expanded.Slots[1].NodeID != model.PODProductDetail || expanded.Pack.PackID != product.Pack.PackID {
		t.Fatalf("product did not persist the selected slots and shared pack: %+v %v", expanded, err)
	}
	selected[0], selected[1] = selected[1], selected[0]
	replayed, err := products.Create(ctx, owner, choice)
	if err != nil || replayed.Project.ProjectID != expanded.Project.ProjectID {
		t.Fatalf("reordered selections must retry the same project: %+v %v", replayed, err)
	}
	if _, _, err := products.History(ctx, owner, expanded.Project.ProjectID, model.PODProductMain, page.Query{}); err == nil {
		t.Fatal("unselected output was available in the product history")
	}
	expandedRequest := model.PODRunCreate{ProjectID: id(expanded.Project.ProjectID), Revision: "1", RequestID: uuid.NewString(), Mode: "workflow", ExecutionMode: "managed"}
	expandedQuote, err := runs.Quote(ctx, owner, model.PODQuoteRequest{Kind: "run", Run: &expandedRequest})
	if err != nil || expandedQuote.Points != 20 || len(expandedQuote.Lines) != 2 {
		t.Fatalf("unselected outputs affected the quote: %+v %v", expandedQuote, err)
	}
	for _, quoteLine := range expandedQuote.Lines {
		if quoteLine.NodeID != model.PODProductSide && quoteLine.NodeID != model.PODProductDetail {
			t.Fatalf("unexpected charged output: %+v", quoteLine)
		}
	}
	for _, kind := range []string{model.PODProductPrint, model.PODProductMockup} {
		local := input
		local.RequestID, local.WorkKind, local.ModelID, local.Ratio = uuid.NewString(), kind, "", ""
		project, err := products.Create(ctx, owner, local)
		if err != nil || project.WorkKind != kind || project.Pack.PackID != product.Pack.PackID || len(project.Slots) != 0 || project.QuickRun {
			t.Fatalf("local workflow did not reuse the pack: %+v %v", project, err)
		}
		job := model.PODRunCreate{ProjectID: id(project.Project.ProjectID), Revision: "1", RequestID: uuid.NewString(), Mode: "workflow", ExecutionMode: "managed"}
		quote, err := runs.Quote(ctx, owner, model.PODQuoteRequest{Kind: "run", Run: &job})
		if err != nil || quote.Points != 0 || len(quote.Lines) != 0 {
			t.Fatalf("local workflow unexpectedly requires provider points: %+v %v", quote, err)
		}
		job.QuoteID = quote.QuoteID
		view, err := runs.Create(ctx, owner, job)
		if err != nil {
			t.Fatal(err)
		}
		finish(view.RunID, model.PODRunSucceeded)
		view, err = runs.Get(ctx, owner, view.RunID)
		if err != nil || len(view.Nodes) != 3 || view.Nodes[2].NodeType != "export" || len(view.Nodes[2].Output.AssetIDs) != 1 || posts != 4 {
			t.Fatalf("local workflow failed to export without a provider call: %+v %v", view, err)
		}
	}
}
