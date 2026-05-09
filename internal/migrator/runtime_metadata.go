package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
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
	if command == "validate" || command == "repair-checksum" {
		record.ComparisonMode = ""
	}
	if command == "validate" {
		record.UpdatePolicy = ""
	}
	if command == "repair-checksum" {
		record.TransactionMode = config.TransactionModeNone
		record.RollbackScope = "none"
	}
	if err := metadata.StartRun(ctx, execer, record); err != nil {
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
		errorClass = classifyError(base, cause)
		errorMessage = buildErrorMessage(base, cause)
	}
	return metadata.FinishRun(ctx, execer, runID, success, errorClass, errorMessage)
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

func classifyError(base error, cause error) string {
	message := strings.ToLower(buildErrorMessage(base, cause))
	switch {
	case strings.Contains(message, "approved plan missing"):
		return "approved plan missing"
	case strings.Contains(message, "approved plan mismatch"):
		return "approved plan mismatch"
	case strings.Contains(message, "missing_metadata_ddl_permission") || strings.Contains(message, "missing metadata ddl permission"):
		return "missing metadata DDL permission"
	case strings.Contains(message, "missing schema creation permission"):
		return "missing schema creation permission"
	case strings.Contains(message, "missing object ddl permission"):
		return "missing object DDL permission"
	case strings.Contains(message, "missing parent object"):
		return "missing parent object"
	case strings.Contains(message, "metadata_schema_incompatible") || strings.Contains(message, "metadata schema incompatible"):
		return "metadata schema incompatible"
	case strings.Contains(message, "invalid repository layout"):
		return "invalid repository layout"
	case strings.Contains(message, "invalid_or_missing_sql_scripts_root"):
		return "invalid or missing SQL scripts root"
	case strings.Contains(message, "invalid_or_missing_base_selection"):
		return "invalid or missing base selection"
	case strings.Contains(message, "invalid_update_policy"):
		return "invalid update policy"
	case strings.Contains(message, "invalid_transaction_mode"):
		return "invalid transaction mode"
	case errors.Is(base, contracts.ErrValidation):
		return "validation failure"
	case errors.Is(base, contracts.ErrSQLExecution):
		return "sql execution failure"
	case errors.Is(base, contracts.ErrCriticalState):
		return "critical metadata state"
	default:
		return "critical metadata state"
	}
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
	for _, schema := range plan.Schemas {
		exists := schema.Exists
		record := metadata.TrackedSchemaRecord{
			RunID:                runID,
			SchemaName:           schema.SchemaName,
			NormalizedSchemaName: strings.ToLower(schema.SchemaName),
			ExistsInDatabase:     boolPtr(exists),
			Action:               schema.Action,
		}
		if schema.Action == "exists" {
			record.Success = boolPtr(true)
		}
		if err := metadata.InsertTrackedSchema(ctx, conn, record); err != nil {
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
		case "skip_unchanged", "adopt_existing":
			record.Success = boolPtr(true)
		case "fail", "reprocess_changed_blocked":
			record.Success = boolPtr(false)
		}
		id, err := metadata.InsertTrackedObject(ctx, conn, record)
		if err != nil {
			return nil, err
		}
		trackedObjectIDs[object.NormalizedKey] = id
	}
	return trackedObjectIDs, nil
}

func persistValidationScope(ctx context.Context, conn *sql.Conn, runID string, layout parser.Layout, catalog validate.CatalogState, successfulByKey map[string]string) (map[string]int64, error) {
	for _, schema := range layout.Schemas {
		_, exists := catalog.Schemas[schema.NormalizedName]
		action := "exists"
		record := metadata.TrackedSchemaRecord{
			RunID:                runID,
			SchemaName:           schema.Name,
			NormalizedSchemaName: schema.NormalizedName,
			ExistsInDatabase:     boolPtr(exists),
			Action:               action,
			Success:              boolPtr(exists),
		}
		if !exists {
			record.Action = "fail"
			record.ErrorMessage = fmt.Sprintf("missing schema: %s", schema.Name)
		}
		if err := metadata.InsertTrackedSchema(ctx, conn, record); err != nil {
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
			PlannedAction:        "validate_checked",
		}
		if !exists {
			record.Success = boolPtr(false)
			record.ErrorMessage = fmt.Sprintf("missing managed object: %s", object.Path)
		}
		id, err := metadata.InsertTrackedObject(ctx, conn, record)
		if err != nil {
			return nil, err
		}
		trackedObjectIDs[object.NormalizedKey] = id
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
