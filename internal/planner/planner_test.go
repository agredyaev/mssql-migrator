package planner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

func TestBuildPlansCreateAndChangedObjects(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)

	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}
	migrationState := map[string]string{"reporting/views/monthly": "old"}

	plan, err := Build(cfg, migrationState)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Schemas) != 1 {
		t.Fatalf("expected one schema, got %d", len(plan.Schemas))
	}
	if len(plan.Objects) != 2 {
		t.Fatalf("expected two objects, got %d", len(plan.Objects))
	}
	if plan.Objects[0].PlannedAction == "" || plan.Objects[1].PlannedAction == "" {
		t.Fatalf("expected planned actions, got %#v", plan.Objects)
	}
}

func TestBuildDefaultsChangedModuleToAutoSafeUpdate(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)
	path := filepath.Join(root, base, "reporting", "views", "monthly.sql")
	writeSQL(t, path, "CREATE OR ALTER VIEW reporting.monthly AS SELECT 1;")
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}
	migrationState := map[string]string{"reporting/views/monthly": currentChecksum + "changed"}

	plan, err := BuildWithCatalog(context.Background(), cfg, migrationState, stubCatalogReader{objects: map[string]CatalogObject{"reporting/views/monthly": {SchemaName: "reporting", Kind: "views", ObjectName: "monthly"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range plan.Objects {
		if object.NormalizedKey == "reporting/views/monthly" && object.PlannedAction != contracts.ActionUpdateExistingModule {
			t.Fatalf("expected default module update action, got %#v", object)
		}
	}
}

func TestBuildBlocksChangedExistingNonModuleObject(t *testing.T) {
	root := t.TempDir()
	base := createTableLayout(t, root)
	path := filepath.Join(root, base, "reporting", "tables", "snapshot.sql")

	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	migrationState := map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}

	plan, err := BuildWithCatalog(context.Background(), cfg, migrationState, stubCatalogReader{objects: map[string]CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Blocked {
		t.Fatal("expected plan to be blocked")
	}
}

func TestBuildUsesCheckedInTransitionForChangedTable(t *testing.T) {
	root := t.TempDir()
	base := createTableLayout(t, root)
	path := filepath.Join(root, base, "reporting", "tables", "snapshot.sql")
	writeSQL(t, filepath.Join(root, base, "reporting", "tables", "_migrations", "snapshot", "001_deadbee_expand_snapshot.sql"), "ALTER TABLE reporting.snapshot ADD name nvarchar(100) NULL;")
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}

	layout, hash, err := resolvePlanningLayout(config.Config{SQLRoot: root, SQLBase: base})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildResolved(context.Background(), config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, layout, hash, stubCatalogReader{objects: map[string]CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Blocked {
		t.Fatalf("expected transition-backed table change not to block, got %#v", plan.BlockReasons)
	}
	for _, object := range plan.Objects {
		if object.NormalizedKey == "reporting/tables/snapshot" {
			if object.PlannedAction != contracts.ActionReprocessChanged {
				t.Fatalf("expected reprocess_changed, got %#v", object)
			}
			if len(object.TransitionPaths) != 1 {
				t.Fatalf("expected one transition path, got %#v", object)
			}
		}
	}
}

func TestBuildRequiresTransitionForChangedTable(t *testing.T) {
	root := t.TempDir()
	base := createTableLayout(t, root)
	path := filepath.Join(root, base, "reporting", "tables", "snapshot.sql")
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildWithCatalog(context.Background(), config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, stubCatalogReader{objects: map[string]CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Blocked {
		t.Fatal("expected missing transition to block")
	}
	if len(plan.BlockReasons) == 0 || !strings.Contains(plan.BlockReasons[0], "tracked table drift detected") {
		t.Fatalf("expected transition-required message, got %#v", plan.BlockReasons)
	}
}

func TestBuildBlocksChangedTableWhenOnlyScaffoldTransitionExists(t *testing.T) {
	root := t.TempDir()
	base := createTableLayout(t, root)
	path := filepath.Join(root, base, "reporting", "tables", "snapshot.sql")
	writeSQL(t, filepath.Join(root, base, "reporting", "tables", "_migrations", "snapshot", "001_deadbee_describe_change.sql"), parser.TransitionScaffoldDirective+"\n")
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildWithCatalog(context.Background(), config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}, map[string]string{"reporting/tables/snapshot": currentChecksum + "changed"}, stubCatalogReader{objects: map[string]CatalogObject{"reporting/tables/snapshot": {SchemaName: "reporting", Kind: "tables", ObjectName: "snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Blocked {
		t.Fatal("expected scaffold-only transition to keep plan blocked")
	}
	if len(plan.BlockReasons) == 0 || !strings.Contains(plan.BlockReasons[0], "Replace the scaffold") {
		t.Fatalf("expected scaffold blocker, got %#v", plan.BlockReasons)
	}
	for _, object := range plan.Objects {
		if object.NormalizedKey == "reporting/tables/snapshot" && len(object.TransitionPaths) != 1 {
			t.Fatalf("expected scaffold transition path in plan, got %#v", object)
		}
	}
}

func TestBuildUpdatesChangedModuleWhenPolicyAllows(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)
	path := filepath.Join(root, base, "reporting", "views", "monthly.sql")
	writeSQL(t, path, "CREATE OR ALTER VIEW reporting.monthly AS SELECT 1;")
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}
	migrationState := map[string]string{"reporting/views/monthly": currentChecksum + "changed"}

	plan, err := BuildWithCatalog(context.Background(), cfg, migrationState, stubCatalogReader{objects: map[string]CatalogObject{"reporting/views/monthly": {SchemaName: "reporting", Kind: "views", ObjectName: "monthly"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range plan.Objects {
		if object.NormalizedKey == "reporting/views/monthly" && object.PlannedAction != contracts.ActionUpdateExistingModule {
			t.Fatalf("expected update_existing_module, got %#v", object)
		}
	}
}

func TestBuildBlocksChangedModuleWithoutCreateOrAlter(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)
	path := filepath.Join(root, base, "reporting", "views", "monthly.sql")
	writeSQL(t, path, "CREATE VIEW reporting.monthly AS SELECT 1;")
	currentChecksum, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyModulesOnly, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}
	migrationState := map[string]string{"reporting/views/monthly": currentChecksum + "changed"}

	plan, err := BuildWithCatalog(context.Background(), cfg, migrationState, stubCatalogReader{objects: map[string]CatalogObject{"reporting/views/monthly": {SchemaName: "reporting", Kind: "views", ObjectName: "monthly"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Blocked {
		t.Fatal("expected unsafe module update to block plan")
	}
	if len(plan.BlockReasons) == 0 || !strings.Contains(plan.BlockReasons[0], "tracked view drift detected") {
		t.Fatalf("expected CREATE OR ALTER block reason, got %#v", plan.BlockReasons)
	}
	for _, object := range plan.Objects {
		if object.NormalizedKey == "reporting/views/monthly" && object.PlannedAction != contracts.ActionReprocessChangedBlocked {
			t.Fatalf("expected blocked changed action, got %#v", object)
		}
	}
}

func TestBuildPreservesNoTransactionRollbackScope(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	writeSQL(t, filepath.Join(root, base, "reporting", "views", "monthly.sql"), "-- migrator: no-transaction\nSELECT 1;")
	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}

	plan, err := Build(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Objects) != 1 {
		t.Fatalf("expected one object, got %#v", plan.Objects)
	}
	if plan.Objects[0].TransactionMode != config.TransactionModeNone || plan.Objects[0].RollbackScope != "none" {
		t.Fatalf("unexpected no-transaction plan object: %#v", plan.Objects[0])
	}
}

func TestBuildAllowsAdoptExistingWithoutBlocker(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)
	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}

	plan, err := BuildWithCatalog(context.Background(), cfg, nil, stubCatalogReader{objects: map[string]CatalogObject{"reporting/views/monthly": {SchemaName: "reporting", Kind: "views", ObjectName: "monthly"}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Blocked {
		t.Fatalf("expected adopt_existing not to block plan, got %#v", plan.BlockReasons)
	}
	for _, object := range plan.Objects {
		if object.NormalizedKey == "reporting/views/monthly" && object.PlannedAction != contracts.ActionAdoptExisting {
			t.Fatalf("expected adopt_existing action, got %#v", object)
		}
	}
}

func TestBuildLayoutHashIgnoresValidationChecks(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)
	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}

	planBefore, err := Build(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeSQL(t, filepath.Join(root, base, "reporting", "checks", "smoke.sql"), "SELECT 1;")
	planAfter, err := Build(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if planBefore.LayoutHash != planAfter.LayoutHash {
		t.Fatalf("expected checks to be excluded from planning layout hash: %s != %s", planBefore.LayoutHash, planAfter.LayoutHash)
	}
}

func TestVerifyApprovedPlanRejectsDifferentObjects(t *testing.T) {
	root := t.TempDir()
	planFile := filepath.Join(root, "plan.json")
	approved := contracts.MigrationPlan{GitCommit: "abc", LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef", Objects: []contracts.PlannedObject{{ObjectPath: "reporting/views/monthly.sql", Kind: "views", ObjectName: "monthly", SchemaName: "reporting", NormalizedKey: "reporting/views/monthly", Checksum: "x", PlannedAction: contracts.ActionCreateObject}}}
	content, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PlanFile: planFile, GitCommit: "abc"}
	current := contracts.MigrationPlan{LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef", Objects: []contracts.PlannedObject{{ObjectPath: "reporting/views/daily.sql", Kind: "views", ObjectName: "daily", SchemaName: "reporting", NormalizedKey: "reporting/views/daily", Checksum: "y", PlannedAction: contracts.ActionCreateObject}}}
	if err := VerifyApprovedPlan(cfg, current); err == nil {
		t.Fatal("expected object mismatch error")
	} else if !strings.Contains(err.Error(), contracts.ErrApprovedPlanMismatch.Error()) {
		t.Fatalf("expected approved plan mismatch sentinel, got %v", err)
	}
}

func TestVerifyApprovedPlanRejectsWrongSchemaVersion(t *testing.T) {
	root := t.TempDir()
	planFile := filepath.Join(root, "plan.json")
	approved := contracts.MigrationPlan{SchemaVersion: "v7", Command: contracts.CommandPlan, GitCommit: "abc", LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef"}
	content, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PlanFile: planFile, GitCommit: "abc"}
	current := contracts.MigrationPlan{SchemaVersion: "v8", Command: contracts.CommandPlan, LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef"}
	if err := VerifyApprovedPlan(cfg, current); err == nil {
		t.Fatal("expected schema version mismatch")
	} else if !strings.Contains(err.Error(), contracts.ErrApprovedPlanMismatch.Error()) {
		t.Fatalf("expected approved plan mismatch sentinel, got %v", err)
	}
}

func TestVerifyApprovedPlanMissingUsesSentinel(t *testing.T) {
	cfg := config.Config{PlanFile: filepath.Join(t.TempDir(), "missing.json")}
	err := VerifyApprovedPlan(cfg, contracts.MigrationPlan{})
	if err == nil {
		t.Fatal("expected missing plan error")
	}
	if !strings.Contains(err.Error(), contracts.ErrApprovedPlanMissing.Error()) {
		t.Fatalf("expected approved plan missing sentinel, got %v", err)
	}
}

func TestVerifyApprovedPlanRejectsApprovalBoundaryDrift(t *testing.T) {
	root := t.TempDir()
	planFile := filepath.Join(root, "plan.json")
	approved := contracts.MigrationPlan{
		SchemaVersion:     "v8",
		Command:           contracts.CommandPlan,
		GitCommit:         "abc",
		LayoutHash:        "hash",
		SQLRoot:           "/sql",
		Base:              "dwh",
		EffectiveBasePath: "/sql/dwh",
		Target:            contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"},
		ToolVersion:       "4.0.0",
		ToolCommit:        "deadbeef",
		ComparisonMode:    config.ComparisonModeCaseInsensitive,
		UpdatePolicy:      config.UpdatePolicyNone,
		TransactionMode:   config.TransactionModeScript,
		Rollback:          "script",
	}
	content, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PlanFile: planFile, GitCommit: "abc"}
	current := approved
	current.UpdatePolicy = config.UpdatePolicyModulesOnly
	current.TransactionMode = config.TransactionModeNone
	current.Rollback = "none"
	if err := VerifyApprovedPlan(cfg, current); err == nil {
		t.Fatal("expected approval boundary mismatch")
	} else if !strings.Contains(err.Error(), "update_policy") || !strings.Contains(err.Error(), "transaction_mode") || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("unexpected mismatch error: %v", err)
	}
}

func TestVerifyApprovedPlanUsesCurrentPlanWhenPlanFileNotSet(t *testing.T) {
	current := contracts.MigrationPlan{SchemaVersion: "v8", Command: contracts.CommandPlan}
	if err := VerifyApprovedPlan(config.Config{}, current); err == nil {
		t.Fatal("expected missing plan file sentinel in file approval mode")
	} else if !strings.Contains(err.Error(), contracts.ErrApprovedPlanMissing.Error()) {
		t.Fatalf("expected approved plan missing sentinel, got %v", err)
	}
}

func TestMapTypeDescToKindSupportsUserTableType(t *testing.T) {
	if got := catalog.MapTypeDescToKind("USER_TABLE_TYPE"); got != "types" {
		t.Fatalf("unexpected type mapping: %q", got)
	}
}

func TestCatalogStateQueryIncludesViewIndexes(t *testing.T) {
	if !strings.Contains(catalog.StateQuery, "o.type IN ('U', 'V')") {
		t.Fatalf("expected catalog query to include view-backed indexes: %s", catalog.StateQuery)
	}
	if strings.Contains(catalog.StateQuery, "JOIN sys.tables") {
		t.Fatalf("expected catalog query not to be limited to sys.tables: %s", catalog.StateQuery)
	}
}

func TestStableObjectsDropsStateDerivedFields(t *testing.T) {
	items := []contracts.PlannedObject{{
		ObjectPath:      "reporting/views/monthly.sql",
		SchemaName:      "reporting",
		Kind:            "views",
		ObjectName:      "monthly",
		NormalizedKey:   "reporting/views/monthly",
		Checksum:        "hash",
		PlannedAction:   contracts.ActionCreateObject,
		Exists:          true,
		MetadataMatch:   boolPtr(true),
		TransactionMode: config.TransactionModeScript,
		RollbackScope:   "script",
		SourceFile:      "reporting/views/monthly.sql",
	}}
	stable := stableObjects(items)
	expected := []contracts.PlannedObject{{
		ObjectPath:      "reporting/views/monthly.sql",
		SchemaName:      "reporting",
		Kind:            "views",
		ObjectName:      "monthly",
		NormalizedKey:   "reporting/views/monthly",
		Checksum:        "hash",
		PlannedAction:   contracts.ActionCreateObject,
		TransactionMode: config.TransactionModeScript,
		RollbackScope:   "script",
		SourceFile:      "reporting/views/monthly.sql",
	}}
	if !reflect.DeepEqual(stable, expected) {
		t.Fatalf("unexpected stable object payload: %#v", stable)
	}
}

type stubCatalogReader struct {
	schemas      map[string]struct{}
	objects      map[string]CatalogObject
	tableColumns map[string][]catalog.TableColumn
}

func (s stubCatalogReader) ReadCatalogState(_ context.Context) (CatalogState, error) {
	return CatalogState{Schemas: s.schemas, Objects: s.objects, TableColumns: s.tableColumns, SuccessfulByKey: map[string]string{}}, nil
}

func createLayout(t *testing.T, root string) string {
	t.Helper()
	base := "dwh"
	writeSQL(t, filepath.Join(root, base, "reporting", "views", "monthly.sql"), "CREATE VIEW reporting.monthly AS SELECT 1;")
	writeSQL(t, filepath.Join(root, base, "reporting", "procedures", "refresh.sql"), "SELECT 2;")
	return base
}

func createTableLayout(t *testing.T, root string) string {
	t.Helper()
	base := "dwh"
	writeSQL(t, filepath.Join(root, base, "reporting", "tables", "snapshot.sql"), "CREATE TABLE reporting.snapshot(id int);")
	return base
}

func writeSQL(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
