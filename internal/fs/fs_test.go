package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func createTestLayout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustMkdir := func(parts ...string) string {
		p := filepath.Join(parts...)
		os.MkdirAll(p, 0755)
		return p
	}
	writeFile := func(path, content string) {
		mustMkdir(filepath.Dir(path))
		os.WriteFile(path, []byte(content), 0644)
	}

	mustMkdir(dir, "dactests", "reporting", "views")
	mustMkdir(dir, "dactests", "reporting", "procedures")
	mustMkdir(dir, "dactests", "reporting", "functions")
	mustMkdir(dir, "dactests", "reporting", "tables")
	mustMkdir(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	mustMkdir(dir, "dactests", "reporting", "checks")

	writeFile(filepath.Join(dir, "dactests", "reporting", "views", "monthly.sql"), "CREATE OR ALTER VIEW monthly AS SELECT 1 AS n")
	writeFile(filepath.Join(dir, "dactests", "reporting", "procedures", "refresh.sql"), "CREATE OR ALTER PROC refresh AS SELECT 1")
	writeFile(filepath.Join(dir, "dactests", "reporting", "tables", "snapshot.sql"), "CREATE TABLE snapshot (id INT)")
	writeFile(filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot", "001_deadbee_add_name.sql"), "ALTER TABLE snapshot ADD name VARCHAR(100)")
	writeFile(filepath.Join(dir, "dactests", "reporting", "checks", "daily.sql"), "EXEC tSQLt.RunAll")

	return dir
}

func TestScanDiscoveresSchemas(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(layout.Schemas))
	}
	if layout.Schemas[0].Name != "reporting" {
		t.Errorf("schema name = %q", layout.Schemas[0].Name)
	}
}

func TestScanDiscoveresObjects(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Objects) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(layout.Objects))
	}
	byKind := map[string]int{}
	for _, obj := range layout.Objects {
		byKind[obj.Kind]++
		if obj.NormalizedKey == "" {
			t.Errorf("object %s has empty NormalizedKey", obj.Path)
		}
		if !strings.HasPrefix(obj.Path, "dactests/reporting/") {
			t.Errorf("object %s has unexpected path prefix", obj.Path)
		}
	}
	if byKind["views"] != 1 {
		t.Errorf("expected 1 view, got %d", byKind["views"])
	}
	if byKind["procedures"] != 1 {
		t.Errorf("expected 1 procedure, got %d", byKind["procedures"])
	}
	if byKind["tables"] != 1 {
		t.Errorf("expected 1 table, got %d", byKind["tables"])
	}
}

func TestScanDiscoveresTransitions(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(layout.Transitions))
	}
	ts := layout.Transitions[0]
	if ts.Ordinal != "001" {
		t.Errorf("ordinal = %q", ts.Ordinal)
	}
	if ts.Commit != "deadbee" {
		t.Errorf("commit = %q", ts.Commit)
	}
	if ts.Slug != "add_name" {
		t.Errorf("slug = %q", ts.Slug)
	}
	if ts.TableName != "snapshot" {
		t.Errorf("table = %q", ts.TableName)
	}
}

func TestScanDiscoveresChecks(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Checks) != 1 {
		t.Fatalf("expected 1 check script, got %d", len(layout.Checks))
	}
	if layout.Checks[0].Name != "daily" {
		t.Errorf("check name = %q", layout.Checks[0].Name)
	}
}

func TestScanEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Schemas) != 0 {
		t.Errorf("expected 0 schemas in empty dir")
	}
}

func TestScanInvalidRoot(t *testing.T) {
	_, err := NewScanner().Scan(context.Background(), "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for invalid root")
	}
}

func TestScanIgnoresMalformedTransitionFileName(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot"), 0755)
	os.WriteFile(filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot", "bad.sql"), []byte("ALTER TABLE"), 0644)

	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Transitions) != 0 {
		t.Errorf("malformed transition file should be ignored")
	}
}

func TestContentLoadedAfterScan(t *testing.T) {
	dir := createTestLayout(t)
	layout, _ := NewScanner().Scan(context.Background(), dir)

	obj := layout.Objects[0]
	content, err := obj.Content()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("content is empty")
	}
}

func TestChecksumDeterministic(t *testing.T) {
	dir := createTestLayout(t)
	layout, _ := NewScanner().Scan(context.Background(), dir)

	obj := layout.Objects[0]
	cs1, err := obj.Checksum()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cs2, _ := obj.Checksum()
	if cs1 == cs2 {
		// they are equal, this is fine
	} else {
		t.Errorf("checksum not stable: %x vs %x", cs1, cs2)
	}
	if cs1 == [32]byte{} {
		t.Fatal("checksum is empty")
	}
}

func TestTransitionScaffoldDetection(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	os.MkdirAll(migrationsDir, 0755)

	os.WriteFile(filepath.Join(migrationsDir, "001_deadbee_add_name.sql"),
		[]byte("-- rmig: transition-scaffold\n-- Replace this scaffold\n"), 0644)

	layout, _ := NewScanner().Scan(context.Background(), dir)
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition")
	}
	if !layout.Transitions[0].Scaffold {
		t.Error("expected scaffold flag to be true")
	}
}

func TestTransitionNonScaffold(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	os.MkdirAll(migrationsDir, 0755)

	os.WriteFile(filepath.Join(migrationsDir, "001_deadbee_add_name.sql"),
		[]byte("ALTER TABLE snapshot ADD name VARCHAR(100);\n"), 0644)

	layout, _ := NewScanner().Scan(context.Background(), dir)
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition")
	}
	if layout.Transitions[0].Scaffold {
		t.Error("expected scaffold flag to be false")
	}
}

func TestTransitionScaffoldCRLF(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	os.MkdirAll(migrationsDir, 0755)

	os.WriteFile(filepath.Join(migrationsDir, "001_deadbee_add_name.sql"),
		[]byte("-- rmig: transition-scaffold\r\n-- Replace this scaffold\r\n"), 0644)

	layout, _ := NewScanner().Scan(context.Background(), dir)
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition")
	}
	if !layout.Transitions[0].Scaffold {
		t.Error("expected scaffold flag to be true for CRLF file")
	}
}

func TestNormalizedKeys(t *testing.T) {
	dir := createTestLayout(t)
	layout, _ := NewScanner().Scan(context.Background(), dir)

	keys := layout.NormalizedKeys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
		if !strings.Contains(k, "/") {
			t.Errorf("key %q does not contain /", k)
		}
	}
	for _, want := range []string{
		"reporting/views/monthly",
		"reporting/procedures/refresh",
		"reporting/tables/snapshot",
	} {
		if !seen[want] {
			t.Errorf("missing expected key %q in %v", want, keys)
		}
	}
}

func TestTransitionSorting(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "t1")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatal(err)
	}
	createSQLFile(t, dir, "dactests/reporting/tables/_migrations/t1/003_cafebabe_third.sql", "-- rmig: transition")
	createSQLFile(t, dir, "dactests/reporting/tables/_migrations/t1/001_deadbeef_first.sql", "-- rmig: transition")
	createSQLFile(t, dir, "dactests/reporting/tables/_migrations/t1/002_beef1234_second.sql", "-- rmig: transition")

	scanner := NewScanner()
	layout, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(layout.Transitions))
	}
	for i, want := range []string{"001", "002", "003"} {
		if layout.Transitions[i].Ordinal != want {
			t.Errorf("transition[%d] ordinal = %q, want %q", i, layout.Transitions[i].Ordinal, want)
		}
	}
}

func TestHasExecutableTransition(t *testing.T) {
	layout := Layout{
		Transitions: []*TransitionScript{
			{Scaffold: true},
			{Scaffold: true},
		},
	}
	if layout.HasExecutableTransition() {
		t.Error("expected false when all are scaffolds")
	}

	layout.Transitions = append(layout.Transitions, &TransitionScript{Scaffold: false})
	if !layout.HasExecutableTransition() {
		t.Error("expected true when at least one is non-scaffold")
	}

	empty := Layout{}
	if empty.HasExecutableTransition() {
		t.Error("expected false when no transitions")
	}
}

func TestScanPopulatesPathIndexes(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if layout.objectsByPath == nil {
		t.Fatal("expected objectsByPath after Scan")
	}
	if layout.transitionsByPath == nil {
		t.Fatal("expected transitionsByPath after Scan")
	}
	p := layout.ObjectsByPath()
	v := p["dactests/reporting/views/monthly.sql"]
	if v == nil || v.ObjectName != "monthly" {
		t.Fatalf("ObjectsByPath: got %#v", v)
	}
}

func TestRebuildPathIndexesAfterAppendObject(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	newPath := "dactests/reporting/views/late.sql"
	createSQLFile(t, dir, newPath, "CREATE OR ALTER VIEW late AS SELECT 1 AS n")
	newAbs := filepath.Join(dir, filepath.FromSlash(newPath))

	layout.Objects = append(layout.Objects, &Object{
		Path:                 newPath,
		DatabaseName:         "dactests",
		SchemaName:           "reporting",
		NormalizedSchemaName: "reporting",
		Kind:                 "views",
		ObjectName:           "late",
		NormalizedKey:        "reporting/views/late",
		CachedFile:           CachedFile{AbsPath: newAbs},
	})
	layout.RebuildPathIndexes()
	if got := layout.ObjectsByPath()[newPath]; got == nil || got.ObjectName != "late" {
		t.Fatalf("expected appended object in path index, got %#v", got)
	}
}

func TestLayoutHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	createSQLFile(t, dir, "dactests/reporting/views/v1.sql", "CREATE VIEW reporting.v1 AS SELECT 1 AS x")
	createSQLFile(t, dir, "dactests/reporting/views/v2.sql", "CREATE VIEW reporting.v2 AS SELECT 2 AS x")

	scanner := NewScanner()
	layout1, _ := scanner.Scan(context.Background(), dir)
	layout2, _ := scanner.Scan(context.Background(), dir)

	h1, err := layout1.LayoutHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := layout2.LayoutHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("LayoutHash not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("LayoutHash should not be empty")
	}
}

func TestParseBatchedGitLogCommitLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line       string
		wantHash   string
		wantAuthor string
		wantDate   string
		wantOK     bool
	}{
		{"COMMIT|abc|Jane Doe|2026-01-02T15:04:05Z", "abc", "Jane Doe", "2026-01-02T15:04:05Z", true},
		{"COMMIT|h||2026-01-01T00:00:00Z", "h", "", "2026-01-01T00:00:00Z", true},
		{"COMMIT|onlyhash", "", "", "", false},
		{"COMMIT|h|author", "", "", "", false},
		{"path/to/file.sql", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, tc := range cases {
		h, a, d, ok := parseBatchedGitLogCommitLine([]byte(tc.line))
		if ok != tc.wantOK {
			t.Errorf("line %q: ok=%v want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if h != tc.wantHash || a != tc.wantAuthor || d != tc.wantDate {
			t.Errorf("line %q: got (%q,%q,%q) want (%q,%q,%q)", tc.line, h, a, d, tc.wantHash, tc.wantAuthor, tc.wantDate)
		}
	}
}

func TestNormalizeGitPathBytesInPlace(t *testing.T) {
	t.Parallel()
	b := []byte(`a\b\c`)
	normalizeGitPathBytesInPlace(b)
	if got, want := string(b), `a/b/c`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPreloadGitInfoCachesBatchedGitLogAcrossScans(t *testing.T) {
	root := createFakeGitRoot(t)
	objPath := filepath.Join(root, "db", "sch", "views", "monthly.sql")
	createSQLFile(t, root, "db/sch/views/monthly.sql", "SELECT 1")

	var gitLogCalls atomic.Int32
	scanner := NewScanner()
	scanner.GitInfo = func(string) (string, string, string, error) {
		return "", "", "", nil
	}
	scanner.GitLog = func(gitRoot string) ([]byte, error) {
		gitLogCalls.Add(1)
		if gitRoot != root {
			t.Fatalf("GitLog root = %q, want %q", gitRoot, root)
		}
		return []byte("COMMIT|abc123|Jane Doe|2026-01-02T15:04:05Z\n" +
			"db/sch/views/monthly.sql\n"), nil
	}

	layout1 := Layout{
		Objects: []*Object{{
			Path: "db/sch/views/monthly.sql",
			CachedFile: CachedFile{
				AbsPath:   objPath,
				gitInfoFn: scanner.GitInfo,
			},
		}},
	}
	layout2 := Layout{
		Objects: []*Object{{
			Path: "db/sch/views/monthly.sql",
			CachedFile: CachedFile{
				AbsPath:   objPath,
				gitInfoFn: scanner.GitInfo,
			},
		}},
	}

	scanner.preloadGitInfo(root, &layout1)
	scanner.preloadGitInfo(root, &layout2)

	if got := gitLogCalls.Load(); got != 1 {
		t.Fatalf("GitLog calls = %d, want 1", got)
	}
	if got, err := layout2.Objects[0].GitHash(); err != nil || got != "abc123" {
		t.Fatalf("GitHash = %q, err=%v, want abc123", got, err)
	}
	if got, err := layout2.Objects[0].GitAuthor(); err != nil || got != "Jane Doe" {
		t.Fatalf("GitAuthor = %q, err=%v, want Jane Doe", got, err)
	}
}

func TestPreloadGitInfoInvalidatesCacheWhenRepoStateChanges(t *testing.T) {
	root := createFakeGitRoot(t)
	objPath := filepath.Join(root, "db", "sch", "views", "monthly.sql")
	createSQLFile(t, root, "db/sch/views/monthly.sql", "SELECT 1")

	var gitLogCalls atomic.Int32
	scanner := NewScanner()
	scanner.GitInfo = func(string) (string, string, string, error) {
		return "", "", "", nil
	}
	scanner.GitLog = func(string) ([]byte, error) {
		n := gitLogCalls.Add(1)
		hash := "abc123"
		if n > 1 {
			hash = "def456"
		}
		return []byte("COMMIT|" + hash + "|Jane Doe|2026-01-02T15:04:05Z\n" +
			"db/sch/views/monthly.sql\n"), nil
	}

	layout1 := Layout{
		Objects: []*Object{{Path: "db/sch/views/monthly.sql", CachedFile: CachedFile{AbsPath: objPath, gitInfoFn: scanner.GitInfo}}},
	}
	layout2 := Layout{
		Objects: []*Object{{Path: "db/sch/views/monthly.sql", CachedFile: CachedFile{AbsPath: objPath, gitInfoFn: scanner.GitInfo}}},
	}

	scanner.preloadGitInfo(root, &layout1)
	touchFakeGitRef(t, root)
	scanner.preloadGitInfo(root, &layout2)

	if got := gitLogCalls.Load(); got != 2 {
		t.Fatalf("GitLog calls = %d, want 2 after repo state change", got)
	}
	if got, err := layout2.Objects[0].GitHash(); err != nil || got != "def456" {
		t.Fatalf("GitHash = %q, err=%v, want def456", got, err)
	}
}

func TestResolveGitDirFindsParentRepo(t *testing.T) {
	root := createFakeGitRoot(t)
	nested := filepath.Join(root, ".temp", "sql")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := resolveGitDir(nested)
	if !ok {
		t.Fatal("resolveGitDir returned ok=false")
	}
	want := filepath.Join(root, ".git")
	if got != want {
		t.Fatalf("resolveGitDir(%q) = %q, want %q", nested, got, want)
	}
}

func TestPreloadGitInfoSkipsFallbackOutsideGitRepo(t *testing.T) {
	root := t.TempDir()
	createSQLFile(t, root, "db/sch/views/monthly.sql", "SELECT 1")

	var gitInfoCalls atomic.Int32
	var gitLogCalls atomic.Int32
	scanner := NewScanner()
	scanner.GitLog = func(string) ([]byte, error) {
		gitLogCalls.Add(1)
		return nil, os.ErrNotExist
	}
	scanner.GitInfo = func(string) (string, string, string, error) {
		gitInfoCalls.Add(1)
		return "", "", "", nil
	}

	layout := Layout{
		Objects: []*Object{{
			Path: "db/sch/views/monthly.sql",
			CachedFile: CachedFile{
				AbsPath:   filepath.Join(root, "db", "sch", "views", "monthly.sql"),
				gitInfoFn: scanner.GitInfo,
			},
		}},
	}
	scanner.preloadGitInfo(root, &layout)
	if got := gitInfoCalls.Load(); got != 0 {
		t.Fatalf("GitInfo calls = %d, want 0 outside git repo", got)
	}
	if got := gitLogCalls.Load(); got != 0 {
		t.Fatalf("GitLog calls = %d, want 0 outside git repo", got)
	}
}

func TestScanDisablesLazyGitInfoOutsideGitRepo(t *testing.T) {
	root := t.TempDir()
	createSQLFile(t, root, "db/sch/views/monthly.sql", "SELECT 1")

	var gitInfoCalls atomic.Int32
	scanner := NewScanner()
	scanner.GitLog = func(string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	scanner.GitInfo = func(string) (string, string, string, error) {
		gitInfoCalls.Add(1)
		return "abc", "Jane", "2026-01-01T00:00:00Z", nil
	}

	layout, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(layout.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(layout.Objects))
	}
	if _, err := layout.Objects[0].GitHash(); err == nil {
		t.Fatal("expected GitHash error outside git repo")
	}
	if got := gitInfoCalls.Load(); got != 0 {
		t.Fatalf("GitInfo calls = %d, want 0 after Scan on non-git root", got)
	}
}

func TestPreloadChecksumsCachesAllFiles(t *testing.T) {
	root := t.TempDir()
	createSQLFile(t, root, "db/sch/views/v1.sql", "SELECT 1")
	createSQLFile(t, root, "db/sch/views/v2.sql", "SELECT 2")

	layout := Layout{
		Objects: []*Object{
			{CachedFile: CachedFile{AbsPath: filepath.Join(root, "db", "sch", "views", "v1.sql")}},
			{CachedFile: CachedFile{AbsPath: filepath.Join(root, "db", "sch", "views", "v2.sql")}},
		},
	}
	NewScanner().preloadChecksums(&layout)
	for i, obj := range layout.Objects {
		if !obj.IsChecksumCached() {
			t.Fatalf("object %d checksum not cached", i)
		}
	}
}

func TestScanPreloadChecksumsCachesObjectsButLeavesTransitionsAndChecksLazy(t *testing.T) {
	dir := createTestLayout(t)

	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(layout.Objects) == 0 || len(layout.Transitions) == 0 || len(layout.Checks) == 0 {
		t.Fatalf("expected object/transition/check fixtures, got %d/%d/%d", len(layout.Objects), len(layout.Transitions), len(layout.Checks))
	}
	if !layout.Objects[0].IsChecksumCached() {
		t.Fatal("expected object checksum to be eagerly cached")
	}
	if layout.Transitions[0].IsChecksumCached() {
		t.Fatal("expected transition checksum to stay lazy")
	}
	if layout.Checks[0].IsChecksumCached() {
		t.Fatal("expected check checksum to stay lazy")
	}

	if _, err := layout.Transitions[0].Checksum(); err != nil {
		t.Fatalf("transition checksum: %v", err)
	}
	if !layout.Transitions[0].IsChecksumCached() {
		t.Fatal("expected transition checksum after lazy access")
	}
	if _, err := layout.Checks[0].Checksum(); err != nil {
		t.Fatalf("check checksum: %v", err)
	}
	if !layout.Checks[0].IsChecksumCached() {
		t.Fatal("expected check checksum after lazy access")
	}
}

func TestPreloadChecksumsUsesSharedCacheAcrossScans(t *testing.T) {
	root := t.TempDir()
	rel := "db/sch/views/v1.sql"
	createSQLFile(t, root, rel, "SELECT 1")

	scanner := NewScanner()
	layout1, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	if !layout1.Objects[0].IsChecksumCached() {
		t.Fatal("expected first scan checksum cache")
	}

	path := filepath.Join(root, rel)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(path, info.Mode())

	layout2, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if !layout2.Objects[0].IsChecksumCached() {
		t.Fatal("expected second scan checksum cache from shared cache")
	}
}

func TestPreloadChecksumsInvalidatesSharedCacheOnFileChange(t *testing.T) {
	root := t.TempDir()
	rel := "db/sch/views/v1.sql"
	createSQLFile(t, root, rel, "SELECT 1")

	scanner := NewScanner()
	layout1, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	sum1, err := layout1.Objects[0].Checksum()
	if err != nil {
		t.Fatalf("first checksum: %v", err)
	}

	path := filepath.Join(root, rel)
	if err := os.WriteFile(path, []byte("SELECT 2"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	now := time.Now().Add(5 * time.Millisecond)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	layout2, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	sum2, err := layout2.Objects[0].Checksum()
	if err != nil {
		t.Fatalf("second checksum: %v", err)
	}
	if sum1 == sum2 {
		t.Fatal("expected checksum to change after file rewrite")
	}
}

func TestScanUsesLayoutCacheAcrossScans(t *testing.T) {
	dir := createTestLayout(t)

	var readDirCalls atomic.Int32
	scanner := NewScanner()
	scanner.ReadDir = func(name string) ([]os.DirEntry, error) {
		readDirCalls.Add(1)
		return os.ReadDir(name)
	}

	layout1, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	firstCalls := readDirCalls.Load()
	if firstCalls == 0 {
		t.Fatal("expected first scan to read directories")
	}

	layout2, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if got := readDirCalls.Load(); got != firstCalls {
		t.Fatalf("expected cached second scan to avoid ReadDir, got %d calls after %d", got, firstCalls)
	}
	if len(layout1.Objects) != len(layout2.Objects) || len(layout1.Transitions) != len(layout2.Transitions) || len(layout1.Checks) != len(layout2.Checks) {
		t.Fatalf("cached layout shape mismatch: objects %d/%d transitions %d/%d checks %d/%d",
			len(layout1.Objects), len(layout2.Objects),
			len(layout1.Transitions), len(layout2.Transitions),
			len(layout1.Checks), len(layout2.Checks))
	}
}

func TestScanUsesSharedLayoutCacheAcrossScanners(t *testing.T) {
	dir := createTestLayout(t)

	first := NewScanner()
	layout1, err := first.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	if len(layout1.Objects) == 0 {
		t.Fatal("expected objects on first scan")
	}

	var readDirCalls atomic.Int32
	second := NewScanner()
	second.ReadDir = func(name string) ([]os.DirEntry, error) {
		readDirCalls.Add(1)
		return os.ReadDir(name)
	}

	layout2, err := second.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if got := readDirCalls.Load(); got != 0 {
		t.Fatalf("expected shared layout cache to avoid ReadDir in second scanner, got %d calls", got)
	}
	if len(layout2.Objects) != len(layout1.Objects) || len(layout2.Transitions) != len(layout1.Transitions) || len(layout2.Checks) != len(layout1.Checks) {
		t.Fatalf("shared cached layout shape mismatch: objects %d/%d transitions %d/%d checks %d/%d",
			len(layout1.Objects), len(layout2.Objects),
			len(layout1.Transitions), len(layout2.Transitions),
			len(layout1.Checks), len(layout2.Checks))
	}
}

func TestScanLayoutCacheInvalidatesOnStructureChange(t *testing.T) {
	dir := createTestLayout(t)

	var readDirCalls atomic.Int32
	scanner := NewScanner()
	scanner.ReadDir = func(name string) ([]os.DirEntry, error) {
		readDirCalls.Add(1)
		return os.ReadDir(name)
	}

	layout1, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	firstCalls := readDirCalls.Load()
	if len(layout1.Objects) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(layout1.Objects))
	}

	createSQLFile(t, dir, "dactests/reporting/views/late.sql", "CREATE OR ALTER VIEW late AS SELECT 1 AS n")
	viewsDir := filepath.Join(dir, "dactests", "reporting", "views")
	now := time.Now().Add(5 * time.Millisecond)
	if err := os.Chtimes(viewsDir, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	layout2, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if got := readDirCalls.Load(); got <= firstCalls {
		t.Fatalf("expected invalidated scan to read directories again, got %d <= %d", got, firstCalls)
	}
	if len(layout2.Objects) != 4 {
		t.Fatalf("expected 4 objects after structure change, got %d", len(layout2.Objects))
	}
}

func TestScanLayoutCacheInvalidatesOnTransitionContentChange(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(migrationsDir, "001_deadbee_add_name.sql")
	if err := os.WriteFile(path, []byte("-- rmig: transition-scaffold\n-- Replace this scaffold\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var readDirCalls atomic.Int32
	scanner := NewScanner()
	scanner.ReadDir = func(name string) ([]os.DirEntry, error) {
		readDirCalls.Add(1)
		return os.ReadDir(name)
	}

	layout1, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	if len(layout1.Transitions) != 1 || !layout1.Transitions[0].Scaffold {
		t.Fatalf("expected initial scaffold transition, got %#v", layout1.Transitions)
	}
	firstCalls := readDirCalls.Load()

	if err := os.WriteFile(path, []byte("ALTER TABLE snapshot ADD name VARCHAR(100);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(5 * time.Millisecond)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}

	layout2, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if got := readDirCalls.Load(); got <= firstCalls {
		t.Fatalf("expected transition content change to invalidate layout cache, got %d <= %d", got, firstCalls)
	}
	if len(layout2.Transitions) != 1 || layout2.Transitions[0].Scaffold {
		t.Fatalf("expected transition scaffold=false after rewrite, got %#v", layout2.Transitions)
	}
}

func TestScanLayoutCacheInvalidatesOnGitRepoStateChange(t *testing.T) {
	root := createFakeGitRoot(t)
	createSQLFile(t, root, "db/sch/views/monthly.sql", "SELECT 1")

	var gitLogCalls atomic.Int32
	scanner := NewScanner()
	scanner.GitInfo = func(string) (string, string, string, error) { return "", "", "", nil }
	scanner.GitLog = func(string) ([]byte, error) {
		n := gitLogCalls.Add(1)
		hash := "abc123"
		if n > 1 {
			hash = "def456"
		}
		return []byte("COMMIT|" + hash + "|Jane Doe|2026-01-02T15:04:05Z\n" +
			"db/sch/views/monthly.sql\n"), nil
	}

	layout1, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	if got, err := layout1.Objects[0].GitHash(); err != nil || got != "abc123" {
		t.Fatalf("first GitHash = %q err=%v, want abc123", got, err)
	}

	touchFakeGitRef(t, root)
	layout2, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if got := gitLogCalls.Load(); got != 2 {
		t.Fatalf("expected repo state change to invalidate cached git metadata, got %d GitLog calls", got)
	}
	if got, err := layout2.Objects[0].GitHash(); err != nil || got != "def456" {
		t.Fatalf("second GitHash = %q err=%v, want def456", got, err)
	}
}

func createSQLFile(t *testing.T, base, relPath, content string) {
	t.Helper()
	abs := filepath.Join(base, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func createFakeGitRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git", "refs", "heads")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "refs", "heads", "main"), []byte("abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "index"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func touchFakeGitRef(t *testing.T, root string) {
	t.Helper()
	refPath := filepath.Join(root, ".git", "refs", "heads", "main")
	now := time.Now().Add(2 * time.Millisecond)
	if err := os.WriteFile(refPath, []byte("def456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(refPath, now, now); err != nil {
		t.Fatal(err)
	}
}
