package fs

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"reporting-db-migrations/internal/types"
)

const TransitionScaffoldDirective = "-- rmig: transition-scaffold"

type Scanner struct {
	GitInfo func(AbsPath string) (hash, author, date string, err error)
}

func NewScanner() *Scanner {
	return &Scanner{GitInfo: gitInfo}
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

	layout := Layout{RootPath: root}
	dbDirs, err := os.ReadDir(root)
	if err != nil {
		return Layout{}, err
	}

	for _, dbEntry := range dbDirs {
		if !dbEntry.IsDir() {
			continue
		}
		dbName := dbEntry.Name()
		dbPath := filepath.Join(root, dbName)

		schemaDirs, err := os.ReadDir(dbPath)
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

			kindDirs, err := os.ReadDir(schemaPath)
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
					if err := s.scanTableDir(&layout, dbName, schemaName, kindPath); err != nil {
						return Layout{}, err
					}
				case kind == "checks":
					if err := s.scanCheckDir(&layout, dbName, schemaName, kindPath); err != nil {
						return Layout{}, err
					}
				case isObjectKind(kind):
					if err := s.scanObjectDir(&layout, dbName, schemaName, kind, kindPath); err != nil {
						return Layout{}, err
					}
				}
			}
		}
	}

	sort.Slice(layout.Transitions, func(i, j int) bool {
		return layout.Transitions[i].Ordinal < layout.Transitions[j].Ordinal
	})

	s.preloadGitInfo(root, &layout)

	return layout, nil
}

func (s *Scanner) scanObjectDir(layout *Layout, dbName, schemaName, kind, dirPath string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("scan: read object dir %s: %w", dirPath, err)
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		obj := s.buildObject(dbName, schemaName, kind, "", f.Name(), dirPath)
		layout.Objects = append(layout.Objects, &obj)
	}
	return nil
}

func (s *Scanner) scanTableDir(layout *Layout, dbName, schemaName, tableDir string) error {
	files, err := os.ReadDir(tableDir)
	if err != nil {
		return fmt.Errorf("scan: read table dir %s: %w", tableDir, err)
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || filepath.Ext(name) != ".sql" {
			continue
		}
		obj := s.buildObject(dbName, schemaName, "tables", "", name, tableDir)
		layout.Objects = append(layout.Objects, &obj)
	}

	migrationsDir := filepath.Join(tableDir, "_migrations")
	entries, err := os.ReadDir(migrationsDir)
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

		migrationFiles, err := os.ReadDir(tableMigrationsDir)
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
			}
		}
	}
	return nil
}

func (s *Scanner) scanCheckDir(layout *Layout, dbName, schemaName, dirPath string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("scan: read check dir %s: %w", dirPath, err)
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		cs := s.buildCheck(dbName, schemaName, f.Name(), dirPath)
		layout.Checks = append(layout.Checks, &cs)
	}
	return nil
}

func (s *Scanner) buildObject(dbName, schemaName, kind, parentName, fileName, dirPath string) Object {
	fullPath := filepath.Join(dirPath, fileName)
	name := strings.TrimSuffix(fileName, ".sql")
	return Object{
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
}

func (s *Scanner) buildCheck(dbName, schemaName, fileName, dirPath string) CheckScript {
	fullPath := filepath.Join(dirPath, fileName)
	name := strings.TrimSuffix(fileName, ".sql")
	return CheckScript{
		Path:          filepath.ToSlash(filepath.Join(dbName, schemaName, "checks", fileName)),
		DatabaseName:  dbName,
		SchemaName:    schemaName,
		Name:          name,
		NoTransaction: true,
		CachedFile:    CachedFile{AbsPath: fullPath, gitInfoFn: s.GitInfo},
	}
}

var transitionPattern = regexp.MustCompile(`^(\d{3})_([0-9a-f]{7,})_(.+)\.sql$`)

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

	f, err := os.Open(fullPath)
	if err == nil {
		firstLine, _ := bufio.NewReader(f).ReadString('\n')
		f.Close()
		if strings.HasPrefix(strings.TrimRight(firstLine, "\r\n"), TransitionScaffoldDirective) {
			ts.Scaffold = true
		}
	}

	return ts, true
}

func isObjectKind(kind string) bool {
	return types.IsKnownKind(kind) && kind != "tables" && kind != "checks"
}

func (s *Scanner) preloadGitInfo(root string, layout *Layout) {
	if s.GitInfo == nil {
		return
	}

	// Try fast batched git log
	cmd := exec.Command("git", "-C", root, "log", "--name-only", "--format=COMMIT|%H|%an|%aI")
	out, err := cmd.Output()
	if err == nil {
		gitMap := make(map[string]struct{ hash, author, date string })
		var currentHash, currentAuthor, currentDate string
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "COMMIT|") {
				parts := strings.SplitN(line, "|", 4)
				if len(parts) == 4 {
					currentHash = parts[1]
					currentAuthor = parts[2]
					currentDate = parts[3]
				}
				continue
			}
			absPath := filepath.Join(root, line)
			if _, ok := gitMap[absPath]; !ok && currentHash != "" {
				gitMap[absPath] = struct{ hash, author, date string }{currentHash, currentAuthor, currentDate}
			}
		}

		for i := range layout.Objects {
			if info, ok := gitMap[layout.Objects[i].AbsPath]; ok {
				layout.Objects[i].CachedFile.preloadGitInfo(info.hash, info.author, info.date)
			}
		}
		for i := range layout.Transitions {
			if info, ok := gitMap[layout.Transitions[i].AbsPath]; ok {
				layout.Transitions[i].CachedFile.preloadGitInfo(info.hash, info.author, info.date)
			}
		}
		for i := range layout.Checks {
			if info, ok := gitMap[layout.Checks[i].AbsPath]; ok {
				layout.Checks[i].CachedFile.preloadGitInfo(info.hash, info.author, info.date)
			}
		}
		return
	}

	// Fallback to slow path if git batched command fails (e.g. testing mocks that don't have a real .git dir)
	type entry struct {
		cf   *CachedFile
		path string
	}
	var entries []entry
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

	workerCount := runtime.GOMAXPROCS(0)
	if workerCount > 32 {
		workerCount = 32
	}

	sem := make(chan struct{}, workerCount)
	var wg sync.WaitGroup

	for _, e := range entries {
		wg.Add(1)
		e := e
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			hash, author, date, err := s.GitInfo(e.path)
			if err == nil {
				e.cf.preloadGitInfo(hash, author, date)
			}
		}()
	}
	wg.Wait()
}
