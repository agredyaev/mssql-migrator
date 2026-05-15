package diff

import (
	"context"
	"fmt"
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
			return nil, fmt.Errorf("checksum %s: %w", obj.NormalizedKey, err)
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
		gitHash, gitErr := obj.GitHash()
		if gitErr != nil {
			gitHash = ""
		}
		gitAuthor, gitErr := obj.GitAuthor()
		if gitErr != nil {
			gitAuthor = ""
		}
		gitDate, gitErr := obj.GitDate()
		if gitErr != nil {
			gitDate = ""
		}
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
			c.handleChanged(changeCtx{
				obj: obj, dbObj: dbObj, plannedObj: &plannedObj,
				plan: plan, state: state,
				transitionsByKey: transitionsByKey, checksums: checksums,
				blockedCount: &blockedCount,
			})
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

type changeCtx struct {
	obj              *fs.Object
	dbObj            db.Object
	plannedObj       *types.PlannedObject
	plan             *types.MigrationPlan
	state            *db.State
	transitionsByKey map[string][]*fs.TransitionScript
	checksums        map[string]string
	blockedCount     *int
}

func (c *Computer) handleChanged(ctx changeCtx) {
	switch {
	case ctx.obj.Kind == "tables":
		transitions := ctx.transitionsByKey[ctx.obj.NormalizedKey]
		if len(transitions) == 0 {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			ctx.plan.Blockers = append(ctx.plan.Blockers, "table "+ctx.obj.NormalizedKey+" changed but has no non-scaffold transition scripts")
			*ctx.blockedCount++
		} else {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChanged
			for _, ts := range transitions {
				ctx.plannedObj.TransitionPaths = append(ctx.plannedObj.TransitionPaths, ts.Path)
			}
		}

	case ctx.obj.Kind == "triggers" && ctx.obj.ParentName != "":
		parentKey := types.NormalizedKey(ctx.obj.SchemaName, "tables", ctx.obj.ParentName)
		if _, ok := ctx.state.Objects[parentKey]; !ok {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			ctx.plan.Blockers = append(ctx.plan.Blockers, "trigger "+ctx.obj.NormalizedKey+" parent table "+parentKey+" not found")
			*ctx.blockedCount++
		} else if ctx.checksums[parentKey] == "" {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			ctx.plan.Blockers = append(ctx.plan.Blockers, "trigger "+ctx.obj.NormalizedKey+" parent table "+parentKey+" is changing")
			*ctx.blockedCount++
		} else {
			ctx.plannedObj.PlannedAction = types.ActionUpdateExistingModule
		}

	default:
		if types.IsModuleKind(ctx.obj.Kind) {
			ctx.plannedObj.PlannedAction = types.ActionUpdateExistingModule
		} else {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChanged
		}
	}
}
