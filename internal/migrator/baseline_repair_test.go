package migrator

import (
	"fmt"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

func TestRequireConfirmation(t *testing.T) {
	runner := NewRunner(config.Config{}, logger.New(logger.Options{}))
	err := runner.requireConfirmation()
	if err == nil || err.Error() == "" {
		t.Fatal("expected confirmation error")
	}
}

func TestWriteFailedMigrationRedactsSecretInFailure(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(config.Config{ReportDir: dir}, logger.New(logger.Options{}))
	err := runner.writeFailedMigration(runner.newMigrationReport(), contracts.ErrSQLExecution, fmt.Errorf("password=secret"))
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
	if !containsAll(report.Failed.Error, "ERROR migration_failed:", "class=sql execution failure", "sql=password=***") {
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
