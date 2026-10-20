package multitenant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

// Credentials stay in memory. A pasted newline in the private local record may
// be trimmed ONLY when the candidate matches the existing database bcrypt hash.
// This never resets a password, rewrites the private record or saves a copy.
func loadTestAccounts(t *testing.T, root string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "logs/multitenant-accounts.local.json"))
	if err != nil {
		t.Fatal("private account record unavailable")
	}
	var fixed []byte
	quoted, escaped := false, false
	for _, b := range raw {
		if quoted && !escaped && b < 32 {
			fixed = append(fixed, []byte(fmt.Sprintf(`\u%04x`, b))...)
			continue
		}
		fixed = append(fixed, b)
		if b == '"' && !escaped {
			quoted = !quoted
		}
		if b == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}
	var a struct {
		Username  string            `json:"username"`
		Passwords map[string]string `json:"passwords"`
	}
	if json.Unmarshal(fixed, &a) != nil {
		t.Fatal("private account record is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Mutating checks create their own customers. Only the existing platform
	// account is required; demo login readiness is a separate operational check.
	for _, code := range []string{"platform"} {
		tenant, err := repository.ResolveTenant(ctx, code)
		if err != nil || tenant == nil {
			t.Fatal("account tenant missing")
		}
		user, err := repository.SelectUserAccountByUserName(tenantctx.With(ctx, tenant.TenantID, code == "platform"), a.Username)
		if err != nil || user == nil {
			t.Fatal("account user missing")
		}
		value := a.Passwords[code]
		if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(value)) == nil {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if value == trimmed || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(trimmed)) != nil {
			t.Fatalf("required private account record needs correction: %s; not resetting password", code)
		}
		a.Passwords[code] = trimmed
	}
	return map[string]string{"platform": a.Passwords["platform"]}
}
