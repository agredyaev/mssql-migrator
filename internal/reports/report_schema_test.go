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
	for _, key := range []string{"tool_version", "git_commit", "sql_dir_hash", "target_env", "target_database"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing json key %s in %s", key, string(content))
		}
	}
	if _, ok := payload["ToolVersion"]; ok {
		t.Fatalf("unexpected Go-style key in %s", string(content))
	}
}
