package audit

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

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
	chunks := chunkKeys(keys, types.SQLServerMaxParameters)
	for _, chunk := range chunks {
		query, args := buildIN(loadChecksumsSQL, "{{keys}}", chunk)
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
	}
	return result, nil
}

func chunkKeys(keys []string, size int) [][]string {
	if len(keys) == 0 {
		return nil
	}
	var chunks [][]string
	for i := 0; i < len(keys); i += size {
		end := i + size
		if end > len(keys) {
			end = len(keys)
		}
		chunks = append(chunks, keys[i:end])
	}
	return chunks
}

func buildIN(template, placeholder string, keys []string) (string, []any) {
	parts := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("@p%d", i+1)
		args[i] = k
	}
	query := strings.Replace(template, placeholder, strings.Join(parts, ", "), 1)
	return query, args
}
