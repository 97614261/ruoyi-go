package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJavaControllerMultiplePathsAndPermission(t *testing.T) {
	input := `
@RequestMapping("/system/user")
public class SysUserController {
    @PreAuthorize("@ss.hasPermi('system:user:query')")
    @GetMapping(value = { "/", "/{userId}" })
    public AjaxResult getInfo(@PathVariable Long userId) { return null; }
}`
	routes := parseJavaController(input, "SysUserController.java")
	if len(routes) != 2 {
		t.Fatalf("routes=%#v", routes)
	}
	for _, item := range routes {
		if item.Method != "GET" || item.Permission != "system:user:query" || item.Handler != "getInfo" {
			t.Fatalf("route=%#v", item)
		}
	}
}

func TestParseGoRouterGroupAndPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.go")
	input := `package router
func register(g *gin.RouterGroup) {
    user := g.Group("/system/user")
    user.GET("/:userId", middleware.HasPermission("system:user:query"), handler.UserGet)
}`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := parseGoRouter(path)
	if err != nil || len(routes) != 1 {
		t.Fatalf("routes=%#v err=%v", routes, err)
	}
	if got := routeKey(routes[0]); got != "GET /system/user/{}" {
		t.Fatalf("key=%q", got)
	}
	if routes[0].Permission != "system:user:query" || routes[0].Handler != "UserGet" {
		t.Fatalf("route=%#v", routes[0])
	}
}

func TestNormalizePathTreatsTrailingSlashAsAlias(t *testing.T) {
	if got := normalizePath(joinPath("/system/user", "/")); got != "/system/user" {
		t.Fatalf("got=%q", got)
	}
}
