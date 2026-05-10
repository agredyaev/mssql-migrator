package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
)

func (r Runner) Baseline(ctx context.Context) error {
	baselineRunner := r
	baselineRunner.cfg.UpdatePolicy = config.UpdatePolicyNone
	state, err := baselineRunner.startProtectedRunState(ctx, "baseline_failed")
	if err != nil {
		return err
	}
	defer state.Close()

	planCtx, err := baselineRunner.prepareBaselineExecution(ctx, state)
	if err != nil {
		return err
	}
	if err := baselineRunner.validateBaselinePlan(ctx, state, planCtx); err != nil {
		return err
	}
	if err := baselineRunner.verifyBaselineCreatePermissions(ctx, state, planCtx); err != nil {
		return err
	}
	if err := state.executeTrackedPlan(ctx, baselineRunner, planCtx.layout.resolved, planCtx.plan, planCtx.itemIDs); err != nil {
		return err
	}
	return state.finishSuccess(ctx)
}

func (r Runner) RepairChecksum(ctx context.Context) error {
	state, err := r.startProtectedRunState(ctx, "repair_checksum_failed")
	if err != nil {
		return err
	}
	defer state.Close()

	repairCtx, err := r.prepareRepairChecksum(ctx, state)
	if err != nil {
		return err
	}
	if err := r.recordRepairChecksum(ctx, state, repairCtx); err != nil {
		return err
	}
	return state.finishSuccess(ctx)
}

type repairChecksumContext struct {
	target          parser.Object
	plannedTarget   contracts.PlannedObject
	currentChecksum string
	itemID          *int64
}

func (r Runner) prepareBaselineExecution(ctx context.Context, state *protectedRunState) (executionPlanContext, error) {
	return r.prepareExecutionPlan(ctx, state, executionPlanOptions{
		layoutBase:     contracts.ErrInvalidInput,
		buildPlanError: func(error) error { return contracts.ErrInvalidInput },
		startRun: startRunOptions{
			command: contracts.CommandBaseline,
		},
	})
}

func (r Runner) validateBaselinePlan(ctx context.Context, state *protectedRunState, planCtx executionPlanContext) error {
	for _, object := range planCtx.plan.Objects {
		if err := r.validateBaselineObject(ctx, state, planCtx.itemIDs, object); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) validateBaselineObject(ctx context.Context, state *protectedRunState, itemIDs map[string]int64, object contracts.PlannedObject) error {
	itemID := lookupItemID(itemIDs, object.NormalizedKey)
	switch object.PlannedAction {
	case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting, contracts.ActionCreateObject:
		return nil
	case contracts.ActionUpdateExistingModule, contracts.ActionUpdateExistingSupported, contracts.ActionReprocessChangedBlocked, contracts.ActionReprocessChanged:
		failure := baselineDriftFailure(object)
		r.warnBaselineAttemptWriteFailure(state.recorder.attempt.ObjectFailure(ctx, baselineFailureMetadataObject(object, itemID, r.cfg.TransactionMode), failure, true))
		return state.fail(ctx, failure, nil)
	default:
		failure := fmt.Errorf("%w: unsupported baseline object state %s for %s", contracts.ErrInvalidInput, object.PlannedAction, object.ObjectPath)
		r.warnBaselineAttemptWriteFailure(state.recorder.attempt.ObjectFailure(ctx, baselineFailureMetadataObject(object, itemID, r.cfg.TransactionMode), failure, true))
		return state.fail(ctx, failure, nil)
	}
}

func (r Runner) verifyBaselineCreatePermissions(ctx context.Context, state *protectedRunState, planCtx executionPlanContext) error {
	if err := verifyBaselineCreatePermissionsBestEffort(ctx, state.session.conn, planCtx.plan, planCtx.layout.resolved); err != nil {
		r.recordBaselinePreflightFailure(ctx, state, planCtx, err)
		return state.fail(ctx, err, nil)
	}
	return nil
}

func (r Runner) recordBaselinePreflightFailure(ctx context.Context, state *protectedRunState, planCtx executionPlanContext, err error) {
	var failure *baselinePreflightFailure
	if !errors.As(err, &failure) {
		return
	}
	if strings.TrimSpace(failure.schemaName) != "" {
		r.warnBaselineAttemptWriteFailure(state.recorder.attempt.SchemaFailure(ctx, failure.schemaName, err, true))
	}
	if strings.TrimSpace(failure.object.Path) == "" {
		return
	}
	itemID := lookupItemID(planCtx.itemIDs, failure.object.NormalizedKey)
	planned := findBaselinePlannedObject(planCtx.plan, failure.object)
	r.warnBaselineAttemptWriteFailure(state.recorder.attempt.ObjectFailure(ctx, baselineFailureMetadataObject(planned, itemID, r.cfg.TransactionMode), err, true))
}

func findBaselinePlannedObject(plan contracts.MigrationPlan, object parser.Object) contracts.PlannedObject {
	for _, planned := range plan.Objects {
		if planned.NormalizedKey == object.NormalizedKey {
			return planned
		}
	}
	return contracts.PlannedObject{ObjectPath: object.Path, NormalizedKey: object.NormalizedKey, Checksum: object.Checksum}
}

func (r Runner) warnBaselineAttemptWriteFailure(err error) {
	if err != nil {
		r.log.Warn("baseline_metadata_write_failed", logger.Redact(err.Error()))
	}
}

func baselineDriftFailure(object contracts.PlannedObject) error {
	if object.Kind == "tables" {
		if len(object.TransitionPaths) > 0 {
			return fmt.Errorf("%w: baseline found tracked table drift for %s; use migrate to apply checked-in transitions from %s", contracts.ErrMetadataDrift, object.ObjectPath, strings.Join(object.TransitionPaths, ", "))
		}
		return fmt.Errorf("%w: baseline found tracked table drift for %s; add a checked-in migration under %s and use migrate", contracts.ErrMetadataDrift, object.ObjectPath, requiredTableTransitionDir(object))
	}
	return fmt.Errorf("%w: baseline found existing metadata drift for %s; use repair-checksum", contracts.ErrMetadataDrift, object.ObjectPath)
}

func requiredTableTransitionDir(object contracts.PlannedObject) string {
	if strings.TrimSpace(object.SchemaName) == "" || strings.TrimSpace(object.ObjectName) == "" {
		return "<schema>/tables/_migrations/<table>/"
	}
	return object.SchemaName + "/tables/_migrations/" + object.ObjectName + "/"
}

func (r Runner) prepareRepairChecksum(ctx context.Context, state *protectedRunState) (repairChecksumContext, error) {
	layout, successfulByKey, err := r.loadProtectedPlanningInputs(ctx, state, contracts.ErrInvalidInput)
	if err != nil {
		return repairChecksumContext{}, err
	}
	target, plannedTarget, err := r.resolveRepairTargets(ctx, state, layout, successfulByKey)
	if err != nil {
		return repairChecksumContext{}, err
	}
	catalog, currentChecksum, err := r.loadRepairTargetState(ctx, state, target, successfulByKey)
	if err != nil {
		return repairChecksumContext{}, err
	}
	if err := state.startRun(ctx, contracts.CommandRepairChecksum, "", "", contracts.RollbackScopeNone); err != nil {
		return repairChecksumContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	itemID, err := state.recorder.scope.Repair(ctx, target, plannedTarget, catalog, currentChecksum)
	if err != nil {
		return repairChecksumContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	return repairChecksumContext{target: target, plannedTarget: plannedTarget, currentChecksum: currentChecksum, itemID: itemID}, nil
}

func (r Runner) resolveRepairTargets(ctx context.Context, state *protectedRunState, layout plannerLayoutContext, successfulByKey map[string]string) (parser.Object, contracts.PlannedObject, error) {
	target, err := resolveRepairObject(layout.resolved, r.cfg.RepairTarget)
	if err != nil {
		return parser.Object{}, contracts.PlannedObject{}, state.fail(ctx, contracts.ErrInvalidInput, err)
	}
	plan, err := state.session.BuildPlan(ctx, successfulByKey, layout.resolved, layout.hash)
	if err != nil {
		return parser.Object{}, contracts.PlannedObject{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	plannedTarget, err := resolveRepairPlanObject(plan, target.NormalizedKey)
	if err != nil {
		return parser.Object{}, contracts.PlannedObject{}, state.fail(ctx, contracts.ErrInvalidInput, err)
	}
	if err := validateRepairEligibility(target, plannedTarget); err != nil {
		return parser.Object{}, contracts.PlannedObject{}, state.fail(ctx, contracts.ErrInvalidInput, err)
	}
	return target, plannedTarget, nil
}

func (r Runner) loadRepairTargetState(ctx context.Context, state *protectedRunState, target parser.Object, successfulByKey map[string]string) (planner.CatalogState, string, error) {
	catalog, err := state.session.ReadPlanningCatalog(ctx)
	if err != nil {
		return planner.CatalogState{}, "", state.fail(ctx, contracts.ErrCriticalState, err)
	}
	if _, exists := catalog.Objects[target.NormalizedKey]; !exists {
		return planner.CatalogState{}, "", state.fail(ctx, contracts.ErrInvalidInput, fmt.Errorf("repair target is missing from the database: %s", target.Path))
	}
	currentChecksum, exists := successfulByKey[target.NormalizedKey]
	if !exists {
		return planner.CatalogState{}, "", state.fail(ctx, contracts.ErrInvalidInput, fmt.Errorf("repair target has no successful metadata row: %s", target.Path))
	}
	return catalog, currentChecksum, nil
}

func (r Runner) recordRepairChecksum(ctx context.Context, state *protectedRunState, repairCtx repairChecksumContext) error {
	report := state.session.MigrationReport()
	if repairCtx.currentChecksum == repairCtx.target.Checksum {
		report.Skipped = append(report.Skipped, contracts.ScriptResult{Script: repairCtx.target.Path, Type: repairCtx.target.Kind, Checksum: repairCtx.target.Checksum, Reason: "checksum_already_matches"})
		return nil
	}
	if err := state.recorder.repair.recordSuccess(ctx, repairCtx.target, repairCtx.itemID); err != nil {
		return state.fail(ctx, contracts.ErrCriticalState, err)
	}
	report.Applied = append(report.Applied, contracts.ScriptResult{Script: repairCtx.target.Path, Type: repairCtx.target.Kind, Checksum: repairCtx.target.Checksum})
	return nil
}

func resolveRepairObject(layout parser.Layout, selector string) (parser.Object, error) {
	selector = strings.TrimSpace(selector)
	normalizedSelector := parser.NormalizeTrackedName(selector)
	for _, object := range layout.Objects {
		if object.Path == selector || object.NormalizedKey == normalizedSelector || object.NormalizedKey == strings.ToLower(strings.TrimSpace(selector)) {
			return object, nil
		}
	}
	return parser.Object{}, fmt.Errorf("repair-checksum target not found in repo layout: %s", selector)
}

func resolveRepairPlanObject(plan contracts.MigrationPlan, normalizedKey string) (contracts.PlannedObject, error) {
	for _, object := range plan.Objects {
		if object.NormalizedKey == normalizedKey {
			return object, nil
		}
	}
	return contracts.PlannedObject{}, fmt.Errorf("repair-checksum target not found in current plan: %s", normalizedKey)
}

func validateRepairEligibility(target parser.Object, planned contracts.PlannedObject) error {
	switch planned.PlannedAction {
	case contracts.ActionUpdateExistingModule, contracts.ActionUpdateExistingSupported, contracts.ActionReprocessChangedBlocked:
		return nil
	case contracts.ActionReprocessChanged:
		if planned.Kind == "tables" || len(planned.TransitionPaths) > 0 {
			return fmt.Errorf("repair-checksum cannot run for %s: the current plan will apply checked-in table transitions, so use migrate instead", target.Path)
		}
		return fmt.Errorf("repair-checksum cannot run for %s: the current plan will reprocess the object, so use migrate instead", target.Path)
	case contracts.ActionSkipUnchanged:
		return fmt.Errorf("repair-checksum is not needed for %s: the latest successful metadata checksum already matches the current repo SQL", target.Path)
	case contracts.ActionAdoptExisting:
		return fmt.Errorf("repair-checksum cannot run for %s: the object exists in the database but is not tracked yet, so use baseline or migrate to adopt it first", target.Path)
	case contracts.ActionCreateObject:
		return fmt.Errorf("repair-checksum cannot run for %s: the object is planned for creation and has no prior successful metadata checksum to repair", target.Path)
	default:
		return fmt.Errorf("repair-checksum cannot run for %s: planned action is %s and does not represent tracked checksum drift", target.Path, planned.PlannedAction)
	}
}
