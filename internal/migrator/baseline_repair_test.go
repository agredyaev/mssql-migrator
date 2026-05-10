package migrator

import (
	"fmt"
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

func TestWriteFailedMigrationRedactsSecretInFailure(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(config.Config{ReportDir: dir}, logger.New(logger.Options{}))
	err := runner.writeFailedMigration(runner.newMigrationReport(), "baseline_failed", contracts.ErrSQLExecution, fmt.Errorf("password=secret"))
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	report, readErr := contractsReadMigrationReport(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if report.Failed == nil {
		t.Fatal("expected failure report")
	}
	if report.Failed.Error == "" || containsSecret(report.Failed.Error) {
		t.Fatalf("expected redacted failure, got %q", report.Failed.Error)
	}
	if !containsAll(report.Failed.Error, "ERROR baseline_failed:", "class=sql execution failure", "sql=password=***") {
		t.Fatalf("expected failure envelope, got %q", report.Failed.Error)
	}
}

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
