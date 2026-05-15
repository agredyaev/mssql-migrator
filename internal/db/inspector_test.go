package db

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type mockRows struct {
	columns []string
	values  [][]any
	pos     int
	closed  bool
}

func newMockRows(cols []string, vals [][]any) *mockRows {
	return &mockRows{columns: cols, values: vals, pos: -1}
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

func (m *mockRows) Err() error { return nil }
func (m *mockRows) Close() error {
	if m.closed {
		return errors.New("already closed")
	}
	m.closed = true
	return nil
}

type mockConn struct {
	queries    []mockQuery
	queryCount atomic.Int32
	queryErr   error
	rows       map[string]*mockRows
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
	key := query[:min(len(query), 30)]
	if r, ok := m.rows[key]; ok {
		r.pos = -1
		return r, nil
	}
	return newMockRows(nil, nil), nil
}

func (m *mockConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	return nil, nil
}

func (m *mockConn) Ping(ctx context.Context) error { return nil }
func (m *mockConn) Close() error                   { return nil }

func TestInspectEmptyScope(t *testing.T) {
	insp := NewInspector()
	conn := &mockConn{}
	layout := fs.Layout{}
	state, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Schemas) != 0 {
		t.Errorf("expected 0 schemas, got %d", len(state.Schemas))
	}
	if len(state.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(state.Objects))
	}
	if conn.queryCount.Load() != 0 {
		t.Errorf("expected 0 queries, got %d", conn.queryCount.Load())
	}
}

func TestInspectCachesResult(t *testing.T) {
	insp := NewInspector()
	conn := &mockConn{}
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}
	state1, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("first Inspect: %v", err)
	}
	state2, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("second Inspect: %v", err)
	}
	if state1 != state2 {
		t.Error("cached state should be same pointer")
	}
	qc := conn.queryCount.Load()
	if qc == 0 {
		t.Fatal("expected at least 1 query for first Inspect")
	}
	state3, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("third Inspect: %v", err)
	}
	if state3 != state1 {
		t.Error("cached state should be same pointer")
	}
	if conn.queryCount.Load() != qc {
		t.Errorf("expected query count to stay at %d, got %d", qc, conn.queryCount.Load())
	}
}

func TestInspectConnectionError(t *testing.T) {
	insp := NewInspector()
	conn := &mockConn{queryErr: errors.New("connection refused")}
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}
	_, err := insp.Inspect(context.Background(), conn, layout)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInspectDifferentScopeNotCached(t *testing.T) {
	insp := NewInspector()
	conn := &mockConn{}
	layout1 := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}
	layout2 := fs.Layout{
		Schemas: []fs.Schema{{Name: "x", NormalizedName: "x"}},
	}
	state1, _ := insp.Inspect(context.Background(), conn, layout1)
	state2, _ := insp.Inspect(context.Background(), conn, layout2)
	if state1 == state2 {
		t.Error("different scopes should not share cached state")
	}
	qc := conn.queryCount.Load()
	if qc < 2 {
		t.Errorf("expected at least 2 queries, got %d", qc)
	}
}

func TestChunkKeys_Empty(t *testing.T) {
	chunks := types.ChunkKeys(nil, driver.DefaultMaxParameters)
	if len(chunks) != 0 {
		t.Error("expected empty chunks for nil")
	}
	chunks = types.ChunkKeys([]string{}, driver.DefaultMaxParameters)
	if len(chunks) != 0 {
		t.Error("expected empty chunks for empty slice")
	}
}

func TestChunkKeys_LargeBatch(t *testing.T) {
	keys := make([]string, 4200)
	for i := range keys {
		keys[i] = "k"
	}
	chunks := types.ChunkKeys(keys, driver.DefaultMaxParameters)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for 4200 keys, got %d", len(chunks))
	}
	if len(chunks[0]) != 2100 {
		t.Errorf("first chunk = %d, want 2100", len(chunks[0]))
	}
	if len(chunks[1]) != 2100 {
		t.Errorf("second chunk = %d, want 2100", len(chunks[1]))
	}
}

func TestBuildINQuery(t *testing.T) {
	q, args := types.BuildINQuery("SELECT * FROM t WHERE c IN ({{list}})", "{{list}}", []string{"a", "b", "c"})
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
	if args[0] != "a" || args[1] != "b" || args[2] != "c" {
		t.Errorf("unexpected args: %v", args)
	}
	if len(q) == 0 {
		t.Error("query is empty")
	}
}

func TestBuildDualINQuery(t *testing.T) {
	q, args := buildDualINQuery(
		"SELECT * FROM t WHERE s IN ({{s}}) AND o IN ({{o}})",
		"{{s}}", []string{"s1"},
		"{{o}}", []string{"o1", "o2"},
	)
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
	if args[0] != "s1" {
		t.Errorf("arg0 = %q", args[0])
	}
	if args[1] != "o1" {
		t.Errorf("arg1 = %q", args[1])
	}
	if args[2] != "o2" {
		t.Errorf("arg2 = %q", args[2])
	}
	if len(q) == 0 {
		t.Error("query is empty")
	}
}
