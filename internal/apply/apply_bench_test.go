package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/testutil"
	"reporting-db-migrations/internal/types"
)

// BenchmarkCollectStatements_500Transactional measures collectStatements (layout
// index lookup, file read, batchedStmt build, sort.Stable per batch) for 500
// transactional "tables" objects with batch size 100 (5 batches).
func BenchmarkCollectStatements_500Transactional(b *testing.B) {
	dir := b.TempDir()
	layout, plan := benchCollectStatementsFixture(b, dir, 500)
	objIndex := layout.ObjectsByPath()
	e := New()
	warm := &ApplyResult{}
	if _, _ = e.collectStatements(plan, objIndex, warm); len(warm.Errors) > 0 {
		b.Fatalf("warmup: %v", warm.Errors)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := &ApplyResult{}
		_, _ = e.collectStatements(plan, objIndex, res)
	}
}

// BenchmarkExecuteTxBatch_SuccessPath_100Statements measures the happy path:
// one batch exec, then one publish per statement.
func BenchmarkExecuteTxBatch_SuccessPath_100Statements(b *testing.B) {
	e := New()
	bb := bus.New()
	ctx := context.Background()
	stmts := benchBatchedStmts(100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := &testutil.MockConn{}
		res := &ApplyResult{}
		e.executeTxBatch(ctx, conn, stmts, res, bb)
	}
}

// BenchmarkExecuteTxBatch_FailurePath_100Statements measures the fallback path:
// batch exec fails (MockConn FailN=1), rollback, then per-statement exec succeeds.
func BenchmarkExecuteTxBatch_FailurePath_100Statements(b *testing.B) {
	e := New()
	bb := bus.New()
	ctx := context.Background()
	stmts := benchBatchedStmts(100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := &testutil.MockConn{FailN: 1}
		res := &ApplyResult{}
		e.executeTxBatch(ctx, conn, stmts, res, bb)
	}
}

func benchCollectStatementsFixture(b *testing.B, dir string, n int) (fs.Layout, types.MigrationPlan) {
	b.Helper()

	tablesDir := filepath.Join(dir, "db", "r", "tables")
	if err := os.MkdirAll(tablesDir, 0o755); err != nil {
		b.Fatal(err)
	}

	var layout fs.Layout
	objs := make([]*fs.Object, 0, n)
	planned := make([]types.PlannedObject, 0, n)

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("t_%04d", i)
		file := name + ".sql"
		abs := filepath.Join(tablesDir, file)
		sql := fmt.Sprintf("CREATE TABLE r.%s (id INT);\n", name)
		if err := os.WriteFile(abs, []byte(sql), 0o644); err != nil {
			b.Fatal(err)
		}
		path := filepath.ToSlash(filepath.Join("db", "r", "tables", file))
		objs = append(objs, &fs.Object{
			Path:                 path,
			DatabaseName:         "db",
			SchemaName:           "r",
			NormalizedSchemaName: "r",
			Kind:                 "tables",
			ObjectName:           name,
			NormalizedKey:        types.NormalizedKey("r", "tables", name),
			File:                 &fs.CachedFile{AbsPath: abs},
		})
		planned = append(planned, types.PlannedObject{
			ObjectRef: types.ObjectRef{
				ObjectPath:    path,
				SchemaName:    "r",
				Kind:          "tables",
				ObjectName:    name,
				NormalizedKey: types.NormalizedKey("r", "tables", name),
			},
			PlannedAction: types.ActionCreateObject,
		})
	}
	layout.Objects = objs
	plan := types.MigrationPlan{Objects: planned}
	return layout, plan
}

func benchBatchedStmts(n int) []batchedStmt {
	out := make([]batchedStmt, n)
	for i := 0; i < n; i++ {
		out[i] = batchedStmt{
			content:       "SELECT 1 AS n;",
			normalizedKey: fmt.Sprintf("r/tables/t_%04d", i),
			kind:          "tables",
			schemaName:    "r",
			objectName:    fmt.Sprintf("t_%04d", i),
			sourceFile:    fmt.Sprintf("db/r/tables/t_%04d.sql", i),
			recordKind:    "object",
		}
	}
	return out
}
