package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type TableColumn struct {
	Name            string
	NormalizedName  string
	TypeName        string
	Length          int
	Precision       int
	Scale           int
	Nullable        bool
	NullableKnown   bool
	DefinitionSQL   string
	AutoAddEligible bool
	SignatureKnown  bool
}

var builtInTableTypes = map[string]struct{}{
	"bigint": {}, "int": {}, "smallint": {}, "tinyint": {}, "bit": {},
	"decimal": {}, "numeric": {}, "float": {}, "real": {},
	"money": {}, "smallmoney": {},
	"date": {}, "datetime": {}, "datetime2": {}, "smalldatetime": {}, "time": {}, "datetimeoffset": {},
	"char": {}, "varchar": {}, "nchar": {}, "nvarchar": {},
	"binary": {}, "varbinary": {},
	"uniqueidentifier": {},
}

var columnNullPattern = regexp.MustCompile(`(?i)\bNULL\b`)
var columnNotNullPattern = regexp.MustCompile(`(?i)\bNOT\s+NULL\b`)

func ParseCreateTableColumns(content string) ([]TableColumn, error) {
	batches, err := SplitGO(content)
	if err != nil {
		return nil, err
	}
	if len(batches) != 1 {
		return nil, fmt.Errorf("automatic table updates require a single CREATE TABLE batch")
	}
	body, err := extractCreateTableBody(batches[0].SQL)
	if err != nil {
		return nil, err
	}
	items, err := splitTopLevelSQLList(body)
	if err != nil {
		return nil, err
	}
	columns := make([]TableColumn, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || isTableConstraintDefinition(trimmed) {
			continue
		}
		column, err := parseTableColumnDefinition(trimmed)
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("automatic table updates require at least one parseable column definition")
	}
	return columns, nil
}

func extractCreateTableBody(sql string) (string, error) {
	upper := strings.ToUpper(sql)
	start := strings.Index(upper, "CREATE TABLE")
	if start < 0 {
		return "", fmt.Errorf("automatic table updates require CREATE TABLE syntax")
	}
	open := -1
	for i := start; i < len(sql); i++ {
		if sql[i] == '(' {
			open = i
			break
		}
	}
	if open < 0 {
		return "", fmt.Errorf("automatic table updates require CREATE TABLE column list")
	}
	depth := 0
	state := sqlSplitState{}
	for i := open; i < len(sql); i++ {
		state.advance(sql, i)
		if state.inTrivia() {
			continue
		}
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return sql[open+1 : i], nil
			}
		}
	}
	return "", fmt.Errorf("automatic table updates require a balanced CREATE TABLE column list")
}

func splitTopLevelSQLList(body string) ([]string, error) {
	items := []string{}
	start := 0
	depth := 0
	state := sqlSplitState{}
	for i := 0; i < len(body); i++ {
		state.advance(body, i)
		if state.inTrivia() {
			continue
		}
		switch body[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				items = append(items, body[start:i])
				start = i + 1
			}
		}
	}
	items = append(items, body[start:])
	return items, nil
}

func isTableConstraintDefinition(item string) bool {
	upper := strings.ToUpper(strings.TrimSpace(item))
	for _, prefix := range []string{"CONSTRAINT ", "PRIMARY KEY", "UNIQUE ", "CHECK ", "FOREIGN KEY"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func parseTableColumnDefinition(item string) (TableColumn, error) {
	nameToken, rest, err := splitLeadingIdentifier(item)
	if err != nil {
		return TableColumn{}, err
	}
	name := normalizeSQLIdentifierToken(nameToken)
	if err := validateSQLIdentifier("column", name); err != nil {
		return TableColumn{}, err
	}
	typeToken, tail := splitLeadingTypeToken(rest)
	if strings.TrimSpace(typeToken) == "" {
		return TableColumn{}, fmt.Errorf("automatic table updates require a data type for column %s", name)
	}
	column := TableColumn{
		Name:           name,
		NormalizedName: strings.ToLower(name),
		DefinitionSQL:  strings.TrimSpace(item),
	}
	baseType, length, precision, scale, known := parseTableColumnType(typeToken)
	column.TypeName = baseType
	column.Length = length
	column.Precision = precision
	column.Scale = scale
	column.SignatureKnown = known
	column.Nullable, column.NullableKnown = parseColumnNullability(tail)
	column.AutoAddEligible = column.SignatureKnown && column.NullableKnown && column.Nullable && autoAddTailAllowed(tail)
	return column, nil
}

func splitLeadingIdentifier(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("automatic table updates require a column definition")
	}
	if value[0] == '[' {
		end := strings.IndexByte(value, ']')
		if end < 0 {
			return "", "", fmt.Errorf("automatic table updates found unterminated bracketed identifier")
		}
		return value[:end+1], strings.TrimSpace(value[end+1:]), nil
	}
	for i := 0; i < len(value); i++ {
		if value[i] == ' ' || value[i] == '\t' || value[i] == '\n' || value[i] == '\r' {
			return value[:i], strings.TrimSpace(value[i:]), nil
		}
	}
	return value, "", nil
}

func splitLeadingTypeToken(value string) (string, string) {
	value = strings.TrimSpace(value)
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ', '\t', '\n', '\r':
			if depth == 0 {
				return strings.TrimSpace(value[:i]), strings.TrimSpace(value[i:])
			}
		}
	}
	return strings.TrimSpace(value), ""
}

func parseTableColumnType(typeToken string) (string, int, int, int, bool) {
	typeToken = strings.TrimSpace(typeToken)
	base := typeToken
	args := ""
	if open := strings.IndexByte(typeToken, '('); open >= 0 && strings.HasSuffix(typeToken, ")") {
		base = strings.TrimSpace(typeToken[:open])
		args = strings.TrimSpace(typeToken[open+1 : len(typeToken)-1])
	}
	base = strings.ToLower(normalizeSQLIdentifierToken(base))
	if _, ok := builtInTableTypes[base]; !ok {
		return base, 0, 0, 0, false
	}
	switch base {
	case "char", "varchar", "nchar", "nvarchar", "binary", "varbinary":
		length, ok := parseTypeLength(args)
		return base, length, 0, 0, ok
	case "decimal", "numeric":
		precision, scale, ok := parsePrecisionScale(args)
		return base, 0, precision, scale, ok
	case "datetime2", "time", "datetimeoffset":
		if args == "" {
			return base, 0, 0, 7, true
		}
		scale, err := strconv.Atoi(strings.TrimSpace(args))
		return base, 0, 0, scale, err == nil
	default:
		if args != "" {
			return base, 0, 0, 0, false
		}
		return base, 0, 0, 0, true
	}
}

func parseTypeLength(args string) (int, bool) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 0, false
	}
	if strings.EqualFold(args, "MAX") {
		return -1, true
	}
	length, err := strconv.Atoi(args)
	return length, err == nil
}

func parsePrecisionScale(args string) (int, int, bool) {
	if strings.TrimSpace(args) == "" {
		return 0, 0, false
	}
	parts := strings.Split(args, ",")
	if len(parts) == 1 {
		precision, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		return precision, 0, err == nil
	}
	if len(parts) != 2 {
		return 0, 0, false
	}
	precision, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	scale, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	return precision, scale, err1 == nil && err2 == nil
}

func parseColumnNullability(tail string) (bool, bool) {
	upper := strings.ToUpper(tail)
	if columnNotNullPattern.MatchString(upper) {
		return false, true
	}
	if columnNullPattern.MatchString(upper) {
		return true, true
	}
	return false, false
}

func autoAddTailAllowed(tail string) bool {
	upper := strings.ToUpper(strings.TrimSpace(tail))
	if upper == "" || upper == "NULL" {
		return true
	}
	return false
}

func normalizeSQLIdentifierToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "[")
	token = strings.TrimSuffix(token, "]")
	return strings.TrimSpace(token)
}

type sqlSplitState struct {
	stringLiteral bool
	lineComment   bool
	blockDepth    int
	bracketIdent  bool
}

func (s *sqlSplitState) inTrivia() bool {
	return s.lineComment || s.blockDepth > 0 || s.stringLiteral || s.bracketIdent
}

func (s *sqlSplitState) advance(input string, i int) {
	if s.lineComment {
		if input[i] == '\n' {
			s.lineComment = false
		}
		return
	}
	if s.stringLiteral {
		if input[i] == '\'' {
			if i+1 < len(input) && input[i+1] == '\'' {
				return
			}
			s.stringLiteral = false
		}
		return
	}
	if s.bracketIdent {
		if input[i] == ']' {
			s.bracketIdent = false
		}
		return
	}
	if s.blockDepth > 0 {
		if i+1 < len(input) && input[i] == '*' && input[i+1] == '/' {
			s.blockDepth--
		}
		return
	}
	if i+1 < len(input) && input[i] == '-' && input[i+1] == '-' {
		s.lineComment = true
		return
	}
	if i+1 < len(input) && input[i] == '/' && input[i+1] == '*' {
		s.blockDepth++
		return
	}
	if input[i] == '\'' {
		s.stringLiteral = true
		return
	}
	if input[i] == '[' {
		s.bracketIdent = true
	}
}
