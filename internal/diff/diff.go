package diff

import (
	"context"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
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

	plan.Schemas = make([]types.PlannedSchema, 0, len(layout.Schemas))
	for _, schema := range layout.Schemas {
		action := types.SchemaActionCreateSchema
		if _, exists := state.Schemas[schema.NormalizedName]; exists {
			action = types.SchemaActionExists
		}
		plan.Schemas = append(plan.Schemas, types.PlannedSchema{
			SchemaName: schema.Name,
			Action:     action,
		})
	}

	transitionsByKey := make(map[string][]*fs.TransitionScript, len(layout.Transitions))
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

	// Pre-calculate checksums and git info concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, obj := range layout.Objects {
		wg.Add(1)
		obj := obj
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, _ = obj.Checksum()
			_, _ = obj.GitHash()
			_, _ = obj.GitAuthor()
			_, _ = obj.GitDate()
		}()
	}
	wg.Wait()

	plan.Objects = make([]types.PlannedObject, 0, len(layout.Objects))
	for _, obj := range layout.Objects {
		dbObj, exists := state.Objects[obj.NormalizedKey]
		plannedObj := types.PlannedObject{
			ObjectRef: types.ObjectRef{
				ObjectPath:    obj.Path,
				SchemaName:    obj.SchemaName,
				Kind:          obj.Kind,
				ObjectName:    obj.ObjectName,
				NormalizedKey: obj.NormalizedKey,
			},
			DatabaseName: obj.DatabaseName,
			Exists:       exists,
			SourceFile:   obj.Path,
		}

		switch {
		case !exists:
			plannedObj.PlannedAction = types.ActionCreateObject
			cs, err := obj.Checksum()
			if err != nil {
				return nil, fmt.Errorf("checksum %s: %w", obj.NormalizedKey, err)
			}
			plannedObj.Checksum = cs
			setGitInfo(obj, &plannedObj)
			createCount++

		case exists && checksums[obj.NormalizedKey] == "":
			plannedObj.PlannedAction = types.ActionAdoptExisting
			cs, err := obj.Checksum()
			if err != nil {
				return nil, fmt.Errorf("checksum %s: %w", obj.NormalizedKey, err)
			}
			plannedObj.Checksum = cs
			setGitInfo(obj, &plannedObj)
			adoptCount++

		case exists && isMatch(obj, checksums[obj.NormalizedKey]):
			plannedObj.PlannedAction = types.ActionSkipUnchanged
			hex.Decode(plannedObj.Checksum[:], []byte(checksums[obj.NormalizedKey]))
			skipCount++

		default:
			changedCount++
			cs, err := obj.Checksum()
			if err != nil {
				return nil, fmt.Errorf("checksum %s: %w", obj.NormalizedKey, err)
			}
			plannedObj.Checksum = cs
			setGitInfo(obj, &plannedObj)
			blockedCount += c.handleChanged(changeCtx{
				obj: obj, dbObj: dbObj, plannedObj: &plannedObj,
				plan: plan, state: state,
				transitionsByKey: transitionsByKey, checksums: checksums,
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

func isMatch(obj *fs.Object, prior string) bool {
	cs, err := obj.Checksum()
	if err != nil || cs == [32]byte{} {
		return false
	}
	return hex.EncodeToString(cs[:]) == prior
}

func setGitInfo(obj *fs.Object, planned *types.PlannedObject) {
	h, err := obj.GitHash()
	if err == nil {
		planned.GitHash = h
	}
	a, err := obj.GitAuthor()
	if err == nil {
		planned.GitAuthor = a
	}
	d, err := obj.GitDate()
	if err == nil {
		planned.GitDate = d
	}
}

type changeCtx struct {
	obj              *fs.Object
	dbObj            db.Object
	plannedObj       *types.PlannedObject
	plan             *types.MigrationPlan
	state            *db.State
	transitionsByKey map[string][]*fs.TransitionScript
	checksums        map[string]string
}

func (c *Computer) handleChanged(ctx changeCtx) int {
	switch {
	case ctx.obj.Kind == "tables":
		transitions := ctx.transitionsByKey[ctx.obj.NormalizedKey]
		if len(transitions) == 0 {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			ctx.plan.Blockers = append(ctx.plan.Blockers, "table "+ctx.obj.NormalizedKey+" changed but has no non-scaffold transition scripts")
			return 1
		}
		ctx.plannedObj.PlannedAction = types.ActionReprocessChanged
		for _, ts := range transitions {
			ctx.plannedObj.TransitionPaths = append(ctx.plannedObj.TransitionPaths, ts.Path)
		}
		return 0

	case ctx.obj.Kind == "triggers" && ctx.obj.ParentName != "":
		parentKey := types.NormalizedKey(ctx.obj.SchemaName, "tables", ctx.obj.ParentName)
		if _, ok := ctx.state.Objects[parentKey]; !ok {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			ctx.plan.Blockers = append(ctx.plan.Blockers, "trigger "+ctx.obj.NormalizedKey+" parent table "+parentKey+" not found")
			return 1
		}
		if ctx.checksums[parentKey] == "" {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			ctx.plan.Blockers = append(ctx.plan.Blockers, "trigger "+ctx.obj.NormalizedKey+" parent table "+parentKey+" is changing")
			return 1
		}
		ctx.plannedObj.PlannedAction = types.ActionUpdateExistingModule
		return 0

	default:
		if types.IsModuleKind(ctx.obj.Kind) {
			ctx.plannedObj.PlannedAction = types.ActionUpdateExistingModule
		} else {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChanged
		}
		return 0
	}
}
