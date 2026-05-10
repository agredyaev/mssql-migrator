package app

import (
	"context"
	"io"

	"reporting-db-migrations/internal/commands"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

type commandResult struct {
	plan *contracts.MigrationPlan
}

type commandRunner func(context.Context, Handler, config.Config, logger.Logger, io.Writer) (commandResult, error)

type failureEventCarrier interface {
	FailureEvent() string
}

type failureEventError struct {
	event string
	err   error
}

func (e failureEventError) Error() string {
	return e.err.Error()
}

func (e failureEventError) Unwrap() error {
	return e.err
}

func (e failureEventError) FailureEvent() string {
	return e.event
}

var commandRunners = map[string]commandRunner{
	commands.Info: func(ctx context.Context, handler Handler, cfg config.Config, log logger.Logger, _ io.Writer) (commandResult, error) {
		return commandResult{}, handler.Info(ctx, cfg, log)
	},
	commands.Plan: func(ctx context.Context, handler Handler, cfg config.Config, log logger.Logger, stdout io.Writer) (commandResult, error) {
		plan, err := handler.Plan(ctx, cfg, log)
		if err != nil {
			return commandResult{}, err
		}
		if cfg.PlanJSON {
			if err := writePlanJSON(stdout, plan); err != nil {
				return commandResult{}, failureEventError{event: "plan_output_failed", err: err}
			}
		}
		return commandResult{plan: &plan}, nil
	},
	commands.Migrate: func(ctx context.Context, handler Handler, cfg config.Config, log logger.Logger, _ io.Writer) (commandResult, error) {
		return commandResult{}, handler.Migrate(ctx, cfg, log)
	},
	commands.Validate: func(ctx context.Context, handler Handler, cfg config.Config, log logger.Logger, _ io.Writer) (commandResult, error) {
		return commandResult{}, handler.Validate(ctx, cfg, log)
	},
	commands.Baseline: func(ctx context.Context, handler Handler, cfg config.Config, log logger.Logger, _ io.Writer) (commandResult, error) {
		return commandResult{}, handler.Baseline(ctx, cfg, log)
	},
	commands.RepairChecksum: func(ctx context.Context, handler Handler, cfg config.Config, log logger.Logger, _ io.Writer) (commandResult, error) {
		return commandResult{}, handler.RepairChecksum(ctx, cfg, log)
	},
}
