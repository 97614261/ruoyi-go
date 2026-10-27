package router

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/config"
	"ruoyi-go/test/permissionmatrix"
)

// No DB, Redis, authentication or business handlers run here. The first
// middleware inspects Gin's actual registered chain, then aborts. Real HTTP
// function denial and live resource revocation are separate integration tests.
func TestERPPermissionRouteInventory(t *testing.T) {
	routes, err := permissionmatrix.Routes()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]permissionmatrix.Route{}
	for _, route := range routes {
		want[route.Key()] = route
	}
	r := gin.New()
	var names []string
	r.Use(func(c *gin.Context) { names = c.HandlerNames(); c.AbortWithStatus(204) })
	noop := func(c *gin.Context) { c.Next() }
	registerAnonymous(r, noop, noop)
	registerAuthed(r, noop, noop)
	// Also compare the complete production constructor. A future ERP endpoint
	// registered outside the normal registrars must not escape this inventory.
	cfg := &config.Config{}
	cfg.Server.Mode = gin.TestMode
	cfg.Upload.URLPrefix = "/profile"
	production := New(cfg)
	complete := map[string]bool{}
	for _, route := range production.Routes() {
		if strings.HasPrefix(route.Path, "/erp/") {
			key := route.Method + " " + route.Path
			complete[key] = true
			if _, ok := want[key]; !ok {
				t.Errorf("production ERP route missing review: %s", key)
			}
		}
	}
	for key := range want {
		if !complete[key] {
			t.Errorf("reviewed route absent from production constructor: %s", key)
		}
	}
	for _, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, "/erp/") {
			continue
		}
		key := route.Method + " " + route.Path
		spec, ok := want[key]
		if !ok {
			t.Errorf("ERP route missing a permission review: %s", key)
			continue
		}
		delete(want, key)
		t.Run(key, func(t *testing.T) {
			names = nil
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(spec.Method, spec.ProbePath(), nil))
			permissions, customer, owner := 0, 0, 0
			for _, name := range names {
				if strings.Contains(name, "HasPermission.func") {
					permissions++
				}
				if strings.Contains(name, "CustomerWorkspaceOnly.func") {
					customer++
				}
				if strings.Contains(name, "TenantOwnerOnly.func") {
					owner++
				}
			}
			if permissions != len(spec.Permissions) || customer == 0 || (owner > 0) != spec.OwnerOnly {
				t.Fatalf("unreviewed middleware chain: permissions=%d customer=%d owner=%d; want %d/%t", permissions, customer, owner, len(spec.Permissions), spec.OwnerOnly)
			}
		})
	}
	for key := range want {
		t.Errorf("reviewed ERP route no longer registered: %s", key)
	}
}
