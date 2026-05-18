package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/types"
)

// BenchmarkLayoutRebuildPathIndexes_500Objects measures map allocation for
// path indexes (objects + transitions) without a full directory Scan.
func BenchmarkLayoutRebuildPathIndexes_500Objects(b *testing.B) {
	layout := benchLayoutForPathIndex(b, 500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layout.RebuildPathIndexes()
	}
}

func benchLayoutForPathIndex(b *testing.B, n int) Layout {
	b.Helper()
	dir := b.TempDir()
	viewsDir := filepath.Join(dir, "db", "r", "views")
	if err := os.MkdirAll(viewsDir, 0o755); err != nil {
		b.Fatal(err)
	}
	objs := make([]*Object, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("v_%04d", i)
		file := name + ".sql"
		abs := filepath.Join(viewsDir, file)
		if err := os.WriteFile(abs, []byte("CREATE VIEW r."+name+" AS SELECT 1 AS x;\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		rel := filepath.ToSlash(filepath.Join("db", "r", "views", file))
		objs = append(objs, &Object{
			Path:                 rel,
			DatabaseName:         "db",
			SchemaName:           "r",
			NormalizedSchemaName: "r",
			Kind:                 "views",
			ObjectName:           name,
			NormalizedKey:        types.NormalizedKey("r", "views", name),
			CachedFile:           CachedFile{AbsPath: abs},
		})
	}
	return Layout{RootPath: dir, Objects: objs}
}
