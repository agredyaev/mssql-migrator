package app

import (
	"context"
	"fmt"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

type NotImplementedHandler struct{}

func (NotImplementedHandler) Info(context.Context, config.Config, logger.Logger) error {
	return fmt.Errorf("%w: info handler not implemented", contracts.ErrInvalidInput)
}

func (NotImplementedHandler) Plan(context.Context, config.Config, logger.Logger) (contracts.MigrationPlan, error) {
	return contracts.MigrationPlan{}, fmt.Errorf("%w: plan handler not implemented", contracts.ErrInvalidInput)
}

func (NotImplementedHandler) Migrate(context.Context, config.Config, logger.Logger) error {
	return fmt.Errorf("%w: migrate handler not implemented", contracts.ErrInvalidInput)
}

func (NotImplementedHandler) Validate(context.Context, config.Config, logger.Logger) error {
	return fmt.Errorf("%w: validate handler not implemented", contracts.ErrInvalidInput)
}

func (NotImplementedHandler) Baseline(context.Context, config.Config, logger.Logger) error {
	return fmt.Errorf("%w: baseline handler not implemented", contracts.ErrInvalidInput)
}

func (NotImplementedHandler) RepairChecksum(context.Context, config.Config, logger.Logger) error {
	return fmt.Errorf("%w: repair-checksum handler not implemented", contracts.ErrInvalidInput)
}
