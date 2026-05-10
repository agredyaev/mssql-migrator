package migrator

import (
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
	"reporting-db-migrations/internal/validate"
)

func TestScopeWriterMigrationRequiresConnection(t *testing.T) {
	s := scopeWriter{writer: newMetadataWriter(config.Config{}, &stubExecer{result: stubResult{rows: 1}}, "run-1")}
	_, err := s.Migration(t.Context(), contracts.MigrationPlan{})
	if err == nil || !strings.Contains(err.Error(), "persist migration scope: missing metadata connection") {
		t.Fatalf("expected missing connection error, got %v", err)
	}
}

func TestScopeWriterValidationRequiresConnection(t *testing.T) {
	s := scopeWriter{writer: newMetadataWriter(config.Config{}, &stubExecer{result: stubResult{rows: 1}}, "run-1")}
	_, err := s.Validation(t.Context(), parser.Layout{}, validate.CatalogState{}, nil)
	if err == nil || !strings.Contains(err.Error(), "persist validation scope: missing metadata connection") {
		t.Fatalf("expected missing connection error, got %v", err)
	}
}

func TestScopeWriterRepairRequiresConnection(t *testing.T) {
	s := scopeWriter{writer: newMetadataWriter(config.Config{}, &stubExecer{result: stubResult{rows: 1}}, "run-1")}
	_, err := s.Repair(t.Context(), parser.Object{}, contracts.PlannedObject{}, planner.CatalogState{}, "")
	if err == nil || !strings.Contains(err.Error(), "persist repair scope: missing metadata connection") {
		t.Fatalf("expected missing connection error, got %v", err)
	}
}
