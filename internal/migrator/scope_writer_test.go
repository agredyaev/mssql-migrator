package migrator

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
	"reporting-db-migrations/internal/validate"
)

type scopeWriterTestDriver struct{}

type scopeWriterTestConnector struct {
	state *scopeWriterTestState
}

type scopeWriterTestConn struct {
	state *scopeWriterTestState
}

type scopeWriterTestTx struct {
	state *scopeWriterTestState
}

type scopeWriterTestRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type scopeWriterTestState struct {
	mu      sync.Mutex
	execs   []string
	queries []string
	args    [][]driver.NamedValue
	rows    [][]driver.Value
}

func init() {
	sql.Register("scope-writer-test-driver", scopeWriterTestDriver{})
}

func TestScopeWriterMigrationRequiresConnection(t *testing.T) {
	s := scopeWriter{writer: newMetadataWriter(config.Config{}, &stubExecer{result: stubResult{rows: 1}}, "run-1")}
	_, err := s.Migration(t.Context(), contracts.MigrationPlan{})
	if err == nil || !strings.Contains(err.Error(), "persist migration scope: missing metadata connection") {
		t.Fatalf("expected missing connection error, got %v", err)
	}
}

func TestScopeWriterValidationRequiresConnection(t *testing.T) {
	s := scopeWriter{writer: newMetadataWriter(config.Config{}, &stubExecer{result: stubResult{rows: 1}}, "run-1")}
	_, err := s.Validation(t.Context(), parser.Layout{}, validate.CatalogState{}, nil)
	if err == nil || !strings.Contains(err.Error(), "persist validation scope: missing metadata connection") {
		t.Fatalf("expected missing connection error, got %v", err)
	}
}

func TestScopeWriterRepairRequiresConnection(t *testing.T) {
	s := scopeWriter{writer: newMetadataWriter(config.Config{}, &stubExecer{result: stubResult{rows: 1}}, "run-1")}
	_, err := s.Repair(t.Context(), parser.Object{}, contracts.PlannedObject{}, planner.CatalogState{}, "")
	if err == nil || !strings.Contains(err.Error(), "persist repair scope: missing metadata connection") {
		t.Fatalf("expected missing connection error, got %v", err)
	}
}

func TestScopeWriterMigrationBatchesItemInserts(t *testing.T) {
	db, state := openScopeWriterTestDB(t, [][]driver.Value{{"reporting/views/monthly", int64(41)}, {"reporting/procedures/refresh", int64(42)}})
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db.Conn() error = %v", err)
	}
	defer conn.Close()

	s := scopeWriter{writer: newMetadataWriter(config.Config{}, conn, "run-1"), conn: conn}
	itemIDs, err := s.Migration(t.Context(), contracts.MigrationPlan{
		Schemas: []contracts.PlannedSchema{{SchemaName: "reporting", Action: contracts.SchemaActionExists, Exists: true}},
		Objects: []contracts.PlannedObject{
			{ObjectPath: "reporting/views/monthly.sql", SchemaName: "reporting", Kind: "views", ObjectName: "monthly", NormalizedKey: "reporting/views/monthly", PlannedAction: contracts.ActionSkipUnchanged},
			{ObjectPath: "reporting/procedures/refresh.sql", SchemaName: "reporting", Kind: "procedures", ObjectName: "refresh", NormalizedKey: "reporting/procedures/refresh", PlannedAction: contracts.ActionCreateObject, Checksum: "abc"},
		},
	})
	if err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if itemIDs["reporting/views/monthly"] != 41 || itemIDs["reporting/procedures/refresh"] != 42 {
		t.Fatalf("unexpected item ids: %#v", itemIDs)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	insertCount := 0
	for _, exec := range state.execs {
		if strings.Contains(exec, "INSERT INTO __migrator.items") {
			insertCount++
		}
	}
	if insertCount != 2 {
		t.Fatalf("expected two batched item inserts, got %d in %#v", insertCount, state.execs)
	}
	if len(state.queries) != 1 || !strings.Contains(state.queries[0], "SELECT normalized_key, item_id") {
		t.Fatalf("expected one id lookup query, got %#v", state.queries)
	}
}

func openScopeWriterTestDB(t *testing.T, rows [][]driver.Value) (*sql.DB, *scopeWriterTestState) {
	t.Helper()
	state := &scopeWriterTestState{rows: rows}
	db := sql.OpenDB(scopeWriterTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	return db, state
}

func (scopeWriterTestDriver) Open(string) (driver.Conn, error) {
	return nil, fmt.Errorf("use OpenDB with connector")
}

func (c scopeWriterTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &scopeWriterTestConn{state: c.state}, nil
}

func (c scopeWriterTestConnector) Driver() driver.Driver { return scopeWriterTestDriver{} }

func (c *scopeWriterTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not supported")
}

func (c *scopeWriterTestConn) Close() error { return nil }

func (c *scopeWriterTestConn) Begin() (driver.Tx, error) {
	return &scopeWriterTestTx{state: c.state}, nil
}

func (c *scopeWriterTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &scopeWriterTestTx{state: c.state}, nil
}

func (c *scopeWriterTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.execs = append(c.state.execs, query)
	c.state.args = append(c.state.args, cloneScopeWriterNamedValues(args))
	return driver.RowsAffected(1), nil
}

func (c *scopeWriterTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.queries = append(c.state.queries, query)
	c.state.args = append(c.state.args, cloneScopeWriterNamedValues(args))
	return &scopeWriterTestRows{columns: []string{"normalized_key", "item_id"}, rows: c.state.rows}, nil
}

func (tx *scopeWriterTestTx) Commit() error { return nil }

func (tx *scopeWriterTestTx) Rollback() error { return nil }

func (r *scopeWriterTestRows) Columns() []string { return r.columns }

func (r *scopeWriterTestRows) Close() error { return nil }

func (r *scopeWriterTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.index]
	r.index++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		} else {
			dest[i] = nil
		}
	}
	return nil
}

func cloneScopeWriterNamedValues(values []driver.NamedValue) []driver.NamedValue {
	if len(values) == 0 {
		return nil
	}
	result := make([]driver.NamedValue, len(values))
	copy(result, values)
	return result
}
