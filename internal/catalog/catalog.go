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

type TableRef struct {
	SchemaName string
	TableName  string
}

type ObjectRef struct {
	SchemaName string
	Kind       string
	ParentName string
	ObjectName string
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
	return ReadSchemaObjectsForSchemas(ctx, conn, nil)
}

func ReadForSchemas(ctx context.Context, conn *sql.Conn, schemas []string) (State, error) {
	return ReadSchemaObjectsForSchemas(ctx, conn, schemas)
}

func ReadForLayout(ctx context.Context, conn *sql.Conn, schemas []string, objects []ObjectRef) (State, error) {
	cleanedObjects := normalizedObjectRefs(objects)
	if len(cleanedObjects) == 0 {
		return ReadSchemaObjectsForSchemas(ctx, conn, schemas)
	}
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

	stateQuery, stateArgs := stateQueryForObjects(cleanedObjects)
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
	return state, nil
}

func ReadSchemaObjectsForSchemas(ctx context.Context, conn *sql.Conn, schemas []string) (State, error) {
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
	return state, nil
}

func ReadColumnsForTables(ctx context.Context, conn *sql.Conn, tables []TableRef) (map[string][]TableColumn, error) {
	if conn == nil {
		return nil, contracts.Wrap(contracts.ErrCriticalState, fmt.Errorf("catalog read: missing database connection"))
	}
	state := map[string][]TableColumn{}
	columnQuery, columnArgs := tableColumnsQueryForTables(tables)
	if strings.TrimSpace(columnQuery) == "" {
		return state, nil
	}
	columnRows, err := conn.QueryContext(ctx, columnQuery, columnArgs...)
	if err != nil {
		return nil, contracts.Wrap(contracts.ErrCriticalState, err)
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
			return nil, contracts.Wrap(contracts.ErrCriticalState, err)
		}
		key := NormalizedKey(schemaName, "tables", "", tableName)
		state[key] = append(state[key], TableColumn{
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
		return nil, contracts.Wrap(contracts.ErrCriticalState, err)
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

func stateQueryForObjects(objects []ObjectRef) (string, []any) {
	cleaned := normalizedObjectRefs(objects)
	if len(cleaned) == 0 {
		return stateQueryForSchemas(nil)
	}
	objectClause, objectArgs, next := objectFilterClauseAt("s.name", "o.type_desc", "parent.name", "o.name", cleaned, 1)
	typeClause, typeArgs, next := objectFilterClauseAt("s.name", "'USER_TABLE_TYPE'", "''", "tt.name", cleaned, next)
	indexClause, indexArgs, _ := objectFilterClauseAt("s.name", "'INDEX'", "o.name", "i.name", cleaned, next)
	args := append(objectArgs, typeArgs...)
	args = append(args, indexArgs...)
	return fmt.Sprintf(stateQueryTemplate, objectClause, typeClause, indexClause), args
}

func tableColumnsQueryForTables(tables []TableRef) (string, []any) {
	cleaned := normalizedTableRefs(tables)
	if len(cleaned) == 0 {
		return "", nil
	}
	clauses := make([]string, 0, len(cleaned))
	args := make([]any, 0, len(cleaned)*2)
	for i, table := range cleaned {
		schemaPlaceholder := fmt.Sprintf("@p%d", i*2+1)
		tablePlaceholder := fmt.Sprintf("@p%d", i*2+2)
		clauses = append(clauses, fmt.Sprintf("(s.name = %s AND t.name = %s)", schemaPlaceholder, tablePlaceholder))
		args = append(args, table.SchemaName, table.TableName)
	}
	return fmt.Sprintf(tableColumnsQueryTemplate, " AND ("+strings.Join(clauses, " OR ")+")"), args
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

func normalizedTableRefs(tables []TableRef) []TableRef {
	if len(tables) == 0 {
		return nil
	}
	result := make([]TableRef, 0, len(tables))
	seen := map[string]struct{}{}
	for _, table := range tables {
		schema := strings.TrimSpace(table.SchemaName)
		name := strings.TrimSpace(table.TableName)
		if schema == "" || name == "" {
			continue
		}
		key := strings.ToLower(schema) + "/" + strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, TableRef{SchemaName: schema, TableName: name})
	}
	return result
}

func objectFilterClauseAt(schemaColumn string, kindColumn string, parentColumn string, nameColumn string, objects []ObjectRef, start int) (string, []any, int) {
	cleaned := normalizedObjectRefs(objects)
	if len(cleaned) == 0 {
		return "", nil, start
	}
	clauses := make([]string, 0, len(cleaned))
	args := make([]any, 0, len(cleaned)*4)
	next := start
	for _, object := range cleaned {
		schemaPlaceholder := fmt.Sprintf("@p%d", next)
		kindPlaceholder := fmt.Sprintf("@p%d", next+1)
		parentPlaceholder := fmt.Sprintf("@p%d", next+2)
		namePlaceholder := fmt.Sprintf("@p%d", next+3)
		clauses = append(clauses, fmt.Sprintf("(%s = %s AND UPPER(%s) = %s AND ISNULL(%s, '') = %s AND %s = %s)", schemaColumn, schemaPlaceholder, kindColumn, kindPlaceholder, parentColumn, parentPlaceholder, nameColumn, namePlaceholder))
		args = append(args, object.SchemaName, objectTypeDescFilterValue(object.Kind), object.ParentName, object.ObjectName)
		next += 4
	}
	return " AND (" + strings.Join(clauses, " OR ") + ")", args, next
}

func normalizedObjectRefs(objects []ObjectRef) []ObjectRef {
	if len(objects) == 0 {
		return nil
	}
	result := make([]ObjectRef, 0, len(objects))
	seen := map[string]struct{}{}
	for _, object := range objects {
		schema := strings.TrimSpace(object.SchemaName)
		kind := strings.TrimSpace(object.Kind)
		name := strings.TrimSpace(object.ObjectName)
		parent := strings.TrimSpace(object.ParentName)
		if schema == "" || kind == "" || name == "" {
			continue
		}
		key := strings.ToLower(schema) + "/" + strings.ToLower(kind) + "/" + strings.ToLower(parent) + "/" + strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ObjectRef{SchemaName: schema, Kind: kind, ParentName: parent, ObjectName: name})
	}
	return result
}

func objectTypeDescFilterValue(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "tables":
		return "USER_TABLE"
	case "views":
		return "VIEW"
	case "procedures":
		return "SQL_STORED_PROCEDURE"
	case "functions":
		return "SQL_SCALAR_FUNCTION"
	case "triggers":
		return "SQL_TRIGGER"
	case "indexes":
		return "INDEX"
	case "types":
		return "USER_TABLE_TYPE"
	case "sequences":
		return "SEQUENCE_OBJECT"
	case "synonyms":
		return "SYNONYM"
	default:
		return strings.ToUpper(strings.TrimSpace(kind))
	}
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
