package migrator

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
		t.Fatalf("expected one batched metadata write for adopt_existing, got %d", len(execer.calls))
	}
	if !containsAll(execer.calls[0].query, "INSERT INTO __migrator.attempts", "VALUES (@p1, @p2, @p3") {
		t.Fatalf("expected attempt insert query, got %#v", execer.calls)
	}
	if len(execer.calls[0].args) < 5 || execer.calls[0].args[2] != "reporting/views/monthly" || execer.calls[0].args[3] != "sum" || execer.calls[0].args[4] != contracts.ActionAdoptExisting {
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
	if !containsAll(execer.calls[0].query, "INSERT INTO __migrator.attempts", "VALUES (@p1, @p2, @p3") {
		t.Fatalf("expected attempt insert query, got %#v", execer.calls)
	}
}

func TestExecutePlanBatchesPassiveObjectAttemptsUntilDDLBoundary(t *testing.T) {
	runner := NewRunner(config.Config{GitCommit: "abc", GitBranch: "main", PipelineRunID: "run-1", PipelineURL: "https://ci.example/run", Actor: "tester", TransactionMode: config.TransactionModeNone}, logger.New(logger.Options{Writer: io.Discard}))
	report := contracts.MigrationReport{Result: "running"}
	layout := parser.Layout{Objects: []parser.Object{{Path: "reporting/views/refresh.sql", AbsolutePath: "/tmp/reporting/views/refresh.sql", Content: "CREATE OR ALTER VIEW reporting.refresh AS SELECT 1;", SchemaName: "reporting", Kind: "views", ObjectName: "refresh", NormalizedKey: "reporting/views/refresh", Checksum: "sum-refresh"}}}
	plan := contracts.MigrationPlan{Objects: []contracts.PlannedObject{{ObjectPath: "reporting/views/monthly.sql", Kind: "views", NormalizedKey: "reporting/views/monthly", Checksum: "sum-monthly", PlannedAction: contracts.ActionAdoptExisting}, {ObjectPath: "reporting/tables/snapshot.sql", Kind: "tables", NormalizedKey: "reporting/tables/snapshot", Checksum: "sum-snapshot", PlannedAction: contracts.ActionSkipUnchanged}, {ObjectPath: "reporting/views/refresh.sql", Kind: "views", NormalizedKey: "reporting/views/refresh", Checksum: "sum-refresh", PlannedAction: contracts.ActionUpdateExistingModule, TransactionMode: config.TransactionModeNone, RollbackScope: contracts.RollbackScopeNone}}}

	execer := &stubExecer{result: stubResult{rows: 1}}
	if err := runner.executePlan(context.Background(), execer, layout, plan, &report); err != nil {
		t.Fatalf("unexpected executePlan error: %v", err)
	}
	if len(execer.calls) < 3 {
		t.Fatalf("expected passive batch, ddl, and active metadata writes, got %#v", execer.calls)
	}
	if !containsAll(execer.calls[0].query, "INSERT INTO __migrator.attempts", "VALUES (@p1, @p2, @p3") {
		t.Fatalf("expected first call to flush passive attempts, got %#v", execer.calls)
	}
	if len(execer.calls[0].args) != 36 {
		t.Fatalf("expected two passive attempts in one batch, got %#v", execer.calls[0].args)
	}
	if !containsAll(execer.calls[1].query, "CREATE OR ALTER VIEW", "reporting.refresh") {
		t.Fatalf("expected ddl after passive flush, got %#v", execer.calls)
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

func TestExecutePlanAppliesCheckedInTransitionsBeforeTableObject(t *testing.T) {
	runner := NewRunner(config.Config{TransactionMode: config.TransactionModeNone}, logger.New(logger.Options{Writer: io.Discard}))
	report := contracts.MigrationReport{Result: "running"}
	layout := parser.Layout{
		Objects: []parser.Object{{
			Path:          "reporting/tables/snapshot.sql",
			AbsolutePath:  "/tmp/reporting/tables/snapshot.sql",
			Content:       "CREATE TABLE reporting.snapshot(id int);",
			SchemaName:    "reporting",
			Kind:          "tables",
			ObjectName:    "snapshot",
			NormalizedKey: "reporting/tables/snapshot",
			Checksum:      "table-sum",
		}},
		Transitions: []parser.TransitionScript{{
			Path:          "reporting/tables/_migrations/snapshot/001_deadbee_expand_snapshot.sql",
			Content:       "-- migrator: no-transaction\nALTER TABLE reporting.snapshot ADD name nvarchar(100) NULL;",
			NormalizedKey: "reporting/tables/snapshot",
			Ordinal:       "001",
			NoTransaction: true,
		}},
	}
	plan := contracts.MigrationPlan{Objects: []contracts.PlannedObject{{
		ObjectPath:      "reporting/tables/snapshot.sql",
		Kind:            "tables",
		NormalizedKey:   "reporting/tables/snapshot",
		Checksum:        "table-sum",
		PlannedAction:   contracts.ActionReprocessChanged,
		TransitionPaths: []string{"reporting/tables/_migrations/snapshot/001_deadbee_expand_snapshot.sql"},
		TransactionMode: config.TransactionModeNone,
		RollbackScope:   contracts.RollbackScopeNone,
		NoTransaction:   true,
	}}}

	execer := &stubExecer{result: stubResult{rows: 1}}
	if err := runner.executePlan(context.Background(), execer, layout, plan, &report); err != nil {
		t.Fatalf("unexpected executePlan error: %v", err)
	}
	if !containsAll(execer.calls[0].query, "ALTER TABLE", "snapshot", "name") {
		t.Fatalf("expected transition SQL first, got %#v", execer.calls)
	}
	for _, call := range execer.calls[1:] {
		if containsAll(call.query, "CREATE TABLE", "snapshot") {
			t.Fatalf("expected no table DDL replay after transition-backed update, got %#v", execer.calls)
		}
	}
	if len(report.Applied) != 1 || report.Applied[0].Script != "reporting/tables/_migrations/snapshot/001_deadbee_expand_snapshot.sql" {
		t.Fatalf("expected transition in applied report, got %#v", report.Applied)
	}
	if len(report.Skipped) != 1 || report.Skipped[0].Script != "reporting/tables/snapshot.sql" || report.Skipped[0].Reason != contracts.ActionReprocessChanged {
		t.Fatalf("expected table metadata completion without table DDL replay, got applied=%#v skipped=%#v", report.Applied, report.Skipped)
	}
}

func TestExecutePlanFailsClosedWhenObjectChangesAfterDiscovery(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "dwh")
	path := filepath.Join(basePath, "reporting", "views", "refresh.sql")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("CREATE OR ALTER VIEW reporting.refresh AS SELECT 1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	layout, err := parser.DiscoverLayout(basePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("CREATE OR ALTER VIEW reporting.refresh AS SELECT 2;"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(config.Config{TransactionMode: config.TransactionModeNone}, logger.New(logger.Options{Writer: io.Discard}))
	report := contracts.MigrationReport{Result: "running"}
	plan := contracts.MigrationPlan{Objects: []contracts.PlannedObject{{ObjectPath: "reporting/views/refresh.sql", Kind: "views", NormalizedKey: "reporting/views/refresh", Checksum: layout.Objects[0].Checksum, PlannedAction: contracts.ActionUpdateExistingModule, TransactionMode: config.TransactionModeNone, RollbackScope: contracts.RollbackScopeNone}}}

	execer := &stubExecer{result: stubResult{rows: 1}}
	if err := runner.executePlan(context.Background(), execer, layout, plan, &report); err == nil || !containsAll(err.Error(), "repo layout changed after discovery", "rerun the command") {
		t.Fatalf("expected post-discovery drift failure, got %v", err)
	}
}
