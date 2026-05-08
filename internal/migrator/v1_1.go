package migrator

import (
	"context"
	"fmt"
	"time"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/reports"
	"reporting-db-migrations/internal/state"
)

func (r Runner) Baseline(ctx context.Context) error {
	if err := r.requireConfirmation(); err != nil {
		return err
	}
	if r.cfg.BaselineUpTo == "" {
		return fmt.Errorf("%w: --up-to is required", contracts.ErrInvalidInput)
	}
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return err
	}
	defer closeFn()
	versioned, _, _, err := parser.Discover(r.cfg.SQLDir)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	for _, script := range versioned {
		if script.Name > r.cfg.BaselineUpTo {
			continue
		}
		attempt := state.Attempt{
			ScriptName:    script.Name,
			ScriptType:    string(script.Type),
			Checksum:      script.Checksum,
			Success:       true,
			GitCommit:     r.cfg.GitCommit,
			GitBranch:     r.cfg.GitBranch,
			PipelineRunID: r.cfg.PipelineRunID,
			PipelineURL:   r.cfg.PipelineURL,
			AppliedBy:     r.cfg.Actor,
		}
		if err := metadata.RecordAttempt(ctx, conn, attempt); err != nil {
			return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
		}
		report.Applied = append(report.Applied, contracts.ScriptResult{Script: script.Name, Type: string(script.Type), Checksum: script.Checksum})
	}
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	return reports.WriteMigration(r.cfg.ReportDir, report)
}

func (r Runner) RepairChecksum(ctx context.Context) error {
	if err := r.requireConfirmation(); err != nil {
		return err
	}
	if r.cfg.RepairScript == "" {
		return fmt.Errorf("%w: --script is required", contracts.ErrInvalidInput)
	}
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return err
	}
	defer closeFn()
	versioned, repeatable, checks, err := parser.Discover(r.cfg.SQLDir)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	all := append(append(versioned, repeatable...), checks...)
	for _, script := range all {
		if script.Name != r.cfg.RepairScript {
			continue
		}
		_, err := conn.ExecContext(ctx, `UPDATE __migrator.schema_migrations SET checksum=@p1 WHERE script_name=@p2 AND success=1`, script.Checksum, script.Name)
		if err != nil {
			return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
		}
		report.Result = "success"
		report.FinishedAt = time.Now().UTC()
		report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
		return reports.WriteMigration(r.cfg.ReportDir, report)
	}
	return r.writeFailedMigration(report, contracts.ErrInvalidInput, fmt.Errorf("script not found: %s", r.cfg.RepairScript))
}
