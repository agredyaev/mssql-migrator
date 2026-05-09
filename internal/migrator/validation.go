package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/reports"
	"reporting-db-migrations/internal/validate"
)

func (r Runner) Validate(ctx context.Context) error {
	if _, err := validate.LoadLayout(r.cfg); err != nil {
		return r.writeValidationFailureReport(err, contracts.ErrInvalidInput)
	}
	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return r.writeValidationFailureReport(err, err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			r.log.Warn("db_close_failed", err.Error())
		}
	}()
	if err := r.acquireLock(ctx, conn); err != nil {
		return r.writeValidationFailureReport(err, err)
	}
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		return r.writeValidationFailureReport(err, contracts.ErrCriticalState)
	}
	vr, err := r.validateWithConnection(ctx, conn)
	if writeErr := reports.WriteValidation(r.cfg.ReportDir, vr); writeErr != nil {
		return fmt.Errorf("%w: %v", contracts.ErrCriticalState, writeErr)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrValidation, err)
	}
	return nil
}

func (r Runner) writeValidationFailureReport(cause error, base error) error {
	vr := contracts.ValidationReport{
		Tool:           "rmig",
		Version:        r.cfg.ToolVersion,
		ToolCommit:     r.cfg.ToolCommit,
		Environment:    r.cfg.Env,
		Database:       r.cfg.Database,
		GitCommit:      r.cfg.GitCommit,
		GitBranch:      r.cfg.GitBranch,
		Command:        "validate",
		SQLRoot:        r.cfg.SQLRoot,
		Base:           r.cfg.SQLBase,
		Scope:          validationScope(true),
		IncludesChecks: true,
		PipelineRunID:  r.cfg.PipelineRunID,
		PipelineURL:    logger.Redact(r.cfg.PipelineURL),
		Actor:          r.cfg.Actor,
		StartedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
		Result:         "failed",
	}
	failure := failures.BuildWithCause(r.cfg, "validation_failed", base, cause)
	vr.Failed = &failure
	if err := reports.WriteValidation(r.cfg.ReportDir, vr); err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	if base == nil {
		return cause
	}
	if cause == base {
		return base
	}
	return fmt.Errorf("%w: %v", base, cause)
}

func (r Runner) validateWithConnection(ctx context.Context, conn *sql.Conn) (contracts.ValidationReport, error) {
	layout, err := validate.LoadLayout(r.cfg)
	if err != nil {
		return validationFailureReport(r.cfg, err), err
	}
	return r.validateScope(ctx, conn, layout, true, "")
}

func (r Runner) validateManagedScope(ctx context.Context, conn *sql.Conn, layout parser.Layout, runID string) (contracts.ValidationReport, error) {
	return r.validateScope(ctx, conn, layout, false, runID)
}

func (r Runner) validateScope(ctx context.Context, conn *sql.Conn, layout parser.Layout, includeChecks bool, existingRunID string) (contracts.ValidationReport, error) {
	vr := contracts.ValidationReport{
		Tool:           "rmig",
		Version:        r.cfg.ToolVersion,
		ToolCommit:     r.cfg.ToolCommit,
		Environment:    r.cfg.Env,
		Database:       r.cfg.Database,
		GitCommit:      r.cfg.GitCommit,
		GitBranch:      r.cfg.GitBranch,
		Command:        validationCommand(includeChecks),
		LayoutHash:     parser.HashLayout(layout, includeChecks),
		SQLRoot:        r.cfg.SQLRoot,
		Base:           r.cfg.SQLBase,
		Scope:          validationScope(includeChecks),
		IncludesChecks: includeChecks,
		PipelineRunID:  r.cfg.PipelineRunID,
		PipelineURL:    logger.Redact(r.cfg.PipelineURL),
		Actor:          r.cfg.Actor,
		StartedAt:      time.Now().UTC(),
		Result:         "success",
	}
	catalog, err := validate.ReadCatalogState(ctx, conn)
	if err != nil {
		return finalizeValidationFailure(vr, err), err
	}
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return finalizeValidationFailure(vr, err), err
	}
	runID := existingRunID
	createdRun := false
	trackedObjectIDs := map[string]int64{}
	if runID == "" {
		runID, err = r.startRun(ctx, conn, "validate", "", "", contracts.RollbackScope(r.cfg.TransactionMode))
		if err != nil {
			return finalizeValidationFailure(vr, err), err
		}
		createdRun = true
		trackedObjectIDs, err = persistValidationScope(ctx, conn, runID, layout, catalog, successfulByKey)
		if err != nil {
			if createdRun {
				if finishErr := finishRun(ctx, conn, runID, false, contracts.ErrCriticalState, err); finishErr != nil {
					r.log.Warn("metadata_finish_run_failed", logger.Redact(finishErr.Error()))
				}
			}
			return finalizeValidationFailure(vr, err), err
		}
	} else {
		trackedObjectIDs, err = loadTrackedObjectIDs(ctx, conn, runID)
		if err != nil {
			return finalizeValidationFailure(vr, err), err
		}
	}
	modules, err := validate.RefreshManagedObjects(ctx, conn, layout, r.log)
	vr.Validation.ModulesRefreshed = modules.ModulesRefreshed
	if err != nil {
		recordValidationFailure(ctx, conn, runID, layout.Objects, trackedObjectIDs, err, includeChecks, r.cfg, r.log)
		if createdRun {
			if finishErr := finishRun(ctx, conn, runID, false, contracts.ErrValidation, err); finishErr != nil {
				r.log.Warn("metadata_finish_run_failed", logger.Redact(finishErr.Error()))
			}
		}
		return finalizeValidationFailure(vr, err), err
	}
	if err := recordValidationSuccesses(ctx, conn, runID, layout.Objects, trackedObjectIDs, includeChecks, r.cfg); err != nil {
		if createdRun {
			if finishErr := finishRun(ctx, conn, runID, false, contracts.ErrCriticalState, err); finishErr != nil {
				r.log.Warn("metadata_finish_run_failed", logger.Redact(finishErr.Error()))
			}
		}
		return finalizeValidationFailure(vr, err), err
	}
	if includeChecks {
		checks, err := validate.RunChecks(ctx, conn, layout)
		vr.Validation.ChecksPassed = checks.ChecksPassed
		vr.Validation.ChecksFailed = checks.ChecksFailed
		if err != nil {
			recordValidationFailure(ctx, conn, runID, nil, nil, err, includeChecks, r.cfg, r.log)
			if createdRun {
				if finishErr := finishRun(ctx, conn, runID, false, contracts.ErrValidation, err); finishErr != nil {
					r.log.Warn("metadata_finish_run_failed", logger.Redact(finishErr.Error()))
				}
			}
			return finalizeValidationFailure(vr, err), err
		}
	}
	vr.FinishedAt = time.Now().UTC()
	if createdRun {
		if err := finishRun(ctx, conn, runID, true, nil, nil); err != nil {
			return finalizeValidationFailure(vr, err), err
		}
	}
	return vr, nil
}

func validationFailureReport(cfg config.Config, err error) contracts.ValidationReport {
	vr := contracts.ValidationReport{
		Tool:           "rmig",
		Version:        cfg.ToolVersion,
		ToolCommit:     cfg.ToolCommit,
		Environment:    cfg.Env,
		Database:       cfg.Database,
		GitCommit:      cfg.GitCommit,
		GitBranch:      cfg.GitBranch,
		Command:        "validate",
		SQLRoot:        cfg.SQLRoot,
		Base:           cfg.SQLBase,
		Scope:          "full_validation",
		IncludesChecks: true,
		PipelineRunID:  cfg.PipelineRunID,
		PipelineURL:    logger.Redact(cfg.PipelineURL),
		Actor:          cfg.Actor,
		StartedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
		Result:         "failed",
	}
	failure := failures.BuildWithCause(cfg, "validation_failed", contracts.ErrValidation, err)
	vr.Failed = &failure
	return vr
}

func finalizeValidationFailure(vr contracts.ValidationReport, err error) contracts.ValidationReport {
	vr.Result = "failed"
	failure := failures.BuildWithCause(config.Config{SQLRoot: vr.SQLRoot, SQLBase: vr.Base}, "validation_failed", contracts.ErrValidation, err)
	vr.Failed = &failure
	vr.FinishedAt = time.Now().UTC()
	return vr
}

func validationScope(includeChecks bool) string {
	if includeChecks {
		return "full_validation"
	}
	return "managed_scope_only"
}

func validationCommand(includeChecks bool) string {
	if includeChecks {
		return "validate"
	}
	return "migrate"
}

func recordValidationSuccesses(ctx context.Context, execer metadata.Execer, runID string, objects []parser.Object, trackedObjectIDs map[string]int64, includeChecks bool, cfg config.Config) error {
	for _, object := range objects {
		trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, object.NormalizedKey)
		action := contracts.ActionValidateChecked
		if !parser.IsModuleKind(object.Kind) {
			action = contracts.ActionValidateSkipped
		}
		metaCtx, cancel := metadataContext(ctx)
		err := metadata.InsertAttempt(metaCtx, execer, metadata.AttemptRecord{
			RunID:            runID,
			TrackedObjectID:  trackedObjectID,
			ScriptName:       object.NormalizedKey,
			ScriptType:       "validate",
			Checksum:         object.Checksum,
			Action:           action,
			ExecutionMS:      0,
			Success:          true,
			TransactionMode:  config.TransactionModeNone,
			TransactionScope: config.TransactionModeNone,
			RollbackScope:    contracts.RollbackScopeNone,
			NoTransaction:    true,
			GitCommit:        cfg.GitCommit,
			GitBranch:        cfg.GitBranch,
			PipelineRunID:    cfg.PipelineRunID,
			PipelineURL:      logger.Redact(cfg.PipelineURL),
			AppliedBy:        cfg.Actor,
		})
		cancel()
		if err != nil {
			return err
		}
		metaCtx, cancel = metadataContext(ctx)
		err = metadata.UpdateTrackedObjectResult(metaCtx, execer, runID, object.NormalizedKey, true, "")
		cancel()
		if err != nil {
			return err
		}
	}
	if !includeChecks {
		return nil
	}
	return nil
}

func recordValidationFailure(ctx context.Context, execer metadata.Execer, runID string, objects []parser.Object, trackedObjectIDs map[string]int64, validationErr error, includeChecks bool, cfg config.Config, log logger.Logger) {
	errorMessage := logger.Redact(validationErr.Error())
	for _, object := range objects {
		metaCtx, cancel := metadataContext(ctx)
		err := metadata.UpdateTrackedObjectResult(metaCtx, execer, runID, object.NormalizedKey, false, errorMessage)
		cancel()
		if err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
			continue
		}
		trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, object.NormalizedKey)
		metaCtx, cancel = metadataContext(ctx)
		err = metadata.InsertAttempt(metaCtx, execer, metadata.AttemptRecord{
			RunID:            runID,
			TrackedObjectID:  trackedObjectID,
			ScriptName:       object.NormalizedKey,
			ScriptType:       "validate",
			Checksum:         object.Checksum,
			Action:           contracts.ActionFail,
			ExecutionMS:      0,
			Success:          false,
			ErrorMessage:     errorMessage,
			TransactionMode:  config.TransactionModeNone,
			TransactionScope: config.TransactionModeNone,
			RollbackScope:    contracts.RollbackScopeNone,
			NoTransaction:    true,
			GitCommit:        cfg.GitCommit,
			GitBranch:        cfg.GitBranch,
			PipelineRunID:    cfg.PipelineRunID,
			PipelineURL:      logger.Redact(cfg.PipelineURL),
			AppliedBy:        cfg.Actor,
		})
		cancel()
		if err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
	if includeChecks {
		metaCtx, cancel := metadataContext(ctx)
		err := metadata.InsertAttempt(metaCtx, execer, metadata.AttemptRecord{
			RunID:            runID,
			ScriptName:       "validation/checks",
			ScriptType:       "validate",
			Checksum:         "-",
			Action:           contracts.ActionFail,
			ExecutionMS:      0,
			Success:          false,
			ErrorMessage:     errorMessage,
			TransactionMode:  config.TransactionModeNone,
			TransactionScope: config.TransactionModeNone,
			RollbackScope:    contracts.RollbackScopeNone,
			NoTransaction:    true,
			GitCommit:        cfg.GitCommit,
			GitBranch:        cfg.GitBranch,
			PipelineRunID:    cfg.PipelineRunID,
			PipelineURL:      logger.Redact(cfg.PipelineURL),
			AppliedBy:        cfg.Actor,
		})
		cancel()
		if err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
}
