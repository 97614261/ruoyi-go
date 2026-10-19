package main

import "testing"

func TestConsoleRefusesOriginalOrRemoteDatabase(t *testing.T) {
	for _, dsn := range []string{"root@tcp(127.0.0.1:3306)/ry", "root@tcp(192.0.2.1:3306)/ruoyi_go_mt", "root@unix(/mysql.sock)/ruoyi_go_mt", "bad"} {
		if validateLocalDSN(dsn) == nil {
			t.Fatal("accepted non-local or original database")
		}
	}
	if err := validateLocalDSN("root@tcp(127.0.0.1:3306)/ruoyi_go_mt"); err != nil {
		t.Fatal(err)
	}
}
