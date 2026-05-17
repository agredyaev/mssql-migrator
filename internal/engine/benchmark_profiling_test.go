package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/fs"
)

func BenchmarkDiffCompute_100Objects(b *testing.B)  { benchmarkDiffComputeCreateHeavy(b, 100) }
func BenchmarkDiffCompute_500Objects(b *testing.B)  { benchmarkDiffComputeCreateHeavy(b, 500) }
func BenchmarkDiffCompute_2000Objects(b *testing.B) { benchmarkDiffComputeCreateHeavy(b, 2000) }
func BenchmarkDiffCompute_5000Objects(b *testing.B) { benchmarkDiffComputeCreateHeavy(b, 5000) }

func BenchmarkDiffCompute_Create_100Objects(b *testing.B)  { benchmarkDiffComputeCreateHeavy(b, 100) }
func BenchmarkDiffCompute_Create_500Objects(b *testing.B)  { benchmarkDiffComputeCreateHeavy(b, 500) }
func BenchmarkDiffCompute_Create_2000Objects(b *testing.B) { benchmarkDiffComputeCreateHeavy(b, 2000) }
func BenchmarkDiffCompute_Create_5000Objects(b *testing.B) { benchmarkDiffComputeCreateHeavy(b, 5000) }

func BenchmarkDiffCompute_SkipHeavy_100Objects(b *testing.B)  { benchmarkDiffComputeSkipHeavy(b, 100) }
func BenchmarkDiffCompute_SkipHeavy_500Objects(b *testing.B)  { benchmarkDiffComputeSkipHeavy(b, 500) }
func BenchmarkDiffCompute_SkipHeavy_2000Objects(b *testing.B) { benchmarkDiffComputeSkipHeavy(b, 2000) }
func BenchmarkDiffCompute_SkipHeavy_5000Objects(b *testing.B) { benchmarkDiffComputeSkipHeavy(b, 5000) }

// benchmarkDiffComputeCreateHeavy uses an empty DB state and empty checksums so every
// layout object is planned as ActionCreateObject (worst-case planner branch for object count).
func benchmarkDiffComputeCreateHeavy(b *testing.B, n int) {
	comp := diff.NewComputer()
	dir := b.TempDir()
	layout := makeRealLayout(b, dir, n)

	state := &db.State{
		Schemas: map[string]struct{}{},
		Objects: map[string]db.Object{},
	}
	checksums := map[string][32]byte{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := comp.Compute(context.Background(), layout, state, checksums)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// benchmarkDiffComputeSkipHeavy fills state and checksums from the scanned layout so every
// object is ActionSkipUnchanged (isolates planner overhead without setGitInfo on the hot path).
func benchmarkDiffComputeSkipHeavy(b *testing.B, n int) {
	comp := diff.NewComputer()
	dir := b.TempDir()
	layout := makeRealLayout(b, dir, n)

	state := &db.State{
		Schemas: make(map[string]struct{}, len(layout.Schemas)),
		Objects: make(map[string]db.Object, len(layout.Objects)),
	}
	for _, sch := range layout.Schemas {
		state.Schemas[sch.NormalizedName] = struct{}{}
	}
	checksums := make(map[string][32]byte, len(layout.Objects))
	for _, obj := range layout.Objects {
		cs, err := obj.Checksum()
		if err != nil {
			b.Fatal(err)
		}
		k := obj.NormalizedKey
		state.Objects[k] = db.Object{
			SchemaName: obj.SchemaName,
			Kind:       obj.Kind,
			ObjectName: obj.ObjectName,
			ParentName: obj.ParentName,
		}
		checksums[k] = cs
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := comp.Compute(context.Background(), layout, state, checksums)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNormalizeAndHash_SmallSQL(b *testing.B)  { benchmarkNormalize(b, 200) }
func BenchmarkNormalizeAndHash_MediumSQL(b *testing.B) { benchmarkNormalize(b, 2000) }
func BenchmarkNormalizeAndHash_LargeSQL(b *testing.B)  { benchmarkNormalize(b, 20000) }

func BenchmarkLayoutHash_500Objects(b *testing.B)  { benchmarkLayoutHash(b, 500) }
func BenchmarkLayoutHash_2000Objects(b *testing.B) { benchmarkLayoutHash(b, 2000) }

func benchmarkLayoutHash(b *testing.B, n int) {
	dir := b.TempDir()
	layout := makeRealLayout(b, dir, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := layout.LayoutHash()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkNormalize(b *testing.B, size int) {
	sqlContent := makeBenchSQL(size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs.NormalizeAndHash(sqlContent)
	}
}

func makeRealLayout(b *testing.B, dir string, n int) fs.Layout {
	makeRealFS(b, dir, n)
	scanner := fs.NewScanner()
	layout, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	return layout
}

func makeRealFS(b *testing.B, dir string, n int) {
	baseDir := filepath.Join(dir, "testdb", "schema")
	kinds := []string{"views", "procedures", "functions", "tables"}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		b.Fatal(err)
	}

	for _, kind := range kinds {
		kindPath := filepath.Join(baseDir, kind)
		if err := os.MkdirAll(kindPath, 0755); err != nil {
			b.Fatal(err)
		}
	}

	objIdx := 0
	for _, kind := range kinds {
		count := n / len(kinds)
		if kind == "tables" {
			count = n / len(kinds) / 2
		}
		for j := 0; j < count && objIdx < n; j++ {
			objIdx++
			objName := fmt.Sprintf("object_%d.sql", objIdx)
			content := makeBenchSQL(300 + (objIdx % 500))
			if err := os.WriteFile(filepath.Join(baseDir, kind, objName), []byte(content), 0644); err != nil {
				b.Fatal(err)
			}
		}
	}

	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "test").Run()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()
}

func makeBenchSQL(size int) string {
	var b strings.Builder
	// One grow avoids O(n²) string concatenation from repeated += in the loop.
	b.Grow(size*48 + 128)
	b.WriteString("CREATE OR ALTER VIEW schema.v_object AS\n")
	for i := 0; i < size; i++ {
		fmt.Fprintf(&b, "  SELECT %d AS col_%d, 'val_%d' AS name UNION ALL\n", i, i, i)
	}
	b.WriteString("  WHERE 1=1\n")
	return b.String()
}
