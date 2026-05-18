package db

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/testutil"
	"reporting-db-migrations/internal/types"
)

func BenchmarkScopeKey_2000Parts(b *testing.B) {
	layout := largeBenchLayout(1, 2000, 0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scopeKey(layout)
	}
}

// BenchmarkScopeKeyPhase3SlotKey_2000Parts measures canonical scopeKey plus
// SHA-256 hex digest used as the per-inspector cache map key (Phase 3).
func BenchmarkScopeKeyPhase3SlotKey_2000Parts(b *testing.B) {
	layout := largeBenchLayout(1, 2000, 0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scopeKeySHA256Hex(scopeKey(layout))
	}
}

func BenchmarkInspectorInspect_Cold_500Objects(b *testing.B) {
	layout := largeBenchLayout(1, 500, 0, 0)
	conn := newInspectorBenchConn()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		insp := NewInspector()
		b.StartTimer()
		_, _ = insp.Inspect(ctx, conn, layout)
	}
}

func BenchmarkInspectorInspect_HotCache_500Objects(b *testing.B) {
	layout := largeBenchLayout(1, 500, 0, 0)
	conn := newInspectorBenchConn()
	ctx := context.Background()
	insp := NewInspector()
	if _, err := insp.Inspect(ctx, conn, layout); err != nil {
		b.Fatal(err)
	}
	q1 := conn.QueryCount.Load()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = insp.Inspect(ctx, conn, layout)
	}
	b.StopTimer()
	if conn.QueryCount.Load() != q1 {
		b.Fatalf("expected hot cache: query count changed from %d to %d", q1, conn.QueryCount.Load())
	}
}

func newInspectorBenchConn() *testutil.MockConn {
	return &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT CASE":                              testutil.NewMockRows([][]any{{1}}),
			"WITH inspector_schema_filter AS (":        testutil.NewMockRows(nil),
			"WITH inspector_object_schema_filter AS (": testutil.NewMockRows(nil),
			"WITH inspector_column_schema_filter AS (": testutil.NewMockRows(nil),
		},
	}
}

func largeBenchLayout(schemaCount, objectsPerSchema, transitions, checks int) fs.Layout {
	var layout fs.Layout
	for s := 0; s < schemaCount; s++ {
		sch := fmt.Sprintf("Sch_%04d", s)
		norm := strings.ToLower(sch)
		layout.Schemas = append(layout.Schemas, fs.Schema{
			DatabaseName:   "db",
			Name:           sch,
			NormalizedName: norm,
		})
		for o := 0; o < objectsPerSchema; o++ {
			file := fmt.Sprintf("obj_%04d.sql", o)
			path := fmt.Sprintf("db/%s/views/%s", sch, file)
			name := strings.TrimSuffix(file, ".sql")
			layout.Objects = append(layout.Objects, &fs.Object{
				Path:                 path,
				DatabaseName:         "db",
				SchemaName:           sch,
				NormalizedSchemaName: norm,
				Kind:                 "views",
				ObjectName:           name,
				NormalizedKey:        types.NormalizedKey(sch, "views", name),
			})
		}
	}
	for t := 0; t < transitions; t++ {
		layout.Transitions = append(layout.Transitions, &fs.TransitionScript{
			Path:          fmt.Sprintf("db/sch/tables/_migrations/t1/%03d_abcd123_slug.sql", t),
			NormalizedKey: "sch/tables/t1",
		})
	}
	for c := 0; c < checks; c++ {
		layout.Checks = append(layout.Checks, &fs.CheckScript{
			Path: fmt.Sprintf("db/sch/checks/check_%04d.sql", c),
		})
	}
	return layout
}
