package catalog

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

func TestReadRejectsNilConnection(t *testing.T) {
	_, err := Read(context.Background(), nil)
	if err == nil {
		t.Fatal("expected nil connection error")
	}
	if !errors.Is(err, contracts.ErrCriticalState) {
		t.Fatalf("expected critical state sentinel, got %v", err)
	}
	if err.Error() == contracts.ErrCriticalState.Error() {
		t.Fatalf("expected descriptive wrapped error, got %v", err)
	}
}

func TestStateQueryForSchemasRepeatsArgsPerUnionBranch(t *testing.T) {
	query, args := stateQueryForSchemas([]string{"reporting", "sales", "reporting"})
	if got, want := len(args), 6; got != want {
		t.Fatalf("stateQueryForSchemas() args=%d, want %d", got, want)
	}
	for _, expected := range []string{
		"s.name IN (@p1, @p2)",
		"s.name IN (@p3, @p4)",
		"s.name IN (@p5, @p6)",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected query to contain %q, got %s", expected, query)
		}
	}
	for i, expected := range []any{"reporting", "sales", "reporting", "sales", "reporting", "sales"} {
		if args[i] != expected {
			t.Fatalf("args[%d]=%v, want %v", i, args[i], expected)
		}
	}
}

func TestNormalizedSchemaFilterDeduplicatesNames(t *testing.T) {
	got := normalizedSchemaFilter([]string{"Reporting", " reporting ", "Sales", "sales", ""})
	if want := []string{"Reporting", "Sales"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("normalizedSchemaFilter()=%#v, want %#v", got, want)
	}
}

func TestReadSchemaObjectsForSchemasSkipsTableColumns(t *testing.T) {
	db, state := openCatalogTestDB(t, false)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	got, err := ReadSchemaObjectsForSchemas(context.Background(), conn, []string{"reporting"})
	if err != nil {
		t.Fatalf("ReadSchemaObjectsForSchemas() error = %v", err)
	}
	if _, ok := got.Schemas["reporting"]; !ok {
		t.Fatalf("expected schema in state, got %#v", got.Schemas)
	}
	if len(got.Objects) != 1 {
		t.Fatalf("expected one object, got %#v", got.Objects)
	}
	state.mu.Lock()
	columnsCalled := state.columnsCalled
	queries := append([]string(nil), state.queries...)
	state.mu.Unlock()
	if columnsCalled {
		t.Fatal("expected table columns query to be skipped")
	}
	if len(queries) != 2 {
		t.Fatalf("expected two catalog queries, got %#v", queries)
	}
}

func TestReadColumnsForTablesReturnsRequestedColumns(t *testing.T) {
	db, _ := openCatalogTestDB(t, true)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	got, err := ReadColumnsForTables(context.Background(), conn, []TableRef{{SchemaName: "reporting", TableName: "snapshot"}})
	if err != nil {
		t.Fatalf("ReadColumnsForTables() error = %v", err)
	}
	cols := got["reporting/tables/snapshot"]
	if len(cols) != 2 {
		t.Fatalf("expected two columns, got %#v", cols)
	}
	if cols[1].Length != 100 {
		t.Fatalf("expected normalized nvarchar length, got %#v", cols[1])
	}
}

func TestNormalizedTableRefsDeduplicatesNames(t *testing.T) {
	got := normalizedTableRefs([]TableRef{{SchemaName: "Reporting", TableName: "Snapshot"}, {SchemaName: " reporting ", TableName: "snapshot"}, {SchemaName: "Sales", TableName: "Daily"}, {SchemaName: "", TableName: "skip"}})
	if len(got) != 2 {
		t.Fatalf("normalizedTableRefs()=%#v", got)
	}
	if got[0].SchemaName != "Reporting" || got[0].TableName != "Snapshot" {
		t.Fatalf("unexpected first table ref: %#v", got[0])
	}
	if got[1].SchemaName != "Sales" || got[1].TableName != "Daily" {
		t.Fatalf("unexpected second table ref: %#v", got[1])
	}
}

func TestNormalizedObjectRefsDeduplicatesNames(t *testing.T) {
	got := normalizedObjectRefs([]ObjectRef{{SchemaName: "Reporting", Kind: "views", ObjectName: "Monthly"}, {SchemaName: " reporting ", Kind: "views", ObjectName: "monthly"}, {SchemaName: "Sales", Kind: "indexes", ParentName: "Daily", ObjectName: "ix_daily"}, {SchemaName: "", Kind: "views", ObjectName: "skip"}})
	if len(got) != 2 {
		t.Fatalf("normalizedObjectRefs()=%#v", got)
	}
	if got[0].SchemaName != "Reporting" || got[0].Kind != "views" || got[0].ObjectName != "Monthly" {
		t.Fatalf("unexpected first object ref: %#v", got[0])
	}
	if got[1].SchemaName != "Sales" || got[1].Kind != "indexes" || got[1].ParentName != "Daily" || got[1].ObjectName != "ix_daily" {
		t.Fatalf("unexpected second object ref: %#v", got[1])
	}
}

func TestStateQueryForObjectsBuildsPerObjectClauses(t *testing.T) {
	query, args := stateQueryForObjects([]ObjectRef{{SchemaName: "reporting", Kind: "views", ObjectName: "monthly"}, {SchemaName: "reporting", Kind: "indexes", ParentName: "snapshot", ObjectName: "ix_snapshot"}})
	for _, expected := range []string{
		"(s.name = @p1 AND UPPER(o.type_desc) = @p2 AND ISNULL(parent.name, '') = @p3 AND o.name = @p4)",
		"(s.name = @p5 AND UPPER(o.type_desc) = @p6 AND ISNULL(parent.name, '') = @p7 AND o.name = @p8)",
		"(s.name = @p9 AND UPPER('USER_TABLE_TYPE') = @p10 AND ISNULL('', '') = @p11 AND tt.name = @p12)",
		"(s.name = @p17 AND UPPER('INDEX') = @p18 AND ISNULL(o.name, '') = @p19 AND i.name = @p20)",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected object query to contain %q, got %s", expected, query)
		}
	}
	if got, want := len(args), 24; got != want {
		t.Fatalf("stateQueryForObjects() args=%d, want %d", got, want)
	}
	for i, expected := range []any{"reporting", "VIEW", "", "monthly", "reporting", "INDEX", "snapshot", "ix_snapshot", "reporting", "VIEW", "", "monthly", "reporting", "INDEX", "snapshot", "ix_snapshot", "reporting", "VIEW", "", "monthly", "reporting", "INDEX", "snapshot", "ix_snapshot"} {
		if args[i] != expected {
			t.Fatalf("args[%d]=%v, want %v", i, args[i], expected)
		}
	}
}

func TestReadForLayoutUsesObjectQuery(t *testing.T) {
	db, state := openCatalogTestDB(t, false)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	got, err := ReadForLayout(context.Background(), conn, []string{"reporting"}, []ObjectRef{{SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}})
	if err != nil {
		t.Fatalf("ReadForLayout() error = %v", err)
	}
	if len(got.Objects) != 1 {
		t.Fatalf("expected one object, got %#v", got.Objects)
	}
	state.mu.Lock()
	queries := append([]string(nil), state.queries...)
	state.mu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("expected two catalog queries, got %#v", queries)
	}
	if !strings.Contains(queries[1], "UPPER(o.type_desc)") || !strings.Contains(queries[1], "o.name = @p4") {
		t.Fatalf("expected narrowed object query, got %s", queries[1])
	}
}

type catalogTestDriver struct{}

type catalogTestConnector struct{ state *catalogTestState }

type catalogTestConn struct{ state *catalogTestState }

type catalogTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

type catalogTestState struct {
	mu            sync.Mutex
	queries       []string
	columnsCalled bool
	columnsMode   bool
}

func init() {
	sql.Register("catalog-test-driver", catalogTestDriver{})
}

func (catalogTestDriver) Open(string) (driver.Conn, error) {
	return nil, fmt.Errorf("use OpenDB with connector")
}

func (c catalogTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &catalogTestConn{state: c.state}, nil
}

func (c catalogTestConnector) Driver() driver.Driver { return catalogTestDriver{} }

func (c *catalogTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not supported")
}

func (c *catalogTestConn) Close() error { return nil }

func (c *catalogTestConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("not supported") }

func (c *catalogTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	c.state.queries = append(c.state.queries, query)
	columnsMode := c.state.columnsMode
	if strings.Contains(query, "sys.columns") {
		c.state.columnsCalled = true
		c.state.mu.Unlock()
		if !columnsMode {
			return nil, fmt.Errorf("unexpected columns query")
		}
		return &catalogTestRows{
			columns: []string{"schema_name", "table_name", "column_name", "type_name", "max_length", "precision", "scale", "nullable"},
			values:  [][]driver.Value{{"reporting", "snapshot", "id", "int", int64(4), int64(10), int64(0), false}, {"reporting", "snapshot", "name", "nvarchar", int64(200), int64(0), int64(0), true}},
		}, nil
	}
	c.state.mu.Unlock()
	if strings.Contains(query, "SELECT name FROM sys.schemas") {
		return &catalogTestRows{columns: []string{"name"}, values: [][]driver.Value{{"reporting"}}}, nil
	}
	if strings.Contains(query, "sys.objects") {
		return &catalogTestRows{columns: []string{"schema_name", "type_desc", "object_name", "parent_name"}, values: [][]driver.Value{{"reporting", "USER_TABLE", "snapshot", ""}}}, nil
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (r *catalogTestRows) Columns() []string { return r.columns }

func (r *catalogTestRows) Close() error { return nil }

func (r *catalogTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func openCatalogTestDB(t *testing.T, columnsMode bool) (*sql.DB, *catalogTestState) {
	t.Helper()
	state := &catalogTestState{columnsMode: columnsMode}
	db := sql.OpenDB(catalogTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	return db, state
}
