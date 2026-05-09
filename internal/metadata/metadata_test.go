package metadata

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestBootstrapExecutesAllStatements(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{})
	defer conn.Close()

	if err := Bootstrap(context.Background(), conn); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	state := metadataTestStateForConn(t, conn)
	if got, want := len(state.execs), len(bootstrapStatements()); got != want {
		t.Fatalf("Bootstrap() executed %d statements, want %d", got, want)
	}
	if !strings.Contains(state.execs[0], "CREATE SCHEMA __migrator") {
		t.Fatalf("first bootstrap statement = %q", state.execs[0])
	}
	if !strings.Contains(state.execs[len(state.execs)-1], "CREATE OR ALTER VIEW __migrator.v_migration_state") {
		t.Fatalf("last bootstrap statement = %q", state.execs[len(state.execs)-1])
	}
}

func TestBootstrapCompatibleExistingMetadataShapeRemainsIdempotent(t *testing.T) {
	statements := bootstrapStatements()
	conn := openMetadataTestConn(t, &metadataTestScenario{
		execErrAt: map[int]error{},
	})
	defer conn.Close()

	for range 2 {
		if err := Bootstrap(context.Background(), conn); err != nil {
			t.Fatalf("Bootstrap() error = %v", err)
		}
	}

	state := metadataTestStateForConn(t, conn)
	if got, want := len(state.execs), len(statements)*2; got != want {
		t.Fatalf("Bootstrap() executed %d statements, want %d after repeated run", got, want)
	}
	if state.execs[len(statements)] != statements[0] {
		t.Fatalf("second bootstrap run did not restart statement sequence")
	}
}

func TestBootstrapClassifiesMissingDDLPermission(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{execErrAt: map[int]error{0: errors.New("CREATE TABLE permission denied in database")}})
	defer conn.Close()

	err := Bootstrap(context.Background(), conn)
	if err == nil {
		t.Fatal("Bootstrap() error = nil, want permission error")
	}
	if !errors.Is(err, ErrMissingDDLPermission) {
		t.Fatalf("Bootstrap() error = %v, want ErrMissingDDLPermission", err)
	}
	if got := err.Error(); !strings.Contains(got, "permission denied") {
		t.Fatalf("Bootstrap() error = %q, want original cause", got)
	}
}

func TestBootstrapClassifiesIncompatibleMetadataShape(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{execErrAt: map[int]error{3: errors.New("column name 'run_id' specified more than once")}})
	defer conn.Close()

	err := Bootstrap(context.Background(), conn)
	if err == nil {
		t.Fatal("Bootstrap() error = nil, want incompatible metadata error")
	}
	if !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("Bootstrap() error = %v, want ErrSchemaIncompatible", err)
	}
	if got := err.Error(); !strings.Contains(got, "run_id") {
		t.Fatalf("Bootstrap() error = %q, want original cause", got)
	}
}

func TestLoadSuccessfulChecksumsIfPresentWithoutMetadataTable(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		queryResponses: map[string]metadataTestRows{
			objectExistsQuery: {
				columns: []string{"exists"},
				rowsByArgs: map[string][][]driver.Value{
					keyForArgs("__migrator.schema_migrations", "U"): {{int64(0)}},
				},
			},
		},
	})
	defer conn.Close()

	got, err := LoadSuccessfulChecksumsIfPresent(context.Background(), conn)
	if err != nil {
		t.Fatalf("LoadSuccessfulChecksumsIfPresent() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadSuccessfulChecksumsIfPresent() = %#v, want empty map", got)
	}
}

func TestLoadSuccessfulChecksumsIfPresentUsesTrackedObjectKeys(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		queryResponses: map[string]metadataTestRows{
			objectExistsQuery: {
				columns: []string{"exists"},
				rowsByArgs: map[string][][]driver.Value{
					keyForArgs("__migrator.schema_migrations", "U"): {{int64(1)}},
					keyForArgs("__migrator.tracked_objects", "U"):   {{int64(1)}},
				},
			},
			trackedObjectChecksumsQuery: {
				columns: []string{"normalized_key", "script_name", "checksum"},
				rows: [][]driver.Value{
					{"reporting/views/monthly", "ignored", "abc"},
					{"", "Reporting/Procedures/RefreshMonthly.sql", "def"},
				},
			},
		},
	})
	defer conn.Close()

	got, err := LoadSuccessfulChecksumsIfPresent(context.Background(), conn)
	if err != nil {
		t.Fatalf("LoadSuccessfulChecksumsIfPresent() error = %v", err)
	}
	if got["reporting/views/monthly"] != "abc" {
		t.Fatalf("tracked object checksum missing, got %#v", got)
	}
	if got["reporting/procedures/refreshmonthly"] != "def" {
		t.Fatalf("fallback normalized key missing, got %#v", got)
	}
}

func TestClassifyBootstrapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "permission", err: errors.New("not authorized to create schema"), want: ErrMissingDDLPermission},
		{name: "denied", err: errors.New("permission denied"), want: ErrMissingDDLPermission},
		{name: "shape", err: errors.New("invalid column name"), want: ErrSchemaIncompatible},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyBootstrapError(tt.err)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("classifyBootstrapError() = %v, want nil", got)
				}
				return
			}
			if !errors.Is(got, tt.want) {
				t.Fatalf("classifyBootstrapError() = %v, want %v", got, tt.want)
			}
		})
	}
}

const (
	objectExistsQuery           = `SELECT CASE WHEN OBJECT_ID(@p1, @p2) IS NULL THEN 0 ELSE 1 END`
	trackedObjectChecksumsQuery = `
SELECT ISNULL(o.normalized_key, ''), m.script_name, m.checksum
FROM __migrator.schema_migrations m
LEFT JOIN __migrator.tracked_objects o ON o.tracked_object_id = m.tracked_object_id
WHERE m.success = 1
ORDER BY m.applied_at ASC, m.id ASC`
)

type metadataTestScenario struct {
	execErrAt      map[int]error
	queryResponses map[string]metadataTestRows
}

type metadataTestRows struct {
	columns    []string
	rows       [][]driver.Value
	rowsByArgs map[string][][]driver.Value
}

type metadataTestState struct {
	mu        sync.Mutex
	scenario  *metadataTestScenario
	execs     []string
	queries   []string
	queryArgs [][]driver.NamedValue
	closed    bool
}

type metadataTestDriver struct{}

type metadataTestConnector struct {
	state *metadataTestState
}

type metadataTestConn struct {
	state *metadataTestState
}

type metadataTestRowsResult struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func TestMain(m *testing.M) {
	sql.Register("metadata-test-driver", metadataTestDriver{})
	m.Run()
}

func (metadataTestDriver) Open(name string) (driver.Conn, error) {
	return nil, fmt.Errorf("use OpenDB with connector")
}

func (c metadataTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &metadataTestConn{state: c.state}, nil
}

func (c metadataTestConnector) Driver() driver.Driver {
	return metadataTestDriver{}
}

func (c *metadataTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("Prepare not supported")
}

func (c *metadataTestConn) Close() error {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.closed = true
	return nil
}

func (c *metadataTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("Begin not supported")
}

func (c *metadataTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	idx := len(c.state.execs)
	c.state.execs = append(c.state.execs, query)
	if err, ok := c.state.scenario.execErrAt[idx]; ok {
		return nil, err
	}
	return driver.RowsAffected(1), nil
}

func (c *metadataTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.queries = append(c.state.queries, query)
	c.state.queryArgs = append(c.state.queryArgs, cloneNamedValues(args))
	response, ok := c.state.scenario.queryResponses[query]
	if !ok {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	rows := response.rows
	if len(response.rowsByArgs) > 0 {
		rows = response.rowsByArgs[keyForNamedValues(args)]
	}
	return &metadataTestRowsResult{columns: response.columns, rows: rows}, nil
}

func (r *metadataTestRowsResult) Columns() []string {
	return r.columns
}

func (r *metadataTestRowsResult) Close() error {
	return nil
}

func (r *metadataTestRowsResult) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.index]
	r.index++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
			continue
		}
		dest[i] = nil
	}
	return nil
}

func openMetadataTestConn(t *testing.T, scenario *metadataTestScenario) *sql.Conn {
	t.Helper()
	state := &metadataTestState{scenario: scenario}
	db := sql.OpenDB(metadataTestConnector{state: state})
	t.Cleanup(func() {
		_ = db.Close()
	})
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db.Conn() error = %v", err)
	}
	return conn
}

func metadataTestStateForConn(t *testing.T, conn *sql.Conn) *metadataTestState {
	t.Helper()
	err := conn.Raw(func(driverConn any) error {
		testConn, ok := driverConn.(*metadataTestConn)
		if !ok {
			return fmt.Errorf("unexpected driver conn type %T", driverConn)
		}
		state := testConn.state
		if state == nil {
			return errors.New("missing test state")
		}
		state.mu.Lock()
		state.mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("conn.Raw() error = %v", err)
	}
	var result *metadataTestState
	err = conn.Raw(func(driverConn any) error {
		testConn := driverConn.(*metadataTestConn)
		result = testConn.state
		return nil
	})
	if err != nil {
		t.Fatalf("conn.Raw() error = %v", err)
	}
	return result
}

func cloneNamedValues(values []driver.NamedValue) []driver.NamedValue {
	if len(values) == 0 {
		return nil
	}
	result := make([]driver.NamedValue, len(values))
	copy(result, values)
	return result
}

func keyForNamedValues(values []driver.NamedValue) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%v", value.Value))
	}
	return strings.Join(parts, "|")
}

func keyForArgs(values ...any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%v", value))
	}
	return strings.Join(parts, "|")
}
