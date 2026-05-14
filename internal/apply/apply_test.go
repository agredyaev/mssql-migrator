package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type mockConn struct {
	execErr     error
	failN       int
	execCount   int
	execQueries []string
	queryCalls  []string
}

func (m *mockConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	m.queryCalls = append(m.queryCalls, query)
	return &mockRows{}, nil
}
func (m *mockConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	m.execCount++
	if m.failN > 0 && m.execCount <= m.failN {
		return nil, fmt.Errorf("injected error after %d calls", m.execCount)
	}
	if m.execErr != nil {
		return nil, m.execErr
	}
	m.execQueries = append(m.execQueries, query)
	return &mockResult{}, nil
}
func (m *mockConn) Ping(ctx context.Context) error { return nil }
func (m *mockConn) Close() error                   { return nil }

type mockRows struct{}

func (m *mockRows) Scan(dest ...any) error { return nil }
func (m *mockRows) Next() bool             { return false }
func (m *mockRows) Close() error           { return nil }

type mockResult struct{}

func (m *mockResult) RowsAffected() (int64, error) { return 0, nil }

func TestExecute_EmptyPlan(t *testing.T) {
	e := New()
	mock := &mockConn{}
	plan := types.MigrationPlan{}
	layout := fs.Layout{}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 0 {
		t.Errorf("applied = %d, want 0", result.Applied)
	}
}

func TestExecute_BlockedPlan(t *testing.T) {
	e := New()
	mock := &mockConn{}
	plan := types.MigrationPlan{Blocked: true}
	layout := fs.Layout{}
	b := bus.New()

	_, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err == nil {
		t.Fatal("expected error for blocked plan")
	}
}

func TestExecute_CreateSchema(t *testing.T) {
	e := New()
	mock := &mockConn{}
	plan := types.MigrationPlan{
		Schemas: []types.PlannedSchema{{
			SchemaName: "r",
			Action:     types.SchemaActionCreateSchema,
		}},
	}
	layout := fs.Layout{}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("applied = %d, want 1", result.Applied)
	}
	if len(mock.execQueries) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mock.execQueries))
	}
}

func TestExecute_SchemaExistsSkip(t *testing.T) {
	e := New()
	mock := &mockConn{}
	plan := types.MigrationPlan{
		Schemas: []types.PlannedSchema{{
			SchemaName: "r",
			Action:     types.SchemaActionExists,
		}},
	}
	layout := fs.Layout{}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if len(mock.execQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.execQueries))
	}
}

func TestExecute_CreateObject(t *testing.T) {
	e := New()
	mock := &mockConn{}
	baseDir := t.TempDir()

	sqlPath := filepath.Join(baseDir, "r", "views", "v1.sql")
	if err := os.MkdirAll(filepath.Dir(sqlPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sqlPath, []byte("CREATE VIEW r.v1 AS SELECT 1 AS x"), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Objects: []*fs.Object{{
			AbsolutePath:  sqlPath,
			Path:          "r/views/v1.sql",
			NormalizedKey: "r/views/v1",
			Kind:          "views",
			SchemaName:    "r",
			ObjectName:    "v1",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/views/v1",
			PlannedAction: types.ActionCreateObject,
			SourceFile:    "r/views/v1.sql",
		}},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("applied = %d, want 1", result.Applied)
	}
	if len(mock.execQueries) == 0 {
		t.Fatal("expected at least 1 exec call")
	}
}

func TestExecute_SkipUnchanged(t *testing.T) {
	e := New()
	mock := &mockConn{}
	layout := fs.Layout{
		Objects: []*fs.Object{{
			Path:          "r/views/v1.sql",
			NormalizedKey: "r/views/v1",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/views/v1",
			PlannedAction: types.ActionSkipUnchanged,
			SourceFile:    "r/views/v1.sql",
		}},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if len(mock.execQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.execQueries))
	}
}

func TestExecute_UpdateModule(t *testing.T) {
	e := New()
	mock := &mockConn{}
	baseDir := t.TempDir()

	sqlPath := filepath.Join(baseDir, "r", "views", "v1.sql")
	if err := os.MkdirAll(filepath.Dir(sqlPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sqlPath, []byte("ALTER VIEW r.v1 AS SELECT 2 AS x"), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Objects: []*fs.Object{{
			AbsolutePath:  sqlPath,
			Path:          "r/views/v1.sql",
			NormalizedKey: "r/views/v1",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/views/v1",
			PlannedAction: types.ActionUpdateExistingModule,
			SourceFile:    "r/views/v1.sql",
		}},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("applied = %d, want 1", result.Applied)
	}
}

func TestExecute_AdoptExisting(t *testing.T) {
	e := New()
	mock := &mockConn{}
	layout := fs.Layout{}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/tables/t1",
			PlannedAction: types.ActionAdoptExisting,
		}},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if len(mock.execQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.execQueries))
	}
}

func TestExecute_ExecError(t *testing.T) {
	e := New()
	layout := fs.Layout{}
	plan := types.MigrationPlan{
		Schemas: []types.PlannedSchema{{
			SchemaName: "r",
			Action:     types.SchemaActionCreateSchema,
		}},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), &mockConn{execErr: fmt.Errorf("boom")}, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestExecute_BatchMultipleObjects(t *testing.T) {
	e := New()
	mock := &mockConn{}
	baseDir := t.TempDir()

	createFile := func(rel string, content string) {
		path := filepath.Join(baseDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writefile: %v", err)
		}
	}
	createFile("r/views/v1.sql", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	createFile("r/views/v2.sql", "CREATE VIEW r.v2 AS SELECT 2 AS x")
	createFile("r/procedures/p1.sql", "CREATE PROCEDURE r.p1 AS SELECT 1")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{AbsolutePath: filepath.Join(baseDir, "r/views/v1.sql"), Path: "r/views/v1.sql", NormalizedKey: "r/views/v1"},
			{AbsolutePath: filepath.Join(baseDir, "r/views/v2.sql"), Path: "r/views/v2.sql", NormalizedKey: "r/views/v2"},
			{AbsolutePath: filepath.Join(baseDir, "r/procedures/p1.sql"), Path: "r/procedures/p1.sql", NormalizedKey: "r/procedures/p1"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{NormalizedKey: "r/views/v1", PlannedAction: types.ActionCreateObject, SourceFile: "r/views/v1.sql"},
			{NormalizedKey: "r/views/v2", PlannedAction: types.ActionCreateObject, SourceFile: "r/views/v2.sql"},
			{NormalizedKey: "r/procedures/p1", PlannedAction: types.ActionUpdateExistingModule, SourceFile: "r/procedures/p1.sql"},
		},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 3 {
		t.Errorf("applied = %d, want 3", result.Applied)
	}
	if len(mock.execQueries) != 1 {
		t.Errorf("expected 1 batch exec call, got %d: %v", len(mock.execQueries), mock.execQueries)
	}
}

func TestExecute_BatchFailsRetriesIndividually(t *testing.T) {
	e := New()
	baseDir := t.TempDir()

	createFile := func(rel string, content string) {
		path := filepath.Join(baseDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writefile: %v", err)
		}
	}
	createFile("r/views/v1.sql", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	createFile("r/views/v2.sql", "CREATE VIEW r.v2 AS SELECT 2 AS x")
	createFile("r/procedures/p1.sql", "CREATE PROCEDURE r.p1 AS SELECT 1")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{AbsolutePath: filepath.Join(baseDir, "r/views/v1.sql"), Path: "r/views/v1.sql", NormalizedKey: "r/views/v1"},
			{AbsolutePath: filepath.Join(baseDir, "r/views/v2.sql"), Path: "r/views/v2.sql", NormalizedKey: "r/views/v2"},
			{AbsolutePath: filepath.Join(baseDir, "r/procedures/p1.sql"), Path: "r/procedures/p1.sql", NormalizedKey: "r/procedures/p1"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{NormalizedKey: "r/views/v1", PlannedAction: types.ActionCreateObject, SourceFile: "r/views/v1.sql"},
			{NormalizedKey: "r/views/v2", PlannedAction: types.ActionCreateObject, SourceFile: "r/views/v2.sql"},
			{NormalizedKey: "r/procedures/p1", PlannedAction: types.ActionUpdateExistingModule, SourceFile: "r/procedures/p1.sql"},
		},
	}
	b := bus.New()

	mock := &mockConn{failN: 1}
	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 3 {
		t.Errorf("applied = %d, want 3", result.Applied)
	}
	if result.Failed != 0 {
		t.Errorf("failed = %d, want 0 (all should pass on retry)", result.Failed)
	}
}
