package fs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"reporting-db-migrations/internal/types"
)

const TransitionScaffoldDirective = "-- rmig: transition-scaffold"

type Scanner struct {
	GitInfo          func(AbsPath string) (hash, author, date string, err error)
	GitLog           func(root string) ([]byte, error)
	ReadDir          func(name string) ([]os.DirEntry, error)
	SkipGit          bool
	gitPreloadByRoot map[string]gitPreloadCache
	layoutByRoot     map[string]layoutCacheEntry
	mu               sync.Mutex
}

func NewScanner() *Scanner {
	return &Scanner{
		GitInfo:          gitInfo,
		GitLog:           batchedGitLog,
		ReadDir:          os.ReadDir,
		gitPreloadByRoot: make(map[string]gitPreloadCache),
		layoutByRoot:     make(map[string]layoutCacheEntry),
	}
}

type gitPreloadCache struct {
	entries   map[string]gitMeta
	repoState string
}

type gitMeta struct {
	hash   string
	author string
	date   string
}

type scanPathState struct {
	size    int64
	modTime int64
}

type layoutCacheEntry struct {
	dirStates        map[string]scanPathState
	transitionStates map[string]scanPathState
	fileStates       map[string]layoutFileCacheState
	resolvedGitDir   string
	repoState        string
	layout           Layout
}

type layoutFileCacheState struct {
	gitMeta  gitMeta
	state    scanPathState
	checksum [32]byte
}

type checksumCacheEntry struct {
	size    int64
	modTime int64
	sum     [32]byte
}

type rawBytesCacheEntry struct {
	data    []byte
	size    int64
	modTime int64
}

var sharedGitPreloadCache = struct {
	byRoot map[string]gitPreloadCache
	mu     sync.Mutex
}{
	byRoot: make(map[string]gitPreloadCache),
}

var sharedChecksumCache = struct {
	byPath map[string]checksumCacheEntry
	mu     sync.Mutex
}{
	byPath: make(map[string]checksumCacheEntry),
}

var sharedObjectBytesCache = struct {
	byPath map[string]rawBytesCacheEntry
	mu     sync.Mutex
}{
	byPath: make(map[string]rawBytesCacheEntry),
}

var sharedLayoutCache = struct {
	byRoot map[string]layoutCacheEntry
	mu     sync.Mutex
}{
	byRoot: make(map[string]layoutCacheEntry),
}

func (s *Scanner) Scan(ctx context.Context, root string) (Layout, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Layout{}, fmt.Errorf("invalid or missing SQL scripts root: %s", root)
	}

	resolvedGitDir, hasGitRepo := resolveGitDir(root)
	if layout, ok := s.loadCachedLayout(root, resolvedGitDir); ok {
		s.finalizeLayout(root, hasGitRepo, &layout, true, true)
		return layout, nil
	}

	layout := Layout{RootPath: root}
	cacheState := newLayoutCacheBuilder()
	cacheState.recordDir(root)
	dbDirs, err := s.readDir(root, cacheState)
	if err != nil {
		return Layout{}, err
	}

	for _, dbEntry := range dbDirs {
		if !dbEntry.IsDir() {
			continue
		}
		dbName := dbEntry.Name()
		dbPath := filepath.Join(root, dbName)

		schemaDirs, err := s.readDir(dbPath, cacheState)
		if err != nil {
			return Layout{}, fmt.Errorf("scan: read schema dir %s: %w", dbPath, err)
		}

		for _, schemaEntry := range schemaDirs {
			if !schemaEntry.IsDir() {
				continue
			}
			schemaName := schemaEntry.Name()
			schemaPath := filepath.Join(dbPath, schemaName)

			layout.Schemas = append(layout.Schemas, Schema{
				DatabaseName:   dbName,
				Name:           schemaName,
				NormalizedName: strings.ToLower(schemaName),
			})

			kindDirs, err := s.readDir(schemaPath, cacheState)
			if err != nil {
				return Layout{}, fmt.Errorf("scan: read kind dir %s: %w", schemaPath, err)
			}

			for _, kindEntry := range kindDirs {
				if !kindEntry.IsDir() {
					continue
				}
				kind := kindEntry.Name()
				kindPath := filepath.Join(schemaPath, kind)

				switch {
				case kind == "tables":
					if err := s.scanTableDir(&layout, cacheState, dbName, schemaName, kindPath); err != nil {
						return Layout{}, err
					}
				case kind == "checks":
					if err := s.scanCheckDir(&layout, cacheState, dbName, schemaName, kindPath); err != nil {
						return Layout{}, err
					}
				case isObjectKind(kind):
					if err := s.scanObjectDir(&layout, cacheState, dbName, schemaName, kind, kindPath); err != nil {
						return Layout{}, err
					}
				}
			}
		}
	}

	sort.Slice(layout.Transitions, func(i, j int) bool {
		return layout.Transitions[i].Ordinal < layout.Transitions[j].Ordinal
	})

	fileStates := s.finalizeLayout(root, hasGitRepo, &layout, false, false)
	s.storeLayoutCache(root, resolvedGitDir, cacheState, layout, fileStates, root)
	return layout, nil
}

func (s *Scanner) finalizeLayout(root string, hasGitRepo bool, layout *Layout, checksumsReady, gitReady bool) map[string]layoutFileCacheState {
	if !hasGitRepo {
		disableLayoutGitInfo(layout)
	}
	layout.RebuildPathIndexes()
	if !gitReady && !s.SkipGit {
		s.preloadGitInfo(root, layout)
	}
	if !checksumsReady {
		s.preloadChecksums(layout)
		fs := buildLayoutFileCacheState(*layout)
		attachObjectByteCacheStatHints(layout, fs)
		return fs
	}
	return nil
}

func (s *Scanner) readDir(path string, cacheState *layoutCacheBuilder) ([]os.DirEntry, error) {
	readDir := s.ReadDir
	if readDir == nil {
		readDir = os.ReadDir
	}
	entries, err := readDir(path)
	if err != nil {
		return nil, err
	}
	if cacheState != nil {
		cacheState.recordDir(path)
	}
	return entries, nil
}

func (s *Scanner) scanObjectDir(layout *Layout, cacheState *layoutCacheBuilder, dbName, schemaName, kind, dirPath string) error {
	files, err := s.readDir(dirPath, cacheState)
	if err != nil {
		return fmt.Errorf("scan: read object dir %s: %w", dirPath, err)
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		layout.Objects = append(layout.Objects, s.newObject(dbName, schemaName, kind, "", f.Name(), dirPath))
	}
	return nil
}

func (s *Scanner) scanTableDir(layout *Layout, cacheState *layoutCacheBuilder, dbName, schemaName, tableDir string) error {
	files, err := s.readDir(tableDir, cacheState)
	if err != nil {
		return fmt.Errorf("scan: read table dir %s: %w", tableDir, err)
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || filepath.Ext(name) != ".sql" {
			continue
		}
		layout.Objects = append(layout.Objects, s.newObject(dbName, schemaName, "tables", "", name, tableDir))
	}

	migrationsDir := filepath.Join(tableDir, "_migrations")
	entries, err := s.readDir(migrationsDir, cacheState)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("scan: read migrations dir %s: %w", migrationsDir, err)
	}
	for _, tableEntry := range entries {
		if !tableEntry.IsDir() {
			continue
		}
		tableName := tableEntry.Name()
		tableMigrationsDir := filepath.Join(migrationsDir, tableName)

		migrationFiles, err := s.readDir(tableMigrationsDir, cacheState)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("scan: read migration dir %s: %w", tableMigrationsDir, err)
		}
		for _, mf := range migrationFiles {
			if mf.IsDir() || filepath.Ext(mf.Name()) != ".sql" {
				continue
			}
			ts, ok := s.parseTransitionFile(dbName, schemaName, tableName, mf.Name(), tableMigrationsDir)
			if ok {
				layout.Transitions = append(layout.Transitions, ts)
				if cacheState != nil {
					cacheState.recordTransition(ts.AbsPath)
				}
			}
		}
	}
	return nil
}

func (s *Scanner) scanCheckDir(layout *Layout, cacheState *layoutCacheBuilder, dbName, schemaName, dirPath string) error {
	files, err := s.readDir(dirPath, cacheState)
	if err != nil {
		return fmt.Errorf("scan: read check dir %s: %w", dirPath, err)
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		layout.Checks = append(layout.Checks, s.newCheck(dbName, schemaName, f.Name(), dirPath))
	}
	return nil
}

func (s *Scanner) newObject(dbName, schemaName, kind, parentName, fileName, dirPath string) *Object {
	fullPath := filepath.Join(dirPath, fileName)
	name := strings.TrimSuffix(fileName, ".sql")
	obj := &Object{
		Path:                 filepath.ToSlash(filepath.Join(dbName, schemaName, kind, fileName)),
		DatabaseName:         dbName,
		SchemaName:           schemaName,
		NormalizedSchemaName: strings.ToLower(schemaName),
		Kind:                 kind,
		ObjectName:           name,
		ParentName:           parentName,
		NormalizedKey:        types.NormalizedKey(schemaName, kind, name),
		NoTransaction:        types.IsModuleKind(kind),
		CachedFile:           CachedFile{AbsPath: fullPath, gitInfoFn: s.GitInfo},
	}
	if kind == "triggers" && parentName != "" {
		obj.ParentNormalizedKey = types.NormalizedKey(schemaName, "tables", parentName)
	}
	return obj
}

func (s *Scanner) newCheck(dbName, schemaName, fileName, dirPath string) *CheckScript {
	fullPath := filepath.Join(dirPath, fileName)
	name := strings.TrimSuffix(fileName, ".sql")
	return &CheckScript{
		Path:          filepath.ToSlash(filepath.Join(dbName, schemaName, "checks", fileName)),
		DatabaseName:  dbName,
		SchemaName:    schemaName,
		Name:          name,
		NoTransaction: true,
		CachedFile:    CachedFile{AbsPath: fullPath, gitInfoFn: s.GitInfo},
	}
}

var transitionPattern = regexp.MustCompile(`^(\d{3})_([0-9a-f]{7,})_(.+)\.sql$`)
var transitionScaffoldDirectiveBytes = []byte(TransitionScaffoldDirective)
var batchedGitLogCommitPrefix = []byte("COMMIT|")

// normalizeGitPathBytesInPlace converts '\' to '/' in-place so git log paths
// match layout Path strings built with filepath.ToSlash. Safe because each
// line slice is only scanned once while parsing cmd.Output().
func normalizeGitPathBytesInPlace(b []byte) {
	for i := range b {
		if b[i] == '\\' {
			b[i] = '/'
		}
	}
}

// parseBatchedGitLogCommitLine extracts metadata from a git log line using
// --format=COMMIT|%H|%an|%aI (trimmed). Returns ok=false if the line is not a
// well-formed COMMIT record.
func parseBatchedGitLogCommitLine(line []byte) (hash, author, date string, ok bool) {
	pl := len(batchedGitLogCommitPrefix)
	if len(line) < pl || !bytes.HasPrefix(line, batchedGitLogCommitPrefix) {
		return "", "", "", false
	}
	rest := line[pl:]
	hEnd := bytes.IndexByte(rest, '|')
	if hEnd <= 0 {
		return "", "", "", false
	}
	hash = string(rest[:hEnd])
	rest = rest[hEnd+1:]
	aEnd := bytes.IndexByte(rest, '|')
	if aEnd < 0 {
		return "", "", "", false
	}
	author = string(rest[:aEnd])
	date = string(rest[aEnd+1:])
	return hash, author, date, true
}

func (s *Scanner) parseTransitionFile(dbName, schemaName, tableName, fileName, dirPath string) (*TransitionScript, bool) {
	matches := transitionPattern.FindStringSubmatch(fileName)
	if matches == nil {
		return nil, false
	}

	fullPath := filepath.Join(dirPath, fileName)
	normalizedKey := types.NormalizedKey(schemaName, "tables", tableName)

	ts := &TransitionScript{
		Path:          filepath.ToSlash(filepath.Join(dbName, schemaName, "tables", "_migrations", tableName, fileName)),
		DatabaseName:  dbName,
		SchemaName:    schemaName,
		TableName:     tableName,
		NormalizedKey: normalizedKey,
		Ordinal:       matches[1],
		Commit:        matches[2],
		Slug:          matches[3],
		CachedFile:    CachedFile{AbsPath: fullPath, gitInfoFn: s.GitInfo},
	}

	if hasTransitionScaffoldDirective(fullPath) {
		ts.Scaffold = true
	}

	return ts, true
}

func hasTransitionScaffoldDirective(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, len(transitionScaffoldDirectiveBytes)+2)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	firstLine := buf[:n]
	if idx := bytes.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	firstLine = bytes.TrimRight(firstLine, "\r")
	return bytes.HasPrefix(firstLine, transitionScaffoldDirectiveBytes)
}

func isObjectKind(kind string) bool {
	return types.IsKnownKind(kind) && kind != "tables" && kind != "checks"
}

func (s *Scanner) preloadGitInfo(root string, layout *Layout) {
	if s.GitInfo == nil {
		return
	}
	targets := buildPreloadGitTargets(layout)
	if len(targets) == 0 {
		return
	}

	_, hasGitRepo := resolveGitDir(root)
	if !hasGitRepo {
		return
	}
	repoState, ok := gitRepoState(root)
	if ok && s.applyGitPreloadCache(root, repoState, targets) {
		return
	}

	// Try fast batched git log
	gitLog := s.GitLog
	if gitLog == nil {
		gitLog = batchedGitLog
	}
	out, err := gitLog(root)
	if err == nil {
		remaining := len(targets)
		var currentHash, currentAuthor, currentDate string
		matched := make(map[string]gitMeta, remaining)
		data := out
		for len(data) > 0 && remaining > 0 {
			var line []byte
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				line = data[:i]
				data = data[i+1:]
			} else {
				line = data
				data = nil
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			if h, a, d, ok := parseBatchedGitLogCommitLine(line); ok {
				currentHash, currentAuthor, currentDate = h, a, d
				continue
			}
			normalizeGitPathBytesInPlace(line)
			rel := string(line)
			cf, ok := targets[rel]
			if !ok || currentHash == "" {
				continue
			}
			cf.preloadGitInfo(currentHash, currentAuthor, currentDate)
			matched[rel] = gitMeta{hash: currentHash, author: currentAuthor, date: currentDate}
			delete(targets, rel)
			remaining--
		}
		if ok {
			s.storeGitPreloadCache(root, repoState, matched)
		}
		return
	}
	// Fallback to slow path if git batched command fails (e.g. testing mocks that don't have a real .git dir)
	type entry struct {
		cf   *CachedFile
		path string
	}
	entries := make([]entry, 0, len(layout.Objects)+len(layout.Transitions)+len(layout.Checks))
	for i := range layout.Objects {
		entries = append(entries, entry{cf: &layout.Objects[i].CachedFile, path: layout.Objects[i].AbsPath})
	}
	for i := range layout.Transitions {
		entries = append(entries, entry{cf: &layout.Transitions[i].CachedFile, path: layout.Transitions[i].AbsPath})
	}
	for i := range layout.Checks {
		entries = append(entries, entry{cf: &layout.Checks[i].CachedFile, path: layout.Checks[i].AbsPath})
	}

	if len(entries) == 0 {
		return
	}

	parallelForEach(entries, boundedWorkerCount(len(entries)), func(e entry) {
		hash, author, date, err := s.GitInfo(e.path)
		if err == nil {
			e.cf.preloadGitInfo(hash, author, date)
		}
	})
}

func (s *Scanner) applyGitPreloadCache(root, repoState string, targets map[string]*CachedFile) bool {
	s.mu.Lock()
	cache, ok := s.gitPreloadByRoot[root]
	s.mu.Unlock()
	if ok && cache.repoState == repoState && applyGitPreloadCacheEntries(cache.entries, targets) {
		return true
	}

	sharedGitPreloadCache.mu.Lock()
	shared, ok := sharedGitPreloadCache.byRoot[root]
	sharedGitPreloadCache.mu.Unlock()
	if !ok || shared.repoState != repoState {
		return false
	}
	if !applyGitPreloadCacheEntries(shared.entries, targets) {
		return false
	}
	s.mu.Lock()
	if s.gitPreloadByRoot == nil {
		s.gitPreloadByRoot = make(map[string]gitPreloadCache)
	}
	s.gitPreloadByRoot[root] = shared
	s.mu.Unlock()
	return true
}

func (s *Scanner) storeGitPreloadCache(root, repoState string, matched map[string]gitMeta) {
	if len(matched) == 0 {
		return
	}
	sharedGitPreloadCache.mu.Lock()
	shared := mergeGitPreloadCache(sharedGitPreloadCache.byRoot[root], repoState, matched)
	sharedGitPreloadCache.byRoot[root] = shared
	sharedGitPreloadCache.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gitPreloadByRoot == nil {
		s.gitPreloadByRoot = make(map[string]gitPreloadCache)
	}
	s.gitPreloadByRoot[root] = mergeGitPreloadCache(s.gitPreloadByRoot[root], repoState, matched)
}

func applyGitPreloadCacheEntries(entries map[string]gitMeta, targets map[string]*CachedFile) bool {
	for rel := range targets {
		if _, ok := entries[rel]; !ok {
			return false
		}
	}
	for rel, cf := range targets {
		meta := entries[rel]
		cf.preloadGitInfo(meta.hash, meta.author, meta.date)
	}
	return true
}

func mergeGitPreloadCache(cache gitPreloadCache, repoState string, matched map[string]gitMeta) gitPreloadCache {
	if cache.repoState != repoState || cache.entries == nil {
		entries := make(map[string]gitMeta, len(matched))
		for rel, meta := range matched {
			entries[rel] = meta
		}
		return gitPreloadCache{
			repoState: repoState,
			entries:   entries,
		}
	}
	for rel, meta := range matched {
		cache.entries[rel] = meta
	}
	return cache
}

func batchedGitLog(root string) ([]byte, error) {
	cmd := exec.Command("git", "-C", root, "log", "--name-only", "--format=COMMIT|%H|%an|%aI")
	return cmd.Output()
}

func gitRepoState(root string) (string, bool) {
	gitDir, ok := resolveGitDir(root)
	if !ok {
		return "", false
	}
	headPath := filepath.Join(gitDir, "HEAD")
	headBytes, err := os.ReadFile(headPath)
	if err != nil {
		return "", false
	}
	head := strings.TrimSpace(string(headBytes))

	var b strings.Builder
	b.Grow(256)
	b.WriteString("head:")
	b.WriteString(head)

	appendRepoStateFile(&b, "headstat", headPath)
	appendRepoStateFile(&b, "index", filepath.Join(gitDir, "index"))

	if ref, ok := strings.CutPrefix(head, "ref: "); ok {
		refPath := filepath.Join(gitDir, filepath.FromSlash(strings.TrimSpace(ref)))
		appendRepoStateFile(&b, "ref", refPath)
	}
	return b.String(), true
}

func resolveGitDir(root string) (string, bool) {
	for dir := root; ; dir = filepath.Dir(dir) {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return gitPath, true
			}
			data, err := os.ReadFile(gitPath)
			if err != nil {
				return "", false
			}
			line := strings.TrimSpace(string(data))
			gitDir, ok := strings.CutPrefix(line, "gitdir: ")
			if !ok {
				return "", false
			}
			gitDir = strings.TrimSpace(gitDir)
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(filepath.Dir(gitPath), gitDir)
			}
			return gitDir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
	}
}

func appendRepoStateFile(b *strings.Builder, label, path string) {
	info, err := os.Stat(path)
	if err != nil {
		b.WriteByte('|')
		b.WriteString(label)
		b.WriteString(":missing")
		return
	}
	b.WriteByte('|')
	b.WriteString(label)
	b.WriteByte(':')
	b.WriteString(path)
	b.WriteByte(':')
	b.WriteString(fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()/int64(time.Millisecond)))
}

func buildPreloadGitTargets(layout *Layout) map[string]*CachedFile {
	mapHint := len(layout.Objects) + len(layout.Transitions) + len(layout.Checks)
	if mapHint < 8 {
		mapHint = 8
	}
	// Layout paths are unique by repository file path in normal scans, so one
	// direct path -> CachedFile target lets the fast path preload metadata
	// without a second gitMap + apply pass.
	targets := make(map[string]*CachedFile, mapHint)
	for i := range layout.Objects {
		targets[filepath.ToSlash(layout.Objects[i].Path)] = &layout.Objects[i].CachedFile
	}
	for i := range layout.Transitions {
		targets[filepath.ToSlash(layout.Transitions[i].Path)] = &layout.Transitions[i].CachedFile
	}
	for i := range layout.Checks {
		targets[filepath.ToSlash(layout.Checks[i].Path)] = &layout.Checks[i].CachedFile
	}
	return targets
}

func disableLayoutGitInfo(layout *Layout) {
	for i := range layout.Objects {
		layout.Objects[i].gitInfoFn = nil
	}
	for i := range layout.Transitions {
		layout.Transitions[i].gitInfoFn = nil
	}
	for i := range layout.Checks {
		layout.Checks[i].gitInfoFn = nil
	}
}

// preloadChecksums eagerly computes checksums only for layout.Objects, because
// diff.Compute's hot path compares object digests but does not need transition
// or check digests. Those remain lazy and compute on first real use.
func (s *Scanner) preloadChecksums(layout *Layout) {
	n := len(layout.Objects)
	if n == 0 {
		return
	}

	workers := boundedWorkerCount(n)
	jobs := make(chan *Object, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for o := range jobs {
				preloadChecksumWithCache(o)
			}
		}()
	}
	for _, o := range layout.Objects {
		jobs <- o
	}
	close(jobs)
	wg.Wait()
}

func preloadChecksumWithCache(o *Object) {
	if o == nil {
		return
	}
	cf := &o.CachedFile
	if sum, ok := lookupSharedChecksum(cf.AbsPath); ok {
		cf.preloadChecksum(sum)
		return
	}
	sum, err := o.Checksum()
	if err != nil {
		return
	}
	storeSharedChecksum(cf.AbsPath, sum)
}

func lookupSharedChecksum(path string) ([32]byte, bool) {
	sharedChecksumCache.mu.Lock()
	entry, ok := sharedChecksumCache.byPath[path]
	sharedChecksumCache.mu.Unlock()
	if !ok {
		return [32]byte{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return [32]byte{}, false
	}
	if entry.size != info.Size() || entry.modTime != info.ModTime().UnixNano() {
		return [32]byte{}, false
	}
	return entry.sum, true
}

func storeSharedChecksum(path string, sum [32]byte) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	sharedChecksumCache.mu.Lock()
	sharedChecksumCache.byPath[path] = checksumCacheEntry{
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
		sum:     sum,
	}
	sharedChecksumCache.mu.Unlock()
}

func attachObjectByteCacheStatHints(layout *Layout, fileStates map[string]layoutFileCacheState) {
	for _, o := range layout.Objects {
		if o == nil {
			continue
		}
		st, ok := fileStates[o.AbsPath]
		if !ok {
			o.objectStatForByteCacheValid = false
			continue
		}
		o.objectStatForByteCache = st.state
		o.objectStatForByteCacheValid = true
	}
}

// lookupSharedObjectBytes returns cached file bytes when the on-disk file
// still matches the cached size and mtime. If hint matches the cache entry
// metadata, os.Stat is skipped; hint must come from attachObjectByteCacheStatHints
// (same Scanner layout snapshot as when the cache entry was validated).
func lookupSharedObjectBytes(path string, hint *scanPathState) ([]byte, bool) {
	sharedObjectBytesCache.mu.Lock()
	entry, ok := sharedObjectBytesCache.byPath[path]
	sharedObjectBytesCache.mu.Unlock()
	if !ok {
		return nil, false
	}
	if hint != nil && hint.size == entry.size && hint.modTime == entry.modTime {
		return entry.data, true
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if entry.size != info.Size() || entry.modTime != info.ModTime().UnixNano() {
		return nil, false
	}
	return entry.data, true
}

func storeSharedObjectBytesWithStat(path string, data []byte, st scanPathState) {
	sharedObjectBytesCache.mu.Lock()
	sharedObjectBytesCache.byPath[path] = rawBytesCacheEntry{
		size:    st.size,
		modTime: st.modTime,
		data:    data,
	}
	sharedObjectBytesCache.mu.Unlock()
}

func boundedWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > 32 {
		workers = 32
	}
	if workers < 1 {
		workers = 1
	}
	if total < workers {
		return total
	}
	return workers
}

type layoutCacheBuilder struct {
	dirStates        map[string]scanPathState
	transitionStates map[string]scanPathState
}

func newLayoutCacheBuilder() *layoutCacheBuilder {
	return &layoutCacheBuilder{
		dirStates:        make(map[string]scanPathState),
		transitionStates: make(map[string]scanPathState),
	}
}

func (b *layoutCacheBuilder) recordDir(path string) {
	state, ok := readPathState(path)
	if !ok {
		return
	}
	b.dirStates[path] = state
}

func (b *layoutCacheBuilder) recordTransition(path string) {
	state, ok := readPathState(path)
	if !ok {
		return
	}
	b.transitionStates[path] = state
}

func readPathState(path string) (scanPathState, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return scanPathState{}, false
	}
	return scanPathState{
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
	}, true
}

func (s *Scanner) loadCachedLayout(root, resolvedGitDir string) (Layout, bool) {
	s.mu.Lock()
	entry, ok := s.layoutByRoot[root]
	s.mu.Unlock()
	if ok {
		if layout, ok := s.tryLayoutCacheEntry(root, resolvedGitDir, entry); ok {
			return layout, true
		}
	}

	sharedLayoutCache.mu.Lock()
	shared, ok := sharedLayoutCache.byRoot[root]
	sharedLayoutCache.mu.Unlock()
	if !ok {
		return Layout{}, false
	}
	layout, ok := s.tryLayoutCacheEntry(root, resolvedGitDir, shared)
	if !ok {
		return Layout{}, false
	}
	s.mu.Lock()
	if s.layoutByRoot == nil {
		s.layoutByRoot = make(map[string]layoutCacheEntry)
	}
	s.layoutByRoot[root] = shared
	s.mu.Unlock()
	return layout, true
}

func (s *Scanner) tryLayoutCacheEntry(root, resolvedGitDir string, entry layoutCacheEntry) (Layout, bool) {
	if entry.resolvedGitDir != resolvedGitDir {
		return Layout{}, false
	}
	if entry.repoState != "" {
		repoState, ok := gitRepoState(root)
		if !ok || repoState != entry.repoState {
			return Layout{}, false
		}
	}
	if !layoutCacheStillValid(entry) {
		return Layout{}, false
	}
	layout := cloneLayoutMetadata(entry.layout, s.GitInfo)
	applyLayoutFileCache(&layout, entry.fileStates)
	attachObjectByteCacheStatHints(&layout, entry.fileStates)
	return layout, true
}

func (s *Scanner) storeLayoutCache(root, resolvedGitDir string, cacheState *layoutCacheBuilder, layout Layout, fileStates map[string]layoutFileCacheState, scanRoot string) {
	if fileStates == nil {
		fileStates = buildLayoutFileCacheState(layout)
	}
	snapshot := cloneLayoutMetadata(layout, s.GitInfo)
	repoState, _ := gitRepoState(scanRoot)
	entry := layoutCacheEntry{
		layout:           snapshot,
		dirStates:        clonePathStateMap(cacheState.dirStates),
		transitionStates: clonePathStateMap(cacheState.transitionStates),
		fileStates:       fileStates,
		resolvedGitDir:   resolvedGitDir,
		repoState:        repoState,
	}
	s.mu.Lock()
	if s.layoutByRoot == nil {
		s.layoutByRoot = make(map[string]layoutCacheEntry)
	}
	s.layoutByRoot[root] = entry
	s.mu.Unlock()

	sharedLayoutCache.mu.Lock()
	sharedLayoutCache.byRoot[root] = entry
	sharedLayoutCache.mu.Unlock()
}

func layoutCacheStillValid(entry layoutCacheEntry) bool {
	for path, want := range entry.dirStates {
		got, ok := readPathState(path)
		if !ok || got != want {
			return false
		}
	}
	for path, want := range entry.fileStates {
		got, ok := readPathState(path)
		if !ok || got != want.state {
			return false
		}
	}
	for path, want := range entry.transitionStates {
		got, ok := readPathState(path)
		if !ok || got != want {
			return false
		}
	}
	return true
}

func clonePathStateMap(src map[string]scanPathState) map[string]scanPathState {
	dst := make(map[string]scanPathState, len(src))
	for path, state := range src {
		dst[path] = state
	}
	return dst
}

func cloneLayoutMetadata(src Layout, gitInfoFn func(string) (string, string, string, error)) Layout {
	dst := Layout{
		RootPath: src.RootPath,
	}
	if len(src.Schemas) > 0 {
		dst.Schemas = append([]Schema(nil), src.Schemas...)
	}
	if len(src.Objects) > 0 {
		dst.Objects = make([]*Object, 0, len(src.Objects))
		for _, obj := range src.Objects {
			if obj == nil {
				continue
			}
			copied := &Object{
				Path:                 obj.Path,
				DatabaseName:         obj.DatabaseName,
				SchemaName:           obj.SchemaName,
				NormalizedSchemaName: obj.NormalizedSchemaName,
				Kind:                 obj.Kind,
				ObjectName:           obj.ObjectName,
				ParentName:           obj.ParentName,
				ParentNormalizedKey:  obj.ParentNormalizedKey,
				NormalizedKey:        obj.NormalizedKey,
				NoTransaction:        obj.NoTransaction,
				CachedFile: CachedFile{
					AbsPath:   obj.AbsPath,
					gitInfoFn: gitInfoFn,
				},
			}
			dst.Objects = append(dst.Objects, copied)
		}
	}
	if len(src.Transitions) > 0 {
		dst.Transitions = make([]*TransitionScript, 0, len(src.Transitions))
		for _, ts := range src.Transitions {
			if ts == nil {
				continue
			}
			copied := &TransitionScript{
				Path:          ts.Path,
				DatabaseName:  ts.DatabaseName,
				SchemaName:    ts.SchemaName,
				TableName:     ts.TableName,
				NormalizedKey: ts.NormalizedKey,
				Ordinal:       ts.Ordinal,
				Commit:        ts.Commit,
				Slug:          ts.Slug,
				NoTransaction: ts.NoTransaction,
				Scaffold:      ts.Scaffold,
				CachedFile: CachedFile{
					AbsPath:   ts.AbsPath,
					gitInfoFn: gitInfoFn,
				},
			}
			dst.Transitions = append(dst.Transitions, copied)
		}
	}
	if len(src.Checks) > 0 {
		dst.Checks = make([]*CheckScript, 0, len(src.Checks))
		for _, check := range src.Checks {
			if check == nil {
				continue
			}
			copied := &CheckScript{
				Path:          check.Path,
				DatabaseName:  check.DatabaseName,
				SchemaName:    check.SchemaName,
				Name:          check.Name,
				NoTransaction: check.NoTransaction,
				CachedFile: CachedFile{
					AbsPath:   check.AbsPath,
					gitInfoFn: gitInfoFn,
				},
			}
			dst.Checks = append(dst.Checks, copied)
		}
	}
	return dst
}

func buildLayoutFileCacheState(layout Layout) map[string]layoutFileCacheState {
	total := len(layout.Objects) + len(layout.Transitions) + len(layout.Checks)
	states := make(map[string]layoutFileCacheState, total)
	for _, obj := range layout.Objects {
		if obj == nil {
			continue
		}
		addLayoutFileCacheState(states, &obj.CachedFile)
	}
	for _, ts := range layout.Transitions {
		if ts == nil {
			continue
		}
		addLayoutFileCacheState(states, &ts.CachedFile)
	}
	for _, check := range layout.Checks {
		if check == nil {
			continue
		}
		addLayoutFileCacheState(states, &check.CachedFile)
	}
	return states
}

func addLayoutFileCacheState(states map[string]layoutFileCacheState, cf *CachedFile) {
	if cf == nil || !cf.IsChecksumCached() {
		return
	}
	state, ok := readPathState(cf.AbsPath)
	if !ok {
		return
	}
	states[cf.AbsPath] = layoutFileCacheState{
		state:    state,
		checksum: cf.checksum,
		gitMeta: gitMeta{
			hash:   cf.gitHash,
			author: cf.gitAuthor,
			date:   cf.gitDate,
		},
	}
}

func applyLayoutFileCache(layout *Layout, fileStates map[string]layoutFileCacheState) {
	for _, obj := range layout.Objects {
		applyCachedChecksum(&obj.CachedFile, fileStates)
	}
	for _, ts := range layout.Transitions {
		applyCachedChecksum(&ts.CachedFile, fileStates)
	}
	for _, check := range layout.Checks {
		applyCachedChecksum(&check.CachedFile, fileStates)
	}
}

func applyCachedChecksum(cf *CachedFile, fileStates map[string]layoutFileCacheState) {
	if cf == nil {
		return
	}
	cached, ok := fileStates[cf.AbsPath]
	if !ok {
		return
	}
	cf.preloadChecksum(cached.checksum)
	if cached.gitMeta.hash != "" || cached.gitMeta.author != "" || cached.gitMeta.date != "" {
		cf.preloadGitInfo(cached.gitMeta.hash, cached.gitMeta.author, cached.gitMeta.date)
	}
}

func parallelForEach[T any](items []T, workers int, fn func(T)) {
	if len(items) == 0 {
		return
	}
	if workers <= 1 {
		for _, item := range items {
			fn(item)
		}
		return
	}

	jobs := make(chan T, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				fn(item)
			}
		}()
	}
	for _, item := range items {
		jobs <- item
	}
	close(jobs)
	wg.Wait()
}
