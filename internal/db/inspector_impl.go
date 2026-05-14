package db

import (
	"context"
	_ "embed"
	"fmt"
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

type inspector struct {
	mu    sync.Mutex
	cache map[string]*State
}

func NewInspector() Inspector {
	return &inspector{
		cache: make(map[string]*State),
	}
}

func (d *inspector) Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*State, error) {
	scopeKey := scopeKey(scope)
	d.mu.Lock()
	if cached, ok := d.cache[scopeKey]; ok {
		d.mu.Unlock()
		return cached, nil
	}
	d.mu.Unlock()

	state, err := d.readState(ctx, conn, scope)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.cache[scopeKey] = state
	d.mu.Unlock()

	return state, nil
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

	objectKeys := scopeObjectKeys(scope)
	objects := make(map[string]Object)
	if len(objectKeys) > 0 {
		for _, chunk := range chunkKeys(schemaNames) {
			for _, objChunk := range chunkKeys(objectKeys) {
				chunkObjs, err := d.queryObjects(ctx, conn, chunk, objChunk)
				if err != nil {
					return nil, err
				}
				for k, v := range chunkObjs {
					objects[k] = v
				}
			}
		}
	}

	tableKeys := scopeTableKeys(scope)
	columns := make(map[string][]TableColumn)
	if len(tableKeys) > 0 {
		for _, chunk := range chunkKeys(schemaNames) {
			for _, tblChunk := range chunkKeys(tableKeys) {
				chunkCols, err := d.queryColumns(ctx, conn, chunk, tblChunk)
				if err != nil {
					return nil, err
				}
				for k, v := range chunkCols {
					columns[k] = v
				}
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
	for _, chunk := range chunkKeys(schemaNames) {
		q, args := buildINQuery(schemaSQL, "{{schema_list}}", chunk)
		rows, err := conn.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			result[name] = struct{}{}
		}
	}
	return result, nil
}

func (d *inspector) queryObjects(ctx context.Context, conn driver.Conn, schemaNames, objectNames []string) (map[string]Object, error) {
	result := make(map[string]Object)
	for _, sc := range chunkKeys(schemaNames) {
		for _, oc := range chunkKeys(objectNames) {
			q, args := buildDualINQuery(objectSQL, "{{schema_list}}", sc, "{{object_list}}", oc)
			rows, err := conn.QueryContext(ctx, q, args...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			for rows.Next() {
				var schemaName, kind, objectName, parentName string
				if err := rows.Scan(&schemaName, &kind, &objectName, &parentName); err != nil {
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
		}
	}
	return result, nil
}

func (d *inspector) queryColumns(ctx context.Context, conn driver.Conn, schemaNames, tableNames []string) (map[string][]TableColumn, error) {
	result := make(map[string][]TableColumn)
	for _, sc := range chunkKeys(schemaNames) {
		for _, tc := range chunkKeys(tableNames) {
			q, args := buildDualINQuery(columnSQL, "{{schema_list}}", sc, "{{table_list}}", tc)
			rows, err := conn.QueryContext(ctx, q, args...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			for rows.Next() {
				var schemaName, tableName, colName, typeName string
				var length, precision, scale int
				var nullable bool
				if err := rows.Scan(&schemaName, &tableName, &colName, &typeName, &length, &precision, &scale, &nullable); err != nil {
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

func scopeObjectKeys(scope fs.Layout) []string {
	names := make([]string, 0, len(scope.Objects))
	for _, obj := range scope.Objects {
		names = append(names, obj.ObjectName)
	}
	return names
}

func scopeTableKeys(scope fs.Layout) []string {
	seen := map[string]struct{}{}
	for _, obj := range scope.Objects {
		if obj.Kind != "tables" {
			continue
		}
		seen[obj.ObjectName] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	return names
}

func scopeKey(scope fs.Layout) string {
	var parts []string
	for _, s := range scope.Schemas {
		parts = append(parts, "s:"+s.NormalizedName)
	}
	for _, obj := range scope.Objects {
		parts = append(parts, "o:"+obj.NormalizedKey)
	}
	return strings.Join(parts, "|")
}

func chunkKeys(keys []string) [][]string {
	if len(keys) == 0 {
		return nil
	}
	var chunks [][]string
	for i := 0; i < len(keys); i += types.SQLServerMaxParameters {
		end := i + types.SQLServerMaxParameters
		if end > len(keys) {
			end = len(keys)
		}
		chunks = append(chunks, keys[i:end])
	}
	return chunks
}

func buildINQuery(template, placeholder string, keys []string) (string, []any) {
	parts := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("@p%d", i+1)
		args[i] = k
	}
	query := strings.Replace(template, placeholder, strings.Join(parts, ", "), 1)
	return query, args
}

func buildDualINQuery(template, placeholder1 string, keys1 []string, placeholder2 string, keys2 []string) (string, []any) {
	q, args1 := buildINQuery(template, placeholder1, keys1)
	offset := len(args1)
	parts := make([]string, len(keys2))
	args := make([]any, len(keys2))
	for i, k := range keys2 {
		parts[i] = fmt.Sprintf("@p%d", offset+i+1)
		args[i] = k
	}
	query := strings.Replace(q, placeholder2, strings.Join(parts, ", "), 1)
	return query, append(args1, args...)
}
