package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// Registration itself detects wildcard conflicts, which endpoint-only handler
// tests do not catch. Both registrar orders must preserve the old package paths.
func TestLogisticsPackageRouteRegistration(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		t.Run(map[bool]string{false: "orders_first", true: "logistics_first"}[reversed], func(t *testing.T) {
			r := gin.New()
			g := r.Group("")
			if reversed {
				registerLogistics(g)
				registerERPOrders(g)
			} else {
				registerERPOrders(g)
				registerLogistics(g)
			}
			want := map[string]bool{
				"GET /erp/package/:packageId":           false,
				"PUT /erp/package/:packageId/logistics": false,
				"PUT /erp/package/:packageId/ship":      false,
				"GET /erp/package/:packageId/shipping":  false,
				"PUT /erp/package/:packageId/shipping":  false,
			}
			for _, route := range r.Routes() {
				key := route.Method + " " + route.Path
				if _, ok := want[key]; ok {
					want[key] = true
				}
			}
			for path, found := range want {
				if !found {
					t.Errorf("missing route: %s", path)
				}
			}
		})
	}
}
