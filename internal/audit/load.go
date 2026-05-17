package audit

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/load_checksums.sql
var loadChecksumsSQL string

//go:embed sql/bootstrap.sql
var bootstrapSQL string

//go:embed sql/load_migrations.sql
var loadMigrationsSQL string

//go:embed sql/load_all_migrations.sql
var loadAllMigrationsSQL string

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
	result := make(map[string][32]byte, len(keys))
	chunks := types.ChunkKeys(keys, driver.DefaultMaxParameters)
	for _, chunk := range chunks {
		query, args := types.BuildINQuery(loadChecksumsSQL, "{{keys}}", chunk, 1)
		rows, err := conn.QueryContext(ctx, query, args...)
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
	return result, nil
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
