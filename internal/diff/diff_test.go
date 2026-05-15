package diff

import (
	"context"
	"os"
	"path/filepath"
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
	if len(plan.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(plan.Objects))
	}
	if plan.PlannedAt.IsZero() {
		t.Error("expected PlannedAt to be set")
	}
	if plan.Summary.ObjectCount != 0 {
		t.Errorf("summary.ObjectCount = %d, want 0", plan.Summary.ObjectCount)
	}
}

func TestComputeNewObject_ActionCreateObject(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/views/v1", "views", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	obj.SchemaName = "r"
	obj.ObjectName = "v1"
	layout := fs.Layout{
		Objects: []*fs.Object{obj},
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
	if plan.Summary.CreateCount != 1 {
		t.Errorf("summary.CreateCount = %d, want 1", plan.Summary.CreateCount)
	}
}

func TestComputeUnchangedObject_ActionSkip(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/views/v1", "views", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	checksum, err := obj.Checksum()
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}

	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/views/v1": {SchemaName: "r", Kind: "views", ObjectName: "v1"},
		},
	}
	checksums := map[string]string{"r/views/v1": checksum}

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
	if plan.Summary.SkipCount != 1 {
		t.Errorf("summary.SkipCount = %d, want 1", plan.Summary.SkipCount)
	}
}

func TestComputeAdoptExisting(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/views/v1", "views", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	layout := fs.Layout{
		Objects: []*fs.Object{obj},
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
	if plan.Objects[0].PlannedAction != types.ActionAdoptExisting {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionAdoptExisting)
	}
	if plan.Summary.AdoptCount != 1 {
		t.Errorf("summary.AdoptCount = %d, want 1", plan.Summary.AdoptCount)
	}
}

func TestComputeChecksumMismatch_Reprocess(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/types/t1", "types", "CREATE TYPE r.t1 AS TABLE (a INT)")
	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/types/t1": {SchemaName: "r", Kind: "types", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{"r/types/t1": "mismatched_checksum"}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionReprocessChanged {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionReprocessChanged)
	}
	if plan.Summary.ChangedCount != 1 {
		t.Errorf("summary.ChangedCount = %d, want 1", plan.Summary.ChangedCount)
	}
}

func TestComputeTableChangedWithoutTransition_Blocked(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/tables/t1", "tables", "CREATE TABLE r.t1 (id INT)")
	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{"r/tables/t1": "old_checksum"}

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
	if plan.Summary.BlockedCount != 1 {
		t.Errorf("summary.BlockedCount = %d, want 1", plan.Summary.BlockedCount)
	}
	if plan.Summary.ChangedCount != 1 {
		t.Errorf("summary.ChangedCount = %d, want 1", plan.Summary.ChangedCount)
	}
}

func TestComputeTableChangedWithTransition_NotBlocked(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/tables/t1", "tables", "CREATE TABLE r.t1 (id INT)")
	layout := fs.Layout{
		Objects: []*fs.Object{obj},
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
	checksums := map[string]string{"r/tables/t1": "old_checksum"}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Blocked {
		t.Fatal("expected plan NOT to be blocked")
	}
	obj2 := plan.Objects[0]
	if obj2.PlannedAction != types.ActionReprocessChanged {
		t.Errorf("action = %q, want %q", obj2.PlannedAction, types.ActionReprocessChanged)
	}
	if len(obj2.TransitionPaths) != 1 {
		t.Fatalf("expected 1 transition path, got %d", len(obj2.TransitionPaths))
	}
	if obj2.TransitionPaths[0] != "r/tables/_migrations/t1/001_a_add_col.sql" {
		t.Errorf("transition path = %q", obj2.TransitionPaths[0])
	}
}

func TestComputeViewChanged_UpdateModule(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/views/v1", "views", "CREATE VIEW r.v1 AS SELECT 2 AS x")
	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/views/v1": {SchemaName: "r", Kind: "views", ObjectName: "v1"},
		},
	}
	checksums := map[string]string{"r/views/v1": "old_checksum"}

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
	obj := makeTempObject(t, "r/views/v1", "views", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	layout := fs.Layout{
		Objects: []*fs.Object{obj},
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
	obj := makeTempObject(t, "r/tables/t1", "tables", "CREATE TABLE r.t1 (id INT)")
	layout := fs.Layout{
		Objects: []*fs.Object{obj},
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
	checksums := map[string]string{"r/tables/t1": "old_checksum"}

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
	obj := makeTempObject(t, "r/triggers/trg1", "triggers", "CREATE TRIGGER r.trg1 ON r.t1 AFTER INSERT AS SELECT 1")
	obj.SchemaName = "r"
	obj.ParentName = "missing_parent"
	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/triggers/trg1": {SchemaName: "r", Kind: "triggers", ObjectName: "trg1"},
		},
	}
	checksums := map[string]string{"r/triggers/trg1": "old_checksum"}

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
	obj := makeTempObject(t, "r/triggers/trg1", "triggers", "CREATE TRIGGER r.trg1 ON r.t1 AFTER INSERT AS SELECT 1")
	obj.SchemaName = "r"
	obj.ParentName = "t1"
	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/triggers/trg1": {SchemaName: "r", Kind: "triggers", ObjectName: "trg1"},
			"r/tables/t1":     {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{
		"r/triggers/trg1": "old_checksum",
		"r/tables/t1":     "abc1234",
	}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Objects[0].PlannedAction != types.ActionUpdateExistingModule {
		t.Errorf("action = %q, want %q", plan.Objects[0].PlannedAction, types.ActionUpdateExistingModule)
	}
}

func TestComputeBlockedHasReason(t *testing.T) {
	computer := NewComputer()
	obj := makeTempObject(t, "r/tables/t1", "tables", "CREATE TABLE r.t1 (id INT)")
	layout := fs.Layout{Objects: []*fs.Object{obj}}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
		},
	}
	checksums := map[string]string{"r/tables/t1": "old_checksum"}

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

func TestComputeMultipleObjectsSummary(t *testing.T) {
	computer := NewComputer()
	obj1 := makeTempObject(t, "r/tables/t1", "tables", "CREATE TABLE r.t1 (id INT)")
	obj2 := makeTempObject(t, "r/views/v1", "views", "CREATE VIEW r.v1 AS SELECT 1 AS x")
	cs1, _ := obj1.Checksum()

	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r"}},
		Objects: []*fs.Object{obj1, obj2},
	}
	state := &db.State{
		Objects: map[string]db.Object{
			"r/tables/t1": {SchemaName: "r", Kind: "tables", ObjectName: "t1"},
			"r/views/v1":  {SchemaName: "r", Kind: "views", ObjectName: "v1"},
		},
	}
	checksums := map[string]string{
		"r/tables/t1": cs1,
		"r/views/v1":  "mismatched",
	}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Summary.SchemaCount != 1 {
		t.Errorf("SchemaCount = %d, want 1", plan.Summary.SchemaCount)
	}
	if plan.Summary.ObjectCount != 2 {
		t.Errorf("ObjectCount = %d, want 2", plan.Summary.ObjectCount)
	}
	if plan.Summary.SkipCount != 1 {
		t.Errorf("SkipCount = %d, want 1", plan.Summary.SkipCount)
	}
	if plan.Summary.ChangedCount != 1 {
		t.Errorf("ChangedCount = %d, want 1", plan.Summary.ChangedCount)
	}
}

func makeTempObject(t *testing.T, key, kind, content string) *fs.Object {
	t.Helper()
	dir := t.TempDir()
	relPath := key + ".sql"
	absPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}
	return &fs.Object{
		Path:          relPath,
		CachedFile:    fs.CachedFile{AbsPath: absPath},
		NormalizedKey: key,
		Kind:          kind,
	}
}

func TestCompute_OrphanedDBObject_NotInLayout(t *testing.T) {
	computer := NewComputer()
	layout := fs.Layout{
		Schemas: []fs.Schema{{Name: "r", NormalizedName: "r"}},
		Objects: []*fs.Object{},
	}
	state := &db.State{
		Schemas: map[string]struct{}{"r": {}},
		Objects: map[string]db.Object{
			"r/views/orphan": {SchemaName: "r", Kind: "views", ObjectName: "orphan"},
		},
	}
	checksums := map[string]string{}

	plan, err := computer.Compute(context.Background(), layout, state, checksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Blocked {
		t.Error("orphaned DB object should not block the plan")
	}
	for _, obj := range plan.Objects {
		if obj.NormalizedKey == "r/views/orphan" {
			t.Error("orphaned DB object should not appear in plan objects")
		}
	}
}
