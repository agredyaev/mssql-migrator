package migrator

import (
	"context"
	"database/sql"
	"fmt"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/lock"
	"reporting-db-migrations/internal/metadata"
)

func (r Runner) prepareProtectedRun(ctx context.Context) (contracts.MigrationReport, *sql.Conn, func(), error) {
	report := r.newMigrationReport()
	hash, err := checksum.SQLDirHash(r.cfg.SQLDir)
	if err != nil {
		return report, nil, nil, fmt.Errorf("%w: %v", contracts.ErrInvalidInput, err)
	}

	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return report, nil, nil, err
	}
	if err := metadata.Bootstrap(ctx, conn); err != nil {
		closeFn()
		return report, nil, nil, fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	if err := lock.Acquire(ctx, conn, r.cfg.LockTimeout); err != nil {
		closeFn()
		return report, nil, nil, fmt.Errorf("%w: %v", contracts.ErrLockTimeout, err)
	}
	report.SQLDirHash = hash
	return report, conn, closeFn, nil
}
