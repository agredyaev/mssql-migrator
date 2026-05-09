package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/validate"
)

func (r Runner) startRun(ctx context.Context, execer metadata.Execer, command string, planFile string, planHash string, rollbackScope string) (string, error) {
	runID := uuid.NewString()
	record := metadata.RunRecord{
		RunID:             runID,
		Command:           command,
		ToolVersion:       r.cfg.ToolVersion,
		ToolCommit:        r.cfg.ToolCommit,
		SQLRoot:           r.cfg.SQLRoot,
		BaseName:          r.cfg.SQLBase,
		EffectiveBasePath: r.cfg.SelectedBasePath(),
		TargetEnvironment: r.cfg.Env,
		TargetDatabase:    r.cfg.Database,
		GitCommit:         r.cfg.GitCommit,
		GitBranch:         r.cfg.GitBranch,
		PipelineRunID:     r.cfg.PipelineRunID,
		PipelineURL:       logger.Redact(r.cfg.PipelineURL),
		AppliedBy:         r.cfg.Actor,
		ComparisonMode:    config.ComparisonModeCaseInsensitive,
		UpdatePolicy:      r.cfg.UpdatePolicy,
		TransactionMode:   r.cfg.TransactionMode,
		RollbackScope:     rollbackScope,
		PlanFile:          planFile,
		PlanHash:          planHash,
	}
	if command == contracts.CommandValidate || command == contracts.CommandRepairChecksum {
		record.ComparisonMode = ""
	}
	if command == contracts.CommandValidate {
		record.UpdatePolicy = ""
	}
	if command == contracts.CommandRepairChecksum {
		record.TransactionMode = config.TransactionModeNone
		record.RollbackScope = contracts.RollbackScopeNone
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	if err := metadata.StartRun(metaCtx, execer, record); err != nil {
		return "", err
	}
	return runID, nil
}

func finishRun(ctx context.Context, execer metadata.Execer, runID string, success bool, base error, cause error) error {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	errorClass := ""
	errorMessage := ""
	if !success {
		errorClass = failures.Classify(base, cause)
		errorMessage = buildErrorMessage(base, cause)
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.FinishRun(metaCtx, execer, runID, success, errorClass, errorMessage)
}

func buildErrorMessage(base error, cause error) string {
	if base == nil && cause == nil {
		return ""
	}
	if base == nil {
		return logger.Redact(cause.Error())
	}
	message := logger.Redact(base.Error())
	if cause != nil {
		message += ": " + logger.Redact(cause.Error())
	}
	return message
}

func planArtifactHash(planFile string) string {
	if strings.TrimSpace(planFile) == "" {
		return ""
	}
	hash, err := checksum.SHA256File(planFile)
	if err != nil {
		return ""
	}
	return hash
}

func persistMigrationScope(ctx context.Context, conn *sql.Conn, runID string, plan contracts.MigrationPlan) (map[string]int64, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin metadata scope transaction: %w", err)
	}
	for _, schema := range plan.Schemas {
		exists := schema.Exists
		record := metadata.TrackedSchemaRecord{
			RunID:                runID,
			SchemaName:           schema.SchemaName,
			NormalizedSchemaName: strings.ToLower(schema.SchemaName),
			ExistsInDatabase:     boolPtr(exists),
			Action:               schema.Action,
		}
		if schema.Action == contracts.SchemaActionExists {
			record.Success = boolPtr(true)
		}
		if err := metadata.InsertTrackedSchema(ctx, tx, record); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	trackedObjectIDs := map[string]int64{}
	for _, object := range plan.Objects {
		exists := object.Exists
		record := metadata.TrackedObjectRecord{
			RunID:                runID,
			ObjectPath:           object.ObjectPath,
			SchemaName:           object.SchemaName,
			NormalizedSchemaName: strings.ToLower(object.SchemaName),
			Kind:                 object.Kind,
			ObjectName:           object.ObjectName,
			NormalizedObjectName: strings.ToLower(object.ObjectName),
			ParentName:           object.ParentName,
			NormalizedParentName: strings.ToLower(object.ParentName),
			NormalizedKey:        object.NormalizedKey,
			Checksum:             object.Checksum,
			ExistsInDatabase:     boolPtr(exists),
			MetadataMatch:        object.MetadataMatch,
			PlannedAction:        object.PlannedAction,
		}
		switch object.PlannedAction {
		case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting:
			record.Success = boolPtr(true)
		case contracts.ActionFail, contracts.ActionReprocessChangedBlocked:
			record.Success = boolPtr(false)
		}
		id, err := metadata.InsertTrackedObject(ctx, tx, record)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		trackedObjectIDs[object.NormalizedKey] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit metadata scope transaction: %w", err)
	}
	return trackedObjectIDs, nil
}

func persistValidationScope(ctx context.Context, conn *sql.Conn, runID string, layout parser.Layout, catalog validate.CatalogState, successfulByKey map[string]string) (map[string]int64, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin validation scope transaction: %w", err)
	}
	for _, schema := range layout.Schemas {
		_, exists := catalog.Schemas[schema.NormalizedName]
		action := contracts.SchemaActionExists
		record := metadata.TrackedSchemaRecord{
			RunID:                runID,
			SchemaName:           schema.Name,
			NormalizedSchemaName: schema.NormalizedName,
			ExistsInDatabase:     boolPtr(exists),
			Action:               action,
			Success:              boolPtr(exists),
		}
		if !exists {
			record.Action = contracts.ActionFail
			record.ErrorMessage = fmt.Sprintf("missing schema: %s", schema.Name)
		}
		if err := metadata.InsertTrackedSchema(ctx, tx, record); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	trackedObjectIDs := map[string]int64{}
	for _, object := range layout.Objects {
		_, exists := catalog.Objects[object.NormalizedKey]
		var metadataMatch *bool
		if checksum, ok := successfulByKey[object.NormalizedKey]; ok {
			metadataMatch = boolPtr(checksum == object.Checksum)
		}
		record := metadata.TrackedObjectRecord{
			RunID:                runID,
			ObjectPath:           object.Path,
			SchemaName:           object.SchemaName,
			NormalizedSchemaName: object.NormalizedSchemaName,
			Kind:                 object.Kind,
			ObjectName:           object.ObjectName,
			NormalizedObjectName: object.NormalizedObjectName,
			ParentName:           object.ParentName,
			NormalizedParentName: object.NormalizedParentName,
			NormalizedKey:        object.NormalizedKey,
			Checksum:             object.Checksum,
			ExistsInDatabase:     boolPtr(exists),
			MetadataMatch:        metadataMatch,
			PlannedAction:        contracts.ActionValidateChecked,
		}
		if !exists {
			record.Success = boolPtr(false)
			record.ErrorMessage = fmt.Sprintf("missing managed object: %s", object.Path)
		}
		id, err := metadata.InsertTrackedObject(ctx, tx, record)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		trackedObjectIDs[object.NormalizedKey] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit validation scope transaction: %w", err)
	}
	return trackedObjectIDs, nil
}

func loadTrackedObjectIDs(ctx context.Context, conn *sql.Conn, runID string) (map[string]int64, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT normalized_key, tracked_object_id
FROM __migrator.tracked_objects
WHERE run_id = @p1`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var normalizedKey string
		var trackedObjectID int64
		if err := rows.Scan(&normalizedKey, &trackedObjectID); err != nil {
			return nil, err
		}
		result[normalizedKey] = trackedObjectID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func boolPtr(value bool) *bool {
	return &value
}
