package audit

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/load_checksums_openjson.sql
var loadChecksumsOpenJSONSQL string

//go:embed sql/bootstrap.sql
var bootstrapSQL string

//go:embed sql/load_migrations.sql
var loadMigrationsSQL string

//go:embed sql/load_all_migrations.sql
var loadAllMigrationsSQL string

var loadChecksumsCache = struct {
	generation map[string]uint64
	entries    map[string]checksumsCacheEntry
	mu         sync.Mutex
}{
	generation: make(map[string]uint64),
	entries:    make(map[string]checksumsCacheEntry),
}

var loadChecksumsLatest = struct {
	byConn map[string]map[string]latestChecksumEntry
	mu     sync.Mutex
}{
	byConn: make(map[string]map[string]latestChecksumEntry),
}

type checksumsCacheEntry struct {
	result     map[string][32]byte
	generation uint64
}

type latestChecksumEntry struct {
	sum   [32]byte
	known bool
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
	if result, ok := lookupLatestChecksums(conn, keys); ok {
		return result, nil
	}
	if cached, ok := lookupChecksumsCache(conn, keys); ok {
		return cached, nil
	}
	result, err := loadChecksumsOpenJSON(ctx, conn, keys)
	if err != nil {
		return nil, err
	}
	storeChecksumsCache(conn, keys, result)
	storeLatestChecksums(conn, keys, result)
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
	return result, nil
}

func lookupChecksumsCache(conn driver.Conn, keys []string) (map[string][32]byte, bool) {
	cacheKey := checksumsCacheKey(conn, keys)
	connKey := driver.ConnStableKey(conn)
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
	connKey := driver.ConnStableKey(conn)
	loadChecksumsCache.mu.Lock()
	loadChecksumsCache.entries[cacheKey] = checksumsCacheEntry{
		generation: loadChecksumsCache.generation[connKey],
		result:     cloneChecksumsMap(result),
	}
	loadChecksumsCache.mu.Unlock()
}

func bumpChecksumsCacheGeneration(conn driver.Conn) {
	connKey := driver.ConnStableKey(conn)
	loadChecksumsCache.mu.Lock()
	loadChecksumsCache.generation[connKey]++
	loadChecksumsCache.mu.Unlock()
}

func checksumsCacheKey(conn driver.Conn, keys []string) string {
	var b strings.Builder
	connKey := driver.ConnStableKey(conn)
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

func lookupLatestChecksums(conn driver.Conn, keys []string) (map[string][32]byte, bool) {
	connKey := driver.ConnStableKey(conn)
	loadChecksumsLatest.mu.Lock()
	entries, ok := loadChecksumsLatest.byConn[connKey]
	loadChecksumsLatest.mu.Unlock()
	if !ok {
		return nil, false
	}
	result := make(map[string][32]byte, len(keys))
	for _, key := range keys {
		entry, ok := entries[key]
		if !ok || !entry.known {
			return nil, false
		}
		if entry.sum != ([32]byte{}) {
			result[key] = entry.sum
		}
	}
	return result, true
}

func storeLatestChecksums(conn driver.Conn, keys []string, result map[string][32]byte) {
	connKey := driver.ConnStableKey(conn)
	loadChecksumsLatest.mu.Lock()
	entries := loadChecksumsLatest.byConn[connKey]
	if entries == nil {
		entries = make(map[string]latestChecksumEntry, len(keys))
		loadChecksumsLatest.byConn[connKey] = entries
	}
	for _, key := range keys {
		sum := result[key]
		entries[key] = latestChecksumEntry{sum: sum, known: true}
	}
	loadChecksumsLatest.mu.Unlock()
}

func storeLatestChecksumsFromHistory(conn driver.Conn, records []historyRecord) {
	connKey := driver.ConnStableKey(conn)
	loadChecksumsLatest.mu.Lock()
	entries := loadChecksumsLatest.byConn[connKey]
	if entries == nil {
		entries = make(map[string]latestChecksumEntry, len(records))
		loadChecksumsLatest.byConn[connKey] = entries
	}
	for _, rec := range records {
		if rec.ev == nil || rec.ev.RecordKind != "object" {
			continue
		}
		if rec.event != "applied" && rec.event != "adopted" {
			continue
		}
		sum, err := parseHistoryChecksum(rec.ev.NormalizedKey, rec.ev.Checksum)
		if err != nil {
			continue
		}
		entries[rec.ev.NormalizedKey] = latestChecksumEntry{sum: sum, known: true}
	}
	loadChecksumsLatest.mu.Unlock()
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
