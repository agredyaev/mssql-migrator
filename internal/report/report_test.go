package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/types"
)

func TestDiffComputed_WritesPlanJSON(t *testing.T) {
	b := bus.New()
	baseDir := t.TempDir()

	NewSubscriber(b, types.Config{ReportDir: baseDir})

	plan := &types.MigrationPlan{
		Command:   "plan",
		PlannedAt: time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC),
		Summary:   types.PlanSummary{ObjectCount: 3, CreateCount: 1, SkipCount: 2},
	}

	b.Publish(types.EventDiffComputed, &types.DiffResult{Plan: plan})

	planPath := filepath.Join(baseDir, ".plan.json")
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("failed to read plan file: %v", err)
	}

	var decoded types.MigrationPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal plan: %v", err)
	}
	if decoded.Command != "plan" {
		t.Errorf("command = %q, want %q", decoded.Command, "plan")
	}
	if decoded.Summary.ObjectCount != 3 {
		t.Errorf("object count = %d, want 3", decoded.Summary.ObjectCount)
	}
}

func TestRunFinished_WritesReportJSON(t *testing.T) {
	b := bus.New()
	baseDir := t.TempDir()

	NewSubscriber(b, types.Config{ReportDir: baseDir})

	b.Publish(types.EventRunFinished, &types.RunFinished{
		Command:  "migrate",
		Result:   "completed",
		ExitCode: 0,
	})

	reportPath := filepath.Join(baseDir, ".report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}

	var decoded types.RunFinished
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}
	if decoded.Command != "migrate" {
		t.Errorf("command = %q, want %q", decoded.Command, "migrate")
	}
}
