package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/testutil"
	"reporting-db-migrations/internal/types"
)

func mockConnector(conn *testutil.MockConn) Connector {
	return func(ctx context.Context, cfg types.Config) (driver.Conn, error) {
		return conn, nil
	}
}

func mockConnectorErr(err error) Connector {
	return func(ctx context.Context, cfg types.Config) (driver.Conn, error) {
		return nil, err
	}
}

func envLookup(server, database, sqlRoot, reportDir string) envLookupFn {
	return func(key string) (string, bool) {
		switch key {
		case "RM_DB_SERVER":
			return server, true
		case "RM_DB_DATABASE":
			return database, true
		case "RM_SQL_ROOT":
			return sqlRoot, true
		case "RM_SQL_BASE":
			return sqlRoot, true
		case "RM_REPORT_DIR":
			return reportDir, true
		default:
			return "", false
		}
	}
}

func createTempSQLLayout(t *testing.T, base string) {
	t.Helper()
	db := "dactests"
	schema := "smoke"
	writeSQLFile(t, base, db, schema, "views", "monthly.sql", "CREATE OR ALTER VIEW smoke.monthly AS SELECT 1 AS n")
	writeSQLFile(t, base, db, schema, "procedures", "refresh.sql", "CREATE OR ALTER PROC smoke.refresh AS SELECT 1")
	writeSQLFile(t, base, db, schema, "tables", "data_table.sql", "CREATE TABLE smoke.data_table (id INT NOT NULL)")
}

func createTempSQLLayoutWithTable(t *testing.T, base string) {
	t.Helper()
	createTempSQLLayout(t, base)
	db := "dactests"
	schema := "smoke"
	writeSQLFile(t, base, db, schema, "tables", "data_table.sql", "CREATE TABLE smoke.data_table (id INT NOT NULL, new_col NVARCHAR(100) NULL)")
}

func writeSQLFile(t *testing.T, base, db, schema, kind, name, content string) {
	t.Helper()
	dir := filepath.Join(base, db, schema, kind)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func emptyDBConn(t *testing.T) *testutil.MockConn {
	t.Helper()
	return &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT LOWER(s.name) AS schem": testutil.NewMockRows([][]any{{"smoke"}}),
		},
	}
}

func lockOKConn(t *testing.T) *testutil.MockConn {
	t.Helper()
	conn := emptyDBConn(t)
	conn.RowsByPrefix["DECLARE @result INT;\nEXEC @res"] = testutil.NewMockRows([][]any{{0}})
	conn.RowsByPrefix["DECLARE @result INT;\nEXEC @resu"] = testutil.NewMockRows([][]any{{0}})
	return conn
}

func lockTimeoutConn(t *testing.T) *testutil.MockConn {
	t.Helper()
	conn := emptyDBConn(t)
	conn.RowsByPrefix["DECLARE @result INT;\nEXEC @res"] = testutil.NewMockRows([][]any{{-1}})
	return conn
}

func TestE2E_ConfigError_MissingServer(t *testing.T) {
	lookup := envLookup("", "testdb", "/tmp/sql", "/tmp/report")
	code := runWithLookup([]string{"rmig", "plan"}, lookup, nil)
	if code != types.ExitConfigError {
		t.Errorf("exit code = %d, want %d (ExitConfigError)", code, types.ExitConfigError)
	}
}

func TestE2E_ConfigError_MissingDatabase(t *testing.T) {
	lookup := envLookup("localhost", "", "/tmp/sql", "/tmp/report")
	code := runWithLookup([]string{"rmig", "plan"}, lookup, nil)
	if code != types.ExitConfigError {
		t.Errorf("exit code = %d, want %d (ExitConfigError)", code, types.ExitConfigError)
	}
}

func TestE2E_ConfigError_ConnectionFailed(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()
	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	code := runWithLookup([]string{"rmig", "plan"}, lookup, mockConnectorErr(errors.New("connection refused")))
	if code == 0 {
		t.Error("expected non-zero exit code for connection failure")
	}
}

func TestE2E_Plan_Success(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()
	createTempSQLLayout(t, sqlRoot)

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	conn := emptyDBConn(t)
	code := runWithLookup([]string{"rmig", "plan"}, lookup, mockConnector(conn))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	if conn.ExecCount.Load() == 0 {
		t.Error("expected at least 1 exec call (bootstrap)")
	}
	if conn.QueryCount.Load() == 0 {
		t.Error("expected at least 1 query call (inspect + checksums)")
	}

	planPath := filepath.Join(reportDir, ".plan.json")
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		t.Error(".plan.json was not written")
	}
	reportPath := filepath.Join(reportDir, ".report.json")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Error(".report.json was not written")
	}
}

func TestE2E_Plan_EmptyLayout(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	conn := emptyDBConn(t)
	code := runWithLookup([]string{"rmig", "plan"}, lookup, mockConnector(conn))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (plan on empty layout should succeed)", code)
	}
	if conn.ExecCount.Load() == 0 {
		t.Error("expected at least 1 exec call (bootstrap)")
	}
}

func TestE2E_Migrate_Success(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()
	createTempSQLLayout(t, sqlRoot)

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"DECLARE @result INT;\nEXEC @res":  testutil.NewMockRows([][]any{{0}}),
			"DECLARE @result INT;\nEXEC @resu": testutil.NewMockRows([][]any{{0}}),
		},
	}
	code := runWithLookup([]string{"rmig", "migrate"}, lookup, mockConnector(conn))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if conn.ExecCount.Load() < 2 {
		t.Errorf("expected >= 2 exec calls (bootstrap + apply), got %d", conn.ExecCount.Load())
	}
	if len(conn.ExecQueries) == 0 {
		t.Fatal("expected SQL execution")
	}

	foundCreate := false
	for _, q := range conn.ExecQueries {
		if strings.HasPrefix(q, "CREATE TABLE") || strings.HasPrefix(q, "CREATE OR ALTER") {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Errorf("expected CREATE TABLE or CREATE OR ALTER in exec queries, got %d: %v", len(conn.ExecQueries), conn.ExecQueries)
	}
}

func TestE2E_Migrate_LockTimeout(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()
	createTempSQLLayout(t, sqlRoot)

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	conn := lockTimeoutConn(t)
	code := runWithLookup([]string{"rmig", "migrate"}, lookup, mockConnector(conn))

	if code == 0 {
		t.Fatal("expected non-zero exit code for lock timeout")
	}
	if code != types.ExitLockTimeout {
		t.Errorf("exit code = %d, want %d (ExitLockTimeout)", code, types.ExitLockTimeout)
	}
}

func TestE2E_Validate_Success(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()
	createTempSQLLayout(t, sqlRoot)

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	conn := emptyDBConn(t)
	code := runWithLookup([]string{"rmig", "validate"}, lookup, mockConnector(conn))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestE2E_Baseline_Success(t *testing.T) {
	sqlRoot := t.TempDir()
	reportDir := t.TempDir()
	createTempSQLLayout(t, sqlRoot)

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	conn := lockOKConn(t)
	code := runWithLookup([]string{"rmig", "baseline"}, lookup, mockConnector(conn))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if conn.ExecCount.Load() < 2 {
		t.Errorf("expected >= 2 exec calls (bootstrap + apply), got %d", conn.ExecCount.Load())
	}
}

func TestE2E_Migrate_BlockedPlan(t *testing.T) {
	sqlRoot := t.TempDir()
	sqlBase := sqlRoot
	reportDir := t.TempDir()

	createTempSQLLayoutWithTable(t, sqlRoot)

	lookup := envLookup("localhost", "testdb", sqlRoot, reportDir)
	lookupWithBase := func(k string) (string, bool) {
		if k == "RM_SQL_BASE" {
			return sqlBase, true
		}
		return lookup(k)
	}

	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT LOWER(s.name) AS schem": testutil.NewMockRows([][]any{{"smoke"}}),
			"SELECT\n    LOWER(s.name) AS ": testutil.NewMockRows([][]any{{"smoke", "tables", "data_table", ""}}),
			"SELECT h.normalized_key, h.che": testutil.NewMockRows([][]any{{"smoke/tables/data_table", "oldhash"}}),
		},
	}

	code := runWithLookup([]string{"rmig", "migrate"}, lookupWithBase, mockConnector(conn))

	if code == 0 {
		t.Fatal("expected non-zero exit code for blocked plan")
	}
	if code != types.ExitPlanBlocked {
		t.Errorf("exit code = %d, want %d (ExitPlanBlocked)", code, types.ExitPlanBlocked)
	}

	migrationDir := filepath.Join(sqlBase, "dactests", "smoke", "tables", "_migrations", "data_table")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("readdir migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected scaffold to create migration file")
	}
	hasContent := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(migrationDir, e.Name()))
		if err != nil {
			continue
		}
		if len(data) > 0 {
			hasContent = true
		}
	}
	if !hasContent {
		t.Error("migration file is empty")
	}
}
