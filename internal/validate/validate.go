package validate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

type objectRef struct {
	Schema string
	Name   string
	Type   string
}

func RefreshModules(ctx context.Context, conn *sql.Conn, cfg config.Config, log logger.Logger) (contracts.ValidationSummary, error) {
	objects, err := listManagedObjects(ctx, conn, cfg.ManagedSchemas)
	if err != nil {
		return contracts.ValidationSummary{}, err
	}

	summary := contracts.ValidationSummary{}
	for _, object := range objects {
		qualifiedName := bracket(object.Schema) + "." + bracket(object.Name)
		if _, err := conn.ExecContext(ctx, "EXEC sys.sp_refreshsqlmodule @p1", qualifiedName); err != nil {
			return summary, fmt.Errorf("refresh module %s: %w", qualifiedName, err)
		}
		if object.Type == "V" {
			if _, err := conn.ExecContext(ctx, "EXEC sys.sp_refreshview @p1", qualifiedName); err != nil {
				return summary, fmt.Errorf("refresh view %s: %w", qualifiedName, err)
			}
		}
		summary.ModulesRefreshed++
	}

	log.Info("modules_refreshed", fmt.Sprintf("count=%d", summary.ModulesRefreshed))
	return summary, nil
}

func RunChecks(ctx context.Context, conn *sql.Conn, sqlDir string) (contracts.ValidationSummary, error) {
	_, _, checks, err := parser.Discover(sqlDir)
	if err != nil {
		return contracts.ValidationSummary{}, err
	}

	summary := contracts.ValidationSummary{}
	for _, script := range checks {
		if err := runCheck(ctx, conn, script); err != nil {
			summary.ChecksFailed++
			return summary, fmt.Errorf("check %s failed: %w", script.Name, err)
		}
		summary.ChecksPassed++
	}
	return summary, nil
}

func runCheck(ctx context.Context, conn *sql.Conn, script parser.Script) error {
	content, err := os.ReadFile(script.Path)
	if err != nil {
		return err
	}
	batches, err := parser.SplitGO(string(content))
	if err != nil {
		return err
	}
	for _, batch := range batches {
		for i := 0; i < batch.Repeat; i++ {
			if _, err := conn.ExecContext(ctx, batch.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func listManagedObjects(ctx context.Context, conn *sql.Conn, schemas []string) ([]objectRef, error) {
	quoted := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		quoted = append(quoted, "'"+strings.ReplaceAll(schema, "'", "''")+"'")
	}

	query := fmt.Sprintf(
		`SELECT s.name, o.name, o.type FROM sys.objects o JOIN sys.schemas s ON s.schema_id=o.schema_id WHERE s.name IN (%s) AND o.type IN ('V','P','FN','IF','TF') ORDER BY s.name,o.name`,
		strings.Join(quoted, ","),
	)
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	objects := []objectRef{}
	for rows.Next() {
		object := objectRef{}
		if err := rows.Scan(&object.Schema, &object.Name, &object.Type); err != nil {
			return nil, err
		}
		objects = append(objects, object)
	}
	return objects, rows.Err()
}

func bracket(value string) string {
	return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
}
