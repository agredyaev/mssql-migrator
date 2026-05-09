package validate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

type objectRef struct {
	Schema string
	Name   string
	Kind   string
}

type CatalogState struct {
	Schemas map[string]struct{}
	Objects map[string]CatalogObject
}

type CatalogObject struct {
	SchemaName string
	Kind       string
	ObjectName string
	ParentName string
}

const catalogStateQuery = `
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

func ReadCatalogState(ctx context.Context, conn *sql.Conn) (CatalogState, error) {
	state := CatalogState{
		Schemas: map[string]struct{}{},
		Objects: map[string]CatalogObject{},
	}
	if conn == nil {
		return state, nil
	}
	schemaRows, err := conn.QueryContext(ctx, `SELECT name FROM sys.schemas WHERE name NOT IN ('sys', 'INFORMATION_SCHEMA') ORDER BY name`)
	if err != nil {
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	for schemaRows.Next() {
		var schemaName string
		if err := schemaRows.Scan(&schemaName); err != nil {
			schemaRows.Close()
			return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
		}
		state.Schemas[strings.ToLower(schemaName)] = struct{}{}
	}
	if err := schemaRows.Err(); err != nil {
		schemaRows.Close()
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	schemaRows.Close()

	rows, err := conn.QueryContext(ctx, catalogStateQuery)
	if err != nil {
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	defer rows.Close()
	for rows.Next() {
		var schemaName string
		var typeDesc string
		var objectName string
		var parentName string
		if err := rows.Scan(&schemaName, &typeDesc, &objectName, &parentName); err != nil {
			return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
		}
		kind := mapTypeDescToKind(typeDesc)
		if kind == "" {
			continue
		}
		key := strings.ToLower(schemaName) + "/" + kind + "/"
		if parentName != "" && (kind == "triggers" || kind == "indexes") {
			key += strings.ToLower(parentName) + "/"
		}
		key += strings.ToLower(objectName)
		state.Objects[key] = CatalogObject{SchemaName: schemaName, Kind: kind, ObjectName: objectName, ParentName: parentName}
	}
	if err := rows.Err(); err != nil {
		return CatalogState{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	return state, nil
}

func RefreshManagedObjects(ctx context.Context, conn *sql.Conn, layout parser.Layout, log logger.Logger) (contracts.ValidationSummary, error) {
	catalog, err := ReadCatalogState(ctx, conn)
	if err != nil {
		return contracts.ValidationSummary{}, err
	}
	objects, missing := managedScopeRefs(layout.Objects, catalog.Objects)

	summary := contracts.ValidationSummary{}
	if len(missing) > 0 {
		return summary, fmt.Errorf("missing managed objects: %s", strings.Join(missing, ", "))
	}
	for _, expected := range layout.Objects {
		if !parser.IsModuleKind(expected.Kind) {
			continue
		}
		object := objects[expected.NormalizedKey]
		qualifiedName := bracket(object.Schema) + "." + bracket(object.Name)
		if _, err := conn.ExecContext(ctx, "EXEC sys.sp_refreshsqlmodule @p1", qualifiedName); err != nil {
			return summary, fmt.Errorf("refresh module %s: %w", qualifiedName, err)
		}
		if object.Kind == "views" {
			if _, err := conn.ExecContext(ctx, "EXEC sys.sp_refreshview @p1", qualifiedName); err != nil {
				return summary, fmt.Errorf("refresh view %s: %w", qualifiedName, err)
			}
		}
		summary.ModulesRefreshed++
	}

	log.Info("modules_refreshed", fmt.Sprintf("count=%d", summary.ModulesRefreshed))
	return summary, nil
}

func RunChecks(ctx context.Context, conn *sql.Conn, layout parser.Layout) (contracts.ValidationSummary, error) {
	summary := contracts.ValidationSummary{}
	for _, script := range layout.Checks {
		if err := runCheck(ctx, conn, script); err != nil {
			summary.ChecksFailed++
			return summary, fmt.Errorf("check %s failed: %w", script.Path, err)
		}
		summary.ChecksPassed++
	}
	return summary, nil
}

func LoadLayout(cfg config.Config) (parser.Layout, error) {
	return parser.DiscoverValidationLayout(cfg.SelectedBasePath())
}

func runCheck(ctx context.Context, conn *sql.Conn, script parser.CheckScript) error {
	batches, err := parser.SplitGO(script.Content)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		for i := 0; i < batch.Repeat; i++ {
			if _, err := conn.ExecContext(ctx, batch.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func managedScopeRefs(expected []parser.Object, actual map[string]CatalogObject) (map[string]objectRef, []string) {
	result := map[string]objectRef{}
	missing := []string{}
	for _, object := range expected {
		catalogObject, ok := actual[object.NormalizedKey]
		if !ok {
			missing = append(missing, object.Path)
			continue
		}
		result[object.NormalizedKey] = objectRef{
			Schema: catalogObject.SchemaName,
			Name:   catalogObject.ObjectName,
			Kind:   catalogObject.Kind,
		}
	}
	return result, missing
}

func bracket(value string) string {
	return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
}

func mapTypeDescToKind(typeDesc string) string {
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
