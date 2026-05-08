package lock

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func Acquire(ctx context.Context, conn *sql.Conn, timeout time.Duration) error {
	var result int
	err := conn.QueryRowContext(ctx, `DECLARE @result INT; EXEC @result = sp_getapplock @Resource='reporting_layer_migration', @LockMode='Exclusive', @LockOwner='Session', @LockTimeout=@p1; SELECT @result;`, int(timeout.Milliseconds())).Scan(&result)
	if err != nil {
		return err
	}
	if result < 0 {
		return fmt.Errorf("could not acquire reporting migration lock: result=%d", result)
	}
	return nil
}
