package migrator

import (
	"context"
	"database/sql"
	"errors"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/lock"
	"reporting-db-migrations/internal/metadata"
)

func (r Runner) prepareProtectedRun(ctx context.Context) (contracts.MigrationReport, *sql.Conn, func() error, error) {
	report := r.newMigrationReport()
	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return report, nil, nil, err
	}
	report.SQLRoot = r.cfg.SQLRoot
	report.Base = r.cfg.SQLBase
	report.EffectiveBasePath = r.cfg.SelectedBasePath()
	return report, conn, closeFn, nil
}

func (r Runner) acquireLock(ctx context.Context, conn *sql.Conn) error {
	if err := lock.Acquire(ctx, conn, r.cfg.LockTimeout); err != nil {
		if errors.Is(err, lock.ErrTimeout) {
			return contracts.Wrap(contracts.ErrLockTimeout, err)
		}
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	return nil
}

func bootstrapMetadata(ctx context.Context, conn *sql.Conn) error {
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	return nil
}
