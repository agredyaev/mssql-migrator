package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/parser"
)

var (
	ErrSchemaIncompatible   = errors.New("metadata_schema_incompatible")
	ErrMissingDDLPermission = errors.New("missing_metadata_ddl_permission")
)

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type RunRecord struct {
	RunID             string
	Command           string
	ToolVersion       string
	ToolCommit        string
	SQLRoot           string
	BaseName          string
	EffectiveBasePath string
	TargetEnvironment string
	TargetDatabase    string
	GitCommit         string
	GitBranch         string
	PipelineRunID     string
	PipelineURL       string
	AppliedBy         string
	ComparisonMode    string
	UpdatePolicy      string
	TransactionMode   string
	RollbackScope     string
	PlanFile          string
	PlanHash          string
	Success           *bool
	ErrorClass        string
	ErrorMessage      string
}

type TrackedSchemaRecord struct {
	RunID                string
	SchemaName           string
	NormalizedSchemaName string
	ExistsInDatabase     *bool
	Action               string
	Success              *bool
	ErrorMessage         string
}

type TrackedObjectRecord struct {
	RunID                string
	ObjectPath           string
	SchemaName           string
	NormalizedSchemaName string
	Kind                 string
	ObjectName           string
	NormalizedObjectName string
	ParentName           string
	NormalizedParentName string
	NormalizedKey        string
	Checksum             string
	ExistsInDatabase     *bool
	MetadataMatch        *bool
	PlannedAction        string
	Success              *bool
	ErrorMessage         string
}

type AttemptRecord struct {
	RunID            string
	TrackedObjectID  *int64
	ScriptName       string
	ScriptType       string
	Checksum         string
	Action           string
	ExecutionMS      int
	Success          bool
	ErrorMessage     string
	SQLErrorNumber   *int
	SQLErrorState    *int
	TransactionMode  string
	TransactionScope string
	RollbackScope    string
	NoTransaction    bool
	GitCommit        string
	GitBranch        string
	PipelineRunID    string
	PipelineURL      string
	AppliedBy        string
}

func Bootstrap(ctx context.Context, conn *sql.Conn) error {
	for _, statement := range bootstrapStatements() {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return classifyBootstrapError(err)
		}
	}
	return nil
}

func LoadSuccessfulChecksumsIfPresent(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	schemaMigrationsExists, err := objectExists(ctx, conn, "__migrator.schema_migrations", "U")
	if err != nil {
		return nil, err
	}
	if !schemaMigrationsExists {
		return map[string]string{}, nil
	}
	trackedObjectsExists, err := objectExists(ctx, conn, "__migrator.tracked_objects", "U")
	if err != nil {
		return nil, err
	}
	query := `
SELECT '', script_name, script_type, checksum, applied_at, execution_ms, success,
       ISNULL(error_message, ''), ISNULL(git_commit, ''), ISNULL(git_branch, ''),
       ISNULL(pipeline_run_id, ''), ISNULL(pipeline_url, ''), ISNULL(applied_by, '')
FROM __migrator.schema_migrations
WHERE success = 1
ORDER BY applied_at ASC, id ASC`
	if trackedObjectsExists {
		query = `
SELECT ISNULL(o.normalized_key, ''), m.script_name, m.checksum
FROM __migrator.schema_migrations m
LEFT JOIN __migrator.tracked_objects o ON o.tracked_object_id = m.tracked_object_id
WHERE m.success = 1
ORDER BY m.applied_at ASC, m.id ASC`
	} else {
		query = `
SELECT '', script_name, checksum
FROM __migrator.schema_migrations
WHERE success = 1
ORDER BY applied_at ASC, id ASC`
	}
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var normalizedKey string
		var scriptName string
		var checksum string
		if err := rows.Scan(&normalizedKey, &scriptName, &checksum); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(normalizedKey))
		if key == "" {
			key = parser.NormalizeTrackedName(scriptName)
		}
		if key == "" {
			continue
		}
		result[key] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func StartRun(ctx context.Context, execer Execer, record RunRecord) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO __migrator.migration_runs
(run_id, command, tool_version, tool_commit, sql_root, base_name, effective_base_path, target_environment, target_database, git_commit, git_branch, pipeline_run_id, pipeline_url, applied_by, comparison_mode, update_policy, transaction_mode, rollback_scope, plan_file, plan_hash, success, error_class, error_message)
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, @p17, @p18, @p19, @p20, @p21, @p22, @p23)`,
		record.RunID,
		record.Command,
		record.ToolVersion,
		nullable(record.ToolCommit),
		nullable(record.SQLRoot),
		nullable(record.BaseName),
		nullable(record.EffectiveBasePath),
		record.TargetEnvironment,
		record.TargetDatabase,
		nullable(record.GitCommit),
		nullable(record.GitBranch),
		nullable(record.PipelineRunID),
		nullable(record.PipelineURL),
		nullable(record.AppliedBy),
		nullable(record.ComparisonMode),
		nullable(record.UpdatePolicy),
		nullable(record.TransactionMode),
		nullable(record.RollbackScope),
		nullable(record.PlanFile),
		nullable(record.PlanHash),
		nullableBool(record.Success),
		nullable(record.ErrorClass),
		nullable(record.ErrorMessage),
	)
	return err
}

func FinishRun(ctx context.Context, execer Execer, runID string, success bool, errorClass string, errorMessage string) error {
	_, err := execer.ExecContext(ctx, `
UPDATE __migrator.migration_runs
SET finished_at = SYSUTCDATETIME(),
    success = @p2,
    error_class = @p3,
    error_message = @p4
WHERE run_id = @p1`, runID, success, nullable(errorClass), nullable(errorMessage))
	return err
}

func InsertTrackedSchema(ctx context.Context, execer Execer, record TrackedSchemaRecord) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO __migrator.tracked_schemas
(run_id, schema_name, normalized_schema_name, exists_in_database, action, success, error_message)
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7)`,
		record.RunID,
		record.SchemaName,
		record.NormalizedSchemaName,
		nullableBool(record.ExistsInDatabase),
		record.Action,
		nullableBool(record.Success),
		nullable(record.ErrorMessage),
	)
	return err
}

func InsertTrackedObject(ctx context.Context, conn *sql.Conn, record TrackedObjectRecord) (int64, error) {
	var trackedObjectID int64
	err := conn.QueryRowContext(ctx, `
INSERT INTO __migrator.tracked_objects
(run_id, object_path, schema_name, normalized_schema_name, kind, object_name, normalized_object_name, parent_name, normalized_parent_name, normalized_key, checksum, exists_in_database, metadata_match, planned_action, success, error_message)
OUTPUT INSERTED.tracked_object_id
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16)`,
		record.RunID,
		record.ObjectPath,
		record.SchemaName,
		record.NormalizedSchemaName,
		record.Kind,
		record.ObjectName,
		record.NormalizedObjectName,
		nullable(record.ParentName),
		nullable(record.NormalizedParentName),
		record.NormalizedKey,
		record.Checksum,
		nullableBool(record.ExistsInDatabase),
		nullableBool(record.MetadataMatch),
		record.PlannedAction,
		nullableBool(record.Success),
		nullable(record.ErrorMessage),
	).Scan(&trackedObjectID)
	if err != nil {
		return 0, err
	}
	return trackedObjectID, nil
}

func UpdateTrackedSchemaResult(ctx context.Context, execer Execer, runID string, normalizedSchemaName string, success bool, errorMessage string) error {
	_, err := execer.ExecContext(ctx, `
UPDATE __migrator.tracked_schemas
SET success = @p3,
    error_message = @p4
WHERE run_id = @p1 AND normalized_schema_name = @p2`, runID, normalizedSchemaName, success, nullable(errorMessage))
	return err
}

func UpdateTrackedObjectResult(ctx context.Context, execer Execer, runID string, normalizedKey string, success bool, errorMessage string) error {
	_, err := execer.ExecContext(ctx, `
UPDATE __migrator.tracked_objects
SET success = @p3,
    error_message = @p4
WHERE run_id = @p1 AND normalized_key = @p2`, runID, normalizedKey, success, nullable(errorMessage))
	return err
}

func InsertAttempt(ctx context.Context, execer Execer, record AttemptRecord) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO __migrator.schema_migrations
(run_id, tracked_object_id, script_name, script_type, checksum, action, execution_ms, success, error_message, sql_error_number, sql_error_state, transaction_mode, transaction_scope, rollback_scope, no_transaction, git_commit, git_branch, pipeline_run_id, pipeline_url, applied_by)
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, @p17, @p18, @p19, @p20)`,
		nullable(record.RunID),
		nullableInt64(record.TrackedObjectID),
		record.ScriptName,
		record.ScriptType,
		record.Checksum,
		record.Action,
		record.ExecutionMS,
		record.Success,
		nullable(record.ErrorMessage),
		nullableInt(record.SQLErrorNumber),
		nullableInt(record.SQLErrorState),
		nullable(record.TransactionMode),
		nullable(record.TransactionScope),
		nullable(record.RollbackScope),
		record.NoTransaction,
		nullable(record.GitCommit),
		nullable(record.GitBranch),
		nullable(record.PipelineRunID),
		nullable(record.PipelineURL),
		nullable(record.AppliedBy),
	)
	return err
}

func objectExists(ctx context.Context, conn *sql.Conn, name string, objectType string) (bool, error) {
	var exists int
	if err := conn.QueryRowContext(ctx, `SELECT CASE WHEN OBJECT_ID(@p1, @p2) IS NULL THEN 0 ELSE 1 END`, name, objectType).Scan(&exists); err != nil {
		return false, err
	}
	return exists == 1, nil
}

func classifyBootstrapError(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "permission") || strings.Contains(lower, "denied") || strings.Contains(lower, "not authorized") {
		return fmt.Errorf("%w: %v", ErrMissingDDLPermission, err)
	}
	return fmt.Errorf("%w: %v", ErrSchemaIncompatible, err)
}

func bootstrapStatements() []string {
	return []string{
		`IF SCHEMA_ID('__migrator') IS NULL EXEC('CREATE SCHEMA __migrator')`,
		`IF OBJECT_ID('__migrator.migration_runs', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.migration_runs (
    run_id UNIQUEIDENTIFIER NOT NULL PRIMARY KEY,
    command NVARCHAR(32) NOT NULL,
    tool_version NVARCHAR(64) NOT NULL,
    tool_commit NVARCHAR(128) NULL,
    sql_root NVARCHAR(1024) NULL,
    base_name NVARCHAR(256) NULL,
    effective_base_path NVARCHAR(2048) NULL,
    target_environment NVARCHAR(256) NOT NULL,
    target_database NVARCHAR(256) NOT NULL,
    git_commit NVARCHAR(100) NULL,
    git_branch NVARCHAR(255) NULL,
    pipeline_run_id NVARCHAR(100) NULL,
    pipeline_url NVARCHAR(500) NULL,
    applied_by NVARCHAR(255) NULL,
    comparison_mode NVARCHAR(32) NULL,
    update_policy NVARCHAR(32) NULL,
    transaction_mode NVARCHAR(32) NULL,
    rollback_scope NVARCHAR(32) NULL,
    plan_file NVARCHAR(2048) NULL,
    plan_hash NVARCHAR(64) NULL,
    started_at DATETIME2 NOT NULL CONSTRAINT DF_migration_runs_started_at DEFAULT SYSUTCDATETIME(),
    finished_at DATETIME2 NULL,
    success BIT NULL,
    error_class NVARCHAR(128) NULL,
    error_message NVARCHAR(MAX) NULL
)
END`,
		`IF OBJECT_ID('__migrator.tracked_schemas', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.tracked_schemas (
    tracked_schema_id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
    run_id UNIQUEIDENTIFIER NOT NULL,
    schema_name NVARCHAR(256) NOT NULL,
    normalized_schema_name NVARCHAR(256) NOT NULL,
    exists_in_database BIT NULL,
    action NVARCHAR(64) NOT NULL,
    success BIT NULL,
    error_message NVARCHAR(MAX) NULL,
    discovered_at DATETIME2 NOT NULL CONSTRAINT DF_tracked_schemas_discovered_at DEFAULT SYSUTCDATETIME()
)
END`,
		`IF OBJECT_ID('__migrator.tracked_objects', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.tracked_objects (
    tracked_object_id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
    run_id UNIQUEIDENTIFIER NOT NULL,
    object_path NVARCHAR(2048) NOT NULL,
    schema_name NVARCHAR(256) NOT NULL,
    normalized_schema_name NVARCHAR(256) NOT NULL,
    kind NVARCHAR(64) NOT NULL,
    object_name NVARCHAR(256) NOT NULL,
    normalized_object_name NVARCHAR(256) NOT NULL,
    parent_name NVARCHAR(256) NULL,
    normalized_parent_name NVARCHAR(256) NULL,
    normalized_key NVARCHAR(2048) NOT NULL,
    checksum NVARCHAR(64) NOT NULL,
    exists_in_database BIT NULL,
    metadata_match BIT NULL,
    planned_action NVARCHAR(64) NOT NULL,
    success BIT NULL,
    error_message NVARCHAR(MAX) NULL,
    discovered_at DATETIME2 NOT NULL CONSTRAINT DF_tracked_objects_discovered_at DEFAULT SYSUTCDATETIME()
)
END`,
		`IF OBJECT_ID('__migrator.schema_migrations', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.schema_migrations (
    id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
    run_id UNIQUEIDENTIFIER NULL,
    tracked_object_id BIGINT NULL,
    script_name NVARCHAR(512) NOT NULL,
    script_type NVARCHAR(64) NOT NULL,
    checksum NVARCHAR(64) NOT NULL,
    action NVARCHAR(64) NOT NULL,
    execution_ms INT NOT NULL,
    success BIT NOT NULL,
    error_message NVARCHAR(MAX) NULL,
    sql_error_number INT NULL,
    sql_error_state INT NULL,
    transaction_mode NVARCHAR(32) NULL,
    transaction_scope NVARCHAR(32) NULL,
    rollback_scope NVARCHAR(32) NULL,
    no_transaction BIT NOT NULL CONSTRAINT DF_schema_migrations_no_transaction DEFAULT 0,
    applied_at DATETIME2 NOT NULL CONSTRAINT DF_schema_migrations_applied_at DEFAULT SYSUTCDATETIME(),
    git_commit NVARCHAR(100) NULL,
    git_branch NVARCHAR(255) NULL,
    pipeline_run_id NVARCHAR(100) NULL,
    pipeline_url NVARCHAR(500) NULL,
    applied_by NVARCHAR(255) NULL
)
END`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'run_id') IS NULL ALTER TABLE __migrator.schema_migrations ADD run_id UNIQUEIDENTIFIER NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'tracked_object_id') IS NULL ALTER TABLE __migrator.schema_migrations ADD tracked_object_id BIGINT NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'action') IS NULL ALTER TABLE __migrator.schema_migrations ADD action NVARCHAR(64) NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'sql_error_number') IS NULL ALTER TABLE __migrator.schema_migrations ADD sql_error_number INT NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'sql_error_state') IS NULL ALTER TABLE __migrator.schema_migrations ADD sql_error_state INT NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'transaction_mode') IS NULL ALTER TABLE __migrator.schema_migrations ADD transaction_mode NVARCHAR(32) NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'transaction_scope') IS NULL ALTER TABLE __migrator.schema_migrations ADD transaction_scope NVARCHAR(32) NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'rollback_scope') IS NULL ALTER TABLE __migrator.schema_migrations ADD rollback_scope NVARCHAR(32) NULL`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'no_transaction') IS NULL ALTER TABLE __migrator.schema_migrations ADD no_transaction BIT NOT NULL CONSTRAINT DF_schema_migrations_no_transaction_upgrade DEFAULT 0`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'script_name') IS NOT NULL AND COL_LENGTH('__migrator.schema_migrations', 'script_name') < 1024
BEGIN
    ALTER TABLE __migrator.schema_migrations ALTER COLUMN script_name NVARCHAR(512) NOT NULL
END`,
		`IF COL_LENGTH('__migrator.schema_migrations', 'script_type') IS NOT NULL AND COL_LENGTH('__migrator.schema_migrations', 'script_type') < 128
BEGIN
    ALTER TABLE __migrator.schema_migrations ALTER COLUMN script_type NVARCHAR(64) NOT NULL
END`,
		`UPDATE __migrator.schema_migrations
SET script_type = 'object'
WHERE script_type NOT IN ('schema', 'object', 'validate', 'baseline', 'repair')`,
		`UPDATE __migrator.schema_migrations
SET action = CASE WHEN success = 1 THEN 'create_object' ELSE 'fail' END
WHERE action IS NULL`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_migration_runs_base' AND object_id = OBJECT_ID('__migrator.migration_runs')) DROP INDEX IX_migration_runs_base ON __migrator.migration_runs`,
		`CREATE INDEX IX_migration_runs_base ON __migrator.migration_runs (base_name, started_at DESC)`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_migration_runs_pipeline' AND object_id = OBJECT_ID('__migrator.migration_runs')) DROP INDEX IX_migration_runs_pipeline ON __migrator.migration_runs`,
		`CREATE INDEX IX_migration_runs_pipeline ON __migrator.migration_runs (pipeline_run_id) WHERE pipeline_run_id IS NOT NULL`,
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_tracked_schemas_run' AND parent_object_id = OBJECT_ID('__migrator.tracked_schemas'))
ALTER TABLE __migrator.tracked_schemas ADD CONSTRAINT FK_tracked_schemas_run FOREIGN KEY (run_id) REFERENCES __migrator.migration_runs(run_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_tracked_objects_run' AND parent_object_id = OBJECT_ID('__migrator.tracked_objects'))
ALTER TABLE __migrator.tracked_objects ADD CONSTRAINT FK_tracked_objects_run FOREIGN KEY (run_id) REFERENCES __migrator.migration_runs(run_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_schema_migrations_run' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations ADD CONSTRAINT FK_schema_migrations_run FOREIGN KEY (run_id) REFERENCES __migrator.migration_runs(run_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_schema_migrations_tracked_object' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations ADD CONSTRAINT FK_schema_migrations_tracked_object FOREIGN KEY (tracked_object_id) REFERENCES __migrator.tracked_objects(tracked_object_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_migration_runs_command' AND parent_object_id = OBJECT_ID('__migrator.migration_runs'))
ALTER TABLE __migrator.migration_runs WITH CHECK ADD CONSTRAINT CK_migration_runs_command CHECK (command IN ('plan', 'migrate', 'validate', 'baseline', 'repair-checksum'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_migration_runs_comparison_mode' AND parent_object_id = OBJECT_ID('__migrator.migration_runs'))
ALTER TABLE __migrator.migration_runs WITH CHECK ADD CONSTRAINT CK_migration_runs_comparison_mode CHECK (comparison_mode IS NULL OR comparison_mode IN ('case_insensitive', 'case_sensitive'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_migration_runs_update_policy' AND parent_object_id = OBJECT_ID('__migrator.migration_runs'))
ALTER TABLE __migrator.migration_runs WITH CHECK ADD CONSTRAINT CK_migration_runs_update_policy CHECK (update_policy IS NULL OR update_policy IN ('none', 'modules_only', 'all_supported'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_migration_runs_transaction_mode' AND parent_object_id = OBJECT_ID('__migrator.migration_runs'))
ALTER TABLE __migrator.migration_runs WITH CHECK ADD CONSTRAINT CK_migration_runs_transaction_mode CHECK (transaction_mode IS NULL OR transaction_mode IN ('script', 'none'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_migration_runs_rollback_scope' AND parent_object_id = OBJECT_ID('__migrator.migration_runs'))
ALTER TABLE __migrator.migration_runs WITH CHECK ADD CONSTRAINT CK_migration_runs_rollback_scope CHECK (rollback_scope IS NULL OR rollback_scope IN ('script', 'none'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_tracked_schemas_action' AND parent_object_id = OBJECT_ID('__migrator.tracked_schemas'))
ALTER TABLE __migrator.tracked_schemas WITH CHECK ADD CONSTRAINT CK_tracked_schemas_action CHECK (action IN ('discovered', 'exists', 'create_schema', 'skip_unchanged', 'fail'))`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'UX_tracked_schemas_run_schema' AND object_id = OBJECT_ID('__migrator.tracked_schemas')) DROP INDEX UX_tracked_schemas_run_schema ON __migrator.tracked_schemas`,
		`CREATE UNIQUE INDEX UX_tracked_schemas_run_schema ON __migrator.tracked_schemas (run_id, normalized_schema_name)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_tracked_objects_kind' AND parent_object_id = OBJECT_ID('__migrator.tracked_objects'))
ALTER TABLE __migrator.tracked_objects WITH CHECK ADD CONSTRAINT CK_tracked_objects_kind CHECK (kind IN ('tables', 'views', 'procedures', 'functions', 'triggers', 'indexes', 'types', 'sequences', 'synonyms'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_tracked_objects_planned_action' AND parent_object_id = OBJECT_ID('__migrator.tracked_objects'))
ALTER TABLE __migrator.tracked_objects WITH CHECK ADD CONSTRAINT CK_tracked_objects_planned_action CHECK (planned_action IN ('create_object', 'adopt_existing', 'skip_unchanged', 'reprocess_changed', 'reprocess_changed_blocked', 'update_existing_module', 'update_existing_supported', 'validate_checked', 'validate_skipped', 'fail'))`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'UX_tracked_objects_run_key' AND object_id = OBJECT_ID('__migrator.tracked_objects')) DROP INDEX UX_tracked_objects_run_key ON __migrator.tracked_objects`,
		`CREATE UNIQUE INDEX UX_tracked_objects_run_key ON __migrator.tracked_objects (run_id, normalized_key)`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_tracked_objects_key' AND object_id = OBJECT_ID('__migrator.tracked_objects')) DROP INDEX IX_tracked_objects_key ON __migrator.tracked_objects`,
		`CREATE INDEX IX_tracked_objects_key ON __migrator.tracked_objects (normalized_key, discovered_at DESC)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_schema_migrations_script_type' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations WITH CHECK ADD CONSTRAINT CK_schema_migrations_script_type CHECK (script_type IN ('schema', 'object', 'validate', 'baseline', 'repair'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_schema_migrations_action' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations WITH CHECK ADD CONSTRAINT CK_schema_migrations_action CHECK (action IN ('create_schema', 'create_object', 'adopt_existing', 'skip_unchanged', 'reprocess_changed', 'reprocess_changed_blocked', 'update_existing_module', 'update_existing_supported', 'validate_checked', 'validate_skipped', 'baseline', 'repair_checksum', 'fail'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_schema_migrations_transaction_mode' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations WITH CHECK ADD CONSTRAINT CK_schema_migrations_transaction_mode CHECK (transaction_mode IS NULL OR transaction_mode IN ('script', 'none'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_schema_migrations_transaction_scope' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations WITH CHECK ADD CONSTRAINT CK_schema_migrations_transaction_scope CHECK (transaction_scope IS NULL OR transaction_scope IN ('script', 'none'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_schema_migrations_rollback_scope' AND parent_object_id = OBJECT_ID('__migrator.schema_migrations'))
ALTER TABLE __migrator.schema_migrations WITH CHECK ADD CONSTRAINT CK_schema_migrations_rollback_scope CHECK (rollback_scope IS NULL OR rollback_scope IN ('script', 'none'))`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_schema_migrations_script_name' AND object_id = OBJECT_ID('__migrator.schema_migrations')) DROP INDEX IX_schema_migrations_script_name ON __migrator.schema_migrations`,
		`CREATE INDEX IX_schema_migrations_script_name ON __migrator.schema_migrations (script_name)`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_schema_migrations_success' AND object_id = OBJECT_ID('__migrator.schema_migrations')) DROP INDEX IX_schema_migrations_success ON __migrator.schema_migrations`,
		`CREATE INDEX IX_schema_migrations_success ON __migrator.schema_migrations (success, applied_at, id)`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_schema_migrations_run_id' AND object_id = OBJECT_ID('__migrator.schema_migrations')) DROP INDEX IX_schema_migrations_run_id ON __migrator.schema_migrations`,
		`CREATE INDEX IX_schema_migrations_run_id ON __migrator.schema_migrations (run_id, id) WHERE run_id IS NOT NULL`,
		`IF EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_schema_migrations_tracked_object' AND object_id = OBJECT_ID('__migrator.schema_migrations')) DROP INDEX IX_schema_migrations_tracked_object ON __migrator.schema_migrations`,
		`CREATE INDEX IX_schema_migrations_tracked_object ON __migrator.schema_migrations (tracked_object_id, success, applied_at DESC, id DESC) WHERE tracked_object_id IS NOT NULL`,
		`CREATE OR ALTER VIEW __migrator.v_migration_state AS
SELECT
    r.run_id,
    r.command,
    r.tool_version,
    r.tool_commit,
    r.sql_root,
    r.base_name,
    r.effective_base_path,
    r.target_environment,
    r.target_database,
    r.git_commit,
    r.git_branch,
    r.pipeline_run_id,
    r.pipeline_url,
    r.applied_by,
    r.comparison_mode,
    r.update_policy,
    r.transaction_mode,
    r.rollback_scope,
    r.plan_file,
    r.plan_hash,
    r.started_at,
    r.finished_at,
    r.success AS run_success,
    r.error_class AS run_error_class,
    r.error_message AS run_error_message,
    s.tracked_schema_id,
    s.schema_name,
    s.normalized_schema_name,
    s.exists_in_database AS schema_exists_in_database,
    s.action AS schema_action,
    s.success AS schema_success,
    s.error_message AS schema_error_message,
    o.tracked_object_id,
    o.object_path,
    o.kind,
    o.object_name,
    o.parent_name,
    o.normalized_key,
    o.checksum AS object_checksum,
    o.exists_in_database AS object_exists_in_database,
    o.metadata_match,
    o.planned_action,
    o.success AS object_success,
    o.error_message AS object_error_message,
    a.id AS latest_attempt_id,
    a.script_name AS latest_script_name,
    a.script_type AS latest_script_type,
    a.checksum AS latest_checksum,
    a.action AS latest_action,
    a.execution_ms AS latest_execution_ms,
    a.success AS latest_success,
    a.error_message AS latest_error_message,
    a.transaction_mode AS latest_transaction_mode,
    a.transaction_scope AS latest_transaction_scope,
    a.rollback_scope AS latest_rollback_scope,
    a.no_transaction AS latest_no_transaction,
    a.applied_at AS latest_applied_at
FROM __migrator.migration_runs r
LEFT JOIN __migrator.tracked_objects o ON o.run_id = r.run_id
LEFT JOIN __migrator.tracked_schemas s ON s.run_id = r.run_id AND (o.tracked_object_id IS NULL OR s.normalized_schema_name = o.normalized_schema_name)
OUTER APPLY (
    SELECT TOP (1) m.*
    FROM __migrator.schema_migrations m
    WHERE (o.tracked_object_id IS NOT NULL AND m.tracked_object_id = o.tracked_object_id)
       OR (o.tracked_object_id IS NULL AND m.run_id = r.run_id AND m.tracked_object_id IS NULL)
    ORDER BY m.applied_at DESC, m.id DESC
) a`,
	}
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
