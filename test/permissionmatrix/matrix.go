// Package permissionmatrix holds a reviewed test-only ERP route inventory.
// It is intentionally independent of runtime registration: a new route must
// receive an explicit review instead of silently expanding its own baseline.
package permissionmatrix

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed routes.tsv
var inventory string

type Route struct {
	Method      string
	Path        string
	Permissions []string
	OwnerOnly   bool
}

func (r Route) Key() string { return r.Method + " " + r.Path }

// ProbePath addresses no existing business record. Negative function-gate
// checks must never substitute IDs belonging to an existing customer.
func (r Route) ProbePath() string {
	parts := strings.Split(r.Path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "9007199254740000"
		}
	}
	return strings.Join(parts, "/")
}

func Routes() ([]Route, error) {
	var routes []Route
	seen := map[string]bool{}
	for i, line := range strings.Split(inventory, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 4 || !strings.HasPrefix(fields[1], "/erp/") || (fields[3] != "employee" && fields[3] != "owner") {
			return nil, fmt.Errorf("invalid permission inventory line %d", i+1)
		}
		if !strings.Contains(" GET POST PUT DELETE ", " "+fields[0]+" ") {
			return nil, fmt.Errorf("invalid method at line %d", i+1)
		}
		r := Route{Method: fields[0], Path: fields[1], Permissions: strings.Split(fields[2], ","), OwnerOnly: fields[3] == "owner"}
		for _, p := range r.Permissions {
			if !strings.HasPrefix(p, "erp:") && p != "system:role:add" {
				return nil, fmt.Errorf("invalid permission at line %d", i+1)
			}
		}
		if seen[r.Key()] {
			return nil, fmt.Errorf("duplicate permission inventory route %s", r.Key())
		}
		seen[r.Key()] = true
		routes = append(routes, r)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("empty permission inventory")
	}
	return routes, nil
}
