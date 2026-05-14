package diff

import (
	"context"
	"testing"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestComputeEmptyLayout(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{}
	state := &db.State{
		Schemas:      map[string]struct{}{},
		Objects:      map[string]db.Object{},
		TableColumns: map[string][]db.TableColumn{},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Schemas) != 0 {
		t.Errorf("expected 0 schemas, got %d", len(plan.Schemas))
	}
	if len(plan.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(plan.Objects))
	}
}

func TestComputeNewObject_ActionCreateObject(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			Path:          "r/views/v1.sql",
			SchemaName:    "r",
			Kind:          "views",
			ObjectName:    "v1",
			NormalizedKey: "r/views/v1",
		}},
	}
	state := &db.State{
		Schemas:      map[string]struct{}{},
		Objects:      map[string]db.Object{},
		TableColumns: map[string][]db.TableColumn{},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(plan.Objects))
	}
	if plan.Objects[0].PlannedAction != types.ActionCreateObject {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionCreateObject)
	}
}

func TestComputeUnchangedObject_ActionSkip(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/views/v1",
			Kind:          "views",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/views/v1": {SchemaName: "r", Kind: "views", ObjectName: "v1"},
		},
	}
	checksums := map[string]string{"r/views/v1": "abc"}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Objects) != 1 {
		t.Fatalf("expected 1 object")
	}
	if plan.Objects[0].PlannedAction != types.ActionSkipUnchanged {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionSkipUnchanged)
	}
}

func TestComputeChangedObject_ActionReprocess(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/types/t1",
			Kind:          "types",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/types/t1": {SchemaName: "r", Kind: "types", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionReprocessChanged {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionReprocessChanged)
	}
}

func TestComputeTableChangedWithoutTransition_Blocked(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.Blocked {
		t.Fatal("expected plan to be blocked")
	}
	if plan.Objects[0].PlannedAction != types.ActionReprocessChangedBlocked {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionReprocessChangedBlocked)
	}
}

func TestComputeTableChangedWithTransition_NotBlocked(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
		}},
		Transitions: []*fs.TransitionScript{{
			TableName:     "t1",
			NormalizedKey: "r/tables/t1",
			Path:          "r/tables/_migrations/t1/001_a_add_col.sql",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Blocked {
		t.Fatal("expected plan NOT to be blocked")
	}
	obj := plan.Objects[0]
	if obj.PlannedAction != types.ActionReprocessChanged {
		t.Errorf("action = %q, want %q", obj.PlannedAction, types.ActionReprocessChanged)
	}
	if len(obj.TransitionPaths) != 1 {
		t.Fatalf("expected 1 transition path, got %d", len(obj.TransitionPaths))
	}
	if obj.TransitionPaths[0] != "r/tables/_migrations/t1/001_a_add_col.sql" {
		t.Errorf("transition path = %q", obj.TransitionPaths[0])
	}
}

func TestComputeViewChanged_UpdateModule(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/views/v1",
			Kind:          "views",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/views/v1": {SchemaName: "r", Kind: "views", ObjectName: "v1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionUpdateExistingModule {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionUpdateExistingModule)
	}
}

func TestComputeNilState_AllCreate(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/views/v1",
			Kind:          "views",
		}},
	}

	plan, err := computer.Compute(context.Background(), layout, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(plan.Objects))
	}
	if plan.Objects[0].PlannedAction != types.ActionCreateObject {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionCreateObject)
	}
}

func TestComputeTableScaffoldOnly_Blocked(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
		}},
		Transitions: []*fs.TransitionScript{{
			TableName:     "t1",
			NormalizedKey: "r/tables/t1",
			Path:          "r/tables/_migrations/t1/001_scaffold.sql",
			Scaffold:      true,
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.Blocked {
		t.Fatal("expected plan to be blocked")
	}
	if plan.Objects[0].PlannedAction != types.ActionReprocessChangedBlocked {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionReprocessChangedBlocked)
	}
}

func TestComputeTriggerMissingParent_Blocked(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/triggers/trg1",
			SchemaName:    "r",
			Kind:          "triggers",
			ObjectName:    "trg1",
			ParentName:    "missing_parent",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/triggers/trg1": {SchemaName: "r", Kind: "triggers", ObjectName: "trg1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.Blocked {
		t.Fatal("expected plan to be blocked")
	}
	if plan.Objects[0].PlannedAction != types.ActionReprocessChangedBlocked {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionReprocessChangedBlocked)
	}
}

func TestComputeTriggerParentStable_UpdateModule(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/triggers/trg1",
			SchemaName:    "r",
			Kind:          "triggers",
			ObjectName:    "trg1",
			ParentName:    "t1",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/triggers/trg1": {SchemaName: "r", Kind: "triggers", ObjectName: "trg1"},
			"r/tables/t1":     {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{
		"r/tables/t1": "abc1234",
	}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionUpdateExistingModule {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionUpdateExistingModule)
	}
}

func TestComputeProcedureChanged_UpdateModule(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/procedures/p1",
			Kind:          "procedures",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/procedures/p1": {SchemaName: "r", Kind: "procedures", ObjectName: "p1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionUpdateExistingModule {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionUpdateExistingModule)
	}
}

func TestComputeFunctionChanged_UpdateModule(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/functions/f1",
			Kind:          "functions",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/functions/f1": {SchemaName: "r", Kind: "functions", ObjectName: "f1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionUpdateExistingModule {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionUpdateExistingModule)
	}
}

func TestComputeIndexesChanged_Reprocess(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/indexes/i1",
			Kind:          "indexes",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/indexes/i1": {SchemaName: "r", Kind: "indexes", ObjectName: "i1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionReprocessChanged {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionReprocessChanged)
	}
}

func TestComputeBlockedHasReason(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
		}},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.Blocked {
		t.Fatal("expected plan to be blocked")
	}
	if len(plan.Blockers) == 0 {
		t.Fatal("expected non-empty Blockers")
	}
}
