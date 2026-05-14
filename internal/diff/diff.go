package diff

import (
	"context"
	"strings"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type Computer struct{}

func NewComputer() *Computer {
	return &Computer{}
}

func (c *Computer) Compute(ctx context.Context, layout fs.Layout, state *db.State, checksums map[string]string) (*types.MigrationPlan, error) {
	plan := &types.MigrationPlan{}

	if state == nil {
		state = &db.State{Objects: map[string]db.Object{}}
	}
	if checksums == nil {
		checksums = map[string]string{}
	}

	for _, obj := range layout.Objects {
		dbObj, exists := state.Objects[obj.NormalizedKey]
		plannedObj := types.PlannedObject{
			ObjectPath:    obj.Path,
			SchemaName:    obj.SchemaName,
			Kind:          obj.Kind,
			ObjectName:    obj.ObjectName,
			NormalizedKey: obj.NormalizedKey,
			Exists:        exists,
		}

		switch {
		case !exists:
			plannedObj.PlannedAction = types.ActionCreateObject
		case exists && checksums[obj.NormalizedKey] != "":
			plannedObj.PlannedAction = types.ActionSkipUnchanged
			_ = dbObj
		case obj.Kind == "tables":
			transitions := layoutTransitions(layout, obj.NormalizedKey)
			if len(transitions) == 0 {
				plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
				plan.Blocked = true
				plan.Blockers = append(plan.Blockers, "table "+obj.NormalizedKey+" changed but has no non-scaffold transition scripts")
			} else {
				plannedObj.PlannedAction = types.ActionReprocessChanged
				for _, ts := range transitions {
					plannedObj.TransitionPaths = append(plannedObj.TransitionPaths, ts.Path)
				}
			}
		case obj.Kind == "triggers" && obj.ParentName != "":
			parentKey := strings.Join([]string{obj.SchemaName, "tables", obj.ParentName}, "/")
			if _, ok := state.Objects[parentKey]; !ok {
				plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
				plan.Blocked = true
				plan.Blockers = append(plan.Blockers, "trigger "+obj.NormalizedKey+" parent table "+parentKey+" not found")
			} else if checksums[parentKey] == "" {
				plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
				plan.Blocked = true
				plan.Blockers = append(plan.Blockers, "trigger "+obj.NormalizedKey+" parent table "+parentKey+" is changing")
			} else {
				plannedObj.PlannedAction = types.ActionUpdateExistingModule
			}
		default:
			if isModuleKind(obj.Kind) {
				plannedObj.PlannedAction = types.ActionUpdateExistingModule
			} else {
				plannedObj.PlannedAction = types.ActionReprocessChanged
			}
		}
		plan.Objects = append(plan.Objects, plannedObj)
	}

	return plan, nil
}

func layoutTransitions(layout fs.Layout, normalizedKey string) []*fs.TransitionScript {
	var result []*fs.TransitionScript
	for _, ts := range layout.Transitions {
		if ts.NormalizedKey == normalizedKey && !ts.Scaffold {
			result = append(result, ts)
		}
	}
	return result
}

func isModuleKind(kind string) bool {
	switch kind {
	case "views", "procedures", "functions", "triggers":
		return true
	default:
		return false
	}
}
