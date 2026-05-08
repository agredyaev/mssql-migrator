package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"reporting-db-migrations/internal/contracts"
)

func contractsReadMigrationReport(dir string) (contracts.MigrationReport, error) {
	content, err := os.ReadFile(filepath.Join(dir, "migration-report.json"))
	if err != nil {
		return contracts.MigrationReport{}, err
	}
	var report contracts.MigrationReport
	if err := json.Unmarshal(content, &report); err != nil {
		return contracts.MigrationReport{}, err
	}
	return report, nil
}

func containsSecret(value string) bool {
	return strings.Contains(value, "password=secret") || strings.Contains(value, "secret")
}

type execCall struct {
	query string
	args  []any
}

type stubExecer struct {
	result sql.Result
	err    error
	calls  []execCall
}

func (s *stubExecer) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	s.calls = append(s.calls, execCall{query: query, args: args})
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

type stubResult struct {
	rows int64
}

func (s stubResult) LastInsertId() (int64, error) { return 0, nil }

func (s stubResult) RowsAffected() (int64, error) { return s.rows, nil }

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func containsAny(value string, parts []string) bool {
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}
