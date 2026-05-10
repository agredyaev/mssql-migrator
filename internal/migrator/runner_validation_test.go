package migrator

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestRunSessionFailUsesExplicitPhase(t *testing.T) {
	dir := t.TempDir()
	session := &runSession{runner: NewRunner(config.Config{ReportDir: dir}, logger.New(logger.Options{})), report: contracts.MigrationReport{StartedAt: assertStartedAt()}}

	err := session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, assertErr("boom"))
	if err == nil {
		t.Fatal("expected error")
	}
	report, readErr := contractsReadMigrationReport(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if report.Failed == nil || !containsAll(report.Failed.Error, "ERROR repair_checksum_failed:", "class=invalid input", "reason=boom") {
		t.Fatalf("expected repair checksum phase in report, got %#v", report.Failed)
	}
}

func TestPlanPrefersLayoutErrorBeforeConnectionError(t *testing.T) {
	runner := NewRunner(config.Config{
		Env:             "prod",
		Database:        "db",
		Server:          "127.0.0.1",
		Port:            "1",
		User:            "user",
		Password:        "password",
		SQLRoot:         t.TempDir(),
		SQLBase:         "missing",
		UpdatePolicy:    config.UpdatePolicyNone,
		TransactionMode: config.TransactionModeScript,
	}, logger.New(logger.Options{}))

	_, err := runner.Plan(t.Context())
	if err == nil {
		t.Fatal("expected plan error")
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if errors.Is(err, contracts.ErrConnection) {
		t.Fatalf("expected layout error before connection attempt, got %v", err)
	}
	if !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("expected layout discovery error, got %v", err)
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

	err := session.Fail("migration_failed", contracts.ErrConnection, assertErr("boom"))
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

func assertStartedAt() time.Time {
	return time.Now().UTC()
}
