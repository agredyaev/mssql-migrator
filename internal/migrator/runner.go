package migrator

import (
	"context"
	"database/sql"
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
		return fmt.Errorf("%w: %v", contracts.ErrConnection, err)
	}
	defer database.Close()
	r.log.Info("connection_ok", r.cfg.MaskedTarget())
	return nil
}

func (r Runner) Plan(ctx context.Context) (contracts.MigrationPlan, error) {
	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	defer closeFn()
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		return contracts.MigrationPlan{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	migrationState, err := metadata.LoadState(ctx, conn)
	if err != nil {
		return contracts.MigrationPlan{}, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	plan, err := planner.Build(r.cfg, migrationState)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	if err := reports.WritePlan(r.cfg.ReportDir, plan); err != nil {
		return contracts.MigrationPlan{}, err
	}
	return plan, nil
}

func (r Runner) Migrate(ctx context.Context) error {
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return err
	}
	defer closeFn()
	if r.cfg.PlanFile != "" {
		if err := planner.VerifyApprovedPlan(r.cfg, report.SQLDirHash); err != nil {
			return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
		}
	}
	migrationState, err := metadata.LoadState(ctx, conn)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	plan, err := planner.Build(r.cfg, migrationState)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	if plan.Blocked {
		return r.writeFailedMigration(report, contracts.ErrChecksumMismatch, fmt.Errorf("%v", plan.BlockReasons))
	}
	if err := r.executePlan(ctx, conn, migrationState, &report); err != nil {
		_ = reports.WriteMigration(r.cfg.ReportDir, report)
		return err
	}
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	return reports.WriteMigration(r.cfg.ReportDir, report)
}

func (r Runner) openReservedConnection(ctx context.Context) (*sql.Conn, func(), error) {
	database, err := db.Open(ctx, r.cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", contracts.ErrConnection, err)
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("%w: %v", contracts.ErrConnection, err)
	}
	closeFn := func() {
		conn.Close()
		database.Close()
	}
	return conn, closeFn, nil
}

func (r Runner) newMigrationReport() contracts.MigrationReport {
	return contracts.MigrationReport{
		Tool:          "rmig",
		Version:       r.cfg.ToolVersion,
		Environment:   r.cfg.Env,
		Database:      r.cfg.Database,
		GitCommit:     r.cfg.GitCommit,
		GitBranch:     r.cfg.GitBranch,
		PipelineRunID: r.cfg.PipelineRunID,
		PipelineURL:   r.cfg.PipelineURL,
		Actor:         r.cfg.Actor,
		StartedAt:     time.Now().UTC(),
		Result:        "running",
		Applied:       []contracts.ScriptResult{},
		Skipped:       []contracts.ScriptResult{},
	}
}

func (r Runner) requireConfirmation() error {
	if !r.cfg.Confirm {
		return fmt.Errorf("%w: confirm flag required", contracts.ErrInvalidInput)
	}
	return nil
}

func (r Runner) writeFailedMigration(report contracts.MigrationReport, base error, cause error) error {
	report.Result = "failed"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	message := base.Error()
	if cause != nil {
		message += ": " + cause.Error()
	}
	report.Failed = &contracts.Failure{Error: message}
	_ = reports.WriteMigration(r.cfg.ReportDir, report)
	if cause == nil {
		return base
	}
	return fmt.Errorf("%w: %v", base, cause)
}
