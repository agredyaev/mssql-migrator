package lock

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type appLockConnector struct {
	result int64
	err    error
}

type appLockConn struct {
	result int64
	err    error
}

type appLockRows struct {
	result int64
	read   bool
}

type appLockDriver struct{}

func TestMain(m *testing.M) {
	sql.Register("applock-test-driver", appLockDriver{})
	m.Run()
}

func (appLockDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

func (c appLockConnector) Connect(context.Context) (driver.Conn, error) {
	return &appLockConn{result: c.result, err: c.err}, nil
}

func (c appLockConnector) Driver() driver.Driver { return appLockDriver{} }

func (c *appLockConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c *appLockConn) Close() error                        { return nil }
func (c *appLockConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (c *appLockConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &appLockRows{result: c.result}, nil
}

func (r *appLockRows) Columns() []string { return []string{"result"} }
func (r *appLockRows) Close() error      { return nil }

func (r *appLockRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = r.result
	return nil
}

func openAppLockConn(t *testing.T, result int64, err error) *sql.Conn {
	t.Helper()
	db := sql.OpenDB(appLockConnector{result: result, err: err})
	t.Cleanup(func() { _ = db.Close() })
	conn, openErr := db.Conn(context.Background())
	if openErr != nil {
		t.Fatalf("db.Conn() error = %v", openErr)
	}
	return conn
}

func TestAcquireTreatsCanceledResultAsTimeout(t *testing.T) {
	conn := openAppLockConn(t, -2, nil)
	defer conn.Close()
	err := Acquire(context.Background(), conn, time.Second)
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestAcquireTreatsDeadlockResultAsFailure(t *testing.T) {
	conn := openAppLockConn(t, -3, nil)
	defer conn.Close()
	err := Acquire(context.Background(), conn, time.Second)
	if err == nil || !errors.Is(err, ErrFailed) {
		t.Fatalf("expected failure error, got %v", err)
	}
}

func TestAcquireTreatsContextCanceledAsTimeout(t *testing.T) {
	conn := openAppLockConn(t, 0, context.Canceled)
	defer conn.Close()
	err := Acquire(context.Background(), conn, time.Second)
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "context canceled") {
		t.Fatalf("expected original context error, got %q", got)
	}
}
