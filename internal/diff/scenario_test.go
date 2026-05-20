package diff

import (
	"testing"

	"reporting-db-migrations/internal/fs"
)

func TestResolvePlanScenarioSkip(t *testing.T) {
	cs := [32]byte{1}
	s := resolvePlanScenario(true, true, cs, cs, fs.KindCodeViews, &fs.Object{Kind: "views"}, nil, nil, nil)
	if s != ScenarioSkipUnchanged {
		t.Fatalf("got %v want skip", s)
	}
}

func TestResolvePlanScenarioCreate(t *testing.T) {
	s := resolvePlanScenario(false, false, [32]byte{}, [32]byte{1}, fs.KindCodeTables, &fs.Object{}, nil, nil, nil)
	if s != ScenarioCreate {
		t.Fatalf("got %v want create", s)
	}
}

func TestPlanScenarioAction(t *testing.T) {
	if ScenarioTableReprocess.action() != "reprocess_changed" {
		t.Fatal("table reprocess action")
	}
	if ScenarioModuleUpdate.action() != "update_existing_module" {
		t.Fatal("module action")
	}
}
