package migrator

import (
	"context"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
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

func (r metadataRecorder) updateTrackedSchemaResult(ctx context.Context, normalizedSchemaName string, success bool, errorMessage string) error {
	if r.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateTrackedSchemaResult(metaCtx, r.execer, r.runID, normalizedSchemaName, success, errorMessage)
}

func (r metadataRecorder) updateTrackedObjectResult(ctx context.Context, normalizedKey string, success bool, errorMessage string) error {
	if r.runID == "" {
		return nil
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.UpdateTrackedObjectResult(metaCtx, r.execer, r.runID, normalizedKey, success, errorMessage)
}

func (r metadataRecorder) insertAttempt(ctx context.Context, attempt metadata.AttemptRecord) error {
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	attempt.RunID = r.runID
	return metadata.InsertAttempt(metaCtx, r.execer, attempt)
}

func (r metadataRecorder) recordSchemaSuccess(ctx context.Context, schemaName string, action string, writeAttempt bool) error {
	normalizedSchemaName := strings.ToLower(strings.TrimSpace(schemaName))
	if err := r.updateTrackedSchemaResult(ctx, normalizedSchemaName, true, ""); err != nil {
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
	if err := r.updateTrackedSchemaResult(ctx, normalizedSchemaName, false, message); err != nil {
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
	return r.updateTrackedObjectResult(ctx, object.normalizedKey(), true, "")
}

func (r metadataRecorder) recordObjectFailure(ctx context.Context, object plannedMetadataObject, failure error, writeAttempt bool) error {
	message := logger.Redact(failure.Error())
	if writeAttempt {
		attempt := object.failureAttempt(r.cfg, message)
		if err := r.insertAttempt(ctx, attempt); err != nil {
			return err
		}
	}
	return r.updateTrackedObjectResult(ctx, object.normalizedKey(), false, message)
}

func (r metadataRecorder) recordValidationFailure(ctx context.Context, objects []parser.Object, trackedObjectIDs map[string]int64, validationErr error, includeChecks bool, log logger.Logger) {
	errorMessage := logger.Redact(validationErr.Error())
	for _, object := range objects {
		planned := validationMetadataObject(object, lookupTrackedObjectID(trackedObjectIDs, object.NormalizedKey))
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

type plannedMetadataObject struct {
	objectPath       string
	kind             string
	normalizedKeyVal string
	checksum         string
	scriptType       string
	actionValue      string
	trackedObjectID  *int64
	executionMS      int
	transactionMode  string
	transactionScope string
	rollbackScope    string
	noTransaction    bool
}

func passiveMetadataObject(planned contracts.PlannedObject, trackedObjectID *int64) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       planned.ObjectPath,
		kind:             planned.Kind,
		normalizedKeyVal: planned.NormalizedKey,
		checksum:         planned.Checksum,
		scriptType:       contracts.ScriptTypeObject,
		actionValue:      planned.PlannedAction,
		trackedObjectID:  trackedObjectID,
		transactionMode:  planned.TransactionMode,
		transactionScope: planned.TransactionMode,
		rollbackScope:    planned.RollbackScope,
		noTransaction:    planned.NoTransaction,
	}
}

func executedMetadataObject(object parser.Object, planned contracts.PlannedObject, trackedObjectID *int64, executionMS int) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       object.Path,
		kind:             object.Kind,
		normalizedKeyVal: object.NormalizedKey,
		checksum:         object.Checksum,
		scriptType:       contracts.ScriptTypeObject,
		actionValue:      planned.PlannedAction,
		trackedObjectID:  trackedObjectID,
		executionMS:      executionMS,
		transactionMode:  planned.TransactionMode,
		transactionScope: planned.TransactionMode,
		rollbackScope:    planned.RollbackScope,
		noTransaction:    planned.NoTransaction,
	}
}

func validationMetadataObject(object parser.Object, trackedObjectID *int64) plannedMetadataObject {
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
		trackedObjectID:  trackedObjectID,
		executionMS:      0,
		transactionMode:  config.TransactionModeNone,
		transactionScope: config.TransactionModeNone,
		rollbackScope:    contracts.RollbackScopeNone,
		noTransaction:    true,
	}
}


func baselineFailureMetadataObject(planned contracts.PlannedObject, trackedObjectID *int64, defaultMode string) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       planned.ObjectPath,
		kind:             planned.Kind,
		normalizedKeyVal: planned.NormalizedKey,
		checksum:         planned.Checksum,
		scriptType:       contracts.ScriptTypeObject,
		actionValue:      planned.PlannedAction,
		trackedObjectID:  trackedObjectID,
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
		TrackedObjectID:  o.trackedObjectID,
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
