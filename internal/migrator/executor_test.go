package migrator

import (
	"context"
	"io"
	"testing"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

func TestExecutePlanTreatsAdoptExistingAsNoDDLSkip(t *testing.T) {
	runner := NewRunner(config.Config{GitCommit: "abc", GitBranch: "main", PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", Actor: "tester"}, logger.New(logger.Options{}))
	report := contracts.MigrationReport{Result: "running"}
	plan := contracts.MigrationPlan{
		Objects: []contracts.PlannedObject{{
			ObjectPath:    "reporting/views/monthly.sql",
			Kind:          "views",
			NormalizedKey: "reporting/views/monthly",
			Checksum:      "sum",
			PlannedAction: contracts.ActionAdoptExisting,
		}},
	}

	execer := &stubExecer{result: stubResult{rows: 1}}
	if err := runner.executePlan(context.Background(), execer, parser.Layout{}, plan, &report); err != nil {
		t.Fatalf("unexpected executePlan error: %v", err)
	}
	if len(execer.calls) != 1 {
		t.Fatalf("expected one metadata write for adopt_existing, got %d", len(execer.calls))
	}
	if len(execer.calls[0].args) < 6 || execer.calls[0].args[2] != "reporting/views/monthly" || execer.calls[0].args[3] != contracts.ScriptTypeObject || execer.calls[0].args[4] != "sum" || execer.calls[0].args[5] != contracts.ActionAdoptExisting {
		t.Fatalf("unexpected adopt_existing metadata args: %#v", execer.calls[0].args)
	}
	if len(report.Skipped) != 1 {
		t.Fatalf("expected one skipped object, got %#v", report.Skipped)
	}
	if report.Skipped[0].Reason != contracts.ActionAdoptExisting {
		t.Fatalf("expected adopt_existing skip reason, got %#v", report.Skipped[0])
	}
	if report.Failed != nil {
		t.Fatalf("expected no failure report, got %#v", report.Failed)
	}
}

func TestRecordAdoptedObjectWritesSuccessAttempt(t *testing.T) {
	runner := NewRunner(config.Config{GitCommit: "abc", GitBranch: "main", PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", Actor: "tester"}, logger.New(logger.Options{}))
	execer := &stubExecer{result: stubResult{rows: 1}}
	planned := contracts.PlannedObject{NormalizedKey: "reporting/views/monthly", Kind: "views", Checksum: "sum"}

	if err := runner.recordAdoptedObject(context.Background(), execer, planned); err != nil {
		t.Fatalf("unexpected recordAdoptedObject error: %v", err)
	}
	if len(execer.calls) != 1 {
		t.Fatalf("expected one metadata write, got %d", len(execer.calls))
	}
}

func TestPlannedObjectsInExecutionOrderPlacesParentsFirst(t *testing.T) {
	ordered := plannedObjectsInExecutionOrder([]contracts.PlannedObject{
		{NormalizedKey: "reporting/indexes/snapshot/ix_snapshot", SchemaName: "reporting", ParentName: "snapshot", Kind: "indexes"},
		{NormalizedKey: "reporting/tables/snapshot", SchemaName: "reporting", Kind: "tables"},
		{NormalizedKey: "reporting/triggers/monthly/trg_refresh", SchemaName: "reporting", ParentName: "monthly", Kind: "triggers"},
		{NormalizedKey: "reporting/views/monthly", SchemaName: "reporting", Kind: "views"},
	})
	keys := make([]string, 0, len(ordered))
	for _, item := range ordered {
		keys = append(keys, item.NormalizedKey)
	}
	want := []string{
		"reporting/tables/snapshot",
		"reporting/indexes/snapshot/ix_snapshot",
		"reporting/views/monthly",
		"reporting/triggers/monthly/trg_refresh",
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("unexpected execution order: %#v", keys)
		}
	}
}

func TestProgressLoggerStopIsIdempotent(t *testing.T) {
	runner := NewRunner(config.Config{}, logger.New(logger.Options{Writer: io.Discard}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := runner.startProgressLogger(ctx, "test.sql", time.Now())
	stop()
	stop()
}
