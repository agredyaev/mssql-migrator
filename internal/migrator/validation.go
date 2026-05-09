package migrator

import (
	"context"
	"database/sql"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/validate"
)

func (r Runner) Validate(ctx context.Context) error {
	if _, err := validate.LoadLayout(r.cfg); err != nil {
		return r.writeValidationFailureReport(err, contracts.ErrInvalidInput)
	}
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return r.writeValidationFailureReport(err, err)
	}
	defer session.Close()
	conn := session.conn
	if err := session.BootstrapMetadata(ctx); err != nil {
		return r.writeValidationFailureReport(err, contracts.ErrCriticalState)
	}
	vr, err := r.validateWithConnection(ctx, conn)
	if writeErr := writeValidationReport(r.cfg.ReportDir, vr); writeErr != nil {
		return contracts.Wrap(contracts.ErrCriticalState, writeErr)
	}
	if err != nil {
		return contracts.Wrap(contracts.ErrValidation, err)
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
		Command:        contracts.CommandValidate,
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
	if err := writeValidationReport(r.cfg.ReportDir, vr); err != nil {
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	if base == nil {
		return cause
	}
	if cause == base {
		return base
	}
	return contracts.Wrap(base, cause)
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
	recorder := newMetadataRecorder(r.cfg, conn, runID)
	if runID == "" {
		runID, err = r.startRun(ctx, conn, contracts.CommandValidate, "", "", contracts.RollbackScope(r.cfg.TransactionMode))
		if err != nil {
			return finalizeValidationFailure(vr, err), err
		}
		createdRun = true
		recorder = newMetadataRecorder(r.cfg, conn, runID)
		trackedObjectIDs, err = persistValidationScope(ctx, conn, runID, layout, catalog, successfulByKey)
		if err != nil {
			if createdRun {
				if finishErr := recorder.recordRunResult(ctx, false, contracts.ErrCriticalState, err); finishErr != nil {
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
		recorder.recordValidationFailure(ctx, layout.Objects, trackedObjectIDs, err, includeChecks, r.log)
		if createdRun {
			if finishErr := recorder.recordRunResult(ctx, false, contracts.ErrValidation, err); finishErr != nil {
				r.log.Warn("metadata_finish_run_failed", logger.Redact(finishErr.Error()))
			}
		}
		return finalizeValidationFailure(vr, err), err
	}
	if err := recordValidationSuccesses(ctx, recorder, layout.Objects, trackedObjectIDs); err != nil {
		if createdRun {
			if finishErr := recorder.recordRunResult(ctx, false, contracts.ErrCriticalState, err); finishErr != nil {
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
			recorder.recordValidationFailure(ctx, nil, nil, err, includeChecks, r.log)
			if createdRun {
				if finishErr := recorder.recordRunResult(ctx, false, contracts.ErrValidation, err); finishErr != nil {
					r.log.Warn("metadata_finish_run_failed", logger.Redact(finishErr.Error()))
				}
			}
			return finalizeValidationFailure(vr, err), err
		}
	}
	vr.FinishedAt = time.Now().UTC()
	if createdRun {
		if err := recorder.recordRunResult(ctx, true, nil, nil); err != nil {
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
		Command:        contracts.CommandValidate,
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

func recordValidationSuccesses(ctx context.Context, recorder metadataRecorder, objects []parser.Object, trackedObjectIDs map[string]int64) error {
	for _, object := range objects {
		if err := recorder.updateTrackedObjectResult(ctx, object.NormalizedKey, true, ""); err != nil {
			return err
		}
	}
	return nil
}
