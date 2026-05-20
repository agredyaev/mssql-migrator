package diff

import (
	"fmt"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type PlanScenario uint8

const (
	ScenarioCreate PlanScenario = iota
	ScenarioAdopt
	ScenarioSkipUnchanged
	ScenarioTableReprocess
	ScenarioTableBlockedNoTransitions
	ScenarioTriggerUpdateModule
	ScenarioTriggerBlockedParentMissing
	ScenarioTriggerBlockedParentChanging
	ScenarioModuleUpdate
	ScenarioReprocess
)

func (s PlanScenario) action() string {
	switch s {
	case ScenarioCreate:
		return types.ActionCreateObject
	case ScenarioAdopt:
		return types.ActionAdoptExisting
	case ScenarioSkipUnchanged:
		return types.ActionSkipUnchanged
	case ScenarioTableReprocess, ScenarioReprocess:
		return types.ActionReprocessChanged
	case ScenarioTableBlockedNoTransitions, ScenarioTriggerBlockedParentMissing, ScenarioTriggerBlockedParentChanging:
		return types.ActionReprocessChangedBlocked
	case ScenarioTriggerUpdateModule, ScenarioModuleUpdate:
		return types.ActionUpdateExistingModule
	default:
		return types.ActionReprocessChanged
	}
}

func (s PlanScenario) withGit() bool {
	return s != ScenarioSkipUnchanged
}

func (s PlanScenario) blockedDelta() int {
	switch s {
	case ScenarioTableBlockedNoTransitions, ScenarioTriggerBlockedParentMissing, ScenarioTriggerBlockedParentChanging:
		return 1
	default:
		return 0
	}
}

type scenarioCounter int

const (
	counterCreate scenarioCounter = iota
	counterAdopt
	counterSkip
	counterChanged
)

func (s PlanScenario) counterKind() scenarioCounter {
	switch s {
	case ScenarioCreate:
		return counterCreate
	case ScenarioAdopt:
		return counterAdopt
	case ScenarioSkipUnchanged:
		return counterSkip
	default:
		return counterChanged
	}
}

func resolvePlanScenario(
	exists bool,
	hasPrior bool,
	prior [32]byte,
	checksum [32]byte,
	kindCode uint8,
	obj *fs.Object,
	state *db.State,
	checksums map[string][32]byte,
	transitions []*fs.TransitionScript,
) PlanScenario {
	if !exists {
		return ScenarioCreate
	}
	if !hasPrior || prior == ([32]byte{}) {
		return ScenarioAdopt
	}
	if checksum != ([32]byte{}) && checksum == prior {
		return ScenarioSkipUnchanged
	}
	return resolveChangedScenario(kindCode, obj, state, checksums, transitions)
}

func resolveChangedScenario(
	kindCode uint8,
	obj *fs.Object,
	state *db.State,
	checksums map[string][32]byte,
	transitions []*fs.TransitionScript,
) PlanScenario {
	switch kindCode {
	case fs.KindCodeTables:
		if len(transitions) == 0 {
			return ScenarioTableBlockedNoTransitions
		}
		return ScenarioTableReprocess
	case fs.KindCodeTriggers:
		if obj.ParentName == "" {
			return changedDefaultScenario(kindCode)
		}
		parentKey := obj.ParentNormalizedKey
		if parentKey == "" {
			parentKey = types.NormalizedKey(obj.SchemaName, "tables", obj.ParentName)
		}
		if _, ok := state.Objects[parentKey]; !ok {
			return ScenarioTriggerBlockedParentMissing
		}
		if !priorDigestPresent(checksums, parentKey) {
			return ScenarioTriggerBlockedParentChanging
		}
		return ScenarioTriggerUpdateModule
	default:
		return changedDefaultScenario(kindCode)
	}
}

func changedDefaultScenario(kindCode uint8) PlanScenario {
	if fs.IsModuleKindCode(kindCode) {
		return ScenarioModuleUpdate
	}
	return ScenarioReprocess
}

func applyScenario(scenario PlanScenario, ctx changeCtx) int {
	switch scenario {
	case ScenarioTableBlockedNoTransitions:
		ctx.plannedObj.PlannedAction = scenario.action()
		ctx.plan.Blocked = true
		appendBlocker(ctx.plan, fmt.Sprintf("table %s changed but has no non-scaffold transition scripts", ctx.obj.NormalizedKey))
	case ScenarioTriggerBlockedParentMissing:
		parentKey := ctx.obj.ParentNormalizedKey
		if parentKey == "" {
			parentKey = types.NormalizedKey(ctx.obj.SchemaName, "tables", ctx.obj.ParentName)
		}
		ctx.plannedObj.PlannedAction = scenario.action()
		ctx.plan.Blocked = true
		appendBlocker(ctx.plan, fmt.Sprintf("trigger %s parent table %s not found", ctx.obj.NormalizedKey, parentKey))
	case ScenarioTriggerBlockedParentChanging:
		parentKey := ctx.obj.ParentNormalizedKey
		if parentKey == "" {
			parentKey = types.NormalizedKey(ctx.obj.SchemaName, "tables", ctx.obj.ParentName)
		}
		ctx.plannedObj.PlannedAction = scenario.action()
		ctx.plan.Blocked = true
		appendBlocker(ctx.plan, fmt.Sprintf("trigger %s parent table %s is changing", ctx.obj.NormalizedKey, parentKey))
	case ScenarioTableReprocess:
		ctx.plannedObj.PlannedAction = scenario.action()
		transitions := ctx.transitionsByKey[ctx.obj.NormalizedKey]
		paths := make([]string, len(transitions))
		for i, ts := range transitions {
			paths[i] = ts.Path
		}
		ctx.plannedObj.TransitionPaths = paths
	default:
		ctx.plannedObj.PlannedAction = scenario.action()
	}
	return scenario.blockedDelta()
}

func applyObjectScenario(
	scenario PlanScenario,
	c *Computer,
	obj *fs.Object,
	plannedObj *types.PlannedObject,
	plan *types.MigrationPlan,
	state *db.State,
	transitionsByKey map[string][]*fs.TransitionScript,
	checksums map[string][32]byte,
) int {
	if scenario.withGit() {
		c.setGitInfo(obj, plannedObj)
	}
	return applyScenario(scenario, changeCtx{
		obj:              obj,
		plannedObj:       plannedObj,
		plan:             plan,
		state:            state,
		transitionsByKey: transitionsByKey,
		checksums:        checksums,
	})
}
