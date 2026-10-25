package main

import (
	"testing"
	"time"
)

func TestSalesFinanceCLIGates(t *testing.T) {
	for _, dsn := range []string{"root@tcp(8.8.8.8:3306)/ruoyi_go_mt", "root@tcp(127.0.0.1:3306)/ry", "root@unix(/tmp/mysql.sock)/ruoyi_go_mt"} {
		if validateDSN(dsn) == nil {
			t.Fatal("unsafe DSN")
		}
	}
	if validateDSN("root@tcp(127.0.0.1:3306)/ruoyi_go_mt") != nil {
		t.Fatal("local refused")
	}
	if validateBackup("", time.Now()) == nil {
		t.Fatal("backup optional")
	}
}
