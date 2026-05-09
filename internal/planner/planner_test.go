package planner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
)

func TestBuildPlansCreateAndChangedObjects(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)

	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLRoot: root, SQLBase: base, EffectiveBasePath: filepath.Join(root, base), ToolVersion: "4.0.0", UpdatePolicy: config.UpdatePolicyNone, TransactionMode: config.TransactionModeScript, ComparisonMode: config.ComparisonModeCaseInsensitive}
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

func TestBuildUpdatesChangedModuleWhenPolicyAllows(t *testing.T) {
	root := t.TempDir()
	base := createLayout(t, root)
	path := filepath.Join(root, base, "reporting", "views", "monthly.sql")
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
		if object.NormalizedKey == "reporting/views/monthly" && object.PlannedAction != "update_existing_module" {
			t.Fatalf("expected update_existing_module, got %#v", object)
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
		if object.NormalizedKey == "reporting/views/monthly" && object.PlannedAction != "adopt_existing" {
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
	approved := contracts.MigrationPlan{GitCommit: "abc", LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef", Objects: []contracts.PlannedObject{{ObjectPath: "reporting/views/monthly.sql", Kind: "views", ObjectName: "monthly", SchemaName: "reporting", NormalizedKey: "reporting/views/monthly", Checksum: "x", PlannedAction: "create_object"}}}
	content, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PlanFile: planFile, GitCommit: "abc"}
	current := contracts.MigrationPlan{LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef", Objects: []contracts.PlannedObject{{ObjectPath: "reporting/views/daily.sql", Kind: "views", ObjectName: "daily", SchemaName: "reporting", NormalizedKey: "reporting/views/daily", Checksum: "y", PlannedAction: "create_object"}}}
	if err := VerifyApprovedPlan(cfg, current); err == nil {
		t.Fatal("expected object mismatch error")
	}
}

func TestVerifyApprovedPlanRejectsWrongSchemaVersion(t *testing.T) {
	root := t.TempDir()
	planFile := filepath.Join(root, "plan.json")
	approved := contracts.MigrationPlan{SchemaVersion: "v7", Command: "plan", GitCommit: "abc", LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef"}
	content, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PlanFile: planFile, GitCommit: "abc"}
	current := contracts.MigrationPlan{SchemaVersion: "v8", Command: "plan", LayoutHash: "hash", SQLRoot: "/sql", Base: "dwh", EffectiveBasePath: "/sql/dwh", Target: contracts.PlanTarget{Environment: "prod", Database: "ReportingDB"}, ToolVersion: "4.0.0", ToolCommit: "deadbeef"}
	if err := VerifyApprovedPlan(cfg, current); err == nil {
		t.Fatal("expected schema version mismatch")
	}
}

func TestVerifyApprovedPlanRejectsApprovalBoundaryDrift(t *testing.T) {
	root := t.TempDir()
	planFile := filepath.Join(root, "plan.json")
	approved := contracts.MigrationPlan{
		SchemaVersion:     "v8",
		Command:           "plan",
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

func TestMapTypeDescToKindSupportsUserTableType(t *testing.T) {
	if got := mapTypeDescToKind("USER_TABLE_TYPE"); got != "types" {
		t.Fatalf("unexpected type mapping: %q", got)
	}
}

func TestCatalogStateQueryIncludesViewIndexes(t *testing.T) {
	if !strings.Contains(catalogStateQuery, "o.type IN ('U', 'V')") {
		t.Fatalf("expected catalog query to include view-backed indexes: %s", catalogStateQuery)
	}
	if strings.Contains(catalogStateQuery, "JOIN sys.tables") {
		t.Fatalf("expected catalog query not to be limited to sys.tables: %s", catalogStateQuery)
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
		PlannedAction:   "create_object",
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
		PlannedAction:   "create_object",
		TransactionMode: config.TransactionModeScript,
		RollbackScope:   "script",
		SourceFile:      "reporting/views/monthly.sql",
	}}
	if !reflect.DeepEqual(stable, expected) {
		t.Fatalf("unexpected stable object payload: %#v", stable)
	}
}

type stubCatalogReader struct {
	schemas map[string]struct{}
	objects map[string]CatalogObject
}

func (s stubCatalogReader) ReadCatalogState(_ context.Context) (CatalogState, error) {
	return CatalogState{Schemas: s.schemas, Objects: s.objects, SuccessfulByKey: map[string]string{}}, nil
}

func createLayout(t *testing.T, root string) string {
	t.Helper()
	base := "dwh"
	writeSQL(t, filepath.Join(root, base, "reporting", "views", "monthly.sql"), "SELECT 1;")
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
