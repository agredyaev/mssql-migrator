package audit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

type mockRows struct {
	values [][]any
	pos    int
	closed bool
}

func newMockRows(vals [][]any) *mockRows {
	return &mockRows{values: vals, pos: -1}
}

func (m *mockRows) Scan(dest ...any) error {
	if m.pos < 0 || m.pos >= len(m.values) {
		return errors.New("no rows")
	}
	for i, v := range m.values[m.pos] {
		switch d := dest[i].(type) {
		case *string:
			*d = v.(string)
		case *int:
			*d = v.(int)
		case *bool:
			*d = v.(bool)
		}
	}
	return nil
}

func (m *mockRows) Next() bool {
	m.pos++
	return m.pos < len(m.values)
}

func (m *mockRows) Close() error {
	m.closed = true
	return nil
}

type mockConn struct {
	queries      []mockQuery
	queryCount   atomic.Int32
	queryErr     error
	execCount    atomic.Int32
	execErr      error
	rowsByPrefix map[string]*mockRows
}

type mockQuery struct {
	query string
	args  []any
}

func (m *mockConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	m.queryCount.Add(1)
	m.queries = append(m.queries, mockQuery{query: query, args: args})
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	for prefix, r := range m.rowsByPrefix {
		if len(query) >= len(prefix) && query[:len(prefix)] == prefix {
			r.pos = -1
			return r, nil
		}
	}
	return newMockRows(nil), nil
}

func (m *mockConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	m.execCount.Add(1)
	if m.execErr != nil {
		return nil, m.execErr
	}
	return nil, nil
}

func (m *mockConn) Ping(ctx context.Context) error { return nil }
func (m *mockConn) Close() error                   { return nil }

func TestLoadChecksumsEmptyKeys(t *testing.T) {
	conn := &mockConn{}
	result, err := LoadChecksums(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
	if conn.queryCount.Load() != 0 {
		t.Errorf("expected 0 queries for empty keys")
	}
}

func TestLoadChecksumsConnectionError(t *testing.T) {
	conn := &mockConn{queryErr: errors.New("dead")}
	_, err := LoadChecksums(context.Background(), conn, []string{"k1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadChecksumsReturnsResults(t *testing.T) {
	conn := &mockConn{
		rowsByPrefix: map[string]*mockRows{
			"SELECT": newMockRows([][]any{{"k1", "abc123"}}),
		},
	}
	result, err := LoadChecksums(context.Background(), conn, []string{"k1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result["k1"] != "abc123" {
		t.Errorf("checksum = %q", result["k1"])
	}
}

func TestLoadChecksumsChunking(t *testing.T) {
	keys := make([]string, 2500)
	for i := range keys {
		keys[i] = "k"
	}
	conn := &mockConn{}
	LoadChecksums(context.Background(), conn, keys)
	if conn.queryCount.Load() < 2 {
		t.Errorf("expected at least 2 queries for 2500 keys, got %d", conn.queryCount.Load())
	}
}

func TestSubscriberBootstrapOnRunStarted(t *testing.T) {
	conn := &mockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(types.EventRunStarted, &types.RunStarted{Command: "plan"})

	if conn.execCount.Load() == 0 {
		t.Error("expected bootstrap DDL on run.started")
	}
}

func TestSubscriberInsertAttemptOnObjectApplied(t *testing.T) {
	conn := &mockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(types.EventObjectApplied, &types.ObjectEvent{
		NormalizedKey: "r/tables/t1",
		Kind:          "tables",
		ObjectName:    "t1",
	})

	if conn.execCount.Load() == 0 {
		t.Error("expected INSERT attempt on object.applied")
	}
}

func TestSubscriberUpdateRunOnRunFinished(t *testing.T) {
	conn := &mockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(types.EventRunFinished, &types.RunFinished{
		Command:  "plan",
		Result:   "success",
		ExitCode: 0,
	})

	if conn.execCount.Load() < 1 {
		t.Error("expected UPDATE runs on run.finished")
	}
}

func TestSubscriberInsertItemsOnDiffComputed(t *testing.T) {
	conn := &mockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(types.EventDiffComputed, &types.DiffResult{
		Plan: &types.MigrationPlan{
			Objects: []types.PlannedObject{
				{NormalizedKey: "r/views/v1"},
			},
		},
	})

	if conn.execCount.Load() == 0 {
		t.Error("expected INSERT items on diff.computed")
	}
}
