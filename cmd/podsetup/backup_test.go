package main

import "testing"

func TestMySQLOptionQuotesSpecialCredentials(t *testing.T) {
	if got := mysqlOption("a\\b\"c\nnew\rline\ttab#; "); got != `"a\\b\"c\nnew\rline\ttab#; "` {
		t.Fatalf("option escaping mismatch: %q", got)
	}
}
