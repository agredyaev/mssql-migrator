package migrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
)

func TestEnsureTableTransitionFilesCreatesMissingTableScaffold(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	basePath := filepath.Join(root, base)
	createRepoObject(t, root, base, "reporting", "tables", "snapshot.sql", "CREATE TABLE reporting.snapshot(id int);")
	layout, err := parser.DiscoverLayout(basePath)
	if err != nil {
		t.Fatal(err)
	}
	created, err := ensureTableTransitionFiles(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, GitCommit: "deadbeefcafebabe"}, layout, contracts.MigrationPlan{Objects: []contracts.PlannedObject{{
		ObjectPath:    "reporting/tables/snapshot.sql",
		SchemaName:    "reporting",
		ObjectName:    "snapshot",
		Kind:          "tables",
		NormalizedKey: "reporting/tables/snapshot",
		PlannedAction: contracts.ActionReprocessChangedBlocked,
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected scaffold creation")
	}
	scaffoldPath := filepath.Join(basePath, "reporting", "tables", "_migrations", "snapshot", "001_deadbee_describe_change.sql")
	content, err := os.ReadFile(scaffoldPath)
	if err != nil {
		t.Fatalf("expected scaffold file: %v", err)
	}
	for _, expected := range []string{parser.TransitionScaffoldDirective, "Replace this scaffold", "Commit token: deadbee"} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected scaffold content %q, got %s", expected, string(content))
		}
	}
}

func TestEnsureTableTransitionFilesDoesNotOverwriteExistingTransition(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	basePath := filepath.Join(root, base)
	createRepoObject(t, root, base, "reporting", "tables", "snapshot.sql", "CREATE TABLE reporting.snapshot(id int);")
	path := filepath.Join(root, base, "reporting", "tables", "_migrations", "snapshot")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir transition dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "001_deadbee_expand_snapshot.sql"), []byte("ALTER TABLE reporting.snapshot ADD name nvarchar(100) NULL;"), 0o644); err != nil {
		t.Fatalf("write transition file: %v", err)
	}
	layout, err := parser.DiscoverLayout(basePath)
	if err != nil {
		t.Fatal(err)
	}
	created, err := ensureTableTransitionFiles(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, GitCommit: "deadbeefcafebabe"}, layout, contracts.MigrationPlan{Objects: []contracts.PlannedObject{{
		ObjectPath:      "reporting/tables/snapshot.sql",
		SchemaName:      "reporting",
		ObjectName:      "snapshot",
		Kind:            "tables",
		PlannedAction:   contracts.ActionReprocessChanged,
		TransitionPaths: []string{layout.Transitions[0].Path},
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected existing transition to suppress scaffold creation")
	}
	if _, err := os.Stat(filepath.Join(basePath, "reporting", "tables", "_migrations", "snapshot", "001_deadbee_describe_change.sql")); !os.IsNotExist(err) {
		t.Fatalf("expected no scaffold next to existing transition, got %v", err)
	}
}

func TestEnsureTableTransitionFilesAutoGeneratesAddColumnMigration(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	basePath := filepath.Join(root, base)
	createRepoObject(t, root, base, "reporting", "tables", "snapshot.sql", "CREATE TABLE reporting.snapshot(id int NOT NULL, name nvarchar(100) NULL);")
	layout, err := parser.DiscoverLayout(basePath)
	if err != nil {
		t.Fatal(err)
	}
	created, err := ensureTableTransitionFiles(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, GitCommit: "deadbeefcafebabe"}, layout, contracts.MigrationPlan{Objects: []contracts.PlannedObject{{
		ObjectPath:    "reporting/tables/snapshot.sql",
		SchemaName:    "reporting",
		ObjectName:    "snapshot",
		Kind:          "tables",
		NormalizedKey: "reporting/tables/snapshot",
		PlannedAction: contracts.ActionReprocessChangedBlocked,
	}}}, map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}}})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected automatic checked-in migration creation")
	}
	path := filepath.Join(basePath, "reporting", "tables", "_migrations", "snapshot", "001_deadbee_auto_add_columns.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected auto-generated migration file: %v", err)
	}
	if !strings.Contains(string(content), "ALTER TABLE [reporting].[snapshot] ADD name nvarchar(100) NULL;") {
		t.Fatalf("expected additive column migration SQL, got %s", string(content))
	}
}

func TestEnsureTableTransitionFilesReplanUsesGeneratedAddColumnMigration(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	basePath := filepath.Join(root, base)
	createRepoObject(t, root, base, "reporting", "tables", "snapshot.sql", "CREATE TABLE reporting.snapshot(id int NOT NULL, name nvarchar(100) NULL);")
	layout, hash, err := planner.ResolvePlanningLayoutForRunner(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath})
	if err != nil {
		t.Fatal(err)
	}
	currentChecksum := layout.Objects[0].Checksum
	plan, err := planner.BuildResolved(context.Background(), config.Config{Env: "pred", Database: "db", SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, UpdatePolicy: config.UpdatePolicyModulesOnly}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, layout, hash, scaffoldCatalogReader{objects: map[string]planner.CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}, tableColumns: map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}}}})
	if err != nil {
		t.Fatal(err)
	}
	if created, err := ensureTableTransitionFiles(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, GitCommit: "deadbeefcafebabe"}, layout, plan, map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}}}); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("expected generated checked-in migration file")
	}
	layout, hash, err = planner.ResolvePlanningLayoutForRunner(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = planner.BuildResolved(context.Background(), config.Config{Env: "pred", Database: "db", SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, UpdatePolicy: config.UpdatePolicyModulesOnly}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, layout, hash, scaffoldCatalogReader{objects: map[string]planner.CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}, tableColumns: map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}, {Name: "name", NormalizedName: "name", TypeName: "nvarchar", Length: 100, Nullable: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Blocked {
		t.Fatalf("expected generated migration to unblock plan, got %#v", plan.BlockReasons)
	}
	if len(plan.Objects) != 1 || plan.Objects[0].PlannedAction != contracts.ActionReprocessChanged {
		t.Fatalf("expected transition-backed reprocess plan, got %#v", plan.Objects)
	}
	if len(plan.Objects[0].TransitionPaths) != 1 || !strings.Contains(plan.Objects[0].TransitionPaths[0], "001_deadbee_auto_add_columns.sql") {
		t.Fatalf("expected generated transition path in replanned object, got %#v", plan.Objects[0])
	}
}

func TestEnsureTableTransitionFilesReplanKeepsScaffoldBlocked(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	basePath := filepath.Join(root, base)
	createRepoObject(t, root, base, "reporting", "tables", "snapshot.sql", "CREATE TABLE reporting.snapshot(id int NOT NULL, name nvarchar(100) NOT NULL);")
	layout, hash, err := planner.ResolvePlanningLayoutForRunner(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath})
	if err != nil {
		t.Fatal(err)
	}
	currentChecksum := layout.Objects[0].Checksum
	plan, err := planner.BuildResolved(context.Background(), config.Config{Env: "pred", Database: "db", SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, UpdatePolicy: config.UpdatePolicyModulesOnly}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, layout, hash, scaffoldCatalogReader{objects: map[string]planner.CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}, tableColumns: map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}}}})
	if err != nil {
		t.Fatal(err)
	}
	if created, err := ensureTableTransitionFiles(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, GitCommit: "deadbeefcafebabe"}, layout, plan, map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}}}); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("expected scaffold generation")
	}
	layout, hash, err = planner.ResolvePlanningLayoutForRunner(config.Config{SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = planner.BuildResolved(context.Background(), config.Config{Env: "pred", Database: "db", SQLRoot: root, SQLBase: base, EffectiveBasePath: basePath, UpdatePolicy: config.UpdatePolicyModulesOnly}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, layout, hash, scaffoldCatalogReader{objects: map[string]planner.CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}, tableColumns: map[string][]catalog.TableColumn{"reporting/tables/snapshot": {{Name: "id", NormalizedName: "id", TypeName: "int", Nullable: false}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Blocked {
		t.Fatal("expected scaffold-backed replan to remain blocked")
	}
	if len(plan.Objects) != 1 || len(plan.Objects[0].TransitionPaths) != 1 || !strings.Contains(plan.Objects[0].TransitionPaths[0], "001_deadbee_describe_change.sql") {
		t.Fatalf("expected scaffold transition path in replanned object, got %#v", plan.Objects)
	}
	if len(plan.BlockReasons) == 0 || !strings.Contains(plan.BlockReasons[0], "auto-created scaffold") {
		t.Fatalf("expected scaffold blocker after replan, got %#v", plan.BlockReasons)
	}
}

type scaffoldCatalogReader struct {
	objects      map[string]planner.CatalogObject
	tableColumns map[string][]catalog.TableColumn
}

func (s scaffoldCatalogReader) ReadCatalogState(_ context.Context) (planner.CatalogState, error) {
	return planner.CatalogState{
		Schemas:         map[string]struct{}{},
		Objects:         s.objects,
		TableColumns:    s.tableColumns,
		SuccessfulByKey: map[string]string{},
	}, nil
}
