//go:build integration

package app

import (
	"context"
	"fmt"
	"os"
	"testing"

	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/types"
)

// TestMain creates the integration test database once per `go test` invocation.
// Individual tests must not call DROP/CREATE again unless resetTestDatabase is used
// or RMIG_TEST_DB_RESET=always.
func TestMain(m *testing.M) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") == "1" && testDBResetMode() != "never" {
		ctx := context.Background()
		if err := doEnsureTestDatabase(ctx, configFromEnv()); err != nil {
			fmt.Fprintf(os.Stderr, "integration TestMain: ensure database: %v\n", err)
			os.Exit(2)
		}
	}
	os.Exit(m.Run())
}

func testDBResetMode() string {
	if v := os.Getenv("RMIG_TEST_DB_RESET"); v != "" {
		return v
	}
	return "once"
}

// ensureTestDatabase is a no-op when RMIG_TEST_DB_RESET=once (default): TestMain already
// dropped/created the database. Set RMIG_TEST_DB_RESET=always to reset per call, or
// never/skip to never reset.
func ensureTestDatabase(t *testing.T, ctx context.Context) {
	t.Helper()
	switch testDBResetMode() {
	case "never", "skip":
		t.Log("ensureTestDatabase: skipped (RMIG_TEST_DB_RESET)")
		return
	case "always":
		if err := doEnsureTestDatabase(ctx, configFromEnv()); err != nil {
			t.Fatalf("ensureTestDatabase: %v", err)
		}
	default:
		t.Log("ensureTestDatabase: skipped (once per go test; see TestMain)")
	}
}

// resetTestDatabase forces DROP/CREATE (e.g. RMIG_TEST_DB_RESET=always on one test).
func resetTestDatabase(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := doEnsureTestDatabase(ctx, configFromEnv()); err != nil {
		t.Fatalf("resetTestDatabase: %v", err)
	}
}

func doEnsureTestDatabase(ctx context.Context, cfg types.Config) error {
	dbName := cfg.Database
	masterCfg := cfg
	masterCfg.Database = "master"
	conn, err := mssql.Open(ctx, masterCfg)
	if err != nil {
		return fmt.Errorf("connect to master: %w", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, `
		IF DB_ID('`+dbName+`') IS NOT NULL
		BEGIN
			ALTER DATABASE [`+dbName+`] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
			DROP DATABASE [`+dbName+`];
		END
		CREATE DATABASE [`+dbName+`]
	`, nil)
	if err != nil {
		return fmt.Errorf("drop/create database %s: %w", dbName, err)
	}
	return nil
}
