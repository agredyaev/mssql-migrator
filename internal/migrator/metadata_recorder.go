package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
	"reporting-db-migrations/internal/validate"
)

type metadataRecorder struct {
	cfg    config.Config
	execer metadata.Execer
	runID  string
}

func newMetadataRecorder(cfg config.Config, execer metadata.Execer, runID string) metadataRecorder {
	return metadataRecorder{cfg: cfg, execer: execer, runID: strings.TrimSpace(runID)}
}

func (r metadataRecorder) recordRunResult(ctx context.Context, success bool, base error, cause error) error {
	return finishRun(ctx, r.execer, r.runID, success, base, cause)
}

func (r metadataRecorder) updateSchemaItemResult(ctx context.Context, normalizedSchemaName string, success bool, errorMessage string) error {
	if r.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateItemResult(metaCtx, r.execer, r.runID, metadata.ItemTypeSchema, normalizedSchemaName, success, errorMessage)
}

func (r metadataRecorder) updateObjectItemResult(ctx context.Context, normalizedKey string, success bool, errorMessage string) error {
	if r.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateItemResult(metaCtx, r.execer, r.runID, metadata.ItemTypeObject, normalizedKey, success, errorMessage)
}

func (r metadataRecorder) insertAttempt(ctx context.Context, attempt metadata.AttemptRecord) error {
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	attempt.RunID = r.runID
	return metadata.InsertAttempt(metaCtx, r.execer, attempt)
}

func (r metadataRecorder) recordSchemaSuccess(ctx context.Context, schemaName string, action string, writeAttempt bool) error {
	normalizedSchemaName := strings.ToLower(strings.TrimSpace(schemaName))
	if err := r.updateSchemaItemResult(ctx, normalizedSchemaName, true, ""); err != nil {
		return err
	}
	if !writeAttempt {
		return nil
	}
	return r.insertAttempt(ctx, metadata.AttemptRecord{
		ScriptName:    schemaName,
		ScriptType:    contracts.ScriptTypeSchema,
		Checksum:      "-",
		Action:        action,
		ExecutionMS:   0,
		Success:       true,
		GitCommit:     r.cfg.GitCommit,
		GitBranch:     r.cfg.GitBranch,
		PipelineRunID: r.cfg.PipelineRunID,
		PipelineURL:   logger.Redact(r.cfg.PipelineURL),
		AppliedBy:     r.cfg.Actor,
	})
}

func (r metadataRecorder) recordSchemaFailure(ctx context.Context, schemaName string, failure error, writeAttempt bool) error {
	message := logger.Redact(failure.Error())
	normalizedSchemaName := strings.ToLower(strings.TrimSpace(schemaName))
	if err := r.updateSchemaItemResult(ctx, normalizedSchemaName, false, message); err != nil {
		return err
	}
	if !writeAttempt {
		return nil
	}
	return r.insertAttempt(ctx, metadata.AttemptRecord{
		ScriptName:    schemaName,
		ScriptType:    contracts.ScriptTypeSchema,
		Checksum:      "-",
		Action:        contracts.ActionFail,
		ExecutionMS:   0,
		Success:       false,
		ErrorMessage:  message,
		GitCommit:     r.cfg.GitCommit,
		GitBranch:     r.cfg.GitBranch,
		PipelineRunID: r.cfg.PipelineRunID,
		PipelineURL:   logger.Redact(r.cfg.PipelineURL),
		AppliedBy:     r.cfg.Actor,
	})
}

func (r metadataRecorder) recordObjectSuccess(ctx context.Context, object plannedMetadataObject, writeAttempt bool) error {
	if writeAttempt {
		if err := r.insertAttempt(ctx, object.successAttempt(r.cfg)); err != nil {
			return fmt.Errorf("critical metadata failure after %s: database object state may drift from metadata: %w", object.action(), err)
		}
	}
	return r.updateObjectItemResult(ctx, object.normalizedKey(), true, "")
}

func (r metadataRecorder) recordObjectFailure(ctx context.Context, object plannedMetadataObject, failure error, writeAttempt bool) error {
	message := logger.Redact(failure.Error())
	if writeAttempt {
		attempt := object.failureAttempt(r.cfg, message)
		if err := r.insertAttempt(ctx, attempt); err != nil {
			return err
		}
	}
	return r.updateObjectItemResult(ctx, object.normalizedKey(), false, message)
}

func (r metadataRecorder) recordValidationFailure(ctx context.Context, objects []parser.Object, itemIDs map[string]int64, validationErr error, includeChecks bool, log logger.Logger) {
	errorMessage := logger.Redact(validationErr.Error())
	for _, object := range objects {
		planned := validationMetadataObject(object, lookupItemID(itemIDs, object.NormalizedKey))
		if err := r.recordObjectFailure(ctx, planned, validationErr, true); err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
	if includeChecks {
		if err := r.insertAttempt(ctx, metadata.AttemptRecord{
			ScriptName:       "validation/checks",
			ScriptType:       contracts.ScriptTypeValidate,
			Checksum:         "-",
			Action:           contracts.ActionFail,
			ExecutionMS:      0,
			Success:          false,
			ErrorMessage:     errorMessage,
			TransactionMode:  config.TransactionModeNone,
			TransactionScope: config.TransactionModeNone,
			RollbackScope:    contracts.RollbackScopeNone,
			NoTransaction:    true,
			GitCommit:        r.cfg.GitCommit,
			GitBranch:        r.cfg.GitBranch,
			PipelineRunID:    r.cfg.PipelineRunID,
			PipelineURL:      logger.Redact(r.cfg.PipelineURL),
			AppliedBy:        r.cfg.Actor,
		}); err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
}

func (r metadataRecorder) persistMigrationScope(ctx context.Context, conn *sql.Conn, plan contracts.MigrationPlan) (map[string]int64, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("persist migration scope: missing run id")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin metadata scope transaction: %w", err)
	}
	for _, schema := range plan.Schemas {
		exists := schema.Exists
		record := metadata.ItemRecord{
			RunID:                r.runID,
			ItemType:             metadata.ItemTypeSchema,
			SchemaName:           schema.SchemaName,
			NormalizedSchemaName: strings.ToLower(schema.SchemaName),
			NormalizedKey:        strings.ToLower(schema.SchemaName),
			ExistsInDatabase:     boolPtr(exists),
			Action:               schema.Action,
		}
		if schema.Action == contracts.SchemaActionExists {
			record.Success = boolPtr(true)
		}
		if _, err := metadata.InsertItem(ctx, tx, record); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	itemIDs := map[string]int64{}
	for _, object := range plan.Objects {
		exists := object.Exists
		record := metadata.ItemRecord{
			RunID:                r.runID,
			ItemType:             metadata.ItemTypeObject,
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
			Action:               object.PlannedAction,
		}
		switch object.PlannedAction {
		case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting:
			record.Success = boolPtr(true)
		case contracts.ActionFail, contracts.ActionReprocessChangedBlocked:
			record.Success = boolPtr(false)
		}
		id, err := metadata.InsertItem(ctx, tx, record)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		itemIDs[object.NormalizedKey] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit metadata scope transaction: %w", err)
	}
	return itemIDs, nil
}

func (r metadataRecorder) persistValidationScope(ctx context.Context, conn *sql.Conn, layout parser.Layout, catalog validate.CatalogState, successfulByKey map[string]string) (map[string]int64, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("persist validation scope: missing run id")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin validation scope transaction: %w", err)
	}
	for _, schema := range layout.Schemas {
		_, exists := catalog.Schemas[schema.NormalizedName]
		action := contracts.SchemaActionExists
		record := metadata.ItemRecord{
			RunID:                r.runID,
			ItemType:             metadata.ItemTypeSchema,
			SchemaName:           schema.Name,
			NormalizedSchemaName: schema.NormalizedName,
			NormalizedKey:        schema.NormalizedName,
			ExistsInDatabase:     boolPtr(exists),
			Action:               action,
			Success:              boolPtr(exists),
		}
		if !exists {
			record.Action = contracts.ActionFail
			record.ErrorMessage = fmt.Sprintf("missing schema: %s", schema.Name)
		}
		if _, err := metadata.InsertItem(ctx, tx, record); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	itemIDs := map[string]int64{}
	for _, object := range layout.Objects {
		_, exists := catalog.Objects[object.NormalizedKey]
		var metadataMatch *bool
		if checksum, ok := successfulByKey[object.NormalizedKey]; ok {
			metadataMatch = boolPtr(checksum == object.Checksum)
		}
		record := metadata.ItemRecord{
			RunID:                r.runID,
			ItemType:             metadata.ItemTypeObject,
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
			Action:               contracts.ActionValidateChecked,
		}
		if !exists {
			record.Success = boolPtr(false)
			record.ErrorMessage = fmt.Sprintf("missing managed object: %s", object.Path)
		}
		id, err := metadata.InsertItem(ctx, tx, record)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		itemIDs[object.NormalizedKey] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit validation scope transaction: %w", err)
	}
	return itemIDs, nil
}

func (r metadataRecorder) persistRepairScope(ctx context.Context, conn *sql.Conn, object parser.Object, planned contracts.PlannedObject, catalog planner.CatalogState, currentChecksum string) (*int64, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("persist repair scope: missing run id")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repair scope transaction: %w", err)
	}
	_, schemaExists := catalog.Schemas[object.NormalizedSchemaName]
	if _, err := metadata.InsertItem(ctx, tx, metadata.ItemRecord{
		RunID:                r.runID,
		ItemType:             metadata.ItemTypeSchema,
		SchemaName:           object.SchemaName,
		NormalizedSchemaName: object.NormalizedSchemaName,
		NormalizedKey:        object.NormalizedSchemaName,
		ExistsInDatabase:     boolPtr(schemaExists),
		Action:               contracts.SchemaActionExists,
		Success:              boolPtr(schemaExists),
	}); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	metadataMatch := currentChecksum == object.Checksum
	itemID, err := metadata.InsertItem(ctx, tx, metadata.ItemRecord{
		RunID:                r.runID,
		ItemType:             metadata.ItemTypeObject,
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
		ExistsInDatabase:     boolPtr(true),
		MetadataMatch:        boolPtr(metadataMatch),
		Action:               planned.PlannedAction,
	})
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repair scope transaction: %w", err)
	}
	return &itemID, nil
}

func (r metadataRecorder) loadObjectItemIDs(ctx context.Context, conn *sql.Conn) (map[string]int64, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT normalized_key, item_id
FROM __migrator.items
WHERE run_id = @p1 AND item_type = @p2`, r.runID, metadata.ItemTypeObject)
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

func (r metadataRecorder) recordValidationSuccesses(ctx context.Context, objects []parser.Object) error {
	for _, object := range objects {
		if err := r.updateObjectItemResult(ctx, object.NormalizedKey, true, ""); err != nil {
			return err
		}
	}
	return nil
}

func (r metadataRecorder) recordRepairSuccess(ctx context.Context, object parser.Object, itemID *int64) error {
	if err := r.insertAttempt(ctx, metadata.AttemptRecord{
		ItemID:           itemID,
		ScriptName:       object.NormalizedKey,
		ScriptType:       contracts.ScriptTypeRepair,
		Checksum:         object.Checksum,
		Action:           contracts.ActionRepairChecksum,
		ExecutionMS:      0,
		Success:          true,
		TransactionMode:  config.TransactionModeNone,
		TransactionScope: config.TransactionModeNone,
		RollbackScope:    contracts.RollbackScopeNone,
		NoTransaction:    true,
		GitCommit:        r.cfg.GitCommit,
		GitBranch:        r.cfg.GitBranch,
		PipelineRunID:    r.cfg.PipelineRunID,
		PipelineURL:      logger.Redact(r.cfg.PipelineURL),
		AppliedBy:        r.cfg.Actor,
	}); err != nil {
		return err
	}
	return r.updateObjectItemResult(ctx, object.NormalizedKey, true, "")
}

type plannedMetadataObject struct {
	objectPath       string
	kind             string
	normalizedKeyVal string
	checksum         string
	scriptType       string
	actionValue      string
	itemID           *int64
	executionMS      int
	transactionMode  string
	transactionScope string
	rollbackScope    string
	noTransaction    bool
}

func passiveMetadataObject(planned contracts.PlannedObject, itemID *int64) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       planned.ObjectPath,
		kind:             planned.Kind,
		normalizedKeyVal: planned.NormalizedKey,
		checksum:         planned.Checksum,
		scriptType:       contracts.ScriptTypeObject,
		actionValue:      planned.PlannedAction,
		itemID:           itemID,
		transactionMode:  planned.TransactionMode,
		transactionScope: planned.TransactionMode,
		rollbackScope:    planned.RollbackScope,
		noTransaction:    planned.NoTransaction,
	}
}

func executedMetadataObject(object parser.Object, planned contracts.PlannedObject, itemID *int64, executionMS int) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       object.Path,
		kind:             object.Kind,
		normalizedKeyVal: object.NormalizedKey,
		checksum:         object.Checksum,
		scriptType:       contracts.ScriptTypeObject,
		actionValue:      planned.PlannedAction,
		itemID:           itemID,
		executionMS:      executionMS,
		transactionMode:  planned.TransactionMode,
		transactionScope: planned.TransactionMode,
		rollbackScope:    planned.RollbackScope,
		noTransaction:    planned.NoTransaction,
	}
}

func validationMetadataObject(object parser.Object, itemID *int64) plannedMetadataObject {
	action := contracts.ActionValidateChecked
	if !parser.IsModuleKind(object.Kind) {
		action = contracts.ActionValidateSkipped
	}
	return plannedMetadataObject{
		objectPath:       object.Path,
		kind:             object.Kind,
		normalizedKeyVal: object.NormalizedKey,
		checksum:         object.Checksum,
		scriptType:       contracts.ScriptTypeValidate,
		actionValue:      action,
		itemID:           itemID,
		executionMS:      0,
		transactionMode:  config.TransactionModeNone,
		transactionScope: config.TransactionModeNone,
		rollbackScope:    contracts.RollbackScopeNone,
		noTransaction:    true,
	}
}

func baselineFailureMetadataObject(planned contracts.PlannedObject, itemID *int64, defaultMode string) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       planned.ObjectPath,
		kind:             planned.Kind,
		normalizedKeyVal: planned.NormalizedKey,
		checksum:         planned.Checksum,
		scriptType:       contracts.ScriptTypeObject,
		actionValue:      planned.PlannedAction,
		itemID:           itemID,
		executionMS:      0,
		transactionMode:  contracts.TransactionModeForObject(defaultMode, false),
		transactionScope: contracts.TransactionModeForObject(defaultMode, false),
		rollbackScope:    contracts.RollbackScope(defaultMode),
		noTransaction:    contracts.NoTransactionForObject(defaultMode, false),
	}
}

func (o plannedMetadataObject) action() string {
	return o.actionValue
}

func (o plannedMetadataObject) normalizedKey() string {
	return o.normalizedKeyVal
}

func (o plannedMetadataObject) successAttempt(cfg config.Config) metadata.AttemptRecord {
	return metadata.AttemptRecord{
		ItemID:           o.itemID,
		ScriptName:       o.normalizedKeyVal,
		ScriptType:       o.scriptType,
		Checksum:         o.checksum,
		Action:           o.actionValue,
		ExecutionMS:      o.executionMS,
		Success:          true,
		TransactionMode:  o.transactionMode,
		TransactionScope: o.transactionScope,
		RollbackScope:    o.rollbackScope,
		NoTransaction:    o.noTransaction,
		GitCommit:        cfg.GitCommit,
		GitBranch:        cfg.GitBranch,
		PipelineRunID:    cfg.PipelineRunID,
		PipelineURL:      logger.Redact(cfg.PipelineURL),
		AppliedBy:        cfg.Actor,
	}
}

func (o plannedMetadataObject) failureAttempt(cfg config.Config, message string) metadata.AttemptRecord {
	attempt := o.successAttempt(cfg)
	attempt.Action = contracts.ActionFail
	attempt.Success = false
	attempt.ErrorMessage = message
	return attempt
}
