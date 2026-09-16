package service

import "testing"

func TestParseCreateTableStatements(t *testing.T) {
	sqlText := "-- first\nCREATE TABLE `demo_one` (id bigint primary key, note varchar(20) default ';');\n" +
		"/* second */ CREATE TABLE IF NOT EXISTS demo_two (id bigint, body text);"
	statements, names, err := parseCreateTableStatements(sqlText)
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 || len(names) != 2 || names[0] != "demo_one" || names[1] != "demo_two" {
		t.Fatalf("unexpected parse result: statements=%v names=%v", statements, names)
	}
}

func TestParseCreateTableStatementsRejectsNonDDL(t *testing.T) {
	bad := []string{
		"DROP TABLE sys_user",
		"CREATE TABLE db_name.demo (id bigint)",
		"CREATE TEMPORARY TABLE demo (id bigint)",
		"CREATE TABLE demo (id bigint); DELETE FROM sys_user",
		"CREATE TABLE demo (id bigint); CREATE TABLE demo (id bigint)",
		"CREATE TABLE demo (name varchar(20) default 'x)",
		"/*!50000 CREATE TABLE demo (id bigint) */",
		"DELIMITER $$ CREATE TABLE demo (id bigint)$$",
		"CREATE TABLE demo (id bigint)\x00",
	}
	for _, input := range bad {
		if _, _, err := parseCreateTableStatements(input); err == nil {
			t.Fatalf("expected rejection for %q", input)
		}
	}
}
