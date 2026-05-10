package migrator

import (
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

func TestResolveRepairObjectAcceptsObjectPath(t *testing.T) {
	layout := parser.Layout{Objects: []parser.Object{{Path: "reporting/views/monthly.sql", NormalizedKey: "reporting/views/monthly"}}}
	object, err := resolveRepairObject(layout, "reporting/views/monthly.sql")
	if err != nil {
		t.Fatal(err)
	}
	if object.NormalizedKey != "reporting/views/monthly" {
		t.Fatalf("unexpected repair object: %#v", object)
	}
}

func TestResolveRepairObjectAcceptsNormalizedKey(t *testing.T) {
	layout := parser.Layout{Objects: []parser.Object{{Path: "reporting/views/monthly.sql", NormalizedKey: "reporting/views/monthly"}}}
	object, err := resolveRepairObject(layout, "reporting/views/monthly")
	if err != nil {
		t.Fatal(err)
	}
	if object.Path != "reporting/views/monthly.sql" {
		t.Fatalf("unexpected repair object: %#v", object)
	}
}

func TestResolveRepairObjectFailsWhenTargetMissing(t *testing.T) {
	layout := parser.Layout{Objects: []parser.Object{{Path: "reporting/views/monthly.sql", NormalizedKey: "reporting/views/monthly"}}}
	if _, err := resolveRepairObject(layout, "reporting/views/daily.sql"); err == nil {
		t.Fatal("expected repair target lookup to fail")
	}
}

func TestNewMigrationReportRedactsPipelineURL(t *testing.T) {
	runner := NewRunner(config.Config{ToolVersion: "1.0.0", Env: "prod", Database: "db", PipelineURL: "https://ci.example/run?token=abc123&sig=xyz987"}, logger.New(logger.Options{}))
	report := runner.newMigrationReport()
	if containsAny(report.PipelineURL, []string{"token=abc123", "sig=xyz987"}) {
		t.Fatalf("pipeline url was not redacted: %s", report.PipelineURL)
	}
}

func TestResolveRepairPlanObjectFindsTarget(t *testing.T) {
	object, err := resolveRepairPlanObject(contracts.MigrationPlan{Objects: []contracts.PlannedObject{{NormalizedKey: "reporting/views/monthly", PlannedAction: contracts.ActionUpdateExistingModule}}}, "reporting/views/monthly")
	if err != nil {
		t.Fatal(err)
	}
	if object.PlannedAction != contracts.ActionUpdateExistingModule {
		t.Fatalf("unexpected repair plan object: %#v", object)
	}
}

func TestValidateRepairEligibilityAllowsChangedTrackedObject(t *testing.T) {
	err := validateRepairEligibility(parser.Object{Path: "reporting/views/monthly.sql"}, contracts.PlannedObject{PlannedAction: contracts.ActionUpdateExistingModule})
	if err != nil {
		t.Fatalf("expected changed tracked object to be eligible, got %v", err)
	}
}

func TestValidateRepairEligibilityRejectsUnchangedObject(t *testing.T) {
	err := validateRepairEligibility(parser.Object{Path: "reporting/views/monthly.sql"}, contracts.PlannedObject{PlannedAction: contracts.ActionSkipUnchanged})
	if err == nil || !strings.Contains(err.Error(), "repair-checksum is not needed") {
		t.Fatal("expected unchanged object to be rejected")
	}
}

func TestValidateRepairEligibilityRejectsAdoptExistingObject(t *testing.T) {
	err := validateRepairEligibility(parser.Object{Path: "reporting/views/monthly.sql"}, contracts.PlannedObject{PlannedAction: contracts.ActionAdoptExisting})
	if err == nil || !strings.Contains(err.Error(), "use baseline or migrate to adopt it first") {
		t.Fatal("expected adopt_existing object to be rejected")
	}
}

func TestBaselineDriftFailureForTransitionBackedTableUsesMigrateMessage(t *testing.T) {
	err := baselineDriftFailure(contracts.PlannedObject{
		ObjectPath:      "reporting/tables/snapshot.sql",
		SchemaName:      "reporting",
		ObjectName:      "snapshot",
		Kind:            "tables",
		TransitionPaths: []string{"reporting/tables/_migrations/snapshot/001_deadbee_expand_snapshot.sql"},
	})
	if err == nil || !strings.Contains(err.Error(), "use migrate to apply checked-in transitions") || !strings.Contains(err.Error(), "001_deadbee_expand_snapshot.sql") {
		t.Fatalf("expected transition-backed baseline drift message, got %v", err)
	}
}

func TestValidateRepairEligibilityRejectsTransitionBackedTableUpdate(t *testing.T) {
	err := validateRepairEligibility(parser.Object{Path: "reporting/tables/snapshot.sql"}, contracts.PlannedObject{
		Kind:            "tables",
		PlannedAction:   contracts.ActionReprocessChanged,
		TransitionPaths: []string{"reporting/tables/_migrations/snapshot/001_deadbee_expand_snapshot.sql"},
	})
	if err == nil || !strings.Contains(err.Error(), "use migrate instead") {
		t.Fatalf("expected transition-backed table update to require migrate, got %v", err)
	}
}
