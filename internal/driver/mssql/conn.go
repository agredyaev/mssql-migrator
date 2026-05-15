package mssql

import (
	"context"
	"database/sql"

	_ "github.com/microsoft/go-mssqldb"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

type Conn struct {
	db *sql.DB
}

func Open(ctx context.Context, cfg types.Config) (*Conn, error) {
	dsn := BuildDSN(cfg)
	database, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, err
	}
	return &Conn{db: database}, nil
}

func (c *Conn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &rowsAdapter{rows: rows}, nil
}

func (c *Conn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	res, err := c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &resultAdapter{res: res}, nil
}

func (c *Conn) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *Conn) Close() error {
	return c.db.Close()
}

type rowsAdapter struct {
	rows *sql.Rows
}

func (r *rowsAdapter) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *rowsAdapter) Next() bool             { return r.rows.Next() }
func (r *rowsAdapter) Err() error             { return r.rows.Err() }
func (r *rowsAdapter) Close() error           { return r.rows.Close() }

type resultAdapter struct {
	res sql.Result
}

func (r *resultAdapter) RowsAffected() (int64, error) {
	return r.res.RowsAffected()
}
