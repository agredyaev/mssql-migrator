package audit

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/load_checksums.sql
var loadChecksumsSQL string

//go:embed sql/load_checksums_openjson.sql
var loadChecksumsOpenJSONSQL string

//go:embed sql/bootstrap.sql
var bootstrapSQL string

//go:embed sql/load_migrations.sql
var loadMigrationsSQL string

//go:embed sql/load_all_migrations.sql
var loadAllMigrationsSQL string

var loadChecksumsOpenJSONSupport sync.Map

var loadChecksumsCache = struct {
	mu         sync.Mutex
	generation map[string]uint64
	entries    map[string]checksumsCacheEntry
}{
	generation: make(map[string]uint64),
	entries:    make(map[string]checksumsCacheEntry),
}

type checksumsCacheEntry struct {
	generation uint64
	result     map[string][32]byte
}

func EnsureTables(ctx context.Context, conn driver.Conn) error {
	_, err := conn.ExecContext(ctx, bootstrapSQL)
	return err
}

// LoadChecksums returns the latest applied/adopted SHA-256 digest per normalized_key.
// Values are raw 32-byte digests; the database column remains hex-encoded — decoding
// happens once here so diff.Compute can compare without per-object hex parsing.
func LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string][32]byte, error) {
	if len(keys) == 0 {
		return map[string][32]byte{}, nil
	}
	if cached, ok := lookupChecksumsCache(conn, keys); ok {
		return cached, nil
	}
	if useOpenJSONForChecksums(conn) {
		result, err := loadChecksumsOpenJSON(ctx, conn, keys)
		if err == nil {
			storeChecksumsCache(conn, keys, result)
			return result, nil
		}
		if openJSONStateKnown(conn) {
			return nil, err
		}
		setOpenJSONSupport(conn, false)
	}
	result := make(map[string][32]byte, len(keys))
	chunks := types.ChunkKeys(keys, driver.DefaultMaxParameters)
	for _, chunk := range chunks {
		query, args := types.BuildINQuery(loadChecksumsSQL, "{{keys}}", chunk, 1)
		rows, err := conn.QueryStringsContext(ctx, query, args)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key, checksum string
			if err := rows.Scan(&key, &checksum); err != nil {
				rows.Close()
				return nil, err
			}
			sum, err := parseHistoryChecksum(key, checksum)
			if err != nil {
				rows.Close()
				return nil, err
			}
			result[key] = sum
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	storeChecksumsCache(conn, keys, result)
	return result, nil
}

func loadChecksumsOpenJSON(ctx context.Context, conn driver.Conn, keys []string) (map[string][32]byte, error) {
	rows, err := conn.QueryStringsContext(ctx, loadChecksumsOpenJSONSQL, []string{types.MarshalStringSliceJSON(keys)})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][32]byte, len(keys))
	for rows.Next() {
		var key, checksum string
		if err := rows.Scan(&key, &checksum); err != nil {
			return nil, err
		}
		sum, err := parseHistoryChecksum(key, checksum)
		if err != nil {
			return nil, err
		}
		result[key] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	setOpenJSONSupport(conn, true)
	return result, nil
}

func connSupportKey(conn driver.Conn) string {
	v := reflect.ValueOf(conn)
	if !v.IsValid() {
		return "<nil>"
	}
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		return fmt.Sprintf("%T:%x", conn, v.Pointer())
	}
	return fmt.Sprintf("%T", conn)
}

func useOpenJSONForChecksums(conn driver.Conn) bool {
	key := connSupportKey(conn)
	if v, ok := loadChecksumsOpenJSONSupport.Load(key); ok {
		return v.(bool)
	}
	t := fmt.Sprintf("%T", conn)
	return strings.Contains(strings.ToLower(t), "mssql")
}

func openJSONStateKnown(conn driver.Conn) bool {
	_, ok := loadChecksumsOpenJSONSupport.Load(connSupportKey(conn))
	return ok
}

func setOpenJSONSupport(conn driver.Conn, enabled bool) {
	loadChecksumsOpenJSONSupport.Store(connSupportKey(conn), enabled)
}

func lookupChecksumsCache(conn driver.Conn, keys []string) (map[string][32]byte, bool) {
	cacheKey := checksumsCacheKey(conn, keys)
	connKey := connSupportKey(conn)
	loadChecksumsCache.mu.Lock()
	defer loadChecksumsCache.mu.Unlock()
	entry, ok := loadChecksumsCache.entries[cacheKey]
	if !ok {
		return nil, false
	}
	if entry.generation != loadChecksumsCache.generation[connKey] {
		delete(loadChecksumsCache.entries, cacheKey)
		return nil, false
	}
	return cloneChecksumsMap(entry.result), true
}

func storeChecksumsCache(conn driver.Conn, keys []string, result map[string][32]byte) {
	cacheKey := checksumsCacheKey(conn, keys)
	connKey := connSupportKey(conn)
	loadChecksumsCache.mu.Lock()
	loadChecksumsCache.entries[cacheKey] = checksumsCacheEntry{
		generation: loadChecksumsCache.generation[connKey],
		result:     cloneChecksumsMap(result),
	}
	loadChecksumsCache.mu.Unlock()
}

func bumpChecksumsCacheGeneration(conn driver.Conn) {
	connKey := connSupportKey(conn)
	loadChecksumsCache.mu.Lock()
	loadChecksumsCache.generation[connKey]++
	loadChecksumsCache.mu.Unlock()
}

func checksumsCacheKey(conn driver.Conn, keys []string) string {
	var b strings.Builder
	connKey := connSupportKey(conn)
	total := len(connKey)
	for _, key := range keys {
		total += 1 + len(key)
	}
	b.Grow(total)
	b.WriteString(connKey)
	for _, key := range keys {
		b.WriteByte('\x00')
		b.WriteString(key)
	}
	return b.String()
}

func cloneChecksumsMap(src map[string][32]byte) map[string][32]byte {
	dst := make(map[string][32]byte, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func parseHistoryChecksum(key, s string) ([32]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return [32]byte{}, nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return [32]byte{}, fmt.Errorf("history checksum %s: %w", key, err)
	}
	if len(b) != 32 {
		return [32]byte{}, fmt.Errorf("history checksum %s: decoded length %d, want 32", key, len(b))
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}

func LoadAppliedMigrations(ctx context.Context, conn driver.Conn, tableKey string) (map[string]bool, error) {
	result := make(map[string]bool)
	rows, err := conn.QueryContext(ctx, loadMigrationsSQL, tableKey+"/%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func LoadAllAppliedMigrations(ctx context.Context, conn driver.Conn) (map[string]bool, error) {
	result := make(map[string]bool)
	rows, err := conn.QueryContext(ctx, loadAllMigrationsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
