package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"reporting-db-migrations/internal/commands"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

const metadataSchemaVersion = 2

var (
	ErrSchemaIncompatible   = errors.New("metadata_schema_incompatible")
	ErrMissingDDLPermission = errors.New("missing_metadata_ddl_permission")
)

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type QueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type ExecQueryRower interface {
	Execer
	QueryRower
}

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type ExecQueryer interface {
	Execer
	Queryer
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

type ItemRecord struct {
	RunID            string
	ObjectPath       string
	SchemaName       string
	Kind             string
	ObjectName       string
	ParentName       string
	NormalizedKey    string
	Checksum         string
	ExistsInDatabase *bool
	MetadataMatch    *bool
	Action           string
	Success          *bool
	ErrorMessage     string
}

type AttemptRecord struct {
	RunID           string
	ItemID          *int64
	ScriptName      string
	Checksum        string
	Action          string
	ExecutionMS     int
	Success         bool
	ErrorMessage    string
	SQLErrorNumber  *int
	SQLErrorState   *int
	TransactionMode string
	RollbackScope   string
	NoTransaction   bool
	GitCommit       string
	GitBranch       string
	PipelineRunID   string
	PipelineURL     string
	AppliedBy       string
}

type ItemResult struct {
	NormalizedKey string
	Success       bool
	ErrorMessage  string
}

type metadataShape struct {
	schemaVersionExists bool
	runsExists          bool
	itemsExists         bool
	attemptsExists      bool
	objectStateExists   bool
	legacyObjects       []string
}

func Bootstrap(ctx context.Context, conn *sql.Conn) error {
	shape, err := inspectMetadataShape(ctx, conn)
	if err != nil {
		return classifyBootstrapError(err)
	}
	if err := shape.ensureNoLegacy(); err != nil {
		return err
	}
	if shape.schemaVersionExists {
		if err := verifySchemaVersion(ctx, conn); err != nil {
			return err
		}
	}
	if shape.allCurrent() {
		if shape.objectStateExists {
			return nil
		}
		for _, statement := range bootstrapObjectStateStatements() {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				return classifyBootstrapError(err)
			}
		}
		return nil
	}
	for _, statement := range bootstrapStatements() {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return classifyBootstrapError(err)
		}
	}
	if err := verifySchemaVersion(ctx, conn); err != nil {
		return err
	}
	return nil
}

func LoadSuccessfulChecksumsIfPresent(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	shape, err := inspectMetadataShape(ctx, conn)
	if err != nil {
		return nil, err
	}
	if err := shape.ensureNoLegacy(); err != nil {
		return nil, err
	}
	if !shape.anyCurrent() {
		return map[string]string{}, nil
	}
	if !shape.allCurrent() {
		return nil, contracts.Wrap(ErrSchemaIncompatible, fmt.Errorf("metadata shape is incomplete: missing %s", strings.Join(shape.missingCurrentNames(), ", ")))
	}
	if err := verifySchemaVersion(ctx, conn); err != nil {
		return nil, err
	}
	if !shape.objectStateExists {
		return loadSuccessfulChecksumsFromHistory(ctx, conn)
	}
	return LoadSuccessfulChecksums(ctx, conn)
}

func LoadSuccessfulChecksums(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	return loadSuccessfulChecksums(ctx, conn, successfulChecksumsQuery())
}

func loadSuccessfulChecksumsFromHistory(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	return loadSuccessfulChecksums(ctx, conn, successfulChecksumsHistoryQuery())
}

func loadSuccessfulChecksums(ctx context.Context, conn *sql.Conn, query string) (map[string]string, error) {
	if conn == nil {
		return nil, fmt.Errorf("load successful checksums: missing connection")
	}
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var normalizedKey string
		var checksum string
		if err := rows.Scan(&normalizedKey, &checksum); err != nil {
			return nil, err
		}
		key := parser.NormalizeTrackedName(strings.ToLower(strings.TrimSpace(normalizedKey)))
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
INSERT INTO __migrator.runs
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
	result, err := execer.ExecContext(ctx, `
UPDATE __migrator.runs
SET finished_at = SYSUTCDATETIME(),
    success = @p2,
    error_class = @p3,
    error_message = @p4
WHERE run_id = @p1`, runID, success, nullable(errorClass), nullable(errorMessage))
	if err != nil {
		return err
	}
	return requireSingleRow(result, "finish run")
}

func InsertItem(ctx context.Context, execer ExecQueryRower, record ItemRecord) (int64, error) {
	var itemID int64
	err := execer.QueryRowContext(ctx, `
INSERT INTO __migrator.items
(run_id, object_path, schema_name, kind, object_name, parent_name, normalized_key, checksum, exists_in_database, metadata_match, action, success, error_message)
OUTPUT INSERTED.item_id
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13)`,
		record.RunID,
		nullable(record.ObjectPath),
		record.SchemaName,
		record.Kind,
		nullable(record.ObjectName),
		nullable(record.ParentName),
		record.NormalizedKey,
		nullable(record.Checksum),
		nullableBool(record.ExistsInDatabase),
		nullableBool(record.MetadataMatch),
		record.Action,
		nullableBool(record.Success),
		nullable(record.ErrorMessage),
	).Scan(&itemID)
	if err != nil {
		return 0, err
	}
	return itemID, nil
}

func InsertItems(ctx context.Context, execer Execer, records []ItemRecord) error {
	if len(records) == 0 {
		return nil
	}
	const columnsPerRow = 13
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
INSERT INTO __migrator.items
(run_id, object_path, schema_name, kind, object_name, parent_name, normalized_key, checksum, exists_in_database, metadata_match, action, success, error_message)
VALUES `)
	args := make([]any, 0, len(records)*columnsPerRow)
	placeholderIndex := 1
	for i, record := range records {
		if i > 0 {
			queryBuilder.WriteString(",")
		}
		queryBuilder.WriteString("(")
		for j := 0; j < columnsPerRow; j++ {
			if j > 0 {
				queryBuilder.WriteString(", ")
			}
			queryBuilder.WriteString(fmt.Sprintf("@p%d", placeholderIndex))
			placeholderIndex++
		}
		queryBuilder.WriteString(")")
		args = append(args,
			record.RunID,
			nullable(record.ObjectPath),
			record.SchemaName,
			record.Kind,
			nullable(record.ObjectName),
			nullable(record.ParentName),
			record.NormalizedKey,
			nullable(record.Checksum),
			nullableBool(record.ExistsInDatabase),
			nullableBool(record.MetadataMatch),
			record.Action,
			nullableBool(record.Success),
			nullable(record.ErrorMessage),
		)
	}
	_, err := execer.ExecContext(ctx, queryBuilder.String(), args...)
	return err
}

func LoadItemIDs(ctx context.Context, queryer Queryer, runID string, includeSchemas bool) (map[string]int64, error) {
	where := "WHERE run_id = @p1"
	if !includeSchemas {
		where += " AND kind <> 'schema'"
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT normalized_key, item_id
FROM __migrator.items
`+where, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var normalizedKey string
		var itemID int64
		if err := rows.Scan(&normalizedKey, &itemID); err != nil {
			return nil, err
		}
		result[normalizedKey] = itemID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func UpdateItemResult(ctx context.Context, execer Execer, runID string, normalizedKey string, success bool, errorMessage string) error {
	result, err := execer.ExecContext(ctx, `
UPDATE __migrator.items
SET success = @p3,
    error_message = @p4
WHERE run_id = @p1 AND normalized_key = @p2`, runID, normalizedKey, success, nullable(errorMessage))
	if err != nil {
		return err
	}
	return requireSingleRow(result, "update item result")
}

func UpdateItemResults(ctx context.Context, execer Execer, runID string, results []ItemResult) error {
	if len(results) == 0 {
		return nil
	}
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
UPDATE i
SET success = v.success,
    error_message = v.error_message
FROM __migrator.items i
JOIN (
    VALUES `)
	args := make([]any, 0, len(results)*3+1)
	placeholderIndex := 1
	for i, result := range results {
		if i > 0 {
			queryBuilder.WriteString(",")
		}
		queryBuilder.WriteString("(")
		for j := 0; j < 3; j++ {
			if j > 0 {
				queryBuilder.WriteString(", ")
			}
			queryBuilder.WriteString(fmt.Sprintf("@p%d", placeholderIndex))
			placeholderIndex++
		}
		queryBuilder.WriteString(")")
		args = append(args, result.NormalizedKey, result.Success, nullable(result.ErrorMessage))
	}
	queryBuilder.WriteString(fmt.Sprintf(`
) v(normalized_key, success, error_message)
    ON i.normalized_key = v.normalized_key
WHERE i.run_id = @p%d`, placeholderIndex))
	args = append(args, runID)
	result, err := execer.ExecContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update item results: read rows affected: %w", err)
	}
	if rows != int64(len(results)) {
		return fmt.Errorf("update item results: expected %d rows affected, got %d", len(results), rows)
	}
	return nil
}

func InsertAttempt(ctx context.Context, execer Execer, record AttemptRecord) error {
	_, err := execer.ExecContext(ctx, insertAttemptQuery(),
		attemptRecordArgs(record)...,
	)
	return err
}

func InsertAttempts(ctx context.Context, execer Execer, records []AttemptRecord) error {
	if len(records) == 0 {
		return nil
	}
	if len(records) == 1 {
		return InsertAttempt(ctx, execer, records[0])
	}
	const columnsPerRow = 18
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
DECLARE @inserted TABLE (
    run_id UNIQUEIDENTIFIER NULL,
    item_id BIGINT NULL,
    script_name NVARCHAR(512) NOT NULL,
    checksum NVARCHAR(64) NOT NULL,
    action NVARCHAR(64) NOT NULL,
    attempt_id BIGINT NOT NULL
);

INSERT INTO __migrator.attempts
(run_id, item_id, script_name, checksum, action, execution_ms, success, error_message, sql_error_number, sql_error_state, transaction_mode, rollback_scope, no_transaction, git_commit, git_branch, pipeline_run_id, pipeline_url, applied_by)
OUTPUT INSERTED.run_id, INSERTED.item_id, INSERTED.script_name, INSERTED.checksum, INSERTED.action, INSERTED.id
INTO @inserted (run_id, item_id, script_name, checksum, action, attempt_id)
SELECT
    TRY_CONVERT(UNIQUEIDENTIFIER, v.run_id),
    v.item_id,
    v.script_name,
    v.checksum,
    v.action,
    v.execution_ms,
    v.success,
    v.error_message,
    v.sql_error_number,
    v.sql_error_state,
    v.transaction_mode,
    v.rollback_scope,
    v.no_transaction,
    v.git_commit,
    v.git_branch,
    v.pipeline_run_id,
    v.pipeline_url,
    v.applied_by
FROM (
    VALUES `)
	args := make([]any, 0, len(records)*columnsPerRow)
	placeholderIndex := 1
	for i, record := range records {
		if i > 0 {
			queryBuilder.WriteString(",")
		}
		queryBuilder.WriteString("(")
		for j := 0; j < columnsPerRow; j++ {
			if j > 0 {
				queryBuilder.WriteString(", ")
			}
			queryBuilder.WriteString(fmt.Sprintf("@p%d", placeholderIndex))
			placeholderIndex++
		}
		queryBuilder.WriteString(")")
		args = append(args, attemptRecordArgs(record)...)
	}
	queryBuilder.WriteString(fmt.Sprintf(`
) v(run_id, item_id, script_name, checksum, action, execution_ms, success, error_message, sql_error_number, sql_error_state, transaction_mode, rollback_scope, no_transaction, git_commit, git_branch, pipeline_run_id, pipeline_url, applied_by);

IF OBJECT_ID('__migrator.object_state', 'U') IS NOT NULL
BEGIN
    WITH source_rows AS (
        SELECT
            COALESCE(NULLIF(i.normalized_key, ''), NULLIF(inserted.script_name, '')) AS normalized_key,
            inserted.checksum,
            inserted.attempt_id AS last_attempt_id,
            inserted.run_id AS last_run_id,
            ROW_NUMBER() OVER (
                PARTITION BY COALESCE(NULLIF(i.normalized_key, ''), NULLIF(inserted.script_name, ''))
                ORDER BY inserted.attempt_id DESC
            ) AS rn
        FROM @inserted inserted
        LEFT JOIN __migrator.items i ON i.item_id = inserted.item_id
        WHERE inserted.run_id IS NOT NULL
          AND inserted.action IN ('%s')
    )
    MERGE __migrator.object_state AS target
    USING (
        SELECT normalized_key, checksum, last_attempt_id, last_run_id
        FROM source_rows
        WHERE rn = 1
          AND normalized_key IS NOT NULL
          AND normalized_key <> ''
    ) AS source
    ON target.normalized_key = source.normalized_key
    WHEN MATCHED THEN
        UPDATE SET checksum = source.checksum,
                   last_attempt_id = source.last_attempt_id,
                   last_run_id = source.last_run_id,
                   updated_at = SYSUTCDATETIME()
    WHEN NOT MATCHED THEN
        INSERT (normalized_key, checksum, last_attempt_id, last_run_id)
        VALUES (source.normalized_key, source.checksum, source.last_attempt_id, source.last_run_id);
END`, successfulChecksumActionList()))
	_, err := execer.ExecContext(ctx, queryBuilder.String(), args...)
	return err
}

func attemptRecordArgs(record AttemptRecord) []any {
	return []any{
		nullable(record.RunID),
		nullableInt64(record.ItemID),
		record.ScriptName,
		record.Checksum,
		record.Action,
		record.ExecutionMS,
		record.Success,
		nullable(record.ErrorMessage),
		nullableInt(record.SQLErrorNumber),
		nullableInt(record.SQLErrorState),
		nullable(record.TransactionMode),
		nullable(record.RollbackScope),
		record.NoTransaction,
		nullable(record.GitCommit),
		nullable(record.GitBranch),
		nullable(record.PipelineRunID),
		nullable(record.PipelineURL),
		nullable(record.AppliedBy),
	}
}

func insertAttemptQuery() string {
	return fmt.Sprintf(`
DECLARE @attempt_id BIGINT;
DECLARE @normalized_key NVARCHAR(2048);

INSERT INTO __migrator.attempts
(run_id, item_id, script_name, checksum, action, execution_ms, success, error_message, sql_error_number, sql_error_state, transaction_mode, rollback_scope, no_transaction, git_commit, git_branch, pipeline_run_id, pipeline_url, applied_by)
VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, @p17, @p18);

SET @attempt_id = SCOPE_IDENTITY();
SET @normalized_key = COALESCE((SELECT TOP (1) NULLIF(normalized_key, '') FROM __migrator.items WHERE item_id = @p2), NULLIF(@p3, ''));

IF OBJECT_ID('__migrator.object_state', 'U') IS NOT NULL
   AND @p1 IS NOT NULL
   AND @p7 = 1
   AND @p5 IN ('%s')
   AND @normalized_key IS NOT NULL
BEGIN
    MERGE __migrator.object_state AS target
    USING (
        SELECT
            @normalized_key AS normalized_key,
            @p4 AS checksum,
            @attempt_id AS last_attempt_id,
            TRY_CONVERT(UNIQUEIDENTIFIER, @p1) AS last_run_id
    ) AS source
    ON target.normalized_key = source.normalized_key
    WHEN MATCHED THEN
        UPDATE SET checksum = source.checksum,
                   last_attempt_id = source.last_attempt_id,
                   last_run_id = source.last_run_id,
                   updated_at = SYSUTCDATETIME()
    WHEN NOT MATCHED THEN
        INSERT (normalized_key, checksum, last_attempt_id, last_run_id)
        VALUES (source.normalized_key, source.checksum, source.last_attempt_id, source.last_run_id);
END`, successfulChecksumActionList())
}

func inspectMetadataShape(ctx context.Context, conn *sql.Conn) (metadataShape, error) {
	shape := metadataShape{}
	rows, err := conn.QueryContext(ctx, metadataShapeQuery())
	if err != nil {
		return metadataShape{}, err
	}
	defer rows.Close()
	exists := map[string]bool{}
	for rows.Next() {
		var schemaName string
		var objectName string
		var objectType string
		if err := rows.Scan(&schemaName, &objectName, &objectType); err != nil {
			return metadataShape{}, err
		}
		exists[metadataShapeKey(schemaName, objectName, objectType)] = true
	}
	if err := rows.Err(); err != nil {
		return metadataShape{}, err
	}
	shape.schemaVersionExists = exists[metadataShapeKey("__migrator", "schema_version", "U")]
	shape.runsExists = exists[metadataShapeKey("__migrator", "runs", "U")]
	shape.itemsExists = exists[metadataShapeKey("__migrator", "items", "U")]
	shape.attemptsExists = exists[metadataShapeKey("__migrator", "attempts", "U")]
	shape.objectStateExists = exists[metadataShapeKey("__migrator", "object_state", "U")]
	for _, item := range legacyMetadataObjects() {
		schemaName, objectName := splitQualifiedObjectName(item.name)
		if exists[metadataShapeKey(schemaName, objectName, item.objectType)] {
			shape.legacyObjects = append(shape.legacyObjects, item.name)
		}
	}
	sort.Strings(shape.legacyObjects)
	return shape, nil
}

func (s metadataShape) anyCurrent() bool {
	return s.schemaVersionExists || s.runsExists || s.itemsExists || s.attemptsExists || s.objectStateExists
}

func (s metadataShape) allCurrent() bool {
	return s.schemaVersionExists && s.runsExists && s.itemsExists && s.attemptsExists
}

func (s metadataShape) ensureNoLegacy() error {
	if len(s.legacyObjects) == 0 {
		return nil
	}
	return contracts.Wrap(ErrSchemaIncompatible, fmt.Errorf("legacy metadata objects present: %s", strings.Join(s.legacyObjects, ", ")))
}

func (s metadataShape) missingCurrentNames() []string {
	missing := []string{}
	if !s.schemaVersionExists {
		missing = append(missing, "__migrator.schema_version")
	}
	if !s.runsExists {
		missing = append(missing, "__migrator.runs")
	}
	if !s.itemsExists {
		missing = append(missing, "__migrator.items")
	}
	if !s.attemptsExists {
		missing = append(missing, "__migrator.attempts")
	}
	return missing
}

func legacyMetadataObjects() []struct {
	name       string
	objectType string
} {
	return []struct {
		name       string
		objectType string
	}{
		{name: "__migrator.schema_migrations", objectType: "U"},
		{name: "__migrator.migration_runs", objectType: "U"},
		{name: "__migrator.tracked_schemas", objectType: "U"},
		{name: "__migrator.tracked_objects", objectType: "U"},
		{name: "__migrator.migration_attempts", objectType: "U"},
		{name: "__migrator.v_migration_state", objectType: "V"},
	}
}

func classifyBootstrapError(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "permission") || strings.Contains(lower, "denied") || strings.Contains(lower, "not authorized") {
		return contracts.Wrap(ErrMissingDDLPermission, err)
	}
	return contracts.Wrap(ErrSchemaIncompatible, err)
}

func verifySchemaVersion(ctx context.Context, conn *sql.Conn) error {
	var maxVersion sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT MAX(version) FROM __migrator.schema_version`).Scan(&maxVersion); err != nil {
		return contracts.Wrap(ErrSchemaIncompatible, err)
	}
	if !maxVersion.Valid {
		return contracts.Wrap(ErrSchemaIncompatible, fmt.Errorf("metadata schema version row is missing"))
	}
	if maxVersion.Valid && maxVersion.Int64 != metadataSchemaVersion {
		return contracts.Wrap(ErrSchemaIncompatible, fmt.Errorf("metadata schema version %d does not match supported version %d", maxVersion.Int64, metadataSchemaVersion))
	}
	return nil
}

func successfulChecksumsQuery() string {
	return `
SELECT normalized_key, checksum
FROM __migrator.object_state`
}

func successfulChecksumsHistoryQuery() string {
	return fmt.Sprintf(`
WITH ranked AS (
    SELECT
        COALESCE(NULLIF(i.normalized_key, ''), NULLIF(a.script_name, '')) AS normalized_key,
        a.checksum,
        ROW_NUMBER() OVER (
            PARTITION BY COALESCE(NULLIF(i.normalized_key, ''), NULLIF(a.script_name, ''))
            ORDER BY a.applied_at DESC, a.id DESC
        ) AS rn
    FROM __migrator.attempts a
    LEFT JOIN __migrator.items i ON i.item_id = a.item_id
    WHERE a.success = 1
      AND a.action IN ('%s')
)
SELECT normalized_key, checksum
FROM ranked
WHERE rn = 1
  AND normalized_key IS NOT NULL
  AND normalized_key <> ''`, successfulChecksumActionList())
}

func metadataShapeQuery() string {
	return `
SELECT
    s.name,
    o.name,
    o.type
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE s.name = '__migrator'
  AND o.name IN (
      'schema_version',
      'runs',
      'items',
      'attempts',
      'object_state',
      'schema_migrations',
      'migration_runs',
      'tracked_schemas',
      'tracked_objects',
      'migration_attempts',
      'v_migration_state'
  )`
}

func metadataShapeKey(schemaName string, objectName string, objectType string) string {
	return strings.ToLower(strings.TrimSpace(schemaName)) + "." + strings.ToLower(strings.TrimSpace(objectName)) + ":" + strings.ToUpper(strings.TrimSpace(objectType))
}

func splitQualifiedObjectName(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 {
		return "", trimmed
	}
	return parts[0], parts[1]
}

func bootstrapStatements() []string {
	statements := []string{}
	statements = append(statements, bootstrapSchemaStatements()...)
	statements = append(statements, bootstrapTableStatements()...)
	statements = append(statements, bootstrapObjectStateStatements()...)
	statements = append(statements, bootstrapForeignKeyStatements()...)
	statements = append(statements, bootstrapCheckConstraintStatements()...)
	statements = append(statements, bootstrapIndexStatements()...)
	return statements
}

func bootstrapSchemaStatements() []string {
	return []string{
		`IF SCHEMA_ID('__migrator') IS NULL EXEC('CREATE SCHEMA __migrator')`,
		`IF OBJECT_ID('__migrator.schema_version', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.schema_version (
    version INT NOT NULL PRIMARY KEY,
    applied_at DATETIME2 NOT NULL CONSTRAINT DF_schema_version_applied_at DEFAULT SYSUTCDATETIME()
)
END`,
		fmt.Sprintf(`IF NOT EXISTS (SELECT 1 FROM __migrator.schema_version WHERE version = %d)
INSERT INTO __migrator.schema_version(version) VALUES (%d)`, metadataSchemaVersion, metadataSchemaVersion),
	}
}

func bootstrapTableStatements() []string {
	return []string{
		`IF OBJECT_ID('__migrator.runs', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.runs (
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
    started_at DATETIME2 NOT NULL CONSTRAINT DF_runs_started_at DEFAULT SYSUTCDATETIME(),
    finished_at DATETIME2 NULL,
    success BIT NULL,
    error_class NVARCHAR(128) NULL,
    error_message NVARCHAR(MAX) NULL
)
END`,
		`IF OBJECT_ID('__migrator.items', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.items (
    item_id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
    run_id UNIQUEIDENTIFIER NOT NULL,
    object_path NVARCHAR(2048) NULL,
    schema_name NVARCHAR(256) NOT NULL,
    kind NVARCHAR(64) NOT NULL,
    object_name NVARCHAR(256) NULL,
    parent_name NVARCHAR(256) NULL,
    normalized_key NVARCHAR(2048) NOT NULL,
    checksum NVARCHAR(64) NULL,
    exists_in_database BIT NULL,
    metadata_match BIT NULL,
    action NVARCHAR(64) NOT NULL,
    success BIT NULL,
    error_message NVARCHAR(MAX) NULL,
    created_at DATETIME2 NOT NULL CONSTRAINT DF_items_created_at DEFAULT SYSUTCDATETIME()
)
END`,
		`IF OBJECT_ID('__migrator.attempts', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.attempts (
    id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
    run_id UNIQUEIDENTIFIER NULL,
    item_id BIGINT NULL,
    script_name NVARCHAR(512) NOT NULL,
    checksum NVARCHAR(64) NOT NULL,
    action NVARCHAR(64) NOT NULL,
    execution_ms INT NOT NULL,
    success BIT NOT NULL,
    error_message NVARCHAR(MAX) NULL,
    sql_error_number INT NULL,
    sql_error_state INT NULL,
    transaction_mode NVARCHAR(32) NULL,
    rollback_scope NVARCHAR(32) NULL,
    no_transaction BIT NOT NULL CONSTRAINT DF_attempts_no_transaction DEFAULT 0,
    applied_at DATETIME2 NOT NULL CONSTRAINT DF_attempts_applied_at DEFAULT SYSUTCDATETIME(),
    git_commit NVARCHAR(100) NULL,
    git_branch NVARCHAR(255) NULL,
    pipeline_run_id NVARCHAR(100) NULL,
    pipeline_url NVARCHAR(500) NULL,
    applied_by NVARCHAR(255) NULL
)
END`,
	}
}

func bootstrapObjectStateStatements() []string {
	return []string{
		`IF OBJECT_ID('__migrator.object_state', 'U') IS NULL
BEGIN
CREATE TABLE __migrator.object_state (
    normalized_key NVARCHAR(2048) NOT NULL PRIMARY KEY,
    checksum NVARCHAR(64) NOT NULL,
    last_attempt_id BIGINT NULL,
    last_run_id UNIQUEIDENTIFIER NULL,
    updated_at DATETIME2 NOT NULL CONSTRAINT DF_object_state_updated_at DEFAULT SYSUTCDATETIME()
)
END`,
		fmt.Sprintf(`IF OBJECT_ID('__migrator.object_state', 'U') IS NOT NULL
   AND OBJECT_ID('__migrator.attempts', 'U') IS NOT NULL
   AND OBJECT_ID('__migrator.items', 'U') IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM __migrator.object_state)
BEGIN
WITH ranked AS (
    SELECT
        COALESCE(NULLIF(i.normalized_key, ''), NULLIF(a.script_name, '')) AS normalized_key,
        a.checksum,
        a.id AS last_attempt_id,
        a.run_id AS last_run_id,
        ROW_NUMBER() OVER (
            PARTITION BY COALESCE(NULLIF(i.normalized_key, ''), NULLIF(a.script_name, ''))
            ORDER BY a.applied_at DESC, a.id DESC
        ) AS rn
    FROM __migrator.attempts a
    LEFT JOIN __migrator.items i ON i.item_id = a.item_id
    WHERE a.success = 1
      AND a.action IN ('%s')
)
INSERT INTO __migrator.object_state (normalized_key, checksum, last_attempt_id, last_run_id)
SELECT normalized_key, checksum, last_attempt_id, last_run_id
FROM ranked
WHERE rn = 1
  AND normalized_key IS NOT NULL
  AND normalized_key <> ''
END`, successfulChecksumActionList()),
	}
}

func successfulChecksumActionList() string {
	return strings.Join([]string{
		contracts.ActionSkipUnchanged,
		contracts.ActionAdoptExisting,
		contracts.ActionCreateObject,
		contracts.ActionReprocessChanged,
		contracts.ActionUpdateExistingModule,
		contracts.ActionUpdateExistingSupported,
		contracts.ActionRepairChecksum,
	}, `', '`)
}

func bootstrapForeignKeyStatements() []string {
	return []string{
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_items_run' AND parent_object_id = OBJECT_ID('__migrator.items'))
ALTER TABLE __migrator.items ADD CONSTRAINT FK_items_run FOREIGN KEY (run_id) REFERENCES __migrator.runs(run_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_attempts_run' AND parent_object_id = OBJECT_ID('__migrator.attempts'))
ALTER TABLE __migrator.attempts ADD CONSTRAINT FK_attempts_run FOREIGN KEY (run_id) REFERENCES __migrator.runs(run_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = 'FK_attempts_item' AND parent_object_id = OBJECT_ID('__migrator.attempts'))
ALTER TABLE __migrator.attempts ADD CONSTRAINT FK_attempts_item FOREIGN KEY (item_id) REFERENCES __migrator.items(item_id)`,
	}
}

func bootstrapCheckConstraintStatements() []string {
	commandNames := commands.Names()
	kindNames := bootstrapKindNames()
	actionNames := bootstrapActionNames()
	return []string{
		fmt.Sprintf(`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_runs_command' AND parent_object_id = OBJECT_ID('__migrator.runs'))
ALTER TABLE __migrator.runs WITH CHECK ADD CONSTRAINT CK_runs_command CHECK (command IN ('%s'))`, strings.Join(commandNames, `', '`)),
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_runs_comparison_mode' AND parent_object_id = OBJECT_ID('__migrator.runs'))
ALTER TABLE __migrator.runs WITH CHECK ADD CONSTRAINT CK_runs_comparison_mode CHECK (comparison_mode IS NULL OR comparison_mode IN ('case_insensitive', 'case_sensitive'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_runs_update_policy' AND parent_object_id = OBJECT_ID('__migrator.runs'))
ALTER TABLE __migrator.runs WITH CHECK ADD CONSTRAINT CK_runs_update_policy CHECK (update_policy IS NULL OR update_policy IN ('none', 'modules_only', 'all_supported'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_runs_transaction_mode' AND parent_object_id = OBJECT_ID('__migrator.runs'))
ALTER TABLE __migrator.runs WITH CHECK ADD CONSTRAINT CK_runs_transaction_mode CHECK (transaction_mode IS NULL OR transaction_mode IN ('script', 'none'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_runs_rollback_scope' AND parent_object_id = OBJECT_ID('__migrator.runs'))
ALTER TABLE __migrator.runs WITH CHECK ADD CONSTRAINT CK_runs_rollback_scope CHECK (rollback_scope IS NULL OR rollback_scope IN ('script', 'none'))`,
		fmt.Sprintf(`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_items_kind' AND parent_object_id = OBJECT_ID('__migrator.items'))
ALTER TABLE __migrator.items WITH CHECK ADD CONSTRAINT CK_items_kind CHECK (kind IN ('schema', '%s'))`, strings.Join(kindNames, `', '`)),
		fmt.Sprintf(`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_items_action' AND parent_object_id = OBJECT_ID('__migrator.items'))
ALTER TABLE __migrator.items WITH CHECK ADD CONSTRAINT CK_items_action CHECK (action IN ('%s'))`, strings.Join(actionNames, `', '`)),
		fmt.Sprintf(`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_attempts_action' AND parent_object_id = OBJECT_ID('__migrator.attempts'))
ALTER TABLE __migrator.attempts WITH CHECK ADD CONSTRAINT CK_attempts_action CHECK (action IN ('%s'))`, strings.Join(actionNames, `', '`)),
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_attempts_transaction_mode' AND parent_object_id = OBJECT_ID('__migrator.attempts'))
ALTER TABLE __migrator.attempts WITH CHECK ADD CONSTRAINT CK_attempts_transaction_mode CHECK (transaction_mode IS NULL OR transaction_mode IN ('script', 'none'))`,
		`IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_attempts_rollback_scope' AND parent_object_id = OBJECT_ID('__migrator.attempts'))
ALTER TABLE __migrator.attempts WITH CHECK ADD CONSTRAINT CK_attempts_rollback_scope CHECK (rollback_scope IS NULL OR rollback_scope IN ('script', 'none'))`,
	}
}

func bootstrapIndexStatements() []string {
	return []string{
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_runs_base' AND object_id = OBJECT_ID('__migrator.runs'))
CREATE INDEX IX_runs_base ON __migrator.runs (base_name, started_at DESC)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_runs_pipeline' AND object_id = OBJECT_ID('__migrator.runs'))
CREATE INDEX IX_runs_pipeline ON __migrator.runs (pipeline_run_id) WHERE pipeline_run_id IS NOT NULL`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'UX_items_run_key' AND object_id = OBJECT_ID('__migrator.items'))
CREATE UNIQUE INDEX UX_items_run_key ON __migrator.items (run_id, normalized_key)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_items_key' AND object_id = OBJECT_ID('__migrator.items'))
CREATE INDEX IX_items_key ON __migrator.items (normalized_key, created_at DESC)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_items_run_kind' AND object_id = OBJECT_ID('__migrator.items'))
		CREATE INDEX IX_items_run_kind ON __migrator.items (run_id, kind, item_id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_attempts_script_name' AND object_id = OBJECT_ID('__migrator.attempts'))
CREATE INDEX IX_attempts_script_name ON __migrator.attempts (script_name)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_attempts_success' AND object_id = OBJECT_ID('__migrator.attempts'))
		CREATE INDEX IX_attempts_success ON __migrator.attempts (success, applied_at, id)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_attempts_run_id' AND object_id = OBJECT_ID('__migrator.attempts'))
CREATE INDEX IX_attempts_run_id ON __migrator.attempts (run_id, id) WHERE run_id IS NOT NULL`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'IX_attempts_item' AND object_id = OBJECT_ID('__migrator.attempts'))
CREATE INDEX IX_attempts_item ON __migrator.attempts (item_id, success, applied_at DESC, id DESC) WHERE item_id IS NOT NULL`,
	}
}

func bootstrapKindNames() []string {
	return []string{"tables", "views", "procedures", "functions", "triggers", "indexes", "types", "sequences", "synonyms"}
}

func bootstrapActionNames() []string {
	return []string{
		contracts.SchemaActionExists,
		contracts.SchemaActionCreateSchema,
		contracts.ActionCreateObject,
		contracts.ActionAdoptExisting,
		contracts.ActionSkipUnchanged,
		contracts.ActionReprocessChanged,
		contracts.ActionReprocessChangedBlocked,
		contracts.ActionUpdateExistingModule,
		contracts.ActionUpdateExistingSupported,
		contracts.ActionValidateChecked,
		contracts.ActionValidateSkipped,
		contracts.ActionRepairChecksum,
		contracts.ActionFail,
	}
}

func requireSingleRow(result sql.Result, operation string) error {
	if result == nil {
		return fmt.Errorf("%s: missing SQL result", operation)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: read rows affected: %w", operation, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s: expected 1 row affected, got %d", operation, rows)
	}
	return nil
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
