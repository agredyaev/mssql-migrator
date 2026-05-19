package prodgate

import (
	"testing"

	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestExpandDeltaClosure_TriggerParent(t *testing.T) {
	layout := fs.Layout{
		Objects: []*fs.Object{
			{
				SchemaName:          "r",
				Kind:                "tables",
				ObjectName:          "t1",
				NormalizedKey:       types.NormalizedKey("r", "tables", "t1"),
				ParentNormalizedKey: "",
			},
			{
				SchemaName:          "r",
				Kind:                "triggers",
				ObjectName:          "tr1",
				NormalizedKey:       types.NormalizedKey("r", "triggers", "tr1"),
				ParentName:          "t1",
				ParentNormalizedKey: types.NormalizedKey("r", "tables", "t1"),
			},
		},
	}
	delta := map[string]struct{}{
		types.NormalizedKey("r", "triggers", "tr1"): {},
	}
	out := ExpandDeltaClosure(layout, delta)
	if _, ok := out[types.NormalizedKey("r", "tables", "t1")]; !ok {
		t.Fatal("expected parent table in closure")
	}
}

func TestExpandDeltaClosure_TransitionTable(t *testing.T) {
	layout := fs.Layout{
		Transitions: []*fs.TransitionScript{
			{
				SchemaName:    "r",
				TableName:     "t1",
				NormalizedKey: "r/tables/t1/trans/001.sql",
				Path:          "r/tables/t1/_migrations/t1/001.sql",
			},
		},
	}
	delta := map[string]struct{}{
		"r/tables/t1/trans/001.sql": {},
	}
	out := ExpandDeltaClosure(layout, delta)
	tableKey := types.NormalizedKey("r", "tables", "t1")
	if _, ok := out[tableKey]; !ok {
		t.Fatalf("expected table key %s in closure, got %v", tableKey, out)
	}
}
