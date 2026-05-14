package scaffold

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type Scaffolder struct{}

func New() *Scaffolder {
	return &Scaffolder{}
}

func (s *Scaffolder) Ensure(ctx context.Context, cfg types.Config, layout fs.Layout, plan *types.MigrationPlan, columns map[string][]db.TableColumn) (bool, error) {
	return s.EnsureTransitionFiles(ctx, cfg, layout, plan, columns)
}

func (s *Scaffolder) EnsureTransitionFiles(ctx context.Context, cfg types.Config, layout fs.Layout, plan *types.MigrationPlan, columns map[string][]db.TableColumn) (bool, error) {
	baseDir := cfg.SQLBase
	created := false
	commit := gitShortHash()

	for _, obj := range plan.Objects {
		if obj.PlannedAction != types.ActionReprocessChangedBlocked {
			continue
		}

		db := obj.DatabaseName
		if db == "" {
			db = cfg.Database
		}

		dir := filepath.Join(baseDir, db, obj.SchemaName, "tables", "_migrations", obj.ObjectName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return created, fmt.Errorf("scaffold: mkdir %s: %w", dir, err)
		}

		if hasExistingTransitionFile(dir) {
			continue
		}

		var fileName, content string
		content, fileName = tryAutoMigration(obj, layout, columns, commit, dir)
		if content == "" {
			fileName = fmt.Sprintf("001_%s_describe_change.sql", commit)
			content = scaffoldContent(obj.SchemaName, obj.ObjectName, columns[obj.NormalizedKey])
		}

		path := filepath.Join(dir, fileName)
		if _, err := os.Stat(path); err == nil {
			continue
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", path, err)
		}

		created = true
	}

	return created, nil
}

func tryAutoMigration(obj types.PlannedObject, layout fs.Layout, columns map[string][]db.TableColumn, commit, dir string) (string, string) {
	fsObj := lookupObjectByKey(layout, obj.NormalizedKey)
	if fsObj == nil {
		return "", ""
	}

	content, err := fsObj.Content()
	if err != nil {
		return "", ""
	}

	dbColumns := columns[obj.NormalizedKey]
	migrationSQL, ok := tryAutoAddColumn(obj.SchemaName, obj.ObjectName, dbColumns, content)
	if !ok {
		return "", ""
	}

	fileName := fmt.Sprintf("001_%s_auto_add_columns.sql", commit)
	if _, err := os.Stat(filepath.Join(dir, fileName)); err == nil {
		return "", ""
	}

	return migrationSQL, fileName
}

func lookupObjectByKey(layout fs.Layout, key string) *fs.Object {
	for _, obj := range layout.Objects {
		if obj.NormalizedKey == key {
			return obj
		}
	}
	return nil
}

func hasExistingTransitionFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			if !strings.Contains(string(data), "-- rmig: transition-scaffold") {
				return true
			}
		}
	}
	return false
}

func gitShortHash() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "0000000"
	}
	return strings.TrimSpace(string(out))
}

func scaffoldContent(schemaName, tableName string, columns []db.TableColumn) string {
	var b strings.Builder
	b.WriteString("-- rmig: transition-scaffold\n")
	b.WriteString(fmt.Sprintf("-- Table: [%s].[%s]\n", schemaName, tableName))
	b.WriteString("-- Replace this scaffold with the actual migration SQL.\n")
	b.WriteString(fmt.Sprintf("-- Schema: %s\n", schemaName))
	b.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))
	b.WriteString("-- Press Ctrl+C to stop migration.\n")
	if len(columns) > 0 {
		b.WriteString("-- Detected columns:\n")
		for _, col := range columns {
			nullable := ""
			if col.Nullable {
				nullable = " NULL"
			} else {
				nullable = " NOT NULL"
			}
			b.WriteString(fmt.Sprintf("--   %s %s%s\n", col.Name, col.TypeName, nullable))
		}
	}
	return b.String()
}
