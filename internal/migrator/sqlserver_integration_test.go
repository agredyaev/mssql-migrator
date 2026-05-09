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

	_ "github.com/microsoft/go-mssqldb"
)

func TestSQLServerPlanBlocksChangedModuleWithoutCreateOrAlter(t *testing.T) {
	ctx := context.Background()
	conn, cfg := openSQLServerIntegrationConn(t)
	defer conn.Close()

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
	if err := metadata.InsertAttempt(ctx, conn, metadata.AttemptRecord{
		ScriptName:       schemaName + "/views/monthly.sql",
		ScriptType:       contracts.ScriptTypeObject,
		Checksum:         "previous-checksum",
		Action:           contracts.ActionCreateObject,
		Success:          true,
		TransactionMode:  config.TransactionModeScript,
		TransactionScope: config.TransactionModeScript,
		RollbackScope:    contracts.RollbackScopeScript,
		NoTransaction:    false,
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

func openSQLServerIntegrationConn(t *testing.T) (*sql.Conn, config.Config) {
	t.Helper()
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1 to run live SQL Server integration tests")
	}
	cfg := config.Config{
		Server:                 strings.TrimSpace(os.Getenv("RM_DB_SERVER")),
		Port:                   strings.TrimSpace(os.Getenv("RM_DB_PORT")),
		Database:               strings.TrimSpace(os.Getenv("RM_DB_DATABASE")),
		DBAuth:                 strings.TrimSpace(os.Getenv("RM_DB_AUTH")),
		User:                   os.Getenv("RM_DB_USER"),
		Password:               os.Getenv("RM_DB_PASSWORD"),
		Encrypt:                config.GetenvBool("RM_DB_ENCRYPT", true),
		TrustServerCertificate: config.GetenvBool("RM_DB_TRUST_SERVER_CERTIFICATE", false),
	}
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
