package audit

import (
	"context"
	_ "embed"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/load_checksums.sql
var loadChecksumsSQL string

func LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(keys))
	chunks := types.ChunkKeys(keys, types.SQLServerMaxParameters)
	for _, chunk := range chunks {
		query, args := types.BuildINQuery(loadChecksumsSQL, "{{keys}}", chunk)
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
			result[key] = checksum
		}
		rows.Close()
	}
	return result, nil
}
