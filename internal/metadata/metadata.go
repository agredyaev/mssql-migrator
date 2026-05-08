package metadata

import (
	"context"
	"database/sql"

	"reporting-db-migrations/internal/state"
)

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func Bootstrap(ctx context.Context, conn *sql.Conn) error {
	statements := []string{
		`IF SCHEMA_ID('__migrator') IS NULL EXEC('CREATE SCHEMA __migrator')`,
		`IF OBJECT_ID('__migrator.schema_migrations', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.schema_migrations (
    id BIGINT IDENTITY(1,1) PRIMARY KEY,
    script_name NVARCHAR(255) NOT NULL,
    script_type NVARCHAR(20) NOT NULL,
    checksum NVARCHAR(64) NOT NULL,
    applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
    execution_ms INT NOT NULL,
    success BIT NOT NULL,
    error_message NVARCHAR(MAX) NULL,
    git_commit NVARCHAR(100) NULL,
    git_branch NVARCHAR(255) NULL,
    pipeline_run_id NVARCHAR(100) NULL,
    pipeline_url NVARCHAR(500) NULL,
    applied_by NVARCHAR(255) NULL
)
END`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_schema_migrations_script_name' AND object_id = OBJECT_ID('__migrator.schema_migrations'))
CREATE INDEX IX_schema_migrations_script_name ON __migrator.schema_migrations(script_name)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_schema_migrations_success' AND object_id = OBJECT_ID('__migrator.schema_migrations'))
CREATE INDEX IX_schema_migrations_success ON __migrator.schema_migrations(success, applied_at)`,
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func LoadState(ctx context.Context, conn *sql.Conn) (state.State, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT script_name, script_type, checksum, applied_at, execution_ms, success,
       ISNULL(error_message, ''), ISNULL(git_commit, ''), ISNULL(git_branch, ''),
       ISNULL(pipeline_run_id, ''), ISNULL(pipeline_url, ''), ISNULL(applied_by, '')
FROM __migrator.schema_migrations
ORDER BY applied_at ASC, id ASC`)
	if err != nil {
		return state.State{}, err
	}
	defer rows.Close()
	attempts := make([]state.Attempt, 0)
	for rows.Next() {
		attempt := state.Attempt{}
		if err := rows.Scan(&attempt.ScriptName, &attempt.ScriptType, &attempt.Checksum, &attempt.AppliedAt, &attempt.ExecutionMS, &attempt.Success, &attempt.ErrorMessage, &attempt.GitCommit, &attempt.GitBranch, &attempt.PipelineRunID, &attempt.PipelineURL, &attempt.AppliedBy); err != nil {
			return state.State{}, err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return state.State{}, err
	}
	return state.New(attempts), nil
}

func RecordAttempt(ctx context.Context, execer Execer, attempt state.Attempt) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO __migrator.schema_migrations
(script_name, script_type, checksum, execution_ms, success, error_message, git_commit, git_branch, pipeline_run_id, pipeline_url, applied_by)
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11)`,
		attempt.ScriptName, attempt.ScriptType, attempt.Checksum, attempt.ExecutionMS, attempt.Success, nullable(attempt.ErrorMessage), attempt.GitCommit, attempt.GitBranch, attempt.PipelineRunID, attempt.PipelineURL, attempt.AppliedBy)
	return err
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
