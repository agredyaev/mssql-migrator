package runreport_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/runreport"
)

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestWriteMigrationFailureRedactsSecretInFailure(t *testing.T) {
	dir := t.TempDir()
	err := runreport.WriteMigrationFailure(config.Config{ReportDir: dir}, contracts.MigrationReport{StartedAt: time.Now().UTC()}, "baseline_failed", contracts.ErrSQLExecution, assertErr("password=secret"))
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !errors.Is(err, contracts.ErrSQLExecution) {
		t.Fatalf("expected sql execution sentinel, got %v", err)
	}
	report := readMigrationReport(t, dir)
	if report.Failed == nil {
		t.Fatal("expected failure report")
	}
	if report.Failed.Error == "" || strings.Contains(report.Failed.Error, "secret") {
		t.Fatalf("expected redacted failure, got %q", report.Failed.Error)
	}
	if !containsAll(report.Failed.Error, "ERROR baseline_failed:", "class=sql execution failure", "sql=password=***") {
		t.Fatalf("expected failure envelope, got %q", report.Failed.Error)
	}
}

func TestFinalizeValidationFailureFromReportUsesProvidedClassification(t *testing.T) {
	report := runreport.FinalizeValidationFailureFromReport(contracts.ValidationReport{SQLRoot: "/sql", Base: "dwh"}, runreport.ValidationFailurePhase, contracts.ErrCriticalState, assertErr("boom"))
	if report.Failed == nil {
		t.Fatal("expected failure payload")
	}
	if report.Failed.Class != "critical metadata state" {
		t.Fatalf("expected critical metadata state class, got %#v", report.Failed)
	}
	if report.Failed.Class == "validation failure" {
		t.Fatalf("expected non-validation class, got %#v", report.Failed)
	}
}

func TestFinalizeMigrationSuccessSetsResultAndDuration(t *testing.T) {
	report := contracts.MigrationReport{StartedAt: time.Now().UTC().Add(-2 * time.Second), Result: "running"}
	runreport.FinalizeMigrationSuccess(&report)
	if report.Result != "success" {
		t.Fatalf("expected success result, got %#v", report)
	}
	if report.FinishedAt.IsZero() {
		t.Fatalf("expected finished time, got %#v", report)
	}
	if report.DurationMS <= 0 {
		t.Fatalf("expected positive duration, got %#v", report)
	}
}

func TestWriteValidationOutcomeWritesReportAndReturnsValidationError(t *testing.T) {
	dir := t.TempDir()
	report := contracts.ValidationReport{
		StartedAt: time.Now().UTC(),
		Result:    "failed",
		Failed:    &contracts.Failure{Error: "ERROR validation_failed: sql_root=/sql base=dwh path=- class=validation failure reason=boom; sql=boom"},
	}
	err := runreport.WriteValidationOutcome(dir, report, contracts.Wrap(contracts.ErrValidation, assertErr("boom")))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, contracts.ErrValidation) {
		t.Fatalf("expected validation sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped validation cause, got %v", err)
	}
	written := readValidationReport(t, dir)
	if written.Result != "failed" || written.Failed == nil || written.Failed.Error != report.Failed.Error {
		t.Fatalf("unexpected written validation report: %#v", written)
	}
}

func TestWriteValidationOutcomePreservesCriticalStateClassification(t *testing.T) {
	dir := t.TempDir()
	report := contracts.ValidationReport{StartedAt: time.Now().UTC(), Result: "failed"}
	err := runreport.WriteValidationOutcome(dir, report, contracts.Wrap(contracts.ErrCriticalState, assertErr("boom")))
	if err == nil {
		t.Fatal("expected validation outcome error")
	}
	if !errors.Is(err, contracts.ErrCriticalState) {
		t.Fatalf("expected critical state sentinel, got %v", err)
	}
}

func TestReturnFailureWrapsDistinctBaseAndCause(t *testing.T) {
	err := runreport.ReturnFailure(contracts.ErrConnection, assertErr("boom"))
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !errors.Is(err, contracts.ErrConnection) {
		t.Fatalf("expected connection sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped cause, got %v", err)
	}
}

func readMigrationReport(t *testing.T, dir string) contracts.MigrationReport {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, "migration-report.json"))
	if err != nil {
		t.Fatalf("read migration report: %v", err)
	}
	var report contracts.MigrationReport
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("unmarshal migration report: %v", err)
	}
	return report
}

func readValidationReport(t *testing.T, dir string) contracts.ValidationReport {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, "validation-report.json"))
	if err != nil {
		t.Fatalf("read validation report: %v", err)
	}
	var report contracts.ValidationReport
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("unmarshal validation report: %v", err)
	}
	return report
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
