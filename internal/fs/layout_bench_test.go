package fs

import (
	"context"
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
		if err := os.WriteFile(abs, []byte(fmt.Sprintf("CREATE VIEW r.%s AS SELECT 1 AS x;\n", name)), 0o644); err != nil {
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
			File:                 &CachedFile{AbsPath: abs},
		})
	}
	return Layout{RootPath: dir, Objects: objs}
}

// benchMinimalScanObjectFixture builds a tiny repo layout recognized by Scanner
// and returns the scanned layout plus one object (used to validate stat-hint
// fast path vs cold Object without Scan hints).
func benchMinimalScanObjectFixture(b *testing.B) (Layout, *Object) {
	b.Helper()
	dir := b.TempDir()
	views := filepath.Join(dir, "dactests", "reporting", "views")
	if err := os.MkdirAll(views, 0o755); err != nil {
		b.Fatal(err)
	}
	sqlPath := filepath.Join(views, "benchobj.sql")
	if err := os.WriteFile(sqlPath, []byte("CREATE OR ALTER VIEW benchobj AS SELECT 1 AS n"), 0o644); err != nil {
		b.Fatal(err)
	}
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		b.Fatalf("Scan: %v", err)
	}
	if len(layout.Objects) == 0 {
		b.Fatal("expected at least one object")
	}
	return layout, layout.Objects[0]
}

func benchCopyObjectForChecksumLoop(template *Object) *Object {
	return &Object{
		Path:                        template.Path,
		DatabaseName:                template.DatabaseName,
		SchemaName:                  template.SchemaName,
		NormalizedSchemaName:        template.NormalizedSchemaName,
		Kind:                        template.Kind,
		ObjectName:                  template.ObjectName,
		ParentName:                  template.ParentName,
		NormalizedKey:               template.NormalizedKey,
		NoTransaction:               template.NoTransaction,
		objectStatForByteCache:      template.objectStatForByteCache,
		objectStatForByteCacheValid: template.objectStatForByteCacheValid,
		File: &CachedFile{
			AbsPath:   template.cachedFile().AbsPath,
			gitInfoFn: template.cachedFile().gitInfoFn,
		},
	}
}

// BenchmarkObjectChecksumSharedBytesAfterScan measures repeated cold
// (*Object).Checksum on the same file path after the shared byte cache is warm,
// with stat hints copied from a real Scan (fast path skips os.Stat on cache hit).
func BenchmarkObjectChecksumSharedBytesAfterScan(b *testing.B) {
	_, template := benchMinimalScanObjectFixture(b)
	if _, err := template.Checksum(); err != nil {
		b.Fatal(err)
	}
	if !template.objectStatForByteCacheValid {
		b.Fatal("expected Scan to attach byte-cache stat hint")
	}

	b.Run("withScanHint", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			o := benchCopyObjectForChecksumLoop(template)
			if _, err := o.Checksum(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("withoutScanHint", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			o := benchCopyObjectForChecksumLoop(template)
			o.objectStatForByteCacheValid = false
			if _, err := o.Checksum(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCachedFileChecksumThenContent_ColdObject(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "view.sql")
	if err := os.WriteFile(path, []byte("CREATE VIEW r.v AS SELECT 1 AS x;\n"), 0o644); err != nil {
		b.Fatal(err)
	}

	b.Run("current", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			cf := CachedFile{AbsPath: path}
			if _, err := cf.Checksum(); err != nil {
				b.Fatal(err)
			}
			if _, err := cf.Content(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("retainBytes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			o := &Object{File: &CachedFile{AbsPath: path}}
			if _, err := o.Checksum(); err != nil {
				b.Fatal(err)
			}
			if _, err := o.Content(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
