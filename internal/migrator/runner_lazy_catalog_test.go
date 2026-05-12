package migrator

import (
	"testing"

	"reporting-db-migrations/internal/contracts"
)

func TestTablesNeedingTransitionFiltersOnlyBlockedTables(t *testing.T) {
	plan := contracts.MigrationPlan{Objects: []contracts.PlannedObject{{Kind: "tables", PlannedAction: contracts.ActionReprocessChangedBlocked, SchemaName: "reporting", ObjectName: "snapshot"}, {Kind: "tables", PlannedAction: contracts.ActionReprocessChanged, SchemaName: "reporting", ObjectName: "daily"}, {Kind: "views", PlannedAction: contracts.ActionReprocessChangedBlocked, SchemaName: "reporting", ObjectName: "monthly"}, {Kind: "tables", PlannedAction: contracts.ActionReprocessChangedBlocked, TransitionPaths: []string{"x"}, SchemaName: "sales", ObjectName: "facts"}}}
	got := tablesNeedingTransition(plan)
	if len(got) != 1 || got[0].SchemaName != "reporting" || got[0].TableName != "snapshot" {
		t.Fatalf("tablesNeedingTransition()=%#v", got)
	}
}
