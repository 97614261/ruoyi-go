package apitest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/upload"
)

func TestProfileAvatarReplacesFileAndSession(t *testing.T) {
	body := newUserPayload("avatar")
	createUser(t, body)
	token := mustLogin(t, fmt.Sprint(body["userName"]))

	first := requestMultipart("POST", "/system/user/profile/avatar", token,
		"avatarfile", "first.png", []byte("first avatar"))
	mustOK(t, first, "首次上传头像")
	firstURL, _ := first.Raw["imgUrl"].(string)
	firstPath := uploadedAvatarPath(t, firstURL)
	t.Cleanup(func() { _ = os.Remove(firstPath) })
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("首次上传的头像没有落盘：%v", err)
	}

	second := requestMultipart("POST", "/system/user/profile/avatar", token,
		"avatarfile", "second.png", []byte("second avatar"))
	mustOK(t, second, "替换头像")
	secondURL, _ := second.Raw["imgUrl"].(string)
	secondPath := uploadedAvatarPath(t, secondURL)
	t.Cleanup(func() { _ = os.Remove(secondPath) })

	if firstURL == secondURL {
		t.Fatal("两次上传应生成不同的资源地址")
	}
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("替换成功后旧头像应被删除：%v", err)
	}
	if content, err := os.ReadFile(secondPath); err != nil || string(content) != "second avatar" {
		t.Fatalf("新头像应保留且内容正确：content=%q err=%v", content, err)
	}

	loginUser, err := service.GetLoginUser(context.Background(), token)
	if err != nil || loginUser == nil {
		t.Fatalf("读取更新后的登录会话失败：user=%v err=%v", loginUser, err)
	}
	if loginUser.User.Avatar != secondURL {
		t.Fatalf("Redis 会话头像未刷新：got=%q want=%q", loginUser.User.Avatar, secondURL)
	}
}

func TestProfileAvatarConcurrentReplacementLeavesOnlyCurrentFile(t *testing.T) {
	body := newUserPayload("avatar_concurrent")
	createUser(t, body)
	token := mustLogin(t, fmt.Sprint(body["userName"]))

	initial := requestMultipart("POST", "/system/user/profile/avatar", token,
		"avatarfile", "initial.png", []byte("initial avatar"))
	mustOK(t, initial, "准备并发替换前的头像")
	initialURL, _ := initial.Raw["imgUrl"].(string)
	initialPath := uploadedAvatarPath(t, initialURL)
	t.Cleanup(func() { _ = os.Remove(initialPath) })

	responses := make([]response, 2)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			responses[index] = requestMultipart("POST", "/system/user/profile/avatar", token,
				"avatarfile", fmt.Sprintf("parallel-%d.png", index),
				[]byte(fmt.Sprintf("parallel avatar %d", index)))
		}(i)
	}
	wg.Wait()

	responseURLs := make(map[string]string, len(responses))
	for index, result := range responses {
		mustOK(t, result, fmt.Sprintf("第 %d 个并发头像请求", index+1))
		resource, _ := result.Raw["imgUrl"].(string)
		resourcePath := uploadedAvatarPath(t, resource)
		responseURLs[resource] = resourcePath
		t.Cleanup(func() { _ = os.Remove(resourcePath) })
	}

	loginUser, err := service.GetLoginUser(context.Background(), token)
	if err != nil || loginUser == nil {
		t.Fatalf("读取并发更新后的登录会话失败：user=%v err=%v", loginUser, err)
	}
	currentURL := loginUser.User.Avatar
	currentPath, ok := responseURLs[currentURL]
	if !ok {
		t.Fatalf("最终头像 %q 不是任一并发请求写入的地址", currentURL)
	}
	if _, err := os.Stat(currentPath); err != nil {
		t.Fatalf("数据库最终引用的头像必须存在：%v", err)
	}
	if _, err := os.Stat(initialPath); !os.IsNotExist(err) {
		t.Fatalf("并发替换后最初头像应被删除：%v", err)
	}
	for resource, resourcePath := range responseURLs {
		if resource == currentURL {
			continue
		}
		if _, err := os.Stat(resourcePath); !os.IsNotExist(err) {
			t.Fatalf("被后一个请求替换的并发头像应删除：resource=%s err=%v", resource, err)
		}
	}
}

func uploadedAvatarPath(t *testing.T, resource string) string {
	t.Helper()
	relative := upload.StripResourcePrefix(resource, appConfig.Upload.URLPrefix)
	if !strings.HasPrefix(relative, "/avatar/") {
		t.Fatalf("头像资源地址不属于 avatar 目录：%q", resource)
	}
	target, err := upload.SafeJoin(appConfig.Upload.Path, strings.TrimPrefix(relative, "/"))
	if err != nil {
		t.Fatalf("解析头像资源地址失败：%v", err)
	}
	return target
}
