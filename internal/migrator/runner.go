package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/planner"
	"reporting-db-migrations/internal/reports"
)

type Runner struct {
	cfg config.Config
	log logger.Logger
}

func NewRunner(cfg config.Config, log logger.Logger) Runner {
	return Runner{cfg: cfg, log: log}
}

func (r Runner) Info(ctx context.Context) error {
	database, err := db.Open(ctx, r.cfg)
	if err != nil {
		return contracts.Wrap(contracts.ErrConnection, err)
	}
	defer database.Close()
	r.log.Info("connection_ok", r.cfg.MaskedTarget())
	return nil
}

func (r Runner) Plan(ctx context.Context) (contracts.MigrationPlan, error) {
	if _, _, err := planner.ResolvePlanningLayoutForRunner(r.cfg); err != nil {
		return contracts.MigrationPlan{}, err
	}
	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	defer func() {
		if err := closeFn(); err != nil {
			r.log.Warn("db_close_failed", err.Error())
		}
	}()
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return contracts.MigrationPlan{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	plan, err := planner.BuildWithConnection(ctx, r.cfg, successfulByKey, conn)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	if err := reports.WritePlan(r.cfg.ReportDir, plan); err != nil {
		return contracts.MigrationPlan{}, err
	}
	return plan, nil
}

func (r Runner) Migrate(ctx context.Context) error {
	if r.cfg.PlanFile == "" {
		return r.writeFailedMigration(r.newMigrationReport(), contracts.ErrInvalidInput, fmt.Errorf("--plan-file is required"))
	}
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return session.Fail(err, nil)
	}
	defer session.Close()
	conn := session.conn
	runID := ""
	recorder := session.Recorder(runID)
	failWithRun := func(base error, cause error) error {
		if runID != "" {
			session.RecordRunFailure(ctx, recorder, base, cause)
		}
		return session.Fail(base, cause)
	}
	successfulByKey, err := session.LoadSuccessfulChecksums(ctx)
	if err != nil {
		return failWithRun(contracts.ErrCriticalState, err)
	}
	planningLayout, hash, err := session.ResolvePlanningLayout()
	if err != nil {
		return failWithRun(contracts.ErrInvalidInput, err)
	}
	session.SetLayoutHash(hash)
	plan, err := session.BuildPlan(ctx, successfulByKey, planningLayout, hash)
	if err != nil {
		if errors.Is(err, contracts.ErrCriticalState) {
			return failWithRun(contracts.ErrCriticalState, err)
		}
		return failWithRun(contracts.ErrInvalidInput, err)
	}
	if plan.Blocked {
		return failWithRun(contracts.ErrChecksumMismatch, fmt.Errorf("%v", plan.BlockReasons))
	}
	if err := planner.VerifyApprovedPlan(r.cfg, plan); err != nil {
		return failWithRun(contracts.ErrInvalidInput, err)
	}
	if err := session.BootstrapMetadata(ctx); err != nil {
		return failWithRun(contracts.ErrCriticalState, err)
	}
	runID, recorder, err = session.StartRun(ctx, contracts.CommandMigrate, r.cfg.PlanFile, planArtifactHash(r.cfg.PlanFile), plan.Rollback)
	if err != nil {
		return failWithRun(contracts.ErrCriticalState, err)
	}
	itemIDs, err := recorder.persistMigrationScope(ctx, conn, plan)
	if err != nil {
		return failWithRun(contracts.ErrCriticalState, err)
	}
	r.log.Info("rollback_scope", fmt.Sprintf("Rollback scope: %s. Previous successful scripts remain committed. Use database backups or restore points for full recovery guarantees.", plan.Rollback))
	report := session.MigrationReport()
	if err := r.executePlanTracked(ctx, conn, planningLayout, plan, report, runID, itemIDs); err != nil {
		if writeErr := session.WriteMigrationReport(); writeErr != nil {
			return contracts.Wrap(contracts.ErrCriticalState, writeErr)
		}
		session.RecordRunFailure(ctx, recorder, err, nil)
		return err
	}
	if !r.cfg.SkipValidate {
		validationReport, validationErr := r.validateManagedScope(ctx, session, planningLayout, runID)
		report.ValidationScope = validationReport.Scope
		report.Validation = validationReport.Validation
		if writeErr := writeValidationReport(r.cfg.ReportDir, validationReport); writeErr != nil {
			return failWithRun(contracts.ErrCriticalState, writeErr)
		}
		if validationErr != nil {
			return failWithRun(contracts.ErrValidation, validationErr)
		}
	} else {
		report.ValidationSkipped = true
	}
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := session.FinishRun(ctx, recorder); err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	return session.WriteMigrationReport()
}

func (r Runner) openReservedConnection(ctx context.Context) (*sql.Conn, func() error, error) {
	database, err := db.Open(ctx, r.cfg)
	if err != nil {
		return nil, nil, contracts.Wrap(contracts.ErrConnection, err)
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		_ = database.Close()
		return nil, nil, contracts.Wrap(contracts.ErrConnection, err)
	}
	closeFn := func() error {
		connErr := conn.Close()
		dbErr := database.Close()
		if connErr != nil {
			return connErr
		}
		return dbErr
	}
	return conn, closeFn, nil
}

func (r Runner) newMigrationReport() contracts.MigrationReport {
	return contracts.MigrationReport{
		Tool:              "rmig",
		Version:           r.cfg.ToolVersion,
		ToolCommit:        r.cfg.ToolCommit,
		Environment:       r.cfg.Env,
		Database:          r.cfg.Database,
		GitCommit:         r.cfg.GitCommit,
		GitBranch:         r.cfg.GitBranch,
		SQLRoot:           r.cfg.SQLRoot,
		Base:              r.cfg.SQLBase,
		EffectiveBasePath: r.cfg.SelectedBasePath(),
		PipelineRunID:     r.cfg.PipelineRunID,
		PipelineURL:       logger.Redact(r.cfg.PipelineURL),
		Actor:             r.cfg.Actor,
		StartedAt:         time.Now().UTC(),
		Result:            "running",
		Applied:           []contracts.ScriptResult{},
		Skipped:           []contracts.ScriptResult{},
	}
}

func (r Runner) requireConfirmation() error {
	if !r.cfg.Confirm {
		return fmt.Errorf("%w: confirm flag required", contracts.ErrInvalidInput)
	}
	return nil
}

func (r Runner) writeFailedMigration(report contracts.MigrationReport, base error, cause error) error {
	report = finalizeMigrationFailureReport(r.cfg, report, base, cause)
	return writeFailureReport(func() error {
		return reports.WriteMigration(r.cfg.ReportDir, report)
	}, base, cause)
}
