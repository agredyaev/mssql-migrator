package migrator

import (
	"fmt"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/state"
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
}

func TestSelectBaselineScriptsSkipsAlreadyApplied(t *testing.T) {
	scripts := []parser.Script{{Name: "V001__one.sql", Version: "001", Type: parser.ScriptTypeVersioned, Checksum: "sum1"}, {Name: "V002__two.sql", Version: "002", Type: parser.ScriptTypeVersioned, Checksum: "sum2"}}
	migrationState := state.New([]state.Attempt{{ScriptName: "V001__one.sql", Checksum: "sum1", Success: true}})
	toApply, skipped, err := selectBaselineScripts(scripts, migrationState, "002")
	if err != nil {
		t.Fatal(err)
	}
	if len(toApply) != 1 || toApply[0].Name != "V002__two.sql" {
		t.Fatalf("unexpected baseline apply set: %#v", toApply)
	}
	if len(skipped) != 1 || skipped[0].Script != "V001__one.sql" {
		t.Fatalf("unexpected baseline skipped set: %#v", skipped)
	}
}

func TestSelectBaselineScriptsFailsOnChecksumMismatch(t *testing.T) {
	scripts := []parser.Script{{Name: "V001__one.sql", Version: "001", Type: parser.ScriptTypeVersioned, Checksum: "sum1"}}
	migrationState := state.New([]state.Attempt{{ScriptName: "V001__one.sql", Checksum: "old", Success: true}})
	_, _, err := selectBaselineScripts(scripts, migrationState, "001")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestRepairSuccessfulChecksumUpdatesLatestSuccessfulRowOnly(t *testing.T) {
	execer := &stubExecer{result: stubResult{rows: 1}}
	rows, err := repairSuccessfulChecksum(t.Context(), execer, "R001__views.sql", "newsum")
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("unexpected rows affected: %d", rows)
	}
	if len(execer.calls) != 1 {
		t.Fatalf("expected one exec call, got %d", len(execer.calls))
	}
	call := execer.calls[0]
	if !containsAll(call.query, "UPDATE __migrator.schema_migrations", "SELECT TOP (1) id", "ORDER BY applied_at DESC, id DESC") {
		t.Fatalf("unexpected repair query: %s", call.query)
	}
	if len(call.args) != 2 || call.args[0] != "newsum" || call.args[1] != "R001__views.sql" {
		t.Fatalf("unexpected repair args: %#v", call.args)
	}
}

func TestNewMigrationReportRedactsPipelineURL(t *testing.T) {
	runner := NewRunner(config.Config{ToolVersion: "1.0.0", Env: "prod", Database: "db", PipelineURL: "https://ci.example/run?token=abc123&sig=xyz987"}, logger.New(logger.Options{}))
	report := runner.newMigrationReport()
	if containsAny(report.PipelineURL, []string{"token=abc123", "sig=xyz987"}) {
		t.Fatalf("pipeline url was not redacted: %s", report.PipelineURL)
	}
}
