package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	mssql "github.com/microsoft/go-mssqldb"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

type baselinePreflightFailure struct {
	base       error
	class      string
	schemaName string
	object     parser.Object
}

func (e *baselinePreflightFailure) Error() string {
	switch {
	case strings.TrimSpace(e.object.Path) != "":
		return fmt.Sprintf("%s for %s", e.class, e.object.Path)
	case strings.TrimSpace(e.schemaName) != "":
		return fmt.Sprintf("%s for %s", e.class, e.schemaName)
	default:
		return e.class
	}
}

func (e *baselinePreflightFailure) Unwrap() error {
	if e.base == nil {
		return contracts.ErrSQLExecution
	}
	return e.base
}

func verifyBaselineCreatePermissions(ctx context.Context, conn *sql.Conn, plan contracts.MigrationPlan, layout parser.Layout) error {
	for _, schema := range plan.Schemas {
		if schema.Action != contracts.SchemaActionCreateSchema {
			continue
		}
		allowed, err := hasCreateSchemaPermission(ctx, conn)
		if err != nil {
			return fmt.Errorf("%w: permission preflight for schema %s: %v", contracts.ErrCriticalState, schema.SchemaName, err)
		}
		if !allowed {
			return &baselinePreflightFailure{base: contracts.ErrSQLExecution, class: "missing schema creation permission", schemaName: schema.SchemaName}
		}
	}

	objectsByKey := map[string]parser.Object{}
	for _, object := range layout.Objects {
		objectsByKey[object.NormalizedKey] = object
	}
	plannedByKey := map[string]contracts.PlannedObject{}
	for _, object := range plan.Objects {
		plannedByKey[object.NormalizedKey] = object
	}
	checkedSchemas := map[string]struct{}{}

	for _, planned := range plan.Objects {
		if planned.PlannedAction != contracts.ActionCreateObject {
			continue
		}
		object, ok := objectsByKey[planned.NormalizedKey]
		if !ok {
			continue
		}

		if object.ParentName != "" {
			parentExists, err := databaseObjectExists(ctx, conn, object.SchemaName, object.ParentName)
			if err != nil {
				return fmt.Errorf("%w: permission preflight for parent %s.%s: %v", contracts.ErrCriticalState, object.SchemaName, object.ParentName, err)
			}
			if !parentExists && !baselineParentPlanned(plannedByKey, object) {
				return &baselinePreflightFailure{base: contracts.ErrSQLExecution, class: "missing parent object", object: object}
			}
			if parentExists {
				allowed, err := hasObjectAlterPermission(ctx, conn, object.SchemaName, object.ParentName)
				if err != nil {
					return fmt.Errorf("%w: permission preflight for parent %s.%s: %v", contracts.ErrCriticalState, object.SchemaName, object.ParentName, err)
				}
				if !allowed {
					return &baselinePreflightFailure{base: contracts.ErrSQLExecution, class: "missing object DDL permission", object: object}
				}
			}
			continue
		}

		if _, seen := checkedSchemas[object.NormalizedSchemaName]; seen {
			continue
		}
		if permission := requiredDatabaseCreatePermission(object.Kind); permission != "" {
			allowed, err := hasDatabasePermission(ctx, conn, permission)
			if err != nil {
				return fmt.Errorf("%w: permission preflight for %s on %s: %v", contracts.ErrCriticalState, permission, object.Path, err)
			}
			if !allowed {
				return &baselinePreflightFailure{base: contracts.ErrSQLExecution, class: "missing object DDL permission", object: object}
			}
		}
		allowed, err := hasSchemaAlterPermission(ctx, conn, object.SchemaName)
		if err != nil {
			return fmt.Errorf("%w: permission preflight for schema %s: %v", contracts.ErrCriticalState, object.SchemaName, err)
		}
		if !allowed {
			return &baselinePreflightFailure{base: contracts.ErrSQLExecution, class: "missing object DDL permission", object: object}
		}
		checkedSchemas[object.NormalizedSchemaName] = struct{}{}
	}

	return nil
}

func baselineParentPlanned(plannedByKey map[string]contracts.PlannedObject, object parser.Object) bool {
	if strings.TrimSpace(object.ParentName) == "" {
		return true
	}
	parentName := strings.ToLower(strings.TrimSpace(object.ParentName))
	keys := []string{
		object.NormalizedSchemaName + "/tables/" + parentName,
		object.NormalizedSchemaName + "/views/" + parentName,
	}
	for _, key := range keys {
		planned, ok := plannedByKey[key]
		if !ok {
			continue
		}
		switch planned.PlannedAction {
		case contracts.ActionCreateObject, contracts.ActionAdoptExisting, contracts.ActionSkipUnchanged:
			return true
		}
	}
	return false
}

func hasCreateSchemaPermission(ctx context.Context, conn *sql.Conn) (bool, error) {
	var allowed int
	err := conn.QueryRowContext(ctx, `SELECT ISNULL(HAS_PERMS_BY_NAME(DB_NAME(), 'DATABASE', 'CREATE SCHEMA'), 0)`).Scan(&allowed)
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func hasDatabasePermission(ctx context.Context, conn *sql.Conn, permission string) (bool, error) {
	var allowed int
	err := conn.QueryRowContext(ctx, `SELECT ISNULL(HAS_PERMS_BY_NAME(DB_NAME(), 'DATABASE', @p1), 0)`, permission).Scan(&allowed)
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func hasSchemaAlterPermission(ctx context.Context, conn *sql.Conn, schemaName string) (bool, error) {
	var allowed int
	err := conn.QueryRowContext(ctx, `SELECT ISNULL(HAS_PERMS_BY_NAME(@p1, 'SCHEMA', 'ALTER'), 0)`, schemaName).Scan(&allowed)
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func hasObjectAlterPermission(ctx context.Context, conn *sql.Conn, schemaName string, objectName string) (bool, error) {
	qualified := schemaName + "." + objectName
	var allowed int
	err := conn.QueryRowContext(ctx, `SELECT ISNULL(HAS_PERMS_BY_NAME(@p1, 'OBJECT', 'ALTER'), 0)`, qualified).Scan(&allowed)
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func databaseObjectExists(ctx context.Context, conn *sql.Conn, schemaName string, objectName string) (bool, error) {
	var exists int
	err := conn.QueryRowContext(ctx, `
SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM sys.objects o
    JOIN sys.schemas s ON s.schema_id = o.schema_id
    WHERE o.is_ms_shipped = 0 AND s.name = @p1 AND o.name = @p2
) THEN 1 ELSE 0 END`, schemaName, objectName).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func classifySchemaExecutionError(schemaName string, err error) error {
	if isPermissionDeniedError(err) {
		return fmt.Errorf("%w: missing schema creation permission for %s: %v", contracts.ErrSQLExecution, schemaName, err)
	}
	return fmt.Errorf("%w: create schema %s: %v", contracts.ErrSQLExecution, schemaName, err)
}

func classifyObjectExecutionError(object parser.Object, planned contracts.PlannedObject, err error) error {
	if planned.PlannedAction == contracts.ActionCreateObject && looksLikeMissingParentError(err) {
		return fmt.Errorf("%w: missing parent object for %s: %v", contracts.ErrSQLExecution, object.Path, err)
	}
	if isPermissionDeniedError(err) {
		return fmt.Errorf("%w: missing object DDL permission for %s: %v", contracts.ErrSQLExecution, object.Path, err)
	}
	if planned.PlannedAction == contracts.ActionCreateObject {
		return fmt.Errorf("%w: create object %s: %v", contracts.ErrSQLExecution, object.Path, err)
	}
	return fmt.Errorf("%w: %v", contracts.ErrSQLExecution, err)
}

func isPermissionDeniedError(err error) bool {
	if err == nil {
		return false
	}
	var sqlErr mssql.Error
	if errors.As(err, &sqlErr) {
		switch sqlErr.SQLErrorNumber() {
		case 229, 262:
			return true
		}
		if containsPermissionMarker(sqlErr.SQLErrorMessage()) {
			return true
		}
	}
	return containsPermissionMarker(err.Error())
}

func looksLikeMissingParentError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "cannot find the object") ||
		strings.Contains(lower, "could not find object") ||
		strings.Contains(lower, "could not find the object") ||
		strings.Contains(lower, "references invalid object")
}

func containsPermissionMarker(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "permission was denied") ||
		strings.Contains(lower, "do not have permission") ||
		strings.Contains(lower, "does not have permission") ||
		strings.Contains(lower, "not authorized")
}

func requiredDatabaseCreatePermission(kind string) string {
	switch kind {
	case "tables":
		return "CREATE TABLE"
	case "views":
		return "CREATE VIEW"
	case "procedures":
		return "CREATE PROCEDURE"
	case "functions":
		return "CREATE FUNCTION"
	case "types":
		return "CREATE TYPE"
	case "sequences":
		return "CREATE SEQUENCE"
	case "synonyms":
		return "CREATE SYNONYM"
	default:
		return ""
	}
}
