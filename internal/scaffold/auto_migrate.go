package scaffold

import (
	"fmt"
	"strings"

	"reporting-db-migrations/internal/db"
)

func tryAutoAddColumn(schemaName, tableName string, dbColumns []db.TableColumn, fileContent string) (string, bool) {
	fileCols := parseTableColumns(fileContent)
	if len(fileCols) == 0 {
		return "", false
	}

	dbNames := make(map[string]bool, len(dbColumns))
	for _, c := range dbColumns {
		dbNames[strings.ToLower(c.Name)] = true
	}

	added := newColumns(fileCols, dbNames)
	if len(added) == 0 {
		return "", false
	}

	if hasDroppedColumns(fileCols, dbNames) {
		return "", false
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("-- Auto-generated migration for [%s].[%s]\n", schemaName, tableName))
	b.WriteString(fmt.Sprintf("-- Added columns: %d\n", len(added)))
	b.WriteString("-- Review this migration before running.\n\n")

	for _, col := range added {
		nullable := "NULL"
		if !col.nullable {
			nullable = "NOT NULL"
		}
		b.WriteString(fmt.Sprintf("ALTER TABLE [%s].[%s] ADD [%s] %s %s;\n",
			schemaName, tableName, col.name, col.typeName, nullable))
	}

	return b.String(), true
}
