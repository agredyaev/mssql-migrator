package scaffold

import (
	"regexp"
	"strings"
)

var colDefRe = regexp.MustCompile(`(?i)^\s*\[?(\w+)\]?\s+(\[?\w+\]?(?:\s*\(\s*\d+(?:\s*,\s*\d+)?\s*\))?)\s*(NULL|NOT\s+NULL)?`)

type parsedColumn struct {
	name     string
	typeName string
	nullable bool
}

func parseTableColumns(sql string) []parsedColumn {
	body := extractColumnBody(sql)
	if body == "" {
		return nil
	}

	var cols []parsedColumn
	parts := splitColumns(body)
	for _, part := range parts {
		col := parseColumnDef(part)
		if col != nil {
			cols = append(cols, *col)
		}
	}
	return cols
}

func extractColumnBody(sql string) string {
	s := strings.ToUpper(strings.ReplaceAll(sql, "\n", " "))
	idx := strings.Index(s, "(")
	if idx < 0 {
		return ""
	}
	body := s[idx+1:]
	last := strings.LastIndex(body, ")")
	if last < 0 {
		return ""
	}
	return strings.TrimSpace(body[:last])
}

func splitColumns(body string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(body) {
		parts = append(parts, strings.TrimSpace(body[start:]))
	}
	return parts
}

func parseColumnDef(def string) *parsedColumn {
	matches := colDefRe.FindStringSubmatch(def)
	if matches == nil {
		return nil
	}
	col := &parsedColumn{
		name:     strings.ToLower(strings.Trim(matches[1], "[]")),
		typeName: strings.TrimSpace(matches[2]),
		nullable: !strings.Contains(strings.ToUpper(matches[3]), "NOT NULL"),
	}
	return col
}

func newColumns(fileCols []parsedColumn, dbNames map[string]bool) []parsedColumn {
	var added []parsedColumn
	for _, c := range fileCols {
		if !dbNames[c.name] {
			added = append(added, c)
		}
	}
	return added
}

func hasDroppedColumns(fileCols []parsedColumn, dbNames map[string]bool) bool {
	fileSet := make(map[string]bool, len(fileCols))
	for _, c := range fileCols {
		fileSet[c.name] = true
	}
	for name := range dbNames {
		if !fileSet[name] {
			return true
		}
	}
	return false
}
