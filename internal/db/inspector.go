package db

import (
	"context"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
)

type Inspector interface {
	Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*State, error)
	// LoadTableColumns fetches column metadata for tables in scope. Call only when
	// needed (e.g. blocked migrate scaffold); Inspect does not load columns.
	LoadTableColumns(ctx context.Context, conn driver.Conn, scope fs.Layout) (map[string][]TableColumn, error)
}

type State struct {
	Schemas      map[string]struct{}
	Objects      map[string]Object
	TableColumns map[string][]TableColumn
}

type Object struct {
	SchemaName string
	Kind       string
	ObjectName string
	ParentName string
}

type TableColumn struct {
	Name           string
	NormalizedName string
	TypeName       string
	Length         int
	Precision      int
	Scale          int
	Nullable       bool
}
