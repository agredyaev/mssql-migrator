package migrator

import (
	"context"
	"database/sql"
	"fmt"
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

func (w metadataWriter) updateItemResult(ctx context.Context, normalizedKey string, success bool, errorMessage string) error {
	if w.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateItemResult(metaCtx, w.execer, w.runID, normalizedKey, success, errorMessage)
}

func (w metadataWriter) updateSchema(ctx context.Context, normalizedSchemaName string, success bool, errorMessage string) error {
	return w.updateItemResult(ctx, normalizedSchemaName, success, errorMessage)
}

func (w metadataWriter) updateObject(ctx context.Context, normalizedKey string, success bool, errorMessage string) error {
	return w.updateItemResult(ctx, normalizedKey, success, errorMessage)
}

func (w metadataWriter) updateObjectResults(ctx context.Context, results []metadata.ItemResult) error {
	if w.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateItemResults(metaCtx, w.execer, w.runID, results)
}

func (w metadataWriter) insertAttempt(ctx context.Context, attempt metadata.AttemptRecord) error {
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	attempt.RunID = w.runID
	return metadata.InsertAttempt(metaCtx, w.execer, attempt)
}

func (w metadataWriter) loadObjectItemIDs(ctx context.Context, conn *sql.Conn) (map[string]int64, error) {
	return metadata.LoadItemIDs(ctx, conn, w.runID, false)
}

func (w metadataWriter) loadItemIDs(ctx context.Context, queryer metadata.Queryer, includeSchemas bool) (map[string]int64, error) {
	if strings.TrimSpace(w.runID) == "" {
		return nil, fmt.Errorf("load item ids: missing run id")
	}
	return metadata.LoadItemIDs(ctx, queryer, w.runID, includeSchemas)
}
