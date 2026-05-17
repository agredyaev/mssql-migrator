package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func BenchmarkDiffCompute_100Objects(b *testing.B)  { benchmarkDiffCompute(b, 100) }
func BenchmarkDiffCompute_500Objects(b *testing.B)  { benchmarkDiffCompute(b, 500) }
func BenchmarkDiffCompute_2000Objects(b *testing.B) { benchmarkDiffCompute(b, 2000) }
func BenchmarkDiffCompute_5000Objects(b *testing.B) { benchmarkDiffCompute(b, 5000) }

func benchmarkDiffCompute(b *testing.B, n int) {
	comp := diff.NewComputer()
	dir := b.TempDir()
	layout := makeRealLayout(b, dir, n)

	state := &db.State{
		Schemas: map[string]struct{}{},
		Objects: map[string]db.Object{},
	}
	checksums := map[string]string{}
	for i := 0; i < n/2; i++ {
		k := types.NormalizedKey("schema", "views", fmt.Sprintf("v_object_%d", i))
		checksums[k] = "0000000000000000000000000000000000000000000000000000000000000000"
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

func cleanFS(dir string) {
	os.RemoveAll(filepath.Join(dir, "testdb"))
}

func makeBenchSQL(size int) string {
	header := "CREATE OR ALTER VIEW schema.v_object AS\n"
	body := ""
	for i := 0; i < size; i++ {
		body += fmt.Sprintf("  SELECT %d AS col_%d, 'val_%d' AS name UNION ALL\n", i, i, i)
	}
	return header + body + "  WHERE 1=1\n"
}
