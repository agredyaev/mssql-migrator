package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

// benchColdViewLayout builds a layout whose objects have never had Checksum()
// run, so warmupIfNeeded / warmupAll exercise the cold-cache path. Files are
// tiny SQL so hashing stays cheap relative to goroutine / scheduling noise.
func benchColdViewLayout(b *testing.B, n int) fs.Layout {
	b.Helper()
	dir := b.TempDir()
	views := filepath.Join(dir, "db", "sch", "views")
	if err := os.MkdirAll(views, 0o755); err != nil {
		b.Fatal(err)
	}
	objs := make([]*fs.Object, 0, n)
	for i := 0; i < n; i++ {
		fn := fmt.Sprintf("view_%05d.sql", i)
		abs := filepath.Join(views, fn)
		if err := os.WriteFile(abs, []byte("SELECT 1 AS n;\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		base := strings.TrimSuffix(fn, ".sql")
		objs = append(objs, &fs.Object{
			Path:                 filepath.ToSlash(filepath.Join("db", "sch", "views", fn)),
			DatabaseName:         "db",
			SchemaName:           "sch",
			NormalizedSchemaName: "sch",
			Kind:                 "views",
			ObjectName:           base,
			NormalizedKey:        types.NormalizedKey("sch", "views", base),
			CachedFile:           fs.CachedFile{AbsPath: abs},
		})
	}
	return fs.Layout{Objects: objs}
}

// BenchmarkWarmupAll_500ColdObjects measures cold-layout warmup fan-out. Each
// iteration runs the full worker pool over the same layout; after the first
// iteration checksum/git caches are hot, so later iterations isolate pool +
// channel overhead vs the historical one-goroutine-per-object design.
func BenchmarkWarmupAll_500ColdObjects(b *testing.B) {
	layout := benchColdViewLayout(b, 500)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		warmupAll(layout)
	}
}
