package db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/testutil"
	"reporting-db-migrations/internal/types"
)

func TestInspectEmptyScope(t *testing.T) {
	insp := NewInspector()
	conn := &testutil.MockConn{}
	layout := fs.Layout{}
	state, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Schemas) != 0 {
		t.Errorf("expected 0 schemas, got %d", len(state.Schemas))
	}
	if len(state.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(state.Objects))
	}
	if conn.QueryCount.Load() != 0 {
		t.Errorf("expected 0 queries, got %d", conn.QueryCount.Load())
	}
}

func TestScopeKeyEmptyLayout(t *testing.T) {
	if got := scopeKey(fs.Layout{}); got != "" {
		t.Errorf("scopeKey(empty layout) = %q, want empty string", got)
	}
}

func TestScopeKeyGoldenMixed(t *testing.T) {
	layout := fs.Layout{
		Schemas: []fs.Schema{
			{Name: "B", NormalizedName: "b"},
			{Name: "A", NormalizedName: "a"},
		},
		Objects: []*fs.Object{
			{NormalizedKey: "sch/views/x"},
		},
	}
	got := scopeKey(layout)
	const want = "o:sch/views/x|s:a|s:b"
	if got != want {
		t.Errorf("scopeKey = %q, want %q", got, want)
	}
}

func TestScopeKeyGoldenWithAllPartKinds(t *testing.T) {
	layout := fs.Layout{
		Schemas: []fs.Schema{
			{Name: "Z", NormalizedName: "z"},
			{Name: "M", NormalizedName: "m"},
		},
		Objects: []*fs.Object{
			{NormalizedKey: "b/obj"},
		},
		Transitions: []*fs.TransitionScript{
			{NormalizedKey: "a/trans"},
		},
		Checks: []*fs.CheckScript{
			{Path: "db/x/checks/a.sql"},
		},
	}
	got := scopeKey(layout)
	const want = "c:db/x/checks/a.sql|o:b/obj|s:m|s:z|t:a/trans"
	if got != want {
		t.Errorf("scopeKey = %q, want %q", got, want)
	}
}

func TestScopeKeySHA256HexEmptyCanonical(t *testing.T) {
	if got := scopeKeySHA256Hex(""); got != "" {
		t.Errorf("scopeKeySHA256Hex(\"\") = %q, want empty", got)
	}
}

func TestScopeKeySHA256HexGoldenVectors(t *testing.T) {
	cases := []struct {
		canonical string
		wantHex   string
	}{
		{
			"o:sch/views/x|s:a|s:b",
			"307d4ee76fe11f0fa33fbbe496863d483900b6e3190c7eb70997fad37d66ba10",
		},
		{
			"c:db/x/checks/a.sql|o:b/obj|s:m|s:z|t:a/trans",
			"6d238d4bb6bebf01f258289560516771eb3f131ce0be5a0aee179110b7e066f9",
		},
	}
	for _, tc := range cases {
		if got := scopeKeySHA256Hex(tc.canonical); got != tc.wantHex {
			t.Errorf("scopeKeySHA256Hex(%q) = %q, want %q", tc.canonical, got, tc.wantHex)
		}
	}
}

func TestInspectCachesResult(t *testing.T) {
	insp := NewInspector()
	conn := &testutil.MockConn{}
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}
	state1, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("first Inspect: %v", err)
	}
	state2, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("second Inspect: %v", err)
	}
	if state1 != state2 {
		t.Error("cached state should be same pointer")
	}
	qc := conn.QueryCount.Load()
	if qc == 0 {
		t.Fatal("expected at least 1 query for first Inspect")
	}
	state3, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("third Inspect: %v", err)
	}
	if state3 != state1 {
		t.Error("cached state should be same pointer")
	}
	if conn.QueryCount.Load() != qc {
		t.Errorf("expected query count to stay at %d, got %d", qc, conn.QueryCount.Load())
	}
}

func TestInspectConnectionError(t *testing.T) {
	insp := NewInspector()
	conn := &testutil.MockConn{QueryErr: errors.New("connection refused")}
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}
	_, err := insp.Inspect(context.Background(), conn, layout)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInspectDifferentScopeNotCached(t *testing.T) {
	insp := NewInspector()
	conn := &testutil.MockConn{}
	layout1 := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}
	layout2 := fs.Layout{
		Schemas: []fs.Schema{{Name: "x", NormalizedName: "x"}},
	}
	state1, _ := insp.Inspect(context.Background(), conn, layout1)
	state2, _ := insp.Inspect(context.Background(), conn, layout2)
	if state1 == state2 {
		t.Error("different scopes should not share cached state")
	}
	qc := conn.QueryCount.Load()
	if qc < 2 {
		t.Errorf("expected at least 2 queries, got %d", qc)
	}
}

func TestInspectSharedCacheAcrossInspectorInstances(t *testing.T) {
	conn := &testutil.MockConn{}
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}

	state1, err := NewInspector().Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("first Inspect: %v", err)
	}
	qc := conn.QueryCount.Load()
	if qc == 0 {
		t.Fatal("expected at least one query on first inspect")
	}

	state2, err := NewInspector().Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("second Inspect: %v", err)
	}
	if state1 != state2 {
		t.Fatal("expected shared inspector cache to return same state pointer")
	}
	if got := conn.QueryCount.Load(); got != qc {
		t.Fatalf("expected second inspector instance to reuse cache; queries %d != %d", got, qc)
	}
}

func TestInspectSharedCacheInvalidatesAfterGenerationBump(t *testing.T) {
	conn := &testutil.MockConn{}
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
	}

	state1, err := NewInspector().Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("first Inspect: %v", err)
	}
	qc := conn.QueryCount.Load()
	InvalidateInspectorCache(conn)

	state2, err := NewInspector().Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("second Inspect: %v", err)
	}
	if state1 == state2 {
		t.Fatal("expected cache invalidation to force fresh state")
	}
	if got := conn.QueryCount.Load(); got <= qc {
		t.Fatalf("expected more queries after invalidation, got %d <= %d", got, qc)
	}
}

func TestInspectUsesOpenJSONQueriesWhenSupported(t *testing.T) {
	layout := openJSONTestLayout()
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT CASE":                              testutil.NewMockRows([][]any{{1}}),
			"WITH inspector_schema_filter AS (":        testutil.NewMockRows([][]any{{"r"}}),
			"WITH inspector_object_schema_filter AS (": testutil.NewMockRows([][]any{{"r", "tables", "t1", ""}}),
			"WITH inspector_column_schema_filter AS (": testutil.NewMockRows([][]any{{"r", "t1", "id", "int", 4, 10, 0, false}}),
		},
	}

	insp := NewInspector().(*inspector)
	state, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !insp.openJSONEnabled {
		t.Fatal("expected OPENJSON path to be enabled")
	}
	if got := conn.QueryCount.Load(); got != 4 {
		t.Fatalf("expected 4 queries (probe + schemas + objects + columns), got %d", got)
	}
	if !strings.Contains(conn.Queries[1].Query, "OPENJSON(@p1)") {
		t.Fatalf("expected schemas query to use OPENJSON, got %q", conn.Queries[1].Query)
	}
	if !strings.Contains(conn.Queries[2].Query, "OPENJSON(@p2)") {
		t.Fatalf("expected objects query to use OPENJSON, got %q", conn.Queries[2].Query)
	}
	if len(conn.Queries[2].StringArgs1) != 2 {
		t.Fatalf("expected 2 JSON args for objects query, got %d", len(conn.Queries[2].StringArgs1))
	}
	var schemaNames, objectNames []string
	if err := json.Unmarshal([]byte(conn.Queries[2].StringArgs1[0]), &schemaNames); err != nil {
		t.Fatalf("unmarshal schema JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(conn.Queries[2].StringArgs1[1]), &objectNames); err != nil {
		t.Fatalf("unmarshal object JSON: %v", err)
	}
	if len(schemaNames) != 1 || schemaNames[0] != "r" {
		t.Fatalf("schema JSON = %v, want [r]", schemaNames)
	}
	if len(objectNames) != 1 || objectNames[0] != "t1" {
		t.Fatalf("object JSON = %v, want [t1]", objectNames)
	}
	if _, ok := state.Objects["r/tables/t1"]; !ok {
		t.Fatalf("expected object r/tables/t1 in state, got %v", state.Objects)
	}
	if got := len(state.TableColumns["r/tables/t1"]); got != 1 {
		t.Fatalf("expected 1 table column, got %d", got)
	}
}

func TestInspectFallsBackWhenOpenJSONUnsupported(t *testing.T) {
	layout := openJSONTestLayout()
	conn := &testutil.MockConn{
		RowsByPrefix: map[string]*testutil.MockRows{
			"SELECT CASE":                         testutil.NewMockRows([][]any{{0}}),
			"SELECT LOWER(s.name) AS schema_name": testutil.NewMockRows([][]any{{"r"}}),
			"SELECT\n    LOWER(s.name) AS schema_name,\n    CASE LOWER(o.type_desc)":     testutil.NewMockRows([][]any{{"r", "tables", "t1", ""}}),
			"SELECT\n    LOWER(s.name) AS schema_name,\n    LOWER(o.name) AS table_name": testutil.NewMockRows([][]any{{"r", "t1", "id", "int", 4, 10, 0, false}}),
		},
	}

	insp := NewInspector().(*inspector)
	state, err := insp.Inspect(context.Background(), conn, layout)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.openJSONEnabled {
		t.Fatal("expected OPENJSON path to be disabled")
	}
	if got := conn.QueryCount.Load(); got != 4 {
		t.Fatalf("expected 4 queries (probe + schemas + objects + columns), got %d", got)
	}
	if strings.Contains(conn.Queries[1].Query, "OPENJSON(") {
		t.Fatalf("expected fallback schemas query, got %q", conn.Queries[1].Query)
	}
	if len(conn.Queries[1].StringArgs1) != 1 || conn.Queries[1].StringArgs1[0] != "r" {
		t.Fatalf("schema fallback args = %v, want [r]", conn.Queries[1].StringArgs1)
	}
	if len(conn.Queries[2].StringArgs1) != 1 || conn.Queries[2].StringArgs1[0] != "r" {
		t.Fatalf("object fallback schema args = %v, want [r]", conn.Queries[2].StringArgs1)
	}
	if len(conn.Queries[2].StringArgs2) != 1 || conn.Queries[2].StringArgs2[0] != "t1" {
		t.Fatalf("object fallback object args = %v, want [t1]", conn.Queries[2].StringArgs2)
	}
	if _, ok := state.Objects["r/tables/t1"]; !ok {
		t.Fatalf("expected object r/tables/t1 in state, got %v", state.Objects)
	}
}

func TestMarshalStringSliceJSONRoundTripAwkwardIdentifiers(t *testing.T) {
	values := []string{
		`name,with,comma`,
		`quote"name`,
		`bracket]name`,
		`space name`,
	}
	got := types.MarshalStringSliceJSON(values)
	var roundTrip []string
	if err := json.Unmarshal([]byte(got), &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(roundTrip) != len(values) {
		t.Fatalf("roundTrip len = %d, want %d", len(roundTrip), len(values))
	}
	for i := range values {
		if roundTrip[i] != values[i] {
			t.Fatalf("roundTrip[%d] = %q, want %q", i, roundTrip[i], values[i])
		}
	}
}

func openJSONTestLayout() fs.Layout {
	return fs.Layout{
		Schemas: []fs.Schema{
			{Name: "r", NormalizedName: "r"},
		},
		Objects: []*fs.Object{
			{
				SchemaName:           "r",
				NormalizedSchemaName: "r",
				Kind:                 "tables",
				ObjectName:           "t1",
				NormalizedKey:        types.NormalizedKey("r", "tables", "t1"),
			},
		},
	}
}
