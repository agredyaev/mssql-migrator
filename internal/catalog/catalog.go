package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/contracts"
)

type State struct {
	Schemas map[string]struct{}
	Objects map[string]Object
}

type Object struct {
	SchemaName string
	Kind       string
	ObjectName string
	ParentName string
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

func Read(ctx context.Context, conn *sql.Conn) (State, error) {
	state := State{
		Schemas: map[string]struct{}{},
		Objects: map[string]Object{},
	}
	if conn == nil {
		return State{}, contracts.Wrap(contracts.ErrCriticalState, fmt.Errorf("catalog read: missing database connection"))
	}
	schemaRows, err := conn.QueryContext(ctx, `SELECT name FROM sys.schemas WHERE name NOT IN ('sys', 'INFORMATION_SCHEMA') ORDER BY name`)
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

	rows, err := conn.QueryContext(ctx, StateQuery)
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
