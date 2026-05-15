package diff

import (
	"context"
	"time"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type Computer struct{}

func NewComputer() *Computer {
	return &Computer{}
}

func (c *Computer) Compute(ctx context.Context, layout fs.Layout, state *db.State, checksums map[string]string) (*types.MigrationPlan, error) {
	plan := &types.MigrationPlan{
		PlannedAt: time.Now().UTC(),
	}

	if state == nil {
		state = &db.State{Objects: map[string]db.Object{}}
	}
	if checksums == nil {
		checksums = map[string]string{}
	}

	layoutChecksumMap := make(map[string]string, len(layout.Objects))
	for _, obj := range layout.Objects {
		cs, err := obj.Checksum()
		if err != nil {
			cs = ""
		}
		layoutChecksumMap[obj.NormalizedKey] = cs
	}

	transitionsByKey := make(map[string][]*fs.TransitionScript)
	for _, ts := range layout.Transitions {
		if !ts.Scaffold {
			transitionsByKey[ts.NormalizedKey] = append(transitionsByKey[ts.NormalizedKey], ts)
		}
	}

	var (
		createCount  int
		adoptCount   int
		skipCount    int
		changedCount int
		blockedCount int
	)

	for _, obj := range layout.Objects {
		dbObj, exists := state.Objects[obj.NormalizedKey]
		gitHash, _ := obj.GitHash()
		gitAuthor, _ := obj.GitAuthor()
		gitDate, _ := obj.GitDate()
		plannedObj := types.PlannedObject{
			ObjectPath:    obj.Path,
			DatabaseName:  obj.DatabaseName,
			SchemaName:    obj.SchemaName,
			Kind:          obj.Kind,
			ObjectName:    obj.ObjectName,
			NormalizedKey: obj.NormalizedKey,
			Exists:        exists,
			SourceFile:    obj.Path,
			GitHash:       gitHash,
			GitAuthor:     gitAuthor,
			GitDate:       gitDate,
		}

		switch {
		case !exists:
			plannedObj.PlannedAction = types.ActionCreateObject
			plannedObj.Checksum = layoutChecksumMap[obj.NormalizedKey]
			createCount++

		case exists && checksums[obj.NormalizedKey] == "":
			plannedObj.PlannedAction = types.ActionAdoptExisting
			plannedObj.Checksum = layoutChecksumMap[obj.NormalizedKey]
			adoptCount++

		case exists && checksumsMatch(layoutChecksumMap, obj.NormalizedKey, checksums[obj.NormalizedKey]):
			plannedObj.PlannedAction = types.ActionSkipUnchanged
			plannedObj.Checksum = checksums[obj.NormalizedKey]
			skipCount++

		default:
			changedCount++
			plannedObj.Checksum = layoutChecksumMap[obj.NormalizedKey]
			c.handleChanged(obj, dbObj, &plannedObj, plan, state, transitionsByKey, checksums, &blockedCount)
		}

		plan.Objects = append(plan.Objects, plannedObj)
	}

	plan.Summary = types.PlanSummary{
		SchemaCount:  len(layout.Schemas),
		ObjectCount:  len(layout.Objects),
		CheckCount:   len(layout.Checks),
		CreateCount:  createCount,
		AdoptCount:   adoptCount,
		SkipCount:    skipCount,
		ChangedCount: changedCount,
		BlockedCount: blockedCount,
	}

	return plan, nil
}

func checksumsMatch(layoutChecksumMap map[string]string, key, prior string) bool {
	current := layoutChecksumMap[key]
	return current != "" && current == prior
}

func (c *Computer) handleChanged(
	obj *fs.Object, dbObj db.Object,
	plannedObj *types.PlannedObject,
	plan *types.MigrationPlan,
	state *db.State,
	transitionsByKey map[string][]*fs.TransitionScript,
	checksums map[string]string,
	blockedCount *int,
) {
	switch {
	case obj.Kind == "tables":
		transitions := transitionsByKey[obj.NormalizedKey]
		if len(transitions) == 0 {
			plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			plan.Blocked = true
			plan.Blockers = append(plan.Blockers, "table "+obj.NormalizedKey+" changed but has no non-scaffold transition scripts")
			*blockedCount++
		} else {
			plannedObj.PlannedAction = types.ActionReprocessChanged
			for _, ts := range transitions {
				plannedObj.TransitionPaths = append(plannedObj.TransitionPaths, ts.Path)
			}
		}

	case obj.Kind == "triggers" && obj.ParentName != "":
		parentKey := types.NormalizedKey(obj.SchemaName, "tables", obj.ParentName)
		if _, ok := state.Objects[parentKey]; !ok {
			plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			plan.Blocked = true
			plan.Blockers = append(plan.Blockers, "trigger "+obj.NormalizedKey+" parent table "+parentKey+" not found")
			*blockedCount++
		} else if checksums[parentKey] == "" {
			plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			plan.Blocked = true
			plan.Blockers = append(plan.Blockers, "trigger "+obj.NormalizedKey+" parent table "+parentKey+" is changing")
			*blockedCount++
		} else {
			plannedObj.PlannedAction = types.ActionUpdateExistingModule
		}

	default:
		if types.IsModuleKind(obj.Kind) {
			plannedObj.PlannedAction = types.ActionUpdateExistingModule
		} else {
			plannedObj.PlannedAction = types.ActionReprocessChanged
		}
	}
}
