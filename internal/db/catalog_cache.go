package db

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
)

//go:embed sql/catalog_cache_load.sql
var catalogCacheLoadSQL string

//go:embed sql/catalog_cache_invalidate.sql
var catalogCacheInvalidateSQL string

//go:embed sql/catalog_cache_save_meta.sql
var catalogCacheSaveMetaSQL string

//go:embed sql/catalog_cache_delete_all.sql
var catalogCacheDeleteAllSQL string

//go:embed sql/catalog_cache_insert_openjson.sql
var catalogCacheInsertOpenJSONSQL string

type catalogCacheJSONRow struct {
	K string `json:"k"`
	S string `json:"s"`
	G string `json:"g"`
	O string `json:"o"`
	P string `json:"p"`
}

// CatalogCacheEnabled reports whether persistent catalog cache is active (default on).
func CatalogCacheEnabled() bool {
	if strings.TrimSpace(os.Getenv("RMIG_CATALOG_CACHE")) == "0" {
		return false
	}
	if strings.TrimSpace(os.Getenv("RMIG_INSPECT_FULL")) == "1" {
		return false
	}
	return true
}

// SpotCheckCountFromEnv reads RMIG_CATALOG_SPOTCHECK (default 0).
func SpotCheckCountFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("RMIG_CATALOG_SPOTCHECK"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// LayoutDigest is a stable SHA-256 hex digest of the layout scope (schemas, objects, transitions, checks).
func LayoutDigest(layout fs.Layout) string {
	return scopeKeySHA256Hex(scopeKey(layout))
}

func layoutObjectCount(layout fs.Layout) int {
	n := 0
	for _, obj := range layout.Objects {
		if obj != nil {
			n++
		}
	}
	return n
}

// tryLoadCatalogCache returns a full cached state when meta and row count match the layout.
func tryLoadCatalogCache(ctx context.Context, conn driver.Conn, layout fs.Layout, iscope InspectScope) (*State, bool, error) {
	if !CatalogCacheEnabled() {
		return nil, false, nil
	}
	want := layoutObjectCount(layout)
	digest := LayoutDigest(layout)
	rows, err := conn.QueryStringsContext(ctx, catalogCacheLoadSQL, []string{digest, strconv.Itoa(want)})
	if err != nil {
		// Tables may not exist yet (EnsureTables racing inspect); treat as cache miss.
		if isMissingCatalogTableErr(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer rows.Close()

	objects := make(map[string]Object, want)
	schemas := make(map[string]struct{})
	for rows.Next() {
		var key, schema, kind, object, parent string
		if err := rows.Scan(&key, &schema, &kind, &object, &parent); err != nil {
			return nil, false, err
		}
		objects[key] = Object{
			SchemaName: schema,
			Kind:       kind,
			ObjectName: object,
			ParentName: parent,
		}
		schemas[schema] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(objects) != want {
		return nil, false, nil
	}

	for k, obj := range iscope.StableObjects {
		objects[k] = obj
	}

	return &State{
		Schemas:      schemas,
		Objects:      objects,
		TableColumns: map[string][]TableColumn{},
	}, true, nil
}

func saveCatalogCache(ctx context.Context, conn driver.Conn, layout fs.Layout, state *State) error {
	if !CatalogCacheEnabled() || state == nil {
		return nil
	}
	want := layoutObjectCount(layout)
	if want == 0 {
		return nil
	}
	if len(state.Objects) < want {
		return nil
	}
	digest := LayoutDigest(layout)

	if _, err := conn.ExecContext(ctx, catalogCacheDeleteAllSQL); err != nil {
		if isMissingCatalogTableErr(err) {
			return nil
		}
		return err
	}
	payload, err := marshalCatalogCacheJSON(state.Objects, want)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, catalogCacheInsertOpenJSONSQL, []string{payload, digest}); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, catalogCacheSaveMetaSQL, []string{digest, strconv.Itoa(want)})
	return err
}

func invalidateCatalogCache(ctx context.Context, conn driver.Conn) {
	_, _ = conn.ExecContext(ctx, catalogCacheInvalidateSQL)
}

// InvalidateCatalogCacheForConn clears persisted catalog rows (e.g. after audit flush).
func InvalidateCatalogCacheForConn(ctx context.Context, conn driver.Conn) {
	invalidateCatalogCache(ctx, conn)
}

func marshalCatalogCacheJSON(objects map[string]Object, want int) (string, error) {
	rows := make([]catalogCacheJSONRow, 0, want)
	for k, obj := range objects {
		rows = append(rows, catalogCacheJSONRow{
			K: k,
			S: obj.SchemaName,
			G: obj.Kind,
			O: obj.ObjectName,
			P: obj.ParentName,
		})
	}
	if len(rows) != want {
		return "", nil
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func isMissingCatalogTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "catalog_cache") ||
		strings.Contains(msg, "catalog_meta") ||
		strings.Contains(msg, "invalid object name")
}
