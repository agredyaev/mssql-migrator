package reports

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"reporting-db-migrations/internal/contracts"
)

func TestMigrationPlanJSONSchemaUsesSnakeCase(t *testing.T) {
	plan := contracts.MigrationPlan{
		Tool:              "rmig",
		ToolVersion:       "4.0.0",
		ToolCommit:        "deadbeef",
		GitCommit:         "abc",
		GitBranch:         "main",
		SQLRoot:           "/workspace/sql",
		Base:              "dwh",
		EffectiveBasePath: "/workspace/sql/dwh",
		LayoutHash:        "hash",
		Target:            contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"},
		PlannedAt:         time.Unix(0, 0).UTC(),
	}
	content, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tool_version", "tool_commit", "git_commit", "sql_root", "base", "effective_base_path", "layout_hash", "target", "blockers"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing json key %s in %s", key, string(content))
		}
	}
	if _, ok := payload["ToolVersion"]; ok {
		t.Fatalf("unexpected Go-style key in %s", string(content))
	}
}

func TestMigrationReportJSONSchemaUsesSnakeCase(t *testing.T) {
	report := contracts.MigrationReport{Tool: "rmig", Version: "1.0.0", ToolCommit: "deadbeef", Environment: "prod", Database: "db", GitCommit: "abc", ValidationScope: "managed_scope_only", ValidationSkipped: false, PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(1, 0).UTC(), Result: "success"}
	assertJSONKeys(t, report, []string{"tool_commit", "validation_scope", "pipeline_run_id", "pipeline_url", "started_at", "finished_at"}, []string{"ToolCommit", "PipelineRunID", "PipelineURL"})
}

func TestValidationReportJSONSchemaUsesSnakeCase(t *testing.T) {
	report := contracts.ValidationReport{Tool: "rmig", Version: "1.0.0", ToolCommit: "deadbeef", Environment: "prod", Database: "db", GitCommit: "abc", Command: "validate", LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", Scope: "full_validation", IncludesChecks: true, PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(1, 0).UTC(), Result: "success"}
	assertJSONKeys(t, report, []string{"tool_commit", "git_commit", "command", "layout_hash", "sql_root", "base", "scope", "includes_checks", "pipeline_run_id", "pipeline_url", "started_at", "finished_at"}, []string{"ToolCommit", "GitCommit", "PipelineRunID", "PipelineURL", "StartedAt", "FinishedAt"})
}

func assertJSONKeys(t *testing.T, value any, required []string, forbidden []string) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range required {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing json key %s in %s", key, string(content))
		}
	}
	for _, key := range forbidden {
		if _, ok := payload[key]; ok {
			t.Fatalf("unexpected Go-style key %s in %s", key, string(content))
		}
	}
}

func TestWriteMigrationWritesAtomicFiles(t *testing.T) {
	dir := t.TempDir()
	report := contracts.MigrationReport{Tool: "rmig", Version: "1.0.0", Result: "success", StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(1, 0).UTC()}
	if err := WriteMigration(dir, report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"migration-report.json", "migration-report.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected output file %s: %v", name, err)
		}
		matches, err := filepath.Glob(filepath.Join(dir, name+".tmp-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("unexpected temp files for %s: %#v", name, matches)
		}
	}
}

func TestWriteValidationRedactsFailureText(t *testing.T) {
	dir := t.TempDir()
	report := contracts.ValidationReport{
		Tool:       "rmig",
		Version:    "1.0.0",
		Result:     "failed",
		StartedAt:  time.Unix(0, 0).UTC(),
		FinishedAt: time.Unix(1, 0).UTC(),
		Failed:     &contracts.Failure{Error: "ERROR validation_failed: reason=password=secret; sql=password=secret", Reason: "password=secret", SQL: "password=secret"},
	}
	if err := WriteValidation(dir, report); err != nil {
		t.Fatal(err)
	}
	jsonContent, err := os.ReadFile(filepath.Join(dir, "validation-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "validation-report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "password=secret") || strings.Contains(string(jsonContent), "password=secret") {
		t.Fatalf("secret leaked in text report: %s", string(content))
	}
}

func TestWritePlanRedactsBlockReasons(t *testing.T) {
	dir := t.TempDir()
	plan := contracts.MigrationPlan{SchemaVersion: "v8", Command: "plan", Failures: []string{"password=secret"}, BlockReasons: []string{"token=abc"}}
	if err := WritePlan(dir, plan); err != nil {
		t.Fatal(err)
	}
	jsonContent, err := os.ReadFile(filepath.Join(dir, "migration-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonContent), "password=secret") || strings.Contains(string(jsonContent), "token=abc") {
		t.Fatalf("secret leaked in plan report: %s", string(jsonContent))
	}
}

func TestWriteFilePairAtomicKeepsPublishedFirstFileOnSecondRenameFailure(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "pair.json")
	secondPath := filepath.Join(dir, "pair.txt")
	if err := os.WriteFile(firstPath, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondPath, 0o755); err != nil {
		t.Fatal(err)
	}
	err := writeFilePairAtomic(firstPath, []byte("{}\n"), secondPath, []byte("text\n"))
	if err == nil {
		t.Fatal("expected pair write error")
	}
	content, readErr := os.ReadFile(firstPath)
	if readErr != nil {
		t.Fatalf("expected first file to remain published, got %v", readErr)
	}
	if string(content) != "old\n" {
		t.Fatalf("expected original first file content, got %q", string(content))
	}
	if matches, globErr := filepath.Glob(filepath.Join(dir, "*.tmp-*")); globErr != nil {
		t.Fatal(globErr)
	} else if len(matches) != 0 {
		t.Fatalf("unexpected temp files after failed pair write: %#v", matches)
	}
}

func TestWriteFilePairAtomicLeavesOldJSONIfTextWriteFails(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "pair.json")
	textPath := filepath.Join(dir, "pair.txt")
	if err := os.WriteFile(jsonPath, []byte("old-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(textPath, 0o755); err != nil {
		t.Fatal(err)
	}
	err := writeFilePairAtomic(jsonPath, []byte("new-json\n"), textPath, []byte("new-text\n"))
	if err == nil {
		t.Fatal("expected pair write error")
	}
	content, readErr := os.ReadFile(jsonPath)
	if readErr != nil {
		t.Fatalf("expected previous json file, got %v", readErr)
	}
	if string(content) != "old-json\n" {
		t.Fatalf("expected old json content, got %q", string(content))
	}
}

func TestWriteTempFileRemovesTempOnSyncFailure(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing")
	path := filepath.Join(missingDir, "report.json")
	if _, err := writeTempFile(path, []byte("{}\n")); err == nil {
		t.Fatal("expected temp file error")
	}
}

func TestSyncPathDirSyncsExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncPathDir(path); err != nil {
		t.Fatalf("expected directory sync to succeed, got %v", err)
	}
}

func TestIsWindowsAccessDeniedOnlyMatchesWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !isWindowsAccessDenied(errors.New("Access is denied.")) {
			t.Fatal("expected windows access denied match")
		}
		return
	}
	if isWindowsAccessDenied(errors.New("Access is denied.")) {
		t.Fatal("did not expect access denied match on non-windows")
	}
}

func TestFormatPlanTextExplainsObjectDecisions(t *testing.T) {
	text := formatPlanText(contracts.MigrationPlan{
		SchemaVersion: "v8",
		Command:       "plan",
		Target:        contracts.PlanTarget{Environment: "pred", Database: "db"},
		Objects: []contracts.PlannedObject{
			{ObjectPath: "reporting/views/monthly.sql", Kind: "views", PlannedAction: contracts.ActionAdoptExisting},
			{ObjectPath: "reporting/views/daily.sql", Kind: "views", PlannedAction: contracts.ActionSkipUnchanged},
			{ObjectPath: "reporting/views/blocked.sql", Kind: "views", PlannedAction: contracts.ActionReprocessChangedBlocked},
		},
	})
	for _, expected := range []string{
		"Plan object decisions:",
		"adopt existing database object without DDL",
		"latest successful metadata checksum already matches the current repo SQL",
		"blocked because the object is already tracked",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected plan text to contain %q, got %s", expected, text)
		}
	}
}
