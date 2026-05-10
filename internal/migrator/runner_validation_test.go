package migrator

import (
	"errors"
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

func TestValidateWritesFailureReportOnPreflightError(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(config.Config{ReportDir: dir, Env: "prod", Database: "db", SQLRoot: t.TempDir(), SQLBase: "missing", UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript}, logger.New(logger.Options{}))
	err := runner.Validate(t.Context())
	if err == nil {
		t.Fatal("expected validate error")
	}
	report, readErr := contractsReadValidationReport(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if report.Result != "failed" || report.Failed == nil {
		t.Fatalf("expected failed validation report, got %#v", report)
	}
	if !containsAll(report.Failed.Error, "ERROR validation_failed:", "class=invalid input") {
		t.Fatalf("expected validation envelope, got %#v", report.Failed)
	}
}

func TestMigrateFailsBeforeDBWorkWhenPlanMissing(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	createRepoObject(t, root, base, "reporting", "views", "monthly.sql", "SELECT 1;")
	dir := t.TempDir()
	runner := NewRunner(config.Config{ReportDir: dir, Env: "prod", Database: "db", SQLRoot: root, SQLBase: base, UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript}, logger.New(logger.Options{}))
	err := runner.Migrate(t.Context())
	if err == nil || !strings.Contains(err.Error(), "--plan-file is required") {
		t.Fatalf("expected plan-file error, got %v", err)
	}
	report, readErr := contractsReadMigrationReport(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if report.Failed == nil {
		t.Fatalf("expected migration failure report, got %#v", report)
	}
	if !containsAll(report.Failed.Error, "ERROR migration_failed:", "class=invalid input", "reason=--plan-file is required") {
		t.Fatalf("expected migration envelope, got %#v", report.Failed)
	}
}

func TestWriteValidationFailureReportIncludesScopeMetadata(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(config.Config{ReportDir: dir, Env: "prod", Database: "db", SQLRoot: "/sql", SQLBase: "dwh"}, logger.New(logger.Options{}))
	err := runner.writeValidationFailureReport(assertErr("boom"), nil)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected original error, got %v", err)
	}
	report, readErr := contractsReadValidationReport(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if report.Command != "validate" || report.Scope != "full_validation" || !report.IncludesChecks {
		t.Fatalf("unexpected validation failure scope metadata: %#v", report)
	}
	if report.Failed == nil || !containsAll(report.Failed.Error, "ERROR validation_failed:", "reason=boom") {
		t.Fatalf("expected validation envelope, got %#v", report.Failed)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestRunSessionFailIsNilSafe(t *testing.T) {
	var session *runSession

	err := session.Fail(contracts.ErrConnection, assertErr("boom"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, contracts.ErrConnection) {
		t.Fatalf("expected connection sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped cause, got %v", err)
	}
}
