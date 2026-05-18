package audit

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/testutil"
	"reporting-db-migrations/internal/types"
)

func TestLoadChecksumsEmptyKeys(t *testing.T) {
	conn := &testutil.MockConn{}
	result, err := LoadChecksums(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
	if conn.QueryCount.Load() != 0 {
		t.Errorf("expected 0 queries for empty keys")
	}
}

func TestLoadChecksumsConnectionError(t *testing.T) {
	conn := &testutil.MockConn{QueryErr: errors.New("dead")}
	setOpenJSONSupport(conn, false)
	_, err := LoadChecksums(context.Background(), conn, []string{"k1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadChecksumsReturnsResults(t *testing.T) {
	var want [32]byte
	for i := range want {
		want[i] = byte(i + 1)
	}
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT": testutil.NewMockRows([][]any{{"k1", hex.EncodeToString(want[:])}}),
		},
	}
	setOpenJSONSupport(conn, false)
	result, err := LoadChecksums(context.Background(), conn, []string{"k1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result["k1"] != want {
		t.Errorf("checksum = %x, want %x", result["k1"], want)
	}
}

func TestLoadChecksumsChunking(t *testing.T) {
	keys := make([]string, 2500)
	for i := range keys {
		keys[i] = "k"
	}
	conn := &testutil.MockConn{}
	setOpenJSONSupport(conn, false)
	LoadChecksums(context.Background(), conn, keys)
	if conn.QueryCount.Load() < 2 {
		t.Errorf("expected at least 2 queries for 2500 keys, got %d", conn.QueryCount.Load())
	}
}

func TestLoadChecksumsUsesOpenJSONWhenSupported(t *testing.T) {
	conn := &mssqlChecksumQueryConn{checksumQueryConn: checksumQueryConn{
		openJSONRows: testutil.NewMockRows([][]any{{"k1", strings.Repeat("ab", 32)}}),
	}}
	result, err := LoadChecksums(context.Background(), conn, []string{"k1"})
	if err != nil {
		t.Fatalf("LoadChecksums: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if len(conn.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(conn.queries))
	}
	if !strings.Contains(conn.queries[0].query, "OPENJSON(@p1)") {
		t.Fatalf("expected OPENJSON query, got %q", conn.queries[0].query)
	}
	if got := conn.queries[0].args[0]; got != `["k1"]` {
		t.Fatalf("JSON arg = %q, want [\"k1\"]", got)
	}
}

func TestLoadChecksumsFallsBackAfterOpenJSONError(t *testing.T) {
	conn := &mssqlChecksumQueryConn{checksumQueryConn: checksumQueryConn{
		failOpenJSON: true,
		fallbackRows: testutil.NewMockRows([][]any{{"k1", strings.Repeat("cd", 32)}}),
	}}
	if _, err := LoadChecksums(context.Background(), conn, []string{"k1"}); err != nil {
		t.Fatalf("first LoadChecksums: %v", err)
	}
	if len(conn.queries) != 2 {
		t.Fatalf("expected OPENJSON try + fallback query, got %d", len(conn.queries))
	}
	if !strings.Contains(conn.queries[0].query, "OPENJSON(@p1)") {
		t.Fatalf("first query should be OPENJSON, got %q", conn.queries[0].query)
	}
	if strings.Contains(conn.queries[1].query, "OPENJSON(") {
		t.Fatalf("second query should be fallback IN path, got %q", conn.queries[1].query)
	}

	before := len(conn.queries)
	if _, err := LoadChecksums(context.Background(), conn, []string{"k1"}); err != nil {
		t.Fatalf("second LoadChecksums: %v", err)
	}
	if got := len(conn.queries) - before; got != 0 {
		t.Fatalf("expected cached fallback result to issue 0 more queries, got %d", got)
	}
	if len(conn.queries) != before {
		t.Fatalf("expected no new query after cached fallback, got %d total", len(conn.queries))
	}
}

func TestLoadChecksumsCachesUntilAuditWriteInvalidates(t *testing.T) {
	conn := &mssqlChecksumQueryConn{checksumQueryConn: checksumQueryConn{
		openJSONRows: testutil.NewMockRows([][]any{{"k1", strings.Repeat("ab", 32)}}),
	}}
	if _, err := LoadChecksums(context.Background(), conn, []string{"k1"}); err != nil {
		t.Fatalf("first LoadChecksums: %v", err)
	}
	if _, err := LoadChecksums(context.Background(), conn, []string{"k1"}); err != nil {
		t.Fatalf("second LoadChecksums: %v", err)
	}
	if len(conn.queries) != 1 {
		t.Fatalf("expected second LoadChecksums to hit cache, got %d queries", len(conn.queries))
	}

	bumpChecksumsCacheGeneration(conn)
	conn.openJSONRows = testutil.NewMockRows([][]any{{"k1", strings.Repeat("cd", 32)}})
	if _, err := LoadChecksums(context.Background(), conn, []string{"k1"}); err != nil {
		t.Fatalf("third LoadChecksums: %v", err)
	}
	if len(conn.queries) != 2 {
		t.Fatalf("expected cache invalidation to force a new query, got %d", len(conn.queries))
	}
}

func TestSubscriberOnObjectApplied_ExecutesSQL(t *testing.T) {
	conn := &testutil.MockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, &types.ObjectEvent{
		ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1"},
		Checksum:   "abc123",
		GitInfo:    types.GitInfo{GitHash: "deadbeef", GitAuthor: "dev", GitDate: "2024-01-01T00:00:00Z"},
		RecordKind: "object",
	})

	if conn.ExecCount.Load() != 2 {
		t.Errorf("expected 2 execs (bootstrap + insert), got %d", conn.ExecCount.Load())
		return
	}
	last := conn.Queries[len(conn.Queries)-1]
	if last.Args[0] != "r/tables/t1" {
		t.Errorf("normalized_key = %v, want r/tables/t1", last.Args[0])
	}
	if last.Args[2] != "abc123" {
		t.Errorf("checksum = %v, want abc123", last.Args[2])
	}
	if last.Args[3] != "deadbeef" {
		t.Errorf("git_hash = %v, want deadbeef", last.Args[3])
	}
	if last.Args[4] != "dev" {
		t.Errorf("git_author = %v, want dev", last.Args[4])
	}
	if last.Args[6] != "applied" {
		t.Errorf("event = %v, want applied", last.Args[6])
	}
}

type checksumQueryConn struct {
	queries      []checksumQuery
	openJSONRows driver.Rows
	fallbackRows driver.Rows
	failOpenJSON bool
}

type checksumQuery struct {
	query string
	args  []string
}

type mssqlChecksumQueryConn struct {
	checksumQueryConn
}

func (c *checksumQueryConn) QueryContext(context.Context, string, ...any) (driver.Rows, error) {
	return nil, nil
}

func (c *checksumQueryConn) QueryStringsContext(_ context.Context, query string, args []string) (driver.Rows, error) {
	c.queries = append(c.queries, checksumQuery{query: query, args: append([]string(nil), args...)})
	if strings.Contains(query, "OPENJSON(@p1)") {
		if c.failOpenJSON {
			return nil, errors.New("openjson unsupported")
		}
		if c.openJSONRows == nil {
			return testutil.NewMockRows(nil), nil
		}
		c.openJSONRows.(*testutil.MockRows).Reset()
		return c.openJSONRows, nil
	}
	if c.fallbackRows == nil {
		return testutil.NewMockRows(nil), nil
	}
	c.fallbackRows.(*testutil.MockRows).Reset()
	return c.fallbackRows, nil
}

func (c *checksumQueryConn) QueryStringSlicesContext(context.Context, string, []string, []string) (driver.Rows, error) {
	return nil, nil
}

func (c *checksumQueryConn) ExecContext(context.Context, string, ...any) (driver.Result, error) {
	return nil, nil
}

func (c *checksumQueryConn) Ping(context.Context) error { return nil }
func (c *checksumQueryConn) Close() error               { return nil }

func TestSubscriberOnObjectFailed_ExecutesSQL(t *testing.T) {
	conn := &testutil.MockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectFailed, &types.FailureEvent{
		ObjectEvent: types.ObjectEvent{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectName: "v1"},
			RecordKind: "object",
		},
		Error: "syntax error",
	})

	if conn.ExecCount.Load() != 2 {
		t.Errorf("expected 2 execs (bootstrap + insert), got %d", conn.ExecCount.Load())
		return
	}
	last := conn.Queries[len(conn.Queries)-1]
	if last.Args[6] != "failed" {
		t.Errorf("event = %v, want failed", last.Args[6])
	}
	if last.Args[7] != "syntax error" {
		t.Errorf("error_text = %v, want syntax error", last.Args[7])
	}
}

func TestSubscriberOnObjectAppliedBatch_ExecutesSingleInsert(t *testing.T) {
	conn := &testutil.MockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, []*types.ObjectEvent{
		{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1"},
			Checksum:   "abc123",
			GitInfo:    types.GitInfo{GitHash: "deadbeef", GitAuthor: "dev", GitDate: "2024-01-01T00:00:00Z"},
			RecordKind: "object",
		},
		{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectName: "v1"},
			Checksum:   "def456",
			GitInfo:    types.GitInfo{GitHash: "feedface", GitAuthor: "dev2", GitDate: "2024-01-02T00:00:00Z"},
			RecordKind: "object",
		},
	})

	if conn.ExecCount.Load() != 2 {
		t.Fatalf("expected 2 execs (bootstrap + batch insert), got %d", conn.ExecCount.Load())
	}
	last := conn.Queries[len(conn.Queries)-1]
	if got := len(last.Args); got != 16 {
		t.Fatalf("args len = %d, want 16", got)
	}
	if last.Args[0] != "r/tables/t1" || last.Args[8] != "r/views/v1" {
		t.Fatalf("unexpected normalized keys in args: %v", last.Args)
	}
	if !strings.Contains(last.Query, "(@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, SYSUTCDATETIME())") {
		t.Fatalf("expected first VALUES row in query, got %q", last.Query)
	}
	if !strings.Contains(last.Query, "(@p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, SYSUTCDATETIME())") {
		t.Fatalf("expected second VALUES row in query, got %q", last.Query)
	}
}

func TestSubscriberOnObjectAppliedBatch_UsesOpenJSONInsertForMSSQL(t *testing.T) {
	conn := &mssqlAuditExecConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, []*types.ObjectEvent{
		{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1"},
			Checksum:   "abc123",
			GitInfo:    types.GitInfo{GitHash: "deadbeef", GitAuthor: "dev", GitDate: "2024-01-01T00:00:00Z"},
			RecordKind: "object",
		},
		{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectName: "v1"},
			Checksum:   "def456",
			GitInfo:    types.GitInfo{GitHash: "feedface", GitAuthor: "dev2", GitDate: "2024-01-02T00:00:00Z"},
			RecordKind: "object",
		},
	})

	if len(conn.execs) != 2 {
		t.Fatalf("expected bootstrap + OPENJSON insert, got %d execs", len(conn.execs))
	}
	last := conn.execs[1]
	if !strings.Contains(last.query, "FROM OPENJSON(@p1)") {
		t.Fatalf("expected OPENJSON insert query, got %q", last.query)
	}
	if len(last.args) != 1 {
		t.Fatalf("expected 1 JSON arg, got %d", len(last.args))
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(last.args[0].(string)), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("payload len = %d, want 2", len(payload))
	}
	if got := payload[0]["normalized_key"]; got != "r/tables/t1" {
		t.Fatalf("first normalized_key = %v, want r/tables/t1", got)
	}
}

func TestSubscriberOnObjectAppliedBatch_FallsBackFromOpenJSONInsert(t *testing.T) {
	conn := &mssqlAuditExecConn{failOpenJSON: true}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, []*types.ObjectEvent{
		{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1"},
			Checksum:   "abc123",
			GitInfo:    types.GitInfo{GitHash: "deadbeef", GitAuthor: "dev", GitDate: "2024-01-01T00:00:00Z"},
			RecordKind: "object",
		},
	})

	if len(conn.execs) != 3 {
		t.Fatalf("expected bootstrap + OPENJSON attempt + fallback insert, got %d execs", len(conn.execs))
	}
	if !strings.Contains(conn.execs[1].query, "FROM OPENJSON(@p1)") {
		t.Fatalf("expected second exec to be OPENJSON insert, got %q", conn.execs[1].query)
	}
	if strings.Contains(conn.execs[2].query, "OPENJSON(") {
		t.Fatalf("expected fallback exec to use legacy VALUES query, got %q", conn.execs[2].query)
	}
	if len(conn.execs[2].args) != 8 {
		t.Fatalf("expected legacy fallback to use 8 args, got %d", len(conn.execs[2].args))
	}
}

func TestLoadAppliedMigrations_ReturnsMigrationKeys(t *testing.T) {
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT": testutil.NewMockRows([][]any{{"r/tables/t1/001_deadbeef_add_col.sql"}}),
		},
	}
	result, err := LoadAppliedMigrations(context.Background(), conn, "r/tables/t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result["r/tables/t1/001_deadbeef_add_col.sql"] {
		t.Error("expected migration key to be true")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestLoadAppliedMigrations_QueryError(t *testing.T) {
	conn := &testutil.MockConn{QueryErr: errors.New("dead")}
	_, err := LoadAppliedMigrations(context.Background(), conn, "r/tables/t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubscriberBootstrapCalledOnce(t *testing.T) {
	conn := &testutil.MockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, &types.ObjectEvent{
		ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables"},
		RecordKind: "object",
	})

	bootstrapCount := 0
	for _, q := range conn.Queries {
		if len(q.Args) == 0 {
			bootstrapCount++
		}
	}
	if bootstrapCount != 1 {
		t.Errorf("expected 1 bootstrap query, got %d (queries: %d)", bootstrapCount, len(conn.Queries))
	}
}

func TestLoadAllAppliedMigrations_ReturnsKeys(t *testing.T) {
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT": testutil.NewMockRows([][]any{
				{"r/tables/t1/001_abc_add.sql"},
				{"r/tables/t1/002_def_drop.sql"},
			}),
		},
	}
	result, err := LoadAllAppliedMigrations(context.Background(), conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if !result["r/tables/t1/001_abc_add.sql"] {
		t.Error("expected first migration key to be true")
	}
	if !result["r/tables/t1/002_def_drop.sql"] {
		t.Error("expected second migration key to be true")
	}
}

func TestLoadAllAppliedMigrations_QueryError(t *testing.T) {
	conn := &testutil.MockConn{QueryErr: errors.New("dead")}
	_, err := LoadAllAppliedMigrations(context.Background(), conn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadAllAppliedMigrations_EmptyResult(t *testing.T) {
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT": testutil.NewMockRows(nil),
		},
	}
	result, err := LoadAllAppliedMigrations(context.Background(), conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

type auditExecCall struct {
	query string
	args  []any
}

type mssqlAuditExecConn struct {
	execs        []auditExecCall
	failOpenJSON bool
}

func (c *mssqlAuditExecConn) QueryContext(context.Context, string, ...any) (driver.Rows, error) {
	return nil, nil
}

func (c *mssqlAuditExecConn) QueryStringsContext(context.Context, string, []string) (driver.Rows, error) {
	return nil, nil
}

func (c *mssqlAuditExecConn) QueryStringSlicesContext(context.Context, string, []string, []string) (driver.Rows, error) {
	return nil, nil
}

func (c *mssqlAuditExecConn) ExecContext(_ context.Context, query string, args ...any) (driver.Result, error) {
	c.execs = append(c.execs, auditExecCall{query: query, args: append([]any(nil), args...)})
	if c.failOpenJSON && strings.Contains(query, "FROM OPENJSON(@p1)") {
		return nil, errors.New("OPENJSON requires compatibility level 130")
	}
	return &testutil.MockResult{}, nil
}

func (c *mssqlAuditExecConn) Ping(context.Context) error { return nil }
func (c *mssqlAuditExecConn) Close() error               { return nil }
