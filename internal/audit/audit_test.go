package audit

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"reporting-db-migrations/internal/bus"
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
	LoadChecksums(context.Background(), conn, keys)
	if conn.QueryCount.Load() < 2 {
		t.Errorf("expected at least 2 queries for 2500 keys, got %d", conn.QueryCount.Load())
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
