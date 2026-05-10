package migrator

import (
	"context"
	"database/sql"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/metadata"
)

type metadataWriter struct {
	cfg    config.Config
	execer metadata.Execer
	runID  string
}

func newMetadataWriter(cfg config.Config, execer metadata.Execer, runID string) metadataWriter {
	return metadataWriter{cfg: cfg, execer: execer, runID: strings.TrimSpace(runID)}
}

func (w metadataWriter) finishRun(ctx context.Context, success bool, base error, cause error) error {
	return finishRun(ctx, w.execer, w.runID, success, base, cause)
}

func (w metadataWriter) updateSchema(ctx context.Context, normalizedSchemaName string, success bool, errorMessage string) error {
	if w.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateItemResult(metaCtx, w.execer, w.runID, metadata.ItemTypeSchema, normalizedSchemaName, success, errorMessage)
}

func (w metadataWriter) updateObject(ctx context.Context, normalizedKey string, success bool, errorMessage string) error {
	if w.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateItemResult(metaCtx, w.execer, w.runID, metadata.ItemTypeObject, normalizedKey, success, errorMessage)
}

func (w metadataWriter) insertAttempt(ctx context.Context, attempt metadata.AttemptRecord) error {
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	attempt.RunID = w.runID
	return metadata.InsertAttempt(metaCtx, w.execer, attempt)
}

func (w metadataWriter) loadObjectItemIDs(ctx context.Context, conn *sql.Conn) (map[string]int64, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT normalized_key, item_id
FROM __migrator.items
WHERE run_id = @p1 AND item_type = @p2`, w.runID, metadata.ItemTypeObject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var normalizedKey string
		var itemID int64
		if err := rows.Scan(&normalizedKey, &itemID); err != nil {
			return nil, err
		}
		result[normalizedKey] = itemID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
