package scaffold

import (
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

func (s *Scaffolder) EnsureTransitionFiles(baseDir string, layout fs.Layout, plan types.MigrationPlan, columns map[string][]db.TableColumn) ([]string, error) {
	var created []string
	commit := gitShortHash()

	for _, obj := range plan.Objects {
		if obj.PlannedAction != types.ActionReprocessChangedBlocked {
			continue
		}

		dir := filepath.Join(baseDir, obj.SchemaName, "tables", "_migrations", obj.ObjectName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return created, fmt.Errorf("scaffold: mkdir %s: %w", dir, err)
		}

		name := fmt.Sprintf("001_%s_describe_change.sql", commit)
		path := filepath.Join(dir, name)

		if _, err := os.Stat(path); err == nil {
			continue
		}

		if hasExistingTransitionFile(dir) {
			continue
		}

		content := scaffoldContent(obj.SchemaName, obj.ObjectName, columns[obj.NormalizedKey])
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(baseDir, path)
		created = append(created, relPath)
	}

	return created, nil
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
