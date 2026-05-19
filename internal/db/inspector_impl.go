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

//go:embed sql/schemas_openjson.sql
var schemaOpenJSONSQL string

//go:embed sql/objects_openjson.sql
var objectOpenJSONSQL string

//go:embed sql/columns_openjson.sql
var columnOpenJSONSQL string

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
	canonical := scopeKey(scope)
	slotKey := scopeKeySHA256Hex(canonical)
	cacheKey := sharedScopeCacheKey(conn, slotKey)

	d.mu.Lock()
	cs, ok := d.cache[slotKey]
	if !ok {
		cs = sharedCachedScope(cacheKey)
		d.cache[slotKey] = cs
	}
	d.mu.Unlock()

	cs.once.Do(func() {
		cs.state, cs.err = d.readState(ctx, conn, scope)
	})

	return cs.state, cs.err
}

func (d *inspector) LoadTableColumns(ctx context.Context, conn driver.Conn, scope fs.Layout) (map[string][]TableColumn, error) {
	schemaNames := scopeSchemaNames(scope)
	if len(schemaNames) == 0 {
		return map[string][]TableColumn{}, nil
	}
	tableNames := scopeObjectNames(scope, "tables")
	if len(tableNames) == 0 {
		return map[string][]TableColumn{}, nil
	}
	return d.queryColumnsOpenJSON(ctx, conn, schemaNames, tableNames)
}

func InvalidateInspectorCache(conn driver.Conn) {
	connKey := driver.ConnStableKey(conn)
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

	return &State{
		Schemas:      schemas,
		Objects:      objects,
		TableColumns: map[string][]TableColumn{},
	}, nil
}

func (d *inspector) querySchemasOpenJSON(ctx context.Context, conn driver.Conn, schemaNames []string) (map[string]struct{}, error) {
	arg := types.MarshalStringSliceJSON(schemaNames)
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

func (d *inspector) queryObjectsOpenJSON(ctx context.Context, conn driver.Conn, schemaNames, objectNames []string) (map[string]Object, error) {
	schemaArg := types.MarshalStringSliceJSON(schemaNames)
	objectArg := types.MarshalStringSliceJSON(objectNames)
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

func (d *inspector) queryColumnsOpenJSON(ctx context.Context, conn driver.Conn, schemaNames, tableNames []string) (map[string][]TableColumn, error) {
	schemaArg := types.MarshalStringSliceJSON(schemaNames)
	tableArg := types.MarshalStringSliceJSON(tableNames)
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
