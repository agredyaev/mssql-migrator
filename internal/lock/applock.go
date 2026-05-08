package lock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTimeout = errors.New("app lock timeout")
	ErrFailed  = errors.New("app lock failed")
)

func Acquire(ctx context.Context, conn *sql.Conn, timeout time.Duration) error {
	var result int
	err := conn.QueryRowContext(ctx, `DECLARE @result INT; EXEC @result = sp_getapplock @Resource='reporting_layer_migration', @LockMode='Exclusive', @LockOwner='Session', @LockTimeout=@p1; SELECT @result;`, int(timeout.Milliseconds())).Scan(&result)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return fmt.Errorf("%w: %v", ErrFailed, err)
	}
	if result == -1 {
		return fmt.Errorf("%w: result=%d", ErrTimeout, result)
	}
	if result < 0 {
		return fmt.Errorf("%w: result=%d", ErrFailed, result)
	}
	return nil
}
