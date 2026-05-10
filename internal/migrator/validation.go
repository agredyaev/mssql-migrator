package migrator

import (
	"context"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/validate"
)

func (r Runner) Validate(ctx context.Context) error {
	layout, err := validate.LoadLayout(r.cfg)
	if err != nil {
		return r.writeValidationFailureReport(err, contracts.ErrInvalidInput)
	}
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return r.writeValidationFailureReport(err, err)
	}
	defer session.Close()
	if err := session.BootstrapMetadata(ctx); err != nil {
		return r.writeValidationFailureReport(err, contracts.ErrCriticalState)
	}
	vr, err := r.validateScope(ctx, session, layout, true, "")
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
	}
	vr = finalizeValidationFailureReport(r.cfg, vr, "validation_failed", base, cause)
	return writeFailureReport(func() error {
		return writeValidationReport(r.cfg.ReportDir, vr)
	}, base, cause)
}

func (r Runner) validateManagedScope(ctx context.Context, session *runSession, layout parser.Layout, runID string) (contracts.ValidationReport, error) {
	return r.validateScope(ctx, session, layout, false, runID)
}

func (r Runner) validateScope(ctx context.Context, session *runSession, layout parser.Layout, includeChecks bool, existingRunID string) (contracts.ValidationReport, error) {
	conn := session.conn
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
	successfulByKey, err := session.LoadSuccessfulChecksums(ctx)
	if err != nil {
		return finalizeValidationFailure(vr, err), err
	}
	runID := existingRunID
	createdRun := false
	itemIDs := map[string]int64{}
	recorder := session.Recorder(runID)
	if runID == "" {
		runID, recorder, err = session.StartRun(ctx, contracts.CommandValidate, "", "", contracts.RollbackScope(r.cfg.TransactionMode))
		if err != nil {
			return finalizeValidationFailure(vr, err), err
		}
		createdRun = true
		itemIDs, err = recorder.persistValidationScope(ctx, conn, layout, catalog, successfulByKey)
		if err != nil {
			if createdRun {
				session.RecordRunFailure(ctx, recorder, contracts.ErrCriticalState, err)
			}
			return finalizeValidationFailure(vr, err), err
		}
	} else {
		itemIDs, err = recorder.loadObjectItemIDs(ctx, conn)
		if err != nil {
			return finalizeValidationFailure(vr, err), err
		}
	}
	modules, err := validate.RefreshManagedObjects(ctx, conn, layout, r.log)
	vr.Validation.ModulesRefreshed = modules.ModulesRefreshed
	if err != nil {
		recorder.recordValidationFailure(ctx, layout.Objects, itemIDs, err, includeChecks, r.log)
		if createdRun {
			session.RecordRunFailure(ctx, recorder, contracts.ErrValidation, err)
		}
		return finalizeValidationFailure(vr, err), err
	}
	if err := recorder.recordValidationSuccesses(ctx, layout.Objects); err != nil {
		if createdRun {
			session.RecordRunFailure(ctx, recorder, contracts.ErrCriticalState, err)
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
				session.RecordRunFailure(ctx, recorder, contracts.ErrValidation, err)
			}
			return finalizeValidationFailure(vr, err), err
		}
	}
	vr.FinishedAt = time.Now().UTC()
	if createdRun {
		if err := session.FinishRun(ctx, recorder); err != nil {
			return finalizeValidationFailure(vr, err), err
		}
	}
	return vr, nil
}

func finalizeValidationFailure(vr contracts.ValidationReport, err error) contracts.ValidationReport {
	return finalizeValidationFailureReport(config.Config{SQLRoot: vr.SQLRoot, SQLBase: vr.Base}, vr, "validation_failed", contracts.ErrValidation, err)
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
