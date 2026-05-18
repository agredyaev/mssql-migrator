package diff

import (
	"context"
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

func (c *Computer) Compute(ctx context.Context, layout fs.Layout, state *db.State, checksums map[string][32]byte) (*types.MigrationPlan, error) {
	plan := &types.MigrationPlan{
		PlannedAt: time.Now().UTC(),
	}

	if state == nil {
		state = &db.State{Objects: map[string]db.Object{}}
	}
	if checksums == nil {
		checksums = map[string][32]byte{}
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

	var transitionsByKey map[string][]*fs.TransitionScript
	if len(layout.Transitions) > 0 {
		counts := make(map[string]int, len(layout.Transitions))
		for _, ts := range layout.Transitions {
			if ts.Scaffold {
				continue
			}
			counts[ts.NormalizedKey]++
		}
		transitionsByKey = make(map[string][]*fs.TransitionScript, len(counts))
		for k, n := range counts {
			transitionsByKey[k] = make([]*fs.TransitionScript, 0, n)
		}
		for _, ts := range layout.Transitions {
			if ts.Scaffold {
				continue
			}
			k := ts.NormalizedKey
			transitionsByKey[k] = append(transitionsByKey[k], ts)
		}
	}

	var (
		createCount  int
		adoptCount   int
		skipCount    int
		changedCount int
		blockedCount int
	)

	// Warm up checksums and git info concurrently — but only when at least
	// one file still has a cold checksum cache. After Scanner.Scan, preloadChecksums
	// has usually filled every CachedFile.checksumOnce, so this returns immediately
	// and avoids ~N goroutine-stack allocations per Compute.
	warmupIfNeeded(layout)

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
		}

		cs, err := obj.Checksum()
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", obj.NormalizedKey, err)
		}
		plannedObj.Checksum = cs

		switch {
		case !exists:
			plannedObj.PlannedAction = types.ActionCreateObject
			setGitInfo(obj, &plannedObj)
			createCount++

		case exists:
			prior, hasPrior := checksums[obj.NormalizedKey]
			if !hasPrior || prior == ([32]byte{}) {
				plannedObj.PlannedAction = types.ActionAdoptExisting
				setGitInfo(obj, &plannedObj)
				adoptCount++
			} else if cs != ([32]byte{}) && cs == prior {
				plannedObj.PlannedAction = types.ActionSkipUnchanged
				skipCount++
			} else {
				changedCount++
				setGitInfo(obj, &plannedObj)
				blockedCount += c.handleChanged(changeCtx{
					obj: obj, dbObj: dbObj, plannedObj: &plannedObj,
					plan: plan, state: state,
					transitionsByKey: transitionsByKey, checksums: checksums,
				})
			}
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

func isMatch(obj *fs.Object, prior [32]byte) bool {
	if prior == ([32]byte{}) {
		return false
	}
	cs, err := obj.Checksum()
	if err != nil || cs == ([32]byte{}) {
		return false
	}
	return cs == prior
}

func priorDigestPresent(m map[string][32]byte, key string) bool {
	cs, ok := m[key]
	return ok && cs != ([32]byte{})
}

func setGitInfo(obj *fs.Object, planned *types.PlannedObject) {
	var g types.GitInfo
	if h, err := obj.GitHash(); err == nil {
		g.GitHash = h
	}
	if a, err := obj.GitAuthor(); err == nil {
		g.GitAuthor = a
	}
	if d, err := obj.GitDate(); err == nil {
		g.GitDate = d
	}
	if g.GitHash == "" && g.GitAuthor == "" && g.GitDate == "" {
		planned.Git = nil
		return
	}
	cp := g
	planned.Git = &cp
}

type changeCtx struct {
	obj              *fs.Object
	dbObj            db.Object
	plannedObj       *types.PlannedObject
	plan             *types.MigrationPlan
	state            *db.State
	transitionsByKey map[string][]*fs.TransitionScript
	checksums        map[string][32]byte
}

func appendBlocker(plan *types.MigrationPlan, msg string) {
	if plan.Blockers == nil {
		plan.Blockers = make([]string, 0, 8)
	}
	plan.Blockers = append(plan.Blockers, msg)
}

func (c *Computer) handleChanged(ctx changeCtx) int {
	switch {
	case ctx.obj.Kind == "tables":
		transitions := ctx.transitionsByKey[ctx.obj.NormalizedKey]
		if len(transitions) == 0 {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			appendBlocker(ctx.plan, "table "+ctx.obj.NormalizedKey+" changed but has no non-scaffold transition scripts")
			return 1
		}
		ctx.plannedObj.PlannedAction = types.ActionReprocessChanged
		paths := make([]string, len(transitions))
		for i, ts := range transitions {
			paths[i] = ts.Path
		}
		ctx.plannedObj.TransitionPaths = paths
		return 0

	case ctx.obj.Kind == "triggers" && ctx.obj.ParentName != "":
		parentKey := types.NormalizedKey(ctx.obj.SchemaName, "tables", ctx.obj.ParentName)
		if _, ok := ctx.state.Objects[parentKey]; !ok {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			appendBlocker(ctx.plan, "trigger "+ctx.obj.NormalizedKey+" parent table "+parentKey+" not found")
			return 1
		}
		if !priorDigestPresent(ctx.checksums, parentKey) {
			ctx.plannedObj.PlannedAction = types.ActionReprocessChangedBlocked
			ctx.plan.Blocked = true
			appendBlocker(ctx.plan, "trigger "+ctx.obj.NormalizedKey+" parent table "+parentKey+" is changing")
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

// warmupIfNeeded pre-computes Checksum and GitInfo for every object
// concurrently — but only when at least one object still has a cold checksum
// cache (IsChecksumCached is false).
//
// After Scanner.Scan, object checksums are often already warm from preload and
// this returns immediately without spawning goroutines.
//
// Cold path: layouts built without going through Scanner, tests, or checksum
// errors — a small worker pool fans out Checksum/Git work
// instead of spawning one goroutine per object (large alloc/scheduling savings).
func warmupIfNeeded(layout fs.Layout) {
	for _, obj := range layout.Objects {
		if !obj.IsChecksumCached() {
			warmupAll(layout)
			return
		}
	}
}

func warmupAll(layout fs.Layout) {
	n := len(layout.Objects)
	if n == 0 {
		return
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan *fs.Object, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for obj := range jobs {
				_, _ = obj.Checksum()
				_, _ = obj.GitHash()
				_, _ = obj.GitAuthor()
				_, _ = obj.GitDate()
			}
		}()
	}
	for _, obj := range layout.Objects {
		jobs <- obj
	}
	close(jobs)
	wg.Wait()
}
