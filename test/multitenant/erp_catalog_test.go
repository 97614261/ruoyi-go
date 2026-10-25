package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"testing"
	"time"
)

func TestERPCatalog(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (h != "localhost" && h != "127.0.0.1" && h != "::1") {
		t.Fatal("local dedicated DB only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository unavailable")
	}
	defer repository.Close()
	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	state, e := repository.InspectERPCatalog(ctx)
	c()
	if e != nil || state.InstalledTables != 5 || !state.Recorded {
		t.Fatal("explicit010 installation required", e)
	}
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 30 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	data := func(r map[string]any) map[string]any { s.ok(r); return r["data"].(map[string]any) }
	deny := func(r map[string]any) {
		s.t.Helper()
		if r["code"] == float64(200) {
			s.t.Fatal("invalid/unauthorized operation accepted")
		}
	}
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); fn(t) })
	}
	product := func(code string) model.CatalogWrite {
		return model.CatalogWrite{RequestKey: uuid.NewString(), ProductCode: code, ProductName: "页面独立商品", Variants: []model.CatalogVariant{{SkuCode: code + "-RED", Specification: "红色", Unit: "件", Status: "0"}, {SkuCode: code + "-BLUE", Specification: "蓝色", Unit: "件", Status: "0"}}}
	}
	id := func(m map[string]any, k string) int64 { return int64(m[k].(float64)) }
	create := func(token string, in model.CatalogWrite) int64 {
		v := data(s.req("POST", "/erp/product", token, in))
		return int64(v["productIds"].([]any)[0].(float64))
	}
	get := func(pid int64) model.CatalogWrite {
		raw, _ := json.Marshal(data(s.req("GET", fmt.Sprintf("/erp/product/%d", pid), a.owner, nil)))
		var in model.CatalogWrite
		if json.Unmarshal(raw, &in) != nil {
			t.Fatal("product decode")
		}
		in.RequestKey = uuid.NewString()
		return in
	}
	var pid int64
	var original model.CatalogWrite
	run("create_group_and_durable_replay_cross_tenant_isolation", func(t *testing.T) {
		cat := data(s.req("POST", "/erp/productCategory", a.owner, model.CatalogCategoryWrite{RequestKey: uuid.NewString(), CategoryCode: "CAT", CategoryName: "合成分类", Status: "0"}))
		cid := id(cat, "categoryId")
		original = product("CAT-HTTP")
		original.CategoryID = &cid
		pid = create(a.owner, original)
		r := data(s.req("POST", "/erp/product", a.owner, original))
		if r["existing"] != true || int64(r["productIds"].([]any)[0].(float64)) != pid {
			t.Fatal("replay")
		}
		bad := original
		bad.ProductName = "different"
		deny(s.req("POST", "/erp/product", a.owner, bad))
		deny(s.req("GET", fmt.Sprintf("/erp/product/%d", pid), b.owner, nil))
		create(b.owner, product("CAT-HTTP"))
		deny(s.req("GET", "/erp/product/list", p, nil))
		list := s.req("GET", "/erp/product/list?skuCode=CAT-HTTP-RED&pageSize=999", a.owner, nil)
		s.ok(list)
		if list["total"] != float64(1) {
			t.Fatal("SKU filter")
		}
	})
	run("same_key_parallel_replays_once_and_CAS_has_one_winner", func(t *testing.T) {
		for _, r := range parallelJSON(s, 4, func(i int) (string, string, string, any) { return "POST", "/erp/product", a.owner, original }) {
			if r.err != nil || r.code != 200 {
				t.Fatal(r)
			}
		}
		in := get(pid)
		wins := 0
		for _, r := range parallelJSON(s, 4, func(i int) (string, string, string, any) {
			v := in
			v.RequestKey = uuid.NewString()
			return "PUT", fmt.Sprintf("/erp/product/%d", pid), a.owner, v
		}) {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("CAS winners", wins)
		}
	})
	run("referenced_keys_and_legacy_SKU_bypass_are_blocked", func(t *testing.T) {
		s.ok(s.req("POST", "/erp/warehouse", a.owner, map[string]any{"warehouseCode": "CAT-W", "warehouseName": "商品保护验收仓", "status": "0"}))
		list := s.req("GET", "/erp/warehouse/list?warehouseCode=CAT-W", a.owner, nil)
		s.ok(list)
		wh := id(list["rows"].([]any)[0].(map[string]any), "warehouseId")
		in := get(pid)
		sku := in.Variants[0].SkuID
		doc := data(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wh, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: sku, Quantity: 1}}}))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", id(doc, "documentId")), a.owner, nil))
		for _, field := range []string{"skuCode", "unit", "specification"} {
			v := get(pid)
			switch field {
			case "skuCode":
				v.Variants[0].SkuCode = "CHANGED"
			case "unit":
				v.Variants[0].Unit = "箱"
			case "specification":
				v.Variants[0].Specification = "新规格"
			}
			deny(s.req("PUT", fmt.Sprintf("/erp/product/%d", pid), a.owner, v))
		}
		old := data(s.req("GET", fmt.Sprintf("/erp/sku/%d", sku), a.owner, nil))
		deny(s.req("PUT", "/erp/sku", a.owner, old))
		deny(s.req("DELETE", fmt.Sprintf("/erp/sku/%d", sku), a.owner, nil))
		v := get(pid)
		v.Variants = v.Variants[:1]
		deny(s.req("PUT", fmt.Sprintf("/erp/product/%d", pid), a.owner, v))
	})
	upload := func(path, token, key, name string, raw []byte) map[string]any {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		if key != "" {
			w.WriteField("requestKey", key)
		}
		part, e := w.CreateFormFile("file", name)
		if e != nil {
			t.Fatal(e)
		}
		part.Write(raw)
		w.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(ctx, "POST", s.base+path, &body)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", w.FormDataContentType())
		res, e := s.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out map[string]any
		if json.NewDecoder(res.Body).Decode(&out) != nil {
			t.Fatal("multipart response")
		}
		return out
	}
	run("real_image_validation_owned_reference_and_cross_tenant_media", func(t *testing.T) {
		var img bytes.Buffer
		if png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 2, 2))) != nil {
			t.Fatal("PNG")
		}
		key := data(upload("/erp/product/image", a.owner, "", "catalog.png", img.Bytes()))["storageKey"].(string)
		deny(upload("/erp/product/image", a.owner, "", "fake.png", []byte("<html>bad</html>")))
		v := get(pid)
		v.Images = []string{key}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/product/%d", pid), a.owner, v))
		foreign := product("FOREIGN-IMAGE")
		foreign.Images = []string{key}
		deny(s.req("POST", "/erp/product", b.owner, foreign))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", s.base+key, nil)
		req.Header.Set("Authorization", "Bearer "+b.owner)
		res, e := s.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if bytes.Equal(raw, img.Bytes()) {
			t.Fatal("foreign media read")
		}
	})
	book := func(duplicate bool) []byte {
		raw, e := service.CatalogTemplate()
		if e != nil {
			t.Fatal(e)
		}
		f, e := excelize.OpenReader(bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		for i := 0; i < 2; i++ {
			sku := fmt.Sprintf("IMP-S%d", i)
			if duplicate && i == 1 {
				sku = "CAT-HTTP-RED"
			}
			row := []string{fmt.Sprintf("IMP-P%d", i), "批量导入", "", "" + sku, "", "件", "0", fmt.Sprintf("000%d", i), "10", "20", "30", "40", ""}
			for j, v := range row {
				cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
				f.SetCellStr("Sheet1", cell, v)
			}
		}
		b, e := f.WriteToBuffer()
		if e != nil {
			t.Fatal(e)
		}
		return b.Bytes()
	}
	run("XLSX_atomic_failure_then_success_and_original_file_replay", func(t *testing.T) {
		deny(upload("/erp/product/import", a.owner, uuid.NewString(), "goods.xlsx", book(true)))
		r := s.req("GET", "/erp/product/list?skuCode=IMP-S0", a.owner, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("partial import")
		}
		raw, key := book(false), uuid.NewString()
		r = data(upload("/erp/product/import", a.owner, key, "goods.xlsx", raw))
		if r["rows"] != float64(2) {
			t.Fatal("import rows")
		}
		r = data(upload("/erp/product/import", a.owner, key, "goods.xlsx", raw))
		if r["existing"] != true {
			t.Fatal("import replay")
		}
	})
	run("export_download_preserves_text_barcode_and_exact_filter", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(ctx, "POST", s.base+"/erp/product/export?productCode=IMP-P0", nil)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+a.owner)
		res, e := s.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil || res.StatusCode != 200 || res.Header.Get("Content-Disposition") == "" || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("export response")
		}
		f, e := excelize.OpenReader(bytes.NewReader(raw))
		if e != nil {
			t.Fatal("export not XLSX", e)
		}
		defer f.Close()
		rows, e := f.GetRows(f.GetSheetName(0))
		if e != nil || len(rows) != 2 || len(rows[1]) < 12 || rows[1][0] != "IMP-P0" || rows[1][7] != "0000" || rows[1][8] != "10" {
			t.Fatal("export data", rows, e)
		}
		formula, e := f.GetCellFormula(f.GetSheetName(0), "H2")
		if e != nil || formula != "" {
			t.Fatal("barcode became formula")
		}
		// Validate actual template bytes/headers, independently of whether a
		// browser's download-event bridge can observe a FileSaver blob download.
		templateReq, e := http.NewRequestWithContext(ctx, "GET", s.base+"/erp/product/template", nil)
		if e != nil {
			t.Fatal(e)
		}
		templateReq.Header.Set("Authorization", "Bearer "+a.owner)
		templateRes, e := s.client.Do(templateReq)
		if e != nil {
			t.Fatal(e)
		}
		defer templateRes.Body.Close()
		templateRaw, e := io.ReadAll(templateRes.Body)
		if e != nil || templateRes.Header.Get("download-filename") == "" {
			t.Fatal("template response")
		}
		template, e := excelize.OpenReader(bytes.NewReader(templateRaw))
		if e != nil {
			t.Fatal("template is not XLSX", e)
		}
		defer template.Close()
		header, e := template.GetRows(template.GetSheetName(0))
		if e != nil || len(header) != 1 || len(header[0]) != 13 || header[0][0] != "商品编码" || header[0][7] != "条码" {
			t.Fatal("template header")
		}
	})
	run("category_CAS_inactive_selection_foreign_category_and_duplicate_barcode", func(t *testing.T) {
		cat := data(s.req("POST", "/erp/productCategory", a.owner, model.CatalogCategoryWrite{RequestKey: uuid.NewString(), CategoryCode: "DISABLED", CategoryName: "停用测试", Status: "0"}))
		cid := id(cat, "categoryId")
		edit := model.CatalogCategoryWrite{RequestKey: uuid.NewString(), Version: 1, CategoryCode: "DISABLED", CategoryName: "停用测试", Status: "1"}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/productCategory/%d", cid), a.owner, edit))
		edit.RequestKey = uuid.NewString()
		deny(s.req("PUT", fmt.Sprintf("/erp/productCategory/%d", cid), a.owner, edit))
		v := product("CAT-INACTIVE")
		v.CategoryID = &cid
		deny(s.req("POST", "/erp/product", a.owner, v))
		v = product("CAT-FOREIGN")
		v.CategoryID = original.CategoryID
		deny(s.req("POST", "/erp/product", b.owner, v))
		barcode := "0000"
		v = product("DUPLICATE-BARCODE")
		v.Variants[0].Barcode = &barcode
		deny(s.req("POST", "/erp/product", a.owner, v))
		r := s.req("GET", "/erp/product/list?productCode=DUPLICATE-BARCODE", a.owner, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("failed barcode left product")
		}
	})
	run("independent_permissions_old_session_revocation_and_actor_replay", func(t *testing.T) {
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "商品验收员", "roleKey": "catalog_check", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 9000, 9001, 9002, 9003}}))
		rr := s.req("GET", "/system/role/list?roleKey=catalog_check", a.owner, nil)
		s.ok(rr)
		role := rr["rows"].([]any)[0].(map[string]any)
		rid := id(role, "roleId")
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "cataloger", "nickName": "商品验收员", "password": operationsPassword, "status": "0", "roleIds": []int64{rid}}))
		token := s.loginAs(a.code, "cataloger", operationsPassword)
		defer s.req("POST", "/logout", token, nil)
		s.ok(s.req("GET", fmt.Sprintf("/erp/product/%d", pid), token, nil))
		deny(s.req("POST", "/erp/product", token, original))
		deny(upload("/erp/product/image", token, "", "no.png", []byte("bad")))
		deny(s.req("GET", "/erp/product/template", token, nil))
		role["menuIds"] = []int64{}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(s.req("GET", fmt.Sprintf("/erp/product/%d", pid), token, nil))
		deny(s.req("POST", "/erp/product", token, original))
	})
}
