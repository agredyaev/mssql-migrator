package migrator

import (
	"context"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/validate"
)

type validationMode struct {
	includeChecks  bool
	refreshModules bool
	affectedOnly   bool
	existingRunID  string
}

func managedScopeValidationMode(runID string) validationMode {
	return validationMode{includeChecks: false, refreshModules: false, affectedOnly: true, existingRunID: runID}
}

func fullValidationMode() validationMode {
	return validationMode{includeChecks: true, refreshModules: true}
}

func resolveValidationObjects(plan contracts.MigrationPlan, layout parser.Layout, affectedOnly bool) []parser.Object {
	if !affectedOnly {
		return layout.Objects
	}
	selected := make(map[string]struct{}, len(plan.Objects))
	for _, object := range plan.Objects {
		if object.PlannedAction == contracts.ActionSkipUnchanged || object.PlannedAction == contracts.ActionAdoptExisting {
			continue
		}
		selected[object.NormalizedKey] = struct{}{}
	}
	if len(selected) == 0 {
		return nil
	}
	result := make([]parser.Object, 0, len(selected))
	for _, object := range layout.Objects {
		if _, ok := selected[object.NormalizedKey]; !ok {
			continue
		}
		result = append(result, object)
	}
	return result
}

func validateManagedScopeState(layout parser.Layout, catalogState validate.CatalogState) (validate.Scope, error) {
	return validate.ResolveManagedScope(layout, catalogState)
}

func (r Runner) executeValidationScope(ctx context.Context, session *runSession, layout parser.Layout, mode validationMode, vr contracts.ValidationReport, runState validationRunState) (contracts.ValidationReport, error) {
	scope, err := validateManagedScopeState(layout, runState.catalog)
	if err != nil {
		runState.recorder.validation.recordFailure(ctx, scope.Missing, runState.itemIDs, err, mode.includeChecks, r.log)
		return vr, runState.fail(ctx, session, &vr, validationFailureBase(err), err)
	}
	validatedObjects := scope.ExistingObjects()
	if mode.refreshModules {
		modules, err := validate.RefreshManagedObjects(ctx, session.conn, layout, r.log)
		vr.Validation.ModulesRefreshed = modules.ModulesRefreshed
		if err != nil {
			runState.recorder.validation.recordFailure(ctx, validatedObjects, runState.itemIDs, err, mode.includeChecks, r.log)
			return vr, runState.fail(ctx, session, &vr, validationFailureBase(err), err)
		}
	}
	if err := runState.recorder.validation.markSuccesses(ctx, validatedObjects); err != nil {
		return vr, runState.fail(ctx, session, &vr, contracts.ErrCriticalState, err)
	}
	if err := r.runValidationChecks(ctx, session, layout, mode.includeChecks, &vr, runState); err != nil {
		return vr, err
	}
	if err := runState.finish(ctx, session, &vr); err != nil {
		return vr, err
	}
	return vr, nil
}
