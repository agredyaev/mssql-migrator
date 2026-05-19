package db

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/columns_openjson.sql
var columnOpenJSONSQL string

//go:embed sql/catalog_scoped_hit.sql
var catalogScopedHitSQL string

type cachedScope struct {
	err   error
	state *State
	once  sync.Once
}

type inspector struct {
	cache map[string]*cachedScope // keys: scopeKeySHA256Hex(canonical); "" for empty layout
	mu    sync.Mutex
}

var sharedInspectorCache = struct {
	generation map[string]uint64
	entries    map[string]*cachedScope
	mu         sync.Mutex
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
	return d.InspectWithScope(ctx, conn, scope, InspectScope{FullInspect: true})
}

func (d *inspector) InspectWithScope(ctx context.Context, conn driver.Conn, layout fs.Layout, iscope InspectScope) (*State, error) {
	canonical := scopeKey(layout)
	slotKey := inspectCacheSlotKey(canonical, iscope)
	cacheKey := sharedScopeCacheKey(conn, slotKey)

	d.mu.Lock()
	cs, ok := d.cache[slotKey]
	if !ok {
		cs = sharedCachedScope(cacheKey)
		d.cache[slotKey] = cs
	}
	d.mu.Unlock()

	scopeCopy := iscope
	cs.once.Do(func() {
		cs.state, cs.err = d.readState(ctx, conn, layout, scopeCopy)
	})

	return cs.state, cs.err
}

func inspectCacheSlotKey(canonical string, iscope InspectScope) string {
	if iscope.FullInspect {
		return scopeKeySHA256Hex(canonical)
	}
	var b strings.Builder
	b.Grow(len(canonical) + 64)
	b.WriteString(canonical)
	b.WriteString("|scoped|")
	for _, ref := range iscope.HotRefs {
		b.WriteByte('\x00')
		b.WriteString(ref.Schema)
		b.WriteByte('\x00')
		b.WriteString(ref.Kind)
		b.WriteByte('\x00')
		b.WriteString(ref.Object)
	}
	return scopeKeySHA256Hex(b.String())
}

func (d *inspector) LoadTableColumns(ctx context.Context, conn driver.Conn, scope fs.Layout) (map[string][]TableColumn, error) {
	schemaNames := scopeSchemaNames(scope)
	if len(schemaNames) == 0 {
		return map[string][]TableColumn{}, nil
	}
	tableRefs := scopeObjectRefs(scope, "tables")
	if len(tableRefs) == 0 {
		return map[string][]TableColumn{}, nil
	}
	return d.queryColumnsOpenJSON(ctx, conn, tableRefs)
}

func InvalidateInspectorCache(conn driver.Conn) {
	connKey := driver.ConnStableKey(conn)
	sharedInspectorCache.mu.Lock()
	sharedInspectorCache.generation[connKey]++
	sharedInspectorCache.mu.Unlock()
	if CatalogCacheEnabled() {
		invalidateCatalogCache(context.Background(), conn)
	}
}

func (d *inspector) readState(ctx context.Context, conn driver.Conn, layout fs.Layout, iscope InspectScope) (*State, error) {
	if state, ok, err := tryLoadCatalogCache(ctx, conn, layout, iscope); err != nil {
		return nil, err
	} else if ok {
		return state, nil
	}
	var (
		state *State
		err   error
	)
	if iscope.FullInspect {
		state, err = d.readStateFull(ctx, conn, layout)
	} else {
		state, err = d.readStateScoped(ctx, conn, layout, iscope)
	}
	if err != nil {
		return nil, err
	}
	_ = saveCatalogCache(ctx, conn, layout, state)
	return state, nil
}

func (d *inspector) readStateFull(ctx context.Context, conn driver.Conn, layout fs.Layout) (*State, error) {
	schemaNames := scopeSchemaNames(layout)
	if len(schemaNames) == 0 {
		return &State{
			Schemas:      map[string]struct{}{},
			Objects:      map[string]Object{},
			TableColumns: map[string][]TableColumn{},
		}, nil
	}

	objectRefs := scopeObjectRefs(layout, "")
	if len(objectRefs) == 0 {
		existing, err := d.queryExistingLayoutSchemas(ctx, conn, schemaNames)
		if err != nil {
			return nil, err
		}
		return &State{
			Schemas:      existing,
			Objects:      map[string]Object{},
			TableColumns: map[string][]TableColumn{},
		}, nil
	}
	if state, ok, err := d.tryFastEmptyCatalog(ctx, conn, schemaNames, objectRefs); err != nil {
		return nil, err
	} else if ok {
		return state, nil
	}
	kinds := catalogKindsForLayout(layout)
	return d.queryCatalogStateOpenJSON(ctx, conn, objectRefs, schemaNames, kinds)
}

func (d *inspector) readStateScoped(ctx context.Context, conn driver.Conn, layout fs.Layout, iscope InspectScope) (*State, error) {
	schemaNames := scopeSchemaNames(layout)
	schemas := map[string]struct{}{}
	if len(schemaNames) > 0 {
		existing, err := d.queryExistingLayoutSchemas(ctx, conn, schemaNames)
		if err != nil {
			return nil, err
		}
		schemas = existing
	}

	objects := make(map[string]Object, len(iscope.StableObjects)+len(iscope.HotRefs))
	for k, obj := range iscope.StableObjects {
		objects[k] = obj
	}

	hotRefs := iscope.HotRefs
	if len(hotRefs) > 0 {
		if _, ok, err := d.tryFastEmptyCatalog(ctx, conn, schemaNames, hotRefs); err != nil {
			return nil, err
		} else if ok {
			return &State{
				Schemas:      schemas,
				Objects:      objects,
				TableColumns: map[string][]TableColumn{},
			}, nil
		}
		kinds := catalogKindsForRefs(hotRefs)
		hotState, err := d.queryCatalogStateOpenJSON(ctx, conn, hotRefs, schemaNames, kinds)
		if err != nil {
			return nil, err
		}
		for k, obj := range hotState.Objects {
			objects[k] = obj
		}
		for k := range hotState.Schemas {
			schemas[k] = struct{}{}
		}
	}

	return &State{
		Schemas:      schemas,
		Objects:      objects,
		TableColumns: map[string][]TableColumn{},
	}, nil
}

func catalogKindsForRefs(refs []types.ObjectScopeRef) catalogKinds {
	var k catalogKinds
	for _, ref := range refs {
		switch ref.Kind {
		case "types":
			k.types = true
		case "indexes":
			k.indexes = true
		default:
			k.sysObjects = true
		}
	}
	return k
}

func (d *inspector) tryFastEmptyCatalog(
	ctx context.Context,
	conn driver.Conn,
	schemaNames []string,
	refs []types.ObjectScopeRef,
) (*State, bool, error) {
	refArg := types.MarshalObjectScopeJSON(refs)
	rows, err := conn.QueryStringsContext(ctx, catalogScopedHitSQL, []string{refArg})
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	hasHit := rows.Next()
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if hasHit {
		return nil, false, nil
	}
	_ = schemaNames // fast path: no scoped objects → empty schema map (see queryExistingLayoutSchemas for schema-only layouts)
	return &State{
		Schemas:      map[string]struct{}{},
		Objects:      map[string]Object{},
		TableColumns: map[string][]TableColumn{},
	}, true, nil
}

func (d *inspector) queryExistingLayoutSchemas(ctx context.Context, conn driver.Conn, schemaNames []string) (map[string]struct{}, error) {
	query := `WITH layout_schema_filter AS (
    SELECT DISTINCT LOWER(CONVERT(nvarchar(128), [value])) AS schema_name
    FROM OPENJSON(@p1)
)
SELECT LOWER(s.name) AS schema_name
FROM sys.schemas s
INNER JOIN layout_schema_filter lf ON lf.schema_name = LOWER(s.name)`
	schemaArg := types.MarshalStringSliceJSON(schemaNames)
	rows, err := conn.QueryStringsContext(ctx, query, []string{schemaArg})
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
	return result, rows.Err()
}

func (d *inspector) queryCatalogStateOpenJSON(
	ctx context.Context,
	conn driver.Conn,
	refs []types.ObjectScopeRef,
	schemaNames []string,
	kinds catalogKinds,
) (*State, error) {
	query := buildCatalogStateSQL(kinds)
	refArg := types.MarshalObjectScopeJSON(refs)
	schemaArg := types.MarshalStringSliceJSON(schemaNames)
	rows, err := conn.QueryStringSlicesContext(ctx, query, []string{refArg}, []string{schemaArg})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schemas := make(map[string]struct{}, len(schemaNames))
	objects := make(map[string]Object, len(refs))
	for rows.Next() {
		var rowKind, schemaName, kind, objectName, parentName string
		if err := rows.Scan(&rowKind, &schemaName, &kind, &objectName, &parentName); err != nil {
			return nil, err
		}
		switch rowKind {
		case "schema":
			schemas[schemaName] = struct{}{}
		case "object":
			key := types.NormalizedKey(schemaName, kind, objectName)
			objects[key] = Object{
				SchemaName: schemaName,
				Kind:       kind,
				ObjectName: objectName,
				ParentName: parentName,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &State{
		Schemas:      schemas,
		Objects:      objects,
		TableColumns: map[string][]TableColumn{},
	}, nil
}

func (d *inspector) queryColumnsOpenJSON(ctx context.Context, conn driver.Conn, refs []types.ObjectScopeRef) (map[string][]TableColumn, error) {
	arg := types.MarshalObjectScopeJSON(refs)
	rows, err := conn.QueryStringsContext(ctx, columnOpenJSONSQL, []string{arg})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]TableColumn, len(refs))
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

func scopeObjectRefs(scope fs.Layout, kind string) []types.ObjectScopeRef {
	seen := map[string]struct{}{}
	out := make([]types.ObjectScopeRef, 0, len(scope.Objects))
	for _, obj := range scope.Objects {
		if kind != "" && obj.Kind != kind {
			continue
		}
		if _, ok := seen[obj.NormalizedKey]; ok {
			continue
		}
		seen[obj.NormalizedKey] = struct{}{}
		out = append(out, types.ObjectScopeRef{
			Schema: obj.SchemaName,
			Kind:   obj.Kind,
			Object: obj.ObjectName,
		})
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
		parts = append(parts, scopePart{s: s.NormalizedName, kind: 's'})
	}
	for _, obj := range scope.Objects {
		parts = append(parts, scopePart{s: obj.NormalizedKey, kind: 'o'})
	}
	for _, ts := range scope.Transitions {
		parts = append(parts, scopePart{s: ts.NormalizedKey, kind: 't'})
	}
	for _, cs := range scope.Checks {
		parts = append(parts, scopePart{s: cs.Path, kind: 'c'})
	}
	sort.Slice(parts, func(i, j int) bool {
		a, b := parts[i], parts[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.s < b.s
	})

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

// scopeKeySHA256Hex is the Phase 3 inspector cache slot key: SHA-256 over the
// UTF-8 bytes of the canonical scopeKey string, hex-encoded (64 ASCII chars).
// Empty canonical maps to "" so an empty layout keeps the historical empty
// cache slot key.
func scopeKeySHA256Hex(canonical string) string {
	if canonical == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
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

func sharedScopeCacheKey(conn driver.Conn, scopeSlotKey string) string {
	connKey := driver.ConnStableKey(conn)
	sharedInspectorCache.mu.Lock()
	generation := sharedInspectorCache.generation[connKey]
	sharedInspectorCache.mu.Unlock()
	return fmt.Sprintf("%s|g:%d|%s", connKey, generation, scopeSlotKey)
}

type scopePart struct {
	s    string
	kind byte
}
