package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"

	"github.com/google/uuid"
	_ "github.com/microsoft/go-mssqldb"
)

func TestSQLServerPlanBlocksChangedModuleWithoutCreateOrAlter(t *testing.T) {
	ctx := context.Background()
	conn, cfg := openSQLServerIntegrationConn(t)
	defer conn.Close()
	resetIntegrationMetadata(t, ctx, conn)
	t.Cleanup(func() { resetIntegrationMetadata(t, ctx, conn) })

	schemaName := integrationSchemaName("blockedplan")
	resetIntegrationSchema(t, ctx, conn, schemaName)
	t.Cleanup(func() { resetIntegrationSchema(t, ctx, conn, schemaName) })

	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA ["+schemaName+"]"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "EXEC(N'CREATE VIEW ["+schemaName+"].[monthly] AS SELECT 1 AS id;')"); err != nil {
		t.Fatalf("create view: %v", err)
	}
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		t.Fatalf("bootstrap metadata: %v", err)
	}
	if err := seedSuccessfulMetadataObject(ctx, conn, cfg.Database, metadata.ItemRecord{
		ObjectPath:    schemaName + "/views/monthly.sql",
		SchemaName:    schemaName,
		Kind:          "views",
		ObjectName:    "monthly",
		NormalizedKey: schemaName + "/views/monthly",
		Checksum:      "previous-checksum",
		Action:        contracts.ActionCreateObject,
	}, metadata.AttemptRecord{
		ScriptName:      schemaName + "/views/monthly.sql",
		Checksum:        "previous-checksum",
		Action:          contracts.ActionCreateObject,
		Success:         true,
		TransactionMode: config.TransactionModeScript,
		RollbackScope:   contracts.RollbackScopeScript,
		NoTransaction:   false,
	}); err != nil {
		t.Fatalf("seed metadata drift: %v", err)
	}

	root := t.TempDir()
	base := "dwh"
	createRepoObject(t, root, base, schemaName, "views", "monthly.sql", "CREATE VIEW ["+schemaName+"].[monthly] AS SELECT 2 AS id;")

	reportDir := t.TempDir()
	runner := NewRunner(config.Config{
		Env:                    "pred",
		SQLRoot:                root,
		SQLBase:                base,
		EffectiveBasePath:      filepath.Join(root, base),
		ReportDir:              reportDir,
		Database:               cfg.Database,
		Server:                 cfg.Server,
		Port:                   cfg.Port,
		DBAuth:                 cfg.DBAuth,
		User:                   cfg.User,
		Password:               cfg.Password,
		Encrypt:                cfg.Encrypt,
		TrustServerCertificate: cfg.TrustServerCertificate,
		GitCommit:              "integration-test",
		UpdatePolicy:           config.UpdatePolicyModulesOnly,
		TransactionMode:        config.TransactionModeScript,
		ToolVersion:            "test",
		ToolCommit:             "test",
	}, logger.New(logger.Options{}))

	plan, err := runner.Plan(ctx)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if !plan.Blocked {
		t.Fatalf("expected blocked plan, got %#v", plan)
	}
	if len(plan.BlockReasons) == 0 || !strings.Contains(strings.Join(plan.BlockReasons, " | "), "CREATE OR ALTER") {
		t.Fatalf("expected CREATE OR ALTER blocker, got %#v", plan.BlockReasons)
	}
	content, err := os.ReadFile(filepath.Join(reportDir, "migration-plan.json"))
	if err != nil {
		t.Fatalf("read plan report: %v", err)
	}
	var report contracts.MigrationPlan
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode plan report: %v", err)
	}
	if !report.Blocked {
		t.Fatalf("expected blocked plan report, got %#v", report)
	}
}

func TestSQLServerValidateRunsChecksAndRefreshesModules(t *testing.T) {
	ctx := context.Background()
	conn, cfg := openSQLServerIntegrationConn(t)
	defer conn.Close()
	resetIntegrationMetadata(t, ctx, conn)
	t.Cleanup(func() { resetIntegrationMetadata(t, ctx, conn) })

	schemaName := integrationSchemaName("validate")
	resetIntegrationSchema(t, ctx, conn, schemaName)
	t.Cleanup(func() { resetIntegrationSchema(t, ctx, conn, schemaName) })

	root := t.TempDir()
	base := "dwh"
	createRepoObject(t, root, base, schemaName, "views", "monthly.sql", "CREATE OR ALTER VIEW ["+schemaName+"].[monthly] AS SELECT 1 AS id;")
	checkDir := filepath.Join(root, base, schemaName, "checks")
	if err := os.MkdirAll(checkDir, 0o755); err != nil {
		t.Fatalf("mkdir checks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(checkDir, "smoke.sql"), []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatalf("write check: %v", err)
	}

	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA ["+schemaName+"]"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "EXEC(N'CREATE VIEW ["+schemaName+"].[monthly] AS SELECT 1 AS id;')"); err != nil {
		t.Fatalf("create view: %v", err)
	}

	reportDir := t.TempDir()
	runner := NewRunner(config.Config{
		Env:                    "pred",
		SQLRoot:                root,
		SQLBase:                base,
		EffectiveBasePath:      filepath.Join(root, base),
		ReportDir:              reportDir,
		Database:               cfg.Database,
		Server:                 cfg.Server,
		Port:                   cfg.Port,
		DBAuth:                 cfg.DBAuth,
		User:                   cfg.User,
		Password:               cfg.Password,
		Encrypt:                cfg.Encrypt,
		TrustServerCertificate: cfg.TrustServerCertificate,
		GitCommit:              "integration-test",
		UpdatePolicy:           config.UpdatePolicyNone,
		TransactionMode:        config.TransactionModeScript,
		ToolVersion:            "test",
		ToolCommit:             "test",
	}, logger.New(logger.Options{}))

	if err := runner.Validate(ctx); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(reportDir, "validation-report.json"))
	if err != nil {
		t.Fatalf("read validation report: %v", err)
	}
	var report contracts.ValidationReport
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode validation report: %v", err)
	}
	if report.Result != "success" {
		t.Fatalf("expected success validation report, got %#v", report)
	}
	if report.Validation.ModulesRefreshed != 1 {
		t.Fatalf("expected one refreshed module, got %#v", report.Validation)
	}
	if report.Validation.ChecksPassed != 1 {
		t.Fatalf("expected one passed check, got %#v", report.Validation)
	}
}

func TestSQLServerBaselineForcesUpdatePolicyNone(t *testing.T) {
	ctx := context.Background()
	conn, cfg := openSQLServerIntegrationConn(t)
	defer conn.Close()
	resetIntegrationMetadata(t, ctx, conn)
	t.Cleanup(func() { resetIntegrationMetadata(t, ctx, conn) })

	schemaName := integrationSchemaName("baselinepolicy")
	resetIntegrationSchema(t, ctx, conn, schemaName)
	t.Cleanup(func() { resetIntegrationSchema(t, ctx, conn, schemaName) })

	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA ["+schemaName+"]"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "EXEC(N'CREATE VIEW ["+schemaName+"].[monthly] AS SELECT 1 AS id;')"); err != nil {
		t.Fatalf("create view: %v", err)
	}
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		t.Fatalf("bootstrap metadata: %v", err)
	}
	if err := seedSuccessfulMetadataObject(ctx, conn, cfg.Database, metadata.ItemRecord{
		ObjectPath:    schemaName + "/views/monthly.sql",
		SchemaName:    schemaName,
		Kind:          "views",
		ObjectName:    "monthly",
		NormalizedKey: schemaName + "/views/monthly",
		Checksum:      "previous-checksum",
		Action:        contracts.ActionCreateObject,
	}, metadata.AttemptRecord{
		ScriptName:      schemaName + "/views/monthly.sql",
		Checksum:        "previous-checksum",
		Action:          contracts.ActionCreateObject,
		Success:         true,
		TransactionMode: config.TransactionModeScript,
		RollbackScope:   contracts.RollbackScopeScript,
		NoTransaction:   false,
	}); err != nil {
		t.Fatalf("seed metadata drift: %v", err)
	}

	root := t.TempDir()
	base := "dwh"
	createRepoObject(t, root, base, schemaName, "views", "monthly.sql", "CREATE OR ALTER VIEW ["+schemaName+"].[monthly] AS SELECT 2 AS id;")

	runner := NewRunner(config.Config{
		Env:                    "pred",
		SQLRoot:                root,
		SQLBase:                base,
		EffectiveBasePath:      filepath.Join(root, base),
		ReportDir:              t.TempDir(),
		Database:               cfg.Database,
		Server:                 cfg.Server,
		Port:                   cfg.Port,
		DBAuth:                 cfg.DBAuth,
		User:                   cfg.User,
		Password:               cfg.Password,
		Encrypt:                cfg.Encrypt,
		TrustServerCertificate: cfg.TrustServerCertificate,
		GitCommit:              "integration-test",
		UpdatePolicy:           config.UpdatePolicyModulesOnly,
		TransactionMode:        config.TransactionModeScript,
		ToolVersion:            "test",
		ToolCommit:             "test",
	}, logger.New(logger.Options{}))

	err := runner.Baseline(ctx)
	if err == nil {
		t.Fatal("expected baseline drift failure")
	}
	if !strings.Contains(err.Error(), contracts.ErrMetadataDrift.Error()) {
		t.Fatalf("expected metadata drift failure, got %v", err)
	}
	if got := latestRunUpdatePolicy(t, ctx, conn, contracts.CommandBaseline, root); got != config.UpdatePolicyNone {
		t.Fatalf("expected baseline run update_policy %q, got %q", config.UpdatePolicyNone, got)
	}
	if got := latestItemAction(t, ctx, conn, contracts.CommandBaseline, root, schemaName+"/views/monthly"); got != contracts.ActionReprocessChangedBlocked {
		t.Fatalf("expected baseline item action %q, got %q", contracts.ActionReprocessChangedBlocked, got)
	}
}

func TestSQLServerMigrateBootstrapsPartialMetadataBeforeChecksumLoad(t *testing.T) {
	ctx := context.Background()
	conn, cfg := openSQLServerIntegrationConn(t)
	defer conn.Close()
	resetIntegrationMetadata(t, ctx, conn)
	t.Cleanup(func() { resetIntegrationMetadata(t, ctx, conn) })

	schemaName := integrationSchemaName("migratepartial")
	resetIntegrationSchema(t, ctx, conn, schemaName)
	t.Cleanup(func() { resetIntegrationSchema(t, ctx, conn, schemaName) })

	root := t.TempDir()
	base := "dwh"
	createRepoObject(t, root, base, schemaName, "views", "monthly.sql", "CREATE VIEW ["+schemaName+"].[monthly] AS SELECT 1 AS id;")

	reportDir := t.TempDir()
	runner := NewRunner(config.Config{
		Env:                    "pred",
		SQLRoot:                root,
		SQLBase:                base,
		EffectiveBasePath:      filepath.Join(root, base),
		ReportDir:              reportDir,
		Database:               cfg.Database,
		Server:                 cfg.Server,
		Port:                   cfg.Port,
		DBAuth:                 cfg.DBAuth,
		User:                   cfg.User,
		Password:               cfg.Password,
		Encrypt:                cfg.Encrypt,
		TrustServerCertificate: cfg.TrustServerCertificate,
		GitCommit:              "integration-test",
		UpdatePolicy:           config.UpdatePolicyNone,
		TransactionMode:        config.TransactionModeScript,
		ToolVersion:            "test",
		ToolCommit:             "test",
	}, logger.New(logger.Options{}))

	plan, err := runner.Plan(ctx)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if plan.Blocked {
		t.Fatalf("expected unblocked plan, got %#v", plan)
	}

	resetIntegrationMetadata(t, ctx, conn)
	createPartialCurrentMetadata(t, ctx, conn)
	if !integrationObjectExists(t, ctx, conn, "__migrator.schema_version", "U") {
		t.Fatal("expected partial metadata schema_version table")
	}
	if integrationObjectExists(t, ctx, conn, "__migrator.runs", "U") {
		t.Fatal("expected metadata runs table to be missing before bootstrap repair")
	}

	runner.cfg.PlanFile = filepath.Join(reportDir, "migration-plan.json")
	if err := runner.Migrate(ctx); err != nil {
		t.Fatalf("migrate failed with partial metadata shape: %v", err)
	}
	if !integrationObjectExists(t, ctx, conn, "__migrator.runs", "U") {
		t.Fatal("expected bootstrap to create metadata runs table")
	}
	if !integrationObjectExists(t, ctx, conn, "["+schemaName+"].[monthly]", "V") {
		t.Fatal("expected migrate to create repo-managed view")
	}
	content, err := os.ReadFile(filepath.Join(reportDir, "migration-report.json"))
	if err != nil {
		t.Fatalf("read migration report: %v", err)
	}
	var report contracts.MigrationReport
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode migration report: %v", err)
	}
	if report.Validation.ModulesRefreshed != 0 {
		t.Fatalf("expected post-migrate validation to skip module refresh, got %#v", report.Validation)
	}
}

func openSQLServerIntegrationConn(t *testing.T) (*sql.Conn, config.Config) {
	t.Helper()
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1 to run live SQL Server integration tests")
	}
	cfg := config.Config{
		Server:   strings.TrimSpace(os.Getenv("RM_DB_SERVER")),
		Port:     strings.TrimSpace(os.Getenv("RM_DB_PORT")),
		Database: strings.TrimSpace(os.Getenv("RM_DB_DATABASE")),
		DBAuth:   strings.TrimSpace(os.Getenv("RM_DB_AUTH")),
		User:     os.Getenv("RM_DB_USER"),
		Password: os.Getenv("RM_DB_PASSWORD"),
	}
	encrypt, err := config.GetenvBool("RM_DB_ENCRYPT", true)
	if err != nil {
		t.Fatalf("parse RM_DB_ENCRYPT: %v", err)
	}
	trustServerCertificate, err := config.GetenvBool("RM_DB_TRUST_SERVER_CERTIFICATE", false)
	if err != nil {
		t.Fatalf("parse RM_DB_TRUST_SERVER_CERTIFICATE: %v", err)
	}
	cfg.Encrypt = encrypt
	cfg.TrustServerCertificate = trustServerCertificate
	if cfg.Port == "" {
		cfg.Port = "1433"
	}
	if cfg.DBAuth == "" {
		cfg.DBAuth = config.DBAuthSQL
	}
	if cfg.Server == "" || cfg.Database == "" {
		t.Fatal("RM_DB_SERVER and RM_DB_DATABASE are required for live SQL Server integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	conn, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve integration connection: %v", err)
	}
	return conn, cfg
}

func resetIntegrationSchema(t *testing.T, ctx context.Context, conn *sql.Conn, schemaName string) {
	t.Helper()
	cleanup := "IF SCHEMA_ID('" + schemaName + "') IS NOT NULL BEGIN DECLARE @sql NVARCHAR(MAX) = N''; SELECT @sql = @sql + N'DROP VIEW [' + s.name + N'].[' + o.name + N'];' FROM sys.views o JOIN sys.schemas s ON s.schema_id = o.schema_id WHERE s.name = '" + schemaName + "'; EXEC sp_executesql @sql; SET @sql = N''; SELECT @sql = @sql + N'DROP TABLE [' + s.name + N'].[' + o.name + N'];' FROM sys.tables o JOIN sys.schemas s ON s.schema_id = o.schema_id WHERE s.name = '" + schemaName + "'; EXEC sp_executesql @sql; EXEC('DROP SCHEMA [" + schemaName + "]'); END"
	if _, err := conn.ExecContext(ctx, cleanup); err != nil {
		t.Fatalf("reset schema %s: %v", schemaName, err)
	}
}

func integrationSchemaName(prefix string) string {
	return "itest_" + prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func seedSuccessfulMetadataObject(ctx context.Context, conn *sql.Conn, targetDatabase string, item metadata.ItemRecord, attempt metadata.AttemptRecord) error {
	runID := uuid.NewString()
	if err := metadata.StartRun(ctx, conn, metadata.RunRecord{
		RunID:             runID,
		Command:           contracts.CommandMigrate,
		ToolVersion:       "test",
		TargetEnvironment: "pred",
		TargetDatabase:    targetDatabase,
	}); err != nil {
		return err
	}
	item.RunID = runID
	itemID, err := metadata.InsertItem(ctx, conn, item)
	if err != nil {
		return err
	}
	attempt.RunID = runID
	attempt.ItemID = &itemID
	if err := metadata.InsertAttempt(ctx, conn, attempt); err != nil {
		return err
	}
	return metadata.FinishRun(ctx, conn, runID, true, "", "")
}

func resetIntegrationMetadata(t *testing.T, ctx context.Context, conn *sql.Conn) {
	t.Helper()
	cleanup := "IF OBJECT_ID('__migrator.attempts', 'U') IS NOT NULL DROP TABLE __migrator.attempts; IF OBJECT_ID('__migrator.items', 'U') IS NOT NULL DROP TABLE __migrator.items; IF OBJECT_ID('__migrator.runs', 'U') IS NOT NULL DROP TABLE __migrator.runs; IF OBJECT_ID('__migrator.schema_version', 'U') IS NOT NULL DROP TABLE __migrator.schema_version; IF OBJECT_ID('__migrator.v_migration_state', 'V') IS NOT NULL DROP VIEW __migrator.v_migration_state; IF OBJECT_ID('__migrator.migration_attempts', 'U') IS NOT NULL DROP TABLE __migrator.migration_attempts; IF OBJECT_ID('__migrator.tracked_objects', 'U') IS NOT NULL DROP TABLE __migrator.tracked_objects; IF OBJECT_ID('__migrator.tracked_schemas', 'U') IS NOT NULL DROP TABLE __migrator.tracked_schemas; IF OBJECT_ID('__migrator.migration_runs', 'U') IS NOT NULL DROP TABLE __migrator.migration_runs; IF OBJECT_ID('__migrator.schema_migrations', 'U') IS NOT NULL DROP TABLE __migrator.schema_migrations; IF SCHEMA_ID('__migrator') IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sys.objects o JOIN sys.schemas s ON s.schema_id = o.schema_id WHERE s.name = '__migrator') EXEC('DROP SCHEMA __migrator');"
	if _, err := conn.ExecContext(ctx, cleanup); err != nil {
		t.Fatalf("reset metadata: %v", err)
	}
}

func createPartialCurrentMetadata(t *testing.T, ctx context.Context, conn *sql.Conn) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, "IF SCHEMA_ID('__migrator') IS NULL EXEC('CREATE SCHEMA __migrator')"); err != nil {
		t.Fatalf("create metadata schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE TABLE __migrator.schema_version (version INT NOT NULL PRIMARY KEY, applied_at DATETIME2 NOT NULL CONSTRAINT DF_schema_version_applied_at DEFAULT SYSUTCDATETIME())"); err != nil {
		t.Fatalf("create schema_version table: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO __migrator.schema_version(version) VALUES (2)"); err != nil {
		t.Fatalf("seed schema_version row: %v", err)
	}
}

func integrationObjectExists(t *testing.T, ctx context.Context, conn *sql.Conn, name string, objectType string) bool {
	t.Helper()
	var exists int
	if err := conn.QueryRowContext(ctx, "SELECT CASE WHEN OBJECT_ID(@p1, @p2) IS NULL THEN 0 ELSE 1 END", name, objectType).Scan(&exists); err != nil {
		t.Fatalf("check object %s: %v", name, err)
	}
	return exists == 1
}

func latestRunUpdatePolicy(t *testing.T, ctx context.Context, conn *sql.Conn, command string, sqlRoot string) string {
	t.Helper()
	var value string
	if err := conn.QueryRowContext(ctx, "SELECT TOP (1) ISNULL(update_policy, '') FROM __migrator.runs WHERE command = @p1 AND sql_root = @p2 ORDER BY started_at DESC", command, sqlRoot).Scan(&value); err != nil {
		t.Fatalf("read latest run update_policy: %v", err)
	}
	return value
}

func latestItemAction(t *testing.T, ctx context.Context, conn *sql.Conn, command string, sqlRoot string, normalizedKey string) string {
	t.Helper()
	var value string
	query := "SELECT TOP (1) i.action FROM __migrator.items i JOIN __migrator.runs r ON r.run_id = i.run_id WHERE r.command = @p1 AND r.sql_root = @p2 AND i.normalized_key = @p3 ORDER BY i.created_at DESC, i.item_id DESC"
	if err := conn.QueryRowContext(ctx, query, command, sqlRoot, normalizedKey).Scan(&value); err != nil {
		t.Fatalf("read latest item action: %v", err)
	}
	return value
}
