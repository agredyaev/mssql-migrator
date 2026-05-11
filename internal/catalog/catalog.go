package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/contracts"
)

type State struct {
	Schemas      map[string]struct{}
	Objects      map[string]Object
	TableColumns map[string][]TableColumn
}

type Object struct {
	SchemaName string
	Kind       string
	ObjectName string
	ParentName string
}

type TableColumn struct {
	Name           string
	NormalizedName string
	TypeName       string
	Length         int
	Precision      int
	Scale          int
	Nullable       bool
}

const StateQuery = `
SELECT s.name, o.type_desc, o.name, ISNULL(parent.name, '')
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
LEFT JOIN sys.objects parent ON parent.object_id = o.parent_object_id
WHERE o.is_ms_shipped = 0
UNION ALL
SELECT s.name, 'USER_TABLE_TYPE', tt.name, ''
FROM sys.table_types tt
JOIN sys.schemas s ON s.schema_id = tt.schema_id
UNION ALL
SELECT s.name, 'INDEX', i.name, o.name
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE i.is_hypothetical = 0 AND i.name IS NOT NULL AND o.type IN ('U', 'V') AND o.is_ms_shipped = 0
ORDER BY 1, 3`

var stateQueryTemplate = `
SELECT s.name, o.type_desc, o.name, ISNULL(parent.name, '')
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
LEFT JOIN sys.objects parent ON parent.object_id = o.parent_object_id
WHERE o.is_ms_shipped = 0%s
UNION ALL
SELECT s.name, 'USER_TABLE_TYPE', tt.name, ''
FROM sys.table_types tt
JOIN sys.schemas s ON s.schema_id = tt.schema_id
WHERE 1 = 1%s
UNION ALL
SELECT s.name, 'INDEX', i.name, o.name
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE i.is_hypothetical = 0 AND i.name IS NOT NULL AND o.type IN ('U', 'V') AND o.is_ms_shipped = 0%s
ORDER BY 1, 3`

var schemaQueryTemplate = `SELECT name FROM sys.schemas WHERE name NOT IN ('sys', 'INFORMATION_SCHEMA')%s ORDER BY name`

var tableColumnsQueryTemplate = `
SELECT s.name, t.name, c.name, TYPE_NAME(c.user_type_id), c.max_length, c.precision, c.scale, c.is_nullable
FROM sys.tables t
JOIN sys.schemas s ON s.schema_id = t.schema_id
JOIN sys.columns c ON c.object_id = t.object_id
WHERE t.is_ms_shipped = 0%s
ORDER BY s.name, t.name, c.column_id`

func Read(ctx context.Context, conn *sql.Conn) (State, error) {
	return ReadForSchemas(ctx, conn, nil)
}

func ReadForSchemas(ctx context.Context, conn *sql.Conn, schemas []string) (State, error) {
	state := State{
		Schemas:      map[string]struct{}{},
		Objects:      map[string]Object{},
		TableColumns: map[string][]TableColumn{},
	}
	if conn == nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, fmt.Errorf("catalog read: missing database connection"))
	}
	schemaQuery, schemaArgs := schemaQueryForSchemas(schemas)
	schemaRows, err := conn.QueryContext(ctx, schemaQuery, schemaArgs...)
	if err != nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	for schemaRows.Next() {
		var schemaName string
		if err := schemaRows.Scan(&schemaName); err != nil {
			schemaRows.Close()
			return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
		}
		state.Schemas[strings.ToLower(schemaName)] = struct{}{}
	}
	if err := schemaRows.Err(); err != nil {
		schemaRows.Close()
		return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	schemaRows.Close()

	stateQuery, stateArgs := stateQueryForSchemas(schemas)
	rows, err := conn.QueryContext(ctx, stateQuery, stateArgs...)
	if err != nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	defer rows.Close()
	for rows.Next() {
		var schemaName string
		var typeDesc string
		var objectName string
		var parentName string
		if err := rows.Scan(&schemaName, &typeDesc, &objectName, &parentName); err != nil {
			return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
		}
		kind := MapTypeDescToKind(typeDesc)
		if kind == "" {
			continue
		}
		state.Objects[NormalizedKey(schemaName, kind, parentName, objectName)] = Object{
			SchemaName: schemaName,
			Kind:       kind,
			ObjectName: objectName,
			ParentName: parentName,
		}
	}
	if err := rows.Err(); err != nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	columnQuery, columnArgs := tableColumnsQueryForSchemas(schemas)
	columnRows, err := conn.QueryContext(ctx, columnQuery, columnArgs...)
	if err != nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var schemaName string
		var tableName string
		var columnName string
		var typeName string
		var maxLength int
		var precision int
		var scale int
		var nullable bool
		if err := columnRows.Scan(&schemaName, &tableName, &columnName, &typeName, &maxLength, &precision, &scale, &nullable); err != nil {
			return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
		}
		key := NormalizedKey(schemaName, "tables", "", tableName)
		state.TableColumns[key] = append(state.TableColumns[key], TableColumn{
			Name:           columnName,
			NormalizedName: strings.ToLower(columnName),
			TypeName:       strings.ToLower(strings.TrimSpace(typeName)),
			Length:         normalizeCatalogColumnLength(typeName, maxLength),
			Precision:      precision,
			Scale:          scale,
			Nullable:       nullable,
		})
	}
	if err := columnRows.Err(); err != nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	return state, nil
}

func schemaQueryForSchemas(schemas []string) (string, []any) {
	clause, args, _ := schemaFilterClauseAt("name", schemas, 1)
	return fmt.Sprintf(schemaQueryTemplate, clause), args
}

func stateQueryForSchemas(schemas []string) (string, []any) {
	clauseObjects, argsObjects, next := schemaFilterClauseAt("s.name", schemas, 1)
	clauseTypes, argsTypes, next := schemaFilterClauseAt("s.name", schemas, next)
	clauseIndexes, argsIndexes, _ := schemaFilterClauseAt("s.name", schemas, next)
	args := append(argsObjects, argsTypes...)
	args = append(args, argsIndexes...)
	return fmt.Sprintf(stateQueryTemplate, clauseObjects, clauseTypes, clauseIndexes), args
}

func tableColumnsQueryForSchemas(schemas []string) (string, []any) {
	clause, args, _ := schemaFilterClauseAt("s.name", schemas, 1)
	return fmt.Sprintf(tableColumnsQueryTemplate, clause), args
}

func schemaFilterClauseAt(column string, schemas []string, start int) (string, []any, int) {
	cleaned := normalizedSchemaFilter(schemas)
	if len(cleaned) == 0 {
		return "", nil, start
	}
	placeholders := make([]string, len(cleaned))
	args := make([]any, len(cleaned))
	for i, schema := range cleaned {
		placeholders[i] = fmt.Sprintf("@p%d", start+i)
		args[i] = schema
	}
	return fmt.Sprintf(" AND %s IN (%s)", column, strings.Join(placeholders, ", ")), args, start + len(cleaned)
}

func normalizedSchemaFilter(schemas []string) []string {
	if len(schemas) == 0 {
		return nil
	}
	result := make([]string, 0, len(schemas))
	seen := map[string]struct{}{}
	for _, schema := range schemas {
		trimmed := strings.TrimSpace(schema)
		if trimmed == "" {
			continue
		}
		normalized := strings.ToLower(trimmed)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizeCatalogColumnLength(typeName string, maxLength int) int {
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "nchar", "nvarchar":
		if maxLength < 0 {
			return maxLength
		}
		return maxLength / 2
	default:
		return maxLength
	}
}

func MapTypeDescToKind(typeDesc string) string {
	switch strings.ToUpper(strings.TrimSpace(typeDesc)) {
	case "USER_TABLE":
		return "tables"
	case "VIEW":
		return "views"
	case "SQL_STORED_PROCEDURE":
		return "procedures"
	case "SQL_SCALAR_FUNCTION", "SQL_INLINE_TABLE_VALUED_FUNCTION", "SQL_TABLE_VALUED_FUNCTION":
		return "functions"
	case "SQL_TRIGGER":
		return "triggers"
	case "INDEX":
		return "indexes"
	case "USER_TABLE_TYPE":
		return "types"
	case "SEQUENCE_OBJECT":
		return "sequences"
	case "SYNONYM":
		return "synonyms"
	default:
		return ""
	}
}

func NormalizedKey(schema string, kind string, parent string, name string) string {
	key := strings.ToLower(strings.TrimSpace(schema)) + "/" + strings.ToLower(strings.TrimSpace(kind)) + "/"
	if parent = strings.TrimSpace(parent); parent != "" && (kind == "triggers" || kind == "indexes") {
		key += strings.ToLower(parent) + "/"
	}
	key += strings.ToLower(strings.TrimSpace(name))
	return key
}
