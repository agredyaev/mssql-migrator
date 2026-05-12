package planner

import (
	"context"
	"database/sql"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/parser"
)

type sqlCatalogReader struct {
	conn    *sql.Conn
	schemas []string
	objects []catalog.ObjectRef
}

func SQLCatalogReader(conn *sql.Conn) CatalogReader {
	return sqlCatalogReader{conn: conn}
}

func SQLCatalogReaderForSchemas(conn *sql.Conn, schemas []string) CatalogReader {
	return sqlCatalogReader{conn: conn, schemas: append([]string(nil), schemas...)}
}

func SQLCatalogReaderForLayout(conn *sql.Conn, layout parser.Layout) CatalogReader {
	return sqlCatalogReader{
		conn:    conn,
		schemas: parser.ManagedSchemaNames(layout),
		objects: catalog.ObjectRefsForLayout(layout),
	}
}

func (r sqlCatalogReader) ReadCatalogState(ctx context.Context) (CatalogState, error) {
	state := CatalogState{Schemas: map[string]struct{}{}, Objects: map[string]CatalogObject{}, SuccessfulByKey: map[string]string{}}
	catalogState, err := catalog.ReadForLayout(ctx, r.conn, r.schemas, r.objects)
	if err != nil {
		return CatalogState{}, err
	}
	state.Schemas = catalogState.Schemas
	state.Objects = catalogState.Objects
	return state, nil
}
