package fs

import (
	"bufio"
	"context"
	"fmt"
	"os"
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

	type entry struct {
		cf   *CachedFile
		path string
	}
	var entries []entry
	for _, obj := range layout.Objects {
		entries = append(entries, entry{cf: &obj.CachedFile, path: obj.AbsPath})
	}
	for _, ts := range layout.Transitions {
		entries = append(entries, entry{cf: &ts.CachedFile, path: ts.AbsPath})
	}
	for _, cs := range layout.Checks {
		entries = append(entries, entry{cf: &cs.CachedFile, path: cs.AbsPath})
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
