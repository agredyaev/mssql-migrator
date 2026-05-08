package migrator

import (
	"context"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

type Handler struct{}

func (Handler) Info(ctx context.Context, cfg config.Config, log logger.Logger) error {
	return NewRunner(cfg, log).Info(ctx)
}

func (Handler) Plan(ctx context.Context, cfg config.Config, log logger.Logger) (contracts.MigrationPlan, error) {
	return NewRunner(cfg, log).Plan(ctx)
}

func (Handler) Migrate(ctx context.Context, cfg config.Config, log logger.Logger) error {
	return NewRunner(cfg, log).Migrate(ctx)
}

func (Handler) Validate(ctx context.Context, cfg config.Config, log logger.Logger) error {
	return NewRunner(cfg, log).Validate(ctx)
}

func (Handler) Baseline(ctx context.Context, cfg config.Config, log logger.Logger) error {
	return NewRunner(cfg, log).Baseline(ctx)
}

func (Handler) RepairChecksum(ctx context.Context, cfg config.Config, log logger.Logger) error {
	return NewRunner(cfg, log).RepairChecksum(ctx)
}
