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
	_, err := LoadChecksums(context.Background(), conn, []string{"k1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func mockConnWithHistoryRows(extra map[string]*testutil.MockRows) *testutil.MockConn {
	m := map[string]*testutil.MockRows{
		"SELECT CASE": testutil.NewMockRows([][]any{{true}}),
	}
	for k, v := range extra {
		m[k] = v
	}
	return &testutil.MockConn{RowsByPrefix: m}
}

func TestLoadChecksumsSkipsOpenJSONWhenHistoryEmpty(t *testing.T) {
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT CASE": testutil.NewMockRows([][]any{{false}}),
		},
	}
	result, err := LoadChecksums(context.Background(), conn, []string{"k1", "k2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result))
	}
	if conn.QueryCount.Load() != 1 {
		t.Fatalf("expected only history probe query, got %d", conn.QueryCount.Load())
	}
}

func TestLoadChecksumsReturnsResults(t *testing.T) {
	var want [32]byte
	for i := range want {
		want[i] = byte(i + 1)
	}
	conn := mockConnWithHistoryRows(map[string]*testutil.MockRows{
		"WITH checksum_keys": testutil.NewMockRows([][]any{{"k1", hex.EncodeToString(want[:])}}),
	})
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
	conn := mockConnWithHistoryRows(map[string]*testutil.MockRows{
		"WITH checksum_keys": testutil.NewMockRows(nil),
	})
	LoadChecksums(context.Background(), conn, keys)
	if conn.QueryCount.Load() != 2 {
		t.Errorf("expected history probe + 1 OPENJSON query, got %d", conn.QueryCount.Load())
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
	if len(conn.queries) != 2 {
		t.Fatalf("expected history probe + OPENJSON (2 queries), got %d", len(conn.queries))
	}
	var openJSON *checksumQuery
	for i := range conn.queries {
		if strings.Contains(conn.queries[i].query, "OPENJSON(@p1)") {
			openJSON = &conn.queries[i]
			break
		}
	}
	if openJSON == nil {
		t.Fatalf("expected OPENJSON query among %d queries", len(conn.queries))
	}
	if got := openJSON.args[0]; got != `["k1"]` {
		t.Fatalf("JSON arg = %q, want [\"k1\"]", got)
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
	if len(conn.queries) != 2 {
		t.Fatalf("expected second LoadChecksums to hit cache (probe+openjson), got %d queries", len(conn.queries))
	}

	bumpChecksumsCacheGeneration(conn)
	conn.openJSONRows = testutil.NewMockRows([][]any{{"k1", strings.Repeat("cd", 32)}})
	if _, err := LoadChecksums(context.Background(), conn, []string{"k1"}); err != nil {
		t.Fatalf("third LoadChecksums: %v", err)
	}
	if len(conn.queries) != 2 {
		t.Fatalf("expected latest checksum cache to avoid a new query after invalidation, still %d queries", len(conn.queries))
	}
}

func TestLoadChecksumsUsesLatestAuditWriteCache(t *testing.T) {
	conn := &testutil.MockConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, []*types.ObjectEvent{
		{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1"},
			Checksum:   strings.Repeat("ab", 32),
			RecordKind: "object",
		},
	})
	publishAuditFlush(b)

	before := conn.QueryCount.Load()
	result, err := LoadChecksums(context.Background(), conn, []string{"r/tables/t1"})
	if err != nil {
		t.Fatalf("LoadChecksums: %v", err)
	}
	if got := conn.QueryCount.Load(); got != before {
		t.Fatalf("expected LoadChecksums to avoid DB query via latest cache, got query count %d -> %d", before, got)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 checksum from latest cache, got %d", len(result))
	}
}

func TestSubscriberOnObjectApplied_ExecutesSQL(t *testing.T) {
	conn := &mssqlAuditExecConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectApplied, &types.ObjectEvent{
		ObjectRef:  types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1"},
		Checksum:   "abc123",
		GitInfo:    types.GitInfo{GitHash: "deadbeef", GitAuthor: "dev", GitDate: "2024-01-01T00:00:00Z"},
		RecordKind: "object",
	})
	publishAuditFlush(b)

	if len(conn.execs) != 3 {
		t.Fatalf("expected bootstrap tables + index + OPENJSON insert, got %d execs", len(conn.execs))
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(conn.execs[2].args[0].(string)), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload[0]["normalized_key"] != "r/tables/t1" {
		t.Errorf("normalized_key = %v, want r/tables/t1", payload[0]["normalized_key"])
	}
	if payload[0]["checksum"] != "abc123" {
		t.Errorf("checksum = %v, want abc123", payload[0]["checksum"])
	}
	if payload[0]["event"] != "applied" {
		t.Errorf("event = %v, want applied", payload[0]["event"])
	}
}

type checksumQueryConn struct {
	openJSONRows driver.Rows
	fallbackRows driver.Rows
	queries      []checksumQuery
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
	if strings.Contains(query, "has_rows") {
		return testutil.NewMockRows([][]any{{true}}), nil
	}
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
	conn := &mssqlAuditExecConn{}
	b := bus.New()
	NewSubscriber(b, conn)

	b.Publish(context.Background(), types.EventObjectFailed, &types.FailureEvent{
		ObjectEvent: types.ObjectEvent{
			ObjectRef:  types.ObjectRef{NormalizedKey: "r/views/v1", Kind: "views", ObjectName: "v1"},
			RecordKind: "object",
		},
		Error: "syntax error",
	})
	publishAuditFlush(b)

	if len(conn.execs) != 3 {
		t.Fatalf("expected bootstrap tables + index + OPENJSON insert, got %d execs", len(conn.execs))
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(conn.execs[2].args[0].(string)), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload[0]["event"] != "failed" {
		t.Errorf("event = %v, want failed", payload[0]["event"])
	}
	if payload[0]["error_text"] != "syntax error" {
		t.Errorf("error_text = %v, want syntax error", payload[0]["error_text"])
	}
}

func TestSubscriberOnObjectAppliedBatch_ExecutesSingleInsert(t *testing.T) {
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
	publishAuditFlush(b)

	if len(conn.execs) != 3 {
		t.Fatalf("expected bootstrap tables + index + batch insert, got %d execs", len(conn.execs))
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(conn.execs[2].args[0].(string)), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("payload len = %d, want 2", len(payload))
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
	publishAuditFlush(b)

	if len(conn.execs) != 3 {
		t.Fatalf("expected bootstrap tables + index + OPENJSON insert, got %d execs", len(conn.execs))
	}
	last := conn.execs[2]
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
	publishAuditFlush(b)

	tablesBootstrap := 0
	indexBootstrap := 0
	for _, q := range conn.ExecQueries {
		if strings.Contains(q, "CREATE TABLE") && strings.Contains(q, "history") {
			tablesBootstrap++
		}
		if strings.Contains(q, "CREATE INDEX") || strings.Contains(q, "CREATE NONCLUSTERED INDEX") {
			indexBootstrap++
		}
	}
	if tablesBootstrap != 1 {
		t.Errorf("expected 1 tables bootstrap exec, got %d", tablesBootstrap)
	}
	if indexBootstrap != 1 {
		t.Errorf("expected 1 index bootstrap exec, got %d", indexBootstrap)
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

func publishAuditFlush(b bus.EventBus) {
	b.Publish(context.Background(), types.EventRunFinished, &types.RunFinished{Command: "test", Result: "success"})
}
