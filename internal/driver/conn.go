package driver

import "context"

const DefaultMaxParameters = 2100

type Rows interface {
	Scan(dest ...any) error
	Next() bool
	Err() error
	Close() error
}

type Result interface {
	RowsAffected() (int64, error)
}

type Conn interface {
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
	Ping(ctx context.Context) error
	Close() error
}
