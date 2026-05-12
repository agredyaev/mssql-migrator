package migrator

import (
	"context"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/planner"
)

func (r Runner) prepareApprovedMigrationExecution(ctx context.Context, state *protectedRunState, options executionPlanOptions) (executionPlanContext, error) {
	approved, err := planner.ReadApprovedPlan(r.cfg.PlanFile)
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, err)
	}
	current, err := r.resolveExecutionPlan(ctx, state, options)
	if err != nil {
		return executionPlanContext{}, err
	}
	if err := planner.VerifyApprovedPlanMatches(r.cfg, approved, current.plan); err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, err)
	}
	return r.persistExecutionPlan(ctx, state, current, options)
}
