package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type oldAudit struct {
	Watermark int64             `json:"watermarkTenantId"`
	Tables    []string          `json:"tables"`
	Hashes    map[string]string `json:"oldRowHashes"`
}

func oldHash(ctx context.Context, db *sql.DB, table string, wm int64) string {
	rows, e := db.QueryContext(ctx, "SELECT * FROM `"+strings.ReplaceAll(table, "`", "``")+"` WHERE tenant_id<=?", wm)
	must(e)
	defer rows.Close()
	cols, e := rows.Columns()
	must(e)
	all := []string{}
	for rows.Next() {
		v := make([]any, len(cols))
		p := make([]any, len(cols))
		for i := range v {
			p[i] = &v[i]
		}
		must(rows.Scan(p...))
		b, e := json.Marshal(v)
		must(e)
		all = append(all, string(b))
	}
	must(rows.Err())
	sort.Strings(all)
	b, e := json.Marshal(all)
	must(e)
	return digest(b)
}
func captureOld(db *sql.DB) oldAudit {
	ctx, c := context.WithTimeout(context.Background(), time.Minute)
	defer c()
	a := oldAudit{Hashes: map[string]string{}}
	must(db.QueryRowContext(ctx, "SELECT COALESCE(MAX(tenant_id),0) FROM sys_tenant").Scan(&a.Watermark))
	rows, e := db.QueryContext(ctx, "SELECT table_name FROM information_schema.columns WHERE table_schema=DATABASE() AND column_name='tenant_id' AND LEFT(table_name,4)='erp_' ORDER BY table_name")
	must(e)
	for rows.Next() {
		var name string
		must(rows.Scan(&name))
		if !strings.HasPrefix(name, "erp_import_") {
			a.Tables = append(a.Tables, name)
		}
	}
	must(rows.Err())
	must(rows.Close())
	a.Tables = append(a.Tables, "sys_tenant_menu", "sys_role_menu", "sys_user_role")
	for _, name := range a.Tables {
		a.Hashes[name] = oldHash(ctx, db, name, a.Watermark)
	}
	return a
}
func verifyOld(db *sql.DB, a oldAudit) {
	ctx, c := context.WithTimeout(context.Background(), time.Minute)
	defer c()
	for _, name := range a.Tables {
		if oldHash(ctx, db, name, a.Watermark) != a.Hashes[name] {
			panic(fail("existing business/grant rows changed: " + name))
		}
	}
	must(save(relRoot+"/old-data-verified.json", map[string]any{"passed": true, "tables": len(a.Tables), "watermark": a.Watermark, "atUtc": time.Now().UTC(), "scope": fmt.Sprint(a.Tables)}))
}
