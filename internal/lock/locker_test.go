package lock

import (
	"context"
	"errors"
	"testing"
	"time"

	"reporting-db-migrations/internal/driver"
	rmerrors "reporting-db-migrations/internal/errors"
)

type mockRows struct {
	values [][]any
	pos    int
}

func newMockRows(values [][]any) *mockRows {
	return &mockRows{values: values, pos: -1}
}

func (m *mockRows) Scan(dest ...any) error {
	if m.pos < 0 || m.pos >= len(m.values) {
		return errors.New("no rows")
	}
	for i, v := range m.values[m.pos] {
		*(dest[i].(*int)) = v.(int)
	}
	return nil
}

func (m *mockRows) Next() bool {
	m.pos++
	return m.pos < len(m.values)
}

func (m *mockRows) Close() error { return nil }

type mockConn struct {
	querySQL  string
	queryArgs []any
	rows      *mockRows
	queryErr  error
}

func (m *mockConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	m.querySQL = query
	m.queryArgs = args
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.rows, nil
}

func (m *mockConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	return nil, nil
}

func (m *mockConn) Ping(ctx context.Context) error { return nil }
func (m *mockConn) Close() error                   { return nil }

func TestAcquireSuccess(t *testing.T) {
	conn := &mockConn{rows: newMockRows([][]any{{0}})}
	locker := New()
	err := locker.Acquire(context.Background(), conn, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.querySQL == "" {
		t.Fatal("query was not executed")
	}
	t.Logf("query: %s", conn.querySQL)
	timeout, ok := conn.queryArgs[0].(int64)
	if !ok {
		t.Fatalf("unexpected arg type: %T", conn.queryArgs[0])
	}
	if timeout != 30000 {
		t.Errorf("timeout = %d, want 30000", timeout)
	}
}

func TestAcquireTimeout(t *testing.T) {
	conn := &mockConn{rows: newMockRows([][]any{{-1}})}
	locker := New()
	err := locker.Acquire(context.Background(), conn, time.Second)
	if !errors.Is(err, rmerrors.ErrLockTimeout) {
		t.Errorf("expected ErrLockTimeout, got %v", err)
	}
}

func TestAcquireQueryError(t *testing.T) {
	conn := &mockConn{queryErr: errors.New("connection refused")}
	locker := New()
	err := locker.Acquire(context.Background(), conn, time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAcquireResultNegative(t *testing.T) {
	conn := &mockConn{rows: newMockRows([][]any{{-2}})}
	locker := New()
	err := locker.Acquire(context.Background(), conn, time.Second)
	if !errors.Is(err, rmerrors.ErrLockTimeout) {
		t.Errorf("expected ErrLockTimeout for result=-2, got %v", err)
	}
}

func TestReleaseSuccess(t *testing.T) {
	conn := &mockConn{rows: newMockRows([][]any{{0}})}
	locker := New()
	err := locker.Release(context.Background(), conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseQueryError(t *testing.T) {
	conn := &mockConn{queryErr: errors.New("connection lost")}
	locker := New()
	err := locker.Release(context.Background(), conn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLockerImplementsInterface(t *testing.T) {
	var _ Locker = New()
}
