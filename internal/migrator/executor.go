package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/state"
)

func (r Runner) executePlan(ctx context.Context, conn *sql.Conn, migrationState state.State, report *contracts.MigrationReport) error {
	versioned, repeatable, _, err := parser.Discover(r.cfg.SQLDir)
	if err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrInvalidInput, err)
	}
	for _, script := range versioned {
		if _, exists := migrationState.SuccessByScript[script.Name]; exists {
			report.Skipped = append(report.Skipped, toScriptResult(script, "already_applied"))
			continue
		}
		if err := r.applyScript(ctx, conn, script, report); err != nil {
			return err
		}
	}
	updatedState, err := metadata.LoadState(ctx, conn)
	if err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	for _, script := range repeatable {
		latest, exists := updatedState.SuccessByScript[script.Name]
		if exists && latest.Checksum == script.Checksum {
			report.Skipped = append(report.Skipped, toScriptResult(script, "unchanged"))
			continue
		}
		if err := r.applyScript(ctx, conn, script, report); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) applyScript(parent context.Context, conn *sql.Conn, script parser.Script, report *contracts.MigrationReport) error {
	ctx, cancel := context.WithTimeout(parent, r.cfg.ScriptTimeout)
	defer cancel()
	startedAt := time.Now()
	stopProgress := r.startProgressLogger(ctx, script.Name, startedAt)
	executionErr := r.executeScript(ctx, conn, script)
	stopProgress()
	executionMS := int(time.Since(startedAt).Milliseconds())
	attempt := state.Attempt{ScriptName: script.Name, ScriptType: string(script.Type), Checksum: script.Checksum, ExecutionMS: executionMS, Success: executionErr == nil, GitCommit: r.cfg.GitCommit, GitBranch: r.cfg.GitBranch, PipelineRunID: r.cfg.PipelineRunID, PipelineURL: r.cfg.PipelineURL, AppliedBy: r.cfg.Actor}
	if executionErr != nil {
		attempt.ErrorMessage = executionErr.Error()
		_ = metadata.RecordAttempt(parent, conn, attempt)
		report.Result = "failed"
		report.Failed = &contracts.Failure{Script: script.Name, Error: executionErr.Error()}
		return fmt.Errorf("%w: %v", contracts.ErrSQLExecution, executionErr)
	}
	if err := metadata.RecordAttempt(parent, conn, attempt); err != nil {
		report.Result = "failed"
		report.Failed = &contracts.Failure{Script: script.Name, Error: "critical metadata failure after successful SQL: " + err.Error()}
		return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	report.Applied = append(report.Applied, contracts.ScriptResult{Script: script.Name, Type: string(script.Type), Checksum: script.Checksum, ExecutionMS: executionMS})
	r.log.Info("script_applied", fmt.Sprintf("script=%s execution_ms=%d", script.Name, executionMS))
	return nil
}

func (r Runner) executeScript(ctx context.Context, conn *sql.Conn, script parser.Script) error {
	content, err := os.ReadFile(script.Path)
	if err != nil {
		return err
	}
	batches, err := parser.SplitGO(string(content))
	if err != nil {
		return err
	}
	if script.NoTransaction {
		return executeBatches(ctx, conn, batches)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := executeBatches(ctx, tx, batches); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func executeBatches(ctx context.Context, execer metadata.Execer, batches []parser.Batch) error {
	for _, batch := range batches {
		for i := 0; i < batch.Repeat; i++ {
			if _, err := execer.ExecContext(ctx, batch.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r Runner) startProgressLogger(ctx context.Context, scriptName string, startedAt time.Time) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.log.Info("script_running", fmt.Sprintf("script=%s elapsed=%s", scriptName, time.Since(startedAt).Round(time.Second)))
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { close(done) }
}

func toScriptResult(script parser.Script, reason string) contracts.ScriptResult {
	return contracts.ScriptResult{Script: script.Name, Type: string(script.Type), Checksum: script.Checksum, Reason: reason}
}
