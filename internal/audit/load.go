package audit

import (
	"context"
	_ "embed"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/load_checksums.sql
var loadChecksumsSQL string

//go:embed sql/bootstrap.sql
var bootstrapSQL string

//go:embed sql/load_migrations.sql
var loadMigrationsSQL string

func EnsureTables(ctx context.Context, conn driver.Conn) error {
	_, err := conn.ExecContext(ctx, bootstrapSQL)
	return err
}

func LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(keys))
	chunks := types.ChunkKeys(keys, driver.DefaultMaxParameters)
	for _, chunk := range chunks {
		query, args := types.BuildINQuery(loadChecksumsSQL, "{{keys}}", chunk)
		rows, err := conn.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var key, checksum string
			if err := rows.Scan(&key, &checksum); err != nil {
				return nil, err
			}
			result[key] = checksum
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
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
