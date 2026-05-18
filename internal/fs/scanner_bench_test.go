package fs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkScannerScan_TransitionHeavy(b *testing.B) {
	dir := b.TempDir()
	makeScanBenchmarkLayout(b, dir, 200, 10, 512)
	scanner := &Scanner{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scanner.Scan(context.Background(), dir)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScannerPreloadChecksums_2000Files(b *testing.B) {
	dir := b.TempDir()
	paths := makeChecksumBenchmarkFiles(b, dir, 2000, 256)
	scanner := &Scanner{}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		layout := makeChecksumBenchmarkLayout(paths)
		b.StartTimer()

		scanner.preloadChecksums(&layout)
	}
}

func BenchmarkScannerPreloadGitInfo_200Paths(b *testing.B) {
	dir := b.TempDir()
	absPaths := makeGitBenchRepoWithTrackedViews(b, dir, 200)
	scanner := NewScanner()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		layout := freshGitBenchLayout(dir, absPaths)
		b.StartTimer()
		scanner.preloadGitInfo(dir, &layout)
	}
}

// BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles commits thousands of
// unrelated paths so `git log --name-only` is large while the layout still
// references only 200 views. Exercises the wanted-path filter in
// `preloadGitInfo` (see `docs/specs/internals/module-fs.md`).
func BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles(b *testing.B) {
	dir := b.TempDir()
	paths := makeGitBenchRepoWithTrackedViews(b, dir, 200)
	noiseDir := filepath.Join(dir, "noise")
	if err := os.MkdirAll(noiseDir, 0o755); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		p := filepath.Join(noiseDir, fmt.Sprintf("n_%05d.txt", i))
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("-C", dir, "add", ".")
	runGit("-C", dir, "commit", "-m", "noise")

	scanner := NewScanner()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		layout := freshGitBenchLayout(dir, paths)
		b.StartTimer()
		scanner.preloadGitInfo(dir, &layout)
	}
}

func makeScanBenchmarkLayout(b *testing.B, dir string, tables, transitionsPerTable, bodyLines int) {
	b.Helper()

	base := filepath.Join(dir, "benchdb", "reporting")
	tablesDir := filepath.Join(base, "tables")
	if err := os.MkdirAll(tablesDir, 0o755); err != nil {
		b.Fatal(err)
	}

	transitionBody := strings.Repeat("ALTER TABLE t ADD c INT;\n", bodyLines)
	var scb strings.Builder
	scb.Grow(len(TransitionScaffoldDirective) + 1 + len(transitionBody))
	scb.WriteString(TransitionScaffoldDirective)
	scb.WriteByte('\n')
	scb.WriteString(transitionBody)
	scaffoldBody := scb.String()

	for i := 0; i < tables; i++ {
		tableName := fmt.Sprintf("table_%04d", i)
		tableFile := filepath.Join(tablesDir, tableName+".sql")
		if err := os.WriteFile(tableFile, []byte(fmt.Sprintf("CREATE TABLE %s (id INT);\n", tableName)), 0o644); err != nil {
			b.Fatal(err)
		}

		migrationsDir := filepath.Join(tablesDir, "_migrations", tableName)
		if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < transitionsPerTable; j++ {
			name := fmt.Sprintf("%03d_deadbee_change_%02d.sql", j+1, j)
			content := transitionBody
			if j%2 == 0 {
				content = scaffoldBody
			}
			if err := os.WriteFile(filepath.Join(migrationsDir, name), []byte(content), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func makeChecksumBenchmarkFiles(b *testing.B, dir string, count, bodyLines int) []string {
	b.Helper()

	objectsDir := filepath.Join(dir, "benchdb", "reporting", "views")
	if err := os.MkdirAll(objectsDir, 0o755); err != nil {
		b.Fatal(err)
	}

	body := strings.Repeat("SELECT 1 AS n UNION ALL\n", bodyLines)
	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		path := filepath.Join(objectsDir, fmt.Sprintf("view_%04d.sql", i))
		var sqlb strings.Builder
		const head = "CREATE OR ALTER VIEW reporting.view AS\n"
		const tail = "SELECT 1 AS tail;\n"
		sqlb.Grow(len(head) + len(body) + len(tail))
		sqlb.WriteString(head)
		sqlb.WriteString(body)
		sqlb.WriteString(tail)
		if err := os.WriteFile(path, []byte(sqlb.String()), 0o644); err != nil {
			b.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func makeChecksumBenchmarkLayout(paths []string) Layout {
	layout := Layout{
		Objects: make([]*Object, 0, len(paths)),
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".sql")
		layout.Objects = append(layout.Objects, &Object{
			Path:                 filepath.ToSlash(filepath.Join("benchdb", "reporting", "views", filepath.Base(path))),
			DatabaseName:         "benchdb",
			SchemaName:           "reporting",
			NormalizedSchemaName: "reporting",
			Kind:                 "views",
			ObjectName:           name,
			NormalizedKey:        fmt.Sprintf("reporting/views/%s", name),
			CachedFile:           CachedFile{AbsPath: path},
		})
	}
	return layout
}

func makeGitBenchRepoWithTrackedViews(b *testing.B, repoRoot string, n int) []string {
	b.Helper()

	viewsDir := filepath.Join(repoRoot, "db", "sch", "views")
	if err := os.MkdirAll(viewsDir, 0o755); err != nil {
		b.Fatal(err)
	}
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("v_%03d.sql", i)
		abs := filepath.Join(viewsDir, name)
		if err := os.WriteFile(abs, []byte("SELECT 1 AS n;\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		paths = append(paths, abs)
	}

	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	run("git", "init")
	run("git", "-C", repoRoot, "config", "user.email", "bench@test")
	run("git", "-C", repoRoot, "config", "user.name", "bench")
	run("git", "-C", repoRoot, "add", ".")
	run("git", "-C", repoRoot, "commit", "-m", "init")

	return paths
}

func freshGitBenchLayout(repoRoot string, absPaths []string) Layout {
	layout := Layout{RootPath: repoRoot}
	for _, abs := range absPaths {
		rel, err := filepath.Rel(repoRoot, abs)
		if err != nil {
			panic(err)
		}
		p := filepath.ToSlash(rel)
		name := strings.TrimSuffix(filepath.Base(abs), ".sql")
		layout.Objects = append(layout.Objects, &Object{
			Path:          p,
			DatabaseName:  "db",
			SchemaName:    "sch",
			Kind:          "views",
			ObjectName:    name,
			NormalizedKey: fmt.Sprintf("sch/views/%s", name),
			CachedFile:    CachedFile{AbsPath: abs, gitInfoFn: gitInfo},
		})
	}
	return layout
}
