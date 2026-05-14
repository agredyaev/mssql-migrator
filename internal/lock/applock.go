package lock

import (
	"context"
	_ "embed"
	"time"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/errors"
)

//go:embed sql/acquire.sql
var acquireSQL string

//go:embed sql/release.sql
var releaseSQL string

type AppLock struct{}

func New() *AppLock {
	return &AppLock{}
}

func (l *AppLock) Acquire(ctx context.Context, conn driver.Conn, timeout time.Duration) error {
	var result int
	rows, err := conn.QueryContext(ctx, acquireSQL, timeout.Milliseconds())
	if err != nil {
		return errors.Wrap(errors.ErrLockTimeout, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return errors.ErrLockTimeout
	}
	if err := rows.Scan(&result); err != nil {
		return errors.Wrap(errors.ErrLockFailed, err)
	}
	if result < 0 {
		return errors.ErrLockTimeout
	}
	return nil
}

func (l *AppLock) Release(ctx context.Context, conn driver.Conn) error {
	rows, err := conn.QueryContext(ctx, releaseSQL)
	if err != nil {
		return errors.Wrap(errors.ErrLockFailed, err)
	}
	defer rows.Close()
	return nil
}
