package portaltest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/redisx"
)

type cmsClient struct {
	t     *testing.T
	db    *sql.DB
	cache *redis.Client
	http  *http.Client
	admin string
}

func cmsSetup(t *testing.T) *cmsClient {
	t.Helper()
	if os.Getenv("PORTAL_INTEGRATION") != "1" {
		t.Skip("explicit PORTAL_INTEGRATION=1 required")
	}
	cfg, e := config.Load("../../configs/portal.local.yml")
	if e != nil {
		t.Fatal("private portal config unavailable")
	}
	if !strings.Contains(cfg.MySQL.DSN, "/ruoyi_go_portal?") || cfg.Redis.DB != 14 || cfg.Server.Port != 8085 {
		t.Fatal("isolated local portal required")
	}
	db, e := sql.Open("mysql", cfg.MySQL.DSN)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: 14})
	t.Cleanup(func() { cache.Close() })
	account := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}
	body, e := os.ReadFile("../../logs/portal-accounts.local.json")
	if e != nil {
		t.Fatal("local portal account unavailable")
	}
	if e = json.Unmarshal(body, &account); e != nil {
		t.Fatal(e)
	}
	c := &cmsClient{t: t, db: db, cache: cache, http: &http.Client{Timeout: 15 * time.Second}}
	c.admin = c.login(account.Username, account.Password)
	t.Cleanup(func() { c.call("POST", "/logout", c.admin, nil) })
	return c
}
func (c *cmsClient) call(method, path, token string, body any) map[string]any {
	c.t.Helper()
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			c.t.Fatal(e)
		}
		data = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, "http://127.0.0.1:8085"+path, data)
	if e != nil {
		c.t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, e := c.http.Do(req)
	if e != nil {
		c.t.Fatalf("%s request failed", path)
	}
	defer response.Body.Close()
	var out map[string]any
	if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
		c.t.Fatalf("%s returned non-JSON", path)
	}
	return out
}
func (c *cmsClient) ok(method, path string, body any) map[string]any {
	c.t.Helper()
	out := c.call(method, path, c.admin, body)
	cmsCode(c.t, out, 200)
	return out
}
func cmsCode(t *testing.T, out map[string]any, want int) {
	t.Helper()
	if out["code"] != float64(want) {
		t.Fatalf("unexpected business code %v, want %d (msg=%v)", out["code"], want, out["msg"])
	}
}
func cmsData(out map[string]any) map[string]any  { return out["data"].(map[string]any) }
func cmsID(out map[string]any, key string) int64 { return int64(out[key].(float64)) }
func (c *cmsClient) login(name, password string) string {
	c.t.Helper()
	cap := c.call("GET", "/captchaImage", "", nil)
	if cap["captchaEnabled"] == false {
		out := c.call("POST", "/login", "", map[string]any{"username": name, "password": password})
		cmsCode(c.t, out, 200)
		token, ok := out["token"].(string)
		if !ok {
			c.t.Fatal("login token missing")
		}
		return token
	}
	uuid := cap["uuid"].(string)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	answer, e := c.cache.Get(ctx, redisx.CaptchaKey(uuid)).Result()
	if e != nil {
		c.t.Fatal("isolated test captcha unavailable")
	}
	out := c.call("POST", "/login", "", map[string]any{"username": name, "password": password, "uuid": uuid, "code": answer})
	cmsCode(c.t, out, 200)
	token, ok := out["token"].(string)
	if !ok {
		c.t.Fatal("login token missing")
	}
	return token
}
func (c *cmsClient) upload(name string, content []byte) map[string]any {
	c.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, e := form.CreateFormFile("file", name)
	if e != nil {
		c.t.Fatal(e)
	}
	file.Write(content)
	form.Close()
	req, e := http.NewRequest("POST", "http://127.0.0.1:8085/cms/asset/upload", &body)
	if e != nil {
		c.t.Fatal(e)
	}
	req.Header.Set("Authorization", "Bearer "+c.admin)
	req.Header.Set("Content-Type", form.FormDataContentType())
	response, e := c.http.Do(req)
	if e != nil {
		c.t.Fatal("upload request failed")
	}
	defer response.Body.Close()
	out := map[string]any{}
	if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
		c.t.Fatal("upload not JSON")
	}
	return out
}
func (c *cmsClient) image(id int64, token string) (int, string, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:8085/cms/asset/%d/file", id), nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, e := c.http.Do(req)
	if e != nil {
		c.t.Fatal("private image request failed")
	}
	defer response.Body.Close()
	body, e := io.ReadAll(response.Body)
	if e != nil {
		c.t.Fatal(e)
	}
	return response.StatusCode, response.Header.Get("Content-Type"), body
}

func TestCMSChannelAssetFlow(t *testing.T) {
	c := cmsSetup(t)
	prefix := "cms-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	imageData := image.NewRGBA(image.Rect(0, 0, 40, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 40; x++ {
			imageData.Set(x, y, color.RGBA{30, 100, 180, 255})
		}
	}
	var pngData bytes.Buffer
	png.Encode(&pngData, imageData)
	cmsCode(t, c.upload("bad.svg", []byte(`<svg onload="alert(1)"></svg>`)), 500)
	cmsCode(t, c.upload("fake.png", []byte("not an image")), 500)
	uploaded := c.upload(prefix+".png", pngData.Bytes())
	cmsCode(t, uploaded, 200)
	asset := cmsData(uploaded)
	assetID := cmsID(asset, "assetId")
	assetPath := fmt.Sprintf("/cms/asset/%d", assetID)
	if _, exists := asset["storageKey"]; exists {
		t.Fatal("private storage key exposed")
	}
	if _, contentType, body := c.image(assetID, c.admin); !strings.HasPrefix(contentType, "image/png") || len(body) == 0 {
		t.Fatal("authorized preview unavailable")
	}
	_, contentType, body := c.image(assetID, "")
	if strings.HasPrefix(contentType, "image/") {
		t.Fatal("private image leaked anonymously")
	}
	denied := map[string]any{}
	json.Unmarshal(body, &denied)
	cmsCode(t, denied, 401)
	channelIDs := []int64{}
	t.Cleanup(func() {
		for i := len(channelIDs) - 1; i >= 0; i-- {
			path := fmt.Sprintf("/cms/channel/%d", channelIDs[i])
			out := c.call("GET", path, c.admin, nil)
			if out["code"] == float64(200) {
				row := cmsData(out)
				row["coverAssetId"] = 0
				c.call("PUT", "/cms/channel", c.admin, row)
				current := c.call("GET", path, c.admin, nil)
				if current["code"] == float64(200) {
					c.call("DELETE", path, c.admin, map[string]any{"version": cmsData(current)["version"]})
				}
			}
		}
		out := c.call("GET", assetPath, c.admin, nil)
		if out["code"] == float64(200) {
			c.call("DELETE", assetPath, c.admin, map[string]any{"version": cmsData(out)["version"]})
		}
	})
	rootBody := map[string]any{"name": prefix, "slug": prefix, "parentId": 0, "kind": "ARTICLE", "status": "0", "orderNum": 0, "coverAssetId": assetID}
	rootID := cmsID(cmsData(c.ok("POST", "/cms/channel", rootBody)), "channelId")
	channelIDs = append(channelIDs, rootID)
	childBody := map[string]any{"name": prefix + "-child", "slug": prefix + "-child", "parentId": rootID, "kind": "ARTICLE", "status": "0", "orderNum": 0, "coverAssetId": 0}
	childID := cmsID(cmsData(c.ok("POST", "/cms/channel", childBody)), "channelId")
	channelIDs = append(channelIDs, childID)
	rootPath := fmt.Sprintf("/cms/channel/%d", rootID)
	root := cmsData(c.ok("GET", rootPath, nil))
	root["name"] = prefix + "-updated"
	c.ok("PUT", "/cms/channel", root)
	cmsCode(t, c.call("PUT", "/cms/channel", c.admin, root), 500)
	root = cmsData(c.ok("GET", rootPath, nil))
	root["parentId"] = childID
	cmsCode(t, c.call("PUT", "/cms/channel", c.admin, root), 500)
	root["parentId"] = 0
	root["status"] = "1"
	cmsCode(t, c.call("PUT", "/cms/channel", c.admin, root), 500)
	root["status"] = "0"
	root["kind"] = "PRODUCT"
	cmsCode(t, c.call("PUT", "/cms/channel", c.admin, root), 500)
	root["kind"] = "ARTICLE"
	cmsCode(t, c.call("DELETE", rootPath, c.admin, map[string]any{"version": root["version"]}), 500)
	cmsCode(t, c.call("POST", "/cms/channel", c.admin, rootBody), 500)
	refs := c.ok("GET", assetPath+"/references", nil)
	if refs["total"] != float64(1) {
		t.Fatal("cover reference missing")
	}
	cmsCode(t, c.call("DELETE", assetPath, c.admin, map[string]any{"version": 1}), 500)
	updatedAsset := map[string]any{"name": prefix + "-renamed", "altText": "测试图片", "remark": "integration only", "version": 1, "storageKey": "outside.png", "isDeleted": 1}
	c.ok("PUT", assetPath, updatedAsset)
	cmsCode(t, c.call("PUT", assetPath, c.admin, updatedAsset), 500)
	detail := cmsData(c.ok("GET", assetPath, nil))
	if detail["isDeleted"] != float64(0) || detail["version"] != float64(2) {
		t.Fatal("mass assignment or asset CAS failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var key string
	if e := c.db.QueryRowContext(ctx, "SELECT storage_key FROM cms_asset WHERE asset_id=?", assetID).Scan(&key); e != nil {
		t.Fatal(e)
	}
	if key == "outside.png" {
		t.Fatal("private file key was writable")
	}
	response, e := c.http.Get("http://127.0.0.1:8085/profile/" + key)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("private image reachable through public profile")
	}
	// Same-version parallel writes may commit exactly once.
	var wg sync.WaitGroup
	codes := make(chan float64, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			snapshot := map[string]any{}
			for k, v := range root {
				snapshot[k] = v
			}
			snapshot["name"] = prefix + fmt.Sprint(i)
			encoded, _ := json.Marshal(snapshot)
			req, _ := http.NewRequest("PUT", "http://127.0.0.1:8085/cms/channel", bytes.NewReader(encoded))
			req.Header.Set("Authorization", "Bearer "+c.admin)
			req.Header.Set("Content-Type", "application/json")
			response, err := c.http.Do(req)
			if err != nil {
				codes <- -1
				return
			}
			defer response.Body.Close()
			out := map[string]any{}
			if err = json.NewDecoder(response.Body).Decode(&out); err != nil {
				codes <- -2
				return
			}
			code, ok := out["code"].(float64)
			if !ok {
				codes <- -3
				return
			}
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	wins := 0
	for code := range codes {
		if code != 200 && code != 500 {
			t.Fatalf("unexpected concurrent response %v", code)
		}
		if code == 200 {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("CAS committed %d concurrent writes", wins)
	}
	// A limited new account can read but cannot upload/edit/delete; its existing token loses preview after revocation.
	roleKey := "cms_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	username := "cv" + strconv.FormatInt(time.Now().UnixNano(), 36)
	password := "Cms_test_7426!"
	role := map[string]any{"roleName": roleKey, "roleKey": roleKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int{6000, 6001, 6002, 6010, 6011}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	var roleID int64
	if e = c.db.QueryRowContext(ctx, "SELECT role_id FROM sys_role WHERE role_key=?", roleKey).Scan(&roleID); e != nil {
		t.Fatal(e)
	}
	var userID int64
	t.Cleanup(func() {
		if userID > 0 {
			c.call("PUT", "/system/user", c.admin, map[string]any{"userId": userID, "userName": username, "nickName": "CMS测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["roleId"] = roleID
		role["menuIds"] = []int{}
		role["status"] = "1"
		c.call("PUT", "/system/role", c.admin, role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": username, "nickName": "CMS只读测试", "password": password, "deptId": 103, "status": "0", "roleIds": []int64{roleID}, "postIds": []int64{}})
	if e = c.db.QueryRowContext(ctx, "SELECT user_id FROM sys_user WHERE user_name=?", username).Scan(&userID); e != nil {
		t.Fatal(e)
	}
	viewer := c.login(username, password)
	defer c.call("POST", "/logout", viewer, nil)
	cmsCode(t, c.call("GET", "/cms/asset/list", viewer, nil), 200)
	cmsCode(t, c.call("POST", "/cms/channel", viewer, rootBody), 403)
	cmsCode(t, c.call("DELETE", assetPath, viewer, map[string]any{"version": 2}), 403)
	if _, kind, _ := c.image(assetID, viewer); !strings.HasPrefix(kind, "image/") {
		t.Fatal("explicit read permission did not permit preview")
	}
	role["roleId"] = roleID
	role["menuIds"] = []int{}
	c.ok("PUT", "/system/role", role)
	_, kind, body := c.image(assetID, viewer)
	if strings.HasPrefix(kind, "image/") {
		t.Fatal("revoked token still reads private image")
	}
	denied = map[string]any{}
	json.Unmarshal(body, &denied)
	cmsCode(t, denied, 403)
	root = cmsData(c.ok("GET", rootPath, nil))
	root["coverAssetId"] = 0
	c.ok("PUT", "/cms/channel", root)
	if c.ok("GET", assetPath+"/references", nil)["total"] != float64(0) {
		t.Fatal("cover unreference failed")
	}
	c.ok("DELETE", assetPath, map[string]any{"version": 2})
	_, kind, _ = c.image(assetID, c.admin)
	if strings.HasPrefix(kind, "image/") {
		t.Fatal("deleted image remains readable")
	}
}
