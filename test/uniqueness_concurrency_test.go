package apitest

import (
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestBusinessUniquenessIsSerializedInSingleProcess(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		left  map[string]any
		right map[string]any
	}{
		{
			name: "user", path: "/system/user",
			left:  newUserPayload("concurrent_unique"),
			right: with(newUserPayload("concurrent_unique"), "nickName", "另一个昵称"),
		},
		{
			name: "role", path: "/system/role",
			left:  newRolePayload("concurrent_unique"),
			right: with(newRolePayload("concurrent_unique"), "remark", "另一个备注"),
		},
		{
			name: "post", path: "/system/post",
			left:  newPostPayload("concurrent_unique"),
			right: with(newPostPayload("concurrent_unique"), "remark", "另一个备注"),
		},
		{
			name: "config", path: "/system/config",
			left:  newConfigPayload("concurrent_unique"),
			right: with(newConfigPayload("concurrent_unique"), "configName", testPrefix+"另一个参数名"),
		},
		{
			name: "dept", path: "/system/dept",
			left:  newDeptPayload(rootDeptID, "concurrent_unique"),
			right: with(newDeptPayload(rootDeptID, "concurrent_unique"), "leader", "另一个负责人"),
		},
		{
			name: "menu", path: "/system/menu",
			left:  newDirPayload("concurrent_unique"),
			right: with(newDirPayload("concurrent_unique"), "remark", "另一个备注"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start := make(chan struct{})
			results := make(chan response, 2)
			var wg sync.WaitGroup
			for _, body := range []map[string]any{test.left, test.right} {
				wg.Add(1)
				go func(body map[string]any) {
					defer wg.Done()
					<-start
					results <- request(http.MethodPost, test.path, adminToken, body)
				}(body)
			}
			close(start)
			wg.Wait()
			close(results)

			successes := 0
			var messages []string
			for result := range results {
				if result.Code == 200 {
					successes++
				} else if !strings.Contains(result.Msg, "已存在") {
					t.Fatalf("losing request was not rejected by uniqueness check: code=%d msg=%q", result.Code, result.Msg)
				}
				messages = append(messages, result.Msg)
			}
			if successes != 1 {
				t.Fatalf("concurrent creates succeeded %d times, want exactly 1; messages=%v", successes, messages)
			}
		})
	}
}
