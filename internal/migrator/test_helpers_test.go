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

func contractsReadValidationReport(dir string) (contracts.ValidationReport, error) {
	content, err := os.ReadFile(filepath.Join(dir, "validation-report.json"))
	if err != nil {
		return contracts.ValidationReport{}, err
	}
	var report contracts.ValidationReport
	if err := json.Unmarshal(content, &report); err != nil {
		return contracts.ValidationReport{}, err
	}
	return report, nil
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

func (s *stubExecer) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
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

func createRepoObject(t interface {
	Helper()
	Fatalf(string, ...any)
}, root string, base string, schema string, kind string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, base, schema, kind)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir repo object: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write repo object: %v", err)
	}
}
