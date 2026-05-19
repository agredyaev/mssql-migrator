package perf

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
	"reporting-db-migrations/internal/types"
)

// BenchmarkDiffCompute_SkipHeavy_500Objects is the 500-object diff baseline bench.
func BenchmarkDiffCompute_SkipHeavy_500Objects(b *testing.B) {
	benchmarkDiffComputeSkipHeavy(b, 500)
}

// BenchmarkDiffCompute_SkipHeavy_5000Objects is the 5000-object diff baseline bench.
func BenchmarkDiffCompute_SkipHeavy_5000Objects(b *testing.B) {
	benchmarkDiffComputeSkipHeavy(b, 5000)
}

func benchmarkDiffComputeSkipHeavy(b *testing.B, n int) {
	comp := diff.NewComputer(types.Config{SkipGit: true})
	dir := b.TempDir()
	layout := makeBenchLayout(b, dir, n)

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

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := comp.Compute(context.Background(), layout, state, checksums); err != nil {
			b.Fatal(err)
		}
	}
}

func makeBenchLayout(b *testing.B, dir string, n int) fs.Layout {
	makeBenchFS(b, dir, n)
	scanner := fs.NewScanner()
	scanner.SkipGit = true
	layout, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	return layout
}

func makeBenchFS(b *testing.B, dir string, n int) {
	baseDir := filepath.Join(dir, "testdb", "schema")
	kinds := []string{"views", "procedures", "functions", "tables"}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		b.Fatal(err)
	}
	for _, kind := range kinds {
		if err := os.MkdirAll(filepath.Join(baseDir, kind), 0o755); err != nil {
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
			if err := os.WriteFile(filepath.Join(baseDir, kind, objName), []byte(content), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	_ = exec.Command("git", "-C", dir, "init").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.email", "bench@test.com").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.name", "bench").Run()
	_ = exec.Command("git", "-C", dir, "add", ".").Run()
	_ = exec.Command("git", "-C", dir, "commit", "-m", "init").Run()
}

func makeBenchSQL(size int) string {
	var b strings.Builder
	b.Grow(size*48 + 128)
	b.WriteString("CREATE OR ALTER VIEW schema.v_object AS\n")
	for i := 0; i < size; i++ {
		fmt.Fprintf(&b, "  SELECT %d AS col_%d UNION ALL\n", i, i)
	}
	b.WriteString("  SELECT 1 AS z\n")
	return b.String()
}

// RunFootprintBenchmarks executes baseline benches and returns results (for baseline capture tests).
func RunFootprintBenchmarks() []BenchEntry {
	names := []struct {
		name string
		fn   func(*testing.B)
	}{
		{"BenchmarkDiffCompute_SkipHeavy_500Objects", BenchmarkDiffCompute_SkipHeavy_500Objects},
		{"BenchmarkDiffCompute_SkipHeavy_5000Objects", BenchmarkDiffCompute_SkipHeavy_5000Objects},
	}
	out := make([]BenchEntry, 0, len(names))
	for _, nb := range names {
		r := testing.Benchmark(nb.fn)
		out = append(out, BenchEntry{
			Name:        nb.name,
			NsPerOp:     r.NsPerOp(),
			AllocsPerOp: r.AllocsPerOp(),
			BytesPerOp:  r.AllocedBytesPerOp(),
		})
	}
	return out
}
