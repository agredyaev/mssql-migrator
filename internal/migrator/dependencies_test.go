package migrator

import (
	"testing"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

func TestObjectDependencyResolverParentCandidates(t *testing.T) {
	resolver := objectDependencyResolver{}
	got := resolver.ParentCandidates("Reporting", " Monthly ")
	want := []string{"reporting/tables/monthly", "reporting/views/monthly"}
	if len(got) != len(want) {
		t.Fatalf("unexpected candidates: %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected candidates: %#v", got)
		}
	}
}

func TestObjectDependencyResolverParentSatisfied(t *testing.T) {
	resolver := objectDependencyResolver{}
	plannedByKey := map[string]contracts.PlannedObject{
		"reporting/views/monthly": {NormalizedKey: "reporting/views/monthly", PlannedAction: contracts.ActionAdoptExisting},
	}
	object := parser.Object{SchemaName: "reporting", ParentName: "monthly"}
	if !resolver.ParentSatisfied(plannedByKey, object) {
		t.Fatal("expected parent to be satisfied by planned view")
	}
}

func TestObjectDependencyResolverParentNotSatisfiedForBlockedParent(t *testing.T) {
	resolver := objectDependencyResolver{}
	plannedByKey := map[string]contracts.PlannedObject{
		"reporting/tables/snapshot": {NormalizedKey: "reporting/tables/snapshot", PlannedAction: contracts.ActionUpdateExistingSupported},
	}
	object := parser.Object{SchemaName: "reporting", ParentName: "snapshot"}
	if resolver.ParentSatisfied(plannedByKey, object) {
		t.Fatal("expected blocked parent state not to satisfy dependency")
	}
}
