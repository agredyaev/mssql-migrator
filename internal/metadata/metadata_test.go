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

	"reporting-db-migrations/internal/contracts"
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
	if !containsExec(state.execs, "CREATE TABLE __migrator.runs") {
		t.Fatalf("expected bootstrap to create runs table, got %#v", state.execs)
	}
	if !containsExec(state.execs, "CREATE TABLE __migrator.items") {
		t.Fatalf("expected bootstrap to create items table, got %#v", state.execs)
	}
	if !containsExec(state.execs, "CREATE TABLE __migrator.attempts") {
		t.Fatalf("expected bootstrap to create attempts table, got %#v", state.execs)
	}
	if containsExec(state.execs, "CREATE VIEW __migrator.v_migration_state") {
		t.Fatalf("expected bootstrap not to create legacy view, got %#v", state.execs)
	}
}

func TestBootstrapTableStatementsContainRunsItemsAttempts(t *testing.T) {
	statements := bootstrapTableStatements()
	for _, expected := range []string{"CREATE TABLE __migrator.runs", "CREATE TABLE __migrator.items", "CREATE TABLE __migrator.attempts"} {
		if !containsExec(statements, expected) {
			t.Fatalf("expected table bootstrap to contain %q, got %#v", expected, statements)
		}
	}
}

func TestBootstrapConstraintStatementsContainAllowedActions(t *testing.T) {
	statements := strings.Join(bootstrapCheckConstraintStatements(), "\n")
	for _, expected := range []string{"CK_runs_command", "CK_items_action", "CK_attempts_action", contracts.ActionRepairChecksum, contracts.ActionValidateSkipped} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("expected constraint bootstrap to contain %q, got %s", expected, statements)
		}
	}
}

func TestBootstrapIndexStatementsContainExpectedIndexes(t *testing.T) {
	statements := strings.Join(bootstrapIndexStatements(), "\n")
	for _, expected := range []string{"IX_runs_base", "UX_items_run_key", "IX_attempts_item"} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("expected index bootstrap to contain %q, got %s", expected, statements)
		}
	}
}

func TestBootstrapCurrentMetadataDoesNotRewriteObjects(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		objectExists: map[string]bool{
			keyForArgs("__migrator.schema_version", "U"): true,
			keyForArgs("__migrator.runs", "U"):           true,
			keyForArgs("__migrator.items", "U"):          true,
			keyForArgs("__migrator.attempts", "U"):       true,
		},
	})
	defer conn.Close()

	if err := Bootstrap(context.Background(), conn); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	state := metadataTestStateForConn(t, conn)
	if got := len(state.execs); got != 0 {
		t.Fatalf("Bootstrap() executed %d statements for current metadata, want 0", got)
	}
}

func TestBootstrapRejectsLegacyMetadataShape(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		objectExists: map[string]bool{
			keyForArgs("__migrator.migration_runs", "U"): true,
		},
	})
	defer conn.Close()

	err := Bootstrap(context.Background(), conn)
	if err == nil {
		t.Fatal("Bootstrap() error = nil, want incompatible metadata error")
	}
	if !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("Bootstrap() error = %v, want ErrSchemaIncompatible", err)
	}
	if got := err.Error(); !strings.Contains(got, "__migrator.migration_runs") {
		t.Fatalf("Bootstrap() error = %q, want legacy object name", got)
	}
	state := metadataTestStateForConn(t, conn)
	if got := len(state.execs); got != 0 {
		t.Fatalf("Bootstrap() executed %d statements for legacy metadata, want 0", got)
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
	conn := openMetadataTestConn(t, &metadataTestScenario{execErrAt: map[int]error{0: errors.New("column name 'run_id' specified more than once")}})
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

func TestBootstrapFailsWhenSchemaVersionIsNewer(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		objectExists: map[string]bool{
			keyForArgs("__migrator.schema_version", "U"): true,
		},
		queryResponses: map[string]metadataTestRows{
			schemaVersionQuery: {
				columns: []string{""},
				rows:    [][]driver.Value{{int64(metadataSchemaVersion + 1)}},
			},
		},
	})
	defer conn.Close()

	err := Bootstrap(context.Background(), conn)
	if err == nil {
		t.Fatal("expected schema version failure")
	}
	if !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("expected ErrSchemaIncompatible, got %v", err)
	}
	state := metadataTestStateForConn(t, conn)
	if got := len(state.execs); got != 0 {
		t.Fatalf("expected no bootstrap DDL before version failure, got %d statements", got)
	}
}

func TestFinishRunRejectsZeroRowsAffected(t *testing.T) {
	err := FinishRun(context.Background(), stubMetadataExecer{result: stubMetadataResult{rows: 0}}, "run-1", true, "", "")
	if err == nil || !strings.Contains(err.Error(), "expected 1 row affected") {
		t.Fatalf("expected rows affected failure, got %v", err)
	}
}

func TestUpdateItemResultRejectsZeroRowsAffected(t *testing.T) {
	err := UpdateItemResult(context.Background(), stubMetadataExecer{result: stubMetadataResult{rows: 0}}, "run-1", "reporting", true, "")
	if err == nil || !strings.Contains(err.Error(), "expected 1 row affected") {
		t.Fatalf("expected rows affected failure, got %v", err)
	}
}

func TestUpdateItemResultRejectsMultipleRowsAffected(t *testing.T) {
	err := UpdateItemResult(context.Background(), stubMetadataExecer{result: stubMetadataResult{rows: 2}}, "run-1", "reporting/views/monthly", true, "")
	if err == nil || !strings.Contains(err.Error(), "expected 1 row affected") {
		t.Fatalf("expected rows affected failure, got %v", err)
	}
}

func TestUpdateItemResultUsesExpectedPlaceholders(t *testing.T) {
	execer := captureMetadataExecer{result: stubMetadataResult{rows: 1}}
	if err := UpdateItemResult(context.Background(), &execer, "run-1", "reporting/views/monthly", true, "boom"); err != nil {
		t.Fatalf("UpdateItemResult() error = %v", err)
	}
	if !strings.Contains(execer.query, "success = @p3") || !strings.Contains(execer.query, "error_message = @p4") {
		t.Fatalf("unexpected update query: %s", execer.query)
	}
}

func TestLoadSuccessfulChecksumsIfPresentWithoutMetadataTables(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{})
	defer conn.Close()

	got, err := LoadSuccessfulChecksumsIfPresent(context.Background(), conn)
	if err != nil {
		t.Fatalf("LoadSuccessfulChecksumsIfPresent() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadSuccessfulChecksumsIfPresent() = %#v, want empty map", got)
	}
}

func TestLoadSuccessfulChecksumsIfPresentUsesItemKeys(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		objectExists: map[string]bool{
			keyForArgs("__migrator.schema_version", "U"): true,
			keyForArgs("__migrator.runs", "U"):           true,
			keyForArgs("__migrator.items", "U"):          true,
			keyForArgs("__migrator.attempts", "U"):       true,
		},
		queryResponses: map[string]metadataTestRows{
			successfulChecksumsQuery(): {
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
		t.Fatalf("item checksum missing, got %#v", got)
	}
	if got["reporting/procedures/refreshmonthly"] != "def" {
		t.Fatalf("fallback normalized key missing, got %#v", got)
	}
}

func TestLoadSuccessfulChecksumsSkipsShapeInspection(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		queryResponses: map[string]metadataTestRows{
			successfulChecksumsQuery(): {
				columns: []string{"normalized_key", "script_name", "checksum"},
				rows:    [][]driver.Value{{"reporting/views/monthly", "ignored", "abc"}},
			},
		},
	})
	defer conn.Close()

	got, err := LoadSuccessfulChecksums(context.Background(), conn)
	if err != nil {
		t.Fatalf("LoadSuccessfulChecksums() error = %v", err)
	}
	if got["reporting/views/monthly"] != "abc" {
		t.Fatalf("LoadSuccessfulChecksums() = %#v", got)
	}
	state := metadataTestStateForConn(t, conn)
	if len(state.queries) != 1 || state.queries[0] != successfulChecksumsQuery() {
		t.Fatalf("expected direct checksum query, got %#v", state.queries)
	}
}

func TestSuccessfulChecksumsQueryUsesLatestRowPerObject(t *testing.T) {
	query := successfulChecksumsQuery()
	for _, expected := range []string{"ROW_NUMBER() OVER", "PARTITION BY", contracts.ActionSkipUnchanged, contracts.ActionRepairChecksum} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected query to contain %q, got %s", expected, query)
		}
	}
}

func TestLoadSuccessfulChecksumsIfPresentRejectsLegacyMetadataShape(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		objectExists: map[string]bool{
			keyForArgs("__migrator.migration_attempts", "U"): true,
		},
	})
	defer conn.Close()

	_, err := LoadSuccessfulChecksumsIfPresent(context.Background(), conn)
	if err == nil {
		t.Fatal("expected incompatible metadata error")
	}
	if !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("expected ErrSchemaIncompatible, got %v", err)
	}
}

func TestLoadSuccessfulChecksumsIfPresentRejectsPartialCurrentShape(t *testing.T) {
	conn := openMetadataTestConn(t, &metadataTestScenario{
		objectExists: map[string]bool{
			keyForArgs("__migrator.schema_version", "U"): true,
			keyForArgs("__migrator.attempts", "U"):       true,
		},
	})
	defer conn.Close()

	_, err := LoadSuccessfulChecksumsIfPresent(context.Background(), conn)
	if err == nil {
		t.Fatal("expected incompatible metadata error")
	}
	if !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("expected ErrSchemaIncompatible, got %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "__migrator.runs") || !strings.Contains(got, "__migrator.items") {
		t.Fatalf("expected missing object names, got %q", got)
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
	schemaVersionQuery = `SELECT MAX(version) FROM __migrator.schema_version`
)

type metadataTestScenario struct {
	execErrAt      map[int]error
	objectExists   map[string]bool
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
	if query == metadataShapeQuery() {
		if response, ok := c.state.scenario.queryResponses[query]; ok {
			rows := response.rows
			if len(response.rowsByArgs) > 0 {
				rows = response.rowsByArgs[keyForNamedValues(args)]
			}
			return &metadataTestRowsResult{columns: response.columns, rows: rows}, nil
		}
		rows := [][]driver.Value{}
		for key, exists := range c.state.scenario.objectExists {
			if !exists {
				continue
			}
			schemaName, objectName, objectType := metadataShapeRow(key)
			rows = append(rows, []driver.Value{schemaName, objectName, objectType})
		}
		return &metadataTestRowsResult{columns: []string{"schema_name", "object_name", "type"}, rows: rows}, nil
	}
	response, ok := c.state.scenario.queryResponses[query]
	if !ok {
		if query == schemaVersionQuery {
			return &metadataTestRowsResult{columns: []string{""}, rows: [][]driver.Value{{int64(metadataSchemaVersion)}}}, nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	rows := response.rows
	if len(response.rowsByArgs) > 0 {
		rows = response.rowsByArgs[keyForNamedValues(args)]
	}
	return &metadataTestRowsResult{columns: response.columns, rows: rows}, nil
}

func metadataShapeRow(key string) (string, string, string) {
	parts := strings.Split(key, "|")
	if len(parts) != 2 {
		return "", "", ""
	}
	schemaName, objectName := splitQualifiedObjectName(parts[0])
	return schemaName, objectName, parts[1]
}

type stubMetadataExecer struct {
	result sql.Result
	err    error
}

func (s stubMetadataExecer) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

type captureMetadataExecer struct {
	query  string
	args   []any
	result sql.Result
}

func (s *captureMetadataExecer) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	s.query = query
	s.args = append([]any(nil), args...)
	return s.result, nil
}

type stubMetadataResult struct {
	rows int64
}

func (s stubMetadataResult) LastInsertId() (int64, error) { return 0, nil }

func (s stubMetadataResult) RowsAffected() (int64, error) { return s.rows, nil }

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

func containsExec(values []string, part string) bool {
	for _, value := range values {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}
