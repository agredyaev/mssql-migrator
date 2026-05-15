package db

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/schemas.sql
var schemaSQL string

//go:embed sql/objects.sql
var objectSQL string

//go:embed sql/columns.sql
var columnSQL string

type cachedScope struct {
	once  sync.Once
	state *State
	err   error
}

type inspector struct {
	mu    sync.Mutex
	cache map[string]*cachedScope
}

func NewInspector() Inspector {
	return &inspector{
		cache: make(map[string]*cachedScope),
	}
}

func (d *inspector) Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*State, error) {
	scopeKey := scopeKey(scope)

	d.mu.Lock()
	cs, ok := d.cache[scopeKey]
	if !ok {
		cs = &cachedScope{}
		d.cache[scopeKey] = cs
	}
	d.mu.Unlock()

	cs.once.Do(func() {
		cs.state, cs.err = d.readState(ctx, conn, scope)
	})

	return cs.state, cs.err
}

func (d *inspector) readState(ctx context.Context, conn driver.Conn, scope fs.Layout) (*State, error) {
	schemaNames := scopeSchemaNames(scope)
	if len(schemaNames) == 0 {
		return &State{
			Schemas:      map[string]struct{}{},
			Objects:      map[string]Object{},
			TableColumns: map[string][]TableColumn{},
		}, nil
	}

	schemas, err := d.querySchemas(ctx, conn, schemaNames)
	if err != nil {
		return nil, err
	}

	objectsBySchema := scopeObjectNamesBySchema(scope, "")
	objects := make(map[string]Object)
	for _, schemaChunk := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		var allObjNames []string
		for _, schema := range schemaChunk {
			allObjNames = append(allObjNames, objectsBySchema[schema]...)
		}
		if len(allObjNames) == 0 {
			continue
		}
		for _, objChunk := range types.ChunkKeys(allObjNames, driver.DefaultMaxParameters) {
			chunkObjs, err := d.queryObjects(ctx, conn, schemaChunk, objChunk)
			if err != nil {
				return nil, err
			}
			for k, v := range chunkObjs {
				objects[k] = v
			}
		}
	}

	tablesBySchema := scopeObjectNamesBySchema(scope, "tables")
	columns := make(map[string][]TableColumn)
	for _, schemaChunk := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		var allTblNames []string
		for _, schema := range schemaChunk {
			allTblNames = append(allTblNames, tablesBySchema[schema]...)
		}
		if len(allTblNames) == 0 {
			continue
		}
		for _, tblChunk := range types.ChunkKeys(allTblNames, driver.DefaultMaxParameters) {
			chunkCols, err := d.queryColumns(ctx, conn, schemaChunk, tblChunk)
			if err != nil {
				return nil, err
			}
			for k, v := range chunkCols {
				columns[k] = v
			}
		}
	}

	return &State{
		Schemas:      schemas,
		Objects:      objects,
		TableColumns: columns,
	}, nil
}

func (d *inspector) querySchemas(ctx context.Context, conn driver.Conn, schemaNames []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for _, chunk := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		q, args := types.BuildINQuery(schemaSQL, "{{schema_list}}", chunk, 1)
		rows, err := conn.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return nil, err
			}
			result[name] = struct{}{}
		}
		rows.Close()
	}
	return result, nil
}

func (d *inspector) queryObjects(ctx context.Context, conn driver.Conn, schemaNames, objectNames []string) (map[string]Object, error) {
	result := make(map[string]Object)
	for _, sc := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		for _, oc := range types.ChunkKeys(objectNames, driver.DefaultMaxParameters) {
			q, args := buildDualINQuery(objectSQL, "{{schema_list}}", sc, "{{object_list}}", oc)
			rows, err := conn.QueryContext(ctx, q, args...)
			if err != nil {
				return nil, err
			}

			for rows.Next() {
				var schemaName, kind, objectName, parentName string
				if err := rows.Scan(&schemaName, &kind, &objectName, &parentName); err != nil {
					rows.Close()
					return nil, err
				}
				key := types.NormalizedKey(schemaName, kind, objectName)
				result[key] = Object{
					SchemaName: schemaName,
					Kind:       kind,
					ObjectName: objectName,
					ParentName: parentName,
				}
			}
			rows.Close()
		}
	}
	return result, nil
}

func (d *inspector) queryColumns(ctx context.Context, conn driver.Conn, schemaNames, tableNames []string) (map[string][]TableColumn, error) {
	result := make(map[string][]TableColumn)
	for _, sc := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		for _, tc := range types.ChunkKeys(tableNames, driver.DefaultMaxParameters) {
			q, args := buildDualINQuery(columnSQL, "{{schema_list}}", sc, "{{table_list}}", tc)
			rows, err := conn.QueryContext(ctx, q, args...)
			if err != nil {
				return nil, err
			}

			for rows.Next() {
				var schemaName, tableName, colName, typeName string
				var length, precision, scale int
				var nullable bool
				if err := rows.Scan(&schemaName, &tableName, &colName, &typeName, &length, &precision, &scale, &nullable); err != nil {
					rows.Close()
					return nil, err
				}
				key := types.NormalizedKey(schemaName, "tables", tableName)
				result[key] = append(result[key], TableColumn{
					Name:           colName,
					NormalizedName: colName,
					TypeName:       typeName,
					Length:         length,
					Precision:      precision,
					Scale:          scale,
					Nullable:       nullable,
				})
			}
			rows.Close()
		}
	}
	return result, nil
}

func scopeSchemaNames(scope fs.Layout) []string {
	seen := map[string]struct{}{}
	for _, s := range scope.Schemas {
		seen[s.NormalizedName] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	return names
}

func scopeObjectNamesBySchema(scope fs.Layout, kind string) map[string][]string {
	result := make(map[string]map[string]struct{})
	for _, obj := range scope.Objects {
		if kind != "" && obj.Kind != kind {
			continue
		}
		schema := obj.NormalizedSchemaName
		if result[schema] == nil {
			result[schema] = make(map[string]struct{})
		}
		result[schema][obj.ObjectName] = struct{}{}
	}
	out := make(map[string][]string, len(result))
	for s, names := range result {
		list := make([]string, 0, len(names))
		for n := range names {
			list = append(list, n)
		}
		out[s] = list
	}
	return out
}

func scopeKey(scope fs.Layout) string {
	parts := make([]string, 0, len(scope.Schemas)+len(scope.Objects)+len(scope.Transitions)+len(scope.Checks))
	for _, s := range scope.Schemas {
		parts = append(parts, "s:"+s.NormalizedName)
	}
	for _, obj := range scope.Objects {
		parts = append(parts, "o:"+obj.NormalizedKey)
	}
	for _, ts := range scope.Transitions {
		parts = append(parts, "t:"+ts.NormalizedKey)
	}
	for _, cs := range scope.Checks {
		parts = append(parts, "c:"+cs.Path)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func buildDualINQuery(template, placeholder1 string, keys1 []string, placeholder2 string, keys2 []string) (string, []any) {
	q, args1 := types.BuildINQuery(template, placeholder1, keys1, 1)
	parts := make([]string, len(keys2))
	args := make([]any, len(keys2))
	for i, k := range keys2 {
		parts[i] = fmt.Sprintf("@p%d", len(keys1)+1+i)
		args[i] = k
	}
	query := strings.Replace(q, placeholder2, strings.Join(parts, ", "), -1)
	return query, append(args1, args...)
}
