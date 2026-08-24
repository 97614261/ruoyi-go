package apitest

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempFile 在 profile 目录下的某个子目录里放一个文件，返回绝对路径。
func writeTempFile(t *testing.T, subDir, name, content string) string {
	t.Helper()

	dir := filepath.Join(appConfig.Upload.Path, subDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("创建目录 %s 失败：%v", dir, err)
	}
	absPath := filepath.Join(dir, name)
	if err := os.WriteFile(absPath, []byte(content), 0o640); err != nil {
		t.Fatalf("写测试文件 %s 失败：%v", absPath, err)
	}

	t.Cleanup(func() { _ = os.Remove(absPath) })
	return absPath
}

// TestCommonDownload 通用下载：正常下载、下完即删。
func TestCommonDownload(t *testing.T) {
	const content = "zz_test 通用下载内容"
	absPath := writeTempFile(t, "download", "abc123_报表.txt", content)

	r := request(http.MethodGet,
		"/common/download?fileName="+url.QueryEscape("abc123_报表.txt")+"&delete=true", adminToken, nil)

	if r.Status != http.StatusOK {
		t.Fatalf("下载：HTTP 状态码应为 200，实际 %d", r.Status)
	}
	if string(r.Body) != content {
		t.Errorf("下载内容不对，期望 %q，实际 %q", content, string(r.Body))
	}

	// 【前端读的是这个头】plugins/download.js 里
	// decodeURIComponent(res.headers['download-filename'])，缺了就存成 undefined
	name := r.Header.Get("download-filename")
	if name == "" {
		t.Error("响应必须带 download-filename 头，前端靠它取文件名")
	}
	// 中文文件名必须是百分号编码的，直接塞原文会让响应头变成非法字节
	decoded, err := url.QueryUnescape(name)
	if err != nil {
		t.Errorf("download-filename 应是百分号编码，实际 %q：%v", name, err)
	} else if !strings.HasSuffix(decoded, "报表.txt") {
		// Java 的规则：时间戳 + 第一个下划线之后的部分
		t.Errorf("download-filename 应还原成「<时间戳>报表.txt」，实际解码后是 %q", decoded)
	}

	// 跨域时 JS 读不到未放行的响应头
	if exposed := r.Header.Get("Access-Control-Expose-Headers"); exposed == "" {
		t.Error("必须放行 Content-Disposition 和 download-filename，否则跨域时前端读不到")
	}

	// delete=true 要真的把临时文件删掉
	if _, err := os.Stat(absPath); !os.IsNotExist(err) {
		t.Errorf("delete=true 时下载完应删除源文件 %s", absPath)
	}
}

// TestCommonDownloadKeepsFile 不带 delete 时文件要保留。
func TestCommonDownloadKeepsFile(t *testing.T) {
	absPath := writeTempFile(t, "download", "zz_test_keep.txt", "keep")

	r := request(http.MethodGet, "/common/download?fileName=zz_test_keep.txt", adminToken, nil)
	if r.Status != http.StatusOK || string(r.Body) != "keep" {
		t.Fatalf("下载失败，HTTP %d，响应=%s", r.Status, truncBody(r.Body))
	}
	if _, err := os.Stat(absPath); err != nil {
		t.Errorf("没传 delete 时不该删除源文件：%v", err)
	}
}

// TestCommonDownloadRejects 非法请求一律拒掉，且错误响应要是纯 application/json。
func TestCommonDownloadRejects(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"目录上跳", "fileName=" + url.QueryEscape("../../configs/application.yml")},
		{"绝对路径", "fileName=" + url.QueryEscape("C:/Windows/win.ini")},
		{"扩展名不在白名单", "fileName=zz_test.exe"},
		{"没有扩展名", "fileName=zz_test"},
		{"空文件名", "fileName="},
		{"文件不存在", "fileName=zz_test_根本没有这个文件.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := request(http.MethodGet, "/common/download?"+tc.query, adminToken, nil)

			if r.Status != http.StatusOK {
				t.Fatalf("失败时 HTTP 状态码也应为 200，实际 %d", r.Status)
			}
			if r.Code != 500 {
				t.Fatalf("本应被拒，实际 code=%d，响应=%s", r.Code, truncBody(r.Body))
			}
			// 严格相等，不能带 charset —— 前端 blobValidate() 是 !== 比较
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("错误响应的 Content-Type 必须恰好是 application/json，实际 %q", ct)
			}
			// 不能把服务器上的真实磁盘路径漏给用户
			if appConfig.Upload.Path != "" && strings.Contains(r.Msg, appConfig.Upload.Path) {
				t.Errorf("错误提示里泄漏了磁盘路径：%q", r.Msg)
			}
		})
	}
}

// TestCommonDownloadResource 按入库的资源地址下载。
func TestCommonDownloadResource(t *testing.T) {
	const content = "zz_test 资源内容"
	writeTempFile(t, "upload", "zz_test_res.txt", content)

	resource := appConfig.Upload.URLPrefix + "/upload/zz_test_res.txt"
	r := request(http.MethodGet, "/common/download/resource?resource="+url.QueryEscape(resource), adminToken, nil)

	if r.Status != http.StatusOK {
		t.Fatalf("下载资源：HTTP 状态码应为 200，实际 %d", r.Status)
	}
	if string(r.Body) != content {
		t.Errorf("资源内容不对，期望 %q，实际 %q", content, string(r.Body))
	}
	if r.Header.Get("download-filename") == "" {
		t.Error("响应必须带 download-filename 头")
	}
}

// TestCommonDownloadResourceRejects 资源下载的非法输入。
func TestCommonDownloadResourceRejects(t *testing.T) {
	cases := []struct {
		name     string
		resource string
	}{
		{"目录上跳", "/profile/../../configs/application.yml"},
		{"没有 profile 前缀", "/etc/passwd.txt"},
		{"扩展名不在白名单", "/profile/upload/x.exe"},
		{"空值", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := request(http.MethodGet,
				"/common/download/resource?resource="+url.QueryEscape(tc.resource), adminToken, nil)

			if r.Code != 500 {
				t.Fatalf("本应被拒，实际 code=%d，响应=%s", r.Code, truncBody(r.Body))
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("错误响应的 Content-Type 必须恰好是 application/json，实际 %q", ct)
			}
		})
	}
}

// TestCommonDownloadRequiresAuth 下载要登录。
//
// Java 的 SecurityConfig 放行清单里没有 /common/download，
// 放开就等于让任何人随便下 profile 目录里的文件。
func TestCommonDownloadRequiresAuth(t *testing.T) {
	for _, path := range []string{
		"/common/download?fileName=zz_test_keep.txt",
		"/common/download/resource?resource=/profile/upload/zz_test_res.txt",
	} {
		r := request(http.MethodGet, path, "", nil)
		if r.Code != 401 {
			t.Errorf("%s：未登录时业务 code 应为 401，实际 %d", path, r.Code)
		}
	}
}

// TestUnlockScreen 锁屏解锁。
func TestUnlockScreen(t *testing.T) {
	body := newUserPayload("unlock")
	createUser(t, body)

	token := mustLogin(t, fmt.Sprint(body["userName"]))

	r := request(http.MethodPost, "/unlockscreen", token, map[string]any{"password": ""})
	mustFail(t, r, "密码不能为空", "空密码解锁")

	r = request(http.MethodPost, "/unlockscreen", token, map[string]any{})
	mustFail(t, r, "密码不能为空", "不传密码字段")

	r = request(http.MethodPost, "/unlockscreen", token, map[string]any{"password": "wrongpass"})
	mustFail(t, r, "密码错误", "密码填错")

	r = request(http.MethodPost, "/unlockscreen", token, map[string]any{"password": "test123456"})
	mustOK(t, r, "正确密码解锁")
	if r.Msg != "解锁成功" {
		t.Errorf("解锁成功的 msg 应为「解锁成功」，实际 %q", r.Msg)
	}

	// 解锁只是校验，不能顺手改任何状态：改完还能用原密码登录
	if _, err := loginAs(fmt.Sprint(body["userName"]), "test123456"); err != nil {
		t.Errorf("解锁不该影响账号本身：%v", err)
	}

	// 未登录不能解锁
	r = request(http.MethodPost, "/unlockscreen", "", map[string]any{"password": "test123456"})
	if r.Code != 401 {
		t.Errorf("未登录时解锁应返回 401，实际 %d", r.Code)
	}
}
