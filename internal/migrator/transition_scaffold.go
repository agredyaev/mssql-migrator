package migrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

func ensureTableTransitionFiles(cfg config.Config, layout parser.Layout, plan contracts.MigrationPlan, tableColumns map[string][]catalog.TableColumn) (bool, error) {
	basePath := strings.TrimSpace(cfg.SelectedBasePath())
	if basePath == "" {
		return false, nil
	}
	objectsByKey := make(map[string]parser.Object, len(layout.Objects))
	for _, object := range layout.Objects {
		objectsByKey[object.NormalizedKey] = object
	}
	created := false
	for _, object := range plan.Objects {
		if object.Kind != "tables" || object.PlannedAction != contracts.ActionReprocessChangedBlocked || len(object.TransitionPaths) != 0 {
			continue
		}
		layoutObject, ok := objectsByKey[object.NormalizedKey]
		if !ok {
			continue
		}
		if wrote, err := ensureAutomaticAddColumnMigration(basePath, cfg.GitCommit, layoutObject, tableColumns[object.NormalizedKey]); err != nil {
			return created, err
		} else if wrote {
			created = true
			continue
		}
		wrote, err := ensureTransitionScaffoldFile(basePath, cfg.GitCommit, object)
		if err != nil {
			return created, err
		}
		if wrote {
			created = true
		}
	}
	return created, nil
}

func ensureAutomaticAddColumnMigration(basePath string, gitCommit string, object parser.Object, liveColumns []catalog.TableColumn) (bool, error) {
	columnDefs, ok := automaticAddColumnDefinitions(object, liveColumns)
	if !ok || len(columnDefs) == 0 {
		return false, nil
	}
	dir := filepath.Join(basePath, object.SchemaName, "tables", "_migrations", object.ObjectName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create transition dir for %s: %w", object.Path, err)
	}
	commitToken := parser.TransitionCommitToken(gitCommit)
	if commitToken == "" {
		commitToken = "0000000"
	}
	path := filepath.Join(dir, "001_"+commitToken+"_auto_add_columns.sql")
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat automatic transition for %s: %w", object.Path, err)
	}
	statements := make([]string, 0, len(columnDefs))
	for _, columnDef := range columnDefs {
		statements = append(statements, fmt.Sprintf("ALTER TABLE [%s].[%s] ADD %s;", object.SchemaName, object.ObjectName, columnDef))
	}
	content := strings.Join(statements, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write automatic transition for %s: %w", object.Path, err)
	}
	return true, nil
}

func ensureTransitionScaffoldFile(basePath string, gitCommit string, object contracts.PlannedObject) (bool, error) {
	dir := filepath.Join(basePath, object.SchemaName, "tables", "_migrations", object.ObjectName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create transition scaffold dir for %s: %w", object.ObjectPath, err)
	}
	commitToken := parser.TransitionCommitToken(gitCommit)
	if commitToken == "" {
		commitToken = "0000000"
	}
	path := filepath.Join(dir, "001_"+commitToken+"_describe_change.sql")
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat transition scaffold for %s: %w", object.ObjectPath, err)
	}
	content := transitionScaffoldContent(object, commitToken)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write transition scaffold for %s: %w", object.ObjectPath, err)
	}
	return true, nil
}

func transitionScaffoldContent(object contracts.PlannedObject, commitToken string) string {
	return strings.Join([]string{
		parser.TransitionScaffoldDirective,
		"-- Replace this scaffold with the SQL needed to move the live table to the repo shape.",
		"-- Table: " + object.SchemaName + "." + object.ObjectName,
		"-- Commit token: " + commitToken,
		"-- Remove the scaffold directive after you add real transition SQL.",
		"",
	}, "\n")
}

func automaticAddColumnDefinitions(object parser.Object, liveColumns []catalog.TableColumn) ([]string, bool) {
	repoColumns, err := parser.ParseCreateTableColumns(object.Content)
	if err != nil || len(liveColumns) == 0 {
		return nil, false
	}
	liveByName := make(map[string]catalog.TableColumn, len(liveColumns))
	for _, column := range liveColumns {
		liveByName[column.NormalizedName] = column
	}
	missing := make([]string, 0)
	for _, column := range repoColumns {
		if live, exists := liveByName[column.NormalizedName]; exists {
			if !sameTableColumnSignature(column, live) {
				return nil, false
			}
			continue
		}
		if !column.AutoAddEligible {
			return nil, false
		}
		missing = append(missing, column.DefinitionSQL)
	}
	if len(missing) == 0 {
		return nil, false
	}
	return missing, true
}

func sameTableColumnSignature(repo parser.TableColumn, live catalog.TableColumn) bool {
	if !repo.SignatureKnown {
		return false
	}
	if repo.TypeName != live.TypeName {
		return false
	}
	if repo.NullableKnown && repo.Nullable != live.Nullable {
		return false
	}
	switch repo.TypeName {
	case "char", "varchar", "nchar", "nvarchar", "binary", "varbinary":
		return repo.Length == live.Length
	case "decimal", "numeric":
		return repo.Precision == live.Precision && repo.Scale == live.Scale
	case "datetime2", "time", "datetimeoffset":
		return repo.Scale == live.Scale
	default:
		return true
	}
}
