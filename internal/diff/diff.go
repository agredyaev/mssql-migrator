package diff

import (
	"context"
	"strings"
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
			plannedObj.Checksum = getLayoutChecksum(layout, obj.NormalizedKey)
			createCount++

		case exists && checksums[obj.NormalizedKey] == "":
			plannedObj.PlannedAction = types.ActionAdoptExisting
			plannedObj.Checksum = getLayoutChecksum(layout, obj.NormalizedKey)
			adoptCount++

		case exists && checksumsMatch(layout, obj.NormalizedKey, checksums[obj.NormalizedKey]):
			plannedObj.PlannedAction = types.ActionSkipUnchanged
			plannedObj.Checksum = checksums[obj.NormalizedKey]
			skipCount++

		default:
			changedCount++
			currentChecksum := getLayoutChecksum(layout, obj.NormalizedKey)
			plannedObj.Checksum = currentChecksum
			c.handleChanged(obj, dbObj, &plannedObj, plan, state, layout, checksums, &blockedCount)
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

func getLayoutChecksum(layout fs.Layout, key string) string {
	for _, obj := range layout.Objects {
		if obj.NormalizedKey == key {
			cs, err := obj.Checksum()
			if err == nil {
				return cs
			}
			return ""
		}
	}
	return ""
}

func checksumsMatch(layout fs.Layout, key, prior string) bool {
	current := getLayoutChecksum(layout, key)
	return current != "" && current == prior
}

func (c *Computer) handleChanged(
	obj *fs.Object, dbObj db.Object,
	plannedObj *types.PlannedObject,
	plan *types.MigrationPlan,
	state *db.State,
	layout fs.Layout,
	checksums map[string]string,
	blockedCount *int,
) {
	switch {
	case obj.Kind == "tables":
		transitions := layoutTransitions(layout, obj.NormalizedKey)
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
		parentKey := strings.Join([]string{obj.SchemaName, "tables", obj.ParentName}, "/")
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
		if isModuleKind(obj.Kind) {
			plannedObj.PlannedAction = types.ActionUpdateExistingModule
		} else {
			plannedObj.PlannedAction = types.ActionReprocessChanged
		}
	}
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
