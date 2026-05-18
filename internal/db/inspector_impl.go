package db

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
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

//go:embed sql/schemas_openjson.sql
var schemaOpenJSONSQL string

//go:embed sql/objects_openjson.sql
var objectOpenJSONSQL string

//go:embed sql/columns_openjson.sql
var columnOpenJSONSQL string

//go:embed sql/openjson_compatibility.sql
var openJSONCompatibilitySQL string

var (
	objectSQLTemplate = types.CompileDualINTemplate(objectSQL, "{{schema_list}}", "{{object_list}}")
	columnSQLTemplate = types.CompileDualINTemplate(columnSQL, "{{schema_list}}", "{{table_list}}")
)

type cachedScope struct {
	once  sync.Once
	state *State
	err   error
}

type inspector struct {
	mu                sync.Mutex
	cache             map[string]*cachedScope
	openJSONProbeOnce sync.Once
	openJSONEnabled   bool
	openJSONProbeErr  error
}

var sharedInspectorCache = struct {
	mu         sync.Mutex
	generation map[string]uint64
	entries    map[string]*cachedScope
}{
	generation: make(map[string]uint64),
	entries:    make(map[string]*cachedScope),
}

func NewInspector() Inspector {
	return &inspector{
		cache: make(map[string]*cachedScope),
	}
}

func (d *inspector) Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*State, error) {
	scopeKey := scopeKey(scope)
	cacheKey := sharedScopeCacheKey(conn, scopeKey)

	d.mu.Lock()
	cs, ok := d.cache[scopeKey]
	if !ok {
		cs = sharedCachedScope(cacheKey)
		d.cache[scopeKey] = cs
	}
	d.mu.Unlock()

	cs.once.Do(func() {
		cs.state, cs.err = d.readState(ctx, conn, scope)
	})

	return cs.state, cs.err
}

func InvalidateInspectorCache(conn driver.Conn) {
	connKey := inspectorConnKey(conn)
	sharedInspectorCache.mu.Lock()
	sharedInspectorCache.generation[connKey]++
	sharedInspectorCache.mu.Unlock()
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

	useOpenJSON, err := d.supportsOpenJSON(ctx, conn)
	if err != nil {
		return nil, err
	}
	if useOpenJSON {
		return d.readStateOpenJSON(ctx, conn, scope, schemaNames)
	}
	return d.readStateChunked(ctx, conn, scope, schemaNames)
}

func (d *inspector) readStateChunked(ctx context.Context, conn driver.Conn, scope fs.Layout, schemaNames []string) (*State, error) {
	schemas, err := d.querySchemas(ctx, conn, schemaNames)
	if err != nil {
		return nil, err
	}

	objectsBySchema := scopeObjectNamesBySchema(scope, "")
	objects := make(map[string]Object)
	for _, schemaChunk := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		nObj := 0
		for _, schema := range schemaChunk {
			nObj += len(objectsBySchema[schema])
		}
		allObjNames := make([]string, 0, nObj)
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
		nTbl := 0
		for _, schema := range schemaChunk {
			nTbl += len(tablesBySchema[schema])
		}
		allTblNames := make([]string, 0, nTbl)
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

func (d *inspector) readStateOpenJSON(ctx context.Context, conn driver.Conn, scope fs.Layout, schemaNames []string) (*State, error) {
	schemas, err := d.querySchemasOpenJSON(ctx, conn, schemaNames)
	if err != nil {
		return nil, err
	}

	objectNames := scopeObjectNames(scope, "")
	objects := map[string]Object{}
	if len(objectNames) > 0 {
		objects, err = d.queryObjectsOpenJSON(ctx, conn, schemaNames, objectNames)
		if err != nil {
			return nil, err
		}
	}

	tableNames := scopeObjectNames(scope, "tables")
	columns := map[string][]TableColumn{}
	if len(tableNames) > 0 {
		columns, err = d.queryColumnsOpenJSON(ctx, conn, schemaNames, tableNames)
		if err != nil {
			return nil, err
		}
	}

	return &State{
		Schemas:      schemas,
		Objects:      objects,
		TableColumns: columns,
	}, nil
}

func (d *inspector) supportsOpenJSON(ctx context.Context, conn driver.Conn) (bool, error) {
	d.openJSONProbeOnce.Do(func() {
		d.openJSONEnabled, d.openJSONProbeErr = probeOpenJSONSupport(ctx, conn)
	})
	return d.openJSONEnabled, d.openJSONProbeErr
}

func probeOpenJSONSupport(ctx context.Context, conn driver.Conn) (bool, error) {
	rows, err := conn.QueryContext(ctx, openJSONCompatibilitySQL)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var enabled int
	if err := rows.Scan(&enabled); err != nil {
		return false, err
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return enabled == 1, nil
}

func (d *inspector) querySchemas(ctx context.Context, conn driver.Conn, schemaNames []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(schemaNames))
	for _, chunk := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		q, args := types.BuildINQuery(schemaSQL, "{{schema_list}}", chunk, 1)
		rows, err := conn.QueryStringsContext(ctx, q, args)
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

func (d *inspector) querySchemasOpenJSON(ctx context.Context, conn driver.Conn, schemaNames []string) (map[string]struct{}, error) {
	arg, err := marshalStringSliceJSON(schemaNames)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryStringsContext(ctx, schemaOpenJSONSQL, []string{arg})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]struct{}, len(schemaNames))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *inspector) queryObjects(ctx context.Context, conn driver.Conn, schemaNames, objectNames []string) (map[string]Object, error) {
	result := make(map[string]Object, len(objectNames))
	for _, sc := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		for _, oc := range types.ChunkKeys(objectNames, driver.DefaultMaxParameters) {
			q := buildDualINQueryText(objectSQL, "{{schema_list}}", sc, "{{object_list}}", oc)
			rows, err := conn.QueryStringSlicesContext(ctx, q, sc, oc)
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

func (d *inspector) queryObjectsOpenJSON(ctx context.Context, conn driver.Conn, schemaNames, objectNames []string) (map[string]Object, error) {
	schemaArg, err := marshalStringSliceJSON(schemaNames)
	if err != nil {
		return nil, err
	}
	objectArg, err := marshalStringSliceJSON(objectNames)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryStringsContext(ctx, objectOpenJSONSQL, []string{schemaArg, objectArg})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]Object, len(objectNames))
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *inspector) queryColumns(ctx context.Context, conn driver.Conn, schemaNames, tableNames []string) (map[string][]TableColumn, error) {
	result := make(map[string][]TableColumn, len(tableNames))
	for _, sc := range types.ChunkKeys(schemaNames, driver.DefaultMaxParameters) {
		for _, tc := range types.ChunkKeys(tableNames, driver.DefaultMaxParameters) {
			q := buildDualINQueryText(columnSQL, "{{schema_list}}", sc, "{{table_list}}", tc)
			rows, err := conn.QueryStringSlicesContext(ctx, q, sc, tc)
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

func (d *inspector) queryColumnsOpenJSON(ctx context.Context, conn driver.Conn, schemaNames, tableNames []string) (map[string][]TableColumn, error) {
	schemaArg, err := marshalStringSliceJSON(schemaNames)
	if err != nil {
		return nil, err
	}
	tableArg, err := marshalStringSliceJSON(tableNames)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryStringsContext(ctx, columnOpenJSONSQL, []string{schemaArg, tableArg})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]TableColumn, len(tableNames))
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
	if err := rows.Err(); err != nil {
		return nil, err
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

func scopeObjectNames(scope fs.Layout, kind string) []string {
	seen := map[string]struct{}{}
	for _, obj := range scope.Objects {
		if kind != "" && obj.Kind != kind {
			continue
		}
		seen[obj.ObjectName] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}

func scopeKey(scope fs.Layout) string {
	n := len(scope.Schemas) + len(scope.Objects) + len(scope.Transitions) + len(scope.Checks)
	if n == 0 {
		return ""
	}
	parts := make([]scopePart, 0, n)
	for _, s := range scope.Schemas {
		parts = append(parts, scopePart{'s', s.NormalizedName})
	}
	for _, obj := range scope.Objects {
		parts = append(parts, scopePart{'o', obj.NormalizedKey})
	}
	for _, ts := range scope.Transitions {
		parts = append(parts, scopePart{'t', ts.NormalizedKey})
	}
	for _, cs := range scope.Checks {
		parts = append(parts, scopePart{'c', cs.Path})
	}
	sort.Slice(parts, func(i, j int) bool {
		a, b := parts[i], parts[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.s < b.s
	})

	// One pass into strings.Builder avoids ~n separate "x:"+payload string allocations
	// from the previous []string + sort.Strings + strings.Join approach.
	var est int
	for _, p := range parts {
		est += 2 + len(p.s)
	}
	if n > 1 {
		est += n - 1
	}
	var b strings.Builder
	b.Grow(est)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteByte(p.kind)
		b.WriteByte(':')
		b.WriteString(p.s)
	}
	return b.String()
}

func sharedCachedScope(cacheKey string) *cachedScope {
	sharedInspectorCache.mu.Lock()
	defer sharedInspectorCache.mu.Unlock()
	if cs, ok := sharedInspectorCache.entries[cacheKey]; ok {
		return cs
	}
	cs := &cachedScope{}
	sharedInspectorCache.entries[cacheKey] = cs
	return cs
}

func sharedScopeCacheKey(conn driver.Conn, scopeKey string) string {
	connKey := inspectorConnKey(conn)
	sharedInspectorCache.mu.Lock()
	generation := sharedInspectorCache.generation[connKey]
	sharedInspectorCache.mu.Unlock()
	return fmt.Sprintf("%s|g:%d|%s", connKey, generation, scopeKey)
}

func inspectorConnKey(conn driver.Conn) string {
	v := reflect.ValueOf(conn)
	if !v.IsValid() {
		return "<nil>"
	}
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		return fmt.Sprintf("%T:%x", conn, v.Pointer())
	}
	return fmt.Sprintf("%T", conn)
}

// scopePart is one scopeKey fragment: same lexical order as "kind:payload" string.
type scopePart struct {
	kind byte
	s    string
}

func buildDualINQueryText(template, placeholder1 string, keys1 []string, placeholder2 string, keys2 []string) string {
	if template == objectSQL && placeholder1 == "{{schema_list}}" && placeholder2 == "{{object_list}}" {
		return objectSQLTemplate.BuildQuery(keys1, keys2, 1)
	}
	if template == columnSQL && placeholder1 == "{{schema_list}}" && placeholder2 == "{{table_list}}" {
		return columnSQLTemplate.BuildQuery(keys1, keys2, 1)
	}
	q, _ := types.BuildDualINQuery(template, placeholder1, keys1, placeholder2, keys2, 1)
	return q
}

func marshalStringSliceJSON(values []string) (string, error) {
	return types.MarshalStringSliceJSON(values), nil
}
