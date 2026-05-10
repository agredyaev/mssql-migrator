package migrator

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

type sessionTestDriver struct{}

type sessionTestConnector struct {
	state *sessionTestState
}

type sessionTestConn struct {
	state *sessionTestState
}

type sessionTestRows struct {
	result int64
	read   bool
}

type sessionTestState struct {
	mu      sync.Mutex
	lockErr error
	closes  int
}

type sessionTestOpener struct {
	db  *sql.DB
	err error
}

func init() {
	sql.Register("migrator-session-test-driver", sessionTestDriver{})
}

func (sessionTestDriver) Open(string) (driver.Conn, error) {
	return nil, fmt.Errorf("use OpenDB with connector")
}

func (c sessionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &sessionTestConn{state: c.state}, nil
}

func (c sessionTestConnector) Driver() driver.Driver { return sessionTestDriver{} }

func (c *sessionTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}

func (c *sessionTestConn) Close() error {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.closes++
	return nil
}

func (c *sessionTestConn) Begin() (driver.Tx, error) { return nil, errors.New("not supported") }

func (c *sessionTestConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.state.lockErr != nil {
		return nil, c.state.lockErr
	}
	return &sessionTestRows{result: 0}, nil
}

func (r *sessionTestRows) Columns() []string { return []string{"result"} }

func (r *sessionTestRows) Close() error { return nil }

func (r *sessionTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = r.result
	return nil
}

func (o sessionTestOpener) Open(context.Context, config.Config) (*sql.DB, error) {
	if o.err != nil {
		return nil, o.err
	}
	return o.db, nil
}

func openSessionTestDB(t *testing.T, lockErr error) (*sql.DB, *sessionTestState) {
	t.Helper()
	state := &sessionTestState{lockErr: lockErr}
	db := sql.OpenDB(sessionTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	return db, state
}

func TestStartProtectedSessionClosesConnectionOnLockFailure(t *testing.T) {
	db, state := openSessionTestDB(t, errors.New("lock failed"))
	runner := NewRunnerWithDBOpener(config.Config{LockTimeout: time.Second}, logger.New(logger.Options{}), sessionTestOpener{db: db})

	session, err := runner.startProtectedSession(context.Background())
	if err == nil {
		t.Fatal("expected lock failure")
	}
	if !errors.Is(err, contracts.ErrCriticalState) {
		t.Fatalf("expected critical state wrapper, got %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil fallback session")
	}
	if session.conn != nil {
		t.Fatalf("expected fallback session without connection, got %#v", session)
	}

	state.mu.Lock()
	closes := state.closes
	state.mu.Unlock()
	if closes != 1 {
		t.Fatalf("expected closeFn to close connection once, got %d", closes)
	}
}

func TestStartProtectedSessionReturnsFallbackSessionOnOpenFailure(t *testing.T) {
	runner := NewRunnerWithDBOpener(config.Config{}, logger.New(logger.Options{}), sessionTestOpener{err: errors.New("open failed")})

	session, err := runner.startProtectedSession(context.Background())
	if err == nil {
		t.Fatal("expected open failure")
	}
	if !errors.Is(err, contracts.ErrConnection) {
		t.Fatalf("expected connection wrapper, got %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil fallback session")
	}
	if session.conn != nil {
		t.Fatalf("expected fallback session without connection, got %#v", session)
	}
	if session.report.Tool != toolName {
		t.Fatalf("expected initialized fallback report, got %#v", session.report)
	}
}
