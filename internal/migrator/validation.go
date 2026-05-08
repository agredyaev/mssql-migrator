package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/reports"
	"reporting-db-migrations/internal/validate"
)

func (r Runner) Validate(ctx context.Context) error {
	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
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

func (r Runner) validateWithConnection(ctx context.Context, conn *sql.Conn) (contracts.ValidationReport, error) {
	vr := contracts.ValidationReport{
		Tool:          "rmig",
		Version:       r.cfg.ToolVersion,
		ToolCommit:    r.cfg.ToolCommit,
		Environment:   r.cfg.Env,
		Database:      r.cfg.Database,
		GitCommit:     r.cfg.GitCommit,
		GitBranch:     r.cfg.GitBranch,
		PipelineRunID: r.cfg.PipelineRunID,
		PipelineURL:   logger.Redact(r.cfg.PipelineURL),
		Actor:         r.cfg.Actor,
		StartedAt:     time.Now().UTC(),
		Result:        "success",
	}
	modules, err := validate.RefreshModules(ctx, conn, r.cfg, r.log)
	vr.Validation.ModulesRefreshed = modules.ModulesRefreshed
	if err != nil {
		vr.Result = "failed"
		vr.Failed = &contracts.Failure{Error: logger.Redact(err.Error())}
		vr.FinishedAt = time.Now().UTC()
		return vr, err
	}
	checks, err := validate.RunChecks(ctx, conn, r.cfg.SQLDir)
	vr.Validation.ChecksPassed = checks.ChecksPassed
	vr.Validation.ChecksFailed = checks.ChecksFailed
	if err != nil {
		vr.Result = "failed"
		vr.Failed = &contracts.Failure{Error: logger.Redact(err.Error())}
		vr.FinishedAt = time.Now().UTC()
		return vr, err
	}
	vr.FinishedAt = time.Now().UTC()
	return vr, nil
}
