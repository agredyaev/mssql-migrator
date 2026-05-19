package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/testutil"
	"reporting-db-migrations/internal/types"
)

func TestExecute_EmptyPlan(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
	mock := &testutil.MockConn{}
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
	mock := &testutil.MockConn{}
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
	if len(mock.ExecQueries) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mock.ExecQueries))
	}
}

func TestExecute_SchemaExistsSkip(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
	if len(mock.ExecQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.ExecQueries))
	}
}

func TestExecute_CreateObject(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
			File:          &fs.CachedFile{AbsPath: sqlPath},
			Path:          "r/views/v1.sql",
			NormalizedKey: "r/views/v1",
			Kind:          "views",
			SchemaName:    "r",
			ObjectName:    "v1",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectPath: "r/views/v1.sql"},
			PlannedAction: types.ActionCreateObject,
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
	if len(mock.ExecQueries) == 0 {
		t.Fatal("expected at least 1 exec call")
	}
}

func TestExecute_SkipUnchanged(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
	layout := fs.Layout{
		Objects: []*fs.Object{{
			Path:          "r/views/v1.sql",
			NormalizedKey: "r/views/v1",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/views/v1", ObjectPath: "r/views/v1.sql"},
			PlannedAction: types.ActionSkipUnchanged,
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
	if len(mock.ExecQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.ExecQueries))
	}
}

func TestExecute_UpdateModule(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
			File:          &fs.CachedFile{AbsPath: sqlPath},
			Path:          "r/views/v1.sql",
			NormalizedKey: "r/views/v1",
			Kind:          "views",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectPath: "r/views/v1.sql"},
			PlannedAction: types.ActionUpdateExistingModule,
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
	mock := &testutil.MockConn{}
	layout := fs.Layout{}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/tables/t1"},
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
	if len(mock.ExecQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.ExecQueries))
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

	result, err := e.Execute(context.Background(), &testutil.MockConn{ExecErr: fmt.Errorf("boom")}, plan, layout, b)
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

func TestExecuteTxBatchPublishesAppliedBatch(t *testing.T) {
	e := New()
	conn := &testutil.MockConn{}
	b := bus.New()

	gotCalls := 0
	gotLen := 0
	b.Subscribe(types.EventObjectApplied, func(_ context.Context, payload any) {
		gotCalls++
		events, ok := bus.ParseObjectAppliedPayload(payload)
		if !ok {
			t.Fatalf("payload type = %T, want *ObjectEvent or []*ObjectEvent", payload)
		}
		gotLen = len(events)
	})

	res := &ApplyResult{}
	stmts := []batchedStmt{
		{content: "CREATE TABLE t1(id int)", normalizedKey: "r/tables/t1", kind: "tables", schemaName: "r", objectName: "t1", sourceFile: "r/tables/t1.sql", recordKind: "object"},
		{content: "CREATE VIEW v1 AS SELECT 1", normalizedKey: "r/views/v1", kind: "views", schemaName: "r", objectName: "v1", sourceFile: "r/views/v1.sql", recordKind: "object"},
	}

	e.executeTxBatch(context.Background(), conn, stmts, res, b)

	if gotCalls != 1 {
		t.Fatalf("publish calls = %d, want 1", gotCalls)
	}
	if gotLen != 2 {
		t.Fatalf("batched event len = %d, want 2", gotLen)
	}
	if res.Applied != 2 {
		t.Fatalf("applied = %d, want 2", res.Applied)
	}
}

func TestExecuteNonTxPublishesAppliedBatch(t *testing.T) {
	e := New()
	conn := &testutil.MockConn{}
	baseDir := t.TempDir()

	write := func(rel, content string) {
		path := filepath.Join(baseDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("r/procedures/p1.sql", "CREATE OR ALTER PROC r.p1 AS SELECT 1")
	write("r/procedures/p2.sql", "CREATE OR ALTER PROC r.p2 AS SELECT 2")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r", "procedures", "p1.sql")}, Path: "r/procedures/p1.sql", NormalizedKey: "r/procedures/p1", Kind: "procedures", SchemaName: "r", ObjectName: "p1"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r", "procedures", "p2.sql")}, Path: "r/procedures/p2.sql", NormalizedKey: "r/procedures/p2", Kind: "procedures", SchemaName: "r", ObjectName: "p2"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/procedures/p1", Kind: "procedures", ObjectPath: "r/procedures/p1.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/procedures/p2", Kind: "procedures", ObjectPath: "r/procedures/p2.sql"}, PlannedAction: types.ActionCreateObject},
		},
	}
	b := bus.New()
	gotCalls := 0
	gotLen := 0
	b.Subscribe(types.EventObjectApplied, func(_ context.Context, payload any) {
		gotCalls++
		events, ok := bus.ParseObjectAppliedPayload(payload)
		if !ok {
			t.Fatalf("payload type = %T, want *ObjectEvent or []*ObjectEvent", payload)
		}
		gotLen = len(events)
	})

	res, err := e.Execute(context.Background(), conn, plan, layout, b)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Applied != 2 {
		t.Fatalf("applied = %d, want 2", res.Applied)
	}
	if gotCalls != 1 {
		t.Fatalf("publish calls = %d, want 1", gotCalls)
	}
	if gotLen != 2 {
		t.Fatalf("batched event len = %d, want 2", gotLen)
	}
}

func TestExecute_NonTxObjectsExecutedIndividually(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/views/v1.sql")}, Path: "r/views/v1.sql", NormalizedKey: "r/views/v1", Kind: "views"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/views/v2.sql")}, Path: "r/views/v2.sql", NormalizedKey: "r/views/v2", Kind: "views"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/procedures/p1.sql")}, Path: "r/procedures/p1.sql", NormalizedKey: "r/procedures/p1", Kind: "procedures"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectPath: "r/views/v1.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/views/v2", Kind: "views", ObjectPath: "r/views/v2.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/procedures/p1", Kind: "procedures", ObjectPath: "r/procedures/p1.sql"}, PlannedAction: types.ActionUpdateExistingModule},
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
	if len(mock.ExecQueries) != 3 {
		t.Errorf("expected 3 individual exec calls for non-tx objects, got %d: %v", len(mock.ExecQueries), mock.ExecQueries)
	}
}

func TestExecute_NonTxFailDoesNotAffectOthers(t *testing.T) {
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

	layout := fs.Layout{
		Objects: []*fs.Object{
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/views/v1.sql")}, Path: "r/views/v1.sql", NormalizedKey: "r/views/v1", Kind: "views"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/views/v2.sql")}, Path: "r/views/v2.sql", NormalizedKey: "r/views/v2", Kind: "views"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectPath: "r/views/v1.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/views/v2", Kind: "views", ObjectPath: "r/views/v2.sql"}, PlannedAction: types.ActionCreateObject},
		},
	}
	b := bus.New()

	mock := &testutil.MockConn{FailN: 1}
	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("applied = %d, want 1 (v2 should succeed after v1 fails)", result.Applied)
	}
	if result.Failed != 1 {
		t.Errorf("failed = %d, want 1", result.Failed)
	}
}

func TestExecute_TxBatchWrappedInTransaction(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
	createFile("r/tables/t1.sql", "CREATE TABLE r.t1 (id INT)")
	createFile("r/tables/t2.sql", "CREATE TABLE r.t2 (id INT)")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/tables/t1.sql")}, Path: "r/tables/t1.sql", NormalizedKey: "r/tables/t1", Kind: "tables"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/tables/t2.sql")}, Path: "r/tables/t2.sql", NormalizedKey: "r/tables/t2", Kind: "tables"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectPath: "r/tables/t1.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t2", Kind: "tables", ObjectPath: "r/tables/t2.sql"}, PlannedAction: types.ActionCreateObject},
		},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 2 {
		t.Errorf("applied = %d, want 2", result.Applied)
	}
	if len(mock.ExecQueries) != 1 {
		t.Fatalf("expected 1 batch exec call, got %d", len(mock.ExecQueries))
	}
	got := mock.ExecQueries[0]
	if !strings.Contains(got, "BEGIN TRANSACTION") {
		t.Errorf("batch missing BEGIN TRANSACTION: %s", got)
	}
	if !strings.Contains(got, "COMMIT TRANSACTION") {
		t.Errorf("batch missing COMMIT TRANSACTION: %s", got)
	}
}

func TestExecute_TxBatchFailsRetriesIndividuallyWrapped(t *testing.T) {
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
	createFile("r/tables/t1.sql", "CREATE TABLE r.t1 (id INT)")
	createFile("r/tables/t2.sql", "CREATE TABLE r.t2 (id INT)")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/tables/t1.sql")}, Path: "r/tables/t1.sql", NormalizedKey: "r/tables/t1", Kind: "tables"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/tables/t2.sql")}, Path: "r/tables/t2.sql", NormalizedKey: "r/tables/t2", Kind: "tables"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectPath: "r/tables/t1.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t2", Kind: "tables", ObjectPath: "r/tables/t2.sql"}, PlannedAction: types.ActionCreateObject},
		},
	}
	b := bus.New()

	mock := &testutil.MockConn{FailN: 1}
	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 2 {
		t.Errorf("applied = %d, want 2 (both succeed on retry)", result.Applied)
	}
	if result.Failed != 0 {
		t.Errorf("failed = %d, want 0", result.Failed)
	}
	if len(mock.ExecQueries) < 3 {
		t.Fatalf("expected >= 3 exec calls (1 batch fail + rollback + 2 retries), got %d: %v", len(mock.ExecQueries), mock.ExecQueries)
	}
	indivCount := 0
	for _, q := range mock.ExecQueries {
		if strings.Contains(q, "BEGIN TRANSACTION") && strings.Contains(q, "CREATE TABLE r.t") {
			indivCount++
		}
	}
	if indivCount < 2 {
		t.Errorf("expected >= 2 individual retries wrapped in BEGIN TRANSACTION, got %d", indivCount)
	}
}

func TestExecute_TxBatchFailsRollsback(t *testing.T) {
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
	createFile("r/tables/t1.sql", "CREATE TABLE r.t1 (id INT)")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/tables/t1.sql")}, Path: "r/tables/t1.sql", NormalizedKey: "r/tables/t1", Kind: "tables"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectPath: "r/tables/t1.sql"}, PlannedAction: types.ActionCreateObject},
		},
	}
	b := bus.New()

	mock := &testutil.MockConn{FailN: 1}
	_, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hasRollback := false
	for _, q := range mock.ExecQueries {
		if strings.Contains(q, "ROLLBACK TRANSACTION") {
			hasRollback = true
		}
	}
	if !hasRollback {
		t.Errorf("expected ROLLBACK after batch failure, got queries: %v", mock.ExecQueries)
	}
}

func TestExecute_TransitionWrappedInTransaction(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
	baseDir := t.TempDir()

	transPath := filepath.Join(baseDir, "r", "tables", "_migrations", "t1", "001_abc1234_add_col.sql")
	if err := os.MkdirAll(filepath.Dir(transPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(transPath, []byte("ALTER TABLE r.t1 ADD name NVARCHAR(100)"), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Transitions: []*fs.TransitionScript{{
			File:          &fs.CachedFile{AbsPath: transPath},
			Path:          "r/tables/_migrations/t1/001_abc1234_add_col.sql",
			SchemaName:    "r",
			TableName:     "t1",
			NormalizedKey: "r/tables/t1",
			Ordinal:       "001",
			Commit:        "abc1234",
			Slug:          "add_col",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:       types.ObjectRef{NormalizedKey: "r/tables/t1", SchemaName: "r", Kind: "tables", ObjectName: "t1"},
			PlannedAction:   types.ActionReprocessChanged,
			TransitionPaths: []string{"r/tables/_migrations/t1/001_abc1234_add_col.sql"},
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
	if len(mock.ExecQueries) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mock.ExecQueries))
	}
	got := mock.ExecQueries[0]
	if !strings.Contains(got, "BEGIN TRANSACTION") {
		t.Errorf("transition missing BEGIN TRANSACTION: %s", got)
	}
	if !strings.Contains(got, "COMMIT TRANSACTION") {
		t.Errorf("transition missing COMMIT TRANSACTION: %s", got)
	}
}

func TestExecute_TransitionFailRollsback(t *testing.T) {
	e := New()
	baseDir := t.TempDir()

	transPath := filepath.Join(baseDir, "r", "tables", "_migrations", "t1", "001_abc1234_add_col.sql")
	if err := os.MkdirAll(filepath.Dir(transPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(transPath, []byte("ALTER TABLE r.t1 ADD name NVARCHAR(100)"), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Transitions: []*fs.TransitionScript{{
			File:          &fs.CachedFile{AbsPath: transPath},
			Path:          "r/tables/_migrations/t1/001_abc1234_add_col.sql",
			SchemaName:    "r",
			TableName:     "t1",
			NormalizedKey: "r/tables/t1",
			Ordinal:       "001",
			Commit:        "abc1234",
			Slug:          "add_col",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:       types.ObjectRef{NormalizedKey: "r/tables/t1", SchemaName: "r", Kind: "tables", ObjectName: "t1"},
			PlannedAction:   types.ActionReprocessChanged,
			TransitionPaths: []string{"r/tables/_migrations/t1/001_abc1234_add_col.sql"},
		}},
	}
	b := bus.New()

	mock := &testutil.MockConn{FailN: 1}
	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("failed = %d, want 1", result.Failed)
	}
	hasRollback := false
	for _, q := range mock.ExecQueries {
		if strings.Contains(q, "ROLLBACK TRANSACTION") {
			hasRollback = true
		}
	}
	if !hasRollback {
		t.Errorf("expected ROLLBACK after transition failure, got queries: %v", mock.ExecQueries)
	}
}

func TestExecute_ReprocessChangedEmptyTransitions_Skipped(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
	baseDir := t.TempDir()

	sqlPath := filepath.Join(baseDir, "r", "tables", "t1.sql")
	if err := os.MkdirAll(filepath.Dir(sqlPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sqlPath, []byte("CREATE TABLE r.t1 (id INT)"), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Objects: []*fs.Object{{
			File:          &fs.CachedFile{AbsPath: sqlPath},
			Path:          "r/tables/t1.sql",
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			SchemaName:    "r",
			ObjectName:    "t1",
		}},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:       types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectPath: "r/tables/t1.sql"},
			PlannedAction:   types.ActionReprocessChanged,
			TransitionPaths: []string{},
		}},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1 (empty transitions should skip)", result.Skipped)
	}
	if result.Applied != 0 {
		t.Errorf("applied = %d, want 0", result.Applied)
	}
	if len(mock.ExecQueries) != 0 {
		t.Errorf("expected 0 exec calls, got %d", len(mock.ExecQueries))
	}
}

func TestExecute_MixedTxAndNonTx(t *testing.T) {
	e := New()
	mock := &testutil.MockConn{}
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
	createFile("r/tables/t1.sql", "CREATE TABLE r.t1 (id INT)")
	createFile("r/views/v1.sql", "CREATE VIEW r.v1 AS SELECT 1 AS x")

	layout := fs.Layout{
		Objects: []*fs.Object{
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/tables/t1.sql")}, Path: "r/tables/t1.sql", NormalizedKey: "r/tables/t1", Kind: "tables"},
			{File: &fs.CachedFile{AbsPath: filepath.Join(baseDir, "r/views/v1.sql")}, Path: "r/views/v1.sql", NormalizedKey: "r/views/v1", Kind: "views"},
		},
	}
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectPath: "r/tables/t1.sql"}, PlannedAction: types.ActionCreateObject},
			{ObjectRef: types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectPath: "r/views/v1.sql"}, PlannedAction: types.ActionCreateObject},
		},
	}
	b := bus.New()

	result, err := e.Execute(context.Background(), mock, plan, layout, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Applied != 2 {
		t.Errorf("applied = %d, want 2", result.Applied)
	}
	if len(mock.ExecQueries) != 2 {
		t.Errorf("expected 2 exec calls (1 tx batch + 1 non-tx), got %d: %v", len(mock.ExecQueries), mock.ExecQueries)
	}
	txWrapped := false
	nonTxDirect := false
	for _, q := range mock.ExecQueries {
		if strings.Contains(q, "BEGIN TRANSACTION") && strings.Contains(q, "CREATE TABLE") {
			txWrapped = true
		}
		if !strings.Contains(q, "BEGIN TRANSACTION") && strings.Contains(q, "CREATE VIEW") {
			nonTxDirect = true
		}
	}
	if !txWrapped {
		t.Errorf("expected tx batch wrapped in BEGIN TRANSACTION")
	}
	if !nonTxDirect {
		t.Errorf("expected non-tx statement without wrapping")
	}
}

func TestBatchedStmtZeroChecksumHex(t *testing.T) {
	var stmt batchedStmt
	if got := stmt.checksumHexString(); got != allZeroChecksumHex {
		t.Fatalf("checksumHexString() = %q, want %q", got, allZeroChecksumHex)
	}
	if stmt.checksumHex != allZeroChecksumHex {
		t.Fatalf("memoized checksumHex = %q, want constant", stmt.checksumHex)
	}
	stmt.checksumSum[0] = 1
	stmt.checksumHex = ""
	got := stmt.checksumHexString()
	if want := "0100000000000000000000000000000000000000000000000000000000000000"; got != want {
		t.Fatalf("non-zero digest: got %q want %q", got, want)
	}
}
