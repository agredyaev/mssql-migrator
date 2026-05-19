//go:build integration

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestIntegration_Inspect_UsesOpenJSON(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	ctx := context.Background()
	cfg := dbConfigFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SQLBase = sqlRoot

	ensureDBTestDatabase(t, ctx, cfg)

	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	layout, err := fs.NewScanner().Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	state, err := NewInspector().Inspect(ctx, conn, layout)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(state.Objects) != 0 {
		t.Fatalf("expected empty DB state, got %d objects", len(state.Objects))
	}
}

func dbConfigFromEnv() types.Config {
	return types.Config{
		Server:                 dbEnvOrDefault("RM_DB_SERVER", "localhost"),
		Port:                   dbEnvOrDefault("RM_DB_PORT", "1433"),
		Database:               dbEnvOrDefault("RM_DB_DATABASE", "dactests"),
		DBAuth:                 dbEnvOrDefault("RM_DB_AUTH", "sql"),
		User:                   dbEnvOrDefault("RM_DB_USER", "sa"),
		Password:               dbEnvOrDefault("RM_DB_PASSWORD", "yourStrong(!)Password"),
		Encrypt:                false,
		TrustServerCertificate: true,
		SQLRoot:                dbEnvOrDefault("RM_SQL_ROOT", "sql"),
		SQLBase:                dbEnvOrDefault("RM_SQL_BASE", "sql"),
		LogLevel:               dbEnvOrDefault("RM_LOG_LEVEL", "debug"),
	}
}

func ensureDBTestDatabase(t *testing.T, ctx context.Context, cfg types.Config) {
	t.Helper()

	masterCfg := cfg
	masterCfg.Database = "master"
	conn, err := mssql.Open(ctx, masterCfg)
	if err != nil {
		t.Fatalf("connect to master: %v", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, `
		IF DB_ID('`+cfg.Database+`') IS NOT NULL
		BEGIN
			ALTER DATABASE [`+cfg.Database+`] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
			DROP DATABASE [`+cfg.Database+`];
		END
		CREATE DATABASE [`+cfg.Database+`]
	`, nil)
	if err != nil {
		t.Fatalf("drop/create database %s: %v", cfg.Database, err)
	}
}

func dbEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
