package migrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
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
	upToVersion, err := parser.ParseScriptVersion(r.cfg.BaselineUpTo)
	if err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrInvalidInput, err)
	}
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return r.writeFailedMigration(report, err, nil)
	}
	defer closeFn()
	versioned, _, _, err := parser.Discover(r.cfg.SQLDir)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	migrationState, err := metadata.LoadState(ctx, conn)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	toApply, skipped, err := selectBaselineScripts(versioned, migrationState, upToVersion)
	if err != nil {
		return r.writeFailedMigration(report, baselineErrorKind(err), err)
	}
	report.Skipped = append(report.Skipped, skipped...)
	for _, script := range toApply {
		attempt := state.Attempt{
			ScriptName:    script.Name,
			ScriptType:    string(script.Type),
			Checksum:      script.Checksum,
			Success:       true,
			GitCommit:     r.cfg.GitCommit,
			GitBranch:     r.cfg.GitBranch,
			PipelineRunID: r.cfg.PipelineRunID,
			PipelineURL:   logger.Redact(r.cfg.PipelineURL),
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
		return r.writeFailedMigration(report, err, nil)
	}
	defer closeFn()
	versioned, repeatable, _, err := parser.Discover(r.cfg.SQLDir)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	all := append(versioned, repeatable...)
	for _, script := range all {
		if script.Name != r.cfg.RepairScript {
			continue
		}
		rowsAffected, err := repairSuccessfulChecksum(ctx, conn, script.Name, script.Checksum)
		if err != nil {
			return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
		}
		if rowsAffected == 0 {
			return r.writeFailedMigration(report, contracts.ErrInvalidInput, fmt.Errorf("script has no successful applied metadata row: %s", r.cfg.RepairScript))
		}
		report.Result = "success"
		report.FinishedAt = time.Now().UTC()
		report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
		return reports.WriteMigration(r.cfg.ReportDir, report)
	}
	return r.writeFailedMigration(report, contracts.ErrInvalidInput, fmt.Errorf("script not found: %s", r.cfg.RepairScript))
}

func selectBaselineScripts(versioned []parser.Script, migrationState state.State, upToVersion string) ([]parser.Script, []contracts.ScriptResult, error) {
	toApply := make([]parser.Script, 0, len(versioned))
	skipped := make([]contracts.ScriptResult, 0)
	foundTarget := false
	for _, script := range versioned {
		if script.Version == upToVersion {
			foundTarget = true
		}
		if parser.CompareScriptVersions(script.Version, upToVersion) > 0 {
			continue
		}
		latest, exists := migrationState.SuccessByScript[script.Name]
		if !exists {
			toApply = append(toApply, script)
			continue
		}
		if latest.Checksum != script.Checksum {
			return nil, nil, fmt.Errorf("existing successful metadata checksum mismatch: %s", script.Name)
		}
		skipped = append(skipped, contracts.ScriptResult{Script: script.Name, Type: string(script.Type), Checksum: script.Checksum, Reason: "already_applied"})
	}
	if !foundTarget {
		return nil, nil, fmt.Errorf("baseline target version not found: V%s", upToVersion)
	}
	return toApply, skipped, nil
}

func baselineErrorKind(err error) error {
	if strings.Contains(err.Error(), "checksum mismatch") {
		return contracts.ErrChecksumMismatch
	}
	return contracts.ErrInvalidInput
}

func repairSuccessfulChecksum(ctx context.Context, execer metadata.Execer, scriptName string, checksum string) (int64, error) {
	result, err := execer.ExecContext(ctx, latestSuccessfulRepairStatement(), checksum, scriptName)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func latestSuccessfulRepairStatement() string {
	return strings.TrimSpace(`
UPDATE __migrator.schema_migrations
SET checksum=@p1
WHERE id = (
    SELECT TOP (1) id
    FROM __migrator.schema_migrations
    WHERE script_name=@p2 AND success=1
    ORDER BY applied_at DESC, id DESC
)`)
}
