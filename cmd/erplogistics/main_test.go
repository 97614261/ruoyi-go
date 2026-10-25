package main

import "testing"

func TestLogisticsInstallerRejectsOriginalRemoteAndClone(t *testing.T) {
	for _, dsn := range []string{"x:x@tcp(127.0.0.1:3306)/ruoyi_go", "x:x@tcp(127.0.0.1:3306)/ruoyi_go_mt_restore_20261025", "x:x@tcp(192.0.2.1:3306)/ruoyi_go_mt", "x:x@unix(/tmp/mysql.sock)/ruoyi_go_mt", "malformed"} {
		if validateDSN(dsn) == nil {
			t.Fatal("unsafe connection accepted")
		}
	}
	for _, dsn := range []string{"x:x@tcp(127.0.0.1:3306)/ruoyi_go_mt", "x:x@tcp(localhost:3306)/ruoyi_go_mt", "x:x@tcp([::1]:3306)/ruoyi_go_mt"} {
		if err := validateDSN(dsn); err != nil {
			t.Fatal("valid local dedicated DSN rejected")
		}
	}
}
