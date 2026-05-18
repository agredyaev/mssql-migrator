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
	// QueryStringsContext lets hot paths pass string-only parameter lists without
	// boxing them into []any until the concrete driver boundary.
	QueryStringsContext(ctx context.Context, query string, args []string) (Rows, error)
	// QueryStringSlicesContext avoids building an intermediate combined []string
	// when a query naturally has two string-only parameter groups.
	QueryStringSlicesContext(ctx context.Context, query string, args1 []string, args2 []string) (Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
	Ping(ctx context.Context) error
	Close() error
}
