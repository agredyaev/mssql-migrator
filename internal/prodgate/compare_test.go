package prodgate

import (
	"testing"

	"reporting-db-migrations/internal/types"
)

func TestSnapshotFromPlan_roundTripFields(t *testing.T) {
	plan := &types.MigrationPlan{
		Blocked:    false,
		LayoutHash: "abc",
		Objects: []types.PlannedObject{
			{
				ObjectRef: types.ObjectRef{
					ObjectPath:    "db/smoke/tables/smoke_table.sql",
					SchemaName:    "smoke",
					Kind:          "tables",
					ObjectName:    "smoke_table",
					NormalizedKey: "smoke/tables/smoke_table",
				},
				PlannedAction: types.ActionCreateObject,
				Checksum:      [32]byte{1, 2, 3},
				Exists:        false,
			},
		},
	}
	snap := SnapshotFromPlan(plan)
	if snap.Objects["smoke/tables/smoke_table"].PlannedAction != types.ActionCreateObject {
		t.Fatalf("action = %q", snap.Objects["smoke/tables/smoke_table"].PlannedAction)
	}
	if snap.LayoutHash != "abc" {
		t.Fatalf("layout hash = %q", snap.LayoutHash)
	}
}

func TestCompareSnapshots_strictOutsideDelta(t *testing.T) {
	baseline := PlanSnapshot{
		Version: SnapshotVersion,
		Objects: map[string]ObjectEntry{
			"a": {PlannedAction: "skip_unchanged"},
			"b": {PlannedAction: "skip_unchanged"},
		},
	}
	current := PlanSnapshot{
		Version: SnapshotVersion,
		Objects: map[string]ObjectEntry{
			"a": {PlannedAction: "create_object"},
			"b": {PlannedAction: "skip_unchanged"},
		},
	}
	res := CompareSnapshots(baseline, current, CompareOptions{
		DeltaKeys:        map[string]struct{}{"a": {}},
		StrictUnexpected: true,
	})
	if !res.Go {
		t.Fatalf("expected go when only delta key changed: %+v", res)
	}
	if len(res.DeltaChanges) == 0 {
		t.Fatal("expected delta changes for key a")
	}

	current2 := PlanSnapshot{
		Version: SnapshotVersion,
		Objects: map[string]ObjectEntry{
			"a": {PlannedAction: "skip_unchanged"},
			"b": {PlannedAction: "create_object"},
		},
	}
	res2 := CompareSnapshots(baseline, current2, CompareOptions{
		DeltaKeys:        map[string]struct{}{"a": {}},
		StrictUnexpected: true,
	})
	if res2.Go {
		t.Fatalf("expected no-go for change outside delta: %+v", res2)
	}
}

func TestCompareSnapshots_blockedNoGo(t *testing.T) {
	baseline := PlanSnapshot{Version: SnapshotVersion, Objects: map[string]ObjectEntry{}}
	current := PlanSnapshot{Version: SnapshotVersion, Blocked: true, Objects: map[string]ObjectEntry{}}
	res := CompareSnapshots(baseline, current, CompareOptions{StrictUnexpected: true})
	if res.Go {
		t.Fatal("expected no-go when blocked")
	}
}
