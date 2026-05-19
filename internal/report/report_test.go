package report

import (
	"context"
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
		Summary:   types.PlanSummary{ObjectCount: 3, CreateCount: 1, SkipCount: 2, BlockedCount: 1},
		Blocked:   true,
		Blockers:  []string{"table r/tables/t1 changed but has no non-scaffold transition scripts"},
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/views/v1"}, PlannedAction: types.ActionSkipUnchanged},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t1"}, PlannedAction: types.ActionReprocessChangedBlocked},
		},
		Failures: []string{"warning: metadata mismatch"},
	}

	b.Publish(context.Background(), types.EventDiffComputed, &types.DiffResult{Plan: plan})
	b.Publish(context.Background(), types.EventRunFinished, &types.RunFinished{
		Command:  "plan",
		Result:   "success",
		ExitCode: 0,
	})

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
	if decoded.Summary.BlockedCount != 1 {
		t.Errorf("blocked count = %d, want 1", decoded.Summary.BlockedCount)
	}
	if !decoded.Blocked {
		t.Error("expected plan to be blocked")
	}
	if len(decoded.Blockers) != 1 {
		t.Errorf("expected 1 blocker, got %d", len(decoded.Blockers))
	}
	if len(decoded.Objects) != 2 {
		t.Errorf("expected 2 objects, got %d", len(decoded.Objects))
	}
	if len(decoded.Failures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(decoded.Failures))
	}
}

func TestRunFinished_WritesReportJSON(t *testing.T) {
	b := bus.New()
	baseDir := t.TempDir()

	NewSubscriber(b, types.Config{ReportDir: baseDir})

	b.Publish(context.Background(), types.EventRunFinished, &types.RunFinished{
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
	if decoded.Result != "completed" {
		t.Errorf("result = %q, want %q", decoded.Result, "completed")
	}
	if decoded.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", decoded.ExitCode)
	}
}

func TestRunFinished_WritesFailureReportJSON(t *testing.T) {
	b := bus.New()
	baseDir := t.TempDir()

	NewSubscriber(b, types.Config{ReportDir: baseDir})

	b.Publish(context.Background(), types.EventRunFinished, &types.RunFinished{
		Command:  "migrate",
		Result:   "failure",
		ExitCode: 5,
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
	if decoded.Result != "failure" {
		t.Errorf("result = %q, want %q", decoded.Result, "failure")
	}
	if decoded.ExitCode != 5 {
		t.Errorf("exit code = %d, want 5", decoded.ExitCode)
	}
}

func TestDiffComputed_NilPlanIgnored(t *testing.T) {
	b := bus.New()
	baseDir := t.TempDir()

	var errMsg string
	sub := NewSubscriber(b, types.Config{ReportDir: baseDir})
	sub.SetErrorHandler(func(msg string) { errMsg = msg })

	b.Publish(context.Background(), types.EventDiffComputed, &types.DiffResult{Plan: nil})

	_, err := os.Stat(filepath.Join(baseDir, ".plan.json"))
	if !os.IsNotExist(err) {
		t.Error(".plan.json should not exist for nil plan")
	}
	if errMsg != "" {
		t.Errorf("unexpected error: %s", errMsg)
	}
}

func TestRunFinished_ToleratesMissingDir(t *testing.T) {
	b := bus.New()

	var errMsg string
	sub := NewSubscriber(b, types.Config{ReportDir: "/nonexistent/path"})
	sub.SetErrorHandler(func(msg string) { errMsg = msg })

	b.Publish(context.Background(), types.EventRunFinished, &types.RunFinished{
		Command: "migrate",
	})

	if errMsg == "" {
		t.Error("expected write error for missing directory")
	}
}
