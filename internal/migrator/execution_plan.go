package migrator

import (
	"context"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/planner"
)

func (r Runner) resolveExecutionPlan(ctx context.Context, state *protectedRunState, options executionPlanOptions) (executionPlanContext, error) {
	layout, successfulByKey, err := r.loadProtectedPlanningInputs(ctx, state, options.layoutBase)
	if err != nil {
		return executionPlanContext{}, err
	}
	catalogState, err := timedValue(r.log, "read_catalog_ms", func() (planner.CatalogState, error) {
		return state.session.ReadPlanningCatalogForLayout(ctx, layout.resolved)
	})
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	plan, err := timedValue(r.log, "build_plan_ms", func() (contracts.MigrationPlan, error) {
		return state.session.BuildPlanWithCatalog(ctx, successfulByKey, layout.resolved, layout.hash, catalogState)
	})
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, options.buildPlanError(err), err)
	}
	if !options.skipTransitionPreflight {
		created, scaffoldErr := timedValue(r.log, "ensure_table_transitions_ms", func() (bool, error) {
			columns, err := r.loadBlockedTableColumns(ctx, state.session.conn, plan)
			if err != nil {
				return false, err
			}
			return ensureTableTransitionFiles(r.cfg, layout.resolved, plan, columns)
		})
		if scaffoldErr != nil {
			return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, scaffoldErr)
		}
		if created {
		layout, err = timedValue(r.log, "resolve_layout_ms", func() (plannerLayoutContext, error) {
			resolved, hash, err := state.session.ResolvePlanningLayout()
			if err != nil {
				return plannerLayoutContext{}, err
			}
			return plannerLayoutContext{resolved: resolved, hash: hash}, nil
		})
		if err != nil {
			return executionPlanContext{}, state.fail(ctx, options.layoutBase, err)
		}
		state.setLayoutHash(layout.hash)
		plan, err = timedValue(r.log, "build_plan_ms", func() (contracts.MigrationPlan, error) {
			return state.session.BuildPlanWithCatalog(ctx, successfulByKey, layout.resolved, layout.hash, catalogState)
		})
		if err != nil {
			return executionPlanContext{}, state.fail(ctx, options.buildPlanError(err), err)
		}
		}
	}
	if plan.Blocked {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrChecksumMismatch, fmt.Errorf("plan is blocked: %s", strings.Join(plan.BlockReasons, " | ")))
	}
	return executionPlanContext{layout: layout, plan: plan}, nil
}

func (r Runner) persistExecutionPlan(ctx context.Context, state *protectedRunState, planCtx executionPlanContext, options executionPlanOptions) (executionPlanContext, error) {
	if options.startRun.planFile != "" && options.startRun.planHash == "" {
		planHash, err := planArtifactHash(options.startRun.planFile)
		if err != nil {
			return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, err)
		}
		options.startRun.planHash = planHash
	}
	if err := state.startRun(ctx, options.startRun.command, options.startRun.planFile, options.startRun.planHash, planCtx.plan.Rollback); err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	itemIDs, err := timedValue(r.log, "persist_scope_ms", func() (map[string]int64, error) {
		return state.recorder.scope.Migration(ctx, planCtx.plan)
	})
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	planCtx.itemIDs = itemIDs
	return planCtx, nil
}

func (r Runner) prepareExecutionPlan(ctx context.Context, state *protectedRunState, options executionPlanOptions) (executionPlanContext, error) {
	planCtx, err := r.resolveExecutionPlan(ctx, state, options)
	if err != nil {
		return executionPlanContext{}, err
	}
	return r.persistExecutionPlan(ctx, state, planCtx, options)
}
