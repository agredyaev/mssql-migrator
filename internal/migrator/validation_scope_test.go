package migrator

import (
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/validate"
)

func TestResolveValidationObjectsAffectedOnlySkipsAdoptAndUnchanged(t *testing.T) {
	layout := parser.Layout{Objects: []parser.Object{{NormalizedKey: "reporting/views/monthly", Kind: "views"}, {NormalizedKey: "reporting/views/daily", Kind: "views"}, {NormalizedKey: "reporting/tables/snapshot", Kind: "tables"}}}
	plan := contracts.MigrationPlan{Objects: []contracts.PlannedObject{{NormalizedKey: "reporting/views/monthly", PlannedAction: contracts.ActionUpdateExistingModule}, {NormalizedKey: "reporting/views/daily", PlannedAction: contracts.ActionSkipUnchanged}, {NormalizedKey: "reporting/tables/snapshot", PlannedAction: contracts.ActionAdoptExisting}}}

	got := resolveValidationObjects(plan, layout, true)
	if len(got) != 1 || got[0].NormalizedKey != "reporting/views/monthly" {
		t.Fatalf("unexpected affected validation objects: %#v", got)
	}
}

func TestManagedScopeValidationModeEnablesAffectedOnly(t *testing.T) {
	mode := managedScopeValidationMode("run-1")
	if mode.includeChecks || mode.refreshModules || !mode.affectedOnly || mode.existingRunID != "run-1" {
		t.Fatalf("unexpected managed scope validation mode: %#v", mode)
	}
}

func TestFullValidationModeUsesFullScope(t *testing.T) {
	mode := fullValidationMode()
	if !mode.includeChecks || !mode.refreshModules || mode.affectedOnly {
		t.Fatalf("unexpected full validation mode: %#v", mode)
	}
}

func TestValidationRecorderMarkSuccessesSkipsEmptyObjectList(t *testing.T) {
	execer := &stubExecer{result: stubResult{rows: 0}}
	recorder := validationRecorder{writer: newMetadataWriter(config.Config{}, execer, "run-1")}

	if err := recorder.markSuccesses(t.Context(), nil); err != nil {
		t.Fatalf("unexpected empty validation success error: %v", err)
	}
	if len(execer.calls) != 0 {
		t.Fatalf("expected no metadata writes for empty success set, got %#v", execer.calls)
	}
}

func TestValidateManagedScopeStateReturnsMissingSubset(t *testing.T) {
	layout := parser.Layout{Objects: []parser.Object{{Path: "reporting/views/monthly.sql", NormalizedKey: "reporting/views/monthly"}, {Path: "reporting/tables/snapshot.sql", NormalizedKey: "reporting/tables/snapshot"}}}
	catalogState := validate.CatalogState{Objects: map[string]validate.CatalogObject{"reporting/views/monthly": {SchemaName: "reporting", Kind: "views", ObjectName: "monthly"}}}

	scope, err := validateManagedScopeState(layout, catalogState)
	if err == nil {
		t.Fatal("expected missing managed object error")
	}
	if len(scope.Missing) != 1 || scope.Missing[0].NormalizedKey != "reporting/tables/snapshot" {
		t.Fatalf("unexpected missing validation scope: %#v", scope.Missing)
	}
}

func TestValidateScopeFullValidationKeepsRefreshEnabled(t *testing.T) {
	runner := NewRunner(config.Config{}, logger.New(logger.Options{}))
	_ = runner
	mode := fullValidationMode()
	if !mode.refreshModules {
		t.Fatal("expected full validation mode to keep module refresh enabled")
	}
}
