package reports

import (
	"encoding/json"
	"testing"
	"time"

	"reporting-db-migrations/internal/contracts"
)

func TestMigrationPlanJSONSchemaUsesSnakeCase(t *testing.T) {
	plan := contracts.MigrationPlan{
		Tool:           "rmig",
		ToolVersion:    "4.0.0",
		ToolCommit:     "deadbeef",
		GitCommit:      "abc",
		GitBranch:      "main",
		SQLDirHash:     "hash",
		TargetEnv:      "prod",
		TargetDatabase: "ReportingDB",
		PlannedAt:      time.Unix(0, 0).UTC(),
	}
	content, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tool_version", "tool_commit", "git_commit", "sql_dir_hash", "target_env", "target_database"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing json key %s in %s", key, string(content))
		}
	}
	if _, ok := payload["ToolVersion"]; ok {
		t.Fatalf("unexpected Go-style key in %s", string(content))
	}
}

func TestMigrationReportJSONSchemaUsesSnakeCase(t *testing.T) {
	report := contracts.MigrationReport{Tool: "rmig", Version: "1.0.0", ToolCommit: "deadbeef", Environment: "prod", Database: "db", GitCommit: "abc", PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(1, 0).UTC(), Result: "success"}
	assertJSONKeys(t, report, []string{"tool_commit", "pipeline_run_id", "pipeline_url", "started_at", "finished_at"}, []string{"ToolCommit", "PipelineRunID", "PipelineURL"})
}

func TestValidationReportJSONSchemaUsesSnakeCase(t *testing.T) {
	report := contracts.ValidationReport{Tool: "rmig", Version: "1.0.0", ToolCommit: "deadbeef", Environment: "prod", Database: "db", GitCommit: "abc", PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(1, 0).UTC(), Result: "success"}
	assertJSONKeys(t, report, []string{"tool_commit", "git_commit", "pipeline_run_id", "pipeline_url", "started_at", "finished_at"}, []string{"ToolCommit", "GitCommit", "PipelineRunID", "PipelineURL", "StartedAt", "FinishedAt"})
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
