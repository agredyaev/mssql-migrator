package lock

import (
	"context"
	"time"

	"reporting-db-migrations/internal/driver"
)

type Locker interface {
	Acquire(ctx context.Context, conn driver.Conn, timeout time.Duration) error
	Release(ctx context.Context, conn driver.Conn) error
}
