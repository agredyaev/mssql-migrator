package migrator

import (
	"context"
	"errors"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/runreport"
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
	return runreport.WriteValidationOutcome(r.cfg.ReportDir, vr, err)
}

func (r Runner) writeValidationFailureReport(cause error, base error) error {
	vr := newValidationFailureReport(r.cfg, true)
	return runreport.WriteValidationFailure(r.cfg, vr, runreport.ValidationFailurePhase, base, cause)
}

func (r Runner) validateManagedScope(ctx context.Context, session *runSession, layout parser.Layout, runID string) (contracts.ValidationReport, error) {
	return r.validateScope(ctx, session, layout, false, runID)
}

func (r Runner) validateScope(ctx context.Context, session *runSession, layout parser.Layout, includeChecks bool, existingRunID string) (contracts.ValidationReport, error) {
	vr := newValidationReport(r.cfg, layout, includeChecks)
	catalog, successfulByKey, err := r.loadValidationInputs(ctx, session, &vr)
	if err != nil {
		return vr, err
	}
	runState, err := r.ensureValidationRunState(ctx, session, layout, catalog, successfulByKey, existingRunID, &vr)
	if err != nil {
		return vr, err
	}
	return r.executeValidationScope(ctx, session, layout, includeChecks, vr, runState)
}

type validationRunState struct {
	runID      string
	createdRun bool
	recorder   metadataRecorder
	itemIDs    map[string]int64
}

func newValidationReport(cfg config.Config, layout parser.Layout, includeChecks bool) contracts.ValidationReport {
	report := newValidationReportBase(cfg, includeChecks)
	report.LayoutHash = parser.HashLayout(layout, includeChecks)
	report.StartedAt = time.Now().UTC()
	report.Result = reportResultSuccess
	return report
}

func newValidationFailureReport(cfg config.Config, includeChecks bool) contracts.ValidationReport {
	report := newValidationReportBase(cfg, includeChecks)
	report.StartedAt = time.Now().UTC()
	return report
}

func newValidationReportBase(cfg config.Config, includeChecks bool) contracts.ValidationReport {
	return contracts.ValidationReport{
		Tool:           toolName,
		Version:        cfg.ToolVersion,
		ToolCommit:     cfg.ToolCommit,
		Environment:    cfg.Env,
		Database:       cfg.Database,
		GitCommit:      cfg.GitCommit,
		GitBranch:      cfg.GitBranch,
		Command:        validationCommand(includeChecks),
		SQLRoot:        cfg.SQLRoot,
		Base:           cfg.SQLBase,
		Scope:          validationScope(includeChecks),
		IncludesChecks: includeChecks,
		PipelineRunID:  cfg.PipelineRunID,
		PipelineURL:    logger.Redact(cfg.PipelineURL),
		Actor:          cfg.Actor,
	}
}

func (r Runner) loadValidationInputs(ctx context.Context, session *runSession, vr *contracts.ValidationReport) (validate.CatalogState, map[string]string, error) {
	catalog, err := validate.ReadCatalogState(ctx, session.conn)
	if err != nil {
		finalizeValidationFailure(vr, contracts.ErrCriticalState, err)
		return validate.CatalogState{}, nil, validationFailureError(contracts.ErrCriticalState, err)
	}
	successfulByKey, err := session.LoadSuccessfulChecksums(ctx)
	if err != nil {
		finalizeValidationFailure(vr, contracts.ErrCriticalState, err)
		return validate.CatalogState{}, nil, validationFailureError(contracts.ErrCriticalState, err)
	}
	return catalog, successfulByKey, nil
}

func (r Runner) ensureValidationRunState(ctx context.Context, session *runSession, layout parser.Layout, catalog validate.CatalogState, successfulByKey map[string]string, existingRunID string, vr *contracts.ValidationReport) (validationRunState, error) {
	runState := validationRunState{runID: existingRunID, recorder: session.Recorder(existingRunID), itemIDs: map[string]int64{}}
	if existingRunID == "" {
		return r.createValidationRunState(ctx, session, layout, catalog, successfulByKey, vr)
	}
	itemIDs, err := runState.recorder.loadObjectItemIDs(ctx)
	if err != nil {
		finalizeValidationFailure(vr, contracts.ErrCriticalState, err)
		return validationRunState{}, validationFailureError(contracts.ErrCriticalState, err)
	}
	runState.itemIDs = itemIDs
	return runState, nil
}

func (r Runner) createValidationRunState(ctx context.Context, session *runSession, layout parser.Layout, catalog validate.CatalogState, successfulByKey map[string]string, vr *contracts.ValidationReport) (validationRunState, error) {
	runID, recorder, err := session.StartRun(ctx, contracts.CommandValidate, "", "", rollbackScope(r.cfg.TransactionMode))
	if err != nil {
		finalizeValidationFailure(vr, contracts.ErrCriticalState, err)
		return validationRunState{}, validationFailureError(contracts.ErrCriticalState, err)
	}
	itemIDs, err := recorder.scope.Validation(ctx, layout, catalog, successfulByKey)
	if err != nil {
		return validationRunState{}, validationRunState{runID: runID, createdRun: true, recorder: recorder}.fail(ctx, session, vr, contracts.ErrCriticalState, err)
	}
	return validationRunState{runID: runID, createdRun: true, recorder: recorder, itemIDs: itemIDs}, nil
}

func (r Runner) executeValidationScope(ctx context.Context, session *runSession, layout parser.Layout, includeChecks bool, vr contracts.ValidationReport, runState validationRunState) (contracts.ValidationReport, error) {
	modules, err := validate.RefreshManagedObjects(ctx, session.conn, layout, r.log)
	vr.Validation.ModulesRefreshed = modules.ModulesRefreshed
	if err != nil {
		runState.recorder.validation.recordFailure(ctx, layout.Objects, runState.itemIDs, err, includeChecks, r.log)
		return vr, runState.fail(ctx, session, &vr, validationFailureBase(err), err)
	}
	if err := runState.recorder.validation.markSuccesses(ctx, layout.Objects); err != nil {
		return vr, runState.fail(ctx, session, &vr, contracts.ErrCriticalState, err)
	}
	if err := r.runValidationChecks(ctx, session, layout, includeChecks, &vr, runState); err != nil {
		return vr, err
	}
	if err := runState.finish(ctx, session, &vr); err != nil {
		return vr, err
	}
	return vr, nil
}

func (r Runner) runValidationChecks(ctx context.Context, session *runSession, layout parser.Layout, includeChecks bool, vr *contracts.ValidationReport, runState validationRunState) error {
	if !includeChecks {
		return nil
	}
	checks, err := validate.RunChecks(ctx, session.conn, layout)
	vr.Validation.ChecksPassed = checks.ChecksPassed
	vr.Validation.ChecksFailed = checks.ChecksFailed
	if err != nil {
		runState.recorder.validation.recordFailure(ctx, nil, nil, err, includeChecks, r.log)
		return runState.fail(ctx, session, vr, validationFailureBase(err), err)
	}
	return nil
}

func (s validationRunState) finish(ctx context.Context, session *runSession, vr *contracts.ValidationReport) error {
	vr.FinishedAt = time.Now().UTC()
	if !s.createdRun {
		return nil
	}
	if err := session.FinishRun(ctx, s.recorder); err != nil {
		finalizeValidationFailure(vr, contracts.ErrCriticalState, err)
		return validationFailureError(contracts.ErrCriticalState, err)
	}
	return nil
}

func (s validationRunState) recordRunFailure(ctx context.Context, session *runSession, base error, cause error) {
	if !s.createdRun {
		return
	}
	session.RecordRunFailure(ctx, s.recorder, base, cause)
}

func (s validationRunState) fail(ctx context.Context, session *runSession, vr *contracts.ValidationReport, base error, cause error) error {
	s.recordRunFailure(ctx, session, base, cause)
	finalizeValidationFailure(vr, base, cause)
	return validationFailureError(base, cause)
}

func validationFailureError(base error, cause error) error {
	if cause == nil || base == nil || errors.Is(cause, base) {
		return cause
	}
	return contracts.Wrap(base, cause)
}

func validationFailureBase(err error) error {
	if errors.Is(err, contracts.ErrCriticalState) {
		return contracts.ErrCriticalState
	}
	return contracts.ErrValidation
}

func validationFailureReport(vr contracts.ValidationReport, base error, cause error) contracts.ValidationReport {
	return runreport.FinalizeValidationFailureFromReport(vr, runreport.ValidationFailurePhase, base, cause)
}

func finalizeValidationFailure(vr *contracts.ValidationReport, base error, cause error) {
	if vr == nil {
		return
	}
	*vr = validationFailureReport(*vr, base, cause)
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
