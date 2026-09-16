package service

import (
	"regexp"
	"strings"
	"unicode"

	"ruoyi-go/pkg/errs"
)

const (
	maxCreateSQLBytes   = 1 << 20
	maxCreateTableCount = 20
)

var createTableHead = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:` + "`" + `([A-Za-z_][A-Za-z0-9_$]*)` + "`" + `|([A-Za-z_][A-Za-z0-9_$]*))\s*\(`)

// parseCreateTableStatements 将用户输入限制成当前数据库内的 CREATE TABLE。
// 它只负责语句边界和白名单，不尝试实现完整 MySQL 语法；最终语法仍由 MySQL 校验。
func parseCreateTableStatements(sqlText string) ([]string, []string, error) {
	if len(sqlText) == 0 {
		return nil, nil, errs.New("请输入建表语句")
	}
	if len(sqlText) > maxCreateSQLBytes {
		return nil, nil, errs.New("建表语句不能超过1MB")
	}
	upper := strings.ToUpper(sqlText)
	if strings.Contains(sqlText, "\x00") || strings.Contains(sqlText, "/*!") || strings.Contains(upper, "DELIMITER") {
		return nil, nil, errs.New("建表语句包含不允许的 MySQL 执行指令")
	}
	statements, err := splitSQLStatements(strings.TrimPrefix(sqlText, "\ufeff"))
	if err != nil {
		return nil, nil, err
	}
	if len(statements) == 0 {
		return nil, nil, errs.New("请输入建表语句")
	}
	if len(statements) > maxCreateTableCount {
		return nil, nil, errs.Newf("一次最多创建%d张表", maxCreateTableCount)
	}
	names := make([]string, 0, len(statements))
	seen := make(map[string]struct{}, len(statements))
	for i := range statements {
		statement := strings.TrimSpace(stripLeadingSQLComments(statements[i]))
		match := createTableHead.FindStringSubmatch(statement)
		if len(match) == 0 {
			return nil, nil, errs.New("只允许执行 CREATE TABLE 建表语句")
		}
		name := match[1]
		if name == "" {
			name = match[2]
		}
		key := strings.ToLower(name)
		if _, duplicate := seen[key]; duplicate {
			return nil, nil, errs.Newf("表 %s 重复创建", name)
		}
		seen[key] = struct{}{}
		statements[i] = statement
		names = append(names, name)
	}
	return statements, names, nil
}

func splitSQLStatements(input string) ([]string, error) {
	var statements []string
	start := 0
	quote := rune(0)
	escaped := false
	lineComment := false
	blockComment := false
	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		if lineComment {
			if r == '\n' || r == '\r' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if r == '*' && next == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if r == quote {
				// MySQL also escapes a quote by doubling it.
				if next == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		switch {
		case r == '-' && next == '-' && (i+2 >= len(runes) || unicode.IsSpace(runes[i+2])):
			lineComment = true
			i++
		case r == '#':
			lineComment = true
		case r == '/' && next == '*':
			blockComment = true
			i++
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == ';':
			if statement := strings.TrimSpace(string(runes[start:i])); statement != "" {
				statements = append(statements, statement)
			}
			start = i + 1
		}
	}
	if quote != 0 || blockComment {
		return nil, errs.New("建表语句中的引号或注释没有闭合")
	}
	if statement := strings.TrimSpace(string(runes[start:])); statement != "" {
		statements = append(statements, statement)
	}
	return statements, nil
}

func stripLeadingSQLComments(input string) string {
	text := strings.TrimSpace(input)
	for {
		switch {
		case strings.HasPrefix(text, "--"):
			if index := strings.IndexByte(text, '\n'); index >= 0 {
				text = strings.TrimSpace(text[index+1:])
				continue
			}
			return ""
		case strings.HasPrefix(text, "#"):
			if index := strings.IndexByte(text, '\n'); index >= 0 {
				text = strings.TrimSpace(text[index+1:])
				continue
			}
			return ""
		case strings.HasPrefix(text, "/*"):
			if index := strings.Index(text[2:], "*/"); index >= 0 {
				text = strings.TrimSpace(text[index+4:])
				continue
			}
			return text
		default:
			return text
		}
	}
}
