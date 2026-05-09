package migrator

import (
	"fmt"
	"strings"
	"testing"

	mssql "github.com/microsoft/go-mssqldb"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

func TestClassifySchemaExecutionErrorReportsPermissionFailure(t *testing.T) {
	err := classifySchemaExecutionError("reporting", mssql.Error{Number: 229, Message: "The CREATE SCHEMA permission was denied in database 'db'."})
	if err == nil || !strings.Contains(err.Error(), "missing schema creation permission") {
		t.Fatalf("expected schema permission classification, got %v", err)
	}
	if !strings.Contains(err.Error(), "reporting") {
		t.Fatalf("expected schema name in error, got %v", err)
	}
	if !strings.Contains(err.Error(), contracts.ErrSQLExecution.Error()) {
		t.Fatalf("expected sql execution wrapper, got %v", err)
	}
}

func TestClassifyObjectExecutionErrorReportsPermissionFailure(t *testing.T) {
	object := parser.Object{Path: "reporting/views/monthly.sql"}
	planned := contracts.PlannedObject{PlannedAction: contracts.ActionCreateObject}
	err := classifyObjectExecutionError(object, planned, mssql.Error{Number: 262, Message: "CREATE VIEW permission denied."})
	if err == nil || !strings.Contains(err.Error(), "missing object DDL permission") {
		t.Fatalf("expected object permission classification, got %v", err)
	}
}

func TestClassifyObjectExecutionErrorReportsMissingParent(t *testing.T) {
	object := parser.Object{Path: "reporting/indexes/snapshot/ix_snapshot.sql"}
	planned := contracts.PlannedObject{PlannedAction: contracts.ActionCreateObject}
	err := classifyObjectExecutionError(object, planned, fmt.Errorf("Cannot find the object 'snapshot' because it does not exist or you do not have permissions."))
	if err == nil || !strings.Contains(err.Error(), "missing parent object") {
		t.Fatalf("expected missing parent classification, got %v", err)
	}
}
